package problemapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

// DifficultyAssessor is the platform AI difficulty assessment dependency.
// *ai.Service implements it.
type DifficultyAssessor interface {
	PlatformModelConfigured(ctx context.Context) (bool, error)
	AssessDifficulty(ctx context.Context, in ai.DifficultyAssessmentInput) (*ai.DifficultyAssessment, error)
}

// AISettings is the AI feature toggle dependency. *ai.SettingsService
// implements it.
type AISettings interface {
	Enabled(ctx context.Context) (bool, error)
}

// defaultAITimeoutSeconds mirrors difficulty_settings' default. It is only a
// defensive fallback for an unset timeout.
const defaultAITimeoutSeconds = 15

// createDifficultyDecision builds the difficulty metadata for POST
// /api/problems. The legacy path (no settings service wired) keeps the old
// behavior: the request score is stored as a manual value. In the full path an
// automatic problem starts from difficulty_settings.default_score and is
// overwritten by a successful AI assessment; any AI failure keeps the default
// so creation always succeeds.
func (h *handler) createDifficultyDecision(ctx context.Context, in problem.Input, actorID int64) (problem.DifficultyDecision, error) {
	auto := in.DifficultyAuto == nil || *in.DifficultyAuto
	if h.difficulty == nil {
		// Transitional wiring (before Task 11): preserve the pre-AI contract.
		return manualDifficultyDecision(in.Difficulty, true, actorID), nil
	}
	if !auto {
		return manualDifficultyDecision(in.Difficulty, false, actorID), nil
	}

	settings, err := h.difficulty.Get(ctx)
	if err != nil {
		return problem.DifficultyDecision{}, err
	}
	decision := problem.DifficultyDecision{
		Score:       settings.DefaultScore,
		Prior:       settings.DefaultScore,
		PriorSource: problem.DifficultyPriorSourceDefault,
		Source:      problem.DifficultySourceDefault,
		Auto:        true,
		ActorID:     actorID,
	}
	if assessment, ok := h.tryAssessDifficulty(ctx, settings, assessmentInputFromInput(in)); ok {
		decision.Score = assessment.Score
		decision.Prior = assessment.Score
		decision.PriorSource = problem.DifficultyPriorSourceAI
		decision.Source = problem.DifficultySourceAI
		decision.Reason = assessment.Reason
		decision.Confidence = assessment.Confidence
	}
	return normalizeDecisionPrior(decision), nil
}

func manualDifficultyDecision(score int, auto bool, actorID int64) problem.DifficultyDecision {
	return normalizeDecisionPrior(problem.DifficultyDecision{
		Score: score, Prior: score,
		PriorSource: problem.DifficultyPriorSourceManual,
		Source:      problem.DifficultySourceManual,
		Auto:        auto,
		ActorID:     actorID,
	})
}

// normalizeDecisionPrior guarantees the history prior stays on the 1–100
// scale even if a dependency returns an out-of-range value.
func normalizeDecisionPrior(d problem.DifficultyDecision) problem.DifficultyDecision {
	if d.Prior < 1 || d.Prior > 100 {
		d.Prior = d.Score
	}
	return d
}

// tryAssessDifficulty runs the configured platform assessment with the
// difficulty timeout. It is best-effort: every failure is logged and reported
// as not assessed so callers can fall back to the configured default.
func (h *handler) tryAssessDifficulty(
	ctx context.Context,
	settings *difficulty.SettingsView,
	in ai.DifficultyAssessmentInput,
) (*ai.DifficultyAssessment, bool) {
	if h.ai == nil || h.aiSettings == nil || !settings.AIAssessEnabled {
		return nil, false
	}
	enabled, err := h.aiSettings.Enabled(ctx)
	if err != nil {
		slog.Warn("problem: AI settings unavailable; using default difficulty", "err", err)
		return nil, false
	}
	if !enabled {
		return nil, false
	}
	configured, err := h.ai.PlatformModelConfigured(ctx)
	if err != nil {
		slog.Warn("problem: AI platform model check failed; using default difficulty", "err", err)
		return nil, false
	}
	if !configured {
		return nil, false
	}
	assessment, err := h.assessWithTimeout(ctx, settings, in)
	if err != nil {
		slog.Warn("problem: AI difficulty assessment failed; using default difficulty", "err", err)
		return nil, false
	}
	return assessment, true
}

func (h *handler) assessWithTimeout(
	ctx context.Context,
	settings *difficulty.SettingsView,
	in ai.DifficultyAssessmentInput,
) (*ai.DifficultyAssessment, error) {
	timeout := settings.AITimeoutSeconds
	if timeout <= 0 {
		timeout = defaultAITimeoutSeconds
	}
	actx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	assessment, err := h.ai.AssessDifficulty(actx, in)
	if err != nil {
		return nil, err
	}
	if assessment == nil {
		return nil, httpx.ErrBadGateway
	}
	return assessment, nil
}

