package problem

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

// WithPackageExportLimits overrides the uncompressed byte and entry caps used
// by package export. Non-positive values keep the defaults.
func WithPackageExportLimits(maxBytes int64, maxEntries int) Option {
	return func(s *Service) {
		if maxBytes > 0 {
			s.packageExportMaxBytes = maxBytes
		}
		if maxEntries > 0 {
			s.packageExportMaxEntries = maxEntries
		}
	}
}

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

	entries, size, err := s.preflightProblemPackage(ctx, p, testcases, attachments, images)
	if err != nil {
		return PackageManifest{}, err
	}
	if entries > s.packageExportMaxEntries || size > s.packageExportMaxBytes {
		return PackageManifest{}, httpx.ErrValidation("题包内容超过导出限制")
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

type problemPackageBundle struct {
	problem     *Problem
	testcases   []Testcase
	attachments []ProblemAttachment
	images      []ProblemAttachment
}

// ExportProblems writes one batch ZIP with a root manifest and one
// single-problem package per problems/<slug>/ directory. All ids are loaded
// before any byte is written so a missing problem cannot leak a partial ZIP.
func (s *Service) ExportProblems(ctx context.Context, ids []int64, w io.Writer) (ProblemsManifest, error) {
	if len(ids) == 0 {
		return ProblemsManifest{}, httpx.ErrValidation("请选择至少 1 道题目")
	}
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return ProblemsManifest{}, httpx.ErrValidation("题目 ID 不合法")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) > MaxPackageProblems {
		return ProblemsManifest{}, httpx.ErrValidation(fmt.Sprintf("一次最多导出 %d 道题目", MaxPackageProblems))
	}

	bundles := make([]problemPackageBundle, 0, len(unique))
	for _, id := range unique {
		p, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return ProblemsManifest{}, mapRepoErr(err)
		}
		testcases, err := s.repo.ListTestcases(ctx, id)
		if err != nil {
			return ProblemsManifest{}, err
		}
		attachments, err := s.repo.ListAttachments(ctx, id, AttachmentKindFile)
		if err != nil {
			return ProblemsManifest{}, err
		}
		images, err := s.repo.ListAttachments(ctx, id, AttachmentKindStatementImage)
		if err != nil {
			return ProblemsManifest{}, err
		}
		bundles = append(bundles, problemPackageBundle{
			problem: p, testcases: testcases, attachments: attachments, images: images,
		})
	}

	totalEntries := 1 // root manifest.json
	var totalSize int64
	for i := range bundles {
		bundle := &bundles[i]
		entries, size, err := s.preflightProblemPackage(ctx, bundle.problem,
			bundle.testcases, bundle.attachments, bundle.images)
		if err != nil {
			return ProblemsManifest{}, err
		}
		totalEntries += entries
		totalSize += size
	}
	if totalEntries > s.packageExportMaxEntries || totalSize > s.packageExportMaxBytes {
		return ProblemsManifest{}, httpx.ErrValidation("题包内容超过导出限制")
	}

	zw := zip.NewWriter(w)
	manifest := ProblemsManifest{
		Format:        PackageFormatProblems,
		FormatVersion: PackageFormatVersion,
		ExportedAt:    time.Now().UTC(),
		Problems:      make([]ProblemsManifestEntry, 0, len(bundles)),
	}
	for i := range bundles {
		bundle := &bundles[i]
		directory := "problems/" + bundle.problem.Slug
		if _, err := s.writeProblemPackage(ctx, zw, directory+"/", bundle.problem,
			bundle.testcases, bundle.attachments, bundle.images); err != nil {
			return ProblemsManifest{}, err
		}
		manifest.Problems = append(manifest.Problems, ProblemsManifestEntry{
			Slug: bundle.problem.Slug, Title: bundle.problem.Title, Directory: directory,
		})
	}
	if err := writePackageJSON(zw, "manifest.json", manifest); err != nil {
		return ProblemsManifest{}, err
	}
	if err := zw.Close(); err != nil {
		return ProblemsManifest{}, err
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
	imageReplacements := make(map[int64]string, len(images))
	for i := range images {
		item, err := s.writePackageAttachment(ctx, zw, prefix, "images", &images[i], usedImages)
		if err != nil {
			return PackageManifest{}, err
		}
		manifest.Files.Images = append(manifest.Files.Images, item)
		imageReplacements[images[i].ID] = packageImagePlaceholder(item.File)
	}

	statement := replaceStatementAssetURLs(p.StatementMD, p.ID, imageReplacements)
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
// replaceStatementAssetURLs rewrites exact /api/problems/<id>/assets/<assetId>
// URLs. Matching the numeric id as a whole avoids treating asset 4 as a prefix
// of asset 45.
func replaceStatementAssetURLs(statement string, problemID int64, replacements map[int64]string) string {
	if len(replacements) == 0 {
		return statement
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`/api/problems/%d/assets/(\d+)`, problemID))
	return pattern.ReplaceAllStringFunc(statement, func(match string) string {
		index := strings.LastIndexByte(match, '/')
		id, err := strconv.ParseInt(match[index+1:], 10, 64)
		if err != nil {
			return match
		}
		if replacement, ok := replacements[id]; ok {
			return replacement
		}
		return match
	})
}

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

func (s *Service) preflightProblemPackage(
	ctx context.Context,
	p *Problem,
	testcases []Testcase,
	attachments, images []ProblemAttachment,
) (int, int64, error) {
	entries := 7 // manifest, statement, input/output format, hint, samples, testdata meta
	size := int64(len(p.StatementMD)+len(p.InputFormatMD)+len(p.OutputFormatMD)+len(p.HintMD)) + 1024
	if len(p.Answers) > 0 {
		entries++
	}
	for i := range testcases {
		tc := &testcases[i]
		entries += 2
		size += tc.InputSize + tc.OutputSize
		if err := s.preflightStoredFile(ctx, "测试数据", tc.InputFile); err != nil {
			return 0, 0, err
		}
		if err := s.preflightStoredFile(ctx, "测试数据", tc.OutputFile); err != nil {
			return 0, 0, err
		}
	}
	if p.Type == TypeSpecial && p.CheckerPath != "" {
		entries++
		size += maxCheckerBytes
		if err := s.preflightStoredFile(ctx, "checker", p.CheckerPath); err != nil {
			return 0, 0, err
		}
	}
	entries += len(attachments) + len(images)
	for _, item := range append(append([]ProblemAttachment{}, attachments...), images...) {
		size += item.Size
		if err := s.preflightStoredFile(ctx, "附件", item.StorageKey); err != nil {
			return 0, 0, err
		}
	}
	if p.SolutionMD != "" || p.SolutionCode != "" {
		entries += 2
		size += int64(len(p.SolutionMD) + len(p.SolutionCode))
	}
	return entries, size, nil
}

func (s *Service) preflightStoredFile(ctx context.Context, label, key string) error {
	if key == "" {
		return httpx.ErrValidation("题包缺少" + label + "文件")
	}
	rc, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return httpx.ErrValidation("题包缺少" + label + "文件")
	}
	if err != nil {
		return err
	}
	return rc.Close()
}
