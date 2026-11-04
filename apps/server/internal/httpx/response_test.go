package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func run(h gin.HandlerFunc) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h(c)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestOKWritesEnvelope(t *testing.T) {
	w, body := run(func(c *gin.Context) { OK(c, gin.H{"a": 1}) })
	assert.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 0, body["code"])
	assert.Equal(t, "ok", body["message"])
	assert.EqualValues(t, 1, body["data"].(map[string]any)["a"])
}

func TestCreatedWrites201(t *testing.T) {
	w, _ := run(func(c *gin.Context) { Created(c, nil) })
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestFailWritesAppError(t *testing.T) {
	w, body := run(func(c *gin.Context) { Fail(c, ErrForbidden) })
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.EqualValues(t, 40300, body["code"])
}

func TestFailUnwrapsWrappedAppError(t *testing.T) {
	err := errors.Join(errors.New("context"), ErrValidation("username is invalid"))
	w, body := run(func(c *gin.Context) { Fail(c, err) })
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, body["code"])
	assert.Equal(t, "username is invalid", body["message"])
}

func TestFailHidesInternalError(t *testing.T) {
	w, body := run(func(c *gin.Context) { Fail(c, errors.New("pq: secret detail")) })
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.EqualValues(t, 50000, body["code"])
	assert.NotContains(t, w.Body.String(), "secret detail")
}
