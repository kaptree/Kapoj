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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxTestcaseBytes       = 16 << 20
	maxInlineTestcaseBytes = 2 << 20
	maxCheckerBytes        = 256 << 10
	maxScore               = 1000
	maxListPage            = 1_000_000
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

// WithLearningGuard installs a guard that blocks unpublishing or deleting a
// problem referenced by an active learning path.
func WithLearningGuard(g LearningGuard) Option {
	return func(s *Service) { s.learningGuard = g }
}

// Service holds problem business rules. Authorization (who may call write
// methods) is enforced by the HTTP layer; the service only decides what a
// Viewer may see.
type Service struct {
	repo          Repository
	store         storage.Storage
	learningGuard LearningGuard

	hydroUncompressedLimit int64
}

func NewService(repo Repository, store storage.Storage, opts ...Option) *Service {
	s := &Service{repo: repo, store: store}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Create(ctx context.Context, createdBy int64, in Input) (*Problem, error) {
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p := &Problem{CreatedBy: createdBy}
	apply(p, in)
	p.IsPublic = false // new problems are published through Update
	if err := s.repo.Create(ctx, p); err != nil {
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
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	wasPublic := p.IsPublic
	apply(p, in)
	// Taking a public problem offline is guarded against active learning path
	// references.
	if wasPublic && !p.IsPublic && s.learningGuard != nil {
		referenced, err := s.learningGuard.IsProblemInActivePath(ctx, id)
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
		paid, err := s.learningGuard.IsProblemInPaidPath(ctx, id)
		if err != nil {
			return nil, err
		}
		if paid {
			return nil, httpx.ErrValidation("题目被付费学习路线引用，不能公开")
		}
	}
	// Publishability is checked by the repository inside the same transaction
	// as the update, together with the row lock.
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapRepoErr(err)
	}
	return p, nil
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
	if up.Score < 0 || up.Score > maxScore {
		return nil, httpx.ErrValidation(fmt.Sprintf("分数需为 0–%d", maxScore))
	}
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return nil, mapRepoErr(err)
	}

	base := fmt.Sprintf("problems/%d/testcases/%s", id, randomHex())
	tc := &Testcase{
		ProblemID: id, InputFile: base + ".in", OutputFile: base + ".out",
		Score: up.Score, IsSample: up.IsSample,
	}
	var err error
	if tc.InputSize, err = s.put(ctx, tc.InputFile, up.Input, maxTestcaseBytes, "输入文件"); err != nil {
		return nil, err
	}
	if tc.OutputSize, err = s.put(ctx, tc.OutputFile, up.Output, maxTestcaseBytes, "输出文件"); err != nil {
		s.deleteFiles(ctx, []string{tc.InputFile})
		return nil, err
	}
	if err := s.repo.AddTestcase(ctx, tc); err != nil {
		s.deleteFiles(ctx, []string{tc.InputFile, tc.OutputFile})
		return nil, mapRepoErr(err)
	}
	return tc, nil
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
		return 0, httpx.ErrValidation(fmt.Sprintf("%s不能超过 %d KiB", what, max>>10))
	}
	return n, err
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
	br := bufio.NewReader(r)
	if _, err := br.Peek(1); errors.Is(err, io.EOF) {
		return httpx.ErrValidation("checker 文件不能为空")
	}
	key := fmt.Sprintf("problems/%d/checkers/%s.cpp", id, randomHex())
	if _, err := s.put(ctx, key, br, maxCheckerBytes, "checker 文件"); err != nil {
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
