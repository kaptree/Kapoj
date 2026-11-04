package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func serve(db, redis Pinger) (*httptest.ResponseRecorder, map[string]any) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", Handler(db, redis))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestHealthOK(t *testing.T) {
	w, body := serve(pinger{}, pinger{})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string]any{"status": "ok", "db": "ok", "redis": "ok"}, body["data"])
}

func TestHealthDegraded(t *testing.T) {
	w, body := serve(pinger{err: errors.New("down")}, pinger{})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.EqualValues(t, 50300, body["code"])
	data := body["data"].(map[string]any)
	assert.Equal(t, "down", data["db"])
	assert.Equal(t, "ok", data["redis"])
	assert.Equal(t, "degraded", data["status"])
}
