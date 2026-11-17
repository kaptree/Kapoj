package adminapi_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

// 平台模型审计 action/target 是稳定契约，这里用字面量固定，避免常量误改。
const (
	auditActionAIPlatformCreate = "ai.platform_model.create"
	auditActionAIPlatformUpdate = "ai.platform_model.update"
	auditActionAIPlatformDelete = "ai.platform_model.delete"
	auditActionAIPlatformSelect = "ai.platform_model.select"
	auditTargetAIPlatformModel  = "ai_platform_model"
)

// fakePlatformChatClient 控制平台模型连通性测试的返回，不访问真实上游。
type fakePlatformChatClient struct {
	err error
}

func (f *fakePlatformChatClient) Chat(_ context.Context, _ ai.ChatRequest) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "pong", nil
}

type aiPlatformFixture struct {
	*fixture
	client *fakePlatformChatClient
}

// newAIPlatformFixture 构造挂载了平台模型接口的 adminapi 测试引擎；
// keyConfigured 控制是否加载测试加密密钥。
func newAIPlatformFixture(t *testing.T, keyConfigured bool) *aiPlatformFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	require.NoError(t, rbac.Seed(context.Background(), g))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	r.Use(middleware.Authenticate(issuer))
	svc := rbac.NewService(g)

	envKey := ""
	if keyConfigured {
		envKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	}
	keyMgr, err := ai.NewKeyManager(envKey, "")
	require.NoError(t, err)
	settings := ai.NewSettingsService(g, keyMgr, "")
	client := &fakePlatformChatClient{}
	platform := ai.NewPlatformService(g, keyMgr, client, settings)
	adminapi.RegisterRoutes(r.Group("/api"), svc, adminapi.Options{AI: settings, AIPlatform: platform})
	return &aiPlatformFixture{
		fixture: &fixture{engine: r, db: g, issuer: issuer, svc: svc},
		client:  client,
	}
}

// createPlatformModel 是测试辅助：创建平台模型并返回 ID。
func createPlatformModel(t *testing.T, f *aiPlatformFixture, tok, name, body string) int64 {
	t.Helper()
	w, resp := f.do(http.MethodPost, "/api/admin/ai/models", tok, body)
	requireStatus(t, w, resp, http.StatusCreated)
	data := dataMap(t, resp)
	assert.Equal(t, name, data["name"])
	return responseInt64(t, data["id"])
}

