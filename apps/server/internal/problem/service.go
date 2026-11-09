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
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

const (
	maxTestcaseBytes = 16 << 20
	maxCheckerBytes  = 256 << 10
	maxScore         = 1000
	maxListPage      = 1_000_000
)

// Viewer describes who is looking. The zero value is a guest or ordinary
// user. CanViewHidden grants read access to unpublished problems;
// CanViewLocked grants read access to problems locked by an unfinished
// contest; CanViewSolution grants read access to the solution fields
// (solutionMd, solutionCode, checkerUploaded, createdBy).
type Viewer struct {
	CanViewHidden   bool
	CanViewLocked   bool
	CanViewSolution bool
}

type ListQuery struct {
	Keyword       string
	Tag           string
	Difficulty    int
	IncludeHidden bool
}

type TestcaseUpload struct {
	Input    io.Reader
	Output   io.Reader
	Score    int
	IsSample bool
}

type Stats struct {
	SubmitCount int     `json:"submitCount"`
	AcceptCount int     `json:"acceptCount"`
	AcceptRate  float64 `json:"acceptRate"`
}

// LearningGuard decides whether a problem is referenced by an active learning
// path and therefore must not be unpublished or deleted. The zero value (nil)
// disables the guard.
type LearningGuard interface {
	IsProblemInActivePath(ctx context.Context, problemID int64) (bool, error)
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
	// references; publishing or keeping it public is never blocked.
	if wasPublic && !p.IsPublic && s.learningGuard != nil {
		referenced, err := s.learningGuard.IsProblemInActivePath(ctx, id)
		if err != nil {
			return nil, err
		}
		if referenced {
			return nil, httpx.ErrValidation("题目被学习路线引用，不能下线/删除")
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
	if !p.IsPublic && !v.CanViewHidden {
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
	items, total, err := s.repo.List(ctx, Filter{
		Keyword: q.Keyword, Tag: q.Tag, Difficulty: q.Difficulty,
		IncludeHidden: q.IncludeHidden && v.CanViewHidden,
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
	st := Stats{SubmitCount: p.SubmitCount, AcceptCount: p.AcceptCount}
	if p.SubmitCount > 0 {
		st.AcceptRate = float64(p.AcceptCount) / float64(p.SubmitCount)
	}
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
