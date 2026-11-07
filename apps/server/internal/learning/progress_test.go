package learning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

// addAccepted inserts an accepted submission directly, bypassing the judge.
func (e *pathEnv) addAccepted(t *testing.T, userID, problemID int64) {
	t.Helper()
	require.NoError(t, e.db.Create(&submission.Submission{
		UserID: userID, ProblemID: problemID,
		Language: submission.LanguageC, Code: "x",
		Status: submission.StatusAccepted,
	}).Error)
}

// progress loads one user's stored progress row for a node.
func (e *pathEnv) progress(t *testing.T, userID, nodeID int64) learning.UserLearningProgress {
	t.Helper()
	var p learning.UserLearningProgress
	require.NoError(t, e.db.Where("user_id = ? AND node_id = ?", userID, nodeID).First(&p).Error)
	return p
}

// assertProgress checks status and solved count, and that only completed rows
// carry a non-nil completed_at.
func assertProgress(t *testing.T, got learning.UserLearningProgress, status string, solved int) {
	t.Helper()
	assert.Equal(t, status, got.Status)
	assert.Equal(t, solved, got.SolvedCount)
	if status == learning.ProgressCompleted {
		assert.NotNil(t, got.CompletedAt)
	} else {
		assert.Nil(t, got.CompletedAt)
	}
}

func TestEvaluateUserUnlockChain(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "chain-p1", true)
	p2 := e.addProblem(t, "chain-p2", true)
	p3 := e.addProblem(t, "chain-p3", true)
	p4 := e.addProblem(t, "chain-p4", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "解锁链"})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})
	child := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子", ParentID: &root.ID, ProblemIDs: []int64{p3.ID},
	})
	grand := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "孙", ParentID: &child.ID, ProblemIDs: []int64{p4.ID},
	})

	// 无 AC：根 active/0，子/孙 locked/0。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressActive, 0)
	assertProgress(t, e.progress(t, e.author.ID, child.ID), learning.ProgressLocked, 0)
	assertProgress(t, e.progress(t, e.author.ID, grand.ID), learning.ProgressLocked, 0)

	// AC p1：根 active/1。
	e.addAccepted(t, e.author.ID, p1.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressActive, 1)
	assertProgress(t, e.progress(t, e.author.ID, child.ID), learning.ProgressLocked, 0)
	assertProgress(t, e.progress(t, e.author.ID, grand.ID), learning.ProgressLocked, 0)

	// AC p1+p2：根 completed/2，子 active/0。
	e.addAccepted(t, e.author.ID, p2.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
	assertProgress(t, e.progress(t, e.author.ID, child.ID), learning.ProgressActive, 0)
	assertProgress(t, e.progress(t, e.author.ID, grand.ID), learning.ProgressLocked, 0)

	// AC p3：子 completed/1，孙 active/0。
	e.addAccepted(t, e.author.ID, p3.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
	assertProgress(t, e.progress(t, e.author.ID, child.ID), learning.ProgressCompleted, 1)
	assertProgress(t, e.progress(t, e.author.ID, grand.ID), learning.ProgressActive, 0)

	// AC p4：孙 completed/1。
	e.addAccepted(t, e.author.ID, p4.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
	assertProgress(t, e.progress(t, e.author.ID, child.ID), learning.ProgressCompleted, 1)
	assertProgress(t, e.progress(t, e.author.ID, grand.ID), learning.ProgressCompleted, 1)
}

func TestEvaluateUserDeduplicatesAcceptedSubmissions(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "dedup-p1", true)
	p2 := e.addProblem(t, "dedup-p2", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "去重"})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})

	// 同一题多条 accepted 只计 1；非 accepted 不计。
	e.addAccepted(t, e.author.ID, p1.ID)
	e.addAccepted(t, e.author.ID, p1.ID)
	require.NoError(t, e.db.Create(&submission.Submission{
		UserID: e.author.ID, ProblemID: p2.ID,
		Language: submission.LanguageC, Code: "x",
		Status: submission.StatusWrongAnswer,
	}).Error)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressActive, 1)

	// AC p2 后：去重后 solved=2，完成。
	e.addAccepted(t, e.author.ID, p2.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
}

func TestEvaluateUserCompletedLatch(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "latch-p1", true)
	p2 := e.addProblem(t, "latch-p2", true)
	p3 := e.addProblem(t, "latch-p3", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "锁存", IsPublic: boolPtr(true)})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})

	// 作者完成根节点（p1+p2）。
	e.addAccepted(t, e.author.ID, p1.ID)
	e.addAccepted(t, e.author.ID, p2.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)

	// 另一用户只 AC p2，处于进行中。
	e.addAccepted(t, e.other.ID, p2.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.other.ID))
	assertProgress(t, e.progress(t, e.other.ID, root.ID), learning.ProgressActive, 1)

	// 改题：移除作者已 AC 的 p2、换成 p3。已完成用户锁存，保持 completed 与
	// 完成时的 solvedCount=2；未完成用户按新配置重算为 0。
	_, err := e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p3.ID},
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.other.ID))
	assertProgress(t, e.progress(t, e.other.ID, root.ID), learning.ProgressActive, 0)

	// 提高 required：作者实际 solved=1 已不足以完成，但锁存仍保持 completed/2。
	_, err = e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p3.ID, p2.ID}, RequiredCount: intPtr(3),
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 2)
}

func TestEvaluateUserVisibilityAndInactivePath(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	adminView := viewerFor(e.admin)

	p1 := e.addProblem(t, "vis-p1", true)
	p2 := e.addProblem(t, "vis-p2", true)

	private := e.createPath(t, authorView, learning.PathInput{Title: "作者私有"})
	root := e.createNode(t, authorView, private.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})

	e.addAccepted(t, e.author.ID, p1.ID)
	e.addAccepted(t, e.other.ID, p1.ID)

	// 作者私有路线：作者重算为 completed，其他用户不可见、不写进度。
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 1)

	require.NoError(t, e.svc.EvaluateUser(ctx, e.other.ID))
	var count int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).
		Where("user_id = ? AND node_id = ?", e.other.ID, root.ID).Count(&count).Error)
	assert.Zero(t, count, "其他用户不可见私有路线，不写进度")

	// 下架路线：即使改题，已有进度也保持不变（不触碰）。
	require.NoError(t, e.svc.DeletePath(ctx, authorView, private.ID))
	_, err := e.svc.UpdateNode(ctx, adminView, private.ID, root.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p2.ID},
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 1)

	// 重新上架后作者进度继续（锁存仍 completed）。
	_, err = e.svc.UpdatePath(ctx, adminView, private.ID, learning.PathInput{
		Title: "作者私有", IsActive: boolPtr(true),
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 1)
}

func TestEvaluateUserGuestNoop(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "guest-p1", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "游客路线", IsPublic: boolPtr(true)})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})

	require.NoError(t, e.svc.EvaluateUser(ctx, 0))
	require.NoError(t, e.svc.EvaluateUser(ctx, -5))

	var count int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).Count(&count).Error)
	assert.Zero(t, count, "游客不写任何进度行")
}
