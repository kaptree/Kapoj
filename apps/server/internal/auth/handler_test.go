package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type fakeLoginHook struct {
	userIDs []int64
	err     error
}

func (h *fakeLoginHook) OnLogin(_ context.Context, userID int64) error {
	h.userIDs = append(h.userIDs, userID)
	return h.err
}

func newAuthEngine(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc, _ := newService(t)
	r := gin.New()
	RegisterRoutes(r.Group("/api"), svc)
	return r
}

func postJSON(r *gin.Engine, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

// authJSON performs a request with an optional bearer token.
func authJSON(r *gin.Engine, method, path, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

// authenticateForTest mirrors middleware.Authenticate without importing it (the
// auth package would otherwise form an import cycle with middleware).
func authenticateForTest(issuer *TokenIssuer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			c.Next()
			return
		}
		claims, err := issuer.ParseAccess(strings.TrimSpace(token))
		if err != nil {
			httpx.Fail(c, httpx.ErrUnauthorized)
			return
		}
		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

// newAccountEngine mounts the public auth routes plus the authenticated account
// routes, mirroring router.New.
func newAccountEngine(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc, _ := newService(t)
	issuer := NewTokenIssuer(testSecret, 15*time.Minute)
	r := gin.New()
	RegisterRoutes(r.Group("/api"), svc)
	api := r.Group("/api", authenticateForTest(issuer))
	RegisterAccountRoutes(api, svc)
	return r
}

func TestRegisterEndpoint(t *testing.T) {
	r := newAuthEngine(t)
	w, resp := postJSON(r, "/api/auth/register",
		`{"username":"Alice","email":"alice@example.com","password":"correct-horse"}`)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.NotEmpty(t, data["accessToken"])
	assert.NotEmpty(t, data["refreshToken"])
	assert.NotEmpty(t, data["sessionId"])
	u := data["user"].(map[string]any)
	assert.Equal(t, "alice", u["username"])
	assert.Equal(t, "alice@example.com", u["email"])
	assert.NotContains(t, w.Body.String(), "passwordHash")
	assert.NotContains(t, w.Body.String(), "PasswordHash")
}

func TestLoginHookFiresOnRegisterAndLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newService(t)
	hook := &fakeLoginHook{}
	r := gin.New()
	RegisterRoutes(r.Group("/api"), svc, hook)

	w, resp := postJSON(r, "/api/auth/register",
		`{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	registerID := int64(resp["data"].(map[string]any)["user"].(map[string]any)["id"].(float64))
	require.Len(t, hook.userIDs, 1)
	assert.Equal(t, registerID, hook.userIDs[0])

	w, _ = postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, hook.userIDs, 2)
	assert.Equal(t, registerID, hook.userIDs[1])
}

func TestLoginHookFailureDoesNotFailAuthResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newService(t)
	hook := &fakeLoginHook{err: errors.New("redis down")}
	r := gin.New()
	RegisterRoutes(r.Group("/api"), svc, hook)

	w, _ := postJSON(r, "/api/auth/register",
		`{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w, _ = postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Len(t, hook.userIDs, 2)
}

func TestRegisterMalformedJSON(t *testing.T) {
	r := newAuthEngine(t)
	w, resp := postJSON(r, "/api/auth/register", `{"username":`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40000, resp["code"])
}

func TestLoginRefreshLogoutFlow(t *testing.T) {
	r := newAuthEngine(t)
	postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)

	w, resp := postJSON(r, "/api/auth/login", `{"account":"alice@example.com","password":"correct-horse"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	login := resp["data"].(map[string]any)
	refresh := login["refreshToken"].(string)
	sessionID := login["sessionId"].(string)
	assert.NotEmpty(t, sessionID)

	w, resp = postJSON(r, "/api/auth/refresh", `{"refreshToken":"`+refresh+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rotated := resp["data"].(map[string]any)["refreshToken"].(string)
	assert.NotEqual(t, refresh, rotated)
	assert.Equal(t, sessionID, resp["data"].(map[string]any)["sessionId"], "refresh keeps the session")

	w, _ = postJSON(r, "/api/auth/logout", `{"refreshToken":"`+rotated+`"}`)
	require.Equal(t, http.StatusOK, w.Code)

	w, resp = postJSON(r, "/api/auth/refresh", `{"refreshToken":"`+rotated+`"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40100, resp["code"])
}

func TestLoginWrongPasswordEndpoint(t *testing.T) {
	r := newAuthEngine(t)
	postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	w, resp := postJSON(r, "/api/auth/login", `{"account":"alice","password":"nope-nope"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.EqualValues(t, 40101, resp["code"])
}

func TestPasswordEndpoint(t *testing.T) {
	r := newAccountEngine(t)
	_, resp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	token := resp["data"].(map[string]any)["accessToken"].(string)

	// Unauthenticated.
	w, _ := authJSON(r, http.MethodPut, "/api/users/me/password", "", `{"currentPassword":"correct-horse","newPassword":"new-password-1"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Wrong current password.
	w, resp = authJSON(r, http.MethodPut, "/api/users/me/password", token, `{"currentPassword":"nope","newPassword":"new-password-1"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// New password identical to current.
	_, resp = authJSON(r, http.MethodPut, "/api/users/me/password", token, `{"currentPassword":"correct-horse","newPassword":"correct-horse"}`)
	assert.EqualValues(t, 40001, resp["code"])

	// Success.
	w, resp = authJSON(r, http.MethodPut, "/api/users/me/password", token, `{"currentPassword":"correct-horse","newPassword":"new-password-1"}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, resp["data"])
}

func TestPasswordEndpointRateLimit(t *testing.T) {
	r := newAccountEngine(t)
	_, resp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	token := resp["data"].(map[string]any)["accessToken"].(string)

	for i := 0; i < 10; i++ {
		w, _ := authJSON(r, http.MethodPut, "/api/users/me/password", token, `{"currentPassword":"nope","newPassword":"new-password-1"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
	w, resp := authJSON(r, http.MethodPut, "/api/users/me/password", token, `{"currentPassword":"correct-horse","newPassword":"new-password-1"}`)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.EqualValues(t, 42904, resp["code"])
}

func TestSessionEndpoints(t *testing.T) {
	r := newAccountEngine(t)
	_, resp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	data := resp["data"].(map[string]any)
	tokenA := data["accessToken"].(string)
	sessionA := data["sessionId"].(string)
	refreshA := data["refreshToken"].(string)

	_, loginResp := postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)
	refreshB := loginResp["data"].(map[string]any)["refreshToken"].(string)

	// Unauthenticated.
	w, _ := authJSON(r, http.MethodGet, "/api/users/me/sessions", "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// List returns two sessions and never leaks token/hash/userID.
	w, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", tokenA, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	sessions := resp["data"].([]any)
	require.Len(t, sessions, 2)
	for _, sv := range sessions {
		m := sv.(map[string]any)
		assert.NotEmpty(t, m["id"])
		assert.NotContains(t, m, "refreshToken")
		assert.NotContains(t, m, "accessToken")
		assert.NotContains(t, m, "tokenHash")
		assert.NotContains(t, m, "userId")
	}
	// The raw response must not carry either refresh token or its hash.
	for _, secret := range []string{refreshA, refreshB, hashToken(refreshA), hashToken(refreshB)} {
		assert.NotContains(t, w.Body.String(), secret)
	}

	// Deleting another user's session is a 404.
	_, bresp := postJSON(r, "/api/auth/register", `{"username":"bob","email":"bob@example.com","password":"correct-horse"}`)
	bobSession := bresp["data"].(map[string]any)["sessionId"].(string)
	w, resp = authJSON(r, http.MethodDelete, "/api/users/me/sessions/"+bobSession, tokenA, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	// logout-others keeps the current session and revokes the rest.
	w, _ = authJSON(r, http.MethodPost, "/api/users/me/sessions/logout-others", tokenA, `{"sessionId":"`+sessionA+`"}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", tokenA, "")
	sessions = resp["data"].([]any)
	require.Len(t, sessions, 1)
	assert.Equal(t, sessionA, sessions[0].(map[string]any)["id"])

	// Deleting an own session returns 200.
	_, resp = postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)
	sessionC := resp["data"].(map[string]any)["sessionId"].(string)
	w, _ = authJSON(r, http.MethodDelete, "/api/users/me/sessions/"+sessionC, tokenA, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// logout-all clears everything.
	w, _ = authJSON(r, http.MethodPost, "/api/users/me/sessions/logout-all", tokenA, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", tokenA, "")
	assert.Empty(t, resp["data"])
}

func TestLogoutOthersRequiresSessionID(t *testing.T) {
	r := newAccountEngine(t)
	_, resp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	data := resp["data"].(map[string]any)
	token := data["accessToken"].(string)

	postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)

	for _, body := range []string{`{}`, `{"sessionId":""}`, `{"sessionId":"   "}`} {
		w, resp := authJSON(r, http.MethodPost, "/api/users/me/sessions/logout-others", token, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.EqualValues(t, 40001, resp["code"], body)
	}

	// A missing sessionId must not fall back to revoking every session.
	_, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", token, "")
	assert.Len(t, resp["data"], 2)
}

func TestSessionMetaTruncatesUserAgentOnRuneBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// 511 ASCII bytes + a 3-byte rune crosses the 512-byte limit mid-rune.
	c.Request.Header.Set("User-Agent", strings.Repeat("a", 511)+"字")

	meta := sessionMeta(c)
	assert.LessOrEqual(t, len(meta.UserAgent), 512)
	assert.True(t, utf8.ValidString(meta.UserAgent), "truncated UA must be valid UTF-8")
	assert.NotContains(t, meta.UserAgent, "\uFFFD")
	assert.Equal(t, strings.Repeat("a", 511), meta.UserAgent)
}

func TestLogoutOthersRejectsUnknownSession(t *testing.T) {
	r := newAccountEngine(t)
	_, resp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	data := resp["data"].(map[string]any)
	token := data["accessToken"].(string)
	postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)

	w, resp := authJSON(r, http.MethodPost, "/api/users/me/sessions/logout-others", token,
		`{"sessionId":"unknown-session-id"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	_, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", token, "")
	assert.Len(t, resp["data"], 2, "unknown keep session must not revoke all sessions")
}

func TestLogoutOthersRejectsOtherUsersSessionHTTP(t *testing.T) {
	r := newAccountEngine(t)
	_, aresp := postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
	adata := aresp["data"].(map[string]any)
	atoken := adata["accessToken"].(string)
	_, bresp := postJSON(r, "/api/auth/register", `{"username":"bob","email":"bob@example.com","password":"correct-horse"}`)
	bsession := bresp["data"].(map[string]any)["sessionId"].(string)
	postJSON(r, "/api/auth/login", `{"account":"alice","password":"correct-horse"}`)

	w, resp := authJSON(r, http.MethodPost, "/api/users/me/sessions/logout-others", atoken,
		`{"sessionId":"`+bsession+`"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])

	_, resp = authJSON(r, http.MethodGet, "/api/users/me/sessions", atoken, "")
	assert.Len(t, resp["data"], 2)
}
