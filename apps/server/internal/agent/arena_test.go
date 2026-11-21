package agent_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

type seedProblemOpts struct {
	Slug          string
	Title         string
	Type          problem.Type
	Hidden        bool
	Difficulty    int
	CheckerPath   string
	Testcases     int
	Samples       []problem.Sample
	Tags          []string
	StatementMD   string
	TimeLimitMS   int
	MemoryLimitMB int
	Source        string
}

func (f *agentFixture) seedProblem(o seedProblemOpts) *problem.Problem {
	f.t.Helper()
	require.NotEmpty(f.t, o.Slug, "test problem needs an explicit slug")
	typ := o.Type
	if typ == "" {
		typ = problem.TypeTraditional
	}
	difficulty := o.Difficulty
	if difficulty == 0 {
		difficulty = 50
	}
	timeLimit := o.TimeLimitMS
	if timeLimit == 0 {
		timeLimit = 1000
	}
	memoryLimit := o.MemoryLimitMB
	if memoryLimit == 0 {
		memoryLimit = 256
	}
	samples := o.Samples
	if samples == nil {
		samples = []problem.Sample{{Input: "1", Output: "1"}}
	}
	tags := o.Tags
	if tags == nil {
		tags = []string{"test"}
	}
	statement := o.StatementMD
	if statement == "" {
		statement = "statement " + o.Slug
	}
	source := o.Source
	if source == "" {
		source = "test-source"
	}
	p := &problem.Problem{
		Title:          o.Title,
		Slug:           o.Slug,
		StatementMD:    statement,
		InputFormatMD:  "input " + o.Slug,
		OutputFormatMD: "output " + o.Slug,
		Samples:        dbtypes.List[problem.Sample](samples),
		Tags:           dbtypes.List[string](tags),
		Type:           typ,
		Difficulty:     difficulty,
		TimeLimitMS:    timeLimit,
		MemoryLimitMB:  memoryLimit,
		CheckerPath:    o.CheckerPath,
		IsPublic:       !o.Hidden,
		Source:         source,
		CreatedAt:      f.clock.now,
		UpdatedAt:      f.clock.now,
	}
	if p.Title == "" {
		p.Title = "Problem " + o.Slug
	}
	require.NoError(f.t, f.db.Create(p).Error)
	for i := 0; i < o.Testcases; i++ {
		tc := &problem.Testcase{
			ProblemID:  p.ID,
			InputFile:  fmt.Sprintf("%d.in", i+1),
			OutputFile: fmt.Sprintf("%d.out", i+1),
			InputSize:  1, OutputSize: 1, Score: 100, SortOrder: i + 1,
		}
		require.NoError(f.t, f.db.Create(tc).Error)
	}
	return p
}

func (f *agentFixture) lockProblem(p *problem.Problem, end time.Time) {
	f.t.Helper()
	c := &contest.Contest{
		Title:     "Lock " + p.Slug,
		StartTime: end.Add(-2 * time.Hour),
		EndTime:   end,
		Mode:      contest.ModeIOI,
		IsPublic:  true,
		CreatedAt: f.clock.now,
		UpdatedAt: f.clock.now,
	}
	require.NoError(f.t, f.db.Create(c).Error)
	require.NoError(f.t, f.db.Create(&contest.ContestProblem{
		ContestID: c.ID, ProblemID: p.ID, DisplayID: "A", SortOrder: 1, Score: 100,
	}).Error)
}

func (f *agentFixture) seedAttempt(agentID, problemID int64, status agent.AttemptStatus) *agent.AgentProblemAttempt {
	f.t.Helper()
	a := &agent.AgentProblemAttempt{
		AgentID: agentID, ProblemID: problemID, AttemptsUsed: 3, Status: status,
		ClaimedAt: f.clock.now.Add(-time.Hour), DeadlineAt: f.clock.now.Add(-time.Minute),
		CreatedAt: f.clock.now.Add(-time.Hour), UpdatedAt: f.clock.now.Add(-time.Minute),
	}
	require.NoError(f.t, f.db.Create(a).Error)
	return a
}

// resetArena clears problem/attempt data while keeping users and Agents.
func (f *agentFixture) resetArena() {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(`
		TRUNCATE problems, problem_testcases, contests, contest_problems,
		         agent_problem_attempts, agent_rating_history, submissions
		RESTART IDENTITY CASCADE`).Error)
}

type fakeLimiter struct {
	allow  bool
	calls  int
	action string
}

func (l *fakeLimiter) Allow(_ context.Context, _ int64, action string) (bool, error) {
	l.calls++
	l.action = action
	return l.allow, nil
}

func newAgentFixtureWithLimiter(t *testing.T, limiter agent.Limiter) *agentFixture {
	t.Helper()
	return newAgentFixtureWithDeps(t, nil, limiter)
}

func newAgentFixtureWithQueue(t *testing.T, queue submission.Enqueuer) *agentFixture {
	t.Helper()
	return newAgentFixtureWithDeps(t, queue, nil)
}

