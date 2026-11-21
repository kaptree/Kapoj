// Package pipeline turns a queued submission into a stored verdict.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/events"
	"github.com/kaptree/kapoj/apps/judge/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/storage"
	"github.com/kaptree/kapoj/apps/judge/internal/verdict"
)

const (
	maxCheckerBytes       = 256 << 10
	maxMessageBytes       = 4096
	maxCompileMessageSize = 64 << 10
	checkerTimeLimitMS    = 5000
	checkerMemoryMB       = 256
	checkerOutputBytes    = 4096
)

// GamePublisher 投递游戏化事件；失败只记录警告，不影响评测结果。
type GamePublisher interface {
	Publish(ctx context.Context, ev gamequeue.Event) error
}

type Pipeline struct {
	repo      *db.Repository
	store     storage.Store
	sandbox   sandbox.Sandbox
	publisher events.Publisher
	game      GamePublisher
}

func New(
	repo *db.Repository, store storage.Store, sb sandbox.Sandbox,
	publisher events.Publisher, game ...GamePublisher,
) *Pipeline {
	var gamePublisher GamePublisher
	if len(game) > 0 {
		gamePublisher = game[0]
	}
	return &Pipeline{
		repo: repo, store: store, sandbox: sb,
		publisher: publisher, game: gamePublisher,
	}
}

// Run judges one submission. User verdicts (including CE) are written and
// nil is returned. Infrastructure failures are returned so Asynq retries.
func (p *Pipeline) Run(ctx context.Context, submissionID int64) error {
	sub, prob, cases, err := p.repo.Claim(ctx, submissionID)
	if errors.Is(err, db.ErrSkip) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claim submission: %w", err)
	}

	if prob.Type == "objective" {
		p.publish(ctx, sub)
		accepted := false
		user := strings.TrimSpace(sub.Answer)
		for _, answer := range prob.Answers {
			if strings.EqualFold(user, strings.TrimSpace(answer)) {
				accepted = true
				break
			}
		}
		status := model.StatusWrongAnswer
		score := 0
		if accepted {
			status, score = model.StatusAccepted, 100
		}
		return p.finish(ctx, sub, db.FinishInput{
			Status: status, Score: score, Cases: []model.JudgeCaseResult{},
		})
	}

	if len(cases) == 0 {
		return errors.New("problem has no testcases")
	}
	if !sub.Language.Valid() {
		return fmt.Errorf("unsupported language %q", sub.Language)
	}
	p.publish(ctx, sub)

	settings := model.DefaultJudgeSettings()
	if loaded, err := p.repo.JudgeSettings(ctx); err != nil {
		slog.Warn("read judge settings failed; using defaults",
			"submission_id", sub.ID, "err", err)
	} else {
		settings = loaded
	}

	work, err := os.MkdirTemp("", "kapoj-judge-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()

	compiled, err := p.sandbox.Compile(ctx, sandbox.CompileRequest{
		Language: string(sub.Language), Source: []byte(sub.Code),
	})
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}
	if compiled.ArtifactDir != "" {
		defer p.sandbox.Cleanup(compiled.ArtifactDir)
	}
	if compiled.Failed {
		return p.finish(ctx, sub, db.FinishInput{
			Status:       model.StatusCompileError,
			ErrorMessage: truncate(compiled.ErrorOutput, maxCompileMessageSize),
			Cases:        []model.JudgeCaseResult{},
		})
	}

	checkerArtifact := ""
	if prob.Type == "special" {
		source, err := p.readStorage(ctx, prob.CheckerPath, maxCheckerBytes)
		if err != nil {
			return fmt.Errorf("read checker: %w", err)
		}
		checker, err := p.sandbox.Compile(ctx, sandbox.CompileRequest{
			Language: string(model.LanguageCPP), Source: source, Checker: true,
		})
		if err != nil {
			return fmt.Errorf("compile checker: %w", err)
		}
		if checker.ArtifactDir != "" {
			checkerArtifact = checker.ArtifactDir
			defer p.sandbox.Cleanup(checkerArtifact)
		}
		if checker.Failed {
			return errors.New("checker compile failed")
		}
	}

	inputs := make([]string, len(cases))
	batchCases := make([]sandbox.RunCase, len(cases))
	for i, tc := range cases {
		inputPath := filepath.Join(work, fmt.Sprintf("tc%d.in", tc.ID))
		if err := p.copyStorage(ctx, tc.InputFile, inputPath); err != nil {
			return fmt.Errorf("read input %s: %w", tc.InputFile, err)
		}
		inputs[i] = inputPath
		batchCases[i] = sandbox.RunCase{InputPath: inputPath}
	}

	var results []model.JudgeCaseResult
	var judgeErr error
	_, err = p.sandbox.RunBatch(ctx, sandbox.RunBatchRequest{
		Language: string(sub.Language), ArtifactDir: compiled.ArtifactDir, Cases: batchCases,
		TimeLimitMS: prob.TimeLimitMS, MemoryLimitMB: prob.MemoryLimitMB,
		OutputLimitBytes: outputLimitForCases(cases, settings),
		OnResult: func(index int, run sandbox.RunResult) bool {
			if judgeErr != nil {
				return false
			}
			result, err := p.judgeRun(ctx, work, prob, cases[index], inputs[index], run,
				checkerArtifact, settings.MaxOutputBytes)
			if err != nil {
				judgeErr = err
				return false
			}
			results = append(results, result)
			return result.Status == model.StatusAccepted
		},
	})
	if err != nil {
		return fmt.Errorf("run testcases: %w", err)
	}
	if judgeErr != nil {
		return judgeErr
	}
	status, timeMS, memoryKB, score := verdict.Aggregate(results)
	cpuMS := 0
	for _, result := range results {
		if result.CPUTimeMS > cpuMS {
			cpuMS = result.CPUTimeMS
		}
	}
	return p.finish(ctx, sub, db.FinishInput{
		Status: status, TimeUsedMS: timeMS, CPUUsedMS: cpuMS,
		MemoryUsedKB: memoryKB, Score: score, Cases: results,
	})
}

