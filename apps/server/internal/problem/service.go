package problem

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxTestcaseBytes             = 16 << 20
	maxInlineTestcaseBytes       = 2 << 20
	maxCheckerBytes              = 256 << 10
	maxScore                     = 1000
	maxListPage                  = 1_000_000
	maxDifficultyHistoryPageSize = 100
)

// Viewer describes who is looking. The zero value is a guest or ordinary
// user. CanViewHidden grants read access to all unpublished problems;
// CanViewLocked grants read access to problems locked by an unfinished
// contest; CanViewSolution grants read access to the solution fields
// (solutionMd, solutionCode, checkerUploaded, createdBy). CanManageAll grants
// access to every problem; CanManageOwn allows the creator to see their own
// hidden problems.
type Viewer struct {
	UserID          int64
	CanViewHidden   bool
	CanViewLocked   bool
	CanViewSolution bool
	CanManageAll    bool
	CanManageOwn    bool
	// CanViewLiveContest includes others' running-contest submissions in the
	// statistics endpoints. It mirrors submission.view_live_contest.
	CanViewLiveContest bool
}

type ListQuery struct {
	Keyword       string
	Tag           string
	Difficulty    int
	DifficultyMin int
	DifficultyMax int
	IncludeHidden bool
	// OwnerID limits the result to problems created by this user. It is set
	// by the mine scope; zero means no owner filter.
	OwnerID int64
}

type TestcaseUpload struct {
	Input    io.Reader
	Output   io.Reader
	Score    int
	IsSample bool
}

type Stats struct {
	SubmitCount  int           `json:"submitCount"`
	AcceptCount  int           `json:"acceptCount"`
	AcceptRate   float64       `json:"acceptRate"`
	StatusCounts []StatusCount `json:"statusCounts"`
}

// LearningGuard decides whether a problem is referenced by an active learning
// path and therefore must not be unpublished or deleted. The zero value (nil)
// disables the guard.
type LearningGuard interface {
	IsProblemInActivePath(ctx context.Context, problemID int64) (bool, error)
	// IsProblemInPaidPath reports whether an active paid learning path
	// references the problem; such a problem must stay private.
	IsProblemInPaidPath(ctx context.Context, problemID int64) (bool, error)
	// CanAccessPaidProblem reports whether the user bought or authored a paid
	// path that references the private problem.
	CanAccessPaidProblem(ctx context.Context, userID, problemID int64) (bool, error)
}

// Option customizes a problem Service.
type Option func(*Service)

// TestcaseLimits are the upload limits applied to testcase files.
type TestcaseLimits struct {
	MaxInputBytes        int64
	MaxOutputBytes       int64
	MaxBatchTotalBytes   int64
	MaxProblemTotalBytes int64
}

// TestcaseLimitsProvider supplies the current upload limits. It is read once
// at the start of each operation so one request sees a consistent snapshot.
type TestcaseLimitsProvider interface {
	TestcaseLimits(ctx context.Context) (TestcaseLimits, error)
}

// TestcaseLimitsProviderFunc adapts a function to TestcaseLimitsProvider.
type TestcaseLimitsProviderFunc func(ctx context.Context) (TestcaseLimits, error)

func (f TestcaseLimitsProviderFunc) TestcaseLimits(ctx context.Context) (TestcaseLimits, error) {
	return f(ctx)
}

// DefaultTestcaseLimits matches the historical fixed limits.
func DefaultTestcaseLimits() TestcaseLimits {
	return TestcaseLimits{
		MaxInputBytes:        maxTestcaseBytes,
		MaxOutputBytes:       maxTestcaseBytes,
		MaxBatchTotalBytes:   maxBatchTotalBytes,
		MaxProblemTotalBytes: maxBatchTotalBytes,
	}
}

// WithLearningGuard installs a guard that blocks unpublishing or deleting a
// problem referenced by an active learning path.
func WithLearningGuard(g LearningGuard) Option {
	return func(s *Service) { s.learningGuard = g }
}

// WithTestcaseLimitsProvider installs the dynamic testcase upload limits.
func WithTestcaseLimitsProvider(p TestcaseLimitsProvider) Option {
	return func(s *Service) { s.testcaseLimits = p }
}

// Service holds problem business rules. Authorization (who may call write
// methods) is enforced by the HTTP layer; the service only decides what a
// Viewer may see.
type Service struct {
	repo           Repository
	store          storage.Storage
	learningGuard  LearningGuard
	testcaseLimits TestcaseLimitsProvider

	hydroUncompressedLimit  int64
	packageExportMaxBytes   int64
	packageExportMaxEntries int
}

