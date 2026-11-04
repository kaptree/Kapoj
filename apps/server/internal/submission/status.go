package submission

// Status is the lifecycle state of a submission.
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

// Terminal reports whether no further judging is expected.
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

// Language is a supported programming language.
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
