package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// seedAgentSubmission inserts an Agent submission directly. judgedAt is nil
// for submissions that are still in flight.
func (f *agentFixture) seedAgentSubmission(
	agentID, attemptID, problemID int64,
	status submission.Status,
	judgedAt *time.Time,
	settled bool,
) int64 {
	f.t.Helper()
	aid, atid := agentID, attemptID
	row := &submission.Submission{
		UserID: f.agentUserID(agentID), ProblemID: problemID,
		AgentID: &aid, AgentAttemptID: &atid, AgentSettled: settled,
		Language: submission.LanguageCPP, Code: "int main(){}",
		Status: status, JudgedAt: judgedAt,
		CreatedAt: f.clock.now.Add(-5 * time.Minute),
	}
	require.NoError(f.t, f.db.Create(row).Error)
	return row.ID
}

func (f *agentFixture) seedJudgedSubmission(
	agentID, attemptID, problemID int64, status submission.Status, judgedAt time.Time, settled bool,
) int64 {
	f.t.Helper()
	return f.seedAgentSubmission(agentID, attemptID, problemID, status, &judgedAt, settled)
}

func (f *agentFixture) seedAttemptClaimed(
	agentID, problemID int64, used int, claimedAt time.Time,
) *agent.AgentProblemAttempt {
	f.t.Helper()
	a := &agent.AgentProblemAttempt{
		AgentID: agentID, ProblemID: problemID, AttemptsUsed: used, Status: agent.AttemptActive,
		ClaimedAt: claimedAt, DeadlineAt: f.clock.now.Add(time.Hour),
		CreatedAt: claimedAt, UpdatedAt: claimedAt,
	}
	require.NoError(f.t, f.db.Create(a).Error)
	return a
}

func (f *agentFixture) loadAgent(agentID int64) agent.Agent {
	f.t.Helper()
	var a agent.Agent
	require.NoError(f.t, f.db.First(&a, agentID).Error)
	return a
}

func (f *agentFixture) solveHistory(agentID int64) []agent.AgentRatingHistory {
	f.t.Helper()
	var rows []agent.AgentRatingHistory
	require.NoError(f.t, f.db.Where("agent_id = ? AND kind = ?", agentID, agent.RatingSolve).
		Order("id").Find(&rows).Error)
	return rows
}

func TestAgentSettlementAcceptedOnce(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-accepted", "active")
	view := f.create(owner, "Settle Accepted Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "settle-accepted-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 1)

	judgedAt := f.clock.now.Add(-30 * time.Second)
	subID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusAccepted, judgedAt, false)

	n, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var storedAttempt agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&storedAttempt, attempt.ID).Error)
	assert.Equal(t, agent.AttemptSolved, storedAttempt.Status)
	require.NotNil(t, storedAttempt.SolvedAt)
	assert.True(t, storedAttempt.SolvedAt.Equal(judgedAt))
	assert.Equal(t, 30000, storedAttempt.SolveTimeMS, "solve time runs from claim to judge")

	wantDelta := agent.SolveDelta(1500, p.Difficulty)
	storedAgent := f.loadAgent(view.ID)
	assert.Equal(t, 1500+wantDelta, storedAgent.Rating)
	assert.Equal(t, 1, storedAgent.PassedCount)
	assert.Equal(t, 30000, storedAgent.AvgSolveTimeMS)

	var stored submission.Submission
	require.NoError(t, f.db.First(&stored, subID).Error)
	assert.True(t, stored.AgentSettled)

	history := f.solveHistory(view.ID)
	require.Len(t, history, 1)
	assert.Equal(t, agent.RatingSolve, history[0].Kind)
	require.NotNil(t, history[0].SubmissionID)
	assert.Equal(t, subID, *history[0].SubmissionID)
	assert.Equal(t, 1500, history[0].RatingBefore)
	assert.Equal(t, 1500+wantDelta, history[0].RatingAfter)
	assert.Equal(t, wantDelta, history[0].Delta)
	assert.Equal(t, 30000, history[0].SolveTimeMS)

	// Repeated scans see no unsettled submissions and change nothing.
	n, err = f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 1500+wantDelta, f.loadAgent(view.ID).Rating)
	assert.Len(t, f.solveHistory(view.ID), 1)
}

