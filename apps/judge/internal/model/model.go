package model

import "time"

type Language string

const (
	LanguageC       Language = "c"
	LanguageCPP     Language = "cpp"
	LanguagePython3 Language = "python3"
	LanguageJava    Language = "java"
	LanguageGo      Language = "go"
)

func (l Language) Valid() bool {
	switch l {
	case LanguageC, LanguageCPP, LanguagePython3, LanguageJava, LanguageGo:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusPending             Status = "pending"
	StatusJudging             Status = "judging"
	StatusAccepted            Status = "accepted"
	StatusWrongAnswer         Status = "wrong_answer"
	StatusTimeLimitExceeded   Status = "time_limit_exceeded"
	StatusMemoryLimitExceeded Status = "memory_limit_exceeded"
	StatusRuntimeError        Status = "runtime_error"
	StatusCompileError        Status = "compile_error"
	StatusSystemError         Status = "system_error"
)

func (s Status) Terminal() bool {
	switch s {
	case StatusAccepted, StatusWrongAnswer, StatusTimeLimitExceeded,
		StatusMemoryLimitExceeded, StatusRuntimeError, StatusCompileError,
		StatusSystemError:
		return true
	default:
		return false
	}
}

type Submission struct {
	ID           int64
	UserID       int64
	ProblemID    int64
	AgentID      *int64
	Language     Language
	Code         string
	Answer       string
	Status       Status
	ErrorMessage string
	CreatedAt    time.Time
	JudgedAt     *time.Time
}

type Problem struct {
	ID            int64
	Type          string
	TimeLimitMS   int
	MemoryLimitMB int
	CheckerPath   string
	Answers       []string
}

type Testcase struct {
	ID         int64
	InputFile  string
	OutputFile string
	InputSize  int64
	OutputSize int64
	Score      int
	SortOrder  int
}

// JudgeSettings mirrors the judge_settings singleton row shared with the
// server. All byte fields use bytes.
type JudgeSettings struct {
	OutputFloorBytes     int64
	OutputSlackBytes     int64
	MaxOutputBytes       int64
	MaxInputFileBytes    int64
	MaxOutputFileBytes   int64
	MaxBatchTotalBytes   int64
	MaxProblemTotalBytes int64
}

// DefaultJudgeSettings returns the limits used when the judge_settings row is
// missing or unreadable. They mirror apps/server/internal/judgesettings.
func DefaultJudgeSettings() JudgeSettings {
	return JudgeSettings{
		OutputFloorBytes:     1 << 20,
		OutputSlackBytes:     1 << 20,
		MaxOutputBytes:       16 << 20,
		MaxInputFileBytes:    16 << 20,
		MaxOutputFileBytes:   16 << 20,
		MaxBatchTotalBytes:   200 << 20,
		MaxProblemTotalBytes: 200 << 20,
	}
}

// JudgeCaseResult is stored in submissions.judge_result_json. Score is used
// for aggregation and is serialized so the submission detail can show the
// per-testcase score.
type JudgeCaseResult struct {
	TestcaseID int64  `json:"testcaseId"`
	Status     Status `json:"status"`
	TimeMS     int    `json:"timeMs"`
	CPUTimeMS  int    `json:"cpuMs"`
	MemoryKB   int    `json:"memoryKb"`
	Message    string `json:"message,omitempty"`
	Score      int    `json:"score,omitempty"`
}