func (p *Pipeline) finish(ctx context.Context, sub *model.Submission, in db.FinishInput) error {
	if err := p.repo.Finish(ctx, sub.ID, in); err != nil {
		return err
	}
	p.publish(ctx, sub)
	if in.Status == model.StatusAccepted && sub.AgentID == nil {
		p.publishGameAccepted(ctx, sub)
	}
	return nil
}

func (p *Pipeline) publishGameAccepted(ctx context.Context, sub *model.Submission) {
	if p.game == nil {
		return
	}
	ev := gamequeue.Event{
		Type:         gamequeue.EventSubmissionAccepted,
		UserID:       sub.UserID,
		SubmissionID: sub.ID,
		OccurredAt:   time.Now().UTC(),
	}
	if err := p.game.Publish(ctx, ev); err != nil {
		slog.Warn("publish game accepted event failed",
			"submission_id", sub.ID, "user_id", sub.UserID, "err", err)
	}
}

func (p *Pipeline) publish(ctx context.Context, sub *model.Submission) {
	if sub.AgentID != nil {
		// Agent submissions are private to their owner; the human submission
		// SSE stream must never observe them.
		return
	}
	if p.publisher != nil {
		p.publisher.Publish(ctx, sub.ID)
	}
}

func (p *Pipeline) judgeRun(
	ctx context.Context, work string, prob *model.Problem, tc model.Testcase,
	inputPath string, run sandbox.RunResult, checkerArtifact string, maxAnswerBytes int64,
) (model.JudgeCaseResult, error) {
	if maxAnswerBytes <= 0 {
		maxAnswerBytes = model.DefaultJudgeSettings().MaxOutputBytes
	}
	answer, err := p.readStorage(ctx, tc.OutputFile, maxAnswerBytes)
	if err != nil {
		return model.JudgeCaseResult{}, fmt.Errorf("read answer %s: %w", tc.OutputFile, err)
	}
	result := model.JudgeCaseResult{
		TestcaseID: tc.ID, TimeMS: run.TimeMS, CPUTimeMS: run.CPUTimeMS,
		MemoryKB: run.MemoryKB, Score: tc.Score,
	}
	switch {
	case run.TimedOut || run.CPUTimeMS > prob.TimeLimitMS:
		result.Status, result.Message = model.StatusTimeLimitExceeded, "运行超时"
	case run.OOMKilled || (run.ExitCode != 0 && looksLikeOOM(run.Stderr)):
		result.Status, result.Message = model.StatusMemoryLimitExceeded, "内存超限"
	case run.OutputTruncated:
		result.Status, result.Message = model.StatusWrongAnswer, "输出超过限制"
	case run.ExitCode != 0:
		result.Status, result.Message = model.StatusRuntimeError, truncate(string(run.Stderr), maxMessageBytes)
	case prob.Type == "special":
		status, message, err := p.checkSpecial(ctx, work, tc, inputPath, answer, run.Output, checkerArtifact)
		if err != nil {
			return model.JudgeCaseResult{}, err
		}
		result.Status, result.Message = status, message
	case !verdict.Compare(run.Output, answer):
		result.Status = model.StatusWrongAnswer
	default:
		result.Status = model.StatusAccepted
	}
	return result, nil
}