func TestAgentTwentiethAcceptedCounts(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-twentieth-ac", "active")
	view := f.create(owner, "Twentieth AC Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "settle-twentieth-ac-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, agent.MaxAttempts)

	judgedAt := f.clock.now.Add(-time.Minute)
	for i := 0; i < agent.MaxAttempts-1; i++ {
		f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer, judgedAt, true)
	}
	acID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusAccepted,
		f.clock.now.Add(-10*time.Second), false)

	n, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var storedAttempt agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&storedAttempt, attempt.ID).Error)
	assert.Equal(t, agent.AttemptSolved, storedAttempt.Status, "the 20th AC solves the problem")

	wantDelta := agent.SolveDelta(1500, p.Difficulty)
	storedAgent := f.loadAgent(view.ID)
	assert.Equal(t, 1500+wantDelta, storedAgent.Rating)
	assert.Equal(t, 1, storedAgent.PassedCount)
	assert.Equal(t, 50000, storedAgent.AvgSolveTimeMS)
	assert.Empty(t, failureHistory(t, f, view.ID), "the 20th accepted submission is not exhaustion")

	view2, err := f.svc.Submission(ctx, view.ID, acID)
	require.NoError(t, err)
	assert.False(t, view2.Exhausted)
	require.NotNil(t, view2.RatingDelta)
	assert.Equal(t, wantDelta, *view2.RatingDelta)
}

func TestAgentSettlementExhaustedAfter20Failures(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-exhausted", "active")
	view := f.create(owner, "Exhausted Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "settle-exhausted-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, agent.MaxAttempts)

	judgedAt := f.clock.now.Add(-time.Minute)
	for i := 0; i < agent.MaxAttempts-1; i++ {
		f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer, judgedAt, true)
	}
	lastID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer,
		f.clock.now.Add(-5*time.Second), false)

	n, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var storedAttempt agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&storedAttempt, attempt.ID).Error)
	assert.Equal(t, agent.AttemptExhausted, storedAttempt.Status)
	assert.Nil(t, storedAttempt.SolvedAt)

	wantDelta := agent.FailureDelta(1500, p.Difficulty)
	storedAgent := f.loadAgent(view.ID)
	assert.Equal(t, 1500+wantDelta, storedAgent.Rating)
	assert.Zero(t, storedAgent.PassedCount)
	assert.Empty(t, f.solveHistory(view.ID))

	history := failureHistory(t, f, view.ID)
	require.Len(t, history, 1, "exhaustion charges one failure penalty")
	assert.Equal(t, agent.RatingExhausted, history[0].Kind)
	require.NotNil(t, history[0].SubmissionID)
	assert.Equal(t, lastID, *history[0].SubmissionID)
	assert.Equal(t, wantDelta, history[0].Delta)

	// The settled submission exposes the negative delta and the exhausted
	// marker that the UI renders as "错误次数耗尽".
	sub, err := f.svc.Submission(ctx, view.ID, lastID)
	require.NoError(t, err)
	assert.True(t, sub.Exhausted)
	require.NotNil(t, sub.RatingDelta)
	assert.Equal(t, wantDelta, *sub.RatingDelta)

	n, err = f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 1500+wantDelta, f.loadAgent(view.ID).Rating)
	require.Len(t, failureHistory(t, f, view.ID), 1)
}

func TestAgentSubmissionViewIncludesDeltaAfterSettlement(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()

	t.Run("solve delta", func(t *testing.T) {
		owner := f.user("delta-solve", "active")
		view := f.create(owner, "Delta Solve Agent")
		p := f.seedProblem(seedProblemOpts{Slug: "delta-solve-problem", Difficulty: 30, Testcases: 1})
		attempt := f.seedLiveAttempt(view.ID, p.ID, 1)
		subID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusAccepted,
			f.clock.now, false)

		_, err := f.svc.SettleOnce(ctx, 10)
		require.NoError(t, err)

		sub, err := f.svc.Submission(ctx, view.ID, subID)
		require.NoError(t, err)
		wantDelta := agent.SolveDelta(1500, p.Difficulty)
		require.NotNil(t, sub.RatingDelta)
		assert.Equal(t, wantDelta, *sub.RatingDelta)
		assert.False(t, sub.Exhausted)
	})

	t.Run("exhausted delta", func(t *testing.T) {
		owner := f.user("delta-exhausted", "active")
		view := f.create(owner, "Delta Exhausted Agent")
		p := f.seedProblem(seedProblemOpts{Slug: "delta-exhausted-problem", Difficulty: 30, Testcases: 1})
		attempt := f.seedLiveAttempt(view.ID, p.ID, agent.MaxAttempts)
		subID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer,
			f.clock.now, false)

		_, err := f.svc.SettleOnce(ctx, 10)
		require.NoError(t, err)

		sub, err := f.svc.Submission(ctx, view.ID, subID)
		require.NoError(t, err)
		wantDelta := agent.FailureDelta(1500, p.Difficulty)
		require.NotNil(t, sub.RatingDelta)
		assert.Equal(t, wantDelta, *sub.RatingDelta)
		assert.True(t, sub.Exhausted)
	})
}

