package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBody limits the request body to n bytes; reading past it fails with
// *http.MaxBytesError.
func MaxBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
