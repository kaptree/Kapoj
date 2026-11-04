package submission

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// JudgeCaseResult is one entry in submissions.judge_result_json.
type JudgeCaseResult struct {
	TestcaseID int64  `json:"testcaseId"`
	Status     Status `json:"status"`
	TimeMS     int    `json:"timeMs"`
	MemoryKB   int    `json:"memoryKb"`
	Message    string `json:"message,omitempty"`
}

// Submission is a user's code submission and its judging result.
type Submission struct {
	ID           int64                         `gorm:"primaryKey"`
	UserID       int64                         `gorm:"not null;index:idx_submissions_user_created,priority:1;index:idx_submissions_contest_user,priority:2"`
	ProblemID    int64                         `gorm:"not null;index:idx_submissions_problem_status,priority:1"`
	ContestID    *int64                        `gorm:"index:idx_submissions_contest_user,priority:1"`
	Language     Language                      `gorm:"size:20;not null"`
	Code         string                        `gorm:"type:text;not null"`
	Status       Status                        `gorm:"size:30;not null;default:pending;index:idx_submissions_problem_status,priority:2"`
	TimeUsedMS   int                           `gorm:"column:time_used_ms;not null;default:0"`
	MemoryUsedKB int                           `gorm:"column:memory_used_kb;not null;default:0"`
	Score        int                           `gorm:"not null;default:0"`
	ErrorMessage string                        `gorm:"type:text;not null;default:''"`
	JudgeResult  dbtypes.List[JudgeCaseResult] `gorm:"column:judge_result_json;type:jsonb;not null;default:'[]'"`
	CreatedAt    time.Time                     `gorm:"type:timestamptz;not null;index:idx_submissions_user_created,priority:2"`
	JudgedAt     *time.Time                    `gorm:"type:timestamptz"`

	User    user.User       `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
	Problem problem.Problem `gorm:"foreignKey:ProblemID;constraint:OnDelete:RESTRICT" json:"-"`
}
