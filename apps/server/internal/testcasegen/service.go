package testcasegen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxSourceBytes  = 64 * 1024 // 64 KiB
	maxPreviewBytes = 64 * 1024 // 64 KiB preview ceiling
)

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

// Preview is a truncated, sanitized view of one generated input/output file.
type Preview struct {
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

// ConfirmResult reports how many testcases were written into the problem.
type ConfirmResult struct {
	Count int `json:"count"`
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
		return nil, httpx.ErrValidation("仅传统题支持 AI 造数据")
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

// Preview reads one generated file (input or output) up to 64 KiB, replacing
// invalid UTF-8 with U+FFFD and reporting whether it was truncated.
func (s *Service) Preview(ctx context.Context, problemID, id, caseID int64, kind string) (*Preview, error) {
	if kind != "input" && kind != "output" {
		return nil, httpx.ErrValidation("kind 必须是 input 或 output")
	}
	g, err := s.repo.Find(ctx, problemID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c, err := s.repo.FindCase(ctx, g.ID, caseID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	key := c.InputFile
	if kind == "output" {
		key = c.OutputFile
	}
	rc, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	raw, err := io.ReadAll(io.LimitReader(rc, maxPreviewBytes+1))
	if err != nil {
		return nil, err
	}
	truncated := len(raw) > maxPreviewBytes
	content := strings.ToValidUTF8(string(raw), "\uFFFD")
	// Sanitizing invalid bytes can expand a single byte to a 3-byte U+FFFD, so
	// enforce the 64 KiB ceiling on the final (valid) text rather than the raw
	// read, trimming on a rune boundary.
	if len(content) > maxPreviewBytes {
		truncated = true
		content = truncateUTF8(content, maxPreviewBytes)
	}
	return &Preview{
		Kind:      kind,
		Content:   content,
		Truncated: truncated,
	}, nil
}

// truncateUTF8 shortens s to at most max bytes without splitting a rune.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// Confirm writes a ready generation's cases into problem_testcases and marks it
// confirmed. A repeat confirm is idempotent and returns the retained count.
func (s *Service) Confirm(ctx context.Context, problemID, id int64) (*ConfirmResult, error) {
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
	if g.Status == StatusConfirmed {
		return &ConfirmResult{Count: len(cases)}, nil
	}
	count, err := s.repo.Confirm(ctx, problemID, id, cases)
	if errors.Is(err, ErrNotReady) {
		return nil, httpx.ErrValidation("生成结果尚未就绪，不能确认入库")
	}
	if err != nil {
		return nil, err
	}
	return &ConfirmResult{Count: count}, nil
}

// Discard deletes a ready or failed generation's cases and generated files,
// marking it discarded. Confirmed generations are immutable and pending/running
// generations cannot be discarded yet.
func (s *Service) Discard(ctx context.Context, problemID, id int64) error {
	g, err := s.repo.Find(ctx, problemID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	switch g.Status {
	case StatusConfirmed:
		return httpx.ErrValidation("已确认入库的生成结果不能丢弃")
	case StatusPending, StatusRunning:
		return httpx.ErrValidation("生成尚未完成，不能丢弃")
	}
	keys, err := s.repo.Discard(ctx, problemID, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrConfirmed):
			return httpx.ErrValidation("已确认入库的生成结果不能丢弃")
		case errors.Is(err, ErrNotFinished):
			return httpx.ErrValidation("生成尚未完成，不能丢弃")
		}
		return err
	}
	// The judge writes deterministic n.in/n.out keys even when a run failed
	// partway through, so clean those up as well.
	for n := 1; n <= g.TestcaseCount; n++ {
		keys = append(keys, generatedFileKey(problemID, id, n, "in"), generatedFileKey(problemID, id, n, "out"))
	}
	for _, key := range keys {
		if err := s.store.Delete(ctx, key); err != nil {
			slog.Warn("testcasegen: delete generated file failed", "key", key, "err", err)
		}
	}
	return nil
}

// generatedFileKey is the deterministic storage key the judge writes each
// generated input/output pair under.
func generatedFileKey(problemID, generationID int64, n int, ext string) string {
	return fmt.Sprintf("problems/%d/generated/%d/%d.%s", problemID, generationID, n, ext)
}

func validateCreateInput(lang, src string, count int) error {
	if !validLanguages[lang] {
		return httpx.ErrValidation("不支持的语言")
	}
	if count < 1 || count > 10 {
		return httpx.ErrValidation("生成组数必须在 1 到 10 之间")
	}
	if src == "" {
		return httpx.ErrValidation("标程代码不能为空")
	}
	if len(src) > maxSourceBytes {
		return httpx.ErrValidation("标程代码过大")
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
