package problem

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// PackageImportOptions controls conflict handling and publishing for one
// kapOJ or HydroOJ import request.
type PackageImportOptions struct {
	ConflictPolicy string
	OverwriteSlugs []string
	SkipSlugs      []string
	CanManageAll   bool
	Publish        bool
	CanPublish     bool
	// MaxZipBytes and MaxUncompressedBytes are per-request limits. Zero keeps
	// the backward-compatible defaults.
	MaxZipBytes          int64
	MaxUncompressedBytes int64
}

// PackageImportResult reports a kapOJ/HydroOJ import. Per-problem failures do
// not roll back other problems.
type PackageImportResult struct {
	Imported  []PackageImportedProblem `json:"imported"`
	Failed    []PackageImportFailure   `json:"failed"`
	Conflicts []PackageImportConflict  `json:"conflicts,omitempty"`
	Skipped   []PackageImportSkipped   `json:"skipped,omitempty"`
	Warnings  []string                 `json:"warnings,omitempty"`
}

type PackageImportedProblem struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	TestcaseCount int    `json:"testcaseCount"`
	Overwritten   bool   `json:"overwritten"`
	Published     bool   `json:"published"`
	PublishError  string `json:"publishError,omitempty"`
}

type PackageImportFailure struct {
	Folder string `json:"folder"`
	Title  string `json:"title"`
	Error  string `json:"error"`
}

type PackageImportConflict struct {
	Slug          string `json:"slug"`
	Folder        string `json:"folder"`
	ExistingID    int64  `json:"existingProblemId"`
	ExistingTitle string `json:"existingTitle"`
	CanOverwrite  bool   `json:"canOverwrite"`
}

type PackageImportSkipped struct {
	Slug   string `json:"slug"`
	Folder string `json:"folder"`
}

type kapOJArchive struct {
	files  map[string]*zip.File
	actual int64
	limit  int64
}

type kapOJDescriptor struct {
	prefix   string
	folder   string
	manifest *PackageManifest
	err      string
}

// ImportPackage detects a kapOJ single/batch package or a HydroOJ package and
// imports it. Publishing is attempted per problem after the import commits.
func (s *Service) ImportPackage(
	ctx context.Context,
	createdBy int64,
	r io.ReaderAt,
	size int64,
	options PackageImportOptions,
) (*PackageImportResult, error) {
	opt := options
	if opt.ConflictPolicy == "" {
		opt.ConflictPolicy = "ask"
	}
	if opt.ConflictPolicy != "ask" && opt.ConflictPolicy != "skip" && opt.ConflictPolicy != "overwrite" {
		return nil, httpx.ErrValidation("conflictPolicy 必须是 ask、skip 或 overwrite")
	}
	if opt.Publish && !opt.CanPublish {
		return nil, httpx.ErrForbidden
	}
	zipLimit := opt.MaxZipBytes
	if zipLimit <= 0 {
		zipLimit = maxHydroZipBytes
	}
	if size > zipLimit {
		return nil, httpx.ErrValidation(fmt.Sprintf("题包文件不能超过 %d MiB", zipLimit>>20))
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, httpx.ErrValidation("题包文件损坏或不是有效的 zip 文件")
	}
	if len(zr.File) > maxHydroEntries {
		return nil, httpx.ErrValidation(fmt.Sprintf("题包条目数不能超过 %d", maxHydroEntries))
	}

	uncompressedLimit := opt.MaxUncompressedBytes
	if uncompressedLimit <= 0 {
		uncompressedLimit = s.hydroUncompressedLimit
	}
	if uncompressedLimit <= 0 {
		uncompressedLimit = maxHydroUncompressed
	}
	archive, err := newKapOJArchive(zr, uncompressedLimit)
	if err != nil {
		return nil, err
	}
	kind, prefix, err := detectPackageKind(archive)
	if err != nil {
		return nil, err
	}
	if kind == "hydro" {
		hydro, err := s.ImportHydro(ctx, createdBy, r, size, HydroImportOptions{
			ConflictPolicy:       opt.ConflictPolicy,
			OverwriteSlugs:       opt.OverwriteSlugs,
			SkipSlugs:            opt.SkipSlugs,
			CanManageAll:         opt.CanManageAll,
			Publish:              opt.Publish,
			CanPublish:           opt.CanPublish,
			MaxZipBytes:          zipLimit,
			MaxUncompressedBytes: uncompressedLimit,
		})
		if err != nil {
			return nil, err
		}
		return packageResultFromHydro(hydro), nil
	}

	result, err := s.importKapOJ(ctx, createdBy, archive, kind, prefix, opt)
	if err != nil {
		if errors.Is(err, errHydroPackageLimit) {
			return nil, httpx.ErrValidation("题包解压后大小超过限制")
		}
		var appErr *httpx.AppError
		if errors.As(err, &appErr) {
			return nil, err
		}
		slog.Error("problem package import failed", "err", err)
		return nil, httpx.ErrInternal
	}
	return result, nil
}

