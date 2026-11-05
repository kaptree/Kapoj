package gamificationapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
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

	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	tokens map[string]string
	ids    map[string]int64
	nextN  int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
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
		Game: gamification.NewServiceWithOptions(gamification.NewRepository(g),
			gamification.ServiceOptions{MaxEquippedBadges: 3, Storage: store}),
	})
	return &fx{
		t: t, h: h, db: g,
		tokens: map[string]string{}, ids: map[string]int64{},
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

func (f *fx) token(role string) string {
	f.t.Helper()
	if role == "guest" {
		return ""
	}
	if tok, ok := f.tokens[role]; ok {
		return tok
	}
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
	tok := data["accessToken"].(string)
	f.tokens[role] = tok
	f.ids[role] = int64(data["user"].(map[string]any)["id"].(float64))
	return tok
}

func achievementBody(code, name, conditionType string, cfg map[string]any, reward int64) map[string]any {
	return map[string]any{
		"code": code, "name": name, "description": "测试成就", "iconUrl": "",
		"conditionType": conditionType, "conditionConfig": cfg,
		"rewardCoins": reward, "sortOrder": 1,
	}
}

func cloneBody(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func itemsOf(t *testing.T, resp map[string]any) []any {
	t.Helper()
	items, ok := resp["data"].([]any)
	require.True(t, ok, "expected data array, got %T", resp["data"])
	return items
}

func itemByID(t *testing.T, items []any, id int64) map[string]any {
	t.Helper()
	for _, raw := range items {
		item := raw.(map[string]any)
		if int64(item["id"].(float64)) == id {
			return item
		}
	}
	t.Fatalf("achievement %d not found in %+v", id, items)
	return nil
}

func (f *fx) addAcceptedSubmission(t *testing.T, userID int64, slug string) {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug}
	require.NoError(t, f.db.Create(p).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now())`,
		userID, p.ID, submission.StatusAccepted,
	).Error)
}

func TestAchievementPermissionsAndList(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")
	userID := f.ids["user"]

	w, resp := f.do(http.MethodGet, "/api/achievements", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, itemsOf(t, resp))

	firstBody := achievementBody("first-ac", "首次 AC", "ac_count", map[string]any{"threshold": 1}, 50)
	w, _ = f.do(http.MethodPost, "/api/achievements", userToken, firstBody)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp = f.do(http.MethodPost, "/api/achievements", adminToken, firstBody)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	first := resp["data"].(map[string]any)
	firstID := int64(first["id"].(float64))
	assert.Equal(t, true, first["isActive"], "isActive 缺省应为 true")
	assert.EqualValues(t, 50, first["rewardCoins"])
	cfg := first["conditionConfig"].(map[string]any)
	assert.EqualValues(t, 1, cfg["threshold"])
	assert.NotContains(t, cfg, "conditions")

	secondBody := achievementBody("hundred-ac", "百题斩", "ac_count", map[string]any{"threshold": 100}, 0)
	w, resp = f.do(http.MethodPost, "/api/achievements", adminToken, secondBody)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	secondID := int64(resp["data"].(map[string]any)["id"].(float64))

	f.addAcceptedSubmission(t, userID, "api-ac-problem")

	// 登录用户读取会补算并解锁第一个成就。
	w, resp = f.do(http.MethodGet, "/api/achievements", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := itemsOf(t, resp)
	require.Len(t, items, 2)
	firstItem := itemByID(t, items, firstID)
	assert.Equal(t, true, firstItem["unlocked"])
	assert.NotNil(t, firstItem["unlockedAt"])
	assert.Equal(t, float64(1), firstItem["progress"].([]any)[0].(map[string]any)["target"])
	assert.Equal(t, true, firstItem["progress"].([]any)[0].(map[string]any)["done"])

	w, _ = f.do(http.MethodGet, "/api/achievements?includeInactive=true", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/achievements/%d", secondID), adminToken,
		func() map[string]any { b := cloneBody(secondBody); b["isActive"] = false; return b }())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/achievements/%d", firstID), adminToken,
		func() map[string]any { b := cloneBody(firstBody); b["isActive"] = false; return b }())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 已解锁的成就下线后仍对本人可见；未解锁且下线的不可见。
	w, resp = f.do(http.MethodGet, "/api/achievements", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	items = itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, firstID, int64(items[0].(map[string]any)["id"].(float64)))
	assert.Equal(t, true, items[0].(map[string]any)["unlocked"])
	assert.Equal(t, false, items[0].(map[string]any)["isActive"])

	w, resp = f.do(http.MethodGet, "/api/achievements", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, itemsOf(t, resp))

	w, resp = f.do(http.MethodGet, "/api/achievements?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, itemsOf(t, resp), 2)

	w, resp = f.do(http.MethodGet, "/api/achievements", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, itemsOf(t, resp), "管理员默认列表也只含上架或自己已解锁的成就")

	w, _ = f.do(http.MethodGet, "/api/users/me/achievements", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, resp = f.do(http.MethodGet, "/api/users/me/achievements", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, itemsOf(t, resp), 1)

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/achievements/%d", firstID), adminToken,
		func() map[string]any { b := cloneBody(firstBody); b["code"] = "renamed-code"; return b }())
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	badgeBody := achievementBody("badge-reward", "徽章奖励", "ac_count", map[string]any{"threshold": 1}, 0)
	badgeBody["rewardBadgeId"] = 999999
	w, resp = f.do(http.MethodPost, "/api/achievements", adminToken, badgeBody)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "徽章")

	badgeCreate := map[string]any{
		"name": "成就回馈徽章", "description": "", "imageUrl": "",
		"category": "成就", "priceCoins": 0, "isLimited": false, "stock": 0, "sortOrder": 1,
	}
	w, resp = f.do(http.MethodPost, "/api/badges", adminToken, badgeCreate)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rewardBadgeID := int64(resp["data"].(map[string]any)["id"].(float64))

	badgeBody["rewardBadgeId"] = rewardBadgeID
	w, resp = f.do(http.MethodPost, "/api/achievements", adminToken, badgeBody)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.EqualValues(t, rewardBadgeID, resp["data"].(map[string]any)["rewardBadgeId"])

	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/achievements/%d", firstID), userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/achievements/%d", secondID), adminToken, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func taskBody(title string, problemIDs []int64, reward int64, start, end time.Time) map[string]any {
	return map[string]any{
		"title": title, "descriptionMd": "任务描述",
		"problemIds": problemIDs, "rewardCoins": reward,
		"startTime": start.Format(time.RFC3339), "endTime": end.Format(time.RFC3339),
	}
}

func (f *fx) addPublicProblem(t *testing.T, slug string) *problem.Problem {
	t.Helper()
	p := &problem.Problem{Title: slug, Slug: slug, IsPublic: true}
	require.NoError(t, f.db.Create(p).Error)
	return p
}

func (f *fx) addTaskSubmission(t *testing.T, userID, problemID int64, judgedAt time.Time) {
	t.Helper()
	require.NoError(t, f.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at, judged_at)
		 VALUES (?, ?, 'cpp', 'int main() {}', ?, now(), ?)`,
		userID, problemID, submission.StatusAccepted, judgedAt,
	).Error)
}

