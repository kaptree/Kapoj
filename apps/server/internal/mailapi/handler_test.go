package mailapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/mailapi"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/secret"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type fakeSender struct {
	messages []mail.Message
}

func (s *fakeSender) Send(_ context.Context, msg mail.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

type mailFixture struct {
	t      *testing.T
	h      http.Handler
	db     *gorm.DB
	svc    *mail.Service
	sender *fakeSender
	tokens map[string]string
	nextN  int
}

func newMailFixture(t *testing.T) *mailFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g := testutil.PG(t)
	rbacSvc := testutil.SeedRBAC(t, g)
	keys, err := secret.NewKeyManager("", filepath.Join(t.TempDir(), "mail.key"))
	require.NoError(t, err)
	require.NoError(t, keys.EnsureKey())
	sender := &fakeSender{}
	svc := mail.NewService(mail.NewMemoryRepository(), keys,
		mail.WithSenderFactory(func(mail.Config) (mail.Sender, error) { return sender, nil }))

	issuer := auth.NewTokenIssuer(strings.Repeat("s", 32), time.Hour)
	r := gin.New()
	api := r.Group("/api", middleware.Authenticate(issuer))
	mailapi.RegisterRoutes(api, svc, rbacSvc)
	return &mailFixture{t: t, h: r, db: g, svc: svc, sender: sender, tokens: map[string]string{}}
}

func (f *mailFixture) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	f.t.Helper()
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

func (f *mailFixture) tokenForRole(roleCode string, perms []string) string {
	f.t.Helper()
	key := roleCode + "|" + strings.Join(perms, ",")
	if tok, ok := f.tokens[key]; ok {
		return tok
	}
	var role rbac.Role
	err := f.db.Where("code = ?", roleCode).First(&role).Error
	if err == gorm.ErrRecordNotFound {
		_, err = rbac.NewService(f.db).CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
			Code: roleCode, Name: roleCode, PermissionCodes: perms,
		})
		require.NoError(f.t, err)
	} else {
		require.NoError(f.t, err)
	}

	f.nextN++
	name := fmt.Sprintf("mailuser%d", f.nextN)
	repo := user.NewGormRepository(f.db)
	u := &user.User{Username: name, Nickname: name, Email: name + "@example.com", PasswordHash: "x", Role: user.RoleUser, Status: user.StatusActive}
	require.NoError(f.t, repo.Create(context.Background(), u))
	testutil.AssignRole(f.t, f.db, u.ID, roleCode)
	tok, err := auth.NewTokenIssuer(strings.Repeat("s", 32), time.Hour).IssueAccess(u)
	require.NoError(f.t, err)
	f.tokens[key] = tok
	return tok
}

func readyInput(password string) mail.SettingsInput {
	return mail.SettingsInput{
		Provider:                 mail.ProviderQQ,
		SMTPHost:                 "smtp.qq.com",
		SMTPPort:                 465,
		SMTPTLS:                  mail.TLSSSL,
		SMTPUsername:             "bot@qq.com",
		SMTPPassword:             &password,
		FromEmail:                "bot@qq.com",
		FromName:                 "kapOJ",
		PublicBaseURL:            "https://oj.example.com",
		SendingEnabled:           true,
		RequireEmailVerification: true,
	}
}

func TestAdminMailRequiresPermission(t *testing.T) {
	f := newMailFixture(t)

	w, _ := f.do(http.MethodGet, "/api/admin/mail", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	normal := f.tokenForRole("mail_normal", nil)
	w, _ = f.do(http.MethodGet, "/api/admin/mail", normal, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	manager := f.tokenForRole("mail_manager", []string{string(rbac.PermMailSettingsManage)})
	w, resp := f.do(http.MethodGet, "/api/admin/mail", manager, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "qq", data["provider"])
	assert.Equal(t, "none", data["passwordState"])
	assert.NotContains(t, w.Body.String(), "smtpPassword")
}

func TestAdminMailUpdateAndAudit(t *testing.T) {
	f := newMailFixture(t)
	manager := f.tokenForRole("mail_manager", []string{string(rbac.PermMailSettingsManage)})

	w, resp := f.do(http.MethodPut, "/api/admin/mail", manager, readyInput("secret"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.True(t, data["ready"].(bool))
	assert.Equal(t, "ready", data["passwordState"])
	assert.NotContains(t, w.Body.String(), "secret")

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionMailSettingsManage).Order("id DESC").First(&audit).Error)
	assert.Equal(t, rbac.AuditTargetMailSettings, audit.TargetType)

	// Password is not returned on subsequent reads.
	w, _ = f.do(http.MethodGet, "/api/admin/mail", manager, nil)
	assert.NotContains(t, w.Body.String(), "secret")
}

func TestAdminMailRejectsInvalidConfig(t *testing.T) {
	f := newMailFixture(t)
	manager := f.tokenForRole("mail_manager", []string{string(rbac.PermMailSettingsManage)})

	in := readyInput("secret")
	in.Provider = "bogus"
	w, resp := f.do(http.MethodPut, "/api/admin/mail", manager, in)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestAdminMailTestSend(t *testing.T) {
	f := newMailFixture(t)
	manager := f.tokenForRole("mail_manager", []string{string(rbac.PermMailSettingsManage)})

	// Not configured yet.
	w, resp := f.do(http.MethodPost, "/api/admin/mail/test", manager, map[string]string{"to": "alice@example.com"})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.EqualValues(t, 50301, resp["code"])

	w, _ = f.do(http.MethodPut, "/api/admin/mail", manager, readyInput("secret"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, _ = f.do(http.MethodPost, "/api/admin/mail/test", manager, map[string]string{"to": "alice@example.com"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, f.sender.messages, 1)
	assert.Equal(t, "alice@example.com", f.sender.messages[0].To)
	assert.Contains(t, f.sender.messages[0].Subject, "测试")
}