func TestAdminAIPlatformModelPermission(t *testing.T) {
	f := newAIPlatformFixture(t, true)

	// 未登录 401。
	w, resp := f.do(http.MethodGet, "/api/admin/ai/models", "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	// 无 ai.settings.manage（content_admin）403。
	_, contentTok := f.user(t, "ai_platform_content", rbac.RoleCodeContentAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", contentTok, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	// super_admin 200，空列表。
	_, superTok := f.user(t, "ai_platform_super", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", superTok, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.Empty(t, dataList(t, resp))

	// 拥有 ai.settings.manage 的自定义角色同样可用。
	role, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "ai_platform_manager", Name: "AI 平台模型管理员",
		PermissionCodes: []string{"ai.settings.manage"},
	})
	require.NoError(t, err)
	_, mgrTok := f.user(t, "ai_platform_manager_user", role.Code)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", mgrTok, "")
	requireStatus(t, w, resp, http.StatusOK)
}

func TestAdminAIPlatformModelCRUD(t *testing.T) {
	f := newAIPlatformFixture(t, true)
	_, tok := f.user(t, "ai_platform_crud", rbac.RoleCodeSuperAdmin)

	// 创建：201，返回脱敏视图，不含 token 明文/密文。
	w, resp := f.do(http.MethodPost, "/api/admin/ai/models", tok,
		`{"name":"Fake","baseUrl":"https://example.com/v1/","modelName":"fake-chat","apiKey":"sk-test-1234","enabled":true}`)
	requireStatus(t, w, resp, http.StatusCreated)
	created := dataMap(t, resp)
	id := responseInt64(t, created["id"])
	assert.Equal(t, "Fake", created["name"])
	assert.Equal(t, "https://example.com/v1", created["baseUrl"], "base URL 末尾斜杠应被去掉")
	assert.Equal(t, "fake-chat", created["modelName"])
	assert.Equal(t, true, created["enabled"])
	assert.Equal(t, "sk-…1234", created["apiKeyMasked"])
	assert.NotContains(t, w.Body.String(), "sk-test-1234", "响应不得包含 token 明文")
	assert.NotContains(t, w.Body.String(), "apiKeyEncrypted", "响应不得包含密文")

	// 密文落库，且不是明文。
	var row ai.PlatformModel
	require.NoError(t, f.db.First(&row, id).Error)
	assert.NotEmpty(t, row.APIKeyEncrypted)
	assert.NotContains(t, row.APIKeyEncrypted, "sk-test-1234")
	originalCiphertext := row.APIKeyEncrypted

	// 列表。
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	items := dataList(t, resp)
	require.Len(t, items, 1)
	listItem := items[0].(map[string]any)
	assert.EqualValues(t, id, responseInt64(t, listItem["id"]))
	assert.Equal(t, "Fake", listItem["name"])
	assert.NotContains(t, w.Body.String(), "sk-test-1234")

	// 更新元数据：apiKey 缺省表示保留原密文。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/ai/models/%d", id), tok,
		`{"name":"Fake 2","baseUrl":"https://example.com/v2","modelName":"fake-chat-2","enabled":false}`)
	requireStatus(t, w, resp, http.StatusOK)
	updated := dataMap(t, resp)
	assert.Equal(t, "Fake 2", updated["name"])
	assert.Equal(t, "https://example.com/v2", updated["baseUrl"])
	assert.Equal(t, "fake-chat-2", updated["modelName"])
	assert.Equal(t, false, updated["enabled"])
	assert.Equal(t, "sk-…1234", updated["apiKeyMasked"], "空 apiKey 应保留原密文")
	require.NoError(t, f.db.First(&row, id).Error)
	assert.Equal(t, originalCiphertext, row.APIKeyEncrypted, "空 apiKey 不应重写密文")

	// 更新 token：脱敏值变化。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/ai/models/%d", id), tok,
		`{"name":"Fake 2","baseUrl":"https://example.com/v2","modelName":"fake-chat-2","apiKey":"sk-new-9999","enabled":true}`)
	requireStatus(t, w, resp, http.StatusOK)
	assert.Equal(t, "sk-…9999", dataMap(t, resp)["apiKeyMasked"])
	require.NoError(t, f.db.First(&row, id).Error)
	assert.NotEqual(t, originalCiphertext, row.APIKeyEncrypted)
	assert.NotContains(t, row.APIKeyEncrypted, "sk-new-9999")

	// 未选择时删除：200，列表为空。
	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/ai/models/%d", id), tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.Empty(t, dataList(t, resp))

	// 不存在的模型：更新/删除 404。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/ai/models/%d", id), tok,
		`{"name":"Ghost","baseUrl":"https://example.com/v1","modelName":"ghost","enabled":true}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])

	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/ai/models/%d", id), tok, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])

	// 重名/空名称校验：40001。
	createPlatformModel(t, f, tok, "Dup",
		`{"name":"Dup","baseUrl":"https://example.com/v2","modelName":"dup-chat","apiKey":"sk-dup-1234","enabled":true}`)
	w, resp = f.do(http.MethodPost, "/api/admin/ai/models", tok,
		`{"name":"Dup","baseUrl":"https://example.com/v2","modelName":"dup-chat","apiKey":"sk-dup-1234","enabled":true}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	w, resp = f.do(http.MethodPost, "/api/admin/ai/models", tok,
		`{"name":"","baseUrl":"https://example.com/v2","modelName":"dup-chat","apiKey":"sk-dup-1234","enabled":true}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestAdminAIPlatformModelDeleteSelectedRejected(t *testing.T) {
	f := newAIPlatformFixture(t, true)
	_, tok := f.user(t, "ai_platform_select", rbac.RoleCodeSuperAdmin)

	id := createPlatformModel(t, f, tok, "Selected",
		`{"name":"Selected","baseUrl":"https://example.com/v1","modelName":"sel-chat","apiKey":"sk-sel-1234","enabled":true}`)

	// 选择当前模型：返回设置视图。
	w, resp := f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, fmt.Sprintf(`{"modelId":%d}`, id))
	requireStatus(t, w, resp, http.StatusOK)
	view := dataMap(t, resp)
	assert.EqualValues(t, id, view["platformModelId"])
	assert.Equal(t, "Selected", view["platformModelName"])

	// 删除当前使用中的模型：40001，模型保留。
	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/ai/models/%d", id), tok, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "正在使用")
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.Len(t, dataList(t, resp), 1)

	// 取消选择后允许删除。
	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, `{"modelId":null}`)
	requireStatus(t, w, resp, http.StatusOK)
	assert.Nil(t, dataMap(t, resp)["platformModelId"], "取消选择后 platformModelId 应为 null")

	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/ai/models/%d", id), tok, "")
	requireStatus(t, w, resp, http.StatusOK)

	// 选中不存在的模型：404。
	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, fmt.Sprintf(`{"modelId":%d}`, id))
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])

	// 禁用模型不可选择：40001。
	disabledID := createPlatformModel(t, f, tok, "Disabled",
		`{"name":"Disabled","baseUrl":"https://example.com/v1","modelName":"off-chat","apiKey":"sk-off-1234","enabled":false}`)
	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, fmt.Sprintf(`{"modelId":%d}`, disabledID))
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "未启用")
}

func TestAdminAIPlatformModelTestConnection(t *testing.T) {
	f := newAIPlatformFixture(t, true)
	_, tok := f.user(t, "ai_platform_test", rbac.RoleCodeSuperAdmin)

	id := createPlatformModel(t, f, tok, "Testable",
		`{"name":"Testable","baseUrl":"https://example.com/v1","modelName":"test-chat","apiKey":"sk-test-9999","enabled":true}`)

	// 成功：200 {ok:true}。
	w, resp := f.do(http.MethodPost, fmt.Sprintf("/api/admin/ai/models/%d/test", id), tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, true, data["ok"])
	assert.Contains(t, data["message"], "连接成功")
	assert.Contains(t, data, "latencyMs")

	// 上游失败：仍返回 200，ok=false，消息脱敏（不含 token/上游正文）。
	f.client.err = errors.New("upstream body sk-test-9999 exploded")
	w, resp = f.do(http.MethodPost, fmt.Sprintf("/api/admin/ai/models/%d/test", id), tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.Equal(t, false, data["ok"])
	assert.Equal(t, "连接失败", data["message"])
	assert.NotContains(t, w.Body.String(), "sk-test-9999")
	assert.NotContains(t, w.Body.String(), "exploded")

	// 连接测试不写审计（仅创建一条）。
	var count int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).
		Where("target_type = ?", auditTargetAIPlatformModel).Count(&count).Error)
	assert.Equal(t, int64(1), count, "测试连接不写审计")

	// 不存在：404。
	w, resp = f.do(http.MethodPost, "/api/admin/ai/models/999999/test", tok, "")
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}

func TestAdminAIPlatformModelAudit(t *testing.T) {
	f := newAIPlatformFixture(t, true)
	actorID, tok := f.user(t, "ai_platform_audit", rbac.RoleCodeSuperAdmin)

	id := createPlatformModel(t, f, tok, "审计模型",
		`{"name":"审计模型","baseUrl":"https://example.com/v1","modelName":"audit-chat","apiKey":"sk-audit-1234","enabled":true}`)

	w, resp := f.do(http.MethodPut, fmt.Sprintf("/api/admin/ai/models/%d", id), tok,
		`{"name":"审计模型 2","baseUrl":"https://example.com/v1","modelName":"audit-chat","enabled":true}`)
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, fmt.Sprintf(`{"modelId":%d}`, id))
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, `{"modelId":null}`)
	requireStatus(t, w, resp, http.StatusOK)

	w, resp = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/ai/models/%d", id), tok, "")
	requireStatus(t, w, resp, http.StatusOK)

	var logs []rbac.AdminAuditLog
	require.NoError(t, f.db.Where("target_type = ?", auditTargetAIPlatformModel).Order("id").Find(&logs).Error)
	require.Len(t, logs, 5, "create/update/select/unselect/delete 各一条")

	assert.Equal(t, auditActionAIPlatformCreate, logs[0].Action)
	assert.Nil(t, logs[0].TargetID, "create 回调早于插入，targetID 应为 0")
	assert.Contains(t, logs[0].Summary, "审计模型")
	assert.Equal(t, actorID, logs[0].ActorID)
	assert.Equal(t, testUserAgent, logs[0].UserAgent)

	assert.Equal(t, auditActionAIPlatformUpdate, logs[1].Action)
	require.NotNil(t, logs[1].TargetID)
	assert.EqualValues(t, id, *logs[1].TargetID)
	assert.Contains(t, logs[1].Summary, "审计模型 2")

	assert.Equal(t, auditActionAIPlatformSelect, logs[2].Action)
	require.NotNil(t, logs[2].TargetID)
	assert.EqualValues(t, id, *logs[2].TargetID)

	assert.Equal(t, auditActionAIPlatformSelect, logs[3].Action)
	assert.Nil(t, logs[3].TargetID, "取消选择 targetID 应为 0")

	assert.Equal(t, auditActionAIPlatformDelete, logs[4].Action)
	require.NotNil(t, logs[4].TargetID)
	assert.EqualValues(t, id, *logs[4].TargetID)

	for _, log := range logs {
		assert.Equal(t, actorID, log.ActorID)
		assert.NotContains(t, log.Summary, "sk-audit-1234")
	}
}

func TestAdminAIPlatformModelNoKey(t *testing.T) {
	f := newAIPlatformFixture(t, false)
	_, tok := f.user(t, "ai_platform_nokey", rbac.RoleCodeSuperAdmin)

	// 创建需要加密 token：40002。
	w, resp := f.do(http.MethodPost, "/api/admin/ai/models", tok,
		`{"name":"NoKey","baseUrl":"https://example.com/v1","modelName":"nokey-chat","apiKey":"sk-nokey-1234","enabled":true}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40002, resp["code"])

	// 未写入任何模型。
	w, resp = f.do(http.MethodGet, "/api/admin/ai/models", tok, "")
	requireStatus(t, w, resp, http.StatusOK)
	assert.Empty(t, dataList(t, resp))

	// 测试连接缺密钥：40002（先于模型存在性检查）。
	w, resp = f.do(http.MethodPost, "/api/admin/ai/models/1/test", tok, "")
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40002, resp["code"])

	// 选择不存在的模型不涉及加密密钥，返回 404。
	w, resp = f.do(http.MethodPut, "/api/admin/ai/platform-model", tok, `{"modelId":1}`)
	requireStatus(t, w, resp, http.StatusNotFound)
	assert.EqualValues(t, 40400, resp["code"])
}