func TestAgentClaimReturnsActiveAttempt(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("claim-reuse", "active")
	view := f.create(owner, "Reuse Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "reuse-problem", Testcases: 1})

	first, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, p.ID, first.Attempt.ProblemID)
	assert.Equal(t, p.Title, first.Attempt.ProblemTitle)
	assert.Equal(t, p.Slug, first.Attempt.ProblemSlug)
	assert.Equal(t, p.Difficulty, first.Attempt.Difficulty)
	assert.Equal(t, agent.AttemptActive, first.Attempt.Status)
	assert.Zero(t, first.Attempt.AttemptsUsed)
	assert.Equal(t, agent.MaxAttempts, first.Attempt.AttemptsRemaining)
	assert.True(t, first.Attempt.ClaimedAt.Equal(f.clock.now))
	assert.True(t, first.Attempt.DeadlineAt.Equal(f.clock.now.Add(5*time.Minute)))

	// A later claim while the attempt is still live returns the same row and
	// does not reset the deadline or use count.
	require.NoError(t, f.db.Model(&agent.AgentProblemAttempt{}).
		Where("id = ?", first.Attempt.ID).Update("attempts_used", 7).Error)
	f.clock.advance(2 * time.Minute)

	second, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.Attempt.ID, second.Attempt.ID)
	assert.True(t, second.Attempt.ClaimedAt.Equal(first.Attempt.ClaimedAt))
	assert.True(t, second.Attempt.DeadlineAt.Equal(first.Attempt.DeadlineAt))
	assert.Equal(t, 7, second.Attempt.AttemptsUsed)
	assert.Equal(t, agent.MaxAttempts-7, second.Attempt.AttemptsRemaining)

	var attempts int64
	require.NoError(t, f.db.Model(&agent.AgentProblemAttempt{}).Where("agent_id = ?", view.ID).Count(&attempts).Error)
	assert.EqualValues(t, 1, attempts)
}

func TestAgentClaimExcludesUnavailableProblems(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("claim-pool", "active")
	view := f.create(owner, "Pool Agent")

	claimNoProblem := func(t *testing.T) {
		t.Helper()
		_, err := f.svc.Claim(ctx, view.ID)
		requireAppError(t, err, 404, 40401)
	}

	t.Run("hidden", func(t *testing.T) {
		f.resetArena()
		f.seedProblem(seedProblemOpts{Slug: "hidden-global", Hidden: true, Testcases: 1})
		claimNoProblem(t)
	})

	t.Run("locked contest", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "locked-problem", Testcases: 1})
		f.lockProblem(p, f.clock.now.Add(time.Hour))
		claimNoProblem(t)
	})

	t.Run("interactive", func(t *testing.T) {
		f.resetArena()
		f.seedProblem(seedProblemOpts{Slug: "interactive-problem", Type: problem.TypeInteractive, Testcases: 1})
		claimNoProblem(t)
	})

	t.Run("traditional without testcases", func(t *testing.T) {
		f.resetArena()
		f.seedProblem(seedProblemOpts{Slug: "no-testcase-problem", Testcases: 0})
		claimNoProblem(t)
	})

	t.Run("special without checker", func(t *testing.T) {
		f.resetArena()
		f.seedProblem(seedProblemOpts{Slug: "special-no-checker", Type: problem.TypeSpecial, Testcases: 1})
		claimNoProblem(t)
	})

	t.Run("solved by this agent", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "already-solved", Testcases: 1})
		f.seedAttempt(view.ID, p.ID, agent.AttemptSolved)
		claimNoProblem(t)
	})

	t.Run("exhausted by this agent", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "already-exhausted", Testcases: 1})
		f.seedAttempt(view.ID, p.ID, agent.AttemptExhausted)
		claimNoProblem(t)
	})

	t.Run("objective needs no testcases", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "objective-problem", Type: problem.TypeObjective, Testcases: 0})
		result, err := f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, result.Attempt.ProblemID)
		assert.Equal(t, problem.TypeObjective, result.Problem.Type)
	})

	t.Run("special with checker is eligible", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{
			Slug: "special-checker", Type: problem.TypeSpecial,
			CheckerPath: "problems/1/checker.cpp", Testcases: 1,
		})
		result, err := f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, result.Attempt.ProblemID)
	})

	t.Run("abandoned attempt is eligible again", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "abandoned-again", Testcases: 1})
		existing := f.seedAttempt(view.ID, p.ID, agent.AttemptAbandoned)
		result, err := f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)
		assert.Equal(t, existing.ID, result.Attempt.ID)
		assert.Equal(t, p.ID, result.Attempt.ProblemID)
		assert.Equal(t, 3, result.Attempt.AttemptsUsed, "attempts_used survives re-claiming")
		assert.Equal(t, agent.AttemptActive, result.Attempt.Status)
	})

	t.Run("ended contest does not lock", func(t *testing.T) {
		f.resetArena()
		p := f.seedProblem(seedProblemOpts{Slug: "ended-contest", Testcases: 1})
		f.lockProblem(p, f.clock.now.Add(-time.Minute))
		result, err := f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, result.Attempt.ProblemID)
	})
}

