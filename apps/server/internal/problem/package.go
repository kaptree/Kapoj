package problem

import "time"

// kapOJ 题包 v1。单题包以 manifest.json 为入口，所有文件均为包内相对路径。
const (
	PackageFormatProblem  = "kapoj-problem"
	PackageFormatProblems = "kapoj-problems"
	PackageFormatVersion  = 1
	// MaxPackageProblems caps one batch import or export.
	MaxPackageProblems = 100
	// MaxPackageExportBytes caps the uncompressed content of one export.
	MaxPackageExportBytes = 2 << 30

	packageImageScheme = "kapoj-package://"
)

// PackageManifest is the root manifest of one problem package.
type PackageManifest struct {
	Format        string              `json:"format"`
	FormatVersion int                 `json:"formatVersion"`
	ExportedAt    time.Time           `json:"exportedAt"`
	Problem       PackageProblem      `json:"problem"`
	Files         PackageFileManifest `json:"files"`
}

// PackageProblem carries portable problem metadata. Runtime fields such as
// numeric ids, submission counters and the creator are intentionally absent.
type PackageProblem struct {
	Slug               string   `json:"slug"`
	Title              string   `json:"title"`
	Type               Type     `json:"type"`
	Difficulty         int      `json:"difficulty"`
	TimeLimitMS        int      `json:"timeLimitMs"`
	MemoryLimitMB      int      `json:"memoryLimitMb"`
	Source             string   `json:"source"`
	Tags               []string `json:"tags"`
	IsPublic           bool     `json:"isPublic"`
	SolutionPriceCoins int64    `json:"solutionPriceCoins"`
}

// PackageAttachment maps one stored attachment to its package-relative path.
type PackageAttachment struct {
	File        string `json:"file"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
}

// PackageFileManifest lists every optional or required file in the package.
// Empty fields mean the package does not contain that file.
type PackageFileManifest struct {
	Statement    string              `json:"statement"`
	InputFormat  string              `json:"inputFormat"`
	OutputFormat string              `json:"outputFormat"`
	Hint         string              `json:"hint"`
	Samples      string              `json:"samples"`
	Answers      string              `json:"answers,omitempty"`
	Testdata     string              `json:"testdata"`
	Checker      string              `json:"checker,omitempty"`
	Attachments  []PackageAttachment `json:"attachments,omitempty"`
	Images       []PackageAttachment `json:"images,omitempty"`
	Solution     string              `json:"solution,omitempty"`
	SolutionCode string              `json:"solutionCode,omitempty"`
}

// TestcaseManifest describes one testdata pair inside testdata/meta.json.
type TestcaseManifest struct {
	SortOrder int    `json:"sortOrder"`
	Score     int    `json:"score"`
	IsSample  bool   `json:"isSample"`
	Input     string `json:"input"`
	Output    string `json:"output"`
}

// TestdataManifest is the testdata/meta.json document.
type TestdataManifest struct {
	Testcases []TestcaseManifest `json:"testcases"`
}

// PackageInfo is the lightweight metadata shown before downloading a package.
type PackageInfo struct {
	Format        string `json:"format"`
	FormatVersion int    `json:"formatVersion"`
	FileCount     int    `json:"fileCount"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// ProblemsManifest is the root manifest of a batch problem package.
type ProblemsManifest struct {
	Format        string                  `json:"format"`
	FormatVersion int                     `json:"formatVersion"`
	ExportedAt    time.Time               `json:"exportedAt"`
	Problems      []ProblemsManifestEntry `json:"problems"`
}

// ProblemsManifestEntry locates one single-problem package inside the batch.
type ProblemsManifestEntry struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Directory string `json:"directory"`
}

func packageImagePlaceholder(file string) string {
	return packageImageScheme + file
}
