package aiapi

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
)

// legacyUserModelNameMaxLen 与 ai 包的模型名称校验保持一致。
const legacyUserModelNameMaxLen = 100

// currentUserID 返回当前登录用户 ID；解析失败时已写入 401 响应。
func currentUserID(c *gin.Context) (int64, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok || claims == nil {
		httpx.Fail(c, httpx.ErrUnauthorized)
		return 0, false
	}
	return claims.UserID, true
}

// userModelPathID 解析 /models/:id 路径参数；非法 ID 按资源不存在处理。
func userModelPathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return 0, false
	}
	return id, true
}

func (h *handler) listUserModels(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	view, err := h.svc.ListUserModels(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) createUserModel(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[ai.UserModelInput](c)
	if !ok {
		return
	}
	view, err := h.svc.CreateUserModel(c.Request.Context(), userID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, view)
}

func (h *handler) updateUserModel(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := userModelPathID(c)
	if !ok {
		return
	}
	in, ok := httpx.BindJSON[ai.UserModelInput](c)
	if !ok {
		return
	}
	view, err := h.svc.UpdateUserModel(c.Request.Context(), userID, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *handler) deleteUserModel(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := userModelPathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteUserModel(c.Request.Context(), userID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nil)
}

func (h *handler) setActiveUserModel(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := userModelPathID(c)
	if !ok {
		return
	}
	view, err := h.svc.SetActiveUserModel(c.Request.Context(), userID, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// testUserModel 处理连通性测试。上游失败仍返回 200 和 {ok:false,...}；
// 缺少平台密钥（40002）或模型不存在（404）等配置/查找错误才返回错误响应。
func (h *handler) testUserModel(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := userModelPathID(c)
	if !ok {
		return
	}
	result, err := h.svc.TestUserModel(c.Request.Context(), userID, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// activeUserModel 从列表视图中取出当前 active 行。
func activeUserModel(view *ai.UserModelsView) *ai.UserModelView {
	if view == nil || view.ActiveModelID == nil {
		return nil
	}
	for i := range view.Models {
		if view.Models[i].ID == *view.ActiveModelID {
			return &view.Models[i]
		}
	}
	return nil
}

// providerCodeForModel 按 base URL 与协议反查内置 provider；自定义模型返回空。
func providerCodeForModel(providers []ai.ProviderOption, model ai.UserModelView) string {
	for _, provider := range providers {
		if provider.Protocol == model.Protocol && provider.BaseURL == model.BaseURL {
			return provider.Code
		}
	}
	return ""
}

// legacyUserModelName 生成旧接口创建模型时的名称（provider / model），并按
// 模型名称上限截断，避免旧客户端在超长模型名时无法保存。
func legacyUserModelName(provider ai.ProviderOption, modelName string) string {
	name := strings.TrimSpace(provider.Name) + " / " + modelName
	if utf8.RuneCountInString(name) <= legacyUserModelNameMaxLen {
		return name
	}
	return string([]rune(name)[:legacyUserModelNameMaxLen])
}
