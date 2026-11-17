package jobsapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/jobsapi"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

const testJobKey = "test.echo"

// fakeEnqueuer records manual run IDs without executing them.
type fakeEnqueuer struct {
	mu  sync.Mutex
	ids []int64
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, runID)
	return nil
}

func (f *fakeEnqueuer) snapshot() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.ids...)
}

type fixture struct {
	engine    *gin.Engine
	db        *gorm.DB
	issuer    *auth.TokenIssuer
	svc       *jobs.Service
	rbac      *rbac.Service
	enqueuer  *fakeEnqueuer
	settings  *difficulty.SettingsService
	userAgent string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	enqueuer := &fakeEnqueuer{}
	svc := jobs.NewService(g, enqueuer)
	svc.Register(jobs.Definition{
		Key:         testJobKey,
		Name:        "测试任务",
		Description: "测试任务说明",
		DefaultCron: "0 3 * * *",
		Timezone:    "Asia/Shanghai",
		Handler: func(context.Context, *jobs.Run, jobs.Reporter) error {
			return nil
		},
	})
	settings := difficulty.NewSettingsService(g)
	difficulty.RegisterJob(svc, difficulty.NewRecalculateService(g, settings))
	require.NoError(t, svc.Ensure(context.Background()))

	rbacSvc := rbac.NewService(g)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	jobsapi.RegisterRoutes(r.Group("/api"), svc, settings, rbacSvc)

	return &fixture{
		engine: r, db: g, issuer: issuer, svc: svc, rbac: rbacSvc,
		enqueuer: enqueuer, settings: settings, userAgent: "kapoj-jobsapi-test",
	}
}

func (f *fixture) user(t *testing.T, name string, roleCodes ...string) (int64, string) {
	t.Helper()
	u := &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "not-a-real-hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	require.NoError(t, f.db.Create(u).Error)
	for _, code := range roleCodes {
		testutil.AssignRole(t, f.db, u.ID, code)
	}
	token, err := f.issuer.IssueAccess(u)
	require.NoError(t, err)
	return u.ID, token
}

func (f *fixture) do(method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", f.userAgent)
	req.RemoteAddr = "203.0.113.10:1234"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func requireStatus(t *testing.T, w *httptest.ResponseRecorder, resp map[string]any, status int) {
	t.Helper()
	require.Equal(t, status, w.Code, w.Body.String())
	if status >= 400 {
		assert.NotZero(t, resp["code"], w.Body.String())
	}
}

func dataMap(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "响应 data 应为对象: %v", resp)
	return data
}

func dataList(t *testing.T, resp map[string]any) []any {
	t.Helper()
	data, ok := resp["data"].([]any)
	require.True(t, ok, "响应 data 应为数组: %v", resp)
	return data
}

func responseInt64(t *testing.T, value any) int64 {
	t.Helper()
	f, ok := value.(float64)
	require.True(t, ok, "应为数字: %T", value)
	return int64(f)
}

func findJob(t *testing.T, items []any, key string) map[string]any {
	t.Helper()
	for _, raw := range items {
		job := raw.(map[string]any)
		if job["key"] == key {
			return job
		}
	}
	t.Fatalf("任务 %s 未出现在列表中", key)
	return nil
}

func defaultSettingsBody() map[string]any {
	return map[string]any{
		"enabled":          true,
		"defaultScore":     50,
		"minSample":        10,
		"priorStrength":    10,
		"maxStep":          5,
		"windowDays":       365,
		"includeContest":   false,
		"curveGamma":       1.5,
		"aiAssessEnabled":  true,
		"aiTimeoutSeconds": 15,
	}
}

