package contestapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/contestapi"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fakeGamePublisher struct {
	events []gamequeue.Event
	err    error
}

func (p *fakeGamePublisher) Publish(_ context.Context, ev gamequeue.Event) error {
	p.events = append(p.events, ev)
	return p.err
}

type fx struct {
	t          *testing.T
	h          http.Handler
	db         *gorm.DB
	rbac       *rbac.Service
	tokens     map[string]string
	ids        map[string]int64
	gameEvents *fakeGamePublisher
	nextN      int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	return newFxWithPublisher(t, &fakeGamePublisher{})
}

func newFxWithPublisher(t *testing.T, publisher *fakeGamePublisher) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	h := router.New(router.Deps{
		DB: g, Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Storage: store,
		Config: &config.Config{
			JWTSecret: strings.Repeat("s", 32), AccessTokenTTL: 15 * time.Minute,
			RefreshTokenTTL: time.Hour, SubmitCooldown: time.Second,
		},
		Enqueuer: noopQueue{},
		Game:     gamification.NewService(gamification.NewRepository(g), publisher),
		RBAC:     rbacSvc,
	})
	return &fx{
		t: t, h: h, db: g, rbac: rbacSvc, tokens: map[string]string{}, ids: map[string]int64{},
		gameEvents: publisher,
	}
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

// registerUser 创建携带指定 legacy role 的用户并登录，返回 token 与 user id。
// legacy role 在登录前写入，使 JWT 携带旧 role；授权不再依赖 JWT role。
func (f *fx) registerUser(role string) (string, int64) {
	f.t.Helper()
	f.nextN++
	name := fmt.Sprintf("%s%d", strings.ReplaceAll(role, "_", ""), f.nextN)
	w, _ := f.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"username": name, "email": name + "@example.com", "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(f.t, f.db.Exec("UPDATE users SET role = ? WHERE username = ?", role, name).Error)
	w, resp := f.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"account": name, "password": "correct-horse",
	})
	require.Equal(f.t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	return data["accessToken"].(string), int64(data["user"].(map[string]any)["id"].(float64))
}

// token 返回携带给定 legacy role 及其映射 RBAC 角色的用户 token
// （super_admin→super_admin、problem_admin→content_admin、
// contest_admin→contest_admin、user→无管理角色）。
func (f *fx) token(role string) string {
	f.t.Helper()
	if role == "guest" {
		return ""
	}
	if tok, ok := f.tokens[role]; ok {
		return tok
	}
	tok, userID := f.registerUser(role)
	testutil.AssignLegacyRole(f.t, f.db, userID, user.Role(role))
	f.tokens[role] = tok
	f.ids[role] = userID
	return tok
}

// legacyToken 返回携带给定 legacy role 但无 RBAC 角色分配的用户 token，
// 用于证明旧 role 不再授权管理操作。
func (f *fx) legacyToken(role string) string {
	f.t.Helper()
	tok, _ := f.registerUser(role)
	return tok
}

// createRole 创建持有给定权限码的自定义角色。
func (f *fx) createRole(code string, perms ...rbac.PermissionCode) {
	f.t.Helper()
	codes := make([]string, len(perms))
	for i, p := range perms {
		codes[i] = string(p)
	}
	_, err := f.rbac.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: code, Name: code, PermissionCodes: codes,
	})
	require.NoError(f.t, err)
}

// userWithRole 返回被分配给定自定义角色的普通用户 token。
func (f *fx) userWithRole(code string) string {
	f.t.Helper()
	tok, userID := f.registerUser("user")
	testutil.AssignRole(f.t, f.db, userID, code)
	return tok
}

func (f *fx) problem(slug string) *problem.Problem {
	f.t.Helper()
	repo := problem.NewGormRepository(f.db)
	p := &problem.Problem{
		Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
		Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedBy: 1,
	}
	require.NoError(f.t, repo.Create(context.Background(), p))
	return p
}

func body(p1, p2 int64) map[string]any {
	return map[string]any{
		"title": "Weekly", "descriptionMd": "desc",
		"startTime": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"endTime":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"mode":      "icpc", "isRated": false, "isPublic": true, "registerType": "open",
		"problems": []map[string]any{
			{"problemId": p1, "displayId": "A", "score": 100},
			{"problemId": p2, "displayId": "B", "score": 200},
		},
	}
}