func TestTaskAPI(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")
	userID := f.ids["user"]

	w, resp := f.do(http.MethodGet, "/api/tasks", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, itemsOf(t, resp))

	p1 := f.addPublicProblem(t, "api-task-p1")
	p2 := f.addPublicProblem(t, "api-task-p2")
	now := time.Now()
	body := taskBody("API 任务", []int64{p1.ID, p2.ID}, 80, now.Add(-time.Hour), now.Add(time.Hour))

	// 普通用户不能管理任务。
	w, _ = f.do(http.MethodPost, "/api/tasks", userToken, body)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 题目为空返回 40001。
	invalid := taskBody("空任务", nil, 10, now.Add(-time.Hour), now.Add(time.Hour))
	w, resp = f.do(http.MethodPost, "/api/tasks", adminToken, invalid)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/tasks", adminToken, body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	task := resp["data"].(map[string]any)
	taskID := int64(task["id"].(float64))
	assert.Equal(t, true, task["isActive"], "isActive 缺省应为 true")
	assert.Equal(t, "active", task["status"])
	assert.EqualValues(t, 2, task["problemCount"])
	assert.EqualValues(t, 0, task["progress"])
	assert.Nil(t, task["claimedAt"])
	taskPath := fmt.Sprintf("/api/tasks/%d", taskID)

	// 游客可以看到上架任务。
	w, resp = f.do(http.MethodGet, "/api/tasks", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	items := itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, taskID, int64(items[0].(map[string]any)["id"].(float64)))

	w, _ = f.do(http.MethodGet, "/api/tasks?includeInactive=true", "", nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, "/api/tasks?includeInactive=true", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.do(http.MethodPut, taskPath, userToken, body)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, taskPath, userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 游客不能领取，登录用户可以领取且重复领取幂等。
	w, _ = f.do(http.MethodPost, taskPath+"/claim", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, resp = f.do(http.MethodPost, taskPath+"/claim", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	claimed := resp["data"].(map[string]any)
	assert.Equal(t, "claimed", claimed["status"])
	assert.NotNil(t, claimed["claimedAt"])
	assert.EqualValues(t, 0, claimed["progress"])

	w, resp = f.do(http.MethodPost, taskPath+"/claim", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, claimed["claimedAt"], resp["data"].(map[string]any)["claimedAt"])

	w, resp = f.do(http.MethodGet, "/api/tasks", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	items = itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, "claimed", items[0].(map[string]any)["status"])

	// 已有领取记录后冻结题目与奖励。
	frozenProblems := taskBody("API 任务", []int64{p1.ID}, 80, now.Add(-time.Hour), now.Add(time.Hour))
	w, resp = f.do(http.MethodPut, taskPath, adminToken, frozenProblems)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	frozenReward := taskBody("API 任务", []int64{p1.ID, p2.ID}, 90, now.Add(-time.Hour), now.Add(time.Hour))
	w, resp = f.do(http.MethodPut, taskPath, adminToken, frozenReward)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 标题与结束时间仍可修改。
	editable := taskBody("改名后的 API 任务", []int64{p1.ID, p2.ID}, 80,
		now.Add(-time.Hour), now.Add(2*time.Hour))
	w, resp = f.do(http.MethodPut, taskPath, adminToken, editable)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "改名后的 API 任务", resp["data"].(map[string]any)["title"])

	// 领取后 AC 全部题目，读取任务大厅时自动完成并发奖。judged_at 必须晚于
	// 服务端实际写入的 claimed_at，因此从响应解析而不是使用本地 now。
	claimTime, err := time.Parse(time.RFC3339Nano, claimed["claimedAt"].(string))
	require.NoError(t, err)
	claimTime = claimTime.Add(time.Second)
	f.addTaskSubmission(t, userID, p1.ID, claimTime)
	f.addTaskSubmission(t, userID, p2.ID, claimTime.Add(time.Second))

	w, resp = f.do(http.MethodGet, "/api/tasks", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items = itemsOf(t, resp)
	require.Len(t, items, 1)
	completed := items[0].(map[string]any)
	assert.Equal(t, "completed", completed["status"])
	assert.EqualValues(t, 2, completed["progress"])
	assert.NotNil(t, completed["completedAt"])

	w, resp = f.do(http.MethodGet, "/api/coins/transactions", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	coins := resp["data"].(map[string]any)
	assert.EqualValues(t, 80, coins["balance"])
	require.EqualValues(t, 1, coins["total"])
	reward := coins["items"].([]any)[0].(map[string]any)
	assert.EqualValues(t, 80, reward["amount"])
	assert.Equal(t, "task", reward["reason"])
	assert.Equal(t, "task", reward["refType"])
	assert.EqualValues(t, taskID, reward["refId"])

	// 已完成任务不能再次领取。
	w, resp = f.do(http.MethodPost, taskPath+"/claim", userToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 软删除后游客不可见，领取过的用户仍可见完成记录，管理员可见下线任务。
	w, _ = f.do(http.MethodDelete, taskPath, adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, "/api/tasks", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, itemsOf(t, resp))

	w, resp = f.do(http.MethodGet, "/api/tasks", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	items = itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, "completed", items[0].(map[string]any)["status"])
	assert.Equal(t, false, items[0].(map[string]any)["isActive"])

	w, resp = f.do(http.MethodGet, "/api/tasks?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, itemsOf(t, resp), 1)

	w, _ = f.do(http.MethodPost, "/api/tasks/99999/claim", userToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodPut, "/api/tasks/99999", adminToken, body)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodDelete, "/api/tasks/99999", adminToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminAdjustCoinsEndpoint(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")
	userID := f.ids["user"]
	adjustPath := fmt.Sprintf("/api/admin/users/%d/coins", userID)

	w, _ := f.do(http.MethodPost, adjustPath, userToken, map[string]any{"amount": 100, "note": "活动奖励"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp := f.do(http.MethodPost, adjustPath, adminToken, map[string]any{"amount": 0, "note": "活动奖励"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, adjustPath, adminToken, map[string]any{"amount": 100, "note": "   "})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, adjustPath, adminToken,
		map[string]any{"amount": 100, "note": strings.Repeat("奖", 201)})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, adjustPath, adminToken, map[string]any{"amount": 100, "note": "活动奖励"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 100, resp["data"].(map[string]any)["balance"])

	w, resp = f.do(http.MethodGet, "/api/coins/transactions", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 100, data["balance"])
	assert.EqualValues(t, 1, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.EqualValues(t, 100, item["amount"])
	assert.Equal(t, "admin_adjust", item["type"])
	assert.Equal(t, "admin_adjust", item["reason"])
	assert.Equal(t, "活动奖励", item["note"])

	w, _ = f.do(http.MethodGet, "/api/coins/transactions", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, resp = f.do(http.MethodPost, adjustPath, adminToken, map[string]any{"amount": -150, "note": "扣减"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, adjustPath, adminToken, map[string]any{"amount": -30, "note": "扣减"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 70, resp["data"].(map[string]any)["balance"])

	w, resp = f.do(http.MethodGet, "/api/coins/transactions?page=1&page_size=1", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, 70, data["balance"])
	assert.EqualValues(t, 2, data["total"])
	assert.Len(t, data["items"].([]any), 1)

	// 极大页码不能让 offset 溢出并返回 500。
	w, resp = f.do(http.MethodGet,
		"/api/coins/transactions?page=9223372036854775807&page_size=100", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, resp["data"].(map[string]any)["items"])

	w, _ = f.do(http.MethodPost, "/api/admin/users/99999/coins", adminToken,
		map[string]any{"amount": 1, "note": "不存在用户"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func (f *fx) upload(path, token, field, filename string, data []byte) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, filename)
	require.NoError(f.t, err)
	_, err = part.Write(data)
	require.NoError(f.t, err)
	require.NoError(f.t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func badgeBody(name string, price int64) map[string]any {
	return map[string]any{
		"name": name, "description": "徽章描述", "imageUrl": "",
		"category": "测试", "priceCoins": price, "isLimited": false,
		"stock": 0, "sortOrder": 1,
	}
}

var pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}

func TestBadgeAPIPermissionsAndCRUD(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")

	w, resp := f.do(http.MethodGet, "/api/badges", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, itemsOf(t, resp))

	w, _ = f.do(http.MethodPost, "/api/badges", userToken, badgeBody("无权限徽章", 0))
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, resp = f.do(http.MethodPost, "/api/badges", adminToken, badgeBody("  ", 0))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/badges", adminToken, badgeBody("商城徽章", 50))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := resp["data"].(map[string]any)
	badgeID := int64(created["id"].(float64))
	assert.Equal(t, true, created["isActive"], "isActive 缺省应为 true")
	assert.Equal(t, false, created["owned"])
	badgePath := fmt.Sprintf("/api/badges/%d", badgeID)

	w, resp = f.do(http.MethodGet, "/api/badges", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, false, items[0].(map[string]any)["owned"])
	assert.Nil(t, items[0].(map[string]any)["purchasedAt"])

	w, _ = f.do(http.MethodPut, badgePath, userToken, badgeBody("改名", 50))
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodDelete, badgePath, userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	update := badgeBody("改名后的徽章", 80)
	w, resp = f.do(http.MethodPut, badgePath, adminToken, update)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "改名后的徽章", resp["data"].(map[string]any)["name"])
	assert.EqualValues(t, 80, resp["data"].(map[string]any)["priceCoins"])

	update["imageUrl"] = "/api/badges/999999/image"
	w, resp = f.do(http.MethodPut, badgePath, adminToken, update)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/badges/%d", badgeID+1000), adminToken, update)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/badges/%d", badgeID+1000), adminToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = f.do(http.MethodDelete, badgePath, adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, "/api/badges", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, itemsOf(t, resp))

	w, resp = f.do(http.MethodGet, "/api/badges?includeInactive=true", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, itemsOf(t, resp), 1)

	w, _ = f.do(http.MethodGet, "/api/badges?includeInactive=true", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestBadgePurchaseAndEquipAPI(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")
	userID := f.ids["user"]

	w, resp := f.do(http.MethodPost, "/api/badges", adminToken, badgeBody("付费徽章", 100))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	badgeID := int64(resp["data"].(map[string]any)["id"].(float64))
	badgePath := fmt.Sprintf("/api/badges/%d", badgeID)

	w, _ = f.do(http.MethodPost, badgePath+"/purchase", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, resp = f.do(http.MethodPost, badgePath+"/purchase", userToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/admin/users/%d/coins", userID),
		adminToken, map[string]any{"amount": 100, "note": "购买测试"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, _ = f.do(http.MethodPost, badgePath+"/purchase", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, "/api/badges", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	items := itemsOf(t, resp)
	require.Len(t, items, 1)
	assert.Equal(t, true, items[0].(map[string]any)["owned"])
	assert.Equal(t, false, items[0].(map[string]any)["equipped"])
	assert.NotNil(t, items[0].(map[string]any)["purchasedAt"])

	w, resp = f.do(http.MethodPost, badgePath+"/purchase", userToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodGet, "/api/coins/transactions", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	coins := resp["data"].(map[string]any)
	assert.EqualValues(t, 0, coins["balance"])
	txs := coins["items"].([]any)
	require.Len(t, txs, 2)
	// 最新一条是购买流水；管理员调币在前。
	var purchase map[string]any
	for _, raw := range txs {
		tx := raw.(map[string]any)
		if tx["reason"] == "badge_purchase" {
			purchase = tx
		}
	}
	require.NotNil(t, purchase)
	assert.EqualValues(t, -100, purchase["amount"])
	assert.Equal(t, "spend", purchase["type"])
	assert.Equal(t, "badge", purchase["refType"])
	assert.EqualValues(t, badgeID, purchase["refId"])

	w, _ = f.do(http.MethodPost, badgePath+"/equip", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = f.do(http.MethodPost, badgePath+"/equip", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	// 重复装备幂等。
	w, _ = f.do(http.MethodPost, badgePath+"/equip", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/badges", userID), "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	owned := itemsOf(t, resp)
	require.Len(t, owned, 1)
	assert.Equal(t, true, owned[0].(map[string]any)["equipped"])

	w, _ = f.do(http.MethodPost, badgePath+"/unequip", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, resp = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/badges", userID), "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, false, itemsOf(t, resp)[0].(map[string]any)["equipped"])

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/users/%d/badges", userID+1000), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = f.do(http.MethodPost, fmt.Sprintf("/api/badges/%d/purchase", badgeID+1000), userToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, resp = f.do(http.MethodPost, "/api/badges", adminToken, badgeBody("未拥有徽章", 0))
	require.Equal(t, http.StatusCreated, w.Code)
	otherID := int64(resp["data"].(map[string]any)["id"].(float64))
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/badges/%d/equip", otherID), userToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/badges/%d/unequip", otherID), userToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestBadgeImageAPI(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.token("super_admin")

	w, resp := f.do(http.MethodPost, "/api/badges", adminToken, badgeBody("图片徽章", 0))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	badgeID := int64(resp["data"].(map[string]any)["id"].(float64))
	imagePath := fmt.Sprintf("/api/badges/%d/image", badgeID)

	w, _ = f.upload(imagePath, userToken, "file", "badge.png", pngBytes)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = f.upload(imagePath, adminToken, "file", "badge.png", pngBytes)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp = f.do(http.MethodGet, "/api/badges", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, imagePath, itemsOf(t, resp)[0].(map[string]any)["imageUrl"])

	req := httptest.NewRequest(http.MethodGet, imagePath, nil)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
	assert.Equal(t, pngBytes, rec.Body.Bytes())

	w, resp = f.upload(imagePath, adminToken, "file", "badge.txt", []byte("plain text"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	tooLarge := append(append([]byte{}, pngBytes...), make([]byte, 2<<20)...)
	w, resp = f.upload(imagePath, adminToken, "file", "big.png", tooLarge)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.upload(imagePath, adminToken, "wrong", "badge.png", pngBytes)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	w, _ = f.upload(fmt.Sprintf("/api/badges/%d/image", badgeID+1000), adminToken,
		"file", "badge.png", pngBytes)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/badges/%d/image", badgeID+1000), "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
