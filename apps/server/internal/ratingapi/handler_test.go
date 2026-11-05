package ratingapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/ratingapi"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t  *testing.T
	h  http.Handler
	db *gorm.DB
}

func newFx(t *testing.T) *fx {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return newFxWithRedis(t, rdb)
}

func newFxWithRedis(t *testing.T, rdb *redis.Client) *fx {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	gameSvc := gamification.NewServiceWithOptions(
		gamification.NewRepository(g), gamification.ServiceOptions{MaxEquippedBadges: 3},
	)
	ratingSvc := rating.NewService(g, nil, rdb, rating.Options{})
	h := router.New(router.Deps{
		DB: g, Redis: rdb, Storage: store, Enqueuer: noopQueue{}, Game: gameSvc, Rating: ratingSvc,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
	})
	return &fx{t: t, h: h, db: g}
}

func (f *fx) do(method, path string) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBuffer(nil))
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (f *fx) user(username string, ratingValue int) int64 {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, rating, coins, bio, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'user', ?, 0, '', 'active', now(), now())`,
		username, "nick-"+username, username+"@example.com", ratingValue,
	).Error)
	var id int64
	require.NoError(f.t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func (f *fx) contest(title string) int64 {
	f.t.Helper()
	now := time.Now().UTC()
	c := &contest.Contest{
		Title: title, StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour),
		Mode: contest.ModeICPC, IsPublic: true, RegisterType: contest.RegisterOpen,
	}
	require.NoError(f.t, f.db.Create(c).Error)
	return c.ID
}

func (f *fx) history(userID, contestID int64, rank, before, after int, createdAt time.Time) {
	f.t.Helper()
	require.NoError(f.t, f.db.Create(&rating.RatingHistory{
		UserID: userID, ContestID: &contestID, Kind: rating.HistoryContest,
		Rank: rank, RatingBefore: before, RatingAfter: after, CreatedAt: createdAt,
	}).Error)
}

func pageData(t *testing.T, resp map[string]any) (float64, []any) {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object, got %T", resp["data"])
	items, ok := data["items"].([]any)
	require.True(t, ok, "expected data.items array, got %T", data["items"])
	return data["total"].(float64), items
}

func TestRatingLeaderboardPublicPagination(t *testing.T) {
	f := newFx(t)
	low := f.user("low", 1500)
	highA := f.user("higha", 1700)
	highB := f.user("highb", 1700)

	w, resp := f.do(http.MethodGet, "/api/leaderboard/rating?page=1&page_size=2")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.EqualValues(t, 1, first["rank"])
	assert.EqualValues(t, highA, first["userId"])
	assert.Equal(t, "higha", first["username"])
	assert.Equal(t, "nick-higha", first["nickname"])
	assert.EqualValues(t, 1700, first["score"])
	level := first["level"].(map[string]any)
	assert.Equal(t, "blue", level["key"])
	assert.Equal(t, "蓝名", level["name"])
	assert.Equal(t, "#0000FF", level["color"])
	assert.Equal(t, "", level["accentColor"])
	assert.Equal(t, []any{}, first["equippedBadges"])
	assert.Contains(t, first, "equippedTitle")
	assert.Nil(t, first["equippedTitle"])
	assert.NotContains(t, first, "email")
	assert.NotContains(t, first, "role")

	assert.EqualValues(t, 2, items[1].(map[string]any)["rank"])
	assert.EqualValues(t, highB, items[1].(map[string]any)["userId"])

	w, resp = f.do(http.MethodGet, "/api/leaderboard/rating?page=2&page_size=2")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, items = pageData(t, resp)
	require.Len(t, items, 1)
	assert.EqualValues(t, 3, items[0].(map[string]any)["rank"])
	assert.EqualValues(t, low, items[0].(map[string]any)["userId"])

	// The leaderboard is public: no Authorization header is required.
	w, _ = f.do(http.MethodGet, "/api/leaderboard/rating")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRatingLeaderboardEquippedIdentity(t *testing.T) {
	f := newFx(t)
	userID := f.user("decorated", 2600)
	require.NoError(t, f.db.Exec(
		`INSERT INTO badges (id, name, description, image_url, price_coins, category, is_limited, stock, sort_order, is_active, created_at, updated_at)
		 VALUES (7, '早鸟', '', '/api/badges/7/image', 0, '其他', false, 0, 1, true, now(), now())`,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO user_badges (user_id, badge_id, is_equipped, purchased_at) VALUES (?, 7, true, now())`,
		userID,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO titles (id, name, color, unlock_condition_type, unlock_condition_json, price_coins, sort_order, is_active, created_at, updated_at)
		 VALUES (9, '卷王', '#7C3AED', 'rating', '{"rating":1}', 0, 1, true, now(), now())`,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO user_titles (user_id, title_id, is_equipped, unlocked_at, purchased_at) VALUES (?, 9, true, now(), now())`,
		userID,
	).Error)

	w, resp := f.do(http.MethodGet, "/api/leaderboard/rating")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, items := pageData(t, resp)
	require.Len(t, items, 1)
	row := items[0].(map[string]any)

	badges := row["equippedBadges"].([]any)
	require.Len(t, badges, 1)
	assert.EqualValues(t, 7, badges[0].(map[string]any)["id"])
	assert.Equal(t, "早鸟", badges[0].(map[string]any)["name"])
	title := row["equippedTitle"].(map[string]any)
	assert.EqualValues(t, 9, title["id"])
	assert.Equal(t, "卷王", title["name"])
	assert.Equal(t, "#7C3AED", title["color"])
	assert.Equal(t, "black_red", row["level"].(map[string]any)["key"])
	assert.Equal(t, "#FF0000", row["level"].(map[string]any)["accentColor"])
}