func newKapOJArchive(zr *zip.Reader, limit int64) (*kapOJArchive, error) {
	if limit <= 0 {
		limit = maxHydroUncompressed
	}
	archive := &kapOJArchive{files: map[string]*zip.File{}, limit: limit}
	var declared int64
	for _, f := range zr.File {
		declared += int64(f.UncompressedSize64)
		if declared > limit {
			return nil, httpx.ErrValidation("题包解压后大小超过限制")
		}
		if err := validateHydroEntry(f); err != nil {
			return nil, err
		}
		if strings.HasSuffix(f.Name, "/") || f.FileInfo().IsDir() || skipHydroEntry(f.Name) {
			continue
		}
		if _, exists := archive.files[f.Name]; exists {
			return nil, httpx.ErrValidation("题包包含重复文件：" + f.Name)
		}
		archive.files[f.Name] = f
	}
	return archive, nil
}

func detectPackageKind(archive *kapOJArchive) (kind, prefix string, err error) {
	if f, ok := archive.files["manifest.json"]; ok {
		format, err := manifestFormat(f, archive)
		if err != nil {
			return "", "", err
		}
		switch format {
		case PackageFormatProblem:
			return format, "", nil
		case PackageFormatProblems:
			return format, "", nil
		default:
			return "", "", httpx.ErrValidation("不支持的题包格式：" + format)
		}
	}

	var nested []string
	for name := range archive.files {
		if path.Base(name) == "manifest.json" && strings.Count(name, "/") == 1 {
			nested = append(nested, name)
		}
	}
	if len(nested) == 1 {
		format, err := manifestFormat(archive.files[nested[0]], archive)
		if err != nil {
			return "", "", err
		}
		if format == PackageFormatProblem || format == PackageFormatProblems {
			return format, path.Dir(nested[0]), nil
		}
	}
	for name := range archive.files {
		if path.Base(name) == "problem.yaml" {
			return "hydro", "", nil
		}
	}
	return "", "", httpx.ErrValidation("无法识别题包格式：缺少 kapOJ manifest.json 或 HydroOJ problem.yaml")
}

