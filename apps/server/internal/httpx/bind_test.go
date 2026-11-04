package httpx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type bindBody struct {
	Name string `json:"name"`
}

func bindRun(t *testing.T, body string, limit int64) (*httptest.ResponseRecorder, bindBody, bool) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	if limit > 0 {
		c.Request.Body = http.MaxBytesReader(w, c.Request.Body, limit)
	}
	got, ok := BindJSON[bindBody](c)
	return w, got, ok
}

func TestBindJSONOK(t *testing.T) {
	_, got, ok := bindRun(t, `{"name":"x"}`, 0)
	assert.True(t, ok)
	assert.Equal(t, "x", got.Name)
}

func TestBindJSONMalformed(t *testing.T) {
	w, _, ok := bindRun(t, `{"name":`, 0)
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `"code":40000`)
}

func TestBindJSONOversize(t *testing.T) {
	w, _, ok := bindRun(t, `{"name":"`+"aaaaaaaaaaaaaaaaaaaaaaaa"+`"}`, 10)
	assert.False(t, ok)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), `"code":41300`)
}
