package problem

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"go.yaml.in/yaml/v3"
)

const (
	// MaxHydroZipBytes caps an uploaded HydroOJ package file.
	MaxHydroZipBytes = 200 << 20
)

const (
	maxHydroZipBytes             = MaxHydroZipBytes
	maxHydroUncompressed         = 1 << 30
	maxHydroEntries              = 5000
	maxHydroProblems             = 100
	maxHydroTestcases            = 500
	maxHydroTestcaseBytes        = 16 << 20
	maxHydroProblemTestdataBytes = 200 << 20
	maxHydroYAMLBytes            = 1 << 20
	maxHydroStatementRunes       = 100_000
	maxHydroStatementBytes       = maxHydroStatementRunes * 4
)

const (
	hydroSource        = "HydroOJ"
	hydroTestcaseScore = 10
	hydroSlugPrefix    = "hydro-"
)

var errHydroTooLarge = errors.New("hydro: entry exceeds read limit")

// HydroImportResult reports the outcome of a HydroOJ package import.
type HydroImportResult struct {
	Imported []HydroImportedProblem `json:"imported"`
	Failed   []HydroImportFailure   `json:"failed"`
}

// HydroImportedProblem is a problem successfully created from a HydroOJ package.
type HydroImportedProblem struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	TestcaseCount int    `json:"testcaseCount"`
}

// HydroImportFailure records a problem that could not be imported.
type HydroImportFailure struct {
	Folder string `json:"folder"`
	Title  string `json:"title"`
	Error  string `json:"error"`
}

type hydroYAML struct {
	Title string   `yaml:"title"`
	Tags  []string `yaml:"tag"`
}

type hydroEntry struct {
	base string
	f    *zip.File
}

type hydroPair struct {
	in  *zip.File
	out *zip.File
}

// countingReader counts the bytes read through it, guarding against forged
// UncompressedSize64 values by tracking actual decompressed bytes.
type countingReader struct {
	r io.Reader
	n *int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

// ImportHydro parses a HydroOJ exported zip package and imports every valid
// problem as an offline problem owned by createdBy. A package that is damaged,
// empty, over its limits, or carries a dangerous entry is rejected whole with a
// validation error; otherwise per-problem failures are reported in Failed.
func (s *Service) ImportHydro(ctx context.Context, createdBy int64, r io.ReaderAt, size int64) (*HydroImportResult, error) {
	if size > maxHydroZipBytes {
		return nil, httpx.ErrValidation(fmt.Sprintf("题包文件不能超过 %d MiB", maxHydroZipBytes>>20))
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, httpx.ErrValidation("题包文件损坏或不是有效的 zip 文件")
	}
	if len(zr.File) > maxHydroEntries {
		return nil, httpx.ErrValidation(fmt.Sprintf("题包条目数不能超过 %d", maxHydroEntries))
	}

	result := &HydroImportResult{
		Imported: []HydroImportedProblem{},
		Failed:   []HydroImportFailure{},
	}

	byDir := map[string][]hydroEntry{}
	problemDirs := map[string]bool{}
	var declared int64
	for _, f := range zr.File {
		declared += int64(f.UncompressedSize64)
		if declared > maxHydroUncompressed {
			return nil, httpx.ErrValidation("题包解压后大小超过限制")
		}
		if err := validateHydroEntry(f); err != nil {
			return nil, err
		}
		name := f.Name
		if strings.HasSuffix(name, "/") || f.FileInfo().IsDir() {
			continue
		}
		if skipHydroEntry(name) {
			continue
		}
		dir := path.Dir(name)
		base := path.Base(name)
		byDir[dir] = append(byDir[dir], hydroEntry{base: base, f: f})
		if base == "problem.yaml" {
			problemDirs[dir] = true
		}
	}

	if len(problemDirs) == 0 {
		return nil, httpx.ErrValidation("题包中未找到任何题目文件夹")
	}
	if len(problemDirs) > maxHydroProblems {
		return nil, httpx.ErrValidation(fmt.Sprintf("题目文件夹数不能超过 %d", maxHydroProblems))
	}

	dirs := make([]string, 0, len(problemDirs))
	for d := range problemDirs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	// Compute slugs once so duplicate slugs within the package are all failed.
	slugs := make(map[string]string, len(dirs))
	validSlug := make(map[string]bool, len(dirs))
	slugCounts := map[string]int{}
	for _, d := range dirs {
		slug, ok := hydroSlug(d)
		slugs[d] = slug
		validSlug[d] = ok
		if ok {
			slugCounts[slug]++
		}
	}

	var actual int64
	for _, d := range dirs {
		slug := slugs[d]
		folder := path.Base(d)
		switch {
		case !validSlug[d]:
			result.Failed = append(result.Failed, HydroImportFailure{Folder: folder, Error: "题目标识无效"})
			continue
		case slugCounts[slug] > 1:
			result.Failed = append(result.Failed, HydroImportFailure{Folder: folder, Error: "题目标识已存在：" + slug})
			continue
		}
		imp, fail, err := s.importHydroProblem(ctx, createdBy, d, byDir[d], byDir[d+"/testdata"], slug, &actual)
		if err != nil {
			return nil, err
		}
		if fail != nil {
			result.Failed = append(result.Failed, *fail)
			continue
		}
		result.Imported = append(result.Imported, imp)
	}
	return result, nil
}

func validateHydroEntry(f *zip.File) error {
	if f.Flags&0x1 != 0 {
		return httpx.ErrValidation("题包包含加密条目")
	}
	if unsafeZipPath(f.Name) {
		return httpx.ErrValidation("题包包含非法路径")
	}
	mode := f.Mode()
	if mode&fs.ModeSymlink != 0 {
		return httpx.ErrValidation("题包包含符号链接")
	}
	if !mode.IsDir() && !mode.IsRegular() && mode.Type() != 0 {
		return httpx.ErrValidation("题包包含非常规文件")
	}
	return nil
}

// unsafeZipPath reports whether a zip entry name could escape a directory or is
// otherwise untrustworthy (absolute, backslash, "." or ".." segments, or
// interior empty segments). A single trailing slash marking a directory is
// allowed.
func unsafeZipPath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return true
	}
	parts := strings.Split(name, "/")
	for i, p := range parts {
		if p == "." || p == ".." {
			return true
		}
		if p == "" && i != len(parts)-1 {
			return true
		}
	}
	return false
}