func TestAgentSettlementIncrementalAverageSecondProblem(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-average", "active")
	view := f.create(owner, "Average Agent")
	p1 := f.seedProblem(seedProblemOpts{Slug: "settle-average-one", Difficulty: 50, Testcases: 1})

	first := f.seedAttemptClaimed(view.ID, p1.ID, 1, f.clock.now.Add(-10*time.Minute))
	f.seedJudgedSubmission(view.ID, first.ID, p1.ID, submission.StatusAccepted,
		f.clock.now.Add(-4*time.Minute), false)

	_, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	afterFirst := f.loadAgent(view.ID)
	assert.Equal(t, 1, afterFirst.PassedCount)
	assert.Equal(t, 6*60*1000, afterFirst.AvgSolveTimeMS)

	p2 := f.seedProblem(seedProblemOpts{Slug: "settle-average-two", Difficulty: 50, Testcases: 1})
	second := f.seedAttemptClaimed(view.ID, p2.ID, 1, f.clock.now.Add(-2*time.Minute))
	f.seedJudgedSubmission(view.ID, second.ID, p2.ID, submission.StatusAccepted,
		f.clock.now, false)

	_, err = f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	stored := f.loadAgent(view.ID)
	assert.Equal(t, 2, stored.PassedCount)
	assert.Equal(t, 4*60*1000, stored.AvgSolveTimeMS,
		"average combines the old average with the new solve time")

	history := f.solveHistory(view.ID)
	require.Len(t, history, 2)
	assert.Equal(t, afterFirst.Rating, history[1].RatingBefore)
	assert.Equal(t, agent.SolveDelta(afterFirst.Rating, p2.Difficulty), history[1].Delta)
	assert.Equal(t, afterFirst.Rating+history[1].Delta, stored.Rating)
}

func TestAgentFailurePenaltyOncePerProblem(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("failure-once", "active")
	view := f.create(owner, "Failure Once Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "failure-once-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 3)

	abandoned, err := f.svc.Abandon(ctx, view.ID, attempt.ID)
	require.NoError(t, err)
	require.NotNil(t, abandoned.RatingDelta)
	wantDelta := agent.FailureDelta(1500, p.Difficulty)
	assert.Equal(t, wantDelta, *abandoned.RatingDelta)
	require.Len(t, failureHistory(t, f, view.ID), 1)

	reclaimed, err := f.svc.Claim(ctx, view.ID)
	require.NoError(t, err)
	require.Equal(t, attempt.ID, reclaimed.Attempt.ID)
	require.Equal(t, agent.AttemptActive, reclaimed.Attempt.Status)

	f.clock.advance(6 * time.Minute)
	n, err := f.svc.SweepOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var storedAttempt agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&storedAttempt, attempt.ID).Error)
	assert.Equal(t, agent.AttemptExpired, storedAttempt.Status)

	history := failureHistory(t, f, view.ID)
	require.Len(t, history, 1, "abandon and timeout only charge the problem once")
	assert.Equal(t, agent.RatingAbandon, history[0].Kind)
	assert.Equal(t, 1500+wantDelta, f.loadAgent(view.ID).Rating)
}

