package activityapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

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
	activitySvc := activity.NewService(g, rdb)
	h := router.New(router.Deps{
		DB: g, Redis: rdb, Storage: store, Enqueuer: noopQueue{}, Game: gameSvc,
		Activity: activitySvc,
		Config: &config.Config{
			JWTSecret: testJWTSecret, AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
	})
	return &fx{t: t, h: h, db: g}
}

func (f *fx) user(username, status string) int64 {
	f.t.Helper()
	require.NoError(f.t, f.db.Exec(
		`INSERT INTO users (username, nickname, email, password_hash, role, rating, coins, bio, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'x', 'user', 1500, 0, '', ?, now(), now())`,
		username, "nick-"+username, username+"@example.com", status,
	).Error)
	var id int64
	require.NoError(f.t, f.db.Raw(`SELECT id FROM users WHERE username = ?`, username).Scan(&id).Error)
	return id
}

func (f *fx) token(userID int64) string {
	f.t.Helper()
	token, err := auth.NewTokenIssuer(testJWTSecret, 15*time.Minute).
		IssueAccess(&user.User{ID: userID, Role: user.RoleUser})
	require.NoError(f.t, err)
	return token
}

func (f *fx) do(method, path, token string) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBuffer(nil))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func responseData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object, got %T", resp["data"])
	return data
}

func pageData(t *testing.T, resp map[string]any) (float64, []any) {
	t.Helper()
	data := responseData(t, resp)
	items, ok := data["items"].([]any)
	require.True(t, ok, "expected data.items array, got %T", data["items"])
	return data["total"].(float64), items
}

