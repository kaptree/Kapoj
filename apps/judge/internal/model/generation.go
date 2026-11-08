package model

// Generation statuses used by the judge while running an AI testcase
// generation. The server owns the full lifecycle (which also includes
// confirmed and discarded); the judge only ever writes running, ready and
// failed.
const (
	GenerationPending = "pending"
	GenerationRunning = "running"
	GenerationReady   = "ready"
	GenerationFailed  = "failed"
)

// Generation is the subset of a testcase_generations row the judge needs to
// compile the generator and standard program and emit testcase files.
type Generation struct {
	ID            int64
	ProblemID     int64
	Language      Language
	StandardCode  string
	GeneratorCode string
	TestcaseCount int
	TimeLimitMS   int
	MemoryLimitMB int
}

// GenerationCase is one generated input/output pair recorded for a generation.
type GenerationCase struct {
	ID           int64
	GenerationID int64
	SortOrder    int
	InputFile    string
	OutputFile   string
	InputSize    int64
	OutputSize   int64
}
