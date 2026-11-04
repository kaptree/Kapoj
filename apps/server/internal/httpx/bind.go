package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BindJSON decodes the request body into T. On failure it writes the error
// response (400 for malformed JSON, 413 when the body exceeds the limit set
// by http.MaxBytesReader) and returns false.
func BindJSON[T any](c *gin.Context) (T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			Fail(c, ErrPayloadTooLarge)
		} else {
			Fail(c, ErrBadRequest)
		}
		return req, false
	}
	return req, true
}
