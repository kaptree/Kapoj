package certificate

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// retryRepo 是 Review 冲突重试测试用的最小仓储替身：仅实现 Review 与
// FindCertificate，其余接口方法不会被调用。
type retryRepo struct {
	Repository
	cert       *Certificate
	reviewErrs []error
	codes      []string
}

func (r *retryRepo) FindCertificate(context.Context, int64) (*Certificate, error) {
	return r.cert, nil
}

func (r *retryRepo) Review(_ context.Context, _ int64, _ Status, _ string, code *string, _ int64, _ time.Time) (bool, error) {
	if code != nil {
		r.codes = append(r.codes, *code)
	}
	if len(r.codes) <= len(r.reviewErrs) {
		return false, r.reviewErrs[len(r.codes)-1]
	}
	return true, nil
}

func conflictError() error {
	return &pgconn.PgError{Code: "23505", ConstraintName: "uniq_certificates_verify_code"}
}

func pendingCert(id int64) *Certificate {
	return &Certificate{
		ID:                  id,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		Status:              StatusPending,
		SubmittedAt:         time.Now(),
	}
}

func TestReviewRetriesOnVerifyCodeConflict(t *testing.T) {
	repo := &retryRepo{
		cert:       pendingCert(7),
		reviewErrs: []error{conflictError(), conflictError()},
	}
	var generated []string
	svc := &Service{
		repo: repo,
		genCode: func() (string, error) {
			generated = append(generated, fmt.Sprintf("KAPOJ-C%d-C%d", len(generated), len(generated)))
			return generated[len(generated)-1], nil
		},
	}

	v, err := svc.Review(context.Background(), 1, 7, ReviewInput{Action: "approve"})
	require.NoError(t, err)
	assert.Equal(t, StatusApproved, v.Status)
	require.Len(t, generated, 3, "冲突两次后第三次应成功")
	assert.Equal(t, generated[2], v.VerifyCode, "返回的视图应携带最终成功生成的验证码")
	assert.Equal(t, []string{generated[0], generated[1], generated[2]}, repo.codes)
}

func TestReviewVerifyCodeExhaustedReturnsInternal(t *testing.T) {
	repo := &retryRepo{
		cert: pendingCert(7),
		reviewErrs: []error{
			conflictError(), conflictError(), conflictError(), conflictError(), conflictError(),
		},
	}
	var n int
	svc := &Service{
		repo: repo,
		genCode: func() (string, error) {
			n++
			return fmt.Sprintf("KAPOJ-X%d-X%d", n, n), nil
		},
	}

	_, err := svc.Review(context.Background(), 1, 7, ReviewInput{Action: "approve"})
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际 %T", err)
	assert.Equal(t, 50000, appErr.Code)
	assert.Equal(t, 5, n, "最多重试 5 次后放弃")
}

func TestGenerateVerifyCodeFormatAndUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		code, err := generateVerifyCode()
		require.NoError(t, err)
		require.Regexp(t, `^KAPOJ-[A-HJKMNP-TV-Z2-9]{4}-[A-HJKMNP-TV-Z2-9]{4}$`, code)
		assert.False(t, seen[code], "验证码不应重复: %s", code)
		seen[code] = true
	}
}