func TestAgentClaimExpiresStaleAttemptWithTimeoutPenalty(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("claim-timeout", "active")
	view := f.create(owner, "Timeout Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "timeout-problem", Difficulty: 50, Testcases: 1})

	first, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)

	f.clock.advance(6 * time.Minute)
	second, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	assert.Equal(t, first.Attempt.ID, second.Attempt.ID, "expired attempts are re-used")
	assert.True(t, second.Attempt.ClaimedAt.Equal(f.clock.now))
	assert.True(t, second.Attempt.DeadlineAt.Equal(f.clock.now.Add(5*time.Minute)))

	wantDelta := agent.FailureDelta(1500, p.Difficulty)
	var stored agent.Agent
	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, 1500+wantDelta, stored.Rating)

	var history []agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ? AND kind = ?", view.ID, agent.RatingTimeout).
		Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, p.Difficulty, history[0].Difficulty)
	assert.Equal(t, 1500, history[0].RatingBefore)
	assert.Equal(t, 1500+wantDelta, history[0].RatingAfter)
	assert.Equal(t, wantDelta, history[0].Delta)
	assert.Nil(t, history[0].SubmissionID)

	// A second timeout on the same problem does not deduct again.
	f.clock.advance(6 * time.Minute)
	_, err = f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	require.NoError(t, f.db.Where("agent_id = ? AND kind IN ?", view.ID,
		[]agent.RatingKind{agent.RatingAbandon, agent.RatingTimeout, agent.RatingExhausted}).
		Find(&history).Error)
	require.Len(t, history, 1)
	require.NoError(t, f.db.First(&stored, view.ID).Error)
	assert.Equal(t, 1500+wantDelta, stored.Rating)

	t.Run("floor", func(t *testing.T) {
		f.resetArena()
		require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", view.ID).Update("rating", 0).Error)
		f.seedProblem(seedProblemOpts{Slug: "timeout-floor", Difficulty: 100, Testcases: 1})
		_, err := f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)
		f.clock.advance(6 * time.Minute)
		_, err = f.svc.Claim(ctx, view.ID)
		require.NoError(t, err)

		require.NoError(t, f.db.Where("agent_id = ?", view.ID).Find(&history).Error)
		require.Len(t, history, 1)
		assert.Zero(t, history[0].RatingBefore)
		assert.Zero(t, history[0].RatingAfter)
		assert.Zero(t, history[0].Delta)
		require.NoError(t, f.db.First(&stored, view.ID).Error)
		assert.Zero(t, stored.Rating)
	})
}

func TestAgentClaimWaitsForPendingSubmission(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("claim-pending", "active")
	view := f.create(owner, "Pending Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "pending-problem", Testcases: 1})

	claimed, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	agentID := view.ID
	attemptID := claimed.Attempt.ID
	pending := &submission.Submission{
		UserID: owner, ProblemID: p.ID, AgentID: &agentID, AgentAttemptID: &attemptID,
		Language: submission.LanguageCPP, Code: "int main(){}",
		Status: submission.StatusPending, CreatedAt: f.clock.now,
	}
	require.NoError(t, f.db.Create(pending).Error)

	f.clock.advance(6 * time.Minute)
	_, err = f.svc.Claim(ctx, view.ID)
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "当前题目正在评测，请等待结果")

	var stored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&stored, attemptID).Error)
	assert.Equal(t, agent.AttemptActive, stored.Status)
	assert.True(t, stored.DeadlineAt.Equal(claimed.Attempt.DeadlineAt))
	assert.Empty(t, failureHistory(t, f, view.ID))

	// A terminal but unsettled verdict is still in flight, so the stale
	// attempt cannot expire before the settlement worker consumes it.
	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", pending.ID).
		Update("status", submission.StatusAccepted).Error)
	_, err = f.svc.Claim(ctx, view.ID)
	appErr = requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "当前题目正在评测，请等待结果")
	require.NoError(t, f.db.First(&stored, attemptID).Error)
	assert.Equal(t, agent.AttemptActive, stored.Status)
	assert.Empty(t, failureHistory(t, f, view.ID))

	// Once settlement marks the verdict consumed, the stale attempt may expire
	// and the Agent can claim again.
	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", pending.ID).
		Update("agent_settled", true).Error)
	_, err = f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	history := failureHistory(t, f, view.ID)
	require.Len(t, history, 1)
	assert.Equal(t, agent.RatingTimeout, history[0].Kind)
}

func failureHistory(t *testing.T, f *agentFixture, agentID int64) []agent.AgentRatingHistory {
	t.Helper()
	var rows []agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ? AND kind IN ?", agentID,
		[]agent.RatingKind{agent.RatingAbandon, agent.RatingTimeout, agent.RatingExhausted}).
		Order("id").Find(&rows).Error)
	return rows
}

func TestAgentClaimRateLimited(t *testing.T) {
	limiter := &fakeLimiter{allow: false}
	f := newAgentFixtureWithLimiter(t, limiter)
	ctx := context.Background()
	owner := f.user("claim-limited", "active")
	view := f.create(owner, "Limited Agent")
	f.seedProblem(seedProblemOpts{Slug: "limited-problem", Testcases: 1})

	_, err := f.svc.Claim(ctx, view.ID)
	requireAppError(t, err, 429, 42906)
	assert.Equal(t, 1, limiter.calls)
	assert.Equal(t, "claim", limiter.action)

	var attempts int64
	require.NoError(t, f.db.Model(&agent.AgentProblemAttempt{}).Where("agent_id = ?", view.ID).Count(&attempts).Error)
	assert.Zero(t, attempts)
}

