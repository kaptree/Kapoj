package submission

import (
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// JudgeCaseResult is one entry in submissions.judge_result_json.
type JudgeCaseResult struct {
	TestcaseID int64  `json:"testcaseId"`
	Status     Status `json:"status"`
	TimeMS     int    `json:"timeMs"`
	CPUTimeMS  int    `json:"cpuMs"`
	MemoryKB   int    `json:"memoryKb"`
	Message    string `json:"message,omitempty"`
	// Score is nil for historical rows written before per-testcase scores were
	// stored; the frontend renders those as "—" instead of a fake 0.
	Score *int `json:"score,omitempty"`
}

// Submission is a user's code submission and its judging result.
type Submission struct {
	ID              int64                         `gorm:"primaryKey"`
	UserID          int64                         `gorm:"not null;index:idx_submissions_user_created,priority:1;index:idx_submissions_contest_user,priority:2"`
	ProblemID       int64                         `gorm:"not null;index:idx_submissions_problem_status,priority:1"`
	ContestID       *int64                        `gorm:"index:idx_submissions_contest_user,priority:1"`
	AgentID         *int64                        `gorm:"index:idx_submissions_agent_settled,priority:1;index"`
	AgentAttemptID  *int64                        `gorm:"index"`
	AgentSettled    bool                          `gorm:"not null;default:false;index:idx_submissions_agent_settled,priority:2"`
	Language        Language                      `gorm:"size:20;not null"`
	Code            string                        `gorm:"type:text;not null"`
	Answer          string                        `gorm:"type:text;not null;default:''"`
	Status          Status                        `gorm:"size:30;not null;default:pending;index:idx_submissions_problem_status,priority:2"`
	TimeUsedMS      int                           `gorm:"column:time_used_ms;not null;default:0"`
	CPUUsedMS       int                           `gorm:"column:cpu_time_ms;not null;default:0"`
	MemoryUsedKB    int                           `gorm:"column:memory_used_kb;not null;default:0"`
	Score           int                           `gorm:"not null;default:0"`
	CodeLength      int                           `gorm:"column:code_length;not null;default:0"`
	AcceptedCounted bool                          `gorm:"column:accepted_counted;not null;default:false"`
	RejudgedAt      *time.Time                    `gorm:"column:rejudged_at"`
	ErrorMessage    string                        `gorm:"type:text;not null;default:''"`
	JudgeResult     dbtypes.List[JudgeCaseResult] `gorm:"column:judge_result_json;type:jsonb;not null;default:'[]'"`
	CreatedAt       time.Time                     `gorm:"type:timestamptz;not null;index:idx_submissions_user_created,priority:2"`
	JudgedAt        *time.Time                    `gorm:"type:timestamptz"`

	User    user.User        `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"-"`
	Problem problem.Problem  `gorm:"foreignKey:ProblemID;constraint:OnDelete:RESTRICT" json:"-"`
	Contest *contest.Contest `gorm:"foreignKey:ContestID;constraint:OnDelete:RESTRICT" json:"-"`
}
