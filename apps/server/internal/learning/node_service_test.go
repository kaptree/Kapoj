package learning_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/learning"
)

func intPtr(v int) *int { return &v }

func nodePtr(id int64) *int64 { return &id }

func (e *pathEnv) createNode(t *testing.T, v learning.Viewer, pathID int64, in learning.NodeInput) *learning.NodeItem {
	t.Helper()
	item, err := e.svc.CreateNode(context.Background(), v, pathID, in)
	require.NoError(t, err)
	return item
}

func TestNodeValidation(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	path := e.createPath(t, authorView, learning.PathInput{Title: "校验路线"})
	otherPath := e.createPath(t, authorView, learning.PathInput{Title: "另一路线"})
	pubA := e.addProblem(t, "node-pub-a", true)
	pubB := e.addProblem(t, "node-pub-b", true)
	private := e.addProblem(t, "node-private", false)
	otherParent := e.createNode(t, authorView, otherPath.ID, learning.NodeInput{
		Title: "他路父节点", ProblemIDs: []int64{pubA.ID},
	})

	manyIDs := make([]int64, 51)
	for i := range manyIDs {
		manyIDs[i] = int64(i + 1)
	}

	cases := []struct {
		name string
		in   learning.NodeInput
	}{
		{"空标题", learning.NodeInput{ProblemIDs: []int64{pubA.ID}}},
		{"标题超长", learning.NodeInput{Title: strings.Repeat("题", 101), ProblemIDs: []int64{pubA.ID}}},
		{"描述超长", learning.NodeInput{
			Title: "节点", DescriptionMD: strings.Repeat("d", 50001), ProblemIDs: []int64{pubA.ID},
		}},
		{"无题目", learning.NodeInput{Title: "节点"}},
		{"题目过多", learning.NodeInput{Title: "节点", ProblemIDs: manyIDs}},
		{"题目重复", learning.NodeInput{Title: "节点", ProblemIDs: []int64{pubA.ID, pubA.ID}}},
		{"题目不存在", learning.NodeInput{Title: "节点", ProblemIDs: []int64{999999}}},
		{"题目未公开", learning.NodeInput{Title: "节点", ProblemIDs: []int64{private.ID}}},
		{"必做数过小", learning.NodeInput{
			Title: "节点", ProblemIDs: []int64{pubA.ID}, RequiredCount: intPtr(0),
		}},
		{"必做数过大", learning.NodeInput{
			Title: "节点", ProblemIDs: []int64{pubA.ID}, RequiredCount: intPtr(2),
		}},
		{"父节点跨路线", learning.NodeInput{
			Title: "节点", ParentID: &otherParent.ID, ProblemIDs: []int64{pubA.ID},
		}},
		{"父节点不存在", learning.NodeInput{
			Title: "节点", ParentID: nodePtr(888888), ProblemIDs: []int64{pubA.ID},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.CreateNode(ctx, authorView, path.ID, tc.in)
			requireAppError(t, err, 400, 40001)
		})
	}

	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: " 根节点 ", DescriptionMD: "根描述", ProblemIDs: []int64{pubA.ID, pubB.ID}, SortOrder: 9,
	})
	assert.Equal(t, "根节点", root.Title)
	assert.Equal(t, "根描述", root.DescriptionMD)
	assert.Nil(t, root.ParentID)
	assert.Equal(t, []int64{pubA.ID, pubB.ID}, root.ProblemIDs)
	assert.Equal(t, 2, root.RequiredCount, "required_count 默认题目数")
	assert.Equal(t, learning.ProgressActive, root.Status)
	assert.Equal(t, 0, root.SolvedCount)
	assert.Equal(t, 9, root.SortOrder)
	require.Len(t, root.Problems, 2)
	assert.Equal(t, learning.ProblemSummary{
		ProblemID: pubA.ID, Title: "题目-node-pub-a", Slug: "node-pub-a", Difficulty: 1,
	}, root.Problems[0])
	assert.Empty(t, root.Children)

	child := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子节点", ParentID: &root.ID, ProblemIDs: []int64{pubA.ID, pubB.ID},
		RequiredCount: intPtr(1), SortOrder: -3,
	})
	assert.Equal(t, root.ID, *child.ParentID)
	assert.Equal(t, 1, child.RequiredCount)
	assert.Equal(t, -3, child.SortOrder)
	assert.Equal(t, learning.ProgressLocked, child.Status)
	assert.Equal(t, 0, child.SolvedCount)

	// 更新时同样校验标题、题目与父节点。
	_, err := e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "", ProblemIDs: []int64{pubA.ID},
	})
	requireAppError(t, err, 400, 40001)

	_, err = e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根节点", ParentID: &root.ID, ProblemIDs: []int64{pubA.ID},
	})
	requireAppError(t, err, 400, 40001)

	_, err = e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根节点", ParentID: &child.ID, ProblemIDs: []int64{pubA.ID},
	})
	appErr := requireAppError(t, err, 400, 40001)
	assert.Contains(t, appErr.Message, "环")

	// 深层链：根为第 1 层，最多允许到第 8 层。
	depthParent := root.ID
	deepest := root
	for depth := 2; depth <= 8; depth++ {
		deepest = e.createNode(t, authorView, path.ID, learning.NodeInput{
			Title: fmt.Sprintf("第%d层", depth), ParentID: &depthParent, ProblemIDs: []int64{pubA.ID},
		})
		depthParent = deepest.ID
	}
	_, err = e.svc.CreateNode(ctx, authorView, path.ID, learning.NodeInput{
		Title: "第9层", ParentID: &depthParent, ProblemIDs: []int64{pubA.ID},
	})
	requireAppError(t, err, 400, 40001)

	// 移动整棵子树时，子树深度也必须满足上限。
	anchor := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "锚点", ProblemIDs: []int64{pubA.ID},
	})
	_, err = e.svc.UpdateNode(ctx, authorView, path.ID, root.ID, learning.NodeInput{
		Title: "根节点", ParentID: &anchor.ID, ProblemIDs: []int64{pubA.ID},
	})
	requireAppError(t, err, 400, 40001)
}