func (p *Pipeline) checkSpecial(
	ctx context.Context, work string, tc model.Testcase, inputPath string, answer, output []byte, checkerArtifact string,
) (model.Status, string, error) {
	outputPath := filepath.Join(work, fmt.Sprintf("tc%d.user", tc.ID))
	answerPath := filepath.Join(work, fmt.Sprintf("tc%d.answer", tc.ID))
	if err := os.WriteFile(outputPath, output, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(answerPath, answer, 0o644); err != nil {
		return "", "", err
	}
	run, err := p.sandbox.Run(ctx, sandbox.RunRequest{
		Language:    string(model.LanguageCPP),
		ArtifactDir: checkerArtifact,
		InputPath:   inputPath,
		Checker:     &sandbox.CheckerFiles{AnswerPath: answerPath, OutputPath: outputPath},
		TimeLimitMS: checkerTimeLimitMS, MemoryLimitMB: checkerMemoryMB,
		OutputLimitBytes: checkerOutputBytes,
	})
	if err != nil {
		return "", "", fmt.Errorf("run checker for testcase %d: %w", tc.ID, err)
	}
	if run.TimedOut || run.CPUTimeMS > checkerTimeLimitMS {
		return "", "", fmt.Errorf("checker timed out on testcase %d", tc.ID)
	}
	if run.OOMKilled {
		return "", "", fmt.Errorf("checker out of memory on testcase %d", tc.ID)
	}
	if run.OutputTruncated {
		return "", "", fmt.Errorf("checker output limit exceeded on testcase %d", tc.ID)
	}
	switch run.ExitCode {
	case 0:
		return model.StatusAccepted, "", nil
	case 1:
		message := truncate(string(run.Output), maxMessageBytes)
		if message == "" {
			message = truncate(string(run.Stderr), maxMessageBytes)
		}
		return model.StatusWrongAnswer, message, nil
	default:
		return "", "", fmt.Errorf("checker failed with exit code %d on testcase %d", run.ExitCode, tc.ID)
	}
}

func looksLikeOOM(stderr []byte) bool {
	text := strings.ToLower(string(stderr))
	for _, marker := range []string{
		"outofmemoryerror", "memoryerror", "std::bad_alloc",
		"out of memory", "cannot allocate memory",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// outputLimitForCases returns the stdout cap for one batch. It starts at the
// configured floor and grows to cover the largest expected answer plus slack,
// capped at the configured maximum output. Without this, a correct program for
// a problem whose expected output is larger than the floor is reported as
// "输出超过限制" even when it matches exactly. A maximum below the floor (bad
// stored data) is clamped to the floor.
func outputLimitForCases(cases []model.Testcase, settings model.JudgeSettings) int {
	floor := settings.OutputFloorBytes
	if floor <= 0 {
		floor = model.DefaultJudgeSettings().OutputFloorBytes
	}
	maxOutput := settings.MaxOutputBytes
	if maxOutput < floor {
		maxOutput = floor
	}
	limit := floor
	for _, tc := range cases {
		if tc.OutputSize <= 0 {
			continue
		}
		need := tc.OutputSize + settings.OutputSlackBytes
		if need < floor {
			need = floor
		}
		if need > maxOutput {
			need = maxOutput
		}
		if need > limit {
			limit = need
		}
	}
	return int(limit)
}

func (p *Pipeline) readStorage(ctx context.Context, key string, limit int64) ([]byte, error) {
	if key == "" {
		return nil, storage.ErrInvalidKey
	}
	rc, err := p.store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file %s is larger than %d bytes", key, limit)
	}
	return data, nil
}

func (p *Pipeline) copyStorage(ctx context.Context, key, dest string) error {
	rc, err := p.store.Open(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, rc); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// MarkSystemError records the final failure after Asynq retries are exhausted.
func (p *Pipeline) MarkSystemError(ctx context.Context, submissionID int64, cause error) {
	message := "判题系统错误"
	if cause != nil {
		message = truncate(cause.Error(), maxMessageBytes)
	}
	if err := p.repo.MarkSystemError(ctx, submissionID, message); err != nil {
		slog.Error("mark submission system error failed", "submission_id", submissionID, "err", err)
		return
	}
	sub, err := p.repo.FindByID(ctx, submissionID)
	if err != nil {
		slog.Error("load submission after system error failed",
			"submission_id", submissionID, "err", err)
		return
	}
	p.publish(ctx, sub)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
