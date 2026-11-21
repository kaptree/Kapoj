package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

func TestOutputLimitForCases(t *testing.T) {
	t.Run("small outputs keep the 1 MiB floor", func(t *testing.T) {
		got := outputLimitForCases([]model.Testcase{{OutputSize: 12}, {OutputSize: 0}})
		assert.Equal(t, outputLimitBytes, got)
	})

	t.Run("large expected output raises the limit", func(t *testing.T) {
		got := outputLimitForCases([]model.Testcase{
			{OutputSize: 1 << 20},
			{OutputSize: 2 << 20},
		})
		assert.Equal(t, 2<<20+outputLimitSlack, got)
	})

	t.Run("limit is capped", func(t *testing.T) {
		got := outputLimitForCases([]model.Testcase{{OutputSize: 64 << 20}})
		assert.Equal(t, maxOutputLimitBytes, got)
	})
}