// seedLiveAttempt inserts an active attempt that has not yet reached its
// deadline with an explicit used count.
func (f *agentFixture) seedLiveAttempt(agentID, problemID int64, used int) *agent.AgentProblemAttempt {
	f.t.Helper()
	a := &agent.AgentProblemAttempt{
		AgentID: agentID, ProblemID: problemID, AttemptsUsed: used, Status: agent.AttemptActive,
		ClaimedAt: f.clock.now.Add(-time.Minute), DeadlineAt: f.clock.now.Add(time.Hour),
		CreatedAt: f.clock.now.Add(-time.Minute), UpdatedAt: f.clock.now.Add(-time.Minute),
	}
	require.NoError(f.t, f.db.Create(a).Error)
	return a
}

func (f *agentFixture) agentUserID(agentID int64) int64 {
	f.t.Helper()
	var a agent.Agent
	require.NoError(f.t, f.db.Select("id", "user_id").First(&a, agentID).Error)
	return a.UserID
}

// settleSubmission simulates the Task 6 settlement worker for tests that only
// need the attempt to become submit-able again.
func (f *agentFixture) settleSubmission(submissionID int64, status submission.Status) {
	f.t.Helper()
	require.NoError(f.t, f.db.Model(&submission.Submission{}).Where("id = ?", submissionID).
		Updates(map[string]any{
			"status":        status,
			"agent_settled": true,
			"judged_at":     f.clock.now,
		}).Error)
}

func (f *agentFixture) submissionCount(attemptID int64) int64 {
	f.t.Helper()
	var count int64
	require.NoError(f.t, f.db.Model(&submission.Submission{}).
		Where("agent_attempt_id = ?", attemptID).Count(&count).Error)
	return count
}

func TestAgentSubmitLimit(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("submit-limit", "active")
	view := f.create(owner, "Submit Limit Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "submit-limit-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	for i := 1; i <= agent.MaxAttempts; i++ {
		result, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
			Language: submission.LanguageCPP,
			Code:     fmt.Sprintf("int main(){return %d;}", i),
		})
		require.NoError(t, err, "submission %d", i)
		require.NotNil(t, result)
		assert.NotZero(t, result.SubmissionID)
		assert.Equal(t, submission.StatusPending, result.Status)
		assert.Equal(t, i, result.AttemptsUsed)
		assert.Equal(t, agent.MaxAttempts-i, result.AttemptsRemaining)
		assert.False(t, result.Exhausted, "exhaustion is settled by the Task 6 worker")
		if i < agent.MaxAttempts {
			f.settleSubmission(result.SubmissionID, submission.StatusWrongAnswer)
		}
	}

	// The 20th submission is allowed; the 21st is rejected by the hard limit
	// even though the latest submission is still unsettled.
	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "次数")

	var stored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&stored, attempt.ID).Error)
	assert.Equal(t, agent.MaxAttempts, stored.AttemptsUsed)
	require.NotNil(t, stored.LastSubmissionAt)
	assert.True(t, stored.LastSubmissionAt.Equal(f.clock.now))
	assert.Equal(t, agent.AttemptActive, stored.Status)

	var storedAgent agent.Agent
	require.NoError(t, f.db.First(&storedAgent, view.ID).Error)
	assert.Equal(t, 1, storedAgent.AttemptedCount, "attempted_count counts the problem once")
	assert.Equal(t, 1500, storedAgent.Rating)

	assert.EqualValues(t, agent.MaxAttempts, f.submissionCount(attempt.ID))
	var storedProblem problem.Problem
	require.NoError(t, f.db.First(&storedProblem, p.ID).Error)
	assert.Zero(t, storedProblem.SubmitCount, "agent submissions do not touch human problem stats")
	assert.Zero(t, storedProblem.AcceptCount)

	var rows []submission.Submission
	require.NoError(t, f.db.Where("agent_attempt_id = ?", attempt.ID).Order("id").Find(&rows).Error)
	require.Len(t, rows, agent.MaxAttempts)
	for _, row := range rows {
		assert.Equal(t, owner, row.UserID)
		require.NotNil(t, row.AgentID)
		assert.Equal(t, view.ID, *row.AgentID)
		require.NotNil(t, row.AgentAttemptID)
		assert.Equal(t, attempt.ID, *row.AgentAttemptID)
		assert.Nil(t, row.ContestID)
		assert.Equal(t, submission.LanguageCPP, row.Language)
		assert.Equal(t, len(row.Code), row.CodeLength)
		assert.Empty(t, row.JudgeResult)
	}
}

