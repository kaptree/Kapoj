package submission

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// Viewer describes who is looking. The zero UserID is a guest; an ordinary
// user gets no extra capability. Each boolean maps to an RBAC permission set by
// the API layer and is independent of the others: CanViewHiddenProblems
// (problem.view_hidden), CanViewLiveContest (submission.view_live_contest) and
// CanManageContest (contest.update).
type Viewer struct {
	UserID int64
	// CanViewHiddenProblems grants access to hidden-problem submissions and to
	// submitting hidden problems outside a contest.
	CanViewHiddenProblems bool
	// CanViewLiveContest bypasses contest protection to view others' code,
	// score and testcases while a contest is running.
	CanViewLiveContest bool
	// CanManageContest bypasses the registration requirement for contest test
	// submissions. It does not grant hidden-problem access.
	CanManageContest bool
}

type ProblemLookup interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
	CountTestcases(ctx context.Context, problemID int64) (int, error)
}

type ContestLookup interface {
	FindByID(ctx context.Context, id int64) (*contest.Contest, error)
	IsParticipant(ctx context.Context, contestID, userID int64) (bool, error)
	FindProblem(ctx context.Context, contestID, problemID int64) (*contest.ContestProblem, error)
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
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
	contests  ContestLookup
	queue     Enqueuer
	limiter   Limiter
	publisher Publisher
}

func NewService(repo Repository, problems ProblemLookup, contests ContestLookup, queue Enqueuer, limiter Limiter, publisher Publisher) *Service {
	return &Service{
		repo: repo, problems: problems, contests: contests,
		queue: queue, limiter: limiter, publisher: publisher,
	}
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
	now := time.Now()
	for i := range rows {
		items[i] = toListItem(rows[i])
		if contestProtected(rows[i], v, now) {
			items[i].TimeUsedMS = 0
			items[i].MemoryUsedKB = 0
			items[i].Score = 0
		}
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
	if !row.ProblemIsPublic && !v.CanViewHiddenProblems && v.UserID != row.UserID {
		return nil, httpx.ErrNotFound
	}
	detail := &Detail{
		ListItem: toListItem(*row), Code: row.Code, ErrorMessage: row.ErrorMessage,
		JudgeResult: append([]JudgeCaseResult{}, row.JudgeResult...),
	}
	if contestProtected(*row, v, time.Now()) {
		detail.Code = ""
		detail.CodeHidden = true
		detail.ErrorMessage = ""
		detail.JudgeResult = []JudgeCaseResult{}
		detail.TimeUsedMS = 0
		detail.MemoryUsedKB = 0
		detail.Score = 0
	}
	return detail, nil
}

func contestProtected(row ListRow, v Viewer, now time.Time) bool {
	if row.ContestID == nil || v.CanViewLiveContest || v.UserID == row.UserID {
		return false
	}
	if row.ContestStartTime == nil || row.ContestEndTime == nil {
		return false
	}
	return !now.Before(*row.ContestStartTime) && now.Before(*row.ContestEndTime)
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
	var contestID *int64
	if in.ContestID > 0 {
		c, err := s.contests.FindByID(ctx, in.ContestID)
		if err != nil {
			if errors.Is(err, contest.ErrNotFound) {
				return nil, httpx.ErrNotFound
			}
			return nil, err
		}
		now := time.Now()
		if now.Before(c.StartTime) {
			return nil, httpx.ErrValidation("比赛尚未开始")
		}
		if !now.Before(c.EndTime) {
			return nil, httpx.ErrValidation("比赛已结束")
		}
		if !v.CanManageContest {
			registered, err := s.contests.IsParticipant(ctx, c.ID, userID)
			if err != nil {
				return nil, err
			}
			if !registered {
				return nil, httpx.ErrValidation("请先报名比赛")
			}
		}
		if _, err := s.contests.FindProblem(ctx, c.ID, p.ID); err != nil {
			if errors.Is(err, contest.ErrProblemNotInContest) {
				return nil, httpx.ErrValidation("题目不属于该比赛")
			}
			return nil, err
		}
		id := c.ID
		contestID = &id
	}
	if contestID == nil && !v.CanViewHiddenProblems && !v.CanManageContest {
		locked, err := s.contests.IsProblemLocked(ctx, p.ID, time.Now())
		if err != nil {
			return nil, err
		}
		if locked {
			return nil, httpx.ErrNotFound
		}
	}
	if !p.IsPublic && !v.CanViewHiddenProblems && contestID == nil {
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
		UserID: userID, ProblemID: p.ID, ContestID: contestID,
		Language: in.Language, Code: in.Code,
		Status: StatusPending, JudgeResult: dbtypes.List[JudgeCaseResult]{},
	}
	if err := s.repo.Create(ctx, sub); err != nil {
		if errors.Is(err, problem.ErrNotFound) || errors.Is(err, contest.ErrNotFound) {
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