func NewService(repo Repository, store storage.Storage, opts ...Option) *Service {
	s := &Service{
		repo: repo, store: store,
		packageExportMaxBytes:   MaxPackageExportBytes,
		packageExportMaxEntries: maxHydroEntries,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// limits returns the upload limits for one operation. A missing or failing
// provider keeps uploads working with the legacy defaults.
func (s *Service) limits(ctx context.Context) TestcaseLimits {
	if s.testcaseLimits == nil {
		return DefaultTestcaseLimits()
	}
	limits, err := s.testcaseLimits.TestcaseLimits(ctx)
	if err != nil {
		slog.Warn("load testcase limits failed; using defaults", "err", err)
		return DefaultTestcaseLimits()
	}
	return limits
}

func (s *Service) Create(ctx context.Context, createdBy int64, in Input) (*Problem, error) {
	// Legacy callers keep a manual initial score; the AI-aware API layer calls
	// CreateWithDifficulty with the assessment it computed.
	return s.CreateWithDifficulty(ctx, createdBy, in, DifficultyDecision{
		Score: in.Difficulty, Prior: in.Difficulty,
		PriorSource: DifficultyPriorSourceManual,
		Source:      DifficultySourceManual,
		Auto:        true, ActorID: createdBy,
	})
}

// CreateWithDifficulty creates a problem and its initial history row in one
// transaction. d is authoritative for the stored score and metadata.
func (s *Service) CreateWithDifficulty(ctx context.Context, createdBy int64, in Input, d DifficultyDecision) (*Problem, error) {
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	if err := validateDifficultyScore(d.Score); err != nil {
		return nil, err
	}
	p := &Problem{CreatedBy: createdBy}
	apply(p, in)
	p.IsPublic = false // new problems are published through Update
	applyDifficultyDecision(p, d)
	history := newDifficultyHistory(0, d)
	// problems.difficulty_source records "ai" for both AI paths, while the
	// history distinguishes the initial assessment from a reassessment.
	if history.Source == DifficultySourceAI {
		history.Source = DifficultySourceAIInitial
	}
	if err := s.repo.CreateWithHistory(ctx, p, history); err != nil {
		return nil, mapRepoErr(err)
	}
	return p, nil
}

func apply(p *Problem, in Input) {
	p.Title, p.Slug = in.Title, in.Slug
	p.StatementMD, p.InputFormatMD, p.OutputFormatMD = in.StatementMD, in.InputFormatMD, in.OutputFormatMD
	p.Samples, p.Tags = in.Samples, in.Tags
	p.HintMD = in.HintMD
	p.Type, p.Difficulty = in.Type, in.Difficulty
	p.Answers = in.Answers
	p.TimeLimitMS, p.MemoryLimitMB = in.TimeLimitMS, in.MemoryLimitMB
	p.IsPublic, p.Source = in.IsPublic, in.Source
}

func (s *Service) Update(ctx context.Context, id int64, in Input) (*Problem, error) {
	return s.UpdateAs(ctx, id, in, 0)
}

// UpdateAs computes the same decision as Update but records actorID as the
// history author. The HTTP layer uses it so manual difficulty changes are
// attributable to the caller.
func (s *Service) UpdateAs(ctx context.Context, id int64, in Input, actorID int64) (*Problem, error) {
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	d := updateDifficultyDecision(p, in)
	d.ActorID = actorID
	return s.update(ctx, p, in, d)
}

// updateDifficultyDecision applies the PUT /api/problems/:id difficulty rules:
// an explicit difficultyAuto=false locks the score but keeps the existing
// prior/source/reason unless the score actually changes; an explicit true keeps
// auto (anchoring a changed score); omitting the field locks a changed score
// and preserves all metadata when the score is unchanged.
func updateDifficultyDecision(p *Problem, in Input) DifficultyDecision {
	manual := func(auto bool) DifficultyDecision {
		return DifficultyDecision{
			Score: in.Difficulty, Prior: in.Difficulty,
			PriorSource: DifficultyPriorSourceManual,
			Source:      DifficultySourceManual,
			Auto:        auto,
		}
	}
	keep := DifficultyDecision{
		Score: in.Difficulty, Prior: p.DifficultyPrior,
		PriorSource: p.DifficultyPriorSource, Source: p.DifficultySource,
		Auto: p.DifficultyAuto, Reason: p.DifficultyReason,
	}
	if in.DifficultyAuto == nil {
		if in.Difficulty == p.Difficulty {
			return keep
		}
		return manual(false)
	}
	if !*in.DifficultyAuto {
		if in.Difficulty == p.Difficulty {
			// Saving an already-locked problem (or locking it at its current
			// score) must not rewrite provenance it did not change.
			keep.Auto = false
			return keep
		}
		return manual(false)
	}
	if in.Difficulty == p.Difficulty {
		keep.Auto = true
		return keep
	}
	return manual(true)
}

// UpdateWithDifficulty updates a problem with the caller's difficulty decision.
// History is written when the score changes, or whenever the decision forces it
// (imports log even an unchanged score).
func (s *Service) UpdateWithDifficulty(ctx context.Context, id int64, in Input, d DifficultyDecision) (*Problem, error) {
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return s.update(ctx, p, in, d)
}

func (s *Service) update(ctx context.Context, p *Problem, in Input, d DifficultyDecision) (*Problem, error) {
	if err := validateDifficultyScore(d.Score); err != nil {
		return nil, err
	}
	oldScore := p.Difficulty
	wasPublic := p.IsPublic
	apply(p, in)
	applyDifficultyDecision(p, d)
	// Taking a public problem offline is guarded against active learning path
	// references.
	if wasPublic && !p.IsPublic && s.learningGuard != nil {
		referenced, err := s.learningGuard.IsProblemInActivePath(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if referenced {
			return nil, httpx.ErrValidation("题目被学习路线引用，不能下线/删除")
		}
	}
	// A problem referenced by an active paid path must stay private so users
	// cannot bypass the path purchase.
	if p.IsPublic && s.learningGuard != nil {
		paid, err := s.learningGuard.IsProblemInPaidPath(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if paid {
			return nil, httpx.ErrValidation("题目被付费学习路线引用，不能公开")
		}
	}
	var history *DifficultyHistory
	// Regular edits only log actual score changes; forced (import) decisions
	// also log an unchanged score.
	if oldScore != p.Difficulty || d.forceHistory {
		history = newDifficultyHistory(oldScore, d)
	}
	// Publishability is checked by the repository inside the same transaction
	// as the update, together with the row lock.
	if err := s.repo.UpdateWithHistory(ctx, p, history); err != nil {
		return nil, mapRepoErr(err)
	}
	return p, nil
}

// UpdateDifficultyMeta updates only the difficulty metadata, used by
// "re-assess with AI". Every call is audited, including same-score
// reassessments, because the prior/source/reason can still change.
func (s *Service) UpdateDifficultyMeta(ctx context.Context, id int64, d DifficultyDecision) (*Problem, error) {
	if err := validateDifficultyScore(d.Score); err != nil {
		return nil, err
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	oldScore := p.Difficulty
	applyDifficultyDecision(p, d)
	history := newDifficultyHistory(oldScore, d)
	// A successful AI reassessment keeps problems.difficulty_source="ai" but
	// its history row records the cause explicitly.
	if history.Source == DifficultySourceAI {
		history.Source = DifficultySourceAIReassess
	}
	if err := s.repo.UpdateDifficultyWithHistory(ctx, p, history); err != nil {
		return nil, mapRepoErr(err)
	}
	return p, nil
}

// DifficultyHistory returns one page of a problem's difficulty history,
// newest first.
func (s *Service) DifficultyHistory(ctx context.Context, problemID int64, page, pageSize int) ([]DifficultyHistory, int64, error) {
	page = min(max(page, 1), maxListPage)
	pageSize = min(max(pageSize, 1), maxDifficultyHistoryPageSize)
	items, total, err := s.repo.ListDifficultyHistory(ctx, problemID, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, mapRepoErr(err)
	}
	if items == nil {
		items = []DifficultyHistory{}
	}
	return items, total, nil
}

// Get returns a problem the viewer may see. A hidden problem looks exactly
// like a missing one to everyone who cannot view hidden problems.
func (s *Service) Get(ctx context.Context, v Viewer, id int64) (*Problem, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if !p.IsPublic && !v.CanViewHidden && !v.CanManageAll &&
		!(v.CanManageOwn && p.CreatedBy == v.UserID) {
		if s.learningGuard != nil && v.UserID > 0 {
			allowed, err := s.learningGuard.CanAccessPaidProblem(ctx, v.UserID, id)
			if err != nil {
				return nil, err
			}
			if allowed {
				return p, nil
			}
		}
		return nil, httpx.ErrNotFound
	}
	return p, nil
}

// Author returns the public summary of a problem creator. A non-positive id or
// a missing user yields nil; database errors are returned unchanged.
func (s *Service) Author(ctx context.Context, userID int64) (*AuthorSummary, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.repo.FindAuthor(ctx, userID)
}

func (s *Service) List(ctx context.Context, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error) {
	page = min(max(page, 1), maxListPage)

	ownerID := q.OwnerID
	includeHidden := q.IncludeHidden && (v.CanViewHidden || v.CanManageAll)
	if ownerID > 0 {
		// The mine scope is explicitly scoped by the handler; ownership is
		// checked there. Hidden own problems are included when requested.
		includeHidden = q.IncludeHidden
	} else if q.IncludeHidden && !v.CanViewHidden && !v.CanManageAll && v.CanManageOwn {
		// Legacy includeHidden=true for an author means "my hidden problems",
		// not every hidden problem in the bank.
		ownerID = v.UserID
		includeHidden = true
	}

	items, total, err := s.repo.List(ctx, Filter{
		Keyword: q.Keyword, Tag: q.Tag, Difficulty: q.Difficulty,
		DifficultyMin: q.DifficultyMin, DifficultyMax: q.DifficultyMax,
		IncludeHidden: includeHidden, OwnerID: ownerID,
		IncludeLocked: v.CanViewLocked, Now: time.Now(),
	}, (page-1)*size, size)
	if err != nil {
		return httpx.Page[ListItem]{}, err
	}
	out := make([]ListItem, len(items))
	for i := range items {
		out[i] = ToListItem(&items[i])
	}
	return httpx.Page[ListItem]{Total: total, Items: out}, nil
}

func (s *Service) Delete(ctx context.Context, id int64) (bool, error) {
	if s.learningGuard != nil {
		referenced, err := s.learningGuard.IsProblemInActivePath(ctx, id)
		if err != nil {
			return false, err
		}
		if referenced {
			return false, httpx.ErrValidation("题目被学习路线引用，不能下线/删除")
		}
	}
	res, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, mapRepoErr(err)
	}
	s.deleteFiles(ctx, res.FileKeys)
	return res.Offlined, nil
}

func (s *Service) Statistics(ctx context.Context, v Viewer, id int64) (Stats, error) {
	p, err := s.Get(ctx, v, id)
	if err != nil {
		return Stats{}, err
	}
	st := Stats{
		SubmitCount:  p.SubmitCount,
		AcceptCount:  p.AcceptCount,
		StatusCounts: []StatusCount{},
	}
	if p.SubmitCount > 0 {
		st.AcceptRate = float64(p.AcceptCount) / float64(p.SubmitCount)
	}
	counts, err := s.repo.StatusCounts(ctx, id, SubmissionVisibility{
		ViewerID: v.UserID, CanViewLiveContest: v.CanViewLiveContest, Now: time.Now(),
	})
	if err != nil {
		return Stats{}, err
	}
	var visibleTotal int64
	for _, row := range counts {
		visibleTotal += row.Count
	}
	for i := range counts {
		if visibleTotal > 0 {
			counts[i].Percentage = float64(counts[i].Count) / float64(visibleTotal)
		}
	}
	st.StatusCounts = counts
	return st, nil
}

func (s *Service) AddTestcase(ctx context.Context, id int64, up TestcaseUpload) (*Testcase, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, mapRepoErr(err)
	}
	tc, keys, err := s.stageTestcase(ctx, id, up)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddTestcase(ctx, tc); err != nil {
		s.deleteFiles(ctx, keys)
		return nil, mapRepoErr(err)
	}
	return tc, nil
}

// stageTestcase writes testcase files without touching the database. The
// caller owns keys and must delete them if the surrounding import fails.
func (s *Service) stageTestcase(ctx context.Context, problemID int64, up TestcaseUpload) (*Testcase, []string, error) {
	if up.Score < 0 || up.Score > maxScore {
		return nil, nil, httpx.ErrValidation(fmt.Sprintf("分数需为 0–%d", maxScore))
	}
	limits := s.limits(ctx)
	base := fmt.Sprintf("problems/%d/testcases/%s", problemID, randomHex())
	tc := &Testcase{
		ProblemID: problemID, InputFile: base + ".in", OutputFile: base + ".out",
		Score: up.Score, IsSample: up.IsSample,
	}
	var err error
	if tc.InputSize, err = s.put(ctx, tc.InputFile, up.Input, limits.MaxInputBytes, "输入文件"); err != nil {
		return nil, nil, err
	}
	if tc.OutputSize, err = s.put(ctx, tc.OutputFile, up.Output, limits.MaxOutputBytes, "输出文件"); err != nil {
		s.deleteFiles(ctx, []string{tc.InputFile})
		return nil, nil, err
	}
	return tc, []string{tc.InputFile, tc.OutputFile}, nil
}

// GetTestcaseContent reads one testcase for the split online editor.
func (s *Service) GetTestcaseContent(ctx context.Context, problemID, testcaseID int64) (*TestcaseContent, error) {
	tc, err := s.repo.FindTestcase(ctx, problemID, testcaseID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	input, inputSize, err := s.readInlineTestcase(ctx, tc.InputFile)
	if err != nil {
		return nil, err
	}
	output, outputSize, err := s.readInlineTestcase(ctx, tc.OutputFile)
	if err != nil {
		return nil, err
	}
	return &TestcaseContent{
		Input: input, Output: output, InputSize: inputSize, OutputSize: outputSize,
		Score: tc.Score, IsSample: tc.IsSample,
	}, nil
}

func (s *Service) readInlineTestcase(ctx context.Context, key string) (string, int64, error) {
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return "", 0, httpx.ErrNotFound
	}
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxInlineTestcaseBytes+1))
	if err != nil {
		return "", 0, err
	}
	if len(data) > maxInlineTestcaseBytes {
		return "", 0, httpx.ErrValidation("测试数据超过 2 MiB，请下载或替换文件")
	}
	if !utf8.Valid(data) {
		return "", 0, httpx.ErrValidation("测试数据不是 UTF-8 文本，不能在线编辑")
	}
	return string(data), int64(len(data)), nil
}

// UpdateTestcaseContent rewrites both files, then swaps the database keys and
// deletes the old files only after the database update succeeds.
func (s *Service) UpdateTestcaseContent(ctx context.Context, problemID, testcaseID int64, input, output string) error {
	if err := validateInlineTestcase(input, "输入数据"); err != nil {
		return err
	}
	if err := validateInlineTestcase(output, "输出数据"); err != nil {
		return err
	}
	tc, err := s.repo.FindTestcase(ctx, problemID, testcaseID)
	if err != nil {
		return mapRepoErr(err)
	}
	base := fmt.Sprintf("problems/%d/testcases/%s", problemID, randomHex())
	inputKey, outputKey := base+".in", base+".out"
	inputSize, err := s.put(ctx, inputKey, strings.NewReader(input), maxInlineTestcaseBytes, "输入数据")
	if err != nil {
		return err
	}
	outputSize, err := s.put(ctx, outputKey, strings.NewReader(output), maxInlineTestcaseBytes, "输出数据")
	if err != nil {
		s.deleteFiles(ctx, []string{inputKey})
		return err
	}
	if err := s.repo.UpdateTestcaseFiles(ctx, problemID, testcaseID, inputKey, outputKey, inputSize, outputSize); err != nil {
		s.deleteFiles(ctx, []string{inputKey, outputKey})
		return mapRepoErr(err)
	}
	s.deleteFiles(ctx, []string{tc.InputFile, tc.OutputFile})
	return nil
}

// AddOnlineTestcase creates a testcase from textarea content.
func (s *Service) AddOnlineTestcase(ctx context.Context, problemID int64, input, output string, score int, isSample bool) (*Testcase, error) {
	if err := validateInlineTestcase(input, "输入数据"); err != nil {
		return nil, err
	}
	if err := validateInlineTestcase(output, "输出数据"); err != nil {
		return nil, err
	}
	return s.AddTestcase(ctx, problemID, TestcaseUpload{
		Input: strings.NewReader(input), Output: strings.NewReader(output),
		Score: score, IsSample: isSample,
	})
}

func validateInlineTestcase(value, what string) error {
	if len(value) > maxInlineTestcaseBytes {
		return httpx.ErrValidation(what + "不能超过 2 MiB")
	}
	if !utf8.ValidString(value) {
		return httpx.ErrValidation(what + "必须是 UTF-8 文本")
	}
	return nil
}

// put stores r under key and turns a size overflow into a validation error.
func (s *Service) put(ctx context.Context, key string, r io.Reader, max int64, what string) (int64, error) {
	n, err := s.store.Put(ctx, key, r, max)
	if errors.Is(err, storage.ErrTooLarge) {
		return 0, httpx.ErrValidation(fmt.Sprintf("%s不能超过 %s", what, formatLimitBytes(max)))
	}
	return n, err
}

// checkProblemTestcaseTotal enforces the per-problem testcase total against the
// bytes actually imported. The zip-declared size scan in the importers remains
// an early guard only: metadata is not authoritative for what gets stored.
func checkProblemTestcaseTotal(total, limit int64) error {
	if total > limit {
		return httpx.ErrValidation(fmt.Sprintf("单题测试数据不能超过 %s", formatLimitBytes(limit)))
	}
	return nil
}

// formatLimitBytes renders a byte limit in MiB or KiB without integer
// rounding, so configured values appear in messages instead of hardcoded ones.
func formatLimitBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/float64(1<<20), 'f', -1, 64) + " MiB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/float64(1<<10), 'f', -1, 64) + " KiB"
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func (s *Service) ListTestcases(ctx context.Context, id int64) ([]TestcaseDTO, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, mapRepoErr(err)
	}
	cases, err := s.repo.ListTestcases(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]TestcaseDTO, len(cases))
	for i := range cases {
		out[i] = ToTestcaseDTO(&cases[i])
	}
	return out, nil
}

