// Package sandbox defines the judging sandbox contract.
package sandbox

import "context"

type CompileRequest struct {
	Language string
	Source   []byte
	Checker  bool
}

type CompileResult struct {
	ArtifactDir string
	ErrorOutput string
	Failed      bool
}

type CheckerFiles struct {
	AnswerPath string
	OutputPath string
}

type RunRequest struct {
	Language         string
	ArtifactDir      string
	InputPath        string
	Checker          *CheckerFiles
	TimeLimitMS      int
	MemoryLimitMB    int
	OutputLimitBytes int
}

type RunResult struct {
	Output          []byte
	Stderr          []byte
	ExitCode        int
	TimedOut        bool
	OOMKilled       bool
	TimeMS          int
	MemoryKB        int
	OutputTruncated bool
}

// RunCase is one testcase executed inside a shared run container.
type RunCase struct {
	InputPath string
}

// RunBatchRequest runs multiple testcases in one container. OnResult is called
// after each case; returning false stops the batch before the next case.
type RunBatchRequest struct {
	Language         string
	ArtifactDir      string
	Cases            []RunCase
	TimeLimitMS      int
	MemoryLimitMB    int
	OutputLimitBytes int
	OnResult         func(index int, result RunResult) bool
}

type Sandbox interface {
	Compile(ctx context.Context, req CompileRequest) (CompileResult, error)
	Run(ctx context.Context, req RunRequest) (RunResult, error)
	RunBatch(ctx context.Context, req RunBatchRequest) ([]RunResult, error)
	Cleanup(artifactDir string)
}
