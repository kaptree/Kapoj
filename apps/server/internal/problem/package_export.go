package problem

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// ExportProblem writes one kapOJ problem package to w. The caller is
// responsible for checking that the request may download this problem.
func (s *Service) ExportProblem(ctx context.Context, id int64, w io.Writer) (PackageManifest, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return PackageManifest{}, mapRepoErr(err)
	}
	testcases, err := s.repo.ListTestcases(ctx, id)
	if err != nil {
		return PackageManifest{}, err
	}
	attachments, err := s.repo.ListAttachments(ctx, id, AttachmentKindFile)
	if err != nil {
		return PackageManifest{}, err
	}
	images, err := s.repo.ListAttachments(ctx, id, AttachmentKindStatementImage)
	if err != nil {
		return PackageManifest{}, err
	}

	zw := zip.NewWriter(w)
	manifest, err := s.writeProblemPackage(ctx, zw, "", p, testcases, attachments, images)
	if err != nil {
		return PackageManifest{}, err
	}
	if err := zw.Close(); err != nil {
		return PackageManifest{}, err
	}
	return manifest, nil
}

// writeProblemPackage writes one package below prefix ("" or "problems/<slug>/").
// File paths inside the manifest stay relative to the package root so a package
// extracted from a batch ZIP can be imported on its own.
func (s *Service) writeProblemPackage(
	ctx context.Context,
	zw *zip.Writer,
	prefix string,
	p *Problem,
	testcases []Testcase,
	attachments, images []ProblemAttachment,
) (PackageManifest, error) {
	manifest := PackageManifest{
		Format:        PackageFormatProblem,
		FormatVersion: PackageFormatVersion,
		ExportedAt:    time.Now().UTC(),
		Problem: PackageProblem{
			Slug: p.Slug, Title: p.Title, Type: p.Type, Difficulty: p.Difficulty,
			TimeLimitMS: p.TimeLimitMS, MemoryLimitMB: p.MemoryLimitMB,
			Source: p.Source, Tags: []string(p.Tags), IsPublic: p.IsPublic,
			SolutionPriceCoins: p.SolutionPriceCoins,
		},
	}
	if manifest.Problem.Tags == nil {
		manifest.Problem.Tags = []string{}
	}
	manifest.Files.Statement = "statement.md"
	manifest.Files.InputFormat = "input_format.md"
	manifest.Files.OutputFormat = "output_format.md"
	manifest.Files.Hint = "hint.md"
	manifest.Files.Samples = "samples.json"
	manifest.Files.Testdata = "testdata/meta.json"

	usedImages := map[string]bool{}
	imageReplacements := make([][2]string, 0, len(images))
	for i := range images {
		item, err := s.writePackageAttachment(ctx, zw, prefix, "images", &images[i], usedImages)
		if err != nil {
			return PackageManifest{}, err
		}
		manifest.Files.Images = append(manifest.Files.Images, item)
		original := fmt.Sprintf("/api/problems/%d/assets/%d", p.ID, images[i].ID)
		imageReplacements = append(imageReplacements, [2]string{original, packageImagePlaceholder(item.File)})
	}

	statement := p.StatementMD
	for _, replacement := range imageReplacements {
		statement = strings.ReplaceAll(statement, replacement[0], replacement[1])
	}
	if err := writePackageText(zw, prefix+manifest.Files.Statement, statement); err != nil {
		return PackageManifest{}, err
	}
	if err := writePackageText(zw, prefix+manifest.Files.InputFormat, p.InputFormatMD); err != nil {
		return PackageManifest{}, err
	}
	if err := writePackageText(zw, prefix+manifest.Files.OutputFormat, p.OutputFormatMD); err != nil {
		return PackageManifest{}, err
	}
	if err := writePackageText(zw, prefix+manifest.Files.Hint, p.HintMD); err != nil {
		return PackageManifest{}, err
	}
	samples := []Sample(p.Samples)
	if samples == nil {
		samples = []Sample{}
	}
	if err := writePackageJSON(zw, prefix+manifest.Files.Samples, samples); err != nil {
		return PackageManifest{}, err
	}
	if len(p.Answers) > 0 {
		manifest.Files.Answers = "answers.json"
		if err := writePackageJSON(zw, prefix+manifest.Files.Answers, []string(p.Answers)); err != nil {
			return PackageManifest{}, err
		}
	}

	testdata := TestdataManifest{Testcases: make([]TestcaseManifest, 0, len(testcases))}
	for i := range testcases {
		tc := &testcases[i]
		order := tc.SortOrder
		if order <= 0 {
			order = i + 1
		}
		item := TestcaseManifest{
			SortOrder: tc.SortOrder, Score: tc.Score, IsSample: tc.IsSample,
			Input:  fmt.Sprintf("testdata/%03d.in", order),
			Output: fmt.Sprintf("testdata/%03d.out", order),
		}
		if err := s.writePackageStoredFile(ctx, zw, prefix+item.Input, tc.InputFile); err != nil {
			return PackageManifest{}, err
		}
		if err := s.writePackageStoredFile(ctx, zw, prefix+item.Output, tc.OutputFile); err != nil {
			return PackageManifest{}, err
		}
		testdata.Testcases = append(testdata.Testcases, item)
	}
	if err := writePackageJSON(zw, prefix+manifest.Files.Testdata, testdata); err != nil {
		return PackageManifest{}, err
	}

	if p.Type == TypeSpecial && p.CheckerPath != "" {
		manifest.Files.Checker = "checker/checker"
		if err := s.writePackageStoredFile(ctx, zw, prefix+manifest.Files.Checker, p.CheckerPath); err != nil {
			return PackageManifest{}, err
		}
	}

	usedAttachments := map[string]bool{}
	for i := range attachments {
		item, err := s.writePackageAttachment(ctx, zw, prefix, "attachments", &attachments[i], usedAttachments)
		if err != nil {
			return PackageManifest{}, err
		}
		manifest.Files.Attachments = append(manifest.Files.Attachments, item)
	}

	if p.SolutionMD != "" || p.SolutionCode != "" {
		manifest.Files.Solution = "solution.md"
		manifest.Files.SolutionCode = "solution_code.txt"
		if err := writePackageText(zw, prefix+manifest.Files.Solution, p.SolutionMD); err != nil {
			return PackageManifest{}, err
		}
		if err := writePackageText(zw, prefix+manifest.Files.SolutionCode, p.SolutionCode); err != nil {
			return PackageManifest{}, err
		}
	}

	if err := writePackageJSON(zw, prefix+"manifest.json", manifest); err != nil {
		return PackageManifest{}, err
	}
	return manifest, nil
}

