package judgesettings_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/judgesettings"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func requireJudgeSettingsValidation(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "expected AppError, got %T: %v", err, err)
	assert.Equal(t, 40001, appErr.Code)
}

func validJudgeSettingsInput() judgesettings.SettingsInput {
	return judgesettings.SettingsInput{
		OutputFloorBytes:     2 * judgesettings.MiB,
		OutputSlackBytes:     judgesettings.MiB / 2,
		MaxOutputBytes:       8 * judgesettings.MiB,
		MaxInputFileBytes:    32 * judgesettings.MiB,
		MaxOutputFileBytes:   8 * judgesettings.MiB,
		MaxBatchTotalBytes:   300 * judgesettings.MiB,
		MaxProblemTotalBytes: 400 * judgesettings.MiB,
	}
}

func TestJudgeSettingsDefaultsAndUpdate(t *testing.T) {
	g := testutil.PG(t)
	svc := judgesettings.NewSettingsService(g)
	ctx := context.Background()

	view, err := svc.Get(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1<<20, view.OutputFloorBytes)
	assert.EqualValues(t, 1<<20, view.OutputSlackBytes)
	assert.EqualValues(t, 16<<20, view.MaxOutputBytes)
	assert.EqualValues(t, 16<<20, view.MaxInputFileBytes)
	assert.EqualValues(t, 16<<20, view.MaxOutputFileBytes)
	assert.EqualValues(t, 200<<20, view.MaxBatchTotalBytes)
	assert.EqualValues(t, 200<<20, view.MaxProblemTotalBytes)
	assert.False(t, view.UpdatedAt.IsZero())

	limits, err := svc.TestcaseLimits(ctx)
	require.NoError(t, err)
	assert.Equal(t, judgesettings.TestcaseLimits{
		MaxInputBytes:        16 << 20,
		MaxOutputBytes:       16 << 20,
		MaxBatchTotalBytes:   200 << 20,
		MaxProblemTotalBytes: 200 << 20,
	}, limits)

	in := validJudgeSettingsInput()
	updated, err := svc.Update(ctx, 42, in, nil)
	require.NoError(t, err)
	assert.Equal(t, in.OutputFloorBytes, updated.OutputFloorBytes)
	assert.Equal(t, in.OutputSlackBytes, updated.OutputSlackBytes)
	assert.Equal(t, in.MaxOutputBytes, updated.MaxOutputBytes)
	assert.Equal(t, in.MaxInputFileBytes, updated.MaxInputFileBytes)
	assert.Equal(t, in.MaxOutputFileBytes, updated.MaxOutputFileBytes)
	assert.Equal(t, in.MaxBatchTotalBytes, updated.MaxBatchTotalBytes)
	assert.Equal(t, in.MaxProblemTotalBytes, updated.MaxProblemTotalBytes)
	assert.False(t, updated.UpdatedAt.IsZero())

	var row judgesettings.Settings
	require.NoError(t, g.First(&row, 1).Error)
	assert.EqualValues(t, 42, row.UpdatedBy)
	assert.Equal(t, in.MaxOutputBytes, row.MaxOutputBytes)

	read, err := svc.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, in.OutputFloorBytes, read.OutputFloorBytes)
	assert.Equal(t, in.OutputSlackBytes, read.OutputSlackBytes)
	assert.Equal(t, in.MaxOutputBytes, read.MaxOutputBytes)
	assert.Equal(t, in.MaxInputFileBytes, read.MaxInputFileBytes)
	assert.Equal(t, in.MaxOutputFileBytes, read.MaxOutputFileBytes)
	assert.Equal(t, in.MaxBatchTotalBytes, read.MaxBatchTotalBytes)
	assert.Equal(t, in.MaxProblemTotalBytes, read.MaxProblemTotalBytes)

	limits, err = svc.TestcaseLimits(ctx)
	require.NoError(t, err)
	assert.Equal(t, judgesettings.TestcaseLimits{
		MaxInputBytes:        in.MaxInputFileBytes,
		MaxOutputBytes:       in.MaxOutputFileBytes,
		MaxBatchTotalBytes:   in.MaxBatchTotalBytes,
		MaxProblemTotalBytes: in.MaxProblemTotalBytes,
	}, limits)
}

