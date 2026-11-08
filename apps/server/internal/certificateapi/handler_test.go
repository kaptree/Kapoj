package certificateapi_test

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

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, int64) error { return nil }

type fx struct {
	t            *testing.T
	h            http.Handler
	db           *gorm.DB
	rbac         *rbac.Service
	certRoleCode string
	tokens       map[string]string
	ids          map[string]int64
	nextN        int
}

func newFx(t *testing.T) *fx {
	t.Helper()
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	require.NoError(t, certificate.SeedTypes(context.Background(), g))

	// 授予 certificate.manage 权限的自定义角色，供测试管理员使用（内置角色均不含该权限）。
	certRole, err := rbacSvc.CreateRole(context.Background(), rbac.AuditMeta{}, rbac.RoleInput{
		Code: "cert_manage", Name: "证书管理", PermissionCodes: []string{"certificate.manage"},
	})
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	certSvc := certificate.NewService(certificate.NewGormRepository(g), store)
	h := router.New(router.Deps{
		DB:      g,
		Redis:   redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage: store,
		Config: &config.Config{
			JWTSecret:       strings.Repeat("s", 32),
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: time.Hour,
			SubmitCooldown:  time.Second,
		},
		Enqueuer:    noopQueue{},
		RBAC:        rbacSvc,
		Certificate: certSvc,
	})
	return &fx{
		t: t, h: h, db: g, rbac: rbacSvc, certRoleCode: certRole.Code,
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

func (f *fx) token(role string) string {
	f.t.Helper()
	if tok, ok := f.tokens[role]; ok {
		return tok
	}
	f.nextN++
	name := fmt.Sprintf("%s%d", strings.ReplaceAll(role, "_", ""), f.nextN)
	_, tok := f.register(name)
	f.tokens[role] = tok
	return tok
}

// certManageToken 创建并登录一个拥有 certificate.manage 权限的用户。
func (f *fx) certManageToken() string {
	f.t.Helper()
	const key = "cert.manage"
	if tok, ok := f.tokens[key]; ok {
		return tok
	}
	f.nextN++
	name := fmt.Sprintf("certmgr%d", f.nextN)
	userID, tok := f.register(name)
	testutil.AssignRole(f.t, f.db, userID, f.certRoleCode)
	f.tokens[key] = tok
	f.ids[key] = userID
	return tok
}

func itemsOf(t *testing.T, resp map[string]any) []any {
	t.Helper()
	items, ok := resp["data"].([]any)
	require.True(t, ok, "expected data array, got %T", resp["data"])
	return items
}

// pngBytes 以合法 PNG magic bytes 开头，http.DetectContentType 会识别为 image/png。
var pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}

// multipart 构造 multipart/form-data 请求；fileField 为空时省略文件字段。
func (f *fx) multipart(path, token string, fields map[string]string, fileField, filename string, data []byte) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(f.t, mw.WriteField(k, v))
	}
	if fileField != "" {
		part, err := mw.CreateFormFile(fileField, filename)
		require.NoError(f.t, err)
		_, err = part.Write(data)
		require.NoError(f.t, err)
	}
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

// getRaw 发送带 token 的 GET 并返回原始响应（用于图片等二进制响应）。
func (f *fx) getRaw(path, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	return w
}

// activeTypeID 返回一个启用中的证书类型 ID。
func (f *fx) activeTypeID() int64 {
	f.t.Helper()
	var typ certificate.CertificateType
	require.NoError(f.t, f.db.Where("is_active = ?", true).First(&typ).Error)
	return typ.ID
}