// skipHydroEntry ignores macOS metadata and hidden files/directories.
func skipHydroEntry(name string) bool {
	for _, seg := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if seg == "__MACOSX" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// hydroSlug derives a slug from a problem folder's last path component. It is
// invalid when the sanitized folder name is empty or the result no longer fits
// the slug pattern.
func hydroSlug(folderDir string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToLower(path.Base(folderDir)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	sanitized := strings.Trim(b.String(), "-")
	if sanitized == "" {
		return hydroSlugPrefix, false
	}
	slug := hydroSlugPrefix + sanitized
	if !slugPattern.MatchString(slug) {
		return slug, false
	}
	return slug, true
}

// importHydroProblem imports one problem folder, returning the created problem,
// a per-problem failure, or a whole-package error (budget exceeded).
func (s *Service) importHydroProblem(
	ctx context.Context,
	createdBy int64,
	dir string,
	files, testFiles []hydroEntry,
	slug string,
	actual *int64,
) (HydroImportedProblem, *HydroImportFailure, error) {
	folder := path.Base(dir)
	fail := func(title, msg string) (HydroImportedProblem, *HydroImportFailure, error) {
		return HydroImportedProblem{}, &HydroImportFailure{Folder: folder, Title: title, Error: msg}, nil
	}

	var yamlFile *zip.File
	byBase := map[string]*zip.File{}
	for _, e := range files {
		byBase[e.base] = e.f
		if e.base == "problem.yaml" {
			yamlFile = e.f
		}
	}
	if yamlFile == nil {
		return fail("", "缺少 problem.yaml")
	}

	title := ""
	yamlData, err := readHydroEntry(yamlFile, maxHydroYAMLBytes, actual)
	if err != nil {
		if errors.Is(err, errHydroTooLarge) {
			return fail("", "problem.yaml 过大")
		}
		return fail("", "problem.yaml 读取失败")
	}
	if *actual > maxHydroUncompressed {
		return HydroImportedProblem{}, nil, httpx.ErrValidation("题包解压后大小超过限制")
	}
	var meta hydroYAML
	if err := yaml.Unmarshal(yamlData, &meta); err != nil {
		return fail("", "problem.yaml 解析失败")
	}
	title = strings.TrimSpace(meta.Title)

	statement, msg := readHydroStatement(byBase, actual)
	if msg != "" {
		return fail(title, msg)
	}

	pairs, msg := collectHydroTestdata(testFiles)
	if msg != "" {
		return fail(title, msg)
	}

	in := Input{
		Title:         title,
		Slug:          slug,
		StatementMD:   statement,
		Type:          TypeTraditional,
		Difficulty:    1,
		TimeLimitMS:   1000,
		MemoryLimitMB: 256,
		Source:        hydroSource,
		Tags:          meta.Tags,
	}
	p, err := s.Create(ctx, createdBy, in)
	if err != nil {
		return fail(title, hydroCreateError(err, slug))
	}

	for _, pair := range pairs {
		inRC, err := pair.in.Open()
		if err != nil {
			s.cleanupHydroProblem(ctx, p.ID)
			return fail(title, "测试数据读取失败")
		}
		outRC, err := pair.out.Open()
		if err != nil {
			_ = inRC.Close()
			s.cleanupHydroProblem(ctx, p.ID)
			return fail(title, "测试数据读取失败")
		}
		_, err = s.AddTestcase(ctx, p.ID, TestcaseUpload{
			Input:  &countingReader{r: inRC, n: actual},
			Output: &countingReader{r: outRC, n: actual},
			Score:  hydroTestcaseScore,
		})
		_ = inRC.Close()
		_ = outRC.Close()
		if err != nil {
			s.cleanupHydroProblem(ctx, p.ID)
			return fail(title, hydroTestcaseError(err))
		}
		if *actual > maxHydroUncompressed {
			s.cleanupHydroProblem(ctx, p.ID)
			return HydroImportedProblem{}, nil, httpx.ErrValidation("题包解压后大小超过限制")
		}
	}

	return HydroImportedProblem{
		ID:            p.ID,
		Title:         p.Title,
		Slug:          p.Slug,
		TestcaseCount: len(pairs),
	}, nil, nil
}

func (s *Service) cleanupHydroProblem(ctx context.Context, id int64) {
	if _, err := s.Delete(ctx, id); err != nil {
		slog.Warn("hydro import cleanup failed", "problemID", id, "err", err)
	}
}

func hydroCreateError(err error, slug string) string {
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		if appErr.Code == httpx.ErrSlugTaken.Code {
			return "题目标识已存在：" + slug
		}
		return appErr.Message
	}
	return err.Error()
}

func hydroTestcaseError(err error) string {
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return "测试数据写入失败"
}

// readHydroEntry reads an entry with an actual-read cap of max bytes, tracking
// the shared decompressed byte count.
func readHydroEntry(f *zip.File, max int64, actual *int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(&countingReader{r: rc, n: actual}, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errHydroTooLarge
	}
	return data, nil
}

func readHydroStatement(byBase map[string]*zip.File, actual *int64) (string, string) {
	read := func(name string) (string, error) {
		f := byBase[name]
		if f == nil {
			return "", nil
		}
		data, err := readHydroEntry(f, maxHydroStatementBytes, actual)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	statementErr := func(err error) string {
		if errors.Is(err, errHydroTooLarge) {
			return "题面不能超过 100000 个字符"
		}
		return "题面读取失败"
	}

	stmt, err := read("problem_zh.md")
	if err != nil {
		return "", statementErr(err)
	}
	if strings.TrimSpace(stmt) == "" {
		stmt, err = read("problem.md")
		if err != nil {
			return "", statementErr(err)
		}
	}
	if strings.TrimSpace(stmt) == "" {
		return "", "缺少题面文件（problem_zh.md 或 problem.md）"
	}
	if runes(stmt) > maxHydroStatementRunes {
		return "", "题面不能超过 100000 个字符"
	}
	return stmt, ""
}

func collectHydroTestdata(files []hydroEntry) ([]hydroPair, string) {
	type entry struct{ in, out *zip.File }
	m := map[string]*entry{}
	for _, e := range files {
		switch {
		case strings.HasSuffix(e.base, ".in"):
			name := strings.TrimSuffix(e.base, ".in")
			if name == "" {
				continue
			}
			if m[name] == nil {
				m[name] = &entry{}
			}
			m[name].in = e.f
		case strings.HasSuffix(e.base, ".out"):
			name := strings.TrimSuffix(e.base, ".out")
			if name == "" {
				continue
			}
			if m[name] == nil {
				m[name] = &entry{}
			}
			m[name].out = e.f
		}
	}

	names := make([]string, 0, len(m))
	for name, e := range m {
		if e.in == nil || e.out == nil {
			return nil, "测试数据 .in/.out 不配对"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, "缺少测试数据"
	}
	if len(names) > maxHydroTestcases {
		return nil, fmt.Sprintf("测试点数量不能超过 %d", maxHydroTestcases)
	}
	sort.Slice(names, func(i, j int) bool { return lessHydroTestbase(names[i], names[j]) })

	pairs := make([]hydroPair, 0, len(names))
	var total int64
	for _, name := range names {
		e := m[name]
		for _, f := range []*zip.File{e.in, e.out} {
			sz := int64(f.UncompressedSize64)
			if sz > maxHydroTestcaseBytes {
				return nil, "单个测试文件不能超过 16 MiB"
			}
			total += sz
		}
		if total > maxHydroProblemTestdataBytes {
			return nil, fmt.Sprintf("单题测试数据不能超过 %d MiB", maxHydroProblemTestdataBytes>>20)
		}
		pairs = append(pairs, hydroPair{in: e.in, out: e.out})
	}
	return pairs, ""
}

// lessHydroTestbase orders test data basenames numerically first, then
// lexicographically.
func lessHydroTestbase(a, b string) bool {
	na, aerr := strconv.ParseInt(a, 10, 64)
	nb, berr := strconv.ParseInt(b, 10, 64)
	aok, bok := aerr == nil, berr == nil
	switch {
	case aok && bok:
		if na != nb {
			return na < nb
		}
		return a < b
	case aok != bok:
		return aok
	default:
		return a < b
	}
}
