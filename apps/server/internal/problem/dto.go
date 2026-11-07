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
// problem managers and are omitted from the JSON otherwise.
type DetailDTO struct {
	ListItem
	StatementMD    string   `json:"statementMd"`
	InputFormatMD  string   `json:"inputFormatMd"`
	OutputFormatMD string   `json:"outputFormatMd"`
	Samples        []Sample `json:"samples"`
	HintMD         string   `json:"hintMd"`

	SolutionPriceCoins int64   `json:"solutionPriceCoins"`
	SolutionMD         *string `json:"solutionMd,omitempty"`
	SolutionCode       *string `json:"solutionCode,omitempty"`
	CheckerUploaded    *bool   `json:"checkerUploaded,omitempty"`
	CreatedBy          *int64  `json:"createdBy,omitempty"`
}

type TestcaseDTO struct {
	ID         int64 `json:"id"`
	SortOrder  int   `json:"sortOrder"`
	Score      int   `json:"score"`
	IsSample   bool  `json:"isSample"`
	InputSize  int64 `json:"inputSize"`
	OutputSize int64 `json:"outputSize"`
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

func ToDetail(p *Problem, includeSolution bool) DetailDTO {
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
		SolutionPriceCoins: p.SolutionPriceCoins,
	}
	if includeSolution {
		uploaded := p.CheckerPath != ""
		d.SolutionMD, d.SolutionCode = &p.SolutionMD, &p.SolutionCode
		d.CheckerUploaded, d.CreatedBy = &uploaded, &p.CreatedBy
	}
	return d
}

func ToTestcaseDTO(tc *Testcase) TestcaseDTO {
	return TestcaseDTO{
		ID: tc.ID, SortOrder: tc.SortOrder, Score: tc.Score, IsSample: tc.IsSample,
		InputSize: tc.InputSize, OutputSize: tc.OutputSize,
	}
}
