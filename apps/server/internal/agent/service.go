package agent

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const (
	// MaxAttempts is the number of submissions allowed per problem attempt.
	MaxAttempts = 20
	// ClaimTimeout is the default time an Agent may spend on one problem.
	ClaimTimeout = 30 * time.Minute

	initialRating      = 1500
	lastUsedThrottle   = time.Minute
	agentInputMaxName  = 100
	agentInputMaxIntro = 2000

	defaultPageSize = 20
	maxPageSize     = 100
)

// ProblemLookup is the problem dependency used by claim/submission logic. Task
// 3 only defines it; later tasks consume it.
type ProblemLookup interface {
	FindByID(ctx context.Context, id int64) (*problem.Problem, error)
	CountTestcases(ctx context.Context, problemID int64) (int, error)
}

// ContestLookup is the contest dependency used by claim logic.
type ContestLookup interface {
	IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
}

// Limiter throttles Agent actions.
type Limiter interface {
	Allow(ctx context.Context, agentID int64, action string) (bool, error)
}

// Option customizes a Service.
type Option func(*Service)

// WithClock replaces the time source; tests use it for deterministic
// timestamps and last_used_at throttling.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithClaimTimeout sets how long one problem claim stays active.
func WithClaimTimeout(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.claimTimeout = d
		}
	}
}

// AttemptView is the public projection of one Agent problem attempt.
type AttemptView struct {
	ID                int64         `json:"id"`
	ProblemID         int64         `json:"problemId"`
	ProblemTitle      string        `json:"problemTitle"`
	ProblemSlug       string        `json:"problemSlug"`
	Difficulty        int           `json:"difficulty"`
	Status            AttemptStatus `json:"status"`
	AttemptsUsed      int           `json:"attemptsUsed"`
	AttemptsRemaining int           `json:"attemptsRemaining"`
	ClaimedAt         time.Time     `json:"claimedAt"`
	DeadlineAt        time.Time     `json:"deadlineAt"`
	SolvedAt          *time.Time    `json:"solvedAt"`
	SolveTimeMS       int           `json:"solveTimeMs"`
}

// AgentInput is the user-editable Agent profile.
type AgentInput struct {
	Name       string `json:"name"`
	ModelName  string `json:"modelName"`
	ModelIntro string `json:"modelIntro"`
}