func idOf(resp map[string]any) int64 {
	return int64(resp["data"].(map[string]any)["id"].(float64))
}

func (f *fx) equipIdentity(t *testing.T, userID int64, badgeNames []string, titleName string) {
	t.Helper()
	for i, name := range badgeNames {
		badge := &gamification.Badge{
			Name: name, Category: "测试", SortOrder: i, IsActive: true,
		}
		require.NoError(t, f.db.Create(badge).Error)
		require.NoError(t, f.db.Create(&gamification.UserBadge{
			UserID: userID, BadgeID: badge.ID, IsEquipped: true, PurchasedAt: time.Now(),
		}).Error)
	}
	if titleName == "" {
		return
	}
	title := &gamification.Title{
		Name: titleName, Color: "#7C3AED",
		UnlockConditionType: string(gamification.TitleUnlockACCount),
		UnlockConditionConfig: dbtypes.JSON[map[string]any]{
			Data: map[string]any{"threshold": 1},
		},
		IsActive: true,
	}
	require.NoError(t, f.db.Create(title).Error)
	require.NoError(t, f.db.Create(&gamification.UserTitle{
		UserID: userID, TitleID: title.ID, IsEquipped: true, UnlockedAt: time.Now(),
	}).Error)
}

func TestContestPermissionsAndCrud(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)

	w, _ := f.do(http.MethodPost, "/api/contests", "", b)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.do(http.MethodPost, "/api/contests", f.token("user"), b)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	w, resp = f.do(http.MethodGet, "/api/contests", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])

	b["title"] = "Renamed"
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/clone", id), f.token("contest_admin"), map[string]any{})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "Renamed（副本）", resp["data"].(map[string]any)["title"])

	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateRequiresFullContestPayload(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	b["registerType"] = "password"
	b["password"] = "secret"
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	delete(b, "problems")
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	b = body(p1.ID, p2.ID)
	b["registerType"] = "password"
	b["password"] = "secret"
	delete(b, "registerType")
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), b)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("contest_admin"), nil)
	assert.Equal(t, "password", resp["data"].(map[string]any)["registerType"])
}

func TestReferencedProblemCannotBeDeleted(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	w, _ := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)

	w, resp := f.do(http.MethodDelete, fmt.Sprintf("/api/problems/%d", p1.ID), f.token("problem_admin"), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "比赛")

	// 题目已被进行中的比赛派生锁定，游客不可见，但题目仍存在。
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p1.ID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", p1.ID), f.token("problem_admin"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContestDetailRegistrationAndProblems(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	b["isPublic"] = false
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	assert.Equal(t, http.StatusOK, w.Code)
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", id), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, resp["data"].(map[string]any)["registered"])
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	future := body(p1.ID, p2.ID)
	future["title"] = "Future"
	future["startTime"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	future["endTime"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), future)
	futureID := idOf(resp)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", futureID), f.token("user"), nil)
	assert.Empty(t, resp["data"].(map[string]any)["problems"])
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", futureID), f.token("contest_admin"), nil)
	assert.Len(t, resp["data"].(map[string]any)["problems"], 2)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/participants", id), f.token("contest_admin"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	participants := resp["data"].([]any)
	require.Len(t, participants, 1)
	assert.Contains(t, participants[0].(map[string]any)["username"], "user")
}

func TestContestRegisterPublishesGameEvent(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var joins []gamequeue.Event
	loginEvents := 0
	for _, ev := range f.gameEvents.events {
		switch ev.Type {
		case gamequeue.EventContestJoin:
			joins = append(joins, ev)
		case gamequeue.EventUserLogin:
			if ev.UserID == f.ids["user"] {
				loginEvents++
			}
		}
	}
	require.Len(t, joins, 1)
	assert.Equal(t, f.ids["user"], joins[0].UserID)
	assert.Equal(t, id, joins[0].ContestID)
	assert.False(t, joins[0].OccurredAt.IsZero())
	assert.GreaterOrEqual(t, loginEvents, 1, "router must pass the game service as the auth login hook")
}

