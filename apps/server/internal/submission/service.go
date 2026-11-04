package submission

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// Viewer describes who is looking. The zero UserID is a guest; managers may
// submit hidden problems and see protected contest code.
type Viewer struct {
	UserID int64
	// CanManage covers problem/contest/super admins for contest code access.
	CanManage bool
	// CanManageProblems is limited to problem/super admins for hidden problems.
	CanManageProblems bool
}

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

type Publisher interface {
	Publish(ctx context.Context, submissionID int64)
}

type Service struct {
	repo      Repository
	problems  ProblemLookup
	queue     Enqueuer
	limiter   Limiter
	publisher Publisher
}

func NewService(repo Repository, problems ProblemLookup, queue Enqueuer, limiter Limiter, publisher Publisher) *Service {
	return &Service{repo: repo, problems: problems, queue: queue, limiter: limiter, publisher: publisher}
}

func (s *Service) publish(ctx context.Context, submissionID int64) {
	if s.publisher != nil {
		s.publisher.Publish(ctx, submissionID)
	}
}

const (
	maxListPage = 1_000_000
	maxPageSize = 100
)

func (s *Service) List(ctx context.Context, v Viewer, q ListQuery, page, size int) (httpx.Page[ListItem], error) {
	q, err := q.Normalize()
	if err != nil {
		return httpx.Page[ListItem]{}, err
	}
	page = min(max(page, 1), maxListPage)
	size = min(max(size, 1), maxPageSize)
	rows, total, err := s.repo.List(ctx, q.toFilter(v), (page-1)*size, size)
	if err != nil {
		return httpx.Page[ListItem]{}, err
	}
	items := make([]ListItem, len(rows))
	for i := range rows {
		items[i] = toListItem(rows[i])
	}
	return httpx.Page[ListItem]{Total: total, Items: items}, nil
}

func (s *Service) Detail(ctx context.Context, v Viewer, id int64) (*Detail, error) {
	row, err := s.repo.FindDetail(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if !row.ProblemIsPublic && !v.CanManageProblems && v.UserID != row.UserID {
		return nil, httpx.ErrNotFound
	}
	detail := &Detail{
		ListItem: toListItem(*row), Code: row.Code, ErrorMessage: row.ErrorMessage,
		JudgeResult: append([]JudgeCaseResult{}, row.JudgeResult...),
	}
	if row.ContestID != nil && !v.CanManage && v.UserID != row.UserID {
		detail.Code = ""
		detail.CodeHidden = true
		detail.ErrorMessage = ""
		detail.JudgeResult = []JudgeCaseResult{}
	}
	return detail, nil
}

func (s *Service) HasAccepted(ctx context.Context, userID, problemID int64) (bool, error) {
	return s.repo.HasAccepted(ctx, userID, problemID)
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
	s.publish(ctx, sub.ID)
	if err := s.queue.Enqueue(ctx, sub.ID); err != nil {
		if markErr := s.repo.MarkSystemError(ctx, sub.ID, "任务入队失败"); markErr != nil {
			slog.Error("mark submission system error failed", "submission_id", sub.ID, "err", markErr)
		}
		s.publish(ctx, sub.ID)
		return nil, err
	}
	return sub, nil
}
