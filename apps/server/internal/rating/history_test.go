package rating_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
)

func TestRatingHistoryOrderAnd404(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now().UTC().Truncate(time.Second)
	oldContest := e.contest("Old Contest", false, now.Add(-5*time.Hour), now.Add(-4*time.Hour))
	firstTie := e.contest("First Tie", false, now.Add(-4*time.Hour), now.Add(-3*time.Hour))
	secondTie := e.contest("Second Tie", false, now.Add(-3*time.Hour), now.Add(-2*time.Hour))

	oldID, firstID, secondID := oldContest.ID, firstTie.ID, secondTie.ID
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &oldID, Kind: rating.HistoryContest,
		Rank: 2, RatingBefore: 1500, RatingAfter: 1480, CreatedAt: now.Add(-5 * time.Hour),
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &firstID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1480, RatingAfter: 1512, CreatedAt: now.Add(-4 * time.Hour),
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &secondID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1512, RatingAfter: 1544, CreatedAt: now.Add(-3 * time.Hour),
	}).Error)
	// A decay row must now appear alongside contest rows.
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, Kind: rating.HistoryDecay, PeriodKey: "2026-01",
		Rank: 0, RatingBefore: 1544, RatingAfter: 1500, CreatedAt: now,
	}).Error)
	// Another user's history must not leak into userA's page.
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userB, ContestID: &oldID, Kind: rating.HistoryContest,
		Rank: 3, RatingBefore: 1500, RatingAfter: 1490, CreatedAt: now.Add(-2 * time.Hour),
	}).Error)

	first, err := e.svc.RatingHistory(context.Background(), userA, 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 4, first.Total, "total counts contest and decay rows")
	require.Len(t, first.Items, 2)
	assert.Equal(t, rating.HistoryDecay, first.Items[0].Kind, "decay is newest")
	assert.EqualValues(t, 0, first.Items[0].ContestID)
	assert.Equal(t, "Rating 衰减", first.Items[0].ContestTitle)
	assert.Equal(t, 0, first.Items[0].Rank)
	assert.Equal(t, "2026-01", first.Items[0].PeriodKey)
	assert.Equal(t, -44, first.Items[0].Delta)

	assert.Equal(t, secondID, first.Items[1].ContestID, "created_at desc then id desc")
	assert.Equal(t, "Second Tie", first.Items[1].ContestTitle)
	assert.Equal(t, 1, first.Items[1].Rank)
	assert.Equal(t, 1512, first.Items[1].RatingBefore)
	assert.Equal(t, 1544, first.Items[1].RatingAfter)
	assert.Equal(t, 32, first.Items[1].Delta)
	assert.Equal(t, now.Add(-3*time.Hour), first.Items[1].CreatedAt)
	assert.Equal(t, "", first.Items[1].PeriodKey, "contest rows have an empty period key")

	second, err := e.svc.RatingHistory(context.Background(), userA, 2, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 4, second.Total)
	require.Len(t, second.Items, 2)
	assert.Equal(t, firstID, second.Items[0].ContestID)
	assert.Equal(t, "First Tie", second.Items[0].ContestTitle)
	assert.Equal(t, 32, second.Items[0].Delta)
	assert.Equal(t, oldID, second.Items[1].ContestID)
	assert.Equal(t, "Old Contest", second.Items[1].ContestTitle)
	assert.Equal(t, -20, second.Items[1].Delta)

	_, err = e.svc.RatingHistory(context.Background(), 999, 1, 20)
	assert.ErrorIs(t, err, httpx.ErrNotFound)
	_, err = e.svc.RatingHistory(context.Background(), 0, 1, 20)
	assert.ErrorIs(t, err, httpx.ErrNotFound)

	empty, err := e.svc.RatingHistory(context.Background(), userC, 1, 20)
	require.NoError(t, err, "an existing user without history is not a 404")
	assert.EqualValues(t, 0, empty.Total)
	assert.Empty(t, empty.Items)
	assert.NotNil(t, empty.Items)
}