func manifestFormat(f *zip.File, archive *kapOJArchive) (string, error) {
	data, err := readHydroEntry(f, maxHydroYAMLBytes, &archive.actual)
	if err != nil {
		return "", httpx.ErrValidation("题包 manifest.json 读取失败")
	}
	if archive.actual > archive.limit {
		return "", httpx.ErrValidation("题包解压后大小超过限制")
	}
	var envelope struct {
		Format        string `json:"format"`
		FormatVersion int    `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", httpx.ErrValidation("题包 manifest.json 不是有效 JSON")
	}
	if envelope.FormatVersion != PackageFormatVersion {
		return "", httpx.ErrValidation("不支持的题包格式版本")
	}
	return envelope.Format, nil
}

func (s *Service) importKapOJ(
	ctx context.Context,
	createdBy int64,
	archive *kapOJArchive,
	kind, prefix string,
	opt PackageImportOptions,
) (*PackageImportResult, error) {
	result := &PackageImportResult{
		Imported: []PackageImportedProblem{},
		Failed:   []PackageImportFailure{},
	}
	limits := s.hydroTestcaseLimits(ctx)

	descriptors, err := readKapOJDescriptors(archive, kind, prefix)
	if err != nil {
		return nil, err
	}
	if len(descriptors) == 0 {
		return nil, httpx.ErrValidation("题包中未找到任何题目")
	}

	slugCounts := map[string]int{}
	for i := range descriptors {
		if descriptors[i].manifest != nil {
			slugCounts[descriptors[i].manifest.Problem.Slug]++
		}
	}

	existingBySlug := map[string]*Problem{}
	skippedSlugs := map[string]bool{}
	for i := range descriptors {
		desc := &descriptors[i]
		if desc.err != "" {
			result.Failed = append(result.Failed, PackageImportFailure{
				Folder: desc.folder, Error: desc.err,
			})
			continue
		}
		slug := desc.manifest.Problem.Slug
		if slugCounts[slug] > 1 {
			result.Failed = append(result.Failed, PackageImportFailure{
				Folder: desc.folder, Title: desc.manifest.Problem.Title,
				Error: "题目标识在题包中重复：" + slug,
			})
			desc.err = "题目标识在题包中重复"
			continue
		}
		existing, err := s.repo.FindBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			continue
		}
		existingBySlug[slug] = existing
		canOverwrite := opt.CanManageAll || existing.CreatedBy == createdBy
		switch {
		case packageContains(opt.SkipSlugs, slug):
			skippedSlugs[slug] = true
			delete(existingBySlug, slug)
			result.Skipped = append(result.Skipped, PackageImportSkipped{Slug: slug, Folder: desc.folder})
		case opt.ConflictPolicy == "ask":
			result.Conflicts = append(result.Conflicts, PackageImportConflict{
				Slug: slug, Folder: desc.folder, ExistingID: existing.ID,
				ExistingTitle: existing.Title, CanOverwrite: canOverwrite,
			})
		case opt.ConflictPolicy == "skip":
			skippedSlugs[slug] = true
			delete(existingBySlug, slug)
			result.Skipped = append(result.Skipped, PackageImportSkipped{Slug: slug, Folder: desc.folder})
		case opt.ConflictPolicy == "overwrite":
			if packageContains(opt.OverwriteSlugs, slug) || len(opt.OverwriteSlugs) == 0 {
				if !canOverwrite {
					result.Failed = append(result.Failed, PackageImportFailure{
						Folder: desc.folder, Title: existing.Title,
						Error: "无权覆盖他人题目：" + slug,
					})
					desc.err = "无权覆盖他人题目"
					skippedSlugs[slug] = true
					delete(existingBySlug, slug)
				}
			} else {
				skippedSlugs[slug] = true
				delete(existingBySlug, slug)
				result.Skipped = append(result.Skipped, PackageImportSkipped{Slug: slug, Folder: desc.folder})
			}
		}
	}
	if opt.ConflictPolicy == "ask" && len(result.Conflicts) > 0 {
		return result, nil
	}

	createdIDs := make([]int64, 0, len(descriptors))
	for i := range descriptors {
		desc := &descriptors[i]
		if desc.err != "" || desc.manifest == nil {
			continue
		}
		slug := desc.manifest.Problem.Slug
		if skippedSlugs[slug] {
			continue
		}
		imported, failure, err := s.importKapOJProblem(
			ctx, createdBy, archive, desc, existingBySlug[slug], opt, limits, &createdIDs)
		if err != nil {
			for _, id := range createdIDs {
				s.cleanupHydroProblem(ctx, id)
			}
			return nil, err
		}
		if failure != nil {
			result.Failed = append(result.Failed, *failure)
			continue
		}
		result.Imported = append(result.Imported, imported)
	}
	return result, nil
}

func readKapOJDescriptors(archive *kapOJArchive, kind, prefix string) ([]kapOJDescriptor, error) {
	if kind == PackageFormatProblem {
		manifest, err := readPackageManifest(archive, prefix)
		if err != nil {
			return nil, httpx.ErrValidation(packageErrorText(err))
		}
		folder := path.Base(prefix)
		if folder == "." || folder == "/" {
			folder = manifest.Problem.Slug
		}
		return []kapOJDescriptor{{prefix: prefix, folder: folder, manifest: manifest}}, nil
	}

	raw, err := archive.read(prefix, "manifest.json", maxHydroYAMLBytes)
	if err != nil {
		return nil, httpx.ErrValidation(packageErrorText(err))
	}
	var root ProblemsManifest
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, httpx.ErrValidation("批量题包 manifest.json 不是有效 JSON")
	}
	if root.Format != PackageFormatProblems || root.FormatVersion != PackageFormatVersion {
		return nil, httpx.ErrValidation("批量题包 manifest.json 格式不正确")
	}
	if len(root.Problems) == 0 {
		return nil, httpx.ErrValidation("批量题包中没有题目")
	}
	if len(root.Problems) > MaxPackageProblems {
		return nil, httpx.ErrValidation(fmt.Sprintf("题目数量不能超过 %d", MaxPackageProblems))
	}

	descriptors := make([]kapOJDescriptor, 0, len(root.Problems))
	seen := map[string]bool{}
	for _, entry := range root.Problems {
		dir := strings.TrimSuffix(strings.TrimSpace(entry.Directory), "/")
		if dir == "" && entry.Slug != "" {
			dir = "problems/" + entry.Slug
		}
		folder := path.Base(dir)
		fullDir := kapOJPath(prefix, dir)
		desc := kapOJDescriptor{prefix: fullDir, folder: folder}
		switch {
		case !safePackageRel(dir):
			desc.err = "题包目录不合法"
			desc.prefix = ""
		case seen[dir]:
			desc.err = "题包目录重复：" + dir
		default:
			seen[dir] = true
			manifest, err := readPackageManifest(archive, fullDir)
			if err != nil {
				desc.err = packageErrorText(err)
			} else {
				desc.manifest = manifest
			}
		}
		descriptors = append(descriptors, desc)
	}
	return descriptors, nil
}

func readPackageManifest(archive *kapOJArchive, prefix string) (*PackageManifest, error) {
	raw, err := archive.read(prefix, "manifest.json", maxHydroYAMLBytes)
	if err != nil {
		return nil, err
	}
	var manifest PackageManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("manifest.json 不是有效 JSON")
	}
	if manifest.Format != PackageFormatProblem {
		return nil, fmt.Errorf("manifest.json 的 format 必须是 kapoj-problem")
	}
	if manifest.FormatVersion != PackageFormatVersion {
		return nil, fmt.Errorf("不支持的单题包格式版本")
	}
	if !slugPattern.MatchString(manifest.Problem.Slug) {
		return nil, fmt.Errorf("题目标识无效：%s", manifest.Problem.Slug)
	}
	if strings.TrimSpace(manifest.Problem.Title) == "" {
		return nil, fmt.Errorf("题目标题不能为空")
	}
	if !manifest.Problem.Type.Valid() {
		return nil, fmt.Errorf("题目类型无效：%s", manifest.Problem.Type)
	}
	if manifest.Problem.SolutionPriceCoins < 0 || manifest.Problem.SolutionPriceCoins > maxSolutionPrice {
		return nil, fmt.Errorf("官方题解价格不合法")
	}
	if _, err := resolvePackageDifficulty(manifest.Problem); err != nil {
		return nil, err
	}
	return &manifest, nil
}

type kapOJPrepared struct {
	input        Input
	statement    string
	testdata     TestdataManifest
	attachments  []PackageAttachment
	images       []PackageAttachment
	checker      string
	solutionMD   string
	solutionCode string
	price        int64
}

// packageImportDifficultyDecision is the difficulty metadata written for an
// imported problem: the package score becomes both current and prior, the
// problem stays automatic, and the importer is recorded as the actor.
func packageImportDifficultyDecision(score int, actorID int64) DifficultyDecision {
	return DifficultyDecision{
		Score: score, Prior: score,
		PriorSource: DifficultyPriorSourceImport,
		Source:      DifficultySourceImport,
		Auto:        true, ActorID: actorID,
	}
}

func (s *Service) importKapOJProblem(
	ctx context.Context,
	createdBy int64,
	archive *kapOJArchive,
	desc *kapOJDescriptor,
	existing *Problem,
	opt PackageImportOptions,
	limits TestcaseLimits,
	createdIDs *[]int64,
) (PackageImportedProblem, *PackageImportFailure, error) {
	manifest := desc.manifest
	title := manifest.Problem.Title
	fail := func(msg string) (PackageImportedProblem, *PackageImportFailure, error) {
		return PackageImportedProblem{}, &PackageImportFailure{
			Folder: desc.folder, Title: title, Error: msg,
		}, nil
	}
	prepared, err := prepareKapOJProblem(archive, desc.prefix, manifest, limits)
	if err != nil {
		if errors.Is(err, errHydroPackageLimit) {
			return PackageImportedProblem{}, nil, err
		}
		return fail(packageErrorText(err))
	}
	if existing != nil {
		return s.replaceKapOJProblem(ctx, createdBy, archive, desc, existing, prepared, opt, limits)
	}

	created, err := s.CreateWithDifficulty(ctx, createdBy, prepared.input,
		packageImportDifficultyDecision(prepared.input.Difficulty, createdBy))
	if err != nil {
		return fail(packageErrorText(err))
	}
	problemID := created.ID
	*createdIDs = append(*createdIDs, problemID)
	cleanup := func() {
		s.cleanupHydroProblem(ctx, problemID)
		if n := len(*createdIDs); n > 0 && (*createdIDs)[n-1] == problemID {
			*createdIDs = (*createdIDs)[:n-1]
		}
	}

	if _, err := s.addKapOJTestcases(ctx, problemID, archive, desc.prefix, prepared.testdata, limits.MaxProblemTotalBytes); err != nil {
		cleanup()
		if errors.Is(err, errHydroPackageLimit) {
			return PackageImportedProblem{}, nil, err
		}
		return fail(packageErrorText(err))
	}

	if prepared.checker != "" {
		f, err := archive.file(desc.prefix, prepared.checker)
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		rc, err := f.Open()
		if err != nil {
			cleanup()
			return fail("checker 读取失败")
		}
		err = s.UploadChecker(ctx, problemID, &countingReader{r: rc, n: &archive.actual})
		_ = rc.Close()
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
	}

	for _, item := range prepared.attachments {
		if _, err := s.addKapOJAttachment(ctx, problemID, desc.prefix, archive, item, AttachmentKindFile); err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
	}
	addedImages := make([]*ProblemAttachment, 0, len(prepared.images))
	for _, item := range prepared.images {
		attachment, err := s.addKapOJAttachment(ctx, problemID, desc.prefix, archive, item, AttachmentKindStatementImage)
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		addedImages = append(addedImages, attachment)
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
	}

	imageURLs := make(map[string]string, len(prepared.images))
	for i, image := range prepared.images {
		imageURLs[image.File] = fmt.Sprintf("/api/problems/%d/assets/%d", problemID, addedImages[i].ID)
	}
	rewritten := rewriteImagePlaceholders(prepared.statement, imageURLs)
	if rewritten != prepared.statement {
		prepared.input.StatementMD = rewritten
		if _, err := s.Update(ctx, problemID, prepared.input); err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
	}

	if err := s.repo.UpdateSolution(ctx, problemID,
		prepared.solutionMD, prepared.solutionCode, prepared.price); err != nil {
		cleanup()
		return fail(packageErrorText(err))
	}

	imported := PackageImportedProblem{
		ID: problemID, Title: title, Slug: manifest.Problem.Slug,
		TestcaseCount: len(prepared.testdata.Testcases),
	}
	if opt.Publish {
		if err := s.Publish(ctx, problemID); err != nil {
			imported.PublishError = packageErrorText(err)
		} else {
			imported.Published = true
		}
	}
	return imported, nil, nil
}

// replaceKapOJProblem stages every new file first, then swaps the problem rows
// in one transaction. A failure before or during the transaction leaves the
// original problem and its files untouched.
func (s *Service) replaceKapOJProblem(
	ctx context.Context,
	createdBy int64,
	archive *kapOJArchive,
	desc *kapOJDescriptor,
	existing *Problem,
	prepared *kapOJPrepared,
	opt PackageImportOptions,
	limits TestcaseLimits,
) (PackageImportedProblem, *PackageImportFailure, error) {
	manifest := desc.manifest
	title := manifest.Problem.Title
	fail := func(msg string) (PackageImportedProblem, *PackageImportFailure, error) {
		return PackageImportedProblem{}, &PackageImportFailure{
			Folder: desc.folder, Title: title, Error: msg,
		}, nil
	}
	problemID := existing.ID
	ids, err := s.repo.AllocateAttachmentIDs(ctx, len(prepared.attachments)+len(prepared.images))
	if err != nil {
		return fail(packageErrorText(err))
	}
	stagedKeys := make([]string, 0)
	cleanup := func() { s.deleteFiles(ctx, stagedKeys) }

	attachments := make([]*ProblemAttachment, 0, len(prepared.attachments))
	images := make([]*ProblemAttachment, 0, len(prepared.images))
	nextID := 0
	stageOne := func(item PackageAttachment, kind string) (*ProblemAttachment, error) {
		f, err := archive.file(desc.prefix, item.File)
		if err != nil {
			return nil, err
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		filename := item.Filename
		if strings.TrimSpace(filename) == "" {
			filename = path.Base(item.File)
		}
		attachment, err := s.stageAttachment(ctx, problemID, 0, kind, AttachmentUpload{
			Filename: filename, ContentType: item.ContentType,
			Reader: &countingReader{r: rc, n: &archive.actual},
		})
		if err != nil {
			return nil, err
		}
		attachment.ID = ids[nextID]
		nextID++
		stagedKeys = append(stagedKeys, attachment.StorageKey)
		return attachment, nil
	}

	for _, item := range prepared.attachments {
		attachment, err := stageOne(item, AttachmentKindFile)
		if err != nil {
			cleanup()
			if errors.Is(err, errHydroPackageLimit) {
				return PackageImportedProblem{}, nil, err
			}
			return fail(packageErrorText(err))
		}
		attachments = append(attachments, attachment)
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
	}
	for _, item := range prepared.images {
		attachment, err := stageOne(item, AttachmentKindStatementImage)
		if err != nil {
			cleanup()
			if errors.Is(err, errHydroPackageLimit) {
				return PackageImportedProblem{}, nil, err
			}
			return fail(packageErrorText(err))
		}
		images = append(images, attachment)
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
	}

	testdata := append([]TestcaseManifest(nil), prepared.testdata.Testcases...)
	sort.SliceStable(testdata, func(i, j int) bool { return testdata[i].SortOrder < testdata[j].SortOrder })
	testcases := make([]*Testcase, 0, len(testdata))
	var testcaseBytes int64
	for _, item := range testdata {
		inFile, err := archive.file(desc.prefix, item.Input)
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		outFile, err := archive.file(desc.prefix, item.Output)
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		inRC, err := inFile.Open()
		if err != nil {
			cleanup()
			return fail("测试数据读取失败")
		}
		outRC, err := outFile.Open()
		if err != nil {
			_ = inRC.Close()
			cleanup()
			return fail("测试数据读取失败")
		}
		tc, keys, err := s.stageTestcase(ctx, problemID, TestcaseUpload{
			Input:  &countingReader{r: inRC, n: &archive.actual},
			Output: &countingReader{r: outRC, n: &archive.actual},
			Score:  item.Score, IsSample: item.IsSample,
		})
		_ = inRC.Close()
		_ = outRC.Close()
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		stagedKeys = append(stagedKeys, keys...)
		testcases = append(testcases, tc)
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
		testcaseBytes += tc.InputSize + tc.OutputSize
		if err := checkProblemTestcaseTotal(testcaseBytes, limits.MaxProblemTotalBytes); err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
	}

	checkerPath := ""
	if prepared.checker != "" {
		f, err := archive.file(desc.prefix, prepared.checker)
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		rc, err := f.Open()
		if err != nil {
			cleanup()
			return fail("checker 读取失败")
		}
		key, err := s.stageChecker(ctx, problemID, &countingReader{r: rc, n: &archive.actual})
		_ = rc.Close()
		if err != nil {
			cleanup()
			return fail(packageErrorText(err))
		}
		stagedKeys = append(stagedKeys, key)
		checkerPath = key
		if archive.actual > archive.limit {
			cleanup()
			return PackageImportedProblem{}, nil, errHydroPackageLimit
		}
	}

	imageURLs := make(map[string]string, len(images))
	for i, image := range prepared.images {
		imageURLs[image.File] = fmt.Sprintf("/api/problems/%d/assets/%d", problemID, images[i].ID)
	}
	statement := rewriteImagePlaceholders(prepared.statement, imageURLs)

	updated := *existing
	apply(&updated, prepared.input)
	updated.ID = existing.ID
	updated.StatementMD = statement
	updated.IsPublic = false
	updated.CheckerPath = checkerPath
	updated.SolutionMD = prepared.solutionMD
	updated.SolutionCode = prepared.solutionCode
	updated.SolutionPriceCoins = prepared.price

	decision := packageImportDifficultyDecision(prepared.input.Difficulty, createdBy)
	applyDifficultyDecision(&updated, decision)
	history := newDifficultyHistory(existing.Difficulty, decision)

	oldKeys, err := s.repo.ReplaceContentWithHistory(
		ctx, &updated, testcases, append(attachments, images...), history)
	if err != nil {
		cleanup()
		return fail(packageErrorText(err))
	}
	s.deleteFiles(ctx, oldKeys)

	imported := PackageImportedProblem{
		ID: problemID, Title: title, Slug: manifest.Problem.Slug,
		TestcaseCount: len(testcases), Overwritten: true,
	}
	if opt.Publish {
		if err := s.Publish(ctx, problemID); err != nil {
			imported.PublishError = packageErrorText(err)
		} else {
			imported.Published = true
		}
	}
	return imported, nil, nil
}

func prepareKapOJProblem(archive *kapOJArchive, prefix string, manifest *PackageManifest, limits TestcaseLimits) (*kapOJPrepared, error) {
	files := manifest.Files
	statement, err := archive.read(prefix, files.Statement, maxHydroStatementBytes)
	if err != nil {
		return nil, fmt.Errorf("题面读取失败：%w", err)
	}
	if strings.TrimSpace(string(statement)) == "" {
		return nil, fmt.Errorf("题面不能为空")
	}
	inputFormat, err := archive.readOptional(prefix, files.InputFormat, maxHydroStatementBytes)
	if err != nil {
		return nil, fmt.Errorf("输入格式读取失败：%w", err)
	}
	outputFormat, err := archive.readOptional(prefix, files.OutputFormat, maxHydroStatementBytes)
	if err != nil {
		return nil, fmt.Errorf("输出格式读取失败：%w", err)
	}
	hint, err := archive.readOptional(prefix, files.Hint, maxHydroStatementBytes)
	if err != nil {
		return nil, fmt.Errorf("提示读取失败：%w", err)
	}

	samples := []Sample{}
	if files.Samples != "" {
		raw, err := archive.read(prefix, files.Samples, maxHydroYAMLBytes)
		if err != nil {
			return nil, fmt.Errorf("样例读取失败：%w", err)
		}
		if err := json.Unmarshal(raw, &samples); err != nil {
			return nil, fmt.Errorf("样例 JSON 无效")
		}
	}
	answers := []string{}
	if files.Answers != "" {
		raw, err := archive.read(prefix, files.Answers, maxHydroYAMLBytes)
		if err != nil {
			return nil, fmt.Errorf("答案读取失败：%w", err)
		}
		if err := json.Unmarshal(raw, &answers); err != nil {
			return nil, fmt.Errorf("答案 JSON 无效")
		}
	}

	testdata := TestdataManifest{Testcases: []TestcaseManifest{}}
	if files.Testdata != "" {
		raw, err := archive.read(prefix, files.Testdata, maxHydroYAMLBytes)
		if err != nil {
			return nil, fmt.Errorf("测试数据清单读取失败：%w", err)
		}
		if err := json.Unmarshal(raw, &testdata); err != nil {
			return nil, fmt.Errorf("测试数据清单不是有效 JSON")
		}
	}
	if err := validateKapOJTestdata(archive, prefix, testdata, limits); err != nil {
		return nil, err
	}

	attachmentFiles := map[string]bool{}
	var attachmentDeclared int64
	for _, item := range append(append([]PackageAttachment{}, files.Attachments...), files.Images...) {
		if !safePackageRel(item.File) {
			return nil, fmt.Errorf("附件路径不合法")
		}
		if attachmentFiles[item.File] {
			return nil, fmt.Errorf("附件文件重复：%s", item.File)
		}
		attachmentFiles[item.File] = true
		f, err := archive.file(prefix, item.File)
		if err != nil {
			return nil, err
		}
		attachmentDeclared += int64(f.UncompressedSize64)
		if attachmentDeclared > archive.limit {
			return nil, errHydroPackageLimit
		}
	}
	if len(files.Attachments)+len(files.Images) > maxAttachmentsPerProblem {
		return nil, fmt.Errorf("题目附件数量不能超过 %d 个", maxAttachmentsPerProblem)
	}

	if files.Checker != "" {
		if manifest.Problem.Type != TypeSpecial {
			return nil, fmt.Errorf("只有特判题可以包含 checker")
		}
		if !safePackageRel(files.Checker) {
			return nil, fmt.Errorf("checker 路径不合法")
		}
		f, err := archive.file(prefix, files.Checker)
		if err != nil {
			return nil, err
		}
		if int64(f.UncompressedSize64) > maxCheckerBytes {
			return nil, fmt.Errorf("checker 不能超过 %d KiB", maxCheckerBytes>>10)
		}
	}

	solutionMD := ""
	solutionCode := ""
	if files.Solution != "" {
		raw, err := archive.read(prefix, files.Solution, maxSolutionMDChars*4)
		if err != nil {
			return nil, fmt.Errorf("官方题解读取失败：%w", err)
		}
		solutionMD = string(raw)
	}
	if files.SolutionCode != "" {
		raw, err := archive.read(prefix, files.SolutionCode, maxSolutionCode)
		if err != nil {
			return nil, fmt.Errorf("官方题解代码读取失败：%w", err)
		}
		solutionCode = string(raw)
	}
	if runes(solutionMD) > maxSolutionMDChars {
		return nil, fmt.Errorf("官方题解说明不能超过 %d 个字符", maxSolutionMDChars)
	}
	if len(solutionCode) > maxSolutionCode {
		return nil, fmt.Errorf("官方题解代码不能超过 %d 字节", maxSolutionCode)
	}

	difficulty, err := resolvePackageDifficulty(manifest.Problem)
	if err != nil {
		return nil, err
	}
	input := Input{
		Title: manifest.Problem.Title, Slug: manifest.Problem.Slug,
		StatementMD: string(statement), InputFormatMD: string(inputFormat),
		OutputFormatMD: string(outputFormat), Samples: samples, HintMD: string(hint),
		Type: manifest.Problem.Type, Answers: answers, Tags: manifest.Problem.Tags,
		Difficulty: difficulty, TimeLimitMS: manifest.Problem.TimeLimitMS,
		MemoryLimitMB: manifest.Problem.MemoryLimitMB, IsPublic: false,
		Source: manifest.Problem.Source,
	}
	normalized, err := input.Normalize()
	if err != nil {
		return nil, err
	}

	return &kapOJPrepared{
		input: normalized, statement: string(statement), testdata: testdata,
		attachments: files.Attachments, images: files.Images, checker: files.Checker,
		solutionMD: solutionMD, solutionCode: solutionCode,
		price: manifest.Problem.SolutionPriceCoins,
	}, nil
}

func validateKapOJTestdata(archive *kapOJArchive, prefix string, meta TestdataManifest, limits TestcaseLimits) error {
	if len(meta.Testcases) > maxHydroTestcases {
		return fmt.Errorf("测试点数量不能超过 %d", maxHydroTestcases)
	}
	used := map[string]bool{}
	var total int64
	for _, tc := range meta.Testcases {
		if tc.Score < 0 || tc.Score > maxScore {
			return fmt.Errorf("测试点分数需为 0–%d", maxScore)
		}
		for _, rel := range []string{tc.Input, tc.Output} {
			if !safePackageRel(rel) || used[rel] {
				return fmt.Errorf("测试数据路径不合法或重复")
			}
			used[rel] = true
			f, err := archive.file(prefix, rel)
			if err != nil {
				return err
			}
			size := int64(f.UncompressedSize64)
			if rel == tc.Input {
				if size > limits.MaxInputBytes {
					return fmt.Errorf("输入文件不能超过 %s：%s", formatLimitBytes(limits.MaxInputBytes), rel)
				}
			} else if size > limits.MaxOutputBytes {
				return fmt.Errorf("输出文件不能超过 %s：%s", formatLimitBytes(limits.MaxOutputBytes), rel)
			}
			total += size
		}
	}
	if total > limits.MaxProblemTotalBytes {
		return fmt.Errorf("单题测试数据不能超过 %s", formatLimitBytes(limits.MaxProblemTotalBytes))
	}
	return nil
}

func (s *Service) addKapOJTestcases(
	ctx context.Context,
	problemID int64,
	archive *kapOJArchive,
	prefix string,
	meta TestdataManifest,
	problemLimit int64,
) ([]int64, error) {
	cases := append([]TestcaseManifest(nil), meta.Testcases...)
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].SortOrder < cases[j].SortOrder })
	added := make([]int64, 0, len(cases))
	var problemBytes int64
	for _, item := range cases {
		inFile, err := archive.file(prefix, item.Input)
		if err != nil {
			return added, err
		}
		outFile, err := archive.file(prefix, item.Output)
		if err != nil {
			return added, err
		}
		inRC, err := inFile.Open()
		if err != nil {
			return added, err
		}
		outRC, err := outFile.Open()
		if err != nil {
			_ = inRC.Close()
			return added, err
		}
		tc, err := s.AddTestcase(ctx, problemID, TestcaseUpload{
			Input:  &countingReader{r: inRC, n: &archive.actual},
			Output: &countingReader{r: outRC, n: &archive.actual},
			Score:  item.Score, IsSample: item.IsSample,
		})
		_ = inRC.Close()
		_ = outRC.Close()
		if err != nil {
			return added, err
		}
		added = append(added, tc.ID)
		if archive.actual > archive.limit {
			return added, errHydroPackageLimit
		}
		problemBytes += tc.InputSize + tc.OutputSize
		if err := checkProblemTestcaseTotal(problemBytes, problemLimit); err != nil {
			return added, err
		}
	}
	return added, nil
}

func (s *Service) addKapOJAttachment(
	ctx context.Context,
	problemID int64,
	prefix string,
	archive *kapOJArchive,
	item PackageAttachment,
	kind string,
) (*ProblemAttachment, error) {
	f, err := archive.file(prefix, item.File)
	if err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	filename := item.Filename
	if strings.TrimSpace(filename) == "" {
		filename = path.Base(item.File)
	}
	upload := AttachmentUpload{
		Filename: filename, ContentType: item.ContentType,
		Reader: &countingReader{r: rc, n: &archive.actual},
	}
	if kind == AttachmentKindStatementImage {
		return s.AddStatementImage(ctx, problemID, 0, upload)
	}
	return s.AddAttachment(ctx, problemID, 0, upload)
}

// Publish runs the normal publish validation for one problem and keeps it
// offline when the validation fails.
func (s *Service) Publish(ctx context.Context, id int64) error {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoErr(err)
	}
	in := inputFromProblem(p)
	in.IsPublic = true
	_, err = s.Update(ctx, id, in)
	return err
}

func inputFromProblem(p *Problem) Input {
	return Input{
		Title: p.Title, Slug: p.Slug, StatementMD: p.StatementMD,
		InputFormatMD: p.InputFormatMD, OutputFormatMD: p.OutputFormatMD,
		Samples: []Sample(p.Samples), HintMD: p.HintMD,
		Type: p.Type, Answers: []string(p.Answers), Tags: []string(p.Tags),
		Difficulty: p.Difficulty, TimeLimitMS: p.TimeLimitMS,
		MemoryLimitMB: p.MemoryLimitMB, IsPublic: p.IsPublic, Source: p.Source,
	}
}

func (a *kapOJArchive) file(prefix, rel string) (*zip.File, error) {
	if !safePackageRel(rel) {
		return nil, fmt.Errorf("题包文件路径不合法：%s", rel)
	}
	name := kapOJPath(prefix, rel)
	f, ok := a.files[name]
	if !ok {
		return nil, fmt.Errorf("题包缺少文件：%s", rel)
	}
	return f, nil
}

func (a *kapOJArchive) read(prefix, rel string, max int64) ([]byte, error) {
	f, err := a.file(prefix, rel)
	if err != nil {
		return nil, err
	}
	data, err := readHydroEntry(f, max, &a.actual)
	if err != nil {
		return nil, err
	}
	if a.actual > a.limit {
		return nil, errHydroPackageLimit
	}
	return data, nil
}

func (a *kapOJArchive) readOptional(prefix, rel string, max int64) ([]byte, error) {
	if rel == "" {
		return nil, nil
	}
	return a.read(prefix, rel, max)
}

func kapOJPath(prefix, rel string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return rel
	}
	if rel == "" {
		return prefix
	}
	return prefix + "/" + rel
}

// rewriteImagePlaceholders replaces package image placeholders with their new
// URLs. Longest-first ordering keeps "a.png.bak" from being partially rewritten
// by the shorter "a.png" placeholder.
func rewriteImagePlaceholders(statement string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return statement
	}
	keys := make([]string, 0, len(replacements))
	for file := range replacements {
		keys = append(keys, file)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, file := range keys {
		statement = strings.ReplaceAll(statement,
			packageImagePlaceholder(file), replacements[file])
	}
	return statement
}

func safePackageRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\\") {
		return false
	}
	clean := path.Clean(rel)
	return clean == rel && clean != "." && !strings.HasPrefix(clean, "../")
}

func packageContains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func packageErrorText(err error) string {
	if errors.Is(err, errHydroPackageLimit) {
		return "题包解压后大小超过限制"
	}
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return err.Error()
}

func packageResultFromHydro(hydro *HydroImportResult) *PackageImportResult {
	if hydro == nil {
		return &PackageImportResult{Imported: []PackageImportedProblem{}, Failed: []PackageImportFailure{}}
	}
	out := &PackageImportResult{
		Imported: make([]PackageImportedProblem, 0, len(hydro.Imported)),
		Failed:   make([]PackageImportFailure, 0, len(hydro.Failed)),
		Warnings: hydro.Warnings,
	}
	for _, item := range hydro.Imported {
		out.Imported = append(out.Imported, PackageImportedProblem{
			ID: item.ID, Title: item.Title, Slug: item.Slug,
			TestcaseCount: item.TestcaseCount, Overwritten: item.Overwritten,
			Published: item.Published, PublishError: item.PublishError,
		})
	}
	for _, item := range hydro.Failed {
		out.Failed = append(out.Failed, PackageImportFailure(item))
	}
	for _, item := range hydro.Conflicts {
		out.Conflicts = append(out.Conflicts, PackageImportConflict(item))
	}
	for _, item := range hydro.Skipped {
		out.Skipped = append(out.Skipped, PackageImportSkipped(item))
	}
	return out
}
