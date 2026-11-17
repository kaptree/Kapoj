package difficulty_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}

func defaultSettingsInput() difficulty.SettingsInput {
	return difficulty.SettingsInput{
		Enabled: true, DefaultScore: 50, MinSample: 10, PriorStrength: 10,
		MaxStep: 5, WindowDays: 365, IncludeContest: false,
		CurveGamma: 1.5, AIAssessEnabled: true, AITimeoutSeconds: 15,
	}
}

func TestSettingsDefaultsAndLazyCreate(t *testing.T) {
	g := testutil.PG(t)
	svc := difficulty.NewSettingsService(g)
	view, err := svc.Get(context.Background())
	require.NoError(t, err)
	assert.True(t, view.Enabled)
	assert.Equal(t, 50, view.DefaultScore)
	assert.Equal(t, 10, view.MinSample)
	assert.Equal(t, 10, view.PriorStrength)
	assert.Equal(t, 5, view.MaxStep)
	assert.Equal(t, 365, view.WindowDays)
	assert.Equal(t, 1.5, view.CurveGamma)
	assert.False(t, view.IncludeContest)
	assert.True(t, view.AIAssessEnabled)
	assert.Equal(t, 15, view.AITimeoutSeconds)
	assert.False(t, view.UpdatedAt.IsZero())

	// 重复 Get 不产生重复行。
	_, err = svc.Get(context.Background())
	require.NoError(t, err)
	var count int64
	require.NoError(t, g.Model(&difficulty.Settings{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestSettingsUpdateValidatesAndAudits(t *testing.T) {
	g := testutil.PG(t)
	svc := difficulty.NewSettingsService(g)
	audited := false
	in := difficulty.SettingsInput{
		Enabled: true, DefaultScore: 50, MinSample: 10, PriorStrength: 10,
		MaxStep: 5, WindowDays: 365, IncludeContest: false,
		CurveGamma: 1.5, AIAssessEnabled: true, AITimeoutSeconds: 15,
	}
	view, err := svc.Update(context.Background(), 7, in, func(tx *gorm.DB) error {
		audited = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, audited)
	require.NotNil(t, view)
	assert.Equal(t, 50, view.DefaultScore)
	assert.Equal(t, 1.5, view.CurveGamma)
	assert.False(t, view.UpdatedAt.IsZero())

	var row difficulty.Settings
	require.NoError(t, g.First(&row, 1).Error)
	assert.True(t, row.Enabled)
	assert.Equal(t, int64(7), row.UpdatedBy)
	assert.False(t, row.UpdatedAt.IsZero())

	in.DefaultScore = 101
	_, err = svc.Update(context.Background(), 7, in, nil)
	assertCode(t, err, 40001)
}

func TestSettingsUpdateValidatesRanges(t *testing.T) {
	g := testutil.PG(t)
	svc := difficulty.NewSettingsService(g)

	cases := []struct {
		name   string
		mutate func(*difficulty.SettingsInput)
	}{
		{"default_score below", func(in *difficulty.SettingsInput) { in.DefaultScore = 0 }},
		{"default_score above", func(in *difficulty.SettingsInput) { in.DefaultScore = 101 }},
		{"min_sample below", func(in *difficulty.SettingsInput) { in.MinSample = 0 }},
		{"min_sample above", func(in *difficulty.SettingsInput) { in.MinSample = 1001 }},
		{"prior_strength below", func(in *difficulty.SettingsInput) { in.PriorStrength = 0 }},
		{"prior_strength above", func(in *difficulty.SettingsInput) { in.PriorStrength = 101 }},
		{"max_step below", func(in *difficulty.SettingsInput) { in.MaxStep = 0 }},
		{"max_step above", func(in *difficulty.SettingsInput) { in.MaxStep = 101 }},
		{"window_days below", func(in *difficulty.SettingsInput) { in.WindowDays = -1 }},
		{"window_days above", func(in *difficulty.SettingsInput) { in.WindowDays = 3651 }},
		{"curve_gamma below", func(in *difficulty.SettingsInput) { in.CurveGamma = 0.99 }},
		{"curve_gamma above", func(in *difficulty.SettingsInput) { in.CurveGamma = 3.01 }},
		{"ai_timeout_seconds below", func(in *difficulty.SettingsInput) { in.AITimeoutSeconds = 4 }},
		{"ai_timeout_seconds above", func(in *difficulty.SettingsInput) { in.AITimeoutSeconds = 61 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := defaultSettingsInput()
			tc.mutate(&in)
			_, err := svc.Update(context.Background(), 7, in, nil)
			assertCode(t, err, 40001)
		})
	}

	var count int64
	require.NoError(t, g.Model(&difficulty.Settings{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "校验失败不应创建或更新设置行")
}

func TestSettingsUpdateAcceptsBoundaries(t *testing.T) {
	g := testutil.PG(t)
	svc := difficulty.NewSettingsService(g)
	in := difficulty.SettingsInput{
		Enabled: false, DefaultScore: 1, MinSample: 1, PriorStrength: 1,
		MaxStep: 1, WindowDays: 0, IncludeContest: true,
		CurveGamma: 1.0, AIAssessEnabled: false, AITimeoutSeconds: 5,
	}
	view, err := svc.Update(context.Background(), 3, in, nil)
	require.NoError(t, err)
	assert.False(t, view.Enabled)
	assert.Equal(t, 1, view.DefaultScore)
	assert.Equal(t, 1, view.MinSample)
	assert.Equal(t, 1, view.PriorStrength)
	assert.Equal(t, 1, view.MaxStep)
	assert.Equal(t, 0, view.WindowDays)
	assert.True(t, view.IncludeContest)
	assert.Equal(t, 1.0, view.CurveGamma)
	assert.False(t, view.AIAssessEnabled)
	assert.Equal(t, 5, view.AITimeoutSeconds)

	in = difficulty.SettingsInput{
		Enabled: true, DefaultScore: 100, MinSample: 1000, PriorStrength: 100,
		MaxStep: 100, WindowDays: 3650, IncludeContest: false,
		CurveGamma: 3.0, AIAssessEnabled: true, AITimeoutSeconds: 60,
	}
	view, err = svc.Update(context.Background(), 4, in, nil)
	require.NoError(t, err)
	assert.Equal(t, 100, view.DefaultScore)
	assert.Equal(t, 1000, view.MinSample)
	assert.Equal(t, 100, view.PriorStrength)
	assert.Equal(t, 100, view.MaxStep)
	assert.Equal(t, 3650, view.WindowDays)
	assert.Equal(t, 3.0, view.CurveGamma)
	assert.Equal(t, 60, view.AITimeoutSeconds)
}

func TestSettingsUpdateAuditErrorRollsBack(t *testing.T) {
	g := testutil.PG(t)
	svc := difficulty.NewSettingsService(g)

	_, err := svc.Update(context.Background(), 9, defaultSettingsInput(), func(tx *gorm.DB) error {
		return errors.New("audit boom")
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, g.Model(&difficulty.Settings{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "审计失败应回滚整个事务，不产生设置行")
}
