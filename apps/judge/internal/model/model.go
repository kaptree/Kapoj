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
	Score      int
	SortOrder  int
}

// JudgeCaseResult is stored in submissions.judge_result_json. Score is only
// used in memory for aggregation and is not serialized.
type JudgeCaseResult struct {
	TestcaseID int64  `json:"testcaseId"`
	Status     Status `json:"status"`
	TimeMS     int    `json:"timeMs"`
	MemoryKB   int    `json:"memoryKb"`
	Message    string `json:"message,omitempty"`
	Score      int    `json:"-"`
}
