package learning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/learning"
)

func (e *pathEnv) setCoins(t *testing.T, userID, coins int64) {
	t.Helper()
	require.NoError(t, e.db.Exec(`UPDATE users SET coins = ? WHERE id = ?`, coins, userID).Error)
}

func (e *pathEnv) coins(t *testing.T, userID int64) int64 {
	t.Helper()
	var coins int64
	require.NoError(t, e.db.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error)
	return coins
}

func (e *pathEnv) countRows(t *testing.T, table string, pathID int64) int64 {
	t.Helper()
	var count int64
	require.NoError(t, e.db.Raw(`SELECT COUNT(*) FROM `+table+` WHERE path_id = ?`, pathID).Scan(&count).Error)
	return count
}

func TestPaidPathPurchaseIsIdempotent(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	private := e.addProblem(t, "paid-private", false)

	path := e.createPath(t, authorView, learning.PathInput{
		Title: "付费路线", IsPublic: boolPtr(true), PriceCoins: 100,
	})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{private.ID},
	})
	e.setCoins(t, e.other.ID, 250)

	first, err := e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(150), first.Coins)
	assert.True(t, first.Path.Purchased)
	assert.EqualValues(t, 1, first.Path.ParticipantCount)
	assert.EqualValues(t, 1, e.countRows(t, "learning_path_purchases", path.ID))

	second, err := e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(150), second.Coins, "repeat purchase must not charge again")
	assert.EqualValues(t, 1, second.Path.ParticipantCount)
	assert.EqualValues(t, 1, e.countRows(t, "learning_path_purchases", path.ID))
	var coinRows int64
	require.NoError(t, e.db.Raw(
		`SELECT COUNT(*) FROM coin_transactions WHERE user_id = ? AND reason = ? AND ref_id = ?`,
		e.other.ID, "learning_path_purchase", path.ID).Scan(&coinRows).Error)
	assert.EqualValues(t, 1, coinRows)
}

func TestPaidPathPurchaseRollsBackWhenBalanceIsLow(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	private := e.addProblem(t, "paid-balance", false)
	path := e.createPath(t, authorView, learning.PathInput{
		Title: "余额不足", IsPublic: boolPtr(true), PriceCoins: 100,
	})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{private.ID},
	})
	e.setCoins(t, e.other.ID, 50)

	_, err := e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	requireAppError(t, err, 400, 40001)
	assert.Equal(t, int64(50), e.coins(t, e.other.ID))
	assert.Zero(t, e.countRows(t, "learning_path_purchases", path.ID))
}

func TestAuthorAndFreePathPurchasesAreFree(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	private := e.addProblem(t, "paid-author", false)
	paid := e.createPath(t, authorView, learning.PathInput{
		Title: "作者免费", IsPublic: boolPtr(true), PriceCoins: 100,
	})
	e.createNode(t, authorView, paid.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{private.ID},
	})
	e.setCoins(t, e.author.ID, 1000)

	got, err := e.svc.Purchase(ctx, authorView, paid.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1000), got.Coins)
	assert.False(t, got.Path.Purchased)
	assert.Zero(t, e.countRows(t, "learning_path_purchases", paid.ID))

	free := e.createPath(t, authorView, learning.PathInput{Title: "免费", IsPublic: boolPtr(true)})
	e.setCoins(t, e.other.ID, 10)
	got, err = e.svc.Purchase(ctx, viewerFor(e.other), free.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(10), got.Coins)
	assert.False(t, got.Path.Purchased)
}