func (s *Service) writePackageAttachment(
	ctx context.Context,
	zw *zip.Writer,
	prefix, dir string,
	attachment *ProblemAttachment,
	used map[string]bool,
) (PackageAttachment, error) {
	name := packageUniqueName(attachment.Filename, used)
	entry := dir + "/" + name
	if err := s.writePackageStoredFile(ctx, zw, prefix+entry, attachment.StorageKey); err != nil {
		return PackageAttachment{}, err
	}
	return PackageAttachment{
		File: entry, Filename: attachment.Filename, ContentType: attachment.ContentType,
	}, nil
}

func (s *Service) writePackageStoredFile(ctx context.Context, zw *zip.Writer, name, key string) error {
	if key == "" {
		return fmt.Errorf("package entry %q has no storage key", name)
	}
	rc, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("package entry %q is missing from storage", name)
	}
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rc)
	return err
}

func writePackageText(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, content)
	return err
}

func writePackageJSON(zw *zip.Writer, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// packageUniqueName keeps an attachment's display name but makes the ZIP entry
// safe and unique. The manifest keeps the original filename separately.
func packageUniqueName(name string, used map[string]bool) string {
	base := sanitizeAttachmentFilename(name)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	candidate := base
	for i := 2; used[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s-%d%s", stem, i, ext)
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}
