package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

func TestOutputLimitUsesSettings(t *testing.T) {
	t.Run("cases without expected output keep the configured floor", func(t *testing.T) {
		got := outputLimitForCases(
			[]model.Testcase{{OutputSize: 0}, {OutputSize: -1}},
			model.DefaultJudgeSettings(),
		)
		assert.Equal(t, int(model.DefaultJudgeSettings().OutputFloorBytes), got)
	})

	t.Run("small expected outputs keep the configured floor", func(t *testing.T) {
		settings := model.DefaultJudgeSettings()
		settings.OutputSlackBytes = 0
		got := outputLimitForCases([]model.Testcase{{OutputSize: 12}}, settings)
		assert.Equal(t, int(settings.OutputFloorBytes), got)
	})

	t.Run("large expected output raises the limit", func(t *testing.T) {
		settings := model.DefaultJudgeSettings()
		got := outputLimitForCases([]model.Testcase{
			{OutputSize: 1 << 20},
			{OutputSize: 2 << 20},
		}, settings)
		assert.Equal(t, int(2<<20+settings.OutputSlackBytes), got)
	})

	t.Run("limit is capped at max output", func(t *testing.T) {
		got := outputLimitForCases(
			[]model.Testcase{{OutputSize: 64 << 20}},
			model.DefaultJudgeSettings(),
		)
		assert.Equal(t, int(model.DefaultJudgeSettings().MaxOutputBytes), got)
	})

	t.Run("custom floor slack and max", func(t *testing.T) {
		settings := model.JudgeSettings{
			OutputFloorBytes: 2 << 20,
			OutputSlackBytes: 1 << 20,
			MaxOutputBytes:   8 << 20,
		}

		got := outputLimitForCases([]model.Testcase{{OutputSize: 3 << 20}}, settings)
		assert.Equal(t, 4<<20, got, "expected answer plus slack")

		got = outputLimitForCases([]model.Testcase{{OutputSize: 9 << 20}}, settings)
		assert.Equal(t, 8<<20, got, "capped at max output")
	})

	t.Run("max below floor is clamped to floor", func(t *testing.T) {
		settings := model.JudgeSettings{
			OutputFloorBytes: 4 << 20,
			OutputSlackBytes: 1 << 20,
			MaxOutputBytes:   2 << 20,
		}
		got := outputLimitForCases([]model.Testcase{{OutputSize: 64}}, settings)
		assert.Equal(t, 4<<20, got)
	})
}
