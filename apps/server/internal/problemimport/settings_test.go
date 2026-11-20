package problemimport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func requireValidation(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "expected AppError, got %T: %v", err, err)
	assert.Equal(t, 40001, appErr.Code)
}

func TestGetCreatesDefaultAndComputesLimits(t *testing.T) {
	g := testutil.PG(t)
	view, err := NewSettingsService(g).Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxPackageMB, view.MaxPackageMB)
	assert.Equal(t, int64(200*MiB), view.MaxZipBytes)
	assert.Equal(t, MinUncompressedBytes, view.MaxUncompressedBytes)
	assert.False(t, view.UpdatedAt.IsZero())
}

func TestUpdateValidation(t *testing.T) {
	svc := NewSettingsService(testutil.PG(t))
	ctx := context.Background()
	for _, value := range []int{0, -1, MaxMaxPackageMB + 1} {
		_, err := svc.Update(ctx, 1, SettingsInput{MaxPackageMB: value}, nil)
		requireValidation(t, err)
	}
}

func TestUpdateComputesLargePackageLimits(t *testing.T) {
	svc := NewSettingsService(testutil.PG(t))
	view, err := svc.Update(context.Background(), 1, SettingsInput{MaxPackageMB: 2048}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2048, view.MaxPackageMB)
	assert.Equal(t, int64(2048*MiB), view.MaxZipBytes)
	assert.Equal(t, int64(4<<30), view.MaxUncompressedBytes)
}

func TestUpdateAuditFailureRollsBack(t *testing.T) {
	g := testutil.PG(t)
	svc := NewSettingsService(g)
	_, err := svc.Update(context.Background(), 1, SettingsInput{MaxPackageMB: 1024},
		func(*gorm.DB) error { return errors.New("audit failed") })
	require.Error(t, err)
	view, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxPackageMB, view.MaxPackageMB)
}

func TestGetLazilyRecreatesMissingRow(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, g.Where("id = ?", 1).Delete(&Settings{}).Error)
	view, err := NewSettingsService(g).Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxPackageMB, view.MaxPackageMB)
}

var _ = time.Now
