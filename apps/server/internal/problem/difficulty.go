package problem

import "time"

// Values of problems.difficulty_source.
const (
	DifficultySourceAIInitial  = "ai_initial"
	DifficultySourceAIReassess = "ai_reassess"
	DifficultySourcePassRate   = "pass_rate"
	DifficultySourceManual     = "manual"
	DifficultySourceMigration  = "migration"
	DifficultySourceImport     = "import"
)

// Values of problems.difficulty_prior_source.
const (
	DifficultyPriorSourceAI       = "ai"
	DifficultyPriorSourceDefault  = "default"
	DifficultyPriorSourceMigrated = "migrated"
	DifficultyPriorSourceManual   = "manual"
	DifficultyPriorSourceImport   = "import"
)

// DifficultyHistory is one immutable difficulty change for a problem. OldScore
// is 0 when the problem was created. RunID is set only for scheduled recalcs.
type DifficultyHistory struct {
	ID               int64     `gorm:"primaryKey"`
	ProblemID        int64     `gorm:"not null;index:idx_problem_difficulty_history_problem,priority:1"`
	OldScore         int       `gorm:"not null"`
	NewScore         int       `gorm:"not null"`
	Prior            int       `gorm:"not null"`
	Source           string    `gorm:"size:20;not null"`
	SampleUsers      int       `gorm:"not null;default:0"`
	AcceptedUsers    int       `gorm:"not null;default:0"`
	PassRate         float64   `gorm:"type:numeric(6,5);not null;default:0"`
	SmoothedPassRate float64   `gorm:"type:numeric(6,5);not null;default:0"`
	TargetScore      int       `gorm:"not null;default:0"`
	Confidence       float64   `gorm:"type:numeric(4,3);not null;default:0"`
	Reason           string    `gorm:"type:text;not null;default:''"`
	RunID            *int64    `gorm:"index:idx_problem_difficulty_history_run"`
	CreatedBy        int64     `gorm:"not null;default:0"`
	CreatedAt        time.Time `gorm:"type:timestamptz;not null;index:idx_problem_difficulty_history_problem,priority:2,sort:desc"`
}

// TableName keeps the table name from the design document.
func (DifficultyHistory) TableName() string { return "problem_difficulty_history" }