func TestRecommendationsRequireAccessAndAreToggleable(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	private := e.addProblem(t, "paid-recommend", false)
	path := e.createPath(t, authorView, learning.PathInput{
		Title: "推荐路线", IsPublic: boolPtr(true), PriceCoins: 10,
	})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{private.ID},
	})
	e.setCoins(t, e.other.ID, 100)

	_, err := e.svc.Recommend(ctx, viewerFor(e.other), path.ID)
	requireAppError(t, err, 400, 40001)
	_, err = e.svc.Recommend(ctx, authorView, path.ID)
	requireAppError(t, err, 400, 40001)

	_, err = e.svc.Purchase(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	item, err := e.svc.Recommend(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.True(t, item.RecommendedByMe)
	assert.EqualValues(t, 1, item.RecommendCount)

	again, err := e.svc.Recommend(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, again.RecommendCount)

	removed, err := e.svc.Unrecommend(ctx, viewerFor(e.other), path.ID)
	require.NoError(t, err)
	assert.False(t, removed.RecommendedByMe)
	assert.Zero(t, removed.RecommendCount)
}

func TestParticipantCountUnionsPurchasesAndProgress(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	pub := e.addProblem(t, "participant-public", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "参与", IsPublic: boolPtr(true)})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{pub.ID},
	})
	require.NoError(t, e.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`, e.other.ID, pub.ID).Error)
	require.NoError(t, e.svc.EvaluateUser(ctx, e.other.ID))
	item, err := e.svc.FindPath(ctx, authorView, path.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, item.ParticipantCount)

	// Buying the same path later must not double count the same user.
	require.NoError(t, e.db.Exec(
		`INSERT INTO learning_path_purchases (user_id, path_id, price_paid, created_at)
		 VALUES (?, ?, 0, now())`, e.other.ID, path.ID).Error)
	require.NoError(t, learning.NewRepository(e.db).
		RecalculateParticipantCount(ctx, []int64{path.ID}))
	var count int
	require.NoError(t, e.db.Raw(`SELECT participant_count FROM learning_paths WHERE id = ?`, path.ID).
		Scan(&count).Error)
	assert.Equal(t, 1, count)
}

func TestPathSortOrderRules(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	adminView := viewerFor(e.admin)

	first := e.createPath(t, authorView, learning.PathInput{Title: "第一", SortOrder: 99})
	second := e.createPath(t, authorView, learning.PathInput{Title: "第二", SortOrder: 1})
	assert.Greater(t, second.SortOrder, first.SortOrder, "user paths auto-append")

	updated, err := e.svc.UpdatePath(ctx, authorView, first.ID, learning.PathInput{
		Title: "第一改", SortOrder: 50,
	})
	require.NoError(t, err)
	assert.Equal(t, first.SortOrder, updated.SortOrder, "normal users cannot change sort order")

	updated, err = e.svc.UpdatePath(ctx, adminView, first.ID, learning.PathInput{
		Title: "第一改", SortOrder: 50,
	})
	require.NoError(t, err)
	assert.Equal(t, 50, updated.SortOrder)
	assert.True(t, updated.CanEditSort)

	official := e.createPath(t, adminView, learning.PathInput{Title: "官方", IsOfficial: boolPtr(true), SortOrder: 100})
	page, err := e.svc.SearchPaths(ctx, authorView, learning.ListQuery{})
	require.NoError(t, err)
	require.NotEmpty(t, page.Items)
	assert.Equal(t, official.ID, page.Items[0].ID, "official paths sort before user paths")
	assert.False(t, page.Items[0].CanEdit)
	assert.False(t, page.Items[0].CanEditSort)
}

func TestPathSearchFiltersAndScope(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	now := time.Now()
	old := e.createPath(t, authorView, learning.PathInput{Title: "100% 入门", DescriptionMD: "基础课"})
	recent := e.createPath(t, authorView, learning.PathInput{Title: "进阶图论", DescriptionMD: "最短路"})
	require.NoError(t, e.db.Exec(`UPDATE learning_paths SET created_at = ? WHERE id = ?`, now.Add(-48*time.Hour), old.ID).Error)
	require.NoError(t, e.db.Exec(`UPDATE learning_paths SET created_at = ? WHERE id = ?`, now, recent.ID).Error)

	page, err := e.svc.SearchPaths(ctx, authorView, learning.ListQuery{Keyword: "100%"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, old.ID, page.Items[0].ID, "% must match literally")

	page, err = e.svc.SearchPaths(ctx, authorView, learning.ListQuery{Keyword: "图论"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, recent.ID, page.Items[0].ID)

	from := now.Add(-24 * time.Hour)
	page, err = e.svc.SearchPaths(ctx, authorView, learning.ListQuery{CreatedFrom: &from})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, recent.ID, page.Items[0].ID)

	page, err = e.svc.SearchPaths(ctx, authorView, learning.ListQuery{Scope: "official"})
	require.NoError(t, err)
	assert.Empty(t, page.Items)

	page, err = e.svc.SearchPaths(ctx, authorView, learning.ListQuery{Scope: "mine", PageSize: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	assert.Len(t, page.Items, 1)

	_, err = e.svc.SearchPaths(ctx, authorView, learning.ListQuery{Scope: "bogus"})
	requireAppError(t, err, 400, 40001)
}

func TestPaidPathCannotReferencePublicProblems(t *testing.T) {
	e := newPathEnv(t)
	ctx := context.Background()
	authorView := viewerFor(e.author)
	pub := e.addProblem(t, "paid-conflict", true)
	path := e.createPath(t, authorView, learning.PathInput{Title: "改付费", IsPublic: boolPtr(true)})
	e.createNode(t, authorView, path.ID, learning.NodeInput{
		Title: "节点", ProblemIDs: []int64{pub.ID},
	})

	_, err := e.svc.UpdatePath(ctx, authorView, path.ID, learning.PathInput{
		Title: "改付费", IsPublic: boolPtr(true), PriceCoins: 10,
	})
	requireAppError(t, err, 400, 40001)

	require.NoError(t, e.db.Exec(`UPDATE problems SET is_public = false WHERE id = ?`, pub.ID).Error)
	updated, err := e.svc.UpdatePath(ctx, authorView, path.ID, learning.PathInput{
		Title: "改付费", IsPublic: boolPtr(true), PriceCoins: 10,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 10, updated.PriceCoins)
}
