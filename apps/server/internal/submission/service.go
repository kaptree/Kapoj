package submission

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// Viewer describes who is submitting; managers may submit hidden problems.
type Viewer struct{ CanManage bool }

type ProblemLookup interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
	CountTestcases(ctx context.Context, problemID int64) (int, error)
}

type Enqueuer interface {
	Enqueue(ctx context.Context, submissionID int64) error
}

type Limiter interface {
	Allow(ctx context.Context, userID int64) (bool, error)
}

type Service struct {
	repo     Repository
	problems ProblemLookup
	queue    Enqueuer
	limiter  Limiter
}

func NewService(repo Repository, problems ProblemLookup, queue Enqueuer, limiter Limiter) *Service {
	return &Service{repo: repo, problems: problems, queue: queue, limiter: limiter}
}

func (s *Service) Create(ctx context.Context, v Viewer, userID int64, in Input) (*Submission, error) {
	in, err := in.Normalize()
	if err != nil {
		return nil, err
	}
	p, err := s.problems.FindByID(ctx, in.ProblemID)
	if err != nil {
		if errors.Is(err, problem.ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if !p.IsPublic && !v.CanManage {
		return nil, httpx.ErrNotFound
	}
	if p.Type == problem.TypeInteractive {
		return nil, httpx.ErrValidation("交互题暂不支持评测")
	}
	if p.Type == problem.TypeSpecial && p.CheckerPath == "" {
		return nil, httpx.ErrValidation("题目缺少 checker，暂不能提交")
	}
	n, err := s.problems.CountTestcases(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, httpx.ErrValidation("题目还没有测试点")
	}
	if s.limiter != nil {
		allowed, err := s.limiter.Allow(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, httpx.ErrTooManySubmissions
		}
	}

	sub := &Submission{
		UserID: userID, ProblemID: p.ID, Language: in.Language, Code: in.Code,
		Status: StatusPending, JudgeResult: dbtypes.List[JudgeCaseResult]{},
	}
	if err := s.repo.Create(ctx, sub); err != nil {
		if errors.Is(err, problem.ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, sub.ID); err != nil {
		if markErr := s.repo.MarkSystemError(ctx, sub.ID, "任务入队失败"); markErr != nil {
			slog.Error("mark submission system error failed", "submission_id", sub.ID, "err", markErr)
		}
		return nil, err
	}
	return sub, nil
}