func TestContestRegisterHookFailureDoesNotFailResponse(t *testing.T) {
	f := newFxWithPublisher(t, &fakeGamePublisher{err: errors.New("redis down")})
	p1, p2 := f.problem("sum"), f.problem("product")
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRanklistEndpoint(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", id), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "icpc", data["mode"])
	rows := data["rows"].([]any)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].(map[string]any)["username"], "user")
	assert.EqualValues(t, 1, rows[0].(map[string]any)["rank"])
}

func TestRanklistEquippedIdentity(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	userToken := f.token("user")
	userID := f.ids["user"]
	f.equipIdentity(t, userID, []string{"徽章一", "徽章二"}, "卷王")

	for _, mode := range []string{"icpc", "ioi", "oi"} {
		t.Run(mode, func(t *testing.T) {
			b := body(p1.ID, p2.ID)
			b["mode"] = mode
			b["title"] = "Identity " + mode
			w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			id := idOf(resp)

			w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), userToken, map[string]any{})
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", id), userToken, nil)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			data := resp["data"].(map[string]any)
			rows := data["rows"].([]any)
			require.Len(t, rows, 1, "装饰身份不能改变 OI selfOnly 行数")
			row := rows[0].(map[string]any)
			badges := row["equippedBadges"].([]any)
			require.Len(t, badges, 2)
			assert.Equal(t, "徽章一", badges[0].(map[string]any)["name"])
			assert.Equal(t, "徽章二", badges[1].(map[string]any)["name"])
			title := row["equippedTitle"].(map[string]any)
			assert.Equal(t, "卷王", title["name"])
			assert.Equal(t, "#7C3AED", title["color"])
			if mode == "oi" {
				assert.Equal(t, true, data["selfOnly"])
			}
		})
	}
}

func TestRanklistOISelfOnlyKeepsOnlyDecoratedSelfRow(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("oi-self-sum"), f.problem("oi-self-product")
	b := body(p1.ID, p2.ID)
	b["mode"] = "oi"
	b["title"] = "OI selfOnly identity"
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	userToken := f.token("user")
	userID := f.ids["user"]
	otherToken := f.token("problem_admin")
	otherID := f.ids["problem_admin"]
	for _, token := range []string{userToken, otherToken} {
		w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), token, map[string]any{})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	f.equipIdentity(t, userID, []string{"本人徽章"}, "本人称号")
	f.equipIdentity(t, otherID, []string{"他人徽章"}, "他人称号")

	// 管理员看到两行完整榜单及各自装备。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", id), f.token("contest_admin"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["selfOnly"])
	require.Len(t, data["rows"].([]any), 2)

	// 选手本人只保留自己的一行，并且只装饰该行。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", id), userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["selfOnly"])
	rows := data["rows"].([]any)
	require.Len(t, rows, 1, "OI selfOnly 不能改变保留行数")
	row := rows[0].(map[string]any)
	assert.EqualValues(t, userID, row["userId"])
	badges := row["equippedBadges"].([]any)
	require.Len(t, badges, 1)
	assert.Equal(t, "本人徽章", badges[0].(map[string]any)["name"])
	assert.Equal(t, "本人称号", row["equippedTitle"].(map[string]any)["name"])
}

func TestRanklistWithoutEquipmentUsesEmptyArray(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", id), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rows := resp["data"].(map[string]any)["rows"].([]any)
	require.Len(t, rows, 1)
	row := rows[0].(map[string]any)
	assert.Equal(t, []any{}, row["equippedBadges"])
	assert.Nil(t, row["equippedTitle"])

	// 未开始的比赛对普通用户返回空榜单，装饰逻辑不报错。
	future := body(p1.ID, p2.ID)
	future["title"] = "Future ranklist"
	future["startTime"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	future["endTime"] = time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), future)
	futureID := idOf(resp)
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/ranklist", futureID), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, []any{}, data["rows"])
	assert.Equal(t, false, data["selfOnly"])
}