func TestAgentSubmitObjectiveUsesAnswerLength(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("submit-objective", "active")
	view := f.create(owner, "Objective Agent")
	p := f.seedProblem(seedProblemOpts{
		Slug: "submit-objective-problem", Type: problem.TypeObjective, Testcases: 0,
	})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	result, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "ignored", Answer: " 42 ",
	})
	require.NoError(t, err)

	var row submission.Submission
	require.NoError(t, f.db.First(&row, result.SubmissionID).Error)
	assert.Equal(t, submission.LanguageAnswer, row.Language)
	assert.Equal(t, "42", row.Answer)
	assert.Empty(t, row.Code)
	assert.Equal(t, 2, row.CodeLength)
	assert.Empty(t, row.JudgeResult)
	assert.False(t, row.AgentSettled)
}

func TestAgentSubmitRequiresActiveAttemptAndNoPending(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	p := f.seedProblem(seedProblemOpts{Slug: "submit-guard-problem", Difficulty: 50, Testcases: 1})

	newAgent := func(username, name string) int64 {
		owner := f.user(username, "active")
		return f.create(owner, name).ID
	}
	code := agent.SubmissionInput{Language: submission.LanguageCPP, Code: "int main(){}"}

	t.Run("foreign and missing attempts are not found", func(t *testing.T) {
		a := newAgent("submit-foreign-a", "Foreign A")
		b := newAgent("submit-foreign-b", "Foreign B")
		attempt := f.seedLiveAttempt(a, p.ID, 3)

		_, err := f.svc.Submit(ctx, b, attempt.ID, code)
		requireAppError(t, err, 404, 40400)

		_, err = f.svc.Submit(ctx, a, attempt.ID+1000, code)
		requireAppError(t, err, 404, 40400)

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, 3, stored.AttemptsUsed)
		assert.Zero(t, f.submissionCount(attempt.ID))
	})

	t.Run("expired attempt rejected without consuming or expiring", func(t *testing.T) {
		a := newAgent("submit-expired", "Expired Agent")
		attempt := f.seedAttempt(a, p.ID, agent.AttemptActive)

		_, err := f.svc.Submit(ctx, a, attempt.ID, code)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Contains(t, appErr.Message, "超时")

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptActive, stored.Status)
		assert.Equal(t, 3, stored.AttemptsUsed)
		assert.Zero(t, f.submissionCount(attempt.ID))
	})

	t.Run("non-active attempt rejected", func(t *testing.T) {
		a := newAgent("submit-inactive", "Inactive Agent")
		attempt := f.seedAttempt(a, p.ID, agent.AttemptSolved)

		_, err := f.svc.Submit(ctx, a, attempt.ID, code)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Contains(t, appErr.Message, "不可提交")
		assert.Zero(t, f.submissionCount(attempt.ID))
	})

	t.Run("attempts_used at 20 rejected", func(t *testing.T) {
		a := newAgent("submit-limit-guard", "Limit Guard Agent")
		attempt := f.seedLiveAttempt(a, p.ID, agent.MaxAttempts)

		_, err := f.svc.Submit(ctx, a, attempt.ID, code)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Contains(t, appErr.Message, "次数")
		assert.Zero(t, f.submissionCount(attempt.ID))
	})

	t.Run("unsettled submission blocks until the worker settles", func(t *testing.T) {
		a := newAgent("submit-inflight", "In Flight Agent")
		attempt := f.seedLiveAttempt(a, p.ID, 3)

		agentID := a
		attemptID := attempt.ID
		row := &submission.Submission{
			UserID: f.agentUserID(a), ProblemID: p.ID, AgentID: &agentID, AgentAttemptID: &attemptID,
			Language: submission.LanguageCPP, Code: "int main(){}",
			Status: submission.StatusPending, AgentSettled: false, CreatedAt: f.clock.now,
		}
		require.NoError(t, f.db.Create(row).Error)

		_, err := f.svc.Submit(ctx, a, attempt.ID, code)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Equal(t, "当前题目正在评测，请等待结果", appErr.Message)

		// A terminal verdict that the worker has not settled yet is still
		// treated as in flight so it cannot be followed by another submit.
		require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", row.ID).
			Update("status", submission.StatusAccepted).Error)
		_, err = f.svc.Submit(ctx, a, attempt.ID, code)
		appErr = requireAppError(t, err, 400, 40001)
		assert.Equal(t, "当前题目正在评测，请等待结果", appErr.Message)
		assert.EqualValues(t, 1, f.submissionCount(attempt.ID))

		require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", row.ID).
			Update("agent_settled", true).Error)
		result, err := f.svc.Submit(ctx, a, attempt.ID, code)
		require.NoError(t, err)
		assert.Equal(t, 4, result.AttemptsUsed)
		assert.Equal(t, agent.MaxAttempts-4, result.AttemptsRemaining)
		assert.EqualValues(t, 2, f.submissionCount(attempt.ID))

		var storedAgent agent.Agent
		require.NoError(t, f.db.First(&storedAgent, a).Error)
		assert.Zero(t, storedAgent.AttemptedCount, "used count was already non-zero")
	})

	t.Run("problem availability is rechecked", func(t *testing.T) {
		a := newAgent("submit-recheck-hidden", "Recheck Agent")
		hidden := f.seedProblem(seedProblemOpts{Slug: "submit-recheck-hidden-problem", Hidden: true, Testcases: 1})
		attempt := f.seedLiveAttempt(a, hidden.ID, 0)

		_, err := f.svc.Submit(ctx, a, attempt.ID, code)
		requireAppError(t, err, 404, 40400)
		assert.Zero(t, f.submissionCount(attempt.ID))

		b := newAgent("submit-recheck-locked", "Recheck Locked Agent")
		locked := f.seedProblem(seedProblemOpts{Slug: "submit-recheck-locked-problem", Testcases: 1})
		lockedAttempt := f.seedLiveAttempt(b, locked.ID, 0)
		f.lockProblem(locked, f.clock.now.Add(time.Hour))

		_, err = f.svc.Submit(ctx, b, lockedAttempt.ID, code)
		requireAppError(t, err, 404, 40400)
		assert.Zero(t, f.submissionCount(lockedAttempt.ID))

		c := newAgent("submit-recheck-no-tests", "Recheck No Tests Agent")
		untestable := f.seedProblem(seedProblemOpts{Slug: "submit-recheck-no-tests-problem", Testcases: 0})
		untestableAttempt := f.seedLiveAttempt(c, untestable.ID, 0)

		_, err = f.svc.Submit(ctx, c, untestableAttempt.ID, code)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Contains(t, appErr.Message, "测试点")
		assert.Zero(t, f.submissionCount(untestableAttempt.ID))
	})

	t.Run("empty code is rejected", func(t *testing.T) {
		a := newAgent("submit-empty-code", "Empty Code Agent")
		attempt := f.seedLiveAttempt(a, p.ID, 0)

		_, err := f.svc.Submit(ctx, a, attempt.ID, agent.SubmissionInput{
			Language: submission.LanguageCPP, Code: "   ",
		})
		appErr := requireAppError(t, err, 400, 40001)
		assert.Equal(t, "代码不能为空", appErr.Message)
		assert.Zero(t, f.submissionCount(attempt.ID))
	})
}

