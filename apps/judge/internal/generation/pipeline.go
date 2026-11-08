// Package generation runs AI testcase-generation tasks: it compiles the
// Python generator and the standard program, runs them in the sandbox to
// produce input/output files, writes those files to shared storage, and
// records the generated cases.
package generation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/storage"
)

const (
	generatorTimeLimitMS = 5000
	generatorMemoryMB    = 256
	maxCaseBytes         = 16 << 20
	maxErrorMessage      = 8 << 10
)

// Pipeline runs testcase generations.
type Pipeline struct {
	repo        *db.Repository
	storageRoot string
	sandbox     sandbox.Sandbox
}

// New builds a generation pipeline.
func New(repo *db.Repository, root string, sb sandbox.Sandbox) *Pipeline {
	return &Pipeline{repo: repo, storageRoot: root, sandbox: sb}
}

// failure marks a user-level generation failure (bad script or program). It is
// turned into a terminal "failed" status rather than an Asynq retry.
type failure struct{ message string }

func (f *failure) Error() string { return f.message }

// Run executes one generation. User-level failures (compile errors, empty
// output, timeouts, OOM, non-zero exits, truncation) are recorded as a
// terminal "failed" status and nil is returned. Infrastructure failures are
// returned so Asynq can record them.
func (p *Pipeline) Run(ctx context.Context, generationID int64) error {
	gen, err := p.repo.ClaimGeneration(ctx, generationID)
	if errors.Is(err, db.ErrSkip) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim generation: %w", err)
	}
	if !gen.Language.Valid() {
		return fmt.Errorf("unsupported language %q", gen.Language)
	}

	genKey := generatedKey(gen.ProblemID, gen.ID)

	// Clear any defensive leftovers from a previous attempt.
	if err := p.repo.ResetGenerationCases(ctx, gen.ID); err != nil {
		return err
	}
	if err := storage.RemoveAll(p.storageRoot, genKey); err != nil {
		return err
	}

	// Compile the Python generator.
	genCompiled, err := p.sandbox.Compile(ctx, sandbox.CompileRequest{
		Language: string(model.LanguagePython3), Source: []byte(gen.GeneratorCode),
	})
	if err != nil {
		return p.failInfra(ctx, gen, genKey, err)
	}
	if genCompiled.ArtifactDir != "" {
		defer p.sandbox.Cleanup(genCompiled.ArtifactDir)
	}
	if genCompiled.Failed {
		return p.fail(ctx, gen, genKey, "造数据脚本编译失败："+genCompiled.ErrorOutput)
	}

	// Compile the standard program.
	stdCompiled, err := p.sandbox.Compile(ctx, sandbox.CompileRequest{
		Language: string(gen.Language), Source: []byte(gen.StandardCode),
	})
	if err != nil {
		return p.failInfra(ctx, gen, genKey, err)
	}
	if stdCompiled.ArtifactDir != "" {
		defer p.sandbox.Cleanup(stdCompiled.ArtifactDir)
	}
	if stdCompiled.Failed {
		return p.fail(ctx, gen, genKey, "标程编译失败："+stdCompiled.ErrorOutput)
	}

	work, err := os.MkdirTemp("", "kapoj-gen-*")
	if err != nil {
		return p.failInfra(ctx, gen, genKey, err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	for i := 1; i <= gen.TestcaseCount; i++ {
		if err := p.runGroup(ctx, work, gen, genCompiled.ArtifactDir, stdCompiled.ArtifactDir, genKey, i); err != nil {
			var f *failure
			if errors.As(err, &f) {
				return p.fail(ctx, gen, genKey, f.message)
			}
			return p.failInfra(ctx, gen, genKey, err)
		}
	}

	return p.repo.FinishGeneration(ctx, gen.ID, model.GenerationReady, "")
}

// runGroup generates one input/output pair. User-level failures are returned
// as *failure; any other error is an infrastructure failure.
func (p *Pipeline) runGroup(
	ctx context.Context, work string, gen *model.Generation,
	genArtifact, stdArtifact, genKey string, i int,
) error {
	seedPath := filepath.Join(work, "seed.txt")
	if err := os.WriteFile(seedPath, []byte(strconv.Itoa(i)+"\n"), 0o644); err != nil {
		return err
	}

	inputKey := fmt.Sprintf("%s/%d.in", genKey, i)
	outputKey := fmt.Sprintf("%s/%d.out", genKey, i)

	genRun, err := p.sandbox.Run(ctx, sandbox.RunRequest{
		Language: string(model.LanguagePython3), ArtifactDir: genArtifact, InputPath: seedPath,
		TimeLimitMS: generatorTimeLimitMS, MemoryLimitMB: generatorMemoryMB,
		OutputLimitBytes: maxCaseBytes,
	})
	if err != nil {
		return err
	}
	if reason, ok := validateRun(genRun, "生成器"); !ok {
		return &failure{message: fmt.Sprintf("第 %d 组 %s", i, reason)}
	}

	inputSize, err := storage.Put(p.storageRoot, inputKey, bytes.NewReader(genRun.Output), maxCaseBytes)
	if err != nil {
		return err
	}

	inputPath := filepath.Join(work, "input.txt")
	if err := os.WriteFile(inputPath, genRun.Output, 0o644); err != nil {
		return err
	}

	stdRun, err := p.sandbox.Run(ctx, sandbox.RunRequest{
		Language: string(gen.Language), ArtifactDir: stdArtifact, InputPath: inputPath,
		TimeLimitMS: gen.TimeLimitMS, MemoryLimitMB: gen.MemoryLimitMB,
		OutputLimitBytes: maxCaseBytes,
	})
	if err != nil {
		return err
	}
	if reason, ok := validateRun(stdRun, "标程"); !ok {
		return &failure{message: fmt.Sprintf("第 %d 组 %s", i, reason)}
	}

	outputSize, err := storage.Put(p.storageRoot, outputKey, bytes.NewReader(stdRun.Output), maxCaseBytes)
	if err != nil {
		return err
	}

	return p.repo.AddGenerationCase(ctx, &model.GenerationCase{
		GenerationID: gen.ID, SortOrder: i,
		InputFile: inputKey, OutputFile: outputKey,
		InputSize: inputSize, OutputSize: outputSize,
	})
}

// validateRun turns a sandbox result into a concise failure reason, or ok=true
// for a successful run with non-empty, untruncated output.
func validateRun(run sandbox.RunResult, who string) (reason string, ok bool) {
	switch {
	case run.TimedOut:
		return who + "运行超时", false
	case run.OOMKilled:
		return who + "内存超限", false
	case run.ExitCode != 0:
		msg := strings.TrimSpace(string(run.Stderr))
		if msg == "" {
			msg = fmt.Sprintf("退出码 %d", run.ExitCode)
		}
		return who + "运行失败：" + truncate(msg, maxErrorMessage), false
	case run.OutputTruncated:
		return who + "输出超过限制", false
	case len(strings.TrimSpace(string(run.Output))) == 0:
		return who + "输出为空", false
	default:
		return "", true
	}
}

// fail cleans up any generated files/cases and records a terminal failure.
func (p *Pipeline) fail(ctx context.Context, gen *model.Generation, genKey, message string) error {
	if err := p.repo.ResetGenerationCases(ctx, gen.ID); err != nil {
		return err
	}
	if err := storage.RemoveAll(p.storageRoot, genKey); err != nil {
		return err
	}
	return p.repo.FinishGeneration(ctx, gen.ID, model.GenerationFailed, truncate(message, maxErrorMessage))
}

// failInfra performs best-effort cleanup after an infrastructure error and
// marks the generation failed so it can never be left stuck in "running" with
// partial cases or files. The original error is returned so Asynq/logs still
// observe the infrastructure failure.
func (p *Pipeline) failInfra(ctx context.Context, gen *model.Generation, genKey string, cause error) error {
	_ = p.repo.ResetGenerationCases(ctx, gen.ID)
	_ = storage.RemoveAll(p.storageRoot, genKey)
	message := "系统错误"
	if cause != nil {
		message = "系统错误：" + truncate(cause.Error(), maxErrorMessage)
	}
	_ = p.repo.FinishGeneration(ctx, gen.ID, model.GenerationFailed, message)
	return cause
}

// generatedKey is the shared storage directory for one generation's files.
func generatedKey(problemID, generationID int64) string {
	return "problems/" + strconv.FormatInt(problemID, 10) + "/generated/" + strconv.FormatInt(generationID, 10)
}

// truncate shortens s to at most max bytes without splitting a UTF-8 rune.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}
