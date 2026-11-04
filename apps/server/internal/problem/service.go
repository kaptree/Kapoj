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
// user; CanManage is true for problem admins and above.
type Viewer struct{ CanManage bool }

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

// Service holds problem business rules. Authorization (who may call write
// methods) is enforced by the HTTP layer; the service only decides what a
// Viewer may see.
type Service struct {
	repo  Repository
	store storage.Storage
}

func NewService(repo Repository, store storage.Storage) *Service {
	return &Service{repo: repo, store: store}
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
	p.HintMD, p.SolutionMD, p.SolutionCode = in.HintMD, in.SolutionMD, in.SolutionCode
	p.Type, p.Difficulty = in.Type, in.Difficulty
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
	apply(p, in)
	if p.IsPublic {
		if err := s.checkPublishable(ctx, p); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, mapRepoErr(err)
	}
	return p, nil
}

func (s *Service) checkPublishable(ctx context.Context, p *Problem) error {
	n, err := s.repo.CountTestcases(ctx, p.ID)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrValidation("公开题目至少需要 1 个测试点")
	}
	if p.Type == TypeSpecial && p.CheckerPath == "" {
		return httpx.ErrValidation("特判题发布前需要先上传 checker")
	}
	return nil
}

// Get returns a problem the viewer may see. A hidden problem looks exactly
// like a missing one to everyone who cannot manage problems.
func (s *Service) Get(ctx context.Context, v Viewer, id int64) (*Problem, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if !p.IsPublic && !v.CanManage {
		return nil, httpx.ErrNotFound
	}
	return p, nil
}

func (s *Service) List(ctx context.Context, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error) {
	page = min(max(page, 1), maxListPage)
	items, total, err := s.repo.List(ctx, Filter{
		Keyword: q.Keyword, Tag: q.Tag, Difficulty: q.Difficulty,
		IncludeHidden: q.IncludeHidden && v.CanManage,
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
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return mapRepoErr(err)
	}
	if _, err := s.repo.FindTestcase(ctx, id, tid); err != nil {
		return mapRepoErr(err)
	}
	if p.IsPublic {
		n, err := s.repo.CountTestcases(ctx, id)
		if err != nil {
			return err
		}
		if n <= 1 {
			return httpx.ErrValidation("公开题目至少保留 1 个测试点，请先下线题目")
		}
	}
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
	key := fmt.Sprintf("problems/%d/checker.cpp", id)
	if _, err := s.put(ctx, key, br, maxCheckerBytes, "checker 文件"); err != nil {
		return err
	}
	return mapRepoErr(s.repo.SetCheckerPath(ctx, id, key))
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
	}
	return err
}