type failingQueue struct {
	calls int
}

func (q *failingQueue) Enqueue(context.Context, int64) error {
	q.calls++
	return errors.New("judge queue unavailable")
}

type recordingPublisher struct {
	ids []int64
}

func (p *recordingPublisher) Publish(_ context.Context, submissionID int64) {
	p.ids = append(p.ids, submissionID)
}

func TestAgentSubmitPublishesSubmissionEvent(t *testing.T) {
	pub := &recordingPublisher{}
	f := newAgentFixtureWithDeps(t, noopQueue{}, nil, agent.WithPublisher(pub))
	ctx := context.Background()
	owner := f.user("publisher-owner", "active")
	view := f.create(owner, "Publisher Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "publisher-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	result, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{result.SubmissionID}, pub.ids)
}

func TestAgentSubmitEnqueueFailureStillPublishes(t *testing.T) {
	pub := &recordingPublisher{}
	f := newAgentFixtureWithDeps(t, &failingQueue{}, nil, agent.WithPublisher(pub))
	ctx := context.Background()
	owner := f.user("publisher-fail", "active")
	view := f.create(owner, "Publisher Fail Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "publisher-fail-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.Error(t, err)
	require.Len(t, pub.ids, 1)
}

func TestAgentSubmitWithoutQueueDoesNotConsumeAttempt(t *testing.T) {
	f := newAgentFixtureWithDeps(t, nil, nil)
	ctx := context.Background()
	owner := f.user("submit-no-queue", "active")
	view := f.create(owner, "No Queue Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "submit-no-queue-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 2)

	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	appErr := requireAppError(t, err, 503, 50300)
	assert.NotEmpty(t, appErr.Message)

	var stored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&stored, attempt.ID).Error)
	assert.Equal(t, 2, stored.AttemptsUsed, "a missing queue must not consume an attempt")
	assert.Zero(t, f.submissionCount(attempt.ID), "a missing queue must not create a submission")
}

func TestAgentSubmitEnqueueFailureMarksSystemError(t *testing.T) {
	queue := &failingQueue{}
	f := newAgentFixtureWithQueue(t, queue)
	ctx := context.Background()
	owner := f.user("submit-queue", "active")
	view := f.create(owner, "Queue Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "submit-queue-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.Error(t, err)
	assert.Equal(t, 1, queue.calls)

	var stored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&stored, attempt.ID).Error)
	assert.Equal(t, 1, stored.AttemptsUsed, "queued failure keeps the consumed attempt")

	var row submission.Submission
	require.NoError(t, f.db.Where("agent_attempt_id = ?", attempt.ID).First(&row).Error)
	assert.Equal(t, submission.StatusSystemError, row.Status)
	assert.NotEmpty(t, row.ErrorMessage)
	assert.False(t, row.AgentSettled)
}