func (s *Service) UpdateTestcase(ctx context.Context, id, tid int64, score *int, isSample *bool) error {
	if score != nil && (*score < 0 || *score > maxScore) {
		return httpx.ErrValidation(fmt.Sprintf("分数需为 0–%d", maxScore))
	}
	return mapRepoErr(s.repo.UpdateTestcase(ctx, id, tid, score, isSample))
}

func (s *Service) DeleteTestcase(ctx context.Context, id, tid int64) error {
	removed, err := s.repo.DeleteTestcase(ctx, id, tid)
	if err != nil {
		return mapRepoErr(err)
	}
	s.deleteFiles(ctx, []string{removed.InputFile, removed.OutputFile})
	return nil
}

func (s *Service) ReorderTestcases(ctx context.Context, id int64, ids []int64) error {
	return mapRepoErr(s.repo.ReorderTestcases(ctx, id, ids))
}

func (s *Service) UploadChecker(ctx context.Context, id int64, r io.Reader) error {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoErr(err)
	}
	if p.Type != TypeSpecial {
		return httpx.ErrValidation("只有特判题可以上传 checker")
	}
	key, err := s.stageChecker(ctx, id, r)
	if err != nil {
		return err
	}
	if err := s.repo.SetCheckerPath(ctx, id, key); err != nil {
		s.deleteFiles(ctx, []string{key})
		return mapRepoErr(err)
	}
	if p.CheckerPath != "" && p.CheckerPath != key {
		s.deleteFiles(ctx, []string{p.CheckerPath})
	}
	return nil
}

