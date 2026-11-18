// Package agent holds the Agent Arena domain models.
package agent

import "time"

// Status is the lifecycle state of an Agent.
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// AttemptStatus is the lifecycle state of one problem attempt.
type AttemptStatus string

const (
	AttemptActive    AttemptStatus = "active"
	AttemptSolved    AttemptStatus = "solved"
	AttemptExhausted AttemptStatus = "exhausted"
	AttemptAbandoned AttemptStatus = "abandoned"
	AttemptExpired   AttemptStatus = "expired"
)

// RatingKind classifies one rating history entry.
type RatingKind string

const (
	RatingSolve     RatingKind = "solve"
	RatingAbandon   RatingKind = "abandon"
	RatingTimeout   RatingKind = "timeout"
	RatingExhausted RatingKind = "exhausted"
	RatingReset     RatingKind = "reset"
)

// Agent is a user-owned AI agent that solves problems through the Agent API.
// Each user may own one Agent and Agent names are globally unique.
type Agent struct {
	ID             int64     `gorm:"primaryKey"`
	UserID         int64     `gorm:"not null;uniqueIndex:uniq_agents_user"`
	Name           string    `gorm:"size:100;not null;uniqueIndex:uniq_agents_name"`
	ModelName      string    `gorm:"size:100;not null;default:''"`
	ModelIntro     string    `gorm:"size:500;not null;default:''"`
	Status         Status    `gorm:"size:20;not null;default:active"`
	Rating         int       `gorm:"not null;default:1500"`
	PassedCount    int       `gorm:"not null;default:0"`
	AttemptedCount int       `gorm:"not null;default:0"`
	AvgSolveTimeMS int       `gorm:"column:avg_solve_time_ms;not null;default:0"`
	CreatedAt      time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt      time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (Agent) TableName() string { return "agents" }

// AgentToken is a hashed API token. Revoked tokens are kept for auditing; the
// partial unique index uniq_agent_tokens_active allows at most one live token
// per Agent.
type AgentToken struct {
	ID          int64      `gorm:"primaryKey"`
	AgentID     int64      `gorm:"not null;index"`
	TokenHash   string     `gorm:"column:token_hash;type:char(64);not null;uniqueIndex"`
	TokenPrefix string     `gorm:"size:24;not null;default:''"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null"`
	LastUsedAt  *time.Time `gorm:"type:timestamptz"`
	RevokedAt   *time.Time `gorm:"type:timestamptz"`
}

// TableName keeps the table name from the design document.
func (AgentToken) TableName() string { return "agent_tokens" }

// AgentProblemAttempt is one Agent claim on a problem. A unique (agent_id,
// problem_id) pair keeps the attempt history single-rowed; the partial unique
// index uniq_agent_attempts_active keeps at most one active claim per Agent.
type AgentProblemAttempt struct {
	ID               int64         `gorm:"primaryKey"`
	AgentID          int64         `gorm:"not null;uniqueIndex:uniq_agent_problem_attempts,priority:1;index:idx_agent_problem_attempts_agent_status,priority:1"`
	ProblemID        int64         `gorm:"not null;uniqueIndex:uniq_agent_problem_attempts,priority:2"`
	AttemptsUsed     int           `gorm:"not null;default:0"`
	Status           AttemptStatus `gorm:"size:20;not null;default:active;index:idx_agent_problem_attempts_agent_status,priority:2"`
	ClaimedAt        time.Time     `gorm:"type:timestamptz;not null"`
	DeadlineAt       time.Time     `gorm:"type:timestamptz;not null"`
	LastSubmissionAt *time.Time    `gorm:"type:timestamptz"`
	SolvedAt         *time.Time    `gorm:"type:timestamptz"`
	SolveTimeMS      int           `gorm:"column:solve_time_ms;not null;default:0"`
	CreatedAt        time.Time     `gorm:"type:timestamptz;not null"`
	UpdatedAt        time.Time     `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name from the design document.
func (AgentProblemAttempt) TableName() string { return "agent_problem_attempts" }

// AgentRatingHistory is one immutable Agent rating change. ProblemID is
// nullable so reset entries can exist without a problem.
type AgentRatingHistory struct {
	ID           int64 `gorm:"primaryKey"`
	AgentID      int64 `gorm:"not null;index:idx_agent_rating_history_agent_created,priority:1"`
	ProblemID    int64
	SubmissionID *int64
	Kind         RatingKind `gorm:"size:20;not null"`
	Difficulty   int        `gorm:"not null;default:0"`
	RatingBefore int        `gorm:"not null;default:0"`
	RatingAfter  int        `gorm:"not null;default:0"`
	Delta        int        `gorm:"not null;default:0"`
	SolveTimeMS  int        `gorm:"column:solve_time_ms;not null;default:0"`
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null;index:idx_agent_rating_history_agent_created,priority:2"`
}

// TableName keeps the singular history table name from the design document.
func (AgentRatingHistory) TableName() string { return "agent_rating_history" }
