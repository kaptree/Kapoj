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
	agentInputMaxIntro = 500
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
	var author AgentAuthor
	err := s.db.WithContext(ctx).Table("users").
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
