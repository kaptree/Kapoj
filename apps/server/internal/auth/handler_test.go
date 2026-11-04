package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRegisterEndpoint(t *testing.T) {
	r := newAuthEngine(t)
	w, resp := postJSON(r, "/api/auth/register",
		`{"username":"Alice","email":"alice@example.com","password":"correct-horse"}`)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.NotEmpty(t, data["accessToken"])
	assert.NotEmpty(t, data["refreshToken"])
	u := data["user"].(map[string]any)
	assert.Equal(t, "alice", u["username"])
	assert.Equal(t, "alice@example.com", u["email"])
	assert.NotContains(t, w.Body.String(), "passwordHash")
	assert.NotContains(t, w.Body.String(), "PasswordHash")
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
	refresh := resp["data"].(map[string]any)["refreshToken"].(string)

	w, resp = postJSON(r, "/api/auth/refresh", `{"refreshToken":"`+refresh+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rotated := resp["data"].(map[string]any)["refreshToken"].(string)
	assert.NotEqual(t, refresh, rotated)

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
