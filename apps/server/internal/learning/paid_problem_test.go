package learning_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func (e *pathEnv) createPaidPath(t *testing.T, problemIDs []int64) *learning.PathItem {
	t.Helper()
	authorView := viewerFor(e.author)
	path := e.createPath(t, authorView, learning.PathInput{
		Title: "付费题路线", IsPublic: boolPtr(true), PriceCoins: 20,
	})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: problemIDs,
	})
	return path
}

func TestPaidPathTreeIsLockedUntilPurchased(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	private := e.addProblem(t, "tree-paid-private", false)
	path := e.createPaidPath(t, []int64{private.ID})

	locked, err := e.svc.Tree(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.True(t, locked.Locked)
	assert.Empty(t, locked.Nodes)
	assert.EqualValues(t, 20, locked.Path.PriceCoins)

	authorTree, err := e.svc.Tree(ctx, viewerFor(e.author), path.ID)
	require.NoError(t, err)
	assert.False(t, authorTree.Locked)
	require.Len(t, authorTree.Nodes, 1)
	require.Len(t, authorTree.Nodes[0].Problems, 1)
	assert.Equal(t, private.ID, authorTree.Nodes[0].Problems[0].ProblemID)

	e.setCoins(t, e.other.ID, 50)
	_, err = e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	unlocked, err := e.svc.Tree(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.False(t, unlocked.Locked)
	require.Len(t, unlocked.Nodes, 1)
	require.Len(t, unlocked.Nodes[0].Problems, 1)
	assert.Equal(t, "题目-tree-paid-private", unlocked.Nodes[0].Problems[0].Title)
}

func TestPaidPathNodesRejectPublicProblems(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	path := e.createPath(t, authorView, learning.PathInput{
		Title: "付费节点", IsPublic: boolPtr(true), PriceCoins: 20,
	})
	pub := e.addProblem(t, "node-paid-public", true)
	private := e.addProblem(t, "node-paid-private", false)

	_, err := e.svc.CreateNode(ctx, authorView, path.ID, learning.NodeInput{
		Title: "公开冲突", ProblemIDs: []int64{pub.ID},
	})
	requireAppError(t, err, 400, 40001)

	_, err = e.svc.CreateNode(ctx, authorView, path.ID, learning.NodeInput{
		Title: "私有节点", ProblemIDs: []int64{private.ID},
	})
	require.NoError(t, err)
}

func TestProblemPublishAndReadUsePaidPathGuard(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	e.addProblem(t, "guard-seed", false)
	paidProblem := &problem.Problem{
		Title: "付费守卫题", Slug: "paid-guard-problem",
		StatementMD: "题干", Type: problem.TypeObjective,
		Answers: dbtypes.List[string]{"A"}, Difficulty: 1,
		TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: false,
	}
	require.NoError(t, e.db.Create(paidProblem).Error)
	path := e.createPaidPath(t, []int64{paidProblem.ID})

	problemSvc := problem.NewService(problem.NewGormRepository(e.db), nil,
		problem.WithLearningGuard(e.svc))
	in := problem.Input{
		Title: "付费守卫题", Slug: "paid-guard-problem", StatementMD: "题干",
		Type: problem.TypeObjective, Answers: []string{"A"}, Difficulty: 1,
		TimeLimitMS: 1000, MemoryLimitMB: 256, IsPublic: true,
	}
	_, err := problemSvc.Update(ctx, paidProblem.ID, in)
	requireAppError(t, err, 400, 40001)

	// A purchaser may read the private problem through the paid path.
	e.setCoins(t, e.other.ID, 100)
	_, err = e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	got, err := problemSvc.Get(ctx, problem.Viewer{UserID: e.other.ID}, paidProblem.ID)
	require.NoError(t, err)
	assert.Equal(t, paidProblem.ID, got.ID)

	_, err = problemSvc.Get(ctx, problem.Viewer{UserID: e.admin.ID, CanViewHidden: true}, paidProblem.ID)
	require.NoError(t, err, "administrators may read hidden problems")

	// Removing the paid reference allows publishing again.
	require.NoError(t, e.db.Exec(fmt.Sprintf(`UPDATE learning_path_nodes SET problem_ids_json = '[]' WHERE path_id = %d`, path.ID)).Error)
	_, err = problemSvc.Update(ctx, paidProblem.ID, in)
	require.NoError(t, err)
}
