package learning_test

import (
	"context"
	"testing"

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

	// 绕过服务写入孤儿节点、已下架题目引用与不存在的题目 id，验证树构建的防御行为。
	orphan := &learning.LearningPathNode{
		PathID: path.ID, ParentID: nodePtr(999999), Title: "孤儿",
		ProblemIDs: dbtypes.List[int64]{pubA.ID}, RequiredCount: 1,
	}
	require.NoError(t, e.db.Create(orphan).Error)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", willHide.ID).
		Update("is_public", false).Error)
	require.NoError(t, e.db.Model(&learning.LearningPathNode{}).Where("id = ?", childA1.ID).
		Update("problem_ids_json", dbtypes.List[int64]{willHide.ID, 999999}).Error)

	check := func(t *testing.T, v learning.Viewer, fullProblemIDs bool) {
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
		assert.Equal(t, []int64{pubA.ID}, tree.Nodes[0].ProblemIDs, "公开题目 id 对所有查看者可见")
		assert.Equal(t, learning.ProblemSummary{
			ProblemID: pubA.ID, Title: "题目-tree-pub-a", Slug: "tree-pub-a", Difficulty: 1,
		}, tree.Nodes[0].Problems[0])

		hiddenNode := tree.Nodes[0].Children[0]
		assert.Empty(t, hiddenNode.Problems, "只返回公开题目摘要")
		if fullProblemIDs {
			assert.Equal(t, []int64{willHide.ID, 999999}, hiddenNode.ProblemIDs,
				"作者/管理员保留完整存储的题目 id 供编辑")
		} else {
			assert.Empty(t, hiddenNode.ProblemIDs,
				"游客/其他用户只看到公开题目 id，不泄露已下线或缺失题目")
		}
	}

	check(t, learning.Viewer{}, false)
	check(t, viewerFor(e.other), false)
	check(t, authorView, true)
	check(t, viewerFor(e.admin), true)
}

func TestTreeUsesEvaluatedProgress(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "eval-p1", true)
	p2 := e.addProblem(t, "eval-p2", true)
	p3 := e.addProblem(t, "eval-p3", true)
	p4 := e.addProblem(t, "eval-p4", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "进度树", IsPublic: boolPtr(true)})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})
	child := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子", ParentID: &root.ID, ProblemIDs: []int64{p3.ID},
	})
	grand := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "孙", ParentID: &child.ID, ProblemIDs: []int64{p4.ID},
	})

	// 用户 AC 根节点 required 题目后调用 Tree：root completed、child active、
	// grandchild locked，solvedCount 正确。
	e.addAccepted(t, e.author.ID, p1.ID)
	e.addAccepted(t, e.author.ID, p2.ID)

	tree, err := e.svc.Tree(ctx, authorView, path.ID)
	require.NoError(t, err)
	require.Len(t, tree.Nodes, 1)
	rootItem := tree.Nodes[0]
	assert.Equal(t, root.ID, rootItem.ID)
	assert.Equal(t, learning.ProgressCompleted, rootItem.Status)
	assert.Equal(t, 2, rootItem.SolvedCount)

	require.Len(t, rootItem.Children, 1)
	childItem := rootItem.Children[0]
	assert.Equal(t, child.ID, childItem.ID)
	assert.Equal(t, learning.ProgressActive, childItem.Status)
	assert.Zero(t, childItem.SolvedCount)

	require.Len(t, childItem.Children, 1)
	assert.Equal(t, grand.ID, childItem.Children[0].ID)
	assert.Equal(t, learning.ProgressLocked, childItem.Children[0].Status)
	assert.Zero(t, childItem.Children[0].SolvedCount)
}

func TestTreeGuestStaysLocked(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "guest-tree-p1", true)
	p2 := e.addProblem(t, "guest-tree-p2", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "游客树", IsPublic: boolPtr(true)})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})
	child := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子", ParentID: &root.ID, ProblemIDs: []int64{p2.ID},
	})

	// 作者完成根节点，写进度；游客不继承作者进度。
	e.addAccepted(t, e.author.ID, p1.ID)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.author.ID))
	var before int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).Count(&before).Error)
	assert.Equal(t, int64(2), before, "作者完成根节点后仅根与子的两行进度")

	tree, err := e.svc.Tree(ctx, learning.Viewer{}, path.ID)
	require.NoError(t, err)
	require.Len(t, tree.Nodes, 1)
	assert.Equal(t, learning.ProgressActive, tree.Nodes[0].Status)
	assert.Zero(t, tree.Nodes[0].SolvedCount)
	require.Len(t, tree.Nodes[0].Children, 1)
	assert.Equal(t, child.ID, tree.Nodes[0].Children[0].ID)
	assert.Equal(t, learning.ProgressLocked, tree.Nodes[0].Children[0].Status)
	assert.Zero(t, tree.Nodes[0].Children[0].SolvedCount)

	var after int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).Count(&after).Error)
	assert.Equal(t, before, after, "游客读取树不产生任何进度行")
}

func TestListPathsEvaluatesMyCompletedCount(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "list-eval-p1", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "列表进度", IsPublic: boolPtr(true)})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID},
	})

	findItem := func(items []learning.PathItem, id int64) *learning.PathItem {
		for i := range items {
			if items[i].ID == id {
				return &items[i]
			}
		}
		return nil
	}

	// 未 AC：myCompletedCount 为 0。
	items, err := e.svc.ListPaths(ctx, authorView, false, false)
	require.NoError(t, err)
	item := findItem(items, path.ID)
	require.NotNil(t, item)
	assert.Zero(t, item.MyCompletedCount)

	// AC 根节点题目后，ListPaths 先 EvaluateUser，myCompletedCount=1。
	e.addAccepted(t, e.author.ID, p1.ID)
	items, err = e.svc.ListPaths(ctx, authorView, false, false)
	require.NoError(t, err)
	item = findItem(items, path.ID)
	require.NotNil(t, item)
	assert.Equal(t, 1, item.MyCompletedCount)
}

func TestTreeCompletedLatchVisible(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	p1 := e.addProblem(t, "latch-tree-p1", true)
	p2 := e.addProblem(t, "latch-tree-p2", true)
	p3 := e.addProblem(t, "latch-tree-p3", true)

	path := e.createPath(t, authorView, learning.PathInput{Title: "锁存树", IsPublic: boolPtr(true)})
	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p2.ID}, RequiredCount: intPtr(2),
	})

	// 完成根节点（p1+p2）。
	e.addAccepted(t, e.author.ID, p1.ID)
	e.addAccepted(t, e.author.ID, p2.ID)
	tree, err := e.svc.Tree(ctx, authorView, path.ID)
	require.NoError(t, err)
	require.Len(t, tree.Nodes, 1)
	assert.Equal(t, learning.ProgressCompleted, tree.Nodes[0].Status)
	assert.Equal(t, 2, tree.Nodes[0].SolvedCount)

	// 改题并提高 required_count 后，已完成节点锁存：Tree 仍返回 completed +
	// 完成时的 solvedCount，同时返回当前 requiredCount。
	_, err = e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{p1.ID, p3.ID, p2.ID}, RequiredCount: intPtr(3),
	})
	require.NoError(t, err)

	tree, err = e.svc.Tree(ctx, authorView, path.ID)
	require.NoError(t, err)
	require.Len(t, tree.Nodes, 1)
	assert.Equal(t, learning.ProgressCompleted, tree.Nodes[0].Status)
	assert.Equal(t, 2, tree.Nodes[0].SolvedCount)
	assert.Equal(t, 3, tree.Nodes[0].RequiredCount)
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