func TestSignInAPIIIdempotentAndSummary(t *testing.T) {
	f := newFx(t)
	userID := f.user("signer", "active")
	token := f.token(userID)
	today := time.Now().In(time.Local).Format("2006-01-02")

	w, resp := f.do(http.MethodPost, "/api/activity/signin", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])
	assert.Equal(t, "未登录或登录已过期", resp["message"])

	w, resp = f.do(http.MethodPost, "/api/activity/signin", token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := responseData(t, resp)
	assert.Equal(t, true, data["awarded"])
	assert.EqualValues(t, 5, data["activityPoints"])
	assert.EqualValues(t, 5, data["coins"])
	assert.Equal(t, today, data["date"])

	var coins int64
	require.NoError(t, f.db.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error)
	assert.EqualValues(t, 5, coins)

	var transactions int64
	require.NoError(t, f.db.Table("coin_transactions").Where("user_id = ?", userID).Count(&transactions).Error)
	assert.EqualValues(t, 1, transactions)

	w, resp = f.do(http.MethodPost, "/api/activity/signin", token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = responseData(t, resp)
	assert.Equal(t, false, data["awarded"])
	assert.EqualValues(t, 0, data["activityPoints"])
	assert.EqualValues(t, 0, data["coins"])
	assert.Equal(t, today, data["date"])

	require.NoError(t, f.db.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error)
	assert.EqualValues(t, 5, coins, "重复签到不能重复发币")
	require.NoError(t, f.db.Table("coin_transactions").Where("user_id = ?", userID).Count(&transactions).Error)
	assert.EqualValues(t, 1, transactions)

	w, resp = f.do(http.MethodGet, "/api/activity/summary", token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = responseData(t, resp)
	assert.Equal(t, true, data["signedInToday"])
	assert.EqualValues(t, 5, data["todayPoints"])
	assert.EqualValues(t, 5, data["totalPoints"])

	w, resp = f.do(http.MethodGet, "/api/activity/summary", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40100, resp["code"])
}

func TestActivityLeaderboardsPublicAndEquippedIdentity(t *testing.T) {
	f := newFx(t)
	decorated := f.user("decorated", "active")
	plain := f.user("plain", "active")
	banned := f.user("banned", "banned")

	now := time.Now().In(time.Local)
	for _, log := range []activity.ActivityLog{
		{UserID: decorated, Action: activity.ActionContestJoin, Points: 20, RefType: "contest", RefID: 1, CreatedAt: now},
		{UserID: plain, Action: activity.ActionContestJoin, Points: 10, RefType: "contest", RefID: 2, CreatedAt: now},
		{UserID: banned, Action: activity.ActionContestJoin, Points: 30, RefType: "contest", RefID: 3, CreatedAt: now},
	} {
		require.NoError(t, f.db.Create(&log).Error)
	}
	for _, item := range []activity.SharedItem{
		{UserID: decorated, Type: "contest", RefID: 101, CreatedAt: now},
		{UserID: decorated, Type: "contest", RefID: 102, CreatedAt: now},
		{UserID: plain, Type: "contest", RefID: 103, CreatedAt: now},
		{UserID: banned, Type: "contest", RefID: 104, CreatedAt: now},
	} {
		require.NoError(t, f.db.Create(&item).Error)
	}

	// The leaderboard is public and the first request populates the base cache.
	w, resp := f.do(http.MethodGet, "/api/leaderboard/activity", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	row := items[0].(map[string]any)
	assert.EqualValues(t, 1, row["rank"])
	assert.EqualValues(t, decorated, row["userId"])
	assert.Equal(t, "decorated", row["username"])
	assert.Equal(t, "nick-decorated", row["nickname"])
	assert.EqualValues(t, 20, row["score"])
	assert.Equal(t, []any{}, row["equippedBadges"])
	assert.Contains(t, row, "equippedTitle")
	assert.Nil(t, row["equippedTitle"])
	assert.NotContains(t, row, "level")
	assert.NotContains(t, row, "email")
	assert.NotContains(t, row, "role")

	assert.EqualValues(t, 10, items[1].(map[string]any)["score"])

	// Equipping after the base page was cached must still be visible: decorated
	// identities are never cached with the base rows.
	require.NoError(t, f.db.Exec(
		`INSERT INTO badges (id, name, description, image_url, price_coins, category, is_limited, stock, sort_order, is_active, created_at, updated_at)
		 VALUES (7, '早鸟', '', '/api/badges/7/image', 0, '其他', false, 0, 1, true, now(), now())`,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO user_badges (user_id, badge_id, is_equipped, purchased_at) VALUES (?, 7, true, now())`,
		decorated,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO titles (id, name, color, unlock_condition_type, unlock_condition_json, price_coins, sort_order, is_active, created_at, updated_at)
		 VALUES (9, '卷王', '#7C3AED', 'rating', '{"rating":1}', 0, 1, true, now(), now())`,
	).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO user_titles (user_id, title_id, is_equipped, unlocked_at, purchased_at) VALUES (?, 9, true, now(), now())`,
		decorated,
	).Error)

	w, resp = f.do(http.MethodGet, "/api/leaderboard/activity", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, items = pageData(t, resp)
	require.Len(t, items, 2)
	row = items[0].(map[string]any)
	badges := row["equippedBadges"].([]any)
	require.Len(t, badges, 1)
	assert.EqualValues(t, 7, badges[0].(map[string]any)["id"])
	title := row["equippedTitle"].(map[string]any)
	assert.EqualValues(t, 9, title["id"])
	assert.Equal(t, "#7C3AED", title["color"])
	plainRow := items[1].(map[string]any)
	assert.Equal(t, []any{}, plainRow["equippedBadges"])
	assert.Nil(t, plainRow["equippedTitle"])

	w, resp = f.do(http.MethodGet, "/api/leaderboard/sharing", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items = pageData(t, resp)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	sharingRow := items[0].(map[string]any)
	assert.EqualValues(t, decorated, sharingRow["userId"])
	assert.EqualValues(t, 40, sharingRow["score"])
	assert.Len(t, sharingRow["equippedBadges"].([]any), 1)
	assert.NotContains(t, sharingRow, "level")
	assert.EqualValues(t, 20, items[1].(map[string]any)["score"])

	w, resp = f.do(http.MethodGet, "/api/leaderboard/activity?page=2&page_size=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items = pageData(t, resp)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 1)
	assert.EqualValues(t, 2, items[0].(map[string]any)["rank"])
	assert.EqualValues(t, plain, items[0].(map[string]any)["userId"])
}

func TestActivityLeaderboardRedisDownStillOK(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	f := newFxWithRedis(t, rdb)
	userID := f.user("alice", "active")
	require.NoError(t, f.db.Create(&activity.ActivityLog{
		UserID: userID, Action: activity.ActionSubmission, Points: 2,
		RefType: "submission", RefID: 1, CreatedAt: time.Now().In(time.Local),
	}).Error)
	mr.Close()

	w, resp := f.do(http.MethodGet, "/api/leaderboard/activity", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	total, items := pageData(t, resp)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.EqualValues(t, 2, items[0].(map[string]any)["score"])
}