func TestAdminJobsPermissionAndRun(t *testing.T) {
	f := newFixture(t)

	// 未登录 401。
	w, resp := f.do(http.MethodGet, "/api/admin/jobs", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	// 普通用户 403。
	_, plainToken := f.user(t, "plain_jobs_user")
	w, resp = f.do(http.MethodGet, "/api/admin/jobs", plainToken, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// content_admin 没有 system.jobs.manage，403。
	_, contentToken := f.user(t, "content_jobs_user", rbac.RoleCodeContentAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/jobs", contentToken, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// 仅持有 system.jobs.manage 的自定义角色可以访问。
	jobsRole, err := f.rbac.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "jobs_admin", Name: "任务管理员",
		PermissionCodes: []string{string(rbac.PermSystemJobsManage)},
	})
	require.NoError(t, err)
	_, jobsToken := f.user(t, "jobs_admin_user", jobsRole.Code)
	w, resp = f.do(http.MethodGet, "/api/admin/jobs", jobsToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	jobsList := dataList(t, resp)
	findJob(t, jobsList, testJobKey)
	findJob(t, jobsList, difficulty.JobKey)

	// super_admin 200；POST run 202。
	superID, superToken := f.user(t, "super_jobs_user", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/jobs", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodPost, "/api/admin/jobs/"+testJobKey+"/run", superToken, "")
	requireStatus(t, w, resp, http.StatusAccepted)
	run := dataMap(t, resp)
	assert.Equal(t, testJobKey, run["jobKey"])
	assert.Equal(t, jobs.TriggerManual, run["trigger"])
	assert.Equal(t, jobs.StatusQueued, run["status"])
	assert.EqualValues(t, superID, responseInt64(t, run["createdBy"]))
	assert.NotZero(t, responseInt64(t, run["id"]))
	assert.Len(t, f.enqueuer.snapshot(), 1)

	var runAudit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		rbac.AuditActionSystemJobsManage, rbac.AuditTargetScheduledJob).
		Order("id DESC").First(&runAudit).Error)
	assert.EqualValues(t, superID, runAudit.ActorID)
	require.NotNil(t, runAudit.TargetID)
	assert.Contains(t, runAudit.Summary, testJobKey)
	assert.Contains(t, runAudit.Summary, "立即执行")

	// 同一任务已有 queued run，再次执行返回 40904。
	w, resp = f.do(http.MethodPost, "/api/admin/jobs/"+testJobKey+"/run", superToken, "")
	requireStatus(t, w, resp, http.StatusConflict)
	assert.EqualValues(t, 40904, resp["code"])
	assert.Len(t, f.enqueuer.snapshot(), 1)

	// 列表中展示 activeRun。
	w, resp = f.do(http.MethodGet, "/api/admin/jobs", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	job := findJob(t, dataList(t, resp), testJobKey)
	active, ok := job["activeRun"].(map[string]any)
	require.True(t, ok, "activeRun 应为对象: %v", job["activeRun"])
	assert.Equal(t, jobs.StatusQueued, active["status"])

	// 不存在的任务返回 404。
	w, resp = f.do(http.MethodPost, "/api/admin/jobs/no.such.job/run", superToken, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminJobsUpdateCron(t *testing.T) {
	f := newFixture(t)
	userID, token := f.user(t, "super_jobs_update", rbac.RoleCodeSuperAdmin)

	w, resp := f.do(http.MethodGet, "/api/admin/jobs", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	before := findJob(t, dataList(t, resp), testJobKey)["nextRunAt"].(string)

	// 非法 cron / 时区 → 40001。
	w, resp = f.do(http.MethodPut, "/api/admin/jobs/"+testJobKey, token,
		`{"enabled":true,"cronExpr":"not a cron","timezone":"Asia/Shanghai"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPut, "/api/admin/jobs/"+testJobKey, token,
		`{"enabled":true,"cronExpr":"*/15 * * * *","timezone":"Mars/Olympus"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	// 合法更新：cron、时区、nextRunAt 与 updatedBy 均更新。
	w, resp = f.do(http.MethodPut, "/api/admin/jobs/"+testJobKey, token,
		`{"enabled":true,"cronExpr":"*/15 * * * *","timezone":"UTC"}`)
	requireStatus(t, w, resp, http.StatusOK)
	updated := dataMap(t, resp)
	assert.Equal(t, "*/15 * * * *", updated["cronExpr"])
	assert.Equal(t, "UTC", updated["timezone"])
	assert.True(t, updated["enabled"].(bool))
	require.NotNil(t, updated["nextRunAt"])
	assert.NotEqual(t, before, updated["nextRunAt"].(string), "nextRunAt 应随 cron 变化")

	var workDirJob jobs.Job
	require.NoError(t, f.db.Where("key = ?", testJobKey).First(&workDirJob).Error)
	assert.EqualValues(t, userID, workDirJob.UpdatedBy)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		rbac.AuditActionSystemJobsManage, rbac.AuditTargetScheduledJob).
		Order("id DESC").First(&audit).Error)
	assert.EqualValues(t, userID, audit.ActorID)
	require.NotNil(t, audit.TargetID)
	assert.Equal(t, workDirJob.ID, *audit.TargetID)
	assert.Contains(t, audit.Summary, testJobKey)

	// 禁用时清空 nextRunAt。
	w, resp = f.do(http.MethodPut, "/api/admin/jobs/"+testJobKey, token,
		`{"enabled":false,"cronExpr":"0 4 * * *","timezone":"UTC"}`)
	requireStatus(t, w, resp, http.StatusOK)
	disabled := dataMap(t, resp)
	assert.False(t, disabled["enabled"].(bool))
	assert.Nil(t, disabled["nextRunAt"])

	// 不存在的任务返回 40400。
	w, resp = f.do(http.MethodPut, "/api/admin/jobs/no.such.job", token,
		`{"enabled":true,"cronExpr":"0 4 * * *","timezone":"UTC"}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminJobsRunHistory(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_jobs_history", rbac.RoleCodeSuperAdmin)

	w, resp := f.do(http.MethodPost, "/api/admin/jobs/"+testJobKey+"/run", token, "")
	requireStatus(t, w, resp, http.StatusAccepted)
	runID := responseInt64(t, dataMap(t, resp)["id"])

	// 按任务筛选的历史。
	w, resp = f.do(http.MethodGet, "/api/admin/jobs/"+testJobKey+"/runs", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	page := dataMap(t, resp)
	assert.EqualValues(t, 1, page["total"])
	items := page["items"].([]any)
	require.Len(t, items, 1)
	assert.EqualValues(t, runID, responseInt64(t, items[0].(map[string]any)["id"]))

	// 全部 run 列表支持 jobKey/status 过滤。
	w, resp = f.do(http.MethodGet, "/api/admin/job-runs?jobKey="+testJobKey+"&status=queued", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	page = dataMap(t, resp)
	assert.EqualValues(t, 1, page["total"])
	assert.Len(t, page["items"].([]any), 1)

	w, resp = f.do(http.MethodGet, "/api/admin/job-runs?status=failed", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 0, dataMap(t, resp)["total"])

	// 单个 run 详情。
	w, resp = f.do(http.MethodGet, "/api/admin/job-runs/"+intToString(runID), token, "")
	requireStatus(t, w, resp, http.StatusOK)
	detail := dataMap(t, resp)
	assert.EqualValues(t, runID, responseInt64(t, detail["id"]))
	assert.Equal(t, testJobKey, detail["jobKey"])

	// 直接在服务层执行 run，详情反映终态。
	require.NoError(t, f.svc.Execute(context.Background(), runID))
	w, resp = f.do(http.MethodGet, "/api/admin/job-runs/"+intToString(runID), token, "")
	requireStatus(t, w, resp, http.StatusOK)
	detail = dataMap(t, resp)
	assert.Equal(t, jobs.StatusSucceeded, detail["status"])
	require.NotNil(t, detail["finishedAt"])

	// 非法 ID / 不存在 → 40400。
	w, resp = f.do(http.MethodGet, "/api/admin/job-runs/abc", token, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])

	w, resp = f.do(http.MethodGet, "/api/admin/job-runs/999999", token, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminDifficultySettings(t *testing.T) {
	f := newFixture(t)

	w, resp := f.do(http.MethodGet, "/api/admin/difficulty/settings", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	_, contentToken := f.user(t, "content_difficulty_user", rbac.RoleCodeContentAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/difficulty/settings", contentToken, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	userID, token := f.user(t, "super_difficulty_user", rbac.RoleCodeSuperAdmin)

	// 默认值。
	w, resp = f.do(http.MethodGet, "/api/admin/difficulty/settings", token, "")
	requireStatus(t, w, resp, http.StatusOK)
	defaults := dataMap(t, resp)
	assert.True(t, defaults["enabled"].(bool))
	assert.EqualValues(t, 50, responseInt64(t, defaults["defaultScore"]))
	assert.EqualValues(t, 10, responseInt64(t, defaults["minSample"]))
	assert.EqualValues(t, 10, responseInt64(t, defaults["priorStrength"]))
	assert.EqualValues(t, 5, responseInt64(t, defaults["maxStep"]))
	assert.EqualValues(t, 365, responseInt64(t, defaults["windowDays"]))
	assert.Equal(t, false, defaults["includeContest"])
	assert.Equal(t, 1.5, defaults["curveGamma"])
	assert.Equal(t, true, defaults["aiAssessEnabled"])
	assert.EqualValues(t, 15, responseInt64(t, defaults["aiTimeoutSeconds"]))
	assert.NotEmpty(t, defaults["updatedAt"])

	// 合法更新 + 事务内审计。
	body := map[string]any{
		"enabled": false, "defaultScore": 60, "minSample": 20, "priorStrength": 15,
		"maxStep": 8, "windowDays": 30, "includeContest": true,
		"curveGamma": 2.0, "aiAssessEnabled": false, "aiTimeoutSeconds": 20,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w, resp = f.do(http.MethodPut, "/api/admin/difficulty/settings", token, string(raw))
	requireStatus(t, w, resp, http.StatusOK)
	updated := dataMap(t, resp)
	assert.False(t, updated["enabled"].(bool))
	assert.EqualValues(t, 60, responseInt64(t, updated["defaultScore"]))
	assert.EqualValues(t, 20, responseInt64(t, updated["minSample"]))
	assert.EqualValues(t, 15, responseInt64(t, updated["priorStrength"]))
	assert.EqualValues(t, 8, responseInt64(t, updated["maxStep"]))
	assert.EqualValues(t, 30, responseInt64(t, updated["windowDays"]))
	assert.Equal(t, true, updated["includeContest"])
	assert.Equal(t, 2.0, updated["curveGamma"])
	assert.Equal(t, false, updated["aiAssessEnabled"])
	assert.EqualValues(t, 20, responseInt64(t, updated["aiTimeoutSeconds"]))

	var row difficulty.Settings
	require.NoError(t, f.db.First(&row, 1).Error)
	assert.EqualValues(t, userID, row.UpdatedBy)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		rbac.AuditActionSystemJobsManage, rbac.AuditTargetDifficultySettings).
		Order("id DESC").First(&audit).Error)
	assert.EqualValues(t, userID, audit.ActorID)
	require.NotNil(t, audit.TargetID)
	assert.EqualValues(t, 1, *audit.TargetID)

	// 非法值 40001，且不写审计。
	var auditsBefore int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
		Where("target_type = ?", rbac.AuditTargetDifficultySettings).Count(&auditsBefore).Error)
	invalid := defaultSettingsBody()
	invalid["defaultScore"] = 0
	raw, err = json.Marshal(invalid)
	require.NoError(t, err)
	w, resp = f.do(http.MethodPut, "/api/admin/difficulty/settings", token, string(raw))
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	var auditsAfter int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
		Where("target_type = ?", rbac.AuditTargetDifficultySettings).Count(&auditsAfter).Error)
	assert.Equal(t, auditsBefore, auditsAfter, "校验失败不应写审计")

	// 无权限 PUT → 403。
	raw, err = json.Marshal(defaultSettingsBody())
	require.NoError(t, err)
	w, resp = f.do(http.MethodPut, "/api/admin/difficulty/settings", contentToken, string(raw))
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])
}

func TestAdminJobRunRespectsDifficultySettings(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_difficulty_run", rbac.RoleCodeSuperAdmin)

	disabled := defaultSettingsBody()
	disabled["enabled"] = false
	raw, err := json.Marshal(disabled)
	require.NoError(t, err)
	w, resp := f.do(http.MethodPut, "/api/admin/difficulty/settings", token, string(raw))
	requireStatus(t, w, resp, http.StatusOK)

	// 难度自动调整关闭时，手动触发难度任务返回 40001 且不创建 run。
	w, resp = f.do(http.MethodPost, "/api/admin/jobs/"+difficulty.JobKey+"/run", token, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Empty(t, f.enqueuer.snapshot())

	// 重新开启后可以正常触发。
	raw, err = json.Marshal(defaultSettingsBody())
	require.NoError(t, err)
	w, resp = f.do(http.MethodPut, "/api/admin/difficulty/settings", token, string(raw))
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodPost, "/api/admin/jobs/"+difficulty.JobKey+"/run", token, "")
	requireStatus(t, w, resp, http.StatusAccepted)
	assert.Equal(t, difficulty.JobKey, dataMap(t, resp)["jobKey"])
	assert.Len(t, f.enqueuer.snapshot(), 1)
}

func intToString(value int64) string {
	return strconv.FormatInt(value, 10)
}
