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

type Sandbox interface {
	Compile(ctx context.Context, req CompileRequest) (CompileResult, error)
	Run(ctx context.Context, req RunRequest) (RunResult, error)
	Cleanup(artifactDir string)
}
