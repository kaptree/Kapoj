package similarity

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// Run status lifecycle values.
const (
	RunStatusQueued    = "queued"
	RunStatusRunning   = "running"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
)

// DefaultThresholdPercent is the threshold used when a problem has no saved
// settings yet.
const DefaultThresholdPercent = 80

// Settings is the per-problem similarity configuration for one contest.
type Settings struct {
	ID               int64     `gorm:"primaryKey"`
	ContestID        int64     `gorm:"not null;uniqueIndex:uniq_similarity_settings_problem,priority:1"`
	ProblemID        int64     `gorm:"not null;uniqueIndex:uniq_similarity_settings_problem,priority:2"`
	ThresholdPercent int       `gorm:"not null;default:80"`
	IgnoreCommonCode bool      `gorm:"not null;default:true"`
	BaseLanguage     string    `gorm:"size:20;not null;default:''"`
	BaseCode         string    `gorm:"type:text;not null;default:''"`
	UpdatedBy        int64     `gorm:"not null;default:0"`
	UpdatedAt        time.Time `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name stable.
func (Settings) TableName() string { return "contest_similarity_settings" }

// Run is one similarity computation run for a single contest problem.
type Run struct {
	ID               int64      `gorm:"primaryKey"`
	ContestID        int64      `gorm:"not null;index:idx_similarity_runs_problem,priority:1"`
	ProblemID        int64      `gorm:"not null;index:idx_similarity_runs_problem,priority:2"`
	Status           string     `gorm:"size:20;not null;index"`
	ThresholdPercent int        `gorm:"not null"`
	IgnoreCommonCode bool       `gorm:"not null;default:true"`
	BaseLanguage     string     `gorm:"size:20;not null;default:''"`
	BaseCode         string     `gorm:"type:text;not null;default:''"`
	Engine           string     `gorm:"size:30;not null"`
	EngineVersion    string     `gorm:"size:20;not null"`
	RequestedBy      int64      `gorm:"not null;default:0"`
	SubmissionCount  int        `gorm:"not null;default:0"`
	MatchCount       int        `gorm:"not null;default:0"`
	ErrorMessage     string     `gorm:"type:text;not null;default:''"`
	StartedAt        *time.Time `gorm:"type:timestamptz"`
	FinishedAt       *time.Time `gorm:"type:timestamptz"`
	CreatedAt        time.Time  `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name stable.
func (Run) TableName() string { return "contest_similarity_runs" }

// Match is one reported similarity pair for a run.
type Match struct {
	ID                   int64               `gorm:"primaryKey"`
	RunID                int64               `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:1"`
	ContestID            int64               `gorm:"not null;index"`
	ProblemID            int64               `gorm:"not null;index"`
	OriginalSubmissionID int64               `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:2"`
	SuspectSubmissionID  int64               `gorm:"not null;uniqueIndex:uniq_similarity_match_pair,priority:3"`
	OriginalUserID       int64               `gorm:"not null"`
	SuspectUserID        int64               `gorm:"not null"`
	Language             submission.Language `gorm:"size:20;not null"`
	SimilarityPercent    float64             `gorm:"type:double precision;not null"`
	DicePercent          float64             `gorm:"type:double precision;not null"`
	SharedFingerprints   int                 `gorm:"not null"`
	OriginalFingerprints int                 `gorm:"not null"`
	SuspectFingerprints  int                 `gorm:"not null"`
	OriginalCreatedAt    time.Time           `gorm:"type:timestamptz;not null"`
	SuspectCreatedAt     time.Time           `gorm:"type:timestamptz;not null"`
	CreatedAt            time.Time           `gorm:"type:timestamptz;not null"`
}

// TableName keeps the table name stable.
func (Match) TableName() string { return "contest_similarity_matches" }
