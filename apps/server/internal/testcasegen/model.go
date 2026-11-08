// Package testcasegen models AI-driven testcase generation and the API/queue
// contract that ties the server (create/get) to the judge process (run/confirm).
package testcasegen

import "time"

// Status is the lifecycle state of a generation.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusReady     Status = "ready"
	StatusFailed    Status = "failed"
	StatusConfirmed Status = "confirmed"
	StatusDiscarded Status = "discarded"
)

// Generation records one AI testcase generation request and the resulting
// Python generator script.
type Generation struct {
	ID            int64     `gorm:"primaryKey"`
	ProblemID     int64     `gorm:"not null;index:idx_generations_problem_created,priority:1"`
	UserID        int64     `gorm:"not null"`
	Status        Status    `gorm:"size:20;not null"`
	Language      string    `gorm:"size:20;not null"`
	StandardCode  string    `gorm:"type:text;not null"`
	GeneratorCode string    `gorm:"type:text;not null"`
	TestcaseCount int       `gorm:"not null"`
	TimeLimitMS   int       `gorm:"not null"`
	MemoryLimitMB int       `gorm:"not null"`
	ErrorMessage  string    `gorm:"type:text;not null;default:''"`
	CreatedAt     time.Time `gorm:"type:timestamptz;not null;index:idx_generations_problem_created,priority:2"`
	UpdatedAt     time.Time `gorm:"type:timestamptz;not null"`
}

func (Generation) TableName() string { return "testcase_generations" }

// GenerationCase is one generated input/output pair produced by the judge.
type GenerationCase struct {
	ID           int64  `gorm:"primaryKey"`
	GenerationID int64  `gorm:"not null;uniqueIndex:uniq_generation_cases_order,priority:1"`
	SortOrder    int    `gorm:"not null;uniqueIndex:uniq_generation_cases_order,priority:2"`
	InputFile    string `gorm:"size:255;not null"`
	OutputFile   string `gorm:"size:255;not null"`
	InputSize    int64  `gorm:"not null;default:0"`
	OutputSize   int64  `gorm:"not null;default:0"`
}

func (GenerationCase) TableName() string { return "testcase_generation_cases" }
