package agent_test

import (
	"context"
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
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
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
	g := testutil.PG(t)
	clock := &testClock{now: time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)}
	svc := agent.NewService(g, nil, nil, nil, limiter,
		agent.WithClock(clock.time), agent.WithClaimTimeout(5*time.Minute))
	return &agentFixture{t: t, db: g, svc: svc, clock: clock}
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

	// Once judging finishes the stale attempt may expire and the Agent can
	// claim again.
	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", pending.ID).
		Update("status", submission.StatusAccepted).Error)
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