// AgentAuthor identifies the user who owns an Agent.
type AgentAuthor struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// AgentView is the public Agent profile and statistics.
type AgentView struct {
	ID             int64       `json:"id"`
	UserID         int64       `json:"userId"`
	Name           string      `json:"name"`
	ModelName      string      `json:"modelName"`
	ModelIntro     string      `json:"modelIntro"`
	Status         Status      `json:"status"`
	Rating         int         `json:"rating"`
	PassedCount    int         `json:"passedCount"`
	AttemptedCount int         `json:"attemptedCount"`
	AvgSolveTimeMS int         `json:"avgSolveTimeMs"`
	Author         AgentAuthor `json:"author"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
}

// AgentFilter filters and sorts the public Agent leaderboard.
type AgentFilter struct {
	Model string
	Sort  string // rating | passed | avg_time
}

// AgentDetailView is the public Agent detail payload. It intentionally exposes
// only the public profile and statistics, never tokens or the current attempt.
type AgentDetailView struct {
	Agent AgentView `json:"agent"`
}

// AgentRatingRow is one public rating history entry with its problem label.
// Reset entries have no problem: ProblemID is 0 and the title/slug are empty.
type AgentRatingRow struct {
	ID           int64      `json:"id"`
	ProblemID    int64      `json:"problemId"`
	ProblemTitle string     `json:"problemTitle"`
	ProblemSlug  string     `json:"problemSlug"`
	Kind         RatingKind `json:"kind"`
	Difficulty   int        `json:"difficulty"`
	RatingBefore int        `json:"ratingBefore"`
	RatingAfter  int        `json:"ratingAfter"`
	Delta        int        `json:"delta"`
	SolveTimeMS  int        `json:"solveTimeMs"`
	SubmissionID *int64     `json:"submissionId"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// AdminAgentInput is the admin status/reset payload. An empty status keeps the
// current one; ResetRating is only applied when explicitly true.
type AdminAgentInput struct {
	Status      Status `json:"status"`
	ResetRating bool   `json:"resetRating"`
}

// AdminAgentSubmissionView is the admin-only investigation projection of one
// Agent submission. It includes code and objective answers and must never be
// exposed on public endpoints.
type AdminAgentSubmissionView struct {
	ID           int64               `json:"id"`
	ProblemID    int64               `json:"problemId"`
	ProblemTitle string              `json:"problemTitle"`
	ProblemSlug  string              `json:"problemSlug"`
	Language     submission.Language `json:"language"`
	Status       submission.Status   `json:"status"`
	TimeUsedMS   int                 `json:"timeUsedMs"`
	MemoryUsedKB int                 `json:"memoryUsedKb"`
	Score        int                 `json:"score"`
	Code         string              `json:"code"`
	Answer       string              `json:"answer"`
	ErrorMessage string              `json:"errorMessage"`
	CreatedAt    time.Time           `json:"createdAt"`
	JudgedAt     *time.Time          `json:"judgedAt"`
	AgentSettled bool                `json:"agentSettled"`
}

// AgentTokenView is token metadata; the plaintext is never returned here.
type AgentTokenView struct {
	ID          int64      `json:"id"`
	TokenPrefix string     `json:"tokenPrefix"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
}

// TokenIssue is the one-time response that carries a new plaintext token.
type TokenIssue struct {
	Token string         `json:"token"`
	Meta  AgentTokenView `json:"meta"`
}

// MyAgentView is the current user's Agent page payload.
type MyAgentView struct {
	Agent          *AgentView      `json:"agent"`
	Token          *AgentTokenView `json:"token"`
	CurrentAttempt *AttemptView    `json:"currentAttempt"`
}

// AuthAgent is the result of authenticating an Agent token.
type AuthAgent struct {
	Agent   Agent
	TokenID int64
}

// Service implements Agent CRUD and token lifecycle rules.
type Service struct {
	db       *gorm.DB
	problems ProblemLookup
	contests ContestLookup
	queue    submission.Enqueuer
	limiter  Limiter

	now          func() time.Time
	claimTimeout time.Duration
}

// NewService constructs an Agent service. problems, contests, queue and
// limiter are used by later claim/submission tasks; nil is accepted here.
func NewService(
	db *gorm.DB,
	problems ProblemLookup,
	contests ContestLookup,
	queue submission.Enqueuer,
	limiter Limiter,
	opts ...Option,
) *Service {
	s := &Service{
		db:           db,
		problems:     problems,
		contests:     contests,
		queue:        queue,
		limiter:      limiter,
		now:          time.Now,
		claimTimeout: ClaimTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateAgent creates the single Agent owned by userID.
func (s *Service) CreateAgent(ctx context.Context, userID int64, in AgentInput) (*AgentView, error) {
	in, err := normalizeAgentInput(in)
	if err != nil {
		return nil, err
	}

	var existing Agent
	err = s.db.WithContext(ctx).Select("id").Where("user_id = ?", userID).First(&existing).Error
	if err == nil {
		return nil, httpx.ErrValidation("每个用户只能创建一个 Agent")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := s.now()
	a := Agent{
		UserID:     userID,
		Name:       in.Name,
		ModelName:  in.ModelName,
		ModelIntro: in.ModelIntro,
		Status:     StatusActive,
		Rating:     initialRating,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.db.WithContext(ctx).Create(&a).Error; err != nil {
		return nil, mapAgentWriteError(err)
	}
	return s.agentView(ctx, a)
}

// MyAgent returns the current user's Agent page. A user without an Agent gets
// a payload with three null fields rather than an error.
func (s *Service) MyAgent(ctx context.Context, userID int64) (*MyAgentView, error) {
	var a Agent
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &MyAgentView{}, nil
	}
	if err != nil {
		return nil, err
	}

	view, err := s.agentView(ctx, a)
	if err != nil {
		return nil, err
	}
	out := &MyAgentView{Agent: view}

	var token AgentToken
	err = s.db.WithContext(ctx).
		Where("agent_id = ? AND revoked_at IS NULL", a.ID).
		Order("id DESC").First(&token).Error
	if err == nil {
		tv := toTokenView(token)
		out.Token = &tv
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	attempt, err := s.currentAttempt(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out.CurrentAttempt = attempt
	return out, nil
}

// UpdateAgent updates the Agent owned by userID.
func (s *Service) UpdateAgent(ctx context.Context, userID int64, in AgentInput) (*AgentView, error) {
	in, err := normalizeAgentInput(in)
	if err != nil {
		return nil, err
	}

	var a Agent
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).Model(&Agent{}).
		Where("user_id = ?", userID).
		Updates(map[string]any{
			"name":        in.Name,
			"model_name":  in.ModelName,
			"model_intro": in.ModelIntro,
			"updated_at":  s.now(),
		}).Error
	if err != nil {
		return nil, mapAgentWriteError(err)
	}

	if err := s.db.WithContext(ctx).First(&a, a.ID).Error; err != nil {
		return nil, err
	}
	return s.agentView(ctx, a)
}

// ListAgents returns one page of the public leaderboard. The default order is
// rating DESC, id ASC; sort=passed ranks by solved problems and sort=avg_time
// ranks the fastest average solve time among Agents that solved something.
func (s *Service) ListAgents(
	ctx context.Context, f AgentFilter, page, size int,
) (httpx.Page[AgentView], error) {
	return s.listAgentPage(ctx, f.Model, "", f.Sort, page, size)
}

// AgentDetail returns the public profile of one Agent.
func (s *Service) AgentDetail(ctx context.Context, id int64) (*AgentDetailView, error) {
	var a Agent
	err := s.db.WithContext(ctx).First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	view, err := s.agentView(ctx, a)
	if err != nil {
		return nil, err
	}
	return &AgentDetailView{Agent: *view}, nil
}

// RatingHistory returns one page of an Agent's rating history, newest first,
// with problem titles and slugs joined in. viewerID is the optional logged-in
// user; the Agent owner still receives submission ids for redacted problems.
func (s *Service) RatingHistory(
	ctx context.Context, agentID, viewerID int64, page, size int,
) (httpx.Page[AgentRatingRow], error) {
	return s.ratingHistoryPage(ctx, agentID, viewerID, page, size)
}

// RatingHistoryForUser resolves the current user's Agent and returns its
// history. A user without an Agent gets a 404.
func (s *Service) RatingHistoryForUser(
	ctx context.Context, userID int64, page, size int,
) (httpx.Page[AgentRatingRow], error) {
	var a Agent
	err := s.db.WithContext(ctx).Select("id").Where("user_id = ?", userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.Page[AgentRatingRow]{}, httpx.ErrNotFound
	}
	if err != nil {
		return httpx.Page[AgentRatingRow]{}, err
	}
	return s.ratingHistoryPage(ctx, a.ID, userID, page, size)
}

// AdminList returns one page of Agents for the management UI.
func (s *Service) AdminList(
	ctx context.Context, status, model string, page, size int,
) (httpx.Page[AgentView], error) {
	return s.listAgentPage(ctx, model, status, "rating", page, size)
}

// AdminUpdate disables/restores an Agent and optionally resets its rating. The
// agents row is locked first. Disabling revokes every live token in the same
// transaction; restoring only changes the status, so the owner must issue a
// fresh token afterwards.
func (s *Service) AdminUpdate(
	ctx context.Context, id int64, in AdminAgentInput,
) (*AgentView, error) {
	if in.Status != "" && in.Status != StatusActive && in.Status != StatusDisabled {
		return nil, httpx.ErrValidation("Agent 状态无效")
	}
	if in.Status == "" && !in.ResetRating {
		return nil, httpx.ErrValidation("没有需要更新的字段")
	}

	var view *AgentView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}

		now := s.now()
		updates := map[string]any{"updated_at": now}
		if in.Status != "" {
			updates["status"] = in.Status
		}
		if in.ResetRating {
			updates["rating"] = initialRating
			entry := AgentRatingHistory{
				AgentID:      a.ID,
				ProblemID:    0,
				Kind:         RatingReset,
				RatingBefore: a.Rating,
				RatingAfter:  initialRating,
				Delta:        initialRating - a.Rating,
				CreatedAt:    now,
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&Agent{}).Where("id = ?", a.ID).Updates(updates).Error; err != nil {
			return err
		}
		if in.Status == StatusDisabled {
			if err := tx.Model(&AgentToken{}).
				Where("agent_id = ? AND revoked_at IS NULL", a.ID).
				Update("revoked_at", now).Error; err != nil {
				return err
			}
		}

		if in.Status != "" {
			a.Status = in.Status
		}
		if in.ResetRating {
			a.Rating = initialRating
		}
		a.UpdatedAt = now
		updated, err := buildAgentView(tx, a)
		if err != nil {
			return err
		}
		view = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// AdminSubmissions returns one page of an Agent's submissions, newest first.
// This is an admin-only investigation view: unlike the public/user
// projections it includes code and objective answers. Human submission lists
// and counters are untouched.
func (s *Service) AdminSubmissions(
	ctx context.Context, agentID int64, page, size int,
) (httpx.Page[AdminAgentSubmissionView], error) {
	var a Agent
	err := s.db.WithContext(ctx).Select("id").First(&a, agentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.Page[AdminAgentSubmissionView]{}, httpx.ErrNotFound
	}
	if err != nil {
		return httpx.Page[AdminAgentSubmissionView]{}, err
	}

	page, size = normalizePage(page, size)

	var total int64
	err = s.db.WithContext(ctx).Model(&submission.Submission{}).
		Where("agent_id = ?", agentID).Count(&total).Error
	if err != nil {
		return httpx.Page[AdminAgentSubmissionView]{}, err
	}

	var rows []adminSubmissionRow
	err = s.db.WithContext(ctx).Table("submissions AS s").
		Select(`
			s.id, s.problem_id, p.title AS problem_title, p.slug AS problem_slug,
			s.language, s.status, s.time_used_ms, s.memory_used_kb, s.score,
			s.code, s.answer, s.error_message, s.created_at, s.judged_at,
			s.agent_settled`).
		Joins("JOIN problems AS p ON p.id = s.problem_id").
		Where("s.agent_id = ?", agentID).
		Order("s.id DESC").
		Limit(size).
		Offset((page - 1) * size).
		Scan(&rows).Error
	if err != nil {
		return httpx.Page[AdminAgentSubmissionView]{}, err
	}

	items := make([]AdminAgentSubmissionView, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.view())
	}
	return httpx.Page[AdminAgentSubmissionView]{Total: total, Items: items}, nil
}

type adminSubmissionRow struct {
	ID           int64               `gorm:"column:id"`
	ProblemID    int64               `gorm:"column:problem_id"`
	ProblemTitle string              `gorm:"column:problem_title"`
	ProblemSlug  string              `gorm:"column:problem_slug"`
	Language     submission.Language `gorm:"column:language"`
	Status       submission.Status   `gorm:"column:status"`
	TimeUsedMS   int                 `gorm:"column:time_used_ms"`
	MemoryUsedKB int                 `gorm:"column:memory_used_kb"`
	Score        int                 `gorm:"column:score"`
	Code         string              `gorm:"column:code"`
	Answer       string              `gorm:"column:answer"`
	ErrorMessage string              `gorm:"column:error_message"`
	CreatedAt    time.Time           `gorm:"column:created_at"`
	JudgedAt     *time.Time          `gorm:"column:judged_at"`
	AgentSettled bool                `gorm:"column:agent_settled"`
}

func (r adminSubmissionRow) view() AdminAgentSubmissionView {
	return AdminAgentSubmissionView{
		ID:           r.ID,
		ProblemID:    r.ProblemID,
		ProblemTitle: r.ProblemTitle,
		ProblemSlug:  r.ProblemSlug,
		Language:     r.Language,
		Status:       r.Status,
		TimeUsedMS:   r.TimeUsedMS,
		MemoryUsedKB: r.MemoryUsedKB,
		Score:        r.Score,
		Code:         r.Code,
		Answer:       r.Answer,
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt,
		JudgedAt:     r.JudgedAt,
		AgentSettled: r.AgentSettled,
	}
}

// listAgentPage is shared by the public leaderboard and the admin list. Both
// include the owner identity so clients can show the author without N+1 calls.
func (s *Service) listAgentPage(
	ctx context.Context, model, status, sort string, page, size int,
) (httpx.Page[AgentView], error) {
	page, size = normalizePage(page, size)

	base := func() *gorm.DB {
		q := s.db.WithContext(ctx).Table("agents AS a").
			Joins("JOIN users AS u ON u.id = a.user_id")
		if model != "" {
			q = q.Where("a.model_name = ?", model)
		}
		if status != "" {
			q = q.Where("a.status = ?", status)
		}
		if sort == "avg_time" {
			q = q.Where("a.passed_count > 0")
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return httpx.Page[AgentView]{}, err
	}

	var rows []agentListRow
	err := base().Select(`
			a.id, a.user_id, a.name, a.model_name, a.model_intro, a.status,
			a.rating, a.passed_count, a.attempted_count, a.avg_solve_time_ms,
			a.created_at, a.updated_at,
			u.id AS author_id, u.username AS author_username,
			u.nickname AS author_nickname`).
		Order(agentListOrder(sort)).
		Limit(size).
		Offset((page - 1) * size).
		Scan(&rows).Error
	if err != nil {
		return httpx.Page[AgentView]{}, err
	}

	items := make([]AgentView, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.view())
	}
	return httpx.Page[AgentView]{Total: total, Items: items}, nil
}

func agentListOrder(sort string) string {
	switch sort {
	case "passed":
		return "a.passed_count DESC, a.id ASC"
	case "avg_time":
		return "a.avg_solve_time_ms ASC, a.id ASC"
	default:
		return "a.rating DESC, a.id ASC"
	}
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return page, size
}

type agentListRow struct {
	ID             int64     `gorm:"column:id"`
	UserID         int64     `gorm:"column:user_id"`
	Name           string    `gorm:"column:name"`
	ModelName      string    `gorm:"column:model_name"`
	ModelIntro     string    `gorm:"column:model_intro"`
	Status         Status    `gorm:"column:status"`
	Rating         int       `gorm:"column:rating"`
	PassedCount    int       `gorm:"column:passed_count"`
	AttemptedCount int       `gorm:"column:attempted_count"`
	AvgSolveTimeMS int       `gorm:"column:avg_solve_time_ms"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	AuthorID       int64     `gorm:"column:author_id"`
	AuthorUsername string    `gorm:"column:author_username"`
	AuthorNickname string    `gorm:"column:author_nickname"`
}

func (r agentListRow) view() AgentView {
	return AgentView{
		ID:             r.ID,
		UserID:         r.UserID,
		Name:           r.Name,
		ModelName:      r.ModelName,
		ModelIntro:     r.ModelIntro,
		Status:         r.Status,
		Rating:         r.Rating,
		PassedCount:    r.PassedCount,
		AttemptedCount: r.AttemptedCount,
		AvgSolveTimeMS: r.AvgSolveTimeMS,
		Author: AgentAuthor{
			ID:       r.AuthorID,
			Username: r.AuthorUsername,
			Nickname: r.AuthorNickname,
		},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func (s *Service) ratingHistoryPage(
	ctx context.Context, agentID, viewerID int64, page, size int,
) (httpx.Page[AgentRatingRow], error) {
	var a Agent
	err := s.db.WithContext(ctx).Select("id, user_id").First(&a, agentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.Page[AgentRatingRow]{}, httpx.ErrNotFound
	}
	if err != nil {
		return httpx.Page[AgentRatingRow]{}, err
	}

	page, size = normalizePage(page, size)

	var total int64
	err = s.db.WithContext(ctx).Model(&AgentRatingHistory{}).
		Where("agent_id = ?", agentID).Count(&total).Error
	if err != nil {
		return httpx.Page[AgentRatingRow]{}, err
	}

	var rows []ratingHistoryRow
	now := s.now()
	err = s.db.WithContext(ctx).Raw(`
		WITH history AS (
			SELECT h.*, p.title, p.slug, p.is_public,
			       (p.id IS NOT NULL AND p.is_public AND NOT EXISTS (
			           SELECT 1 FROM contest_problems cp
			           JOIN contests c ON c.id = cp.contest_id
			           WHERE cp.problem_id = p.id AND c.end_time > $2
			       )) AS problem_visible
			FROM agent_rating_history h
			LEFT JOIN problems p ON p.id = h.problem_id
			WHERE h.agent_id = $1
		)
		SELECT id, COALESCE(problem_id, 0) AS problem_id,
		       CASE WHEN problem_visible THEN title ELSE '' END AS problem_title,
		       CASE WHEN problem_visible THEN slug ELSE '' END AS problem_slug,
		       problem_visible, submission_id, kind, difficulty, rating_before,
		       rating_after, delta, solve_time_ms, created_at
		FROM history
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`,
		agentID, now, size, (page-1)*size).Scan(&rows).Error
	if err != nil {
		return httpx.Page[AgentRatingRow]{}, err
	}

	items := make([]AgentRatingRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.view(viewerID == a.UserID))
	}
	return httpx.Page[AgentRatingRow]{Total: total, Items: items}, nil
}

type ratingHistoryRow struct {
	ID             int64      `gorm:"column:id"`
	ProblemID      int64      `gorm:"column:problem_id"`
	ProblemTitle   string     `gorm:"column:problem_title"`
	ProblemSlug    string     `gorm:"column:problem_slug"`
	ProblemVisible bool       `gorm:"column:problem_visible"`
	SubmissionID   *int64     `gorm:"column:submission_id"`
	Kind           RatingKind `gorm:"column:kind"`
	Difficulty     int        `gorm:"column:difficulty"`
	RatingBefore   int        `gorm:"column:rating_before"`
	RatingAfter    int        `gorm:"column:rating_after"`
	Delta          int        `gorm:"column:delta"`
	SolveTimeMS    int        `gorm:"column:solve_time_ms"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (r ratingHistoryRow) view(ownerViewer bool) AgentRatingRow {
	submissionID := r.SubmissionID
	if !r.ProblemVisible && !ownerViewer {
		submissionID = nil
	}
	return AgentRatingRow{
		ID:           r.ID,
		ProblemID:    r.ProblemID,
		ProblemTitle: r.ProblemTitle,
		ProblemSlug:  r.ProblemSlug,
		Kind:         r.Kind,
		Difficulty:   r.Difficulty,
		RatingBefore: r.RatingBefore,
		RatingAfter:  r.RatingAfter,
		Delta:        r.Delta,
		SolveTimeMS:  r.SolveTimeMS,
		SubmissionID: submissionID,
		CreatedAt:    r.CreatedAt,
	}
}

// GenerateToken rotates the Agent token: any live token is revoked and a new
// plaintext token is returned exactly once.
func (s *Service) GenerateToken(ctx context.Context, userID int64) (*TokenIssue, error) {
	var issue *TokenIssue
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a Agent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).First(&a).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if a.Status != StatusActive {
			return httpx.ErrAgentDisabled
		}

		now := s.now()
		if err := tx.Model(&AgentToken{}).
			Where("agent_id = ? AND revoked_at IS NULL", a.ID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}

		plain, hash, prefix, err := GenerateToken()
		if err != nil {
			return err
		}
		token := AgentToken{
			AgentID:     a.ID,
			TokenHash:   hash,
			TokenPrefix: prefix,
			CreatedAt:   now,
		}
		if err := tx.Create(&token).Error; err != nil {
			return err
		}
		meta := toTokenView(token)
		issue = &TokenIssue{Token: plain, Meta: meta}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issue, nil
}

// RevokeToken revokes the current live token, if any. It is idempotent.
func (s *Service) RevokeToken(ctx context.Context, userID int64) error {
	var a Agent
	err := s.db.WithContext(ctx).Select("id").Where("user_id = ?", userID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return httpx.ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&AgentToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", a.ID).
		Update("revoked_at", s.now()).Error
}

// AuthenticateToken resolves a plaintext Agent token. Revoked tokens, disabled
// Agents and banned owners are rejected. last_used_at is written at most once
// per minute to keep token authentication cheap.
func (s *Service) AuthenticateToken(ctx context.Context, raw string) (*AuthAgent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, httpx.ErrInvalidAgentToken
	}

	var row authRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT t.id AS token_id,
		       a.id, a.user_id, a.name, a.model_name, a.model_intro, a.status,
		       a.rating, a.passed_count, a.attempted_count, a.avg_solve_time_ms,
		       a.created_at, a.updated_at,
		       u.status AS user_status
		FROM agent_tokens t
		JOIN agents a ON a.id = t.agent_id
		JOIN users u ON u.id = a.user_id
		WHERE t.token_hash = ? AND t.revoked_at IS NULL`, HashToken(raw)).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.TokenID == 0 {
		return nil, httpx.ErrInvalidAgentToken
	}
	if row.Status != StatusActive {
		return nil, httpx.ErrAgentDisabled
	}
	if row.UserStatus == string(user.StatusBanned) {
		return nil, httpx.ErrUserBanned
	}

	now := s.now()
	err = s.db.WithContext(ctx).Model(&AgentToken{}).
		Where("id = ? AND (last_used_at IS NULL OR last_used_at < ?)",
			row.TokenID, now.Add(-lastUsedThrottle)).
		Update("last_used_at", now).Error
	if err != nil {
		return nil, err
	}

	return &AuthAgent{
		Agent: Agent{
			ID:             row.ID,
			UserID:         row.UserID,
			Name:           row.Name,
			ModelName:      row.ModelName,
			ModelIntro:     row.ModelIntro,
			Status:         row.Status,
			Rating:         row.Rating,
			PassedCount:    row.PassedCount,
			AttemptedCount: row.AttemptedCount,
			AvgSolveTimeMS: row.AvgSolveTimeMS,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		},
		TokenID: row.TokenID,
	}, nil
}

type authRow struct {
	TokenID        int64     `gorm:"column:token_id"`
	ID             int64     `gorm:"column:id"`
	UserID         int64     `gorm:"column:user_id"`
	Name           string    `gorm:"column:name"`
	ModelName      string    `gorm:"column:model_name"`
	ModelIntro     string    `gorm:"column:model_intro"`
	Status         Status    `gorm:"column:status"`
	Rating         int       `gorm:"column:rating"`
	PassedCount    int       `gorm:"column:passed_count"`
	AttemptedCount int       `gorm:"column:attempted_count"`
	AvgSolveTimeMS int       `gorm:"column:avg_solve_time_ms"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	UserStatus     string    `gorm:"column:user_status"`
}

type attemptRow struct {
	ID           int64         `gorm:"column:id"`
	ProblemID    int64         `gorm:"column:problem_id"`
	AttemptsUsed int           `gorm:"column:attempts_used"`
	Status       AttemptStatus `gorm:"column:status"`
	ClaimedAt    time.Time     `gorm:"column:claimed_at"`
	DeadlineAt   time.Time     `gorm:"column:deadline_at"`
	SolvedAt     *time.Time    `gorm:"column:solved_at"`
	SolveTimeMS  int           `gorm:"column:solve_time_ms"`
	ProblemTitle string        `gorm:"column:problem_title"`
	ProblemSlug  string        `gorm:"column:problem_slug"`
	Difficulty   int           `gorm:"column:difficulty"`
}

func (s *Service) currentAttempt(ctx context.Context, agentID int64) (*AttemptView, error) {
	var row attemptRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT a.id, a.problem_id, a.attempts_used, a.status, a.claimed_at,
		       a.deadline_at, a.solved_at, a.solve_time_ms,
		       p.title AS problem_title, p.slug AS problem_slug, p.difficulty
		FROM agent_problem_attempts a
		JOIN problems p ON p.id = a.problem_id
		WHERE a.agent_id = ? AND a.status = 'active'
		ORDER BY a.id DESC
		LIMIT 1`, agentID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return toAttemptView(row), nil
}

func (s *Service) agentView(ctx context.Context, a Agent) (*AgentView, error) {
	return buildAgentView(s.db.WithContext(ctx), a)
}

func buildAgentView(db *gorm.DB, a Agent) (*AgentView, error) {
	var author AgentAuthor
	err := db.Table("users").
		Select("id, username, nickname").
		Where("id = ?", a.UserID).
		Scan(&author).Error
	if err != nil {
		return nil, err
	}
	return &AgentView{
		ID:             a.ID,
		UserID:         a.UserID,
		Name:           a.Name,
		ModelName:      a.ModelName,
		ModelIntro:     a.ModelIntro,
		Status:         a.Status,
		Rating:         a.Rating,
		PassedCount:    a.PassedCount,
		AttemptedCount: a.AttemptedCount,
		AvgSolveTimeMS: a.AvgSolveTimeMS,
		Author:         author,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}, nil
}

func toTokenView(t AgentToken) AgentTokenView {
	return AgentTokenView{
		ID:          t.ID,
		TokenPrefix: t.TokenPrefix,
		CreatedAt:   t.CreatedAt,
		LastUsedAt:  t.LastUsedAt,
	}
}

func toAttemptView(row attemptRow) *AttemptView {
	remaining := MaxAttempts - row.AttemptsUsed
	if remaining < 0 {
		remaining = 0
	}
	return &AttemptView{
		ID:                row.ID,
		ProblemID:         row.ProblemID,
		ProblemTitle:      row.ProblemTitle,
		ProblemSlug:       row.ProblemSlug,
		Difficulty:        row.Difficulty,
		Status:            row.Status,
		AttemptsUsed:      row.AttemptsUsed,
		AttemptsRemaining: remaining,
		ClaimedAt:         row.ClaimedAt,
		DeadlineAt:        row.DeadlineAt,
		SolvedAt:          row.SolvedAt,
		SolveTimeMS:       row.SolveTimeMS,
	}
}

func normalizeAgentInput(in AgentInput) (AgentInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.ModelName = strings.TrimSpace(in.ModelName)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > agentInputMaxName {
		return in, httpx.ErrValidation("Agent 名称长度需为 1-100 个字符")
	}
	if n := utf8.RuneCountInString(in.ModelName); n < 1 || n > agentInputMaxName {
		return in, httpx.ErrValidation("模型名称长度需为 1-100 个字符")
	}
	if utf8.RuneCountInString(in.ModelIntro) > agentInputMaxIntro {
		return in, httpx.ErrValidation("模型简介不能超过 500 个字符")
	}
	return in, nil
}

func mapAgentWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uniq_agents_name":
			return httpx.ErrValidation("Agent 名称已存在")
		case "uniq_agents_user":
			return httpx.ErrValidation("每个用户只能创建一个 Agent")
		}
	}
	return err
}
