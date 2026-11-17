package jobs

import (
	"context"
	"time"
)

// Run triggers.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

// Run statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

// DefaultTimezone is used when a job definition does not specify one.
const DefaultTimezone = "Asia/Shanghai"

// Job is one registered scheduled job. The row keeps the admin-editable
// schedule and the most recent run outcome.
type Job struct {
	ID          int64      `gorm:"primaryKey"`
	Key         string     `gorm:"size:100;not null;uniqueIndex"`
	Name        string     `gorm:"size:100;not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	CronExpr    string     `gorm:"size:100;not null"`
	Timezone    string     `gorm:"size:64;not null;default:'Asia/Shanghai'"`
	Enabled     bool       `gorm:"not null;default:true;index:idx_scheduled_jobs_due,priority:1"`
	NextRunAt   *time.Time `gorm:"type:timestamptz;index:idx_scheduled_jobs_due,priority:2"`
	LastRunAt   *time.Time `gorm:"type:timestamptz"`
	LastStatus  string     `gorm:"size:20;not null;default:''"`
	LastError   string     `gorm:"type:text;not null;default:''"`
	UpdatedBy   int64      `gorm:"not null;default:0"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name stable.
func (Job) TableName() string { return "scheduled_jobs" }

// Run is one execution of a scheduled job. A partial unique index on job_key
// for queued/running rows guarantees at most one active run per job.
type Run struct {
	ID              int64          `gorm:"primaryKey"`
	JobKey          string         `gorm:"size:100;not null;index:idx_scheduled_job_runs_job_created,priority:1"`
	Trigger         string         `gorm:"size:20;not null"`
	Status          string         `gorm:"size:20;not null;index"`
	ProgressCurrent int            `gorm:"not null;default:0"`
	ProgressTotal   int            `gorm:"not null;default:0"`
	StatsJSON       map[string]any `gorm:"type:jsonb;not null;serializer:json"`
	ErrorMessage    string         `gorm:"type:text;not null;default:''"`
	CreatedBy       int64          `gorm:"not null;default:0"`
	CreatedAt       time.Time      `gorm:"type:timestamptz;not null;index:idx_scheduled_job_runs_job_created,priority:2,sort:desc"`
	StartedAt       *time.Time     `gorm:"type:timestamptz"`
	// HeartbeatAt is refreshed by the handler through Reporter. The stale-run
	// reconciler only fails a running run when this heartbeat expires, so a
	// long batch that keeps reporting progress is never killed.
	HeartbeatAt *time.Time `gorm:"type:timestamptz"`
	FinishedAt  *time.Time `gorm:"type:timestamptz"`
}

// TableName keeps the table name stable.
func (Run) TableName() string { return "scheduled_job_runs" }

// JobView is the admin-facing read model for a scheduled job.
type JobView struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CronExpr    string     `json:"cronExpr"`
	Timezone    string     `json:"timezone"`
	Enabled     bool       `json:"enabled"`
	NextRunAt   *time.Time `json:"nextRunAt"`
	LastRunAt   *time.Time `json:"lastRunAt"`
	LastStatus  string     `json:"lastStatus"`
	LastError   string     `json:"lastError"`
	ActiveRun   *RunView   `json:"activeRun"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// JobInput is the admin-editable schedule of a job.
type JobInput struct {
	Enabled  bool   `json:"enabled"`
	CronExpr string `json:"cronExpr"`
	Timezone string `json:"timezone"`
}

// RunView is the admin-facing read model for a job run.
type RunView struct {
	ID              int64          `json:"id"`
	JobKey          string         `json:"jobKey"`
	Trigger         string         `json:"trigger"`
	Status          string         `json:"status"`
	ProgressCurrent int            `json:"progressCurrent"`
	ProgressTotal   int            `json:"progressTotal"`
	Stats           map[string]any `json:"stats"`
	ErrorMessage    string         `json:"errorMessage"`
	CreatedBy       int64          `json:"createdBy"`
	CreatedAt       time.Time      `json:"createdAt"`
	StartedAt       *time.Time     `json:"startedAt"`
	FinishedAt      *time.Time     `json:"finishedAt"`
}

// RunFilter narrows the run history listing.
type RunFilter struct {
	JobKey string
	Status string
}

// Reporter lets a handler publish progress and stats while it runs.
type Reporter interface {
	SetProgress(current, total int)
	SetStats(stats map[string]any)
}

// Handler executes one job run.
type Handler func(ctx context.Context, run *Run, reporter Reporter) error

// Definition describes a job handler registered at startup.
type Definition struct {
	Key         string
	Name        string
	Description string
	DefaultCron string
	Timezone    string
	Handler     Handler
}