func TestAgentSubmitRateLimited(t *testing.T) {
	limiter := &fakeLimiter{allow: false}
	f := newAgentFixtureWithLimiter(t, limiter)
	ctx := context.Background()
	owner := f.user("submit-limited", "active")
	view := f.create(owner, "Submit Limited Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "submit-limited-problem", Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	_, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	requireAppError(t, err, 429, 42906)
	assert.Equal(t, 1, limiter.calls)
	assert.Equal(t, "submit", limiter.action)
	assert.Zero(t, f.submissionCount(attempt.ID))
}

func TestAgentAbandonPenaltyAndPendingGuard(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	p := f.seedProblem(seedProblemOpts{Slug: "abandon-problem", Difficulty: 50, Testcases: 1})

	newAgent := func(username, name string) int64 {
		owner := f.user(username, "active")
		return f.create(owner, name).ID
	}

	t.Run("abandon charges once and rejects a second call", func(t *testing.T) {
		a := newAgent("abandon-once", "Abandon Once Agent")
		attempt := f.seedLiveAttempt(a, p.ID, 2)

		result, err := f.svc.Abandon(ctx, a, attempt.ID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, agent.AttemptAbandoned, result.Status)
		assert.Equal(t, 2, result.AttemptsUsed)
		assert.Equal(t, agent.MaxAttempts-2, result.AttemptsRemaining)
		wantDelta := agent.FailureDelta(1500, p.Difficulty)
		require.NotNil(t, result.RatingDelta)
		assert.Equal(t, wantDelta, *result.RatingDelta)

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptAbandoned, stored.Status)
		assert.Equal(t, 2, stored.AttemptsUsed)
		var storedAgent agent.Agent
		require.NoError(t, f.db.First(&storedAgent, a).Error)
		assert.Equal(t, 1500+wantDelta, storedAgent.Rating)

		history := failureHistory(t, f, a)
		require.Len(t, history, 1)
		assert.Equal(t, agent.RatingAbandon, history[0].Kind)
		assert.Equal(t, wantDelta, history[0].Delta)
		assert.Nil(t, history[0].SubmissionID)

		_, err = f.svc.Abandon(ctx, a, attempt.ID)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Contains(t, appErr.Message, "不可放弃")
		require.NoError(t, f.db.First(&storedAgent, a).Error)
		assert.Equal(t, 1500+wantDelta, storedAgent.Rating)
		require.Len(t, failureHistory(t, f, a), 1)
	})

	t.Run("foreign attempt is not found", func(t *testing.T) {
		a := newAgent("abandon-owner", "Abandon Owner")
		b := newAgent("abandon-foreign", "Abandon Foreign")
		attempt := f.seedLiveAttempt(a, p.ID, 1)

		_, err := f.svc.Abandon(ctx, b, attempt.ID)
		requireAppError(t, err, 404, 40400)
		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptActive, stored.Status)
		assert.Empty(t, failureHistory(t, f, b))
	})

	t.Run("unsettled submissions block abandon", func(t *testing.T) {
		a := newAgent("abandon-pending", "Abandon Pending Agent")
		attempt := f.seedLiveAttempt(a, p.ID, 1)

		agentID := a
		attemptID := attempt.ID
		row := &submission.Submission{
			UserID: f.agentUserID(a), ProblemID: p.ID, AgentID: &agentID, AgentAttemptID: &attemptID,
			Language: submission.LanguageCPP, Code: "int main(){}",
			Status: submission.StatusPending, AgentSettled: false, CreatedAt: f.clock.now,
		}
		require.NoError(t, f.db.Create(row).Error)

		_, err := f.svc.Abandon(ctx, a, attempt.ID)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Equal(t, "当前题目正在评测，请等待结果", appErr.Message)

		require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", row.ID).
			Update("status", submission.StatusAccepted).Error)
		_, err = f.svc.Abandon(ctx, a, attempt.ID)
		appErr = requireAppError(t, err, 400, 40001)
		assert.Equal(t, "当前题目正在评测，请等待结果", appErr.Message)

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptActive, stored.Status)
		assert.Empty(t, failureHistory(t, f, a))

		require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", row.ID).
			Update("agent_settled", true).Error)
		result, err := f.svc.Abandon(ctx, a, attempt.ID)
		require.NoError(t, err)
		assert.Equal(t, agent.AttemptAbandoned, result.Status)
		require.NotNil(t, result.RatingDelta)
		require.Len(t, failureHistory(t, f, a), 1)
	})

	t.Run("disabled agent cannot abandon", func(t *testing.T) {
		a := newAgent("abandon-disabled", "Abandon Disabled Agent")
		attempt := f.seedLiveAttempt(a, p.ID, 1)
		require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", a).
			Update("status", agent.StatusDisabled).Error)

		_, err := f.svc.Abandon(ctx, a, attempt.ID)
		requireAppError(t, err, 403, 40304)

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptActive, stored.Status)
		assert.Empty(t, failureHistory(t, f, a))
	})

	t.Run("expired attempt expires with the timeout penalty", func(t *testing.T) {
		a := newAgent("abandon-expired", "Abandon Expired Agent")
		attempt := f.seedAttempt(a, p.ID, agent.AttemptActive)

		_, err := f.svc.Abandon(ctx, a, attempt.ID)
		appErr := requireAppError(t, err, 400, 40001)
		assert.Equal(t, "题目已超时，请重新领取", appErr.Message)

		var stored agent.AgentProblemAttempt
		require.NoError(t, f.db.First(&stored, attempt.ID).Error)
		assert.Equal(t, agent.AttemptExpired, stored.Status)
		wantDelta := agent.FailureDelta(1500, p.Difficulty)
		history := failureHistory(t, f, a)
		require.Len(t, history, 1)
		assert.Equal(t, agent.RatingTimeout, history[0].Kind)
		assert.Equal(t, wantDelta, history[0].Delta)
		var storedAgent agent.Agent
		require.NoError(t, f.db.First(&storedAgent, a).Error)
		assert.Equal(t, 1500+wantDelta, storedAgent.Rating)

		_, err = f.svc.Abandon(ctx, a, attempt.ID)
		requireAppError(t, err, 400, 40001)
		require.Len(t, failureHistory(t, f, a), 1)
		require.NoError(t, f.db.First(&storedAgent, a).Error)
		assert.Equal(t, 1500+wantDelta, storedAgent.Rating)
	})
}

