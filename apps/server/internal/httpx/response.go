package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Page is the standard paginated list payload.
type Page[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 0, Message: "ok", Data: data})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, envelope{Code: 0, Message: "ok", Data: data})
}

// FailWithData writes an AppError response that also carries a data payload.
func FailWithData(c *gin.Context, e *AppError, data any) {
	c.AbortWithStatusJSON(e.Status, envelope{Code: e.Code, Message: e.Message, Data: data})
}

// Fail writes err as an API error. Errors that are not AppErrors are logged
// and reported as a generic internal error so details never reach clients.
func Fail(c *gin.Context, err error) {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		slog.Error("unhandled error", "method", c.Request.Method, "path", c.FullPath(), "err", err)
		appErr = ErrInternal
	}
	FailWithData(c, appErr, nil)
}
