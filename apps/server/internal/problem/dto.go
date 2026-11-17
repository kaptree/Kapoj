package problem

import "time"

type ListItem struct {
	ID            int64     `json:"id"`
	Title         string    `json:"title"`
	Slug          string    `json:"slug"`
	Type          Type      `json:"type"`
	Tags          []string  `json:"tags"`
	Difficulty    int       `json:"difficulty"`
	TimeLimitMS   int       `json:"timeLimitMs"`
	MemoryLimitMB int       `json:"memoryLimitMb"`
	SubmitCount   int       `json:"submitCount"`
	AcceptCount   int       `json:"acceptCount"`
	IsPublic      bool      `json:"isPublic"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"createdAt"`
}

// DetailDTO is the full problem view. The pointer fields are only set for
// problem managers and are omitted from the JSON otherwise, except Author,
// which is public for everyone who can read the problem.
type DetailDTO struct {
	ListItem
	StatementMD    string   `json:"statementMd"`
	InputFormatMD  string   `json:"inputFormatMd"`
	OutputFormatMD string   `json:"outputFormatMd"`
	Samples        []Sample `json:"samples"`
	HintMD         string   `json:"hintMd"`

	// Author is the public creator summary. It is nil when the problem has no
	// creator (created_by <= 0) or the user no longer exists.
	Author *AuthorSummary `json:"author,omitempty"`

	SolutionPriceCoins int64     `json:"solutionPriceCoins"`
	SolutionMD         *string   `json:"solutionMd,omitempty"`
	SolutionCode       *string   `json:"solutionCode,omitempty"`
	CheckerUploaded    *bool     `json:"checkerUploaded,omitempty"`
	CreatedBy          *int64    `json:"createdBy,omitempty"`
	Answers            *[]string `json:"answers,omitempty"`
	CanEdit            bool      `json:"canEdit"`

	// Difficulty metadata is manager-only. Accepted solvers can read the
	// solution but must never see the prior/source internals, so these fields
	// are gated by their own includeDifficultyMeta switch.
	DifficultyAuto        *bool      `json:"difficultyAuto,omitempty"`
	DifficultySource      *string    `json:"difficultySource,omitempty"`
	DifficultyPrior       *int       `json:"difficultyPrior,omitempty"`
	DifficultyPriorSource *string    `json:"difficultyPriorSource,omitempty"`
	DifficultyReason      *string    `json:"difficultyReason,omitempty"`
	DifficultyUpdatedAt   *time.Time `json:"difficultyUpdatedAt,omitempty"`
}

type TestcaseDTO struct {
	ID         int64 `json:"id"`
	SortOrder  int   `json:"sortOrder"`
	Score      int   `json:"score"`
	IsSample   bool  `json:"isSample"`
	InputSize  int64 `json:"inputSize"`
	OutputSize int64 `json:"outputSize"`
}

// TestcaseContent is the inline-editor view of one testcase.
type TestcaseContent struct {
	Input      string `json:"input"`
	Output     string `json:"output"`
	InputSize  int64  `json:"inputSize"`
	OutputSize int64  `json:"outputSize"`
	Score      int    `json:"score"`
	IsSample   bool   `json:"isSample"`
}

func ToListItem(p *Problem) ListItem {
	tags := []string(p.Tags)
	if tags == nil {
		tags = []string{}
	}
	return ListItem{
		ID: p.ID, Title: p.Title, Slug: p.Slug, Type: p.Type, Tags: tags,
		Difficulty: p.Difficulty, TimeLimitMS: p.TimeLimitMS, MemoryLimitMB: p.MemoryLimitMB,
		SubmitCount: p.SubmitCount, AcceptCount: p.AcceptCount, IsPublic: p.IsPublic,
		Source: p.Source, CreatedAt: p.CreatedAt.UTC(),
	}
}

func ToDetail(p *Problem, includeSolution, includeAnswer, includeDifficultyMeta bool, author *AuthorSummary) DetailDTO {
	samples := []Sample(p.Samples)
	if samples == nil {
		samples = []Sample{}
	}
	d := DetailDTO{
		ListItem:           ToListItem(p),
		StatementMD:        p.StatementMD,
		InputFormatMD:      p.InputFormatMD,
		OutputFormatMD:     p.OutputFormatMD,
		Samples:            samples,
		HintMD:             p.HintMD,
		Author:             author,
		SolutionPriceCoins: p.SolutionPriceCoins,
	}
	if includeSolution {
		uploaded := p.CheckerPath != ""
		d.SolutionMD, d.SolutionCode = &p.SolutionMD, &p.SolutionCode
		d.CheckerUploaded, d.CreatedBy = &uploaded, &p.CreatedBy
	}
	if includeAnswer {
		answers := []string(p.Answers)
		if answers == nil {
			answers = []string{}
		}
		d.Answers = &answers
	}
	if includeDifficultyMeta {
		auto := p.DifficultyAuto
		source := p.DifficultySource
		prior := p.DifficultyPrior
		priorSource := p.DifficultyPriorSource
		reason := p.DifficultyReason
		updatedAt := p.DifficultyUpdatedAt.UTC()
		d.DifficultyAuto = &auto
		d.DifficultySource = &source
		d.DifficultyPrior = &prior
		d.DifficultyPriorSource = &priorSource
		d.DifficultyReason = &reason
		d.DifficultyUpdatedAt = &updatedAt
	}
	return d
}

func ToTestcaseDTO(tc *Testcase) TestcaseDTO {
	return TestcaseDTO{
		ID: tc.ID, SortOrder: tc.SortOrder, Score: tc.Score, IsSample: tc.IsSample,
		InputSize: tc.InputSize, OutputSize: tc.OutputSize,
	}
}