func TestAgentSweepSkipsPendingAndExpiresIdle(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	p := f.seedProblem(seedProblemOpts{Slug: "sweep-problem", Difficulty: 50, Testcases: 1})

	pendingOwner := f.user("sweep-pending", "active")
	pendingAgent := f.create(pendingOwner, "Sweep Pending Agent").ID
	pendingAttempt := f.seedAttempt(pendingAgent, p.ID, agent.AttemptActive)
	f.seedAgentSubmission(pendingAgent, pendingAttempt.ID, p.ID, submission.StatusPending, nil, false)

	idleOwner := f.user("sweep-idle", "active")
	idleAgent := f.create(idleOwner, "Sweep Idle Agent").ID
	idleAttempt := f.seedAttempt(idleAgent, p.ID, agent.AttemptActive)

	n, err := f.svc.SweepOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the idle attempt expires")

	var pendingStored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&pendingStored, pendingAttempt.ID).Error)
	assert.Equal(t, agent.AttemptActive, pendingStored.Status, "in-flight work blocks the timeout")
	assert.Empty(t, failureHistory(t, f, pendingAgent))

	var idleStored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&idleStored, idleAttempt.ID).Error)
	assert.Equal(t, agent.AttemptExpired, idleStored.Status)

	wantDelta := agent.FailureDelta(1500, p.Difficulty)
	assert.Equal(t, 1500+wantDelta, f.loadAgent(idleAgent).Rating)
	history := failureHistory(t, f, idleAgent)
	require.Len(t, history, 1)
	assert.Equal(t, agent.RatingTimeout, history[0].Kind)
	assert.Equal(t, wantDelta, history[0].Delta)
	assert.Nil(t, history[0].SubmissionID)

	n, err = f.svc.SweepOnce(ctx, 10)
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestAgentSweepSkipsUnsettledAcceptedSubmission(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("sweep-unsettled", "active")
	view := f.create(owner, "Sweep Unsettled Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "sweep-unsettled-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedAttempt(view.ID, p.ID, agent.AttemptActive)
	subID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusAccepted,
		f.clock.now.Add(-time.Minute), false)

	// An accepted-but-unsettled submission is still in flight: neither the
	// sweeper nor Claim may expire the attempt and charge a timeout first.
	n, err := f.svc.SweepOnce(ctx, 10)
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = f.svc.Claim(ctx, view.ID)
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "当前题目正在评测")

	var stored agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&stored, attempt.ID).Error)
	assert.Equal(t, agent.AttemptActive, stored.Status)
	assert.Empty(t, failureHistory(t, f, view.ID))

	require.NoError(t, f.db.Model(&submission.Submission{}).Where("id = ?", subID).
		Update("agent_settled", true).Error)

	n, err = f.svc.SweepOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, f.db.First(&stored, attempt.ID).Error)
	assert.Equal(t, agent.AttemptExpired, stored.Status)
	require.Len(t, failureHistory(t, f, view.ID), 1)
}

func TestAgentSettlementWrongAnswerKeepsAttemptActive(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-wa", "active")
	view := f.create(owner, "Settle WA Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "settle-wa-problem", Difficulty: 50, Testcases: 1})
	attempt := f.seedLiveAttempt(view.ID, p.ID, 3)
	subID := f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer,
		f.clock.now, false)

	n, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var storedAttempt agent.AgentProblemAttempt
	require.NoError(t, f.db.First(&storedAttempt, attempt.ID).Error)
	assert.Equal(t, agent.AttemptActive, storedAttempt.Status, "an early WA leaves the attempt open")
	assert.Empty(t, failureHistory(t, f, view.ID))
	assert.Equal(t, 1500, f.loadAgent(view.ID).Rating)

	var stored submission.Submission
	require.NoError(t, f.db.First(&stored, subID).Error)
	assert.True(t, stored.AgentSettled)
}

func TestAgentRunWorkersRespondToContext(t *testing.T) {
	f := newAgentFixture(t)

	for name, run := range map[string]func(context.Context, time.Duration){
		"settler": f.svc.RunSettler,
		"sweeper": f.svc.RunSweeper,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				run(ctx, time.Hour)
				close(done)
			}()
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not stop after context cancellation")
			}
		})
	}
}

func TestAgentSettlementRatingFloor(t *testing.T) {
	f := newAgentFixture(t)
	ctx := context.Background()
	owner := f.user("settle-floor", "active")
	view := f.create(owner, "Floor Agent")
	p := f.seedProblem(seedProblemOpts{Slug: "settle-floor-problem", Difficulty: 1, Testcases: 1})
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", view.ID).Update("rating", 1).Error)

	attempt := f.seedLiveAttempt(view.ID, p.ID, agent.MaxAttempts)
	f.seedJudgedSubmission(view.ID, attempt.ID, p.ID, submission.StatusWrongAnswer, f.clock.now, false)

	_, err := f.svc.SettleOnce(ctx, 10)
	require.NoError(t, err)

	// FailureDelta(1, 1) is -1, which is larger than the remaining rating, so
	// the stored delta is clamped to the actual drop to zero.
	require.Equal(t, -1, agent.FailureDelta(1, p.Difficulty))
	history := failureHistory(t, f, view.ID)
	require.Len(t, history, 1)
	assert.Equal(t, 1, history[0].RatingBefore)
	assert.Zero(t, history[0].RatingAfter)
	assert.Equal(t, -1, history[0].Delta)
	assert.Zero(t, f.loadAgent(view.ID).Rating)
}
