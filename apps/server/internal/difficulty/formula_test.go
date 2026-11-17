package difficulty_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
)

func TestComputeMatchesSpecExamples(t *testing.T) {
	cases := []struct {
		n, a int
		want int
	}{
		{10, 9, 17},
		{30, 12, 44},
		{100, 90, 6},
		{100, 5, 87},
		{10, 0, 65},
	}
	for _, tc := range cases {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: tc.n, AcceptedUsers: tc.a,
			PriorStrength: 10, Gamma: 1.5,
		})
		if got.Target != tc.want {
			t.Fatalf("n=%d a=%d: want %d, got %d", tc.n, tc.a, tc.want, got.Target)
		}
	}
}

func TestStepClampsAndMovesTowardTarget(t *testing.T) {
	assert.Equal(t, 45, difficulty.Step(50, 17, 5))
	assert.Equal(t, 55, difficulty.Step(50, 87, 5))
	assert.Equal(t, 17, difficulty.Step(50, 17, 100))
	assert.Equal(t, 1, difficulty.Step(3, 1, 100))
	assert.Equal(t, 100, difficulty.Step(97, 100, 100))
}

func TestComputeClampsInputs(t *testing.T) {
	t.Run("prior below range", func(t *testing.T) {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 0, SampleUsers: 10, AcceptedUsers: 0,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, 1.0, got.PriorPassRate)

		low := difficulty.Compute(difficulty.FormulaInput{
			Prior: 1, SampleUsers: 10, AcceptedUsers: 0,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, low, got)
	})

	t.Run("prior above range", func(t *testing.T) {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 500, SampleUsers: 10, AcceptedUsers: 5,
			PriorStrength: 10, Gamma: 1.5,
		})
		high := difficulty.Compute(difficulty.FormulaInput{
			Prior: 100, SampleUsers: 10, AcceptedUsers: 5,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, high, got)
	})

	t.Run("negative sample users", func(t *testing.T) {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: -5, AcceptedUsers: -3,
			PriorStrength: 10, Gamma: 1.5,
		})
		zero := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: 0, AcceptedUsers: 0,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, zero, got)
	})

	t.Run("accepted users bounded by sample users", func(t *testing.T) {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: 10, AcceptedUsers: 100,
			PriorStrength: 10, Gamma: 1.5,
		})
		all := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: 10, AcceptedUsers: 10,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, all, got)
	})

	t.Run("prior strength at least one", func(t *testing.T) {
		got := difficulty.Compute(difficulty.FormulaInput{
			Prior: 1, SampleUsers: 10, AcceptedUsers: 0,
			PriorStrength: 0, Gamma: 1.5,
		})
		one := difficulty.Compute(difficulty.FormulaInput{
			Prior: 1, SampleUsers: 10, AcceptedUsers: 0,
			PriorStrength: 1, Gamma: 1.5,
		})
		assert.Equal(t, one, got)
		assert.Equal(t, 87, got.Target)
	})

	t.Run("gamma fallback", func(t *testing.T) {
		zero := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: 10, AcceptedUsers: 9,
			PriorStrength: 10, Gamma: 0,
		})
		fallback := difficulty.Compute(difficulty.FormulaInput{
			Prior: 50, SampleUsers: 10, AcceptedUsers: 9,
			PriorStrength: 10, Gamma: 1.5,
		})
		assert.Equal(t, fallback, zero)
		assert.Equal(t, 17, zero.Target)
	})
}

func TestComputeExposesPassRatesAndTargetBounds(t *testing.T) {
	got := difficulty.Compute(difficulty.FormulaInput{
		Prior: 50, SampleUsers: 10, AcceptedUsers: 9,
		PriorStrength: 10, Gamma: 1.5,
	})
	assert.InDelta(t, 50.0/99.0, got.PriorPassRate, 1e-12)
	assert.InDelta(t, (9+10*(50.0/99.0))/20, got.SmoothedPassRate, 1e-12)
	assert.Equal(t, 17, got.Target)

	for prior := 0; prior <= 101; prior++ {
		for _, n := range []int{-1, 0, 1, 10, 100, 1000} {
			for _, a := range []int{-1, 0, 1, n / 2, n, n + 5} {
				for _, gamma := range []float64{-1, 0, 1, 1.5, 3, 10} {
					got := difficulty.Compute(difficulty.FormulaInput{
						Prior: prior, SampleUsers: n, AcceptedUsers: a,
						PriorStrength: 0, Gamma: gamma,
					})
					require.GreaterOrEqual(t, got.Target, 1)
					require.LessOrEqual(t, got.Target, 100)
					require.GreaterOrEqual(t, got.PriorPassRate, 0.0)
					require.LessOrEqual(t, got.PriorPassRate, 1.0)
					require.GreaterOrEqual(t, got.SmoothedPassRate, 0.0)
					require.LessOrEqual(t, got.SmoothedPassRate, 1.0)
				}
			}
		}
	}
}

func TestStepEdgeCases(t *testing.T) {
	assert.Equal(t, 50, difficulty.Step(50, 17, 0))
	assert.Equal(t, 50, difficulty.Step(50, 17, -5))
	assert.Equal(t, 50, difficulty.Step(50, 50, 5))
	assert.Equal(t, 6, difficulty.Step(-3, 50, 5))
	assert.Equal(t, 95, difficulty.Step(150, 50, 5))
}