func TestAnnouncementPermissionsAndCrud(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), f.token("user"),
		map[string]any{"title": "Notice", "contentMd": "body"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), f.token("contest_admin"),
		map[string]any{"title": "Notice", "contentMd": "body"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	aid := idOf(resp)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", id), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, resp["data"].([]any), 1)

	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d/announcements/%d", id, aid),
		f.token("contest_admin"), map[string]any{"title": "Fixed", "contentMd": "new"})
	assert.Equal(t, http.StatusOK, w.Code)
	_, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", id), "", nil)
	assert.Equal(t, "Fixed", resp["data"].([]any)[0].(map[string]any)["title"])

	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d/announcements/%d", id, aid),
		f.token("contest_admin"), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	private := body(p1.ID, p2.ID)
	private["title"] = "Private announcements"
	private["isPublic"] = false
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), private)
	privateID := idOf(resp)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", privateID), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", privateID), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/announcements", privateID), f.token("user"), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContestMineFilter(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	private := body(p1.ID, p2.ID)
	private["title"] = "Mine"
	private["isPublic"] = false
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), private)
	require.Equal(t, http.StatusCreated, w.Code)
	id := idOf(resp)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", id), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, "/api/contests?mine=true", f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 1, data["total"])
	item := data["items"].([]any)[0].(map[string]any)
	assert.Equal(t, true, item["registered"])

	w, _ = f.do(http.MethodGet, "/api/contests?mine=true", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestContestListFilters(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)
	_, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.NotZero(t, idOf(resp))

	_, resp = f.do(http.MethodGet, "/api/contests?status=running&mode=icpc&keyword=week", "", nil)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	_, resp = f.do(http.MethodGet, "/api/contests?status=ended", "", nil)
	assert.EqualValues(t, 0, resp["data"].(map[string]any)["total"])
	w, _ := f.do(http.MethodGet, "/api/contests?status=bogus", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestContestProblemIDValidation(t *testing.T) {
	f := newFx(t)
	for _, bad := range []string{"0", "-1", "abc", "1.5"} {
		w, resp := f.do(http.MethodGet, "/api/contests?problemId="+bad, "", nil)
		require.Equal(t, http.StatusBadRequest, w.Code, "problemId=%q", bad)
		assert.EqualValues(t, 40001, resp["code"], "problemId=%q", bad)
	}

	// 合法正整数返回 200（无比赛时 total=0）。
	w, resp := f.do(http.MethodGet, "/api/contests?problemId=1", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotNil(t, resp["data"])
}

func TestContestListByProblemEndpoint(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")

	// 公开比赛含 p1+p2，私有比赛只含 p1。
	public := body(p1.ID, p2.ID)
	_, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), public)
	publicID := idOf(resp)

	private := body(p1.ID, p2.ID)
	private["title"] = "Private p1"
	private["isPublic"] = false
	private["problems"] = []map[string]any{{"problemId": p1.ID, "displayId": "A", "score": 100}}
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), private)
	privateID := idOf(resp)

	// 游客：只看到包含 p1 的公开比赛。
	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/contests?problemId=%d", p1.ID), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])
	assert.Equal(t, float64(publicID), resp["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["id"])

	// 游客：p2 只被公开比赛包含，也能看到。
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests?problemId=%d", p2.ID), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["total"])

	// 报名私有比赛后，user 通过 problemId 看到 public + private。
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/register", privateID), f.token("user"), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/contests?problemId=%d", p1.ID), f.token("user"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["total"])
}

type fakeContestActivity struct {
	joinUserIDs       []int64
	joinContestIDs    []int64
	createdUserIDs    []int64
	createdContestIDs []int64
	err               error
}

func (f *fakeContestActivity) RecordContestJoin(_ context.Context, userID, contestID int64) error {
	f.joinUserIDs = append(f.joinUserIDs, userID)
	f.joinContestIDs = append(f.joinContestIDs, contestID)
	return f.err
}

func (f *fakeContestActivity) RecordContestCreated(_ context.Context, userID, contestID int64) error {
	f.createdUserIDs = append(f.createdUserIDs, userID)
	f.createdContestIDs = append(f.createdContestIDs, contestID)
	return f.err
}

