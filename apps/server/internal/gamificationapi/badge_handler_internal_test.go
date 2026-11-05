package gamificationapi

import (
	"errors"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func TestUploadFormErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"缺少文件", http.ErrMissingFile, "缺少文件字段 file"},
		{"请求体过大", &http.MaxBytesError{Limit: 40 << 20}, "上传文件过大"},
		{"multipart 过大", multipart.ErrMessageTooLarge, "上传文件过大"},
		{"缺少 boundary", http.ErrMissingBoundary, "上传格式错误"},
		{"其他解析错误", errors.New("multipart: malformed"), "上传格式错误"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := uploadFormError(tc.err)
			var appErr *httpx.AppError
			assert.ErrorAs(t, err, &appErr)
			assert.Equal(t, 40001, appErr.Code)
			assert.Equal(t, tc.want, appErr.Message)
		})
	}
}