func TestRatingLeaderboardWithoutIdentityLookup(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, g.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, rating, created_at, updated_at)
		 VALUES ('solo', 'solo', 'solo@example.com', 'x', 1500, now(), now())`,
	).Error)
	svc := rating.NewService(g, nil, nil, rating.Options{})
	r := gin.New()
	ratingapi.RegisterRoutes(r.Group("/api"), svc)

	req := httptest.NewRequest(http.MethodGet, "/api/leaderboard/rating", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	_, items := pageData(t, resp)
	require.Len(t, items, 1)
	row := items[0].(map[string]any)
	assert.Equal(t, []any{}, row["equippedBadges"])
	assert.Contains(t, row, "equippedTitle")
	assert.Nil(t, row["equippedTitle"])
}

func TestRatingLeaderboardRedisDownStillOK(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	f := newFxWithRedis(t, rdb)
	f.user("alice", 1600)
	mr.Close()

	w, resp := f.do(http.MethodGet, "/api/leaderboard/rating")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.EqualValues(t, 1600, items[0].(map[string]any)["score"])
}

func TestRatingHistoryPublicAndNotFound(t *testing.T) {
	f := newFx(t)
	userID := f.user("historian", 1500)
	now := time.Now().UTC().Truncate(time.Second)
	oldID := f.contest("第一场")
	newID := f.contest("第二场")
	f.history(userID, oldID, 2, 1500, 1480, now.Add(-time.Hour))
	f.history(userID, newID, 1, 1480, 1512, now)

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/rating-history?page=1&page_size=20", userID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.EqualValues(t, newID, first["contestId"])
	assert.Equal(t, "第二场", first["contestTitle"])
	assert.EqualValues(t, 1, first["rank"])
	assert.EqualValues(t, 1480, first["ratingBefore"])
	assert.EqualValues(t, 1512, first["ratingAfter"])
	assert.EqualValues(t, 32, first["delta"])
	assert.NotContains(t, first, "id")
	assert.NotContains(t, first, "userId")

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/rating-history?page=2&page_size=1", userID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, items = pageData(t, resp)
	require.Len(t, items, 1)
	assert.EqualValues(t, oldID, items[0].(map[string]any)["contestId"])
	assert.EqualValues(t, -20, items[0].(map[string]any)["delta"])

	for _, path := range []string{
		"/api/users/999/rating-history",
		"/api/users/abc/rating-history",
		"/api/users/0/rating-history",
		"/api/users/-1/rating-history",
	} {
		w, resp = f.do(http.MethodGet, path)
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.EqualValues(t, 40400, resp["code"], path)
	}
}