// newContestActivityHandler mounts the contest routes with a fake activity
// hook so hook success/failure can be asserted without the real service.
func newContestActivityHandler(t *testing.T, hook contestapi.ActivityHook) (
	h http.Handler, adminToken, userToken string, adminID, userID int64,
	p1, p2 *problem.Problem,
) {
	t.Helper()
	g := testutil.PG(t)
	adminID = int64(9201)
	userID = int64(9202)
	require.NoError(t, g.Exec(
		`INSERT INTO users (id, username, nickname, email, password_hash, role, created_at, updated_at)
		 VALUES (?, 'hookadmin', 'hookadmin', 'hookadmin@example.com', 'x', 'contest_admin', now(), now()),
		        (?, 'hookuser', 'hookuser', 'hookuser@example.com', 'x', 'user', now(), now())`,
		adminID, userID,
	).Error)
	rbacSvc := testutil.SeedRBAC(t, g)
	testutil.AssignLegacyRole(t, g, adminID, user.RoleContestAdmin)

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	var err error
	adminToken, err = issuer.IssueAccess(&user.User{ID: adminID, Role: user.RoleContestAdmin})
	require.NoError(t, err)
	userToken, err = issuer.IssueAccess(&user.User{ID: userID, Role: user.RoleUser})
	require.NoError(t, err)

	repo := problem.NewGormRepository(g)
	makeProblem := func(slug string) *problem.Problem {
		p := &problem.Problem{
			Title: "Problem " + slug, Slug: slug, StatementMD: "statement",
			Type: problem.TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
			IsPublic: true, CreatedBy: adminID,
		}
		require.NoError(t, repo.Create(context.Background(), p))
		return p
	}
	p1, p2 = makeProblem("hook-sum"), makeProblem("hook-product")

	engine := gin.New()
	api := engine.Group("/api", middleware.Authenticate(issuer))
	contestapi.RegisterRoutes(api, contest.NewService(contest.NewGormRepository(g)),
		contestapi.Options{ActivityHook: hook, RBAC: rbacSvc})
	return engine, adminToken, userToken, adminID, userID, p1, p2
}