func TestNodeUpdateAndLeafOnlyDelete(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)

	path := e.createPath(t, authorView, learning.PathInput{Title: "删除路线"})
	otherPath := e.createPath(t, authorView, learning.PathInput{Title: "别的路线"})
	pubA := e.addProblem(t, "delete-pub-a", true)
	pubB := e.addProblem(t, "delete-pub-b", true)

	root := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "根", ProblemIDs: []int64{pubA.ID},
	})
	child := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "子", ParentID: &root.ID, ProblemIDs: []int64{pubB.ID},
	})
	grand := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "孙", ParentID: &child.ID, ProblemIDs: []int64{pubA.ID},
	})
	anchor := e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "锚点", ProblemIDs: []int64{pubA.ID},
	})

	updated, err := e.svc.UpdateNode(ctx, authorView, path.ID, child.ID, learning.NodeInput{
		Title: " 改名子节点 ", DescriptionMD: "新描述", ParentID: &anchor.ID,
		ProblemIDs: []int64{pubA.ID, pubB.ID}, SortOrder: -7,
	})
	require.NoError(t, err)
	assert.Equal(t, "改名子节点", updated.Title)
	assert.Equal(t, "新描述", updated.DescriptionMD)
	assert.Equal(t, anchor.ID, *updated.ParentID)
	assert.Equal(t, []int64{pubA.ID, pubB.ID}, updated.ProblemIDs)
	assert.Equal(t, 2, updated.RequiredCount, "未提交 requiredCount 时默认题目数")
	assert.Equal(t, -7, updated.SortOrder)

	// 可以移回根层级（parentId=null）。
	movedToRoot, err := e.svc.UpdateNode(ctx, authorView, path.ID, child.ID, learning.NodeInput{
		Title: "改名子节点", ProblemIDs: []int64{pubA.ID, pubB.ID},
	})
	require.NoError(t, err)
	assert.Nil(t, movedToRoot.ParentID)
	assert.Equal(t, learning.ProgressActive, movedToRoot.Status)
	var stored learning.LearningPathNode
	require.NoError(t, e.db.First(&stored, child.ID).Error)
	assert.Nil(t, stored.ParentID, "parent_id 需实际写回 NULL")

	movedBack, err := e.svc.UpdateNode(ctx, authorView, path.ID, child.ID, learning.NodeInput{
		Title: "改名子节点", ParentID: &anchor.ID, ProblemIDs: []int64{pubA.ID, pubB.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, anchor.ID, *movedBack.ParentID)

	// 节点必须属于路径；跨路径的节点 id 视为不存在。
	_, err = e.svc.UpdateNode(ctx, authorView, otherPath.ID, root.ID, learning.NodeInput{
		Title: "跨路径", ProblemIDs: []int64{pubA.ID},
	})
	requireAppError(t, err, 404, 40400)
	err = e.svc.DeleteNode(ctx, authorView, otherPath.ID, root.ID)
	requireAppError(t, err, 404, 40400)

	// 只有叶子可删除。
	err = e.svc.DeleteNode(ctx, authorView, path.ID, anchor.ID)
	appErr := requireAppError(t, err, 400, 40001)
	assert.Equal(t, "存在子节点，不能删除", appErr.Message)
	err = e.svc.DeleteNode(ctx, authorView, path.ID, child.ID)
	requireAppError(t, err, 400, 40001)

	// 节点进度随节点删除级联。
	require.NoError(t, e.db.Create(&learning.UserLearningProgress{
		UserID: e.author.ID, PathID: path.ID, NodeID: grand.ID,
		Status: learning.ProgressCompleted, SolvedCount: 1, UpdatedAt: time.Now(),
	}).Error)

	require.NoError(t, e.svc.DeleteNode(ctx, authorView, path.ID, grand.ID))
	var progressCount int64
	require.NoError(t, e.db.Model(&learning.UserLearningProgress{}).
		Where("node_id = ?", grand.ID).Count(&progressCount).Error)
	assert.Zero(t, progressCount)

	require.NoError(t, e.svc.DeleteNode(ctx, authorView, path.ID, child.ID))
	require.NoError(t, e.svc.DeleteNode(ctx, authorView, path.ID, anchor.ID))
	require.NoError(t, e.svc.DeleteNode(ctx, authorView, path.ID, root.ID))

	err = e.svc.DeleteNode(ctx, authorView, path.ID, root.ID)
	requireAppError(t, err, 404, 40400)
}

func TestNodePermissions(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView, otherView, adminView := viewerFor(e.author), viewerFor(e.other), viewerFor(e.admin)
	pub := e.addProblem(t, "perm-node", true)

	private := e.createPath(t, authorView, learning.PathInput{Title: "作者私有路线"})
	node := e.createNode(t, authorView, private.ID, learning.NodeInput{
		Title: "作者节点", ProblemIDs: []int64{pub.ID},
	})
	in := learning.NodeInput{Title: "其他人节点", ProblemIDs: []int64{pub.ID}}

	_, err := e.svc.CreateNode(ctx, otherView, private.ID, in)
	requireAppError(t, err, 404, 40400)
	_, err = e.svc.UpdateNode(ctx, otherView, private.ID, node.ID, in)
	requireAppError(t, err, 404, 40400)
	err = e.svc.DeleteNode(ctx, otherView, private.ID, node.ID)
	requireAppError(t, err, 404, 40400)

	_, err = e.svc.CreateNode(ctx, learning.Viewer{}, private.ID, in)
	requireAppError(t, err, 401, 40100)
	_, err = e.svc.UpdateNode(ctx, learning.Viewer{}, private.ID, node.ID, in)
	requireAppError(t, err, 401, 40100)
	err = e.svc.DeleteNode(ctx, learning.Viewer{}, private.ID, node.ID)
	requireAppError(t, err, 401, 40100)

	adminNode := e.createNode(t, adminView, private.ID, learning.NodeInput{
		Title: "管理员节点", ProblemIDs: []int64{pub.ID},
	})
	updated, err := e.svc.UpdateNode(ctx, adminView, private.ID, adminNode.ID, learning.NodeInput{
		Title: "管理员改名", ProblemIDs: []int64{pub.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, "管理员改名", updated.Title)
	require.NoError(t, e.svc.DeleteNode(ctx, adminView, private.ID, adminNode.ID))

	official := e.createPath(t, adminView, learning.PathInput{Title: "官方路线", IsOfficial: boolPtr(true)})
	officialNode := e.createNode(t, adminView, official.ID, learning.NodeInput{
		Title: "官方节点", ProblemIDs: []int64{pub.ID},
	})

	// 非管理员不是官方路线作者：不可见。
	_, err = e.svc.CreateNode(ctx, authorView, official.ID, in)
	requireAppError(t, err, 404, 40400)
	_, err = e.svc.UpdateNode(ctx, authorView, official.ID, officialNode.ID, in)
	requireAppError(t, err, 404, 40400)
	err = e.svc.DeleteNode(ctx, authorView, official.ID, officialNode.ID)
	requireAppError(t, err, 404, 40400)

	// 即使被记录为作者，非管理员也不能编辑官方路线节点。
	require.NoError(t, e.db.Model(&learning.LearningPath{}).Where("id = ?", official.ID).
		Update("created_by", e.author.ID).Error)
	_, err = e.svc.CreateNode(ctx, authorView, official.ID, in)
	requireAppError(t, err, 403, 40300)
	_, err = e.svc.UpdateNode(ctx, authorView, official.ID, officialNode.ID, in)
	requireAppError(t, err, 403, 40300)
	err = e.svc.DeleteNode(ctx, authorView, official.ID, officialNode.ID)
	requireAppError(t, err, 403, 40300)

	// inactive 路线只有管理员可以写节点。
	inactive := e.createPath(t, authorView, learning.PathInput{Title: "已下架路线"})
	require.NoError(t, e.svc.DeletePath(ctx, authorView, inactive.ID))
	_, err = e.svc.CreateNode(ctx, authorView, inactive.ID, in)
	requireAppError(t, err, 404, 40400)
	adminInactive := e.createNode(t, adminView, inactive.ID, learning.NodeInput{
		Title: "下架路线节点", ProblemIDs: []int64{pub.ID},
	})
	updated, err = e.svc.UpdateNode(ctx, adminView, inactive.ID, adminInactive.ID, learning.NodeInput{
		Title: "下架路线改名", ProblemIDs: []int64{pub.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, "下架路线改名", updated.Title)
}