func TestJudgeSettingsValidation(t *testing.T) {
	svc := judgesettings.NewSettingsService(testutil.PG(t))
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*judgesettings.SettingsInput)
	}{
		{"floor below minimum", func(in *judgesettings.SettingsInput) { in.OutputFloorBytes = 0 }},
		{"floor above maximum", func(in *judgesettings.SettingsInput) {
			in.OutputFloorBytes = 64*judgesettings.MiB + 1
		}},
		{"slack negative", func(in *judgesettings.SettingsInput) { in.OutputSlackBytes = -1 }},
		{"slack above maximum", func(in *judgesettings.SettingsInput) {
			in.OutputSlackBytes = 64*judgesettings.MiB + 1
		}},
		{"max output below minimum", func(in *judgesettings.SettingsInput) { in.MaxOutputBytes = 0 }},
		{"max output above maximum", func(in *judgesettings.SettingsInput) {
			in.MaxOutputBytes = 64*judgesettings.MiB + 1
		}},
		{"max output below floor", func(in *judgesettings.SettingsInput) {
			in.OutputFloorBytes = 8 * judgesettings.MiB
			in.MaxOutputBytes = 4 * judgesettings.MiB
			in.MaxOutputFileBytes = 4 * judgesettings.MiB
		}},
		{"max output below slack", func(in *judgesettings.SettingsInput) {
			in.OutputSlackBytes = 8 * judgesettings.MiB
			in.MaxOutputBytes = 4 * judgesettings.MiB
			in.MaxOutputFileBytes = 4 * judgesettings.MiB
		}},
		{"input file below minimum", func(in *judgesettings.SettingsInput) { in.MaxInputFileBytes = 0 }},
		{"input file above maximum", func(in *judgesettings.SettingsInput) {
			in.MaxInputFileBytes = 256*judgesettings.MiB + 1
		}},
		{"output file below minimum", func(in *judgesettings.SettingsInput) { in.MaxOutputFileBytes = 0 }},
		{"output file above maximum", func(in *judgesettings.SettingsInput) {
			in.MaxOutputFileBytes = 64*judgesettings.MiB + 1
		}},
		{"output file above max output", func(in *judgesettings.SettingsInput) {
			in.MaxOutputFileBytes = 9 * judgesettings.MiB
		}},
		{"batch total below minimum", func(in *judgesettings.SettingsInput) { in.MaxBatchTotalBytes = 0 }},
		{"batch total above maximum", func(in *judgesettings.SettingsInput) {
			in.MaxBatchTotalBytes = 2048*judgesettings.MiB + 1
		}},
		{"problem total below minimum", func(in *judgesettings.SettingsInput) {
			in.MaxProblemTotalBytes = 0
		}},
		{"problem total above maximum", func(in *judgesettings.SettingsInput) {
			in.MaxProblemTotalBytes = 2048*judgesettings.MiB + 1
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validJudgeSettingsInput()
			tc.mutate(&in)
			_, err := svc.Update(ctx, 1, in, nil)
			requireJudgeSettingsValidation(t, err)
		})
	}

	// 边界值可以通过校验。
	_, err := svc.Update(ctx, 1, judgesettings.SettingsInput{
		OutputFloorBytes:     1 << 10,
		OutputSlackBytes:     0,
		MaxOutputBytes:       64 * judgesettings.MiB,
		MaxInputFileBytes:    256 * judgesettings.MiB,
		MaxOutputFileBytes:   64 * judgesettings.MiB,
		MaxBatchTotalBytes:   2048 * judgesettings.MiB,
		MaxProblemTotalBytes: 2048 * judgesettings.MiB,
	}, nil)
	require.NoError(t, err)
}

func TestJudgeSettingsAuditRollback(t *testing.T) {
	g := testutil.PG(t)
	svc := judgesettings.NewSettingsService(g)
	ctx := context.Background()

	_, err := svc.Update(ctx, 1, validJudgeSettingsInput(),
		func(*gorm.DB) error { return errors.New("audit failed") })
	require.Error(t, err)

	view, err := svc.Get(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, judgesettings.DefaultOutputFloorBytes, view.OutputFloorBytes)
	assert.EqualValues(t, judgesettings.DefaultOutputSlackBytes, view.OutputSlackBytes)
	assert.EqualValues(t, judgesettings.DefaultMaxOutputBytes, view.MaxOutputBytes)
	assert.EqualValues(t, judgesettings.DefaultMaxInputFileBytes, view.MaxInputFileBytes)
	assert.EqualValues(t, judgesettings.DefaultMaxOutputFileBytes, view.MaxOutputFileBytes)
	assert.EqualValues(t, judgesettings.DefaultMaxBatchTotalBytes, view.MaxBatchTotalBytes)
	assert.EqualValues(t, judgesettings.DefaultMaxProblemTotalBytes, view.MaxProblemTotalBytes)
}

func TestJudgeSettingsGetLazilyRecreatesMissingRow(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, g.Where("id = ?", 1).Delete(&judgesettings.Settings{}).Error)

	view, err := judgesettings.NewSettingsService(g).Get(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, judgesettings.DefaultMaxOutputBytes, view.MaxOutputBytes)
}
