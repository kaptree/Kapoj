package learning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
)

func TestLearningHandleGameEventAccepted(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "event-p1", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "事件路线"})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})

	// 插入 accepted 提交并投递 submission.accepted 事件。
	e.addAccepted(t, e.author.ID, p1.ID)
	require.NoError(t, e.svc.HandleGameEvent(ctx, gamequeue.Event{
		Type:   gamequeue.EventSubmissionAccepted,
		UserID: e.author.ID,
	}))

	assertProgress(t, e.progress(t, e.author.ID, root.ID), learning.ProgressCompleted, 1)
}

func TestLearningHandleGameEventIgnoresOtherTypes(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "event-ignore-p1", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "事件忽略路线"})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})

	// 已存在 accepted 提交：若错误地调用 EvaluateUser，节点会完成。
	e.addAccepted(t, e.author.ID, p1.ID)

	for _, ev := range []gamequeue.Event{
		{Type: gamequeue.EventUserLogin, UserID: e.author.ID},
		{Type: gamequeue.EventContestJoin, UserID: e.author.ID, ContestID: 1},
		{Type: gamequeue.EventType("unknown.type"), UserID: e.author.ID},
		{Type: gamequeue.EventSubmissionAccepted, UserID: 0},
	} {
		require.NoError(t, e.svc.HandleGameEvent(ctx, ev))
	}

	var count int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).
		Where("user_id = ? AND node_id = ?", e.author.ID, root.ID).Count(&count).Error)
	assert.Zero(t, count, "非 accepted 事件与无效 UserID 不写进度")
}