func TestAgentSubmissionViewBeforeSettlement(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("submission-view", "active")
	view := f.create(owner, "Submission View Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "submission-view-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 0)

	created, err := f.svc.Submit(ctx, view.ID, attempt.ID, agent.SubmissionInput{
		Language: submission.LanguageCPP, Code: "int main(){}",
	})
	require.NoError(t, err)

	pending, err := f.svc.Submission(ctx, view.ID, created.SubmissionID)
	require.NoError(t, err)
	assert.Equal(t, created.SubmissionID, pending.ID)
	assert.Equal(t, p.ID, pending.ProblemID)
	assert.Equal(t, submission.StatusPending, pending.Status)
	assert.Zero(t, pending.TimeUsedMS)
	assert.Zero(t, pending.MemoryUsedKB)
	assert.Zero(t, pending.Score)
	assert.Empty(t, pending.ErrorMessage)
	assert.Equal(t, 1, pending.AttemptsUsed)
	assert.Equal(t, agent.MaxAttempts-1, pending.AttemptsRemaining)
	assert.False(t, pending.Exhausted)
	assert.Nil(t, pending.RatingDelta)
	assert.True(t, pending.CreatedAt.Equal(f.clock.now))
	assert.Nil(t, pending.JudgedAt)

	judgedAt := f.clock.now.Add(2 * time.Second)
	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", created.SubmissionID).
		Updates(map[string]any{
			"status":         submission.StatusAccepted,
			"judged_at":      judgedAt,
			"time_used_ms":   123,
			"memory_used_kb": 456,
			"score":          100,
			"error_message":  "",
		}).Error)

	terminal, err := f.svc.Submission(ctx, view.ID, created.SubmissionID)
	require.NoError(t, err)
	assert.Equal(t, submission.StatusAccepted, terminal.Status)
	assert.Equal(t, 123, terminal.TimeUsedMS)
	assert.Equal(t, 456, terminal.MemoryUsedKB)
	assert.Equal(t, 100, terminal.Score)
	require.NotNil(t, terminal.JudgedAt)
	assert.True(t, terminal.JudgedAt.Equal(judgedAt))
	assert.Nil(t, terminal.RatingDelta, "unsettled terminals expose no rating delta")

	// Even a prematurely written history row stays hidden until settlement.
	subID := created.SubmissionID
	delta := agent.SolveDelta(1500, p.Difficulty)
	history := &agent.AgentRatingHistory{
		AgentID: view.ID, ProblemID: p.ID, SubmissionID: &subID, Kind: agent.RatingSolve,
		Difficulty: p.Difficulty, RatingBefore: 1500, RatingAfter: 1500 + delta,
		Delta: delta, CreatedAt: f.clock.now,
	}
	require.NoError(t, f.db.Create(history).Error)
	unsettled, err := f.svc.Submission(ctx, view.ID, created.SubmissionID)
	require.NoError(t, err)
	assert.Nil(t, unsettled.RatingDelta)

	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", created.SubmissionID).
		Update("agent_settled", true).Error)
	settled, err := f.svc.Submission(ctx, view.ID, created.SubmissionID)
	require.NoError(t, err)
	require.NotNil(t, settled.RatingDelta)
	assert.Equal(t, delta, *settled.RatingDelta)

	require.NoError(t, f.db.Model(&agent.AgentProblemAttempt{}).Where("id = ?", attempt.ID).
		Update("status", agent.AttemptExhausted).Error)
	exhausted, err := f.svc.Submission(ctx, view.ID, created.SubmissionID)
	require.NoError(t, err)
	assert.True(t, exhausted.Exhausted)
	assert.Equal(t, 1, exhausted.AttemptsUsed)
	assert.Equal(t, agent.MaxAttempts-1, exhausted.AttemptsRemaining)

	otherOwner := f.user("submission-view-other", "active")
	other := f.create(otherOwner, "Other View Agent")
	_, err = f.svc.Submission(ctx, other.ID, created.SubmissionID)
	requireAppError(t, err, 404, 40400)
	_, err = f.svc.Submission(ctx, view.ID, created.SubmissionID+1000)
	requireAppError(t, err, 404, 40400)
}