// stageChecker writes a checker file without touching the database.
func (s *Service) stageChecker(ctx context.Context, problemID int64, r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	if _, err := br.Peek(1); errors.Is(err, io.EOF) {
		return "", httpx.ErrValidation("checker 文件不能为空")
	}
	key := fmt.Sprintf("problems/%d/checkers/%s.cpp", problemID, randomHex())
	if _, err := s.put(ctx, key, br, maxCheckerBytes, "checker 文件"); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Service) deleteFiles(ctx context.Context, keys []string) {
	for _, k := range keys {
		if err := s.store.Delete(ctx, k); err != nil {
			slog.Warn("delete stored file failed", "key", k, "err", err)
		}
	}
}

func randomHex() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTestcaseNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, ErrSlugTaken):
		return httpx.ErrSlugTaken
	case errors.Is(err, ErrBadOrder):
		return httpx.ErrValidation("排序列表必须恰好包含该题全部测试点，且不能重复")
	case errors.Is(err, ErrPublishNeedsTestcase):
		return httpx.ErrValidation("公开题目至少需要 1 个测试点")
	case errors.Is(err, ErrPublishNeedsChecker):
		return httpx.ErrValidation("特判题发布前需要先上传 checker")
	case errors.Is(err, ErrLastTestcase):
		return httpx.ErrValidation("公开题目至少保留 1 个测试点，请先下线题目")
	case errors.Is(err, ErrInUse):
		return httpx.ErrValidation("题目已被比赛使用，无法删除，请先下线或从比赛中移除")
	}
	return err
}
