package testcasegen

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const maxSourceBytes = 64 * 1024 // 64 KiB

// validLanguages is the set of standard-code languages the judge can compile.
var validLanguages = map[string]bool{
	"c": true, "cpp": true, "python3": true, "java": true, "go": true,
}

// ProblemReader supplies the problem a generation is created for.
type ProblemReader interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
}

// ScriptGenerator asks the AI for a Python generator script.
type ScriptGenerator interface {
	GenerateTestcaseScript(ctx context.Context, userID int64, in ai.TestcaseScriptInput) (string, error)
}

// Enqueuer hands a generation to the judge queue.
type Enqueuer interface {
	EnqueueTestcaseGeneration(ctx context.Context, id int64) error
}

// CreateInput is the request body for creating a generation.
type CreateInput struct {
	Language      string `json:"language"`
	SourceCode    string `json:"sourceCode"`
	TestcaseCount int    `json:"testcaseCount"`
}

// CaseView is the per-case metadata exposed to administrators.
type CaseView struct {
	ID         int64 `json:"id"`
	SortOrder  int   `json:"sortOrder"`
	InputSize  int64 `json:"inputSize"`
	OutputSize int64 `json:"outputSize"`
}

// View is the generation detail returned by create/get.
type View struct {
	ID            int64      `json:"id"`
	Status        Status     `json:"status"`
	Language      string     `json:"language"`
	TestcaseCount int        `json:"testcaseCount"`
	GeneratorCode string     `json:"generatorCode"`
	ErrorMessage  string     `json:"errorMessage"`
	CreatedAt     time.Time  `json:"createdAt"`
	Cases         []CaseView `json:"cases"`
}

type Service struct {
	repo     Repository
	problems ProblemReader
	store    storage.Storage // Task 3: preview/confirm/discard read generated files.
	scripts  ScriptGenerator
	queue    Enqueuer
}

func NewService(repo Repository, problems ProblemReader, store storage.Storage,
	scripts ScriptGenerator, queue Enqueuer) *Service {
	return &Service{repo: repo, problems: problems, store: store, scripts: scripts, queue: queue}
}

// Create validates input, confirms the problem is a traditional one, asks the
// AI for a script, persists a pending generation and enqueues it for the judge.
func (s *Service) Create(ctx context.Context, problemID, userID int64, in CreateInput) (*View, error) {
	lang := strings.TrimSpace(in.Language)
	src := strings.TrimSpace(in.SourceCode)
	if err := validateCreateInput(lang, src, in.TestcaseCount); err != nil {
		return nil, err
	}

	p, err := s.problems.FindByID(ctx, problemID)
	if err != nil {
		if errors.Is(err, problem.ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if p.Type != problem.TypeTraditional {
		return nil, httpx.ErrValidation("only traditional problems support AI testcase generation")
	}

	samples := make([]ai.TestcaseScriptSample, 0, len(p.Samples))
	for _, sp := range p.Samples {
		samples = append(samples, ai.TestcaseScriptSample{Input: sp.Input, Output: sp.Output})
	}
	script, err := s.scripts.GenerateTestcaseScript(ctx, userID, ai.TestcaseScriptInput{
		Title:          p.Title,
		StatementMD:    p.StatementMD,
		InputFormatMD:  p.InputFormatMD,
		OutputFormatMD: p.OutputFormatMD,
		Samples:        samples,
		StandardCode:   src,
		Language:       lang,
		TestcaseCount:  in.TestcaseCount,
	})
	if err != nil {
		return nil, err
	}

	g := &Generation{
		ProblemID:     problemID,
		UserID:        userID,
		Language:      lang,
		StandardCode:  src,
		GeneratorCode: script,
		TestcaseCount: in.TestcaseCount,
		TimeLimitMS:   p.TimeLimitMS,
		MemoryLimitMB: p.MemoryLimitMB,
	}
	if err := s.repo.Create(ctx, g); err != nil {
		return nil, err
	}
	if err := s.queue.EnqueueTestcaseGeneration(ctx, g.ID); err != nil {
		_ = s.repo.MarkFailed(ctx, g.ID, "任务投递失败")
		return nil, httpx.ErrUnavailable
	}
	return s.toView(g, nil), nil
}

// Get returns the generation view, including any cases already produced.
func (s *Service) Get(ctx context.Context, problemID, id int64) (*View, error) {
	g, err := s.repo.Find(ctx, problemID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cases, err := s.repo.ListCases(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	return s.toView(g, cases), nil
}

func validateCreateInput(lang, src string, count int) error {
	if !validLanguages[lang] {
		return httpx.ErrValidation("unsupported language")
	}
	if count < 1 || count > 10 {
		return httpx.ErrValidation("testcase count must be between 1 and 10")
	}
	if src == "" {
		return httpx.ErrValidation("source code is required")
	}
	if len(src) > maxSourceBytes {
		return httpx.ErrValidation("source code is too large")
	}
	return nil
}

func (s *Service) toView(g *Generation, cases []GenerationCase) *View {
	cv := make([]CaseView, 0, len(cases))
	for _, c := range cases {
		cv = append(cv, CaseView{ID: c.ID, SortOrder: c.SortOrder, InputSize: c.InputSize, OutputSize: c.OutputSize})
	}
	return &View{
		ID:            g.ID,
		Status:        g.Status,
		Language:      g.Language,
		TestcaseCount: g.TestcaseCount,
		GeneratorCode: g.GeneratorCode,
		ErrorMessage:  g.ErrorMessage,
		CreatedAt:     g.CreatedAt,
		Cases:         cv,
	}
}
