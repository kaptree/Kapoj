package learning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func TestTreeStructureAndGuestStatus(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	pubA := e.addProblem(t, "tree-pub-a", true)
	pubB := e.addProblem(t, "tree-pub-b", true)
	pubC := e.addProblem(t, "tree-pub-c", true)
	willHide := e.addProblem(t, "tree-hidden", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "树路线", IsPublic: boolPtr(true)})

	rootB := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根B", ProblemIDs: []int64{pubB.ID}, SortOrder: 2,
	})
	rootA := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根A", ProblemIDs: []int64{pubA.ID}, SortOrder: 1,
	})
	rootC := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根C", ProblemIDs: []int64{pubC.ID}, SortOrder: 2,
	})
	childA2 := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子A2", ParentID: &rootA.ID, ProblemIDs: []int64{pubB.ID}, SortOrder: 2,
	})
	childA1 := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子A1", ParentID: &rootA.ID, ProblemIDs: []int64{willHide.ID}, SortOrder: 1,
	})
	grand := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "孙A1", ParentID: &childA1.ID, ProblemIDs: []int64{pubC.ID},
	})
	childB := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子B", ParentID: &rootB.ID, ProblemIDs: []int64{pubA.ID},
	})

	// 绕过服务写入孤儿节点和已下架题目引用，验证树构建的防御行为。
	orphan := &learning.LearningPathNode{
		PathID: path.ID, ParentID: nodePtr(999999), Title: "孤儿",
		ProblemIDs: dbtypes.List[int64]{pubA.ID}, RequiredCount: 1,
	}
	require.NoError(t, e.db.Create(orphan).Error)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", willHide.ID).
		Update("is_public", false).Error)

	// 本阶段即使有已完成进度也保持游客态。
	require.NoError(t, e.db.Create(&learning.UserLearningProgress{
		UserID: e.author.ID, PathID: path.ID, NodeID: rootA.ID,
		Status: learning.ProgressCompleted, SolvedCount: 5, UpdatedAt: time.Now(),
	}).Error)

	check := func(t *testing.T, v learning.Viewer) {
		t.Helper()
		tree, err := e.svc.Tree(ctx, v, path.ID)
		require.NoError(t, err)
		assert.Equal(t, path.ID, tree.Path.ID)
		assert.Equal(t, "树路线", tree.Path.Title)
		assert.True(t, tree.Path.IsPublic)

		require.Len(t, tree.Nodes, 3, "根节点按 sort_order,id 排序；孤儿节点被忽略")
		assert.Equal(t, []string{"根A", "根B", "根C"}, nodeTitles(tree.Nodes))
		assert.Nil(t, tree.Nodes[0].ParentID)
		assert.Equal(t, learning.ProgressActive, tree.Nodes[0].Status)
		assert.Zero(t, tree.Nodes[0].SolvedCount)
		assert.Equal(t, rootA.ID, tree.Nodes[0].ID)
		assert.Equal(t, rootB.ID, tree.Nodes[1].ID)
		assert.Equal(t, rootC.ID, tree.Nodes[2].ID)

		require.Len(t, tree.Nodes[0].Children, 2)
		assert.Equal(t, []string{"子A1", "子A2"}, nodeTitles(tree.Nodes[0].Children))
		assert.Equal(t, learning.ProgressLocked, tree.Nodes[0].Children[0].Status)
		assert.Equal(t, rootA.ID, *tree.Nodes[0].Children[0].ParentID)
		assert.Equal(t, childA1.ID, tree.Nodes[0].Children[0].ID)
		assert.Equal(t, childA2.ID, tree.Nodes[0].Children[1].ID)

		require.Len(t, tree.Nodes[0].Children[0].Children, 1)
		assert.Equal(t, grand.ID, tree.Nodes[0].Children[0].Children[0].ID)
		assert.Equal(t, learning.ProgressLocked, tree.Nodes[0].Children[0].Children[0].Status)

		require.Len(t, tree.Nodes[1].Children, 1)
		assert.Equal(t, childB.ID, tree.Nodes[1].Children[0].ID)
		assert.Equal(t, learning.ProgressLocked, tree.Nodes[1].Children[0].Status)
		assert.Empty(t, tree.Nodes[2].Children)

		require.Len(t, tree.Nodes[0].Problems, 1)
		assert.Equal(t, learning.ProblemSummary{
			ProblemID: pubA.ID, Title: "题目-tree-pub-a", Slug: "tree-pub-a", Difficulty: 1,
		}, tree.Nodes[0].Problems[0])

		hiddenNode := tree.Nodes[0].Children[0]
		assert.Equal(t, []int64{willHide.ID}, hiddenNode.ProblemIDs,
			"隐藏题仍保留在 problemIds，但不返回题目摘要")
		assert.Empty(t, hiddenNode.Problems)
	}

	check(t, learning.Viewer{})
	check(t, authorView)
}

func TestTreePermissions(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, otherView, adminView := viewerFor(e.author), viewerFor(e.other), viewerFor(e.admin)

	public := e.createPath(t, authorView, learning.PathInput{Title: "公开树", IsPublic: boolPtr(true)})
	private := e.createPath(t, authorView, learning.PathInput{Title: "私有树"})
	inactive := e.createPath(t, authorView, learning.PathInput{Title: "下架树"})
	official := e.createPath(t, adminView, learning.PathInput{Title: "官方树", IsOfficial: boolPtr(true)})
	require.NoError(t, e.svc.DeletePath(ctx, authorView, inactive.ID))

	for _, v := range []learning.Viewer{{}, otherView, authorView, adminView} {
		tree, err := e.svc.Tree(ctx, v, public.ID)
		require.NoError(t, err)
		assert.NotNil(t, tree.Nodes)
		tree, err = e.svc.Tree(ctx, v, official.ID)
		require.NoError(t, err)
		assert.NotNil(t, tree.Nodes)
	}

	_, err := e.svc.Tree(ctx, learning.Viewer{}, private.ID)
	_ = requireAppError(t, err, 404, 40400)
	_, err = e.svc.Tree(ctx, otherView, private.ID)
	_ = requireAppError(t, err, 404, 40400)
	tree, err := e.svc.Tree(ctx, authorView, private.ID)
	require.NoError(t, err)
	assert.Equal(t, private.ID, tree.Path.ID)
	_, err = e.svc.Tree(ctx, adminView, private.ID)
	require.NoError(t, err)

	_, err = e.svc.Tree(ctx, authorView, inactive.ID)
	_ = requireAppError(t, err, 404, 40400)
	_, err = e.svc.Tree(ctx, learning.Viewer{}, inactive.ID)
	_ = requireAppError(t, err, 404, 40400)
	tree, err = e.svc.Tree(ctx, adminView, inactive.ID)
	require.NoError(t, err)
	assert.False(t, tree.Path.IsActive)

	_, err = e.svc.Tree(ctx, adminView, 999999)
	_ = requireAppError(t, err, 404, 40400)
}

func nodeTitles(items []learning.NodeItem) []string {
	out := make([]string, len(items))
	for i := range items {
		out[i] = items[i].Title
	}
	return out
}
