package companionapi_test

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
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t   *testing.T
	h   http.Handler
	db  *gorm.DB
	svc *companion.Service
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := companion.NewService(companion.NewGormRepository(g))
	h := router.New(router.Deps{
		DB: g, Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Storage: store,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
		Enqueuer:  noopQueue{},
		Companion: svc,
	})
	return &fx{t: t, h: h, db: g, svc: svc}
}

func (f *fx) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (f *fx) register(name string) (int64, string) {
	f.t.Helper()
	w, _ := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	w, resp := f.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"account": name, "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	tok := data["accessToken"].(string)
	userID := int64(data["user"].(map[string]any)["id"].(float64))
	return userID, tok
}

func dataOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "expected data object, got %T", resp["data"])
	return data
}

func eventItems(t *testing.T, resp map[string]any) []any {
	t.Helper()
	items, ok := dataOf(t, resp)["items"].([]any)
	require.True(t, ok, "expected items array, got %T", dataOf(t, resp)["items"])
	return items
}

// 1. A fresh user gets the documented defaults without any event having fired.
func TestCompanionStateDefaults(t *testing.T) {
	f := newFx(t)
	_, tok := f.register("alice")

	w, resp := f.do(http.MethodGet, "/api/companion", tok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataOf(t, resp)

	assert.EqualValues(t, 0, data["bond"])
	assert.EqualValues(t, 0, data["level"])
	assert.Equal(t, "陌生", data["levelName"])
	assert.EqualValues(t, 20, data["nextThreshold"])
	assert.EqualValues(t, 0, data["dailyGain"])
	assert.EqualValues(t, 15, data["dailyCap"])
	assert.Equal(t, "normal", data["activityLevel"])
	assert.Equal(t, true, data["dockEnabled"])
	assert.Equal(t, false, data["publicProfile"])
	assert.Equal(t, "", data["equippedSkin"])
	assert.Empty(t, data["unlockedSkins"])
	assert.EqualValues(t, 0, data["unreadCount"])
	assert.Empty(t, data["todayEvents"])
}

// 2. Pet awards bond once per local day and is idempotent on retry.
func TestCompanionPetAwardsOnce(t *testing.T) {
	f := newFx(t)
	userID, tok := f.register("bob")

	w, resp := f.do(http.MethodPost, "/api/companion/pet", tok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	first := dataOf(t, resp)
	assert.Equal(t, true, first["awarded"])
	assert.EqualValues(t, 1, first["bond"])
	assert.EqualValues(t, 0, first["level"])

	w, resp = f.do(http.MethodPost, "/api/companion/pet", tok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	second := dataOf(t, resp)
	assert.Equal(t, false, second["awarded"], "second pet on the same day must not award")
	assert.EqualValues(t, 1, second["bond"])

	w, resp = f.do(http.MethodGet, "/api/companion", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	state := dataOf(t, resp)
	assert.EqualValues(t, 1, state["bond"])
	assert.EqualValues(t, 1, state["dailyGain"])
	assert.EqualValues(t, 1, state["unreadCount"])

	var count int64
	require.NoError(t, f.db.Model(&companion.Event{}).
		Where("user_id = ? AND type = ?", userID, "pet").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// 3. Settings persist and are visible on the next state read.
func TestCompanionSettingsPersist(t *testing.T) {
	f := newFx(t)
	_, tok := f.register("carol")

	w, resp := f.do(http.MethodPost, "/api/companion/settings", tok, map[string]any{
		"activityLevel": "quiet", "dockEnabled": false, "publicProfile": true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataOf(t, resp)
	assert.Equal(t, "quiet", data["activityLevel"])
	assert.Equal(t, false, data["dockEnabled"])
	assert.Equal(t, true, data["publicProfile"])

	w, resp = f.do(http.MethodGet, "/api/companion", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	state := dataOf(t, resp)
	assert.Equal(t, "quiet", state["activityLevel"])
	assert.Equal(t, false, state["dockEnabled"])
	assert.Equal(t, true, state["publicProfile"])

	// Omitted fields keep their stored value instead of resetting to zero.
	w, resp = f.do(http.MethodPost, "/api/companion/settings", tok, map[string]any{
		"publicProfile": false,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	partial := dataOf(t, resp)
	assert.Equal(t, "quiet", partial["activityLevel"])
	assert.Equal(t, false, partial["dockEnabled"])
	assert.Equal(t, false, partial["publicProfile"])

	// Unknown activity levels are rejected rather than persisted.
	w, resp = f.do(http.MethodPost, "/api/companion/settings", tok, map[string]any{
		"activityLevel": "chaotic",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// Skins are limited to the 40-character column width (counted in runes).
	w, resp = f.do(http.MethodPost, "/api/companion/settings", tok, map[string]any{
		"equippedSkin": strings.Repeat("皮", 41),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodGet, "/api/companion", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "", dataOf(t, resp)["equippedSkin"])

	skin := strings.Repeat("皮", 40)
	w, resp = f.do(http.MethodPost, "/api/companion/settings", tok, map[string]any{
		"equippedSkin": skin,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, skin, dataOf(t, resp)["equippedSkin"])
	assert.Equal(t, "quiet", dataOf(t, resp)["activityLevel"], "skin patch keeps other fields")
}

// 4. Unread events are listed newest-last with a clamped limit, and marking them
// read empties the list; ids owned by other users are ignored.
func TestCompanionEventsListAndMarkRead(t *testing.T) {
	f := newFx(t)
	userID, tok := f.register("dave")
	ctx := context.Background()
	require.NoError(t, f.svc.RecordSignIn(ctx, userID, "2026-11-17"))
	require.NoError(t, f.svc.RecordAchievement(ctx, userID, 42, time.Now()))
	require.NoError(t, f.svc.RecordTaskClaim(ctx, userID, 7, time.Now()))

	w, resp := f.do(http.MethodGet, "/api/companion/events?unread=1", tok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := eventItems(t, resp)
	require.Len(t, items, 3)
	first := items[0].(map[string]any)
	assert.Equal(t, "signin", first["type"])
	assert.EqualValues(t, 3, first["bond"])
	assert.NotNil(t, first["payload"])

	ids := make([]int64, 0, len(items))
	for _, raw := range items {
		ids = append(ids, int64(raw.(map[string]any)["id"].(float64)))
	}

	// limit is clamped to 1..20; a bad value falls back to the 20 default.
	w, resp = f.do(http.MethodGet, "/api/companion/events?limit=1", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, eventItems(t, resp), 1)
	w, resp = f.do(http.MethodGet, "/api/companion/events?limit=0", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, eventItems(t, resp), 1)
	w, resp = f.do(http.MethodGet, "/api/companion/events?limit=999", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, eventItems(t, resp), 3)
	w, resp = f.do(http.MethodGet, "/api/companion/events?limit=nope", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, eventItems(t, resp), 3)

	// Another user cannot mark Dave's events read.
	_, otherTok := f.register("erin")
	w, _ = f.do(http.MethodPost, "/api/companion/events/read", otherTok, map[string]any{
		"ids": ids,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodGet, "/api/companion/events", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, eventItems(t, resp), 3)

	// Marking one read removes only that one.
	w, _ = f.do(http.MethodPost, "/api/companion/events/read", tok, map[string]any{
		"ids": ids[:1],
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodGet, "/api/companion/events", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	remaining := eventItems(t, resp)
	require.Len(t, remaining, 2)
	assert.NotEqualValues(t, ids[0], remaining[0].(map[string]any)["id"])

	// Marking the rest empties the list and the state's unread count.
	w, _ = f.do(http.MethodPost, "/api/companion/events/read", tok, map[string]any{
		"ids": ids[1:],
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodGet, "/api/companion/events", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, eventItems(t, resp))
	assert.NotNil(t, dataOf(t, resp)["items"], "empty list must serialize as [], not null")

	w, resp = f.do(http.MethodGet, "/api/companion", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 0, dataOf(t, resp)["unreadCount"])
}

// 5. Every companion endpoint requires authentication.
func TestCompanionRoutesRequireAuth(t *testing.T) {
	f := newFx(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/companion", nil},
		{http.MethodGet, "/api/companion/events", nil},
		{http.MethodPost, "/api/companion/events/read", map[string]any{"ids": []int64{1}}},
		{http.MethodPost, "/api/companion/settings", map[string]any{"activityLevel": "quiet"}},
		{http.MethodPost, "/api/companion/pet", nil},
	}
	for _, tc := range cases {
		w, resp := f.do(tc.method, tc.path, "", tc.body)
		assert.Equal(t, http.StatusUnauthorized, w.Code, tc.path)
		assert.EqualValues(t, 40100, resp["code"], tc.path)
	}
}

// 6. The public summary is privacy-aware and readable without a token: a
// private profile is 404 for everyone but its owner, and the route never
// requires authentication.
func TestCompanionPublicState(t *testing.T) {
	f := newFx(t)
	ownerID, ownerTok := f.register("grace")
	_, otherTok := f.register("heidi")

	path := fmt.Sprintf("/api/users/%d/companion", ownerID)

	// No state row yet: the lookup neither creates one nor leaks a default.
	w, resp := f.do(http.MethodGet, path, "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])

	// Petting creates the state row; the profile is private by default.
	w, _ = f.do(http.MethodPost, "/api/companion/pet", ownerTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, path, "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "guests cannot see a private profile")
	assert.EqualValues(t, 40400, resp["code"])

	w, resp = f.do(http.MethodGet, path, otherTok, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "another user cannot see a private profile")
	assert.EqualValues(t, 40400, resp["code"])

	// The owner always sees their own summary.
	w, resp = f.do(http.MethodGet, path, ownerTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataOf(t, resp)
	assert.EqualValues(t, ownerID, data["userId"])
	assert.EqualValues(t, 1, data["bond"])
	assert.EqualValues(t, 0, data["level"])
	assert.Equal(t, "陌生", data["levelName"])
	assert.EqualValues(t, 20, data["nextThreshold"])
	assert.Equal(t, false, data["publicProfile"])
	assert.Equal(t, "", data["equippedSkin"])
	assert.NotNil(t, data["unlockedSkins"], "unlockedSkins must serialize as []")
	assert.GreaterOrEqual(t, data["daysTogether"], float64(1))
	assert.NotContains(t, data, "activityLevel", "summary must not leak private settings")
	assert.NotContains(t, data, "dailyGain")
	assert.NotContains(t, data, "unreadCount")

	// Publishing the profile exposes it to guests and other signed-in users.
	w, _ = f.do(http.MethodPost, "/api/companion/settings", ownerTok, map[string]any{
		"publicProfile": true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	guest := dataOf(t, resp)
	assert.EqualValues(t, 1, guest["bond"])
	assert.Equal(t, true, guest["publicProfile"])

	w, resp = f.do(http.MethodGet, path, otherTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, dataOf(t, resp)["bond"])

	// Non-numeric and unknown ids are not found.
	for _, bad := range []string{"/api/users/abc/companion", "/api/users/0/companion", "/api/users/999999/companion"} {
		w, resp = f.do(http.MethodGet, bad, "", nil)
		assert.Equal(t, http.StatusNotFound, w.Code, bad)
		assert.EqualValues(t, 40400, resp["code"], bad)
	}
}

// Marking more than the accepted batch size is rejected before hitting the DB.
func TestCompanionMarkReadBatchLimit(t *testing.T) {
	f := newFx(t)
	_, tok := f.register("frank")
	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	w, resp := f.do(http.MethodPost, "/api/companion/events/read", tok, map[string]any{"ids": ids})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.do(http.MethodPost, "/api/companion/events/read", tok, map[string]any{"ids": []int64{}})
	assert.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodPost, "/api/companion/settings", tok,
		map[string]any{"activityLevel": "quiet", "publicProfile": "yes"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40000, resp["code"])
}
