package httpx

import "net/http"

// AppError is an error that maps to a specific HTTP status and API error code.
type AppError struct {
	Status  int
	Code    int
	Message string
}

func (e *AppError) Error() string { return e.Message }

func newErr(status, code int, msg string) *AppError {
	return &AppError{Status: status, Code: code, Message: msg}
}

var (
	ErrBadRequest         = newErr(http.StatusBadRequest, 40000, "请求体格式错误")
	ErrUnauthorized       = newErr(http.StatusUnauthorized, 40100, "未登录或登录已过期")
	ErrInvalidCredentials = newErr(http.StatusUnauthorized, 40101, "账号或密码错误")
	ErrForbidden          = newErr(http.StatusForbidden, 40300, "没有权限执行此操作")
	ErrUserBanned         = newErr(http.StatusForbidden, 40301, "账号已被封禁")
	ErrNotFound           = newErr(http.StatusNotFound, 40400, "资源不存在")
	ErrUsernameTaken      = newErr(http.StatusConflict, 40901, "用户名已被占用")
	ErrEmailTaken         = newErr(http.StatusConflict, 40902, "邮箱已被注册")
	ErrTooManyAttempts    = newErr(http.StatusTooManyRequests, 42901, "登录失败次数过多，请 5 分钟后再试")
	ErrInternal           = newErr(http.StatusInternalServerError, 50000, "服务器内部错误")
	ErrUnavailable        = newErr(http.StatusServiceUnavailable, 50300, "依赖服务不可用")
)

// ErrValidation reports which input failed validation.
func ErrValidation(msg string) *AppError {
	return newErr(http.StatusBadRequest, 40001, msg)
}