// assessProblemDifficulty implements POST /problems/:id/difficulty/assess.
// It overwrites the score/prior/source with a fresh AI assessment but never
// changes difficulty_auto, so a manual lock survives a reassessment.
func (h *handler) assessProblemDifficulty(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	p, ok := h.problemForManage(c, id)
	if !ok {
		return
	}
	settings, err := h.difficultyAssessmentSettings(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	assessment, err := h.assessWithTimeout(c.Request.Context(), settings, assessmentInputFromProblem(p))
	if err != nil {
		h.failAssessment(c, err)
		return
	}
	claims, _ := middleware.CurrentClaims(c)
	updated, err := h.svc.UpdateDifficultyMeta(c.Request.Context(), id, normalizeDecisionPrior(problem.DifficultyDecision{
		Score:       assessment.Score,
		Prior:       assessment.Score,
		PriorSource: problem.DifficultyPriorSourceAI,
		Source:      problem.DifficultySourceAI,
		Auto:        p.DifficultyAuto,
		Reason:      assessment.Reason,
		Confidence:  assessment.Confidence,
		ActorID:     claims.UserID,
	}))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d := problem.ToDetail(updated, true, true, true, nil)
	d.CanEdit = true
	httpx.OK(c, d)
}

// difficultyAssessmentSettings enforces the "AI unavailable → 40001" contract
// for explicit reassessments before any model call is made.
func (h *handler) difficultyAssessmentSettings(ctx context.Context) (*difficulty.SettingsView, error) {
	if h.difficulty == nil || h.ai == nil || h.aiSettings == nil {
		return nil, httpx.ErrValidation("AI 难度评估未配置")
	}
	settings, err := h.difficulty.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.AIAssessEnabled {
		return nil, httpx.ErrValidation("AI 难度评估已关闭")
	}
	enabled, err := h.aiSettings.Enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, httpx.ErrValidation("AI 功能已关闭")
	}
	configured, err := h.ai.PlatformModelConfigured(ctx)
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, httpx.ErrValidation("尚未配置平台 AI 模型")
	}
	return settings, nil
}

// failAssessment keeps configuration errors (40001/40002) intact and turns
// unexpected upstream or parse failures into 50200.
func (h *handler) failAssessment(c *gin.Context, err error) {
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		httpx.Fail(c, err)
		return
	}
	slog.Warn("problem: AI difficulty reassessment failed", "err", err)
	httpx.Fail(c, httpx.ErrBadGateway)
}

// DifficultyHistoryItem is the manager-facing history row. It mirrors the
// stored columns without exposing internal row ids.
type DifficultyHistoryItem struct {
	OldScore         int       `json:"oldScore"`
	NewScore         int       `json:"newScore"`
	Prior            int       `json:"prior"`
	Source           string    `json:"source"`
	SampleUsers      int       `json:"sampleUsers"`
	AcceptedUsers    int       `json:"acceptedUsers"`
	PassRate         float64   `json:"passRate"`
	SmoothedPassRate float64   `json:"smoothedPassRate"`
	TargetScore      int       `json:"targetScore"`
	Confidence       float64   `json:"confidence"`
	Reason           string    `json:"reason"`
	RunID            *int64    `json:"runId"`
	CreatedBy        int64     `json:"createdBy"`
	CreatedAt        time.Time `json:"createdAt"`
}

func toDifficultyHistoryItem(h problem.DifficultyHistory) DifficultyHistoryItem {
	return DifficultyHistoryItem{
		OldScore:         h.OldScore,
		NewScore:         h.NewScore,
		Prior:            h.Prior,
		Source:           h.Source,
		SampleUsers:      h.SampleUsers,
		AcceptedUsers:    h.AcceptedUsers,
		PassRate:         h.PassRate,
		SmoothedPassRate: h.SmoothedPassRate,
		TargetScore:      h.TargetScore,
		Confidence:       h.Confidence,
		Reason:           h.Reason,
		RunID:            h.RunID,
		CreatedBy:        h.CreatedBy,
		CreatedAt:        h.CreatedAt.UTC(),
	}
}

// difficultyHistory implements GET /problems/:id/difficulty/history.
func (h *handler) difficultyHistory(c *gin.Context) {
	id, ok := pathID(c, "id")
	if !ok {
		return
	}
	if _, ok := h.problemForManage(c, id); !ok {
		return
	}
	page, size := httpx.ParsePage(c.Request.URL.RawQuery)
	history, total, err := h.svc.DifficultyHistory(c.Request.Context(), id, page, size)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items := make([]DifficultyHistoryItem, len(history))
	for i := range history {
		items[i] = toDifficultyHistoryItem(history[i])
	}
	httpx.OK(c, httpx.Page[DifficultyHistoryItem]{Total: total, Items: items})
}

func assessmentInputFromInput(in problem.Input) ai.DifficultyAssessmentInput {
	return assessmentInput(in.Title, string(in.Type), in.Tags, in.TimeLimitMS, in.MemoryLimitMB,
		in.InputFormatMD, in.OutputFormatMD, in.StatementMD, in.Samples)
}

func assessmentInputFromProblem(p *problem.Problem) ai.DifficultyAssessmentInput {
	return assessmentInput(p.Title, string(p.Type), p.Tags, p.TimeLimitMS, p.MemoryLimitMB,
		p.InputFormatMD, p.OutputFormatMD, p.StatementMD, p.Samples)
}

func assessmentInput(
	title, typ string,
	tags []string,
	timeLimitMS, memoryLimitMB int,
	inputFormatMD, outputFormatMD, statementMD string,
	samples []problem.Sample,
) ai.DifficultyAssessmentInput {
	out := ai.DifficultyAssessmentInput{
		Title: title, Type: typ, Tags: tags,
		TimeLimitMS: timeLimitMS, MemoryLimitMB: memoryLimitMB,
		InputFormatMD: inputFormatMD, OutputFormatMD: outputFormatMD,
		StatementMD: statementMD,
	}
	if len(samples) > 0 {
		out.Samples = make([]ai.DifficultySample, len(samples))
		for i := range samples {
			out.Samples[i] = ai.DifficultySample{Input: samples[i].Input, Output: samples[i].Output}
		}
	}
	return out
}