func TestRatingHistoryAllKinds(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now().UTC().Truncate(time.Second)
	public := e.contest("Public Contest", false, now.Add(-4*time.Hour), now.Add(-3*time.Hour))
	publicID := public.ID
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &publicID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1500, RatingAfter: 1532, CreatedAt: now.Add(-3 * time.Hour),
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, Kind: rating.HistoryDecay, PeriodKey: "2026-02",
		Rank: 0, RatingBefore: 1532, RatingAfter: 1526, CreatedAt: now.Add(-2 * time.Hour),
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, Kind: rating.HistorySeasonReset, PeriodKey: "2026-01-01",
		Rank: 0, RatingBefore: 1526, RatingAfter: 1518, CreatedAt: now.Add(-time.Hour),
	}).Error)

	page, err := e.svc.RatingHistory(context.Background(), userA, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total, "pagination total includes all kinds")
	require.Len(t, page.Items, 3)

	seasonReset := page.Items[0]
	assert.Equal(t, rating.HistorySeasonReset, seasonReset.Kind)
	assert.EqualValues(t, 0, seasonReset.ContestID)
	assert.Equal(t, "赛季重置", seasonReset.ContestTitle)
	assert.Equal(t, 0, seasonReset.Rank)
	assert.Equal(t, "2026-01-01", seasonReset.PeriodKey)
	assert.Equal(t, -8, seasonReset.Delta)

	decay := page.Items[1]
	assert.Equal(t, rating.HistoryDecay, decay.Kind)
	assert.EqualValues(t, 0, decay.ContestID)
	assert.Equal(t, "Rating 衰减", decay.ContestTitle)
	assert.Equal(t, 0, decay.Rank)
	assert.Equal(t, "2026-02", decay.PeriodKey)
	assert.Equal(t, -6, decay.Delta)

	contestItem := page.Items[2]
	assert.Equal(t, rating.HistoryContest, contestItem.Kind)
	assert.Equal(t, publicID, contestItem.ContestID)
	assert.Equal(t, "Public Contest", contestItem.ContestTitle)
	assert.Equal(t, 1, contestItem.Rank)
	assert.Equal(t, "", contestItem.PeriodKey)
	assert.Equal(t, 32, contestItem.Delta)
}

func TestRatingHistoryMasksPrivateContest(t *testing.T) {
	e := newTestEnv(t)
	now := time.Now().UTC().Truncate(time.Second)
	public := e.contest("Public Contest", true, now.Add(-4*time.Hour), now.Add(-3*time.Hour))
	private := e.contest("Secret Invitational", true, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	require.NoError(t, e.db.Exec(`UPDATE contests SET is_public = false WHERE id = ?`, private.ID).Error)

	publicID, privateID := public.ID, private.ID
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &publicID, Kind: rating.HistoryContest,
		Rank: 1, RatingBefore: 1500, RatingAfter: 1532, CreatedAt: now.Add(-2 * time.Hour),
	}).Error)
	require.NoError(t, e.db.Create(&rating.RatingHistory{
		UserID: userA, ContestID: &privateID, Kind: rating.HistoryContest,
		Rank: 2, RatingBefore: 1480, RatingAfter: 1512, CreatedAt: now.Add(-time.Hour),
	}).Error)

	page, err := e.svc.RatingHistory(context.Background(), userA, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 2)

	// The participant still sees that a private contest changed their rating,
	// but neither its id nor its title is exposed.
	privateItem := page.Items[0]
	assert.EqualValues(t, 0, privateItem.ContestID)
	assert.Equal(t, "私密比赛", privateItem.ContestTitle)
	assert.Equal(t, 2, privateItem.Rank)
	assert.Equal(t, 1480, privateItem.RatingBefore)
	assert.Equal(t, 1512, privateItem.RatingAfter)
	assert.Equal(t, 32, privateItem.Delta)
	assert.Equal(t, now.Add(-time.Hour), privateItem.CreatedAt)

	publicItem := page.Items[1]
	assert.Equal(t, publicID, publicItem.ContestID)
	assert.Equal(t, "Public Contest", publicItem.ContestTitle)
	assert.Equal(t, 32, publicItem.Delta)
}