func doContestRequest(t *testing.T, h http.Handler, method, path, token string, payload any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(payload))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func TestContestActivityHookCreateAndClone(t *testing.T) {
	hook := &fakeContestActivity{}
	h, adminToken, _, adminID, _, p1, p2 := newContestActivityHandler(t, hook)

	w, resp := doContestRequest(t, h, http.MethodPost, "/api/contests", adminToken, body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	contestID := idOf(resp)
	require.Len(t, hook.createdContestIDs, 1)
	assert.Equal(t, []int64{adminID}, hook.createdUserIDs)
	assert.Equal(t, []int64{contestID}, hook.createdContestIDs)

	w, resp = doContestRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/contests/%d/clone", contestID), adminToken, map[string]any{})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	cloneID := idOf(resp)
	require.Len(t, hook.createdContestIDs, 2)
	assert.Equal(t, []int64{adminID, adminID}, hook.createdUserIDs)
	assert.Equal(t, []int64{contestID, cloneID}, hook.createdContestIDs)
}

func TestContestActivityHookRegister(t *testing.T) {
	hook := &fakeContestActivity{}
	h, adminToken, userToken, _, userID, p1, p2 := newContestActivityHandler(t, hook)

	w, resp := doContestRequest(t, h, http.MethodPost, "/api/contests", adminToken, body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	contestID := idOf(resp)

	w, _ = doContestRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/contests/%d/register", contestID), userToken, map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, hook.joinContestIDs, 1)
	assert.Equal(t, []int64{userID}, hook.joinUserIDs)
	assert.Equal(t, []int64{contestID}, hook.joinContestIDs)
}

func TestContestActivityHookFailureDoesNotFailRegisterOrCreate(t *testing.T) {
	hook := &fakeContestActivity{err: errors.New("activity down")}
	h, adminToken, userToken, _, _, p1, p2 := newContestActivityHandler(t, hook)

	w, resp := doContestRequest(t, h, http.MethodPost, "/api/contests", adminToken, body(p1.ID, p2.ID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	contestID := idOf(resp)

	w, _ = doContestRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/contests/%d/clone", contestID), adminToken, map[string]any{})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, _ = doContestRequest(t, h, http.MethodPost,
		fmt.Sprintf("/api/contests/%d/register", contestID), userToken, map[string]any{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, hook.createdContestIDs, 2, "失败后仍尝试记录共享项")
	require.Len(t, hook.joinContestIDs, 1, "失败后仍尝试记录参赛活跃度")
}

func TestContestRoutesRequirePermissions(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)

	legacy := f.legacyToken("contest_admin")

	// legacy contest_admin token 但无 RBAC 角色 → create 403。
	w, _ := f.do(http.MethodPost, "/api/contests", legacy, b)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// contest_admin 角色 → create 201。
	w, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	// legacy token：update/delete/participants/announcement 403。
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), legacy, b)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", id), legacy, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/participants", id), legacy, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), legacy,
		map[string]any{"title": "t", "contentMd": "c"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	// view_hidden-only → 可读隐藏比赛，但不能 create/update/delete。
	f.createRole("view_hidden_only", rbac.PermContestViewHidden)
	viewHidden := f.userWithRole("view_hidden_only")

	hidden := body(p1.ID, p2.ID)
	hidden["title"] = "Hidden"
	hidden["isPublic"] = false
	_, resp = f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), hidden)
	hiddenID := idOf(resp)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d", hiddenID), viewHidden, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = f.do(http.MethodPost, "/api/contests", viewHidden, body(p1.ID, p2.ID))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", hiddenID), viewHidden, hidden)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", hiddenID), viewHidden, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestContestGranularPermissions(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	b := body(p1.ID, p2.ID)

	f.createRole("update_only", rbac.PermContestUpdate)
	f.createRole("participant_read", rbac.PermContestParticipantRead)
	f.createRole("announcement_manage", rbac.PermContestAnnouncementManage)
	f.createRole("create_only", rbac.PermContestCreate)

	updateOnly := f.userWithRole("update_only")
	participantRead := f.userWithRole("participant_read")
	announcementManage := f.userWithRole("announcement_manage")
	createOnly := f.userWithRole("create_only")

	// 用 contest_admin 建一个比赛作为操作对象。
	_, resp := f.do(http.MethodPost, "/api/contests", f.token("contest_admin"), b)
	id := idOf(resp)

	// 只有 contest.update：可更新，不能 create/delete/participants/announcement。
	w, _ := f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), updateOnly, b)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, _ = f.do(http.MethodPost, "/api/contests", updateOnly, b)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", id), updateOnly, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/participants", id), updateOnly, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), updateOnly,
		map[string]any{"title": "t", "contentMd": "c"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 只有 contest.participant.read：可看报名，不能更新。
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/contests/%d/participants", id), participantRead, nil)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), participantRead, b)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 只有 contest.announcement.manage：可管理公告，不能更新比赛。
	w, respAnn := f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), announcementManage,
		map[string]any{"title": "Notice", "contentMd": "body"})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	aid := idOf(respAnn)
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), announcementManage, b)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d/announcements/%d", id, aid), announcementManage, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// 只有 contest.create：可创建/克隆，不能更新。
	w, respCreated := f.do(http.MethodPost, "/api/contests", createOnly, b)
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	createdID := idOf(respCreated)
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/clone", id), createOnly, map[string]any{})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", createdID), createOnly, b)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestContestWritesAudit(t *testing.T) {
	f := newFx(t)
	p1, p2 := f.problem("sum"), f.problem("product")
	admin := f.token("contest_admin")
	b := body(p1.ID, p2.ID)

	auditCount := func(action string) int64 {
		var n int64
		require.NoError(f.t, f.db.Table("admin_audit_logs").Where("action = ?", action).Count(&n).Error)
		return n
	}

	// create → contest.create 审计。
	w, resp := f.do(http.MethodPost, "/api/contests", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)
	assert.Equal(t, int64(1), auditCount("contest.create"))

	// update → contest.update 审计。
	b["title"] = "Audit renamed"
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/contests/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("contest.update"))

	// announcement → contest.announcement.manage 审计。
	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/contests/%d/announcements", id), admin,
		map[string]any{"title": "Notice", "contentMd": "body"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("contest.announcement.manage"))

	// delete → contest.delete 审计。
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/contests/%d", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount("contest.delete"))
}