func TestPublicCertificateTypes(t *testing.T) {
	f := newFx(t)

	// 无 token 200，只返回 active（seed 的 7 条均为 active）。
	w, resp := f.do(http.MethodGet, "/api/certificate-types", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := itemsOf(t, resp)
	require.Len(t, items, 7)
	assert.Equal(t, "蓝桥杯", items[0].(map[string]any)["name"])
	assert.Equal(t, true, items[0].(map[string]any)["isActive"])

	// 停用一条后公开列表不含它。
	require.NoError(t, f.db.Model(&certificate.CertificateType{}).
		Where("name = ?", "NOIP").Update("is_active", false).Error)
	w, resp = f.do(http.MethodGet, "/api/certificate-types", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items = itemsOf(t, resp)
	require.Len(t, items, 6)
	for _, raw := range items {
		assert.Equal(t, true, raw.(map[string]any)["isActive"])
	}
}

func TestAdminCertificateTypesPermission(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	adminToken := f.certManageToken()

	// 无 token 401。
	w, _ := f.do(http.MethodGet, "/api/admin/certificate-types", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 普通用户 403。
	w, _ = f.do(http.MethodGet, "/api/admin/certificate-types", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 有权限用户：GET 200（全部类型，含停用）。
	require.NoError(t, f.db.Model(&certificate.CertificateType{}).
		Where("name = ?", "NOIP").Update("is_active", false).Error)
	w, resp := f.do(http.MethodGet, "/api/admin/certificate-types", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, itemsOf(t, resp), 7)

	// POST 201。
	w, resp = f.do(http.MethodPost, "/api/admin/certificate-types", adminToken, map[string]any{
		"name": "自定义竞赛", "description": "描述", "sortOrder": 8,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := resp["data"].(map[string]any)
	assert.Equal(t, true, created["isActive"])
	typeID := int64(created["id"].(float64))

	// PUT 200（显式停用）。
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/admin/certificate-types/%d", typeID), adminToken, map[string]any{
		"name": "自定义竞赛（改）", "sortOrder": 9, "isActive": false,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, false, resp["data"].(map[string]any)["isActive"])

	// DELETE 200（无引用）。
	w, _ = f.do(http.MethodDelete, fmt.Sprintf("/api/admin/certificate-types/%d", typeID), adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestAdminCertificateTypeValidationAndAudit(t *testing.T) {
	f := newFx(t)
	adminToken := f.certManageToken()

	// 重复名称 40001（seed 已内置「蓝桥杯」）。
	w, resp := f.do(http.MethodPost, "/api/admin/certificate-types", adminToken, map[string]any{
		"name": "蓝桥杯", "sortOrder": 99,
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 成功后写审计：action=certificate.manage、target=certificate_type。
	w, _ = f.do(http.MethodPost, "/api/admin/certificate-types", adminToken, map[string]any{
		"name": "新赛事", "sortOrder": 10,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ? AND target_type = ?",
		"certificate.manage", "certificate_type").Order("id DESC").First(&audit).Error)
	assert.Equal(t, "certificate_type", audit.TargetType)
	assert.NotZero(t, audit.ActorID)
}

func TestDeleteTypeInUseHTTP(t *testing.T) {
	f := newFx(t)
	adminToken := f.certManageToken()

	var typ certificate.CertificateType
	require.NoError(t, f.db.Where("name = ?", "NOI").First(&typ).Error)

	require.NoError(t, f.db.Create(&certificate.Certificate{
		UserID:              1,
		CertificateTypeID:   typ.ID,
		CertificateTypeName: "NOI",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}).Error)

	w, resp := f.do(http.MethodDelete, fmt.Sprintf("/api/admin/certificate-types/%d", typ.ID), adminToken, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "已有证书使用该类型")
}

func TestSubmitCertificateHTTP(t *testing.T) {
	f := newFx(t)
	userToken := f.token("user")
	typeID := f.activeTypeID()

	// 游客 401。
	w, _ := f.multipart("/api/certificates", "", map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
	}, "file", "c.png", pngBytes)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())

	// 缺文件字段 40001。
	w, resp := f.multipart("/api/certificates", userToken, map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
	}, "", "", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法 typeId 40001。
	w, resp = f.multipart("/api/certificates", userToken, map[string]string{
		"typeId": "abc", "level": "省一",
	}, "file", "c.png", pngBytes)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 成功 201。
	w, resp = f.multipart("/api/certificates", userToken, map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
	}, "file", "c.png", pngBytes)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "pending", data["status"])
	assert.Equal(t, "省一", data["level"])
	assert.NotEmpty(t, data["imageUrl"])

	// 文件超 5 MiB -> 40001（body 仍在 uploads 40 MiB 上限内，命中服务层 5 MiB 校验）。
	w, resp = f.multipart("/api/certificates", userToken, map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
	}, "file", "big.png", make([]byte, 5<<20+1))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, resp["message"], "5 MiB")
}

func TestMyCertificatesHTTP(t *testing.T) {
	f := newFx(t)
	userA := f.token("usera")
	userB := f.token("userb")
	typeID := f.activeTypeID()

	for i := 0; i < 2; i++ {
		w, _ := f.multipart("/api/certificates", userA, map[string]string{
			"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
		}, "file", "c.png", pngBytes)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}
	w, _ := f.multipart("/api/certificates", userB, map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "国一",
	}, "file", "c.png", pngBytes)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// userA 只返回本人 2 张。
	w, resp := f.do(http.MethodGet, "/api/certificates/my", userA, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.EqualValues(t, 2, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 2)
	for _, it := range items {
		assert.Equal(t, "省一", it.(map[string]any)["level"])
	}

	// 分页。
	w, resp = f.do(http.MethodGet, "/api/certificates/my?page=1&page_size=1", userA, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.EqualValues(t, 2, data["total"])
	assert.Len(t, data["items"].([]any), 1)

	// 无 token 401。
	w, _ = f.do(http.MethodGet, "/api/certificates/my", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCertificateImageHTTP(t *testing.T) {
	f := newFx(t)
	ownerToken := f.token("owner")
	otherToken := f.token("other")
	adminToken := f.certManageToken()
	typeID := f.activeTypeID()

	w, resp := f.multipart("/api/certificates", ownerToken, map[string]string{
		"typeId": fmt.Sprintf("%d", typeID), "level": "省一",
	}, "file", "c.png", pngBytes)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	certID := int64(resp["data"].(map[string]any)["id"].(float64))
	imageURL := fmt.Sprintf("/api/certificates/%d/image", certID)

	// owner 200 且 Content-Type 正确。
	w = f.getRaw(imageURL, ownerToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, pngBytes, w.Body.Bytes())

	// 其他用户 404。
	w, _ = f.do(http.MethodGet, imageURL, otherToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 管理员 200。
	w = f.getRaw(imageURL, adminToken)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, pngBytes, w.Body.Bytes())

	// 无 token 401。
	w, _ = f.do(http.MethodGet, imageURL, "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 不存在 404。
	w, _ = f.do(http.MethodGet, "/api/certificates/999999/image", ownerToken, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPublicVerifyHTTP(t *testing.T) {
	f := newFx(t)
	typeID := f.activeTypeID()

	userID, _ := f.register("verifyuser")
	require.NoError(t, f.db.Model(&user.User{}).Where("id = ?", userID).Update("nickname", "核验用户").Error)

	code := "KAPOJ-ABCD-EFGH"
	approvedAt := time.Now()
	require.NoError(t, f.db.Create(&certificate.Certificate{
		UserID:              userID,
		CertificateTypeID:   typeID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusApproved,
		VerifyCode:          &code,
		SubmittedAt:         time.Now(),
		ReviewedAt:          &approvedAt,
	}).Error)

	// 无 token 200，返回 valid + 类型/等级/昵称/通过时间。
	w, resp := f.do(http.MethodGet, "/api/certificates/verify?code="+code, "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["valid"])
	assert.Equal(t, "蓝桥杯", data["type"])
	assert.Equal(t, "省一", data["level"])
	assert.Equal(t, "核验用户", data["nickname"])

	// 公开核验不返回图片/用户 ID/邮箱/驳回原因等敏感字段。
	for _, key := range []string{"imageUrl", "userId", "email", "rejectReason", "reviewedBy", "verifyCode"} {
		_, ok := data[key]
		assert.False(t, ok, "公开核验不应包含 %s", key)
	}

	// 未知验证码 200 且 valid:false，且不含类型/等级/昵称。
	w, resp = f.do(http.MethodGet, "/api/certificates/verify?code=KAPOJ-ZZZZ-ZZZZ", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, false, data["valid"])
	for _, key := range []string{"type", "level", "nickname", "approvedAt"} {
		_, ok := data[key]
		assert.False(t, ok, "无效核验不应包含 %s", key)
	}
}
