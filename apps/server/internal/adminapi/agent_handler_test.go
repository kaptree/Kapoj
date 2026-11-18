package adminapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func newAgentAdminFixture(t *testing.T) (*fixture, *agent.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	rbacSvc := rbac.NewService(g)
	agentSvc := agent.NewService(g,
		problem.NewGormRepository(g), contest.NewGormRepository(g), nil, nil)
	adminapi.RegisterRoutes(r.Group("/api"), rbacSvc, adminapi.Options{Agent: agentSvc})
	return &fixture{engine: r, db: g, issuer: issuer, svc: rbacSvc}, agentSvc
}

func createAgentForAdminTest(
	t *testing.T, svc *agent.Service, ownerID int64, name, model string,
) *agent.AgentView {
	t.Helper()
	view, err := svc.CreateAgent(context.Background(), ownerID, agent.AgentInput{
		Name: name, ModelName: model, ModelIntro: "admin test",
	})
	require.NoError(t, err)
	return view
}

func TestAdminAgentRoutesRequirePermission(t *testing.T) {
	f, _ := newAgentAdminFixture(t)

	endpoints := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/agents", ""},
		{http.MethodGet, "/api/admin/agents/1/submissions", ""},
		{http.MethodPut, "/api/admin/agents/1", `{"status":"disabled"}`},
	}
	for _, endpoint := range endpoints {
		w, resp := f.do(endpoint.method, endpoint.path, "", endpoint.body)
		requireStatus(t, w, resp, http.StatusUnauthorized)
		assert.EqualValues(t, 40100, resp["code"], endpoint.path)
	}

	_, plainToken := f.user(t, "agent_plain")
	for _, endpoint := range endpoints {
		w, resp := f.do(endpoint.method, endpoint.path, plainToken, endpoint.body)
		requireStatus(t, w, resp, http.StatusForbidden)
		assert.EqualValues(t, 40300, resp["code"], endpoint.path)
	}

	// auditor 有审计与用户只读权限，但没有 agent.manage。
	_, auditorToken := f.user(t, "agent_auditor", rbac.RoleCodeAuditor)
	for _, endpoint := range endpoints {
		w, resp := f.do(endpoint.method, endpoint.path, auditorToken, endpoint.body)
		requireStatus(t, w, resp, http.StatusForbidden)
		assert.EqualValues(t, 40300, resp["code"], endpoint.path)
	}

	_, superToken := f.user(t, "agent_super", rbac.RoleCodeSuperAdmin)
	w, resp := f.do(http.MethodGet, "/api/admin/agents", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 0, dataMap(t, resp)["total"])

	w, resp = f.do(http.MethodPut, "/api/admin/agents/999999", superToken, `{"status":"disabled"}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminAgentListFiltersViaAPI(t *testing.T) {
	f, agentSvc := newAgentAdminFixture(t)
	_, superToken := f.user(t, "agent_list_super", rbac.RoleCodeSuperAdmin)
	ownerID, _ := f.user(t, "agent_list_owner")
	otherID, _ := f.user(t, "agent_list_other")

	alpha := createAgentForAdminTest(t, agentSvc, ownerID, "List Alpha", "gpt-5")
	beta := createAgentForAdminTest(t, agentSvc, otherID, "List Beta", "claude-4")
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", beta.ID).
		Update("status", agent.StatusDisabled).Error)

	w, resp := f.do(http.MethodGet, "/api/admin/agents", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 2, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.Equal(t, "List Alpha", first["name"])
	assert.Equal(t, "agent_list_owner", first["author"].(map[string]any)["username"])
	assert.Contains(t, first, "status")
	assert.Contains(t, first, "rating")

	w, resp = f.do(http.MethodGet, "/api/admin/agents?status=disabled", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"])
	assert.Equal(t, "List Beta", data["items"].([]any)[0].(map[string]any)["name"])

	w, resp = f.do(http.MethodGet, "/api/admin/agents?model=gpt-5", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"])
	assert.Equal(t, alpha.ID, int64(data["items"].([]any)[0].(map[string]any)["id"].(float64)))

	w, resp = f.do(http.MethodGet, "/api/admin/agents?page=1&page_size=1", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 2, data["total"])
	require.Len(t, data["items"].([]any), 1)

	w, resp = f.do(http.MethodGet, "/api/admin/agents?status=disabled&model=gpt-5", superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.EqualValues(t, 0, dataMap(t, resp)["total"])
}

func TestAdminAgentSubmissionsViaAPI(t *testing.T) {
	f, agentSvc := newAgentAdminFixture(t)
	_, superToken := f.user(t, "agent_submissions_super", rbac.RoleCodeSuperAdmin)
	ownerID, _ := f.user(t, "agent_submissions_owner")
	created := createAgentForAdminTest(t, agentSvc, ownerID, "Submissions Agent", "gpt-5")

	p := &problem.Problem{
		Title: "Admin Submission Problem", Slug: "admin-submission-problem",
		Type: problem.TypeTraditional, TimeLimitMS: 1000, MemoryLimitMB: 256,
		IsPublic: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, f.db.Create(p).Error)
	judgedAt := time.Now().Add(time.Minute)
	row := &submission.Submission{
		UserID: ownerID, ProblemID: p.ID, AgentID: &created.ID,
		Language: submission.LanguageCPP, Code: "int main() { return 0; }",
		Status: submission.StatusAccepted, TimeUsedMS: 42, MemoryUsedKB: 512,
		Score: 100, AgentSettled: true, CreatedAt: time.Now(), JudgedAt: &judgedAt,
	}
	require.NoError(t, f.db.Create(row).Error)

	w, resp := f.do(http.MethodGet,
		fmt.Sprintf("/api/admin/agents/%d/submissions", created.ID), superToken, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 1, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.EqualValues(t, row.ID, item["id"])
	assert.EqualValues(t, p.ID, item["problemId"])
	assert.Equal(t, "Admin Submission Problem", item["problemTitle"])
	assert.Equal(t, "admin-submission-problem", item["problemSlug"])
	assert.Equal(t, "cpp", item["language"])
	assert.Equal(t, "accepted", item["status"])
	assert.EqualValues(t, 42, item["timeUsedMs"])
	assert.EqualValues(t, 512, item["memoryUsedKb"])
	assert.EqualValues(t, 100, item["score"])
	assert.Equal(t, "int main() { return 0; }", item["code"])
	assert.Equal(t, "", item["answer"])
	assert.True(t, item["agentSettled"].(bool))
	require.NotNil(t, item["judgedAt"])

	w, resp = f.do(http.MethodGet, "/api/admin/agents/999999/submissions", superToken, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminAgentUpdateDisablesEnablesAndResets(t *testing.T) {
	f, agentSvc := newAgentAdminFixture(t)
	ctx := context.Background()
	superID, superToken := f.user(t, "agent_update_super", rbac.RoleCodeSuperAdmin)
	ownerID, _ := f.user(t, "agent_update_owner")
	created := createAgentForAdminTest(t, agentSvc, ownerID, "Update Agent", "gpt-5")

	issued, err := agentSvc.GenerateToken(ctx, ownerID)
	require.NoError(t, err)
	_, err = agentSvc.AuthenticateToken(ctx, issued.Token)
	require.NoError(t, err)

	w, resp := f.do(http.MethodPut, fmt.Sprintf("/api/admin/agents/%d", created.ID), superToken,
		`{"status":"disabled"}`)
	requireStatus(t, w, resp, http.StatusOK)
	updated := dataMap(t, resp)
	assert.Equal(t, "disabled", updated["status"])
	assert.Equal(t, "Update Agent", updated["name"])
	assert.Equal(t, "agent_update_owner", updated["author"].(map[string]any)["username"])

	var stored agent.Agent
	require.NoError(t, f.db.First(&stored, created.ID).Error)
	assert.Equal(t, agent.StatusDisabled, stored.Status)

	var activeTokens int64
	require.NoError(t, f.db.Model(&agent.AgentToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", created.ID).Count(&activeTokens).Error)
	assert.Zero(t, activeTokens)

	// The disabled Agent's token stops working immediately.
	_, err = agentSvc.AuthenticateToken(ctx, issued.Token)
	requireAppErrorCode(t, err, 40102)
	_, err = agentSvc.GenerateToken(ctx, ownerID)
	requireAppErrorCode(t, err, 40304)

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_id = ?",
		rbac.AuditActionAgentManage, created.ID).Order("id ASC").First(&audit).Error)
	assert.Equal(t, superID, audit.ActorID)
	assert.Equal(t, rbac.AuditTargetAgent, audit.TargetType)
	assert.Equal(t, "203.0.113.10", audit.IP)
	assert.Equal(t, testUserAgent, audit.UserAgent)

	// Restoring only sets the status; the old token remains revoked.
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/agents/%d", created.ID), superToken,
		`{"status":"active"}`)
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "active", dataMap(t, resp)["status"])
	_, err = agentSvc.AuthenticateToken(ctx, issued.Token)
	requireAppErrorCode(t, err, 40102)

	fresh, err := agentSvc.GenerateToken(ctx, ownerID)
	require.NoError(t, err)
	_, err = agentSvc.AuthenticateToken(ctx, fresh.Token)
	require.NoError(t, err)

	// resetRating is explicit; an empty update returns 400 without touching the
	// Agent or writing an audit row.
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", created.ID).
		Update("rating", 1700).Error)
	var auditsBefore int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
		Where("action = ? AND target_id = ?", rbac.AuditActionAgentManage, created.ID).
		Count(&auditsBefore).Error)
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/agents/%d", created.ID), superToken, `{}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	var afterEmpty agent.Agent
	require.NoError(t, f.db.First(&afterEmpty, created.ID).Error)
	assert.Equal(t, agent.StatusActive, afterEmpty.Status)
	assert.Equal(t, 1700, afterEmpty.Rating)
	var auditsAfter int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
		Where("action = ? AND target_id = ?", rbac.AuditActionAgentManage, created.ID).
		Count(&auditsAfter).Error)
	assert.Equal(t, auditsBefore, auditsAfter, "a rejected empty update must not write an audit row")

	var resets int64
	require.NoError(t, f.db.Model(&agent.AgentRatingHistory{}).
		Where("agent_id = ? AND kind = ?", created.ID, agent.RatingReset).Count(&resets).Error)
	assert.Zero(t, resets)

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/agents/%d", created.ID), superToken,
		`{"resetRating":true}`)
	requireStatus(t, w, resp, http.StatusOK)
	updated = dataMap(t, resp)
	assert.Equal(t, "active", updated["status"])
	assert.EqualValues(t, 1500, updated["rating"])

	var reset agent.AgentRatingHistory
	require.NoError(t, f.db.Where("agent_id = ? AND kind = ?", created.ID, agent.RatingReset).
		Order("id DESC").First(&reset).Error)
	assert.Zero(t, reset.ProblemID)
	assert.Zero(t, reset.Difficulty)
	assert.Equal(t, 1700, reset.RatingBefore)
	assert.Equal(t, 1500, reset.RatingAfter)
	assert.Equal(t, -200, reset.Delta)
	assert.Zero(t, reset.SolveTimeMS)

	_, err = agentSvc.AuthenticateToken(ctx, issued.Token)
	requireAppErrorCode(t, err, 40102)

	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/agents/%d", created.ID), superToken,
		`{"status":"banned"}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPut, "/api/admin/agents/999999", superToken, `{"status":"disabled"}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}
