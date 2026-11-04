package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func readAll(limit int64, body string) error {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var readErr error
	r.POST("/", MaxBody(limit), func(c *gin.Context) {
		_, readErr = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body)))
	return readErr
}

func TestMaxBodyRejectsOversize(t *testing.T) {
	err := readAll(5, "0123456789")
	var tooLarge *http.MaxBytesError
	assert.True(t, errors.As(err, &tooLarge), "want MaxBytesError, got %v", err)
}

func TestMaxBodyAllowsWithinLimit(t *testing.T) {
	assert.NoError(t, readAll(10, "0123456789"))
}
