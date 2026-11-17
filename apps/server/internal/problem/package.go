package problem

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

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
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Type  Type   `json:"type"`
	// Difficulty is the legacy 1–5 band kept so pre-1-100 applications can
	// still import new packages. New exports derive it from DifficultyScore.
	Difficulty int `json:"difficulty"`
	// DifficultyScore is the exact 1–100 score. Old applications ignore it.
	DifficultyScore *int `json:"difficultyScore,omitempty"`
	// DifficultyScale describes DifficultyScore's scale ("1-100" in new
	// exports). It is informational and only used to reject unknown formats.
	DifficultyScale    string   `json:"difficultyScale,omitempty"`
	TimeLimitMS        int      `json:"timeLimitMs"`
	MemoryLimitMB      int      `json:"memoryLimitMb"`
	Source             string   `json:"source"`
	Tags               []string `json:"tags"`
	IsPublic           bool     `json:"isPublic"`
	SolutionPriceCoins int64    `json:"solutionPriceCoins"`
}

const (
	// packageDifficultyScale marks exact 1–100 scores in package manifests.
	packageDifficultyScale = "1-100"
	// legacyPackageDifficultyScale is accepted for packages that declare the
	// old five-band scale explicitly.
	legacyPackageDifficultyScale = "1-5"
)

// packageDifficultyBand maps an exact 1–100 score to the legacy five-band
// scale used by old applications: 1–20→1, 21–40→2, 41–60→3, 61–80→4, 81–100→5.
func packageDifficultyBand(score int) int {
	switch {
	case score <= 20:
		return 1
	case score <= 40:
		return 2
	case score <= 60:
		return 3
	case score <= 80:
		return 4
	default:
		return 5
	}
}

// resolvePackageDifficulty chooses the exact score for an imported problem.
// A difficultyScore wins; otherwise the legacy 1–5 field is mapped to
// 10/30/50/70/90. Unknown scales and out-of-range values are validation errors.
func resolvePackageDifficulty(manifest PackageProblem) (int, error) {
	switch manifest.DifficultyScale {
	case "", packageDifficultyScale, legacyPackageDifficultyScale:
	default:
		return 0, httpx.ErrValidation("题包 difficultyScale 必须是 1-100 或 1-5")
	}
	if manifest.DifficultyScore != nil {
		score := *manifest.DifficultyScore
		if score < 1 || score > 100 {
			return 0, httpx.ErrValidation("题包 difficultyScore 需为 1–100")
		}
		return score, nil
	}
	if manifest.Difficulty < 1 || manifest.Difficulty > 5 {
		return 0, httpx.ErrValidation("题包 difficulty 需为 1–5")
	}
	return [...]int{10, 30, 50, 70, 90}[manifest.Difficulty-1], nil
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
