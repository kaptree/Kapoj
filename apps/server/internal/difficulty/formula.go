package difficulty

import "math"

// FormulaInput is the input of the difficulty formula.
//
// Prior is the Bayesian prior score (1..100). SampleUsers and AcceptedUsers are
// deduplicated user counts within the statistics window. PriorStrength is the
// equivalent number of virtual users. Gamma shapes the pass-rate curve.
type FormulaInput struct {
	Prior         int
	SampleUsers   int
	AcceptedUsers int
	PriorStrength int
	Gamma         float64
}

// FormulaResult is the output of the difficulty formula.
//
// PriorPassRate is p0, the pass rate implied by the prior. SmoothedPassRate is
// p_hat after mixing in the observed sample. Target is the rounded and clamped
// target score (1..100).
type FormulaResult struct {
	PriorPassRate    float64
	SmoothedPassRate float64
	Target           int
}

// Compute evaluates the difficulty formula from the design document:
//
//	p0    = 1 - (prior - 1) / 99
//	p_hat = (a + m*p0) / (n + m)
//	target = 1 + 99 * (1 - p_hat)^gamma
//
// Inputs are clamped defensively: prior to 1..100, sample users to >= 0,
// accepted users to 0..sample users, prior strength to >= 1, and gamma
// defaults to 1.5 when it is not positive. The target is rounded with
// math.Round and clamped to 1..100.
func Compute(in FormulaInput) FormulaResult {
	prior := clamp(in.Prior, 1, 100)
	n := in.SampleUsers
	if n < 0 {
		n = 0
	}
	a := clamp(in.AcceptedUsers, 0, n)
	m := in.PriorStrength
	if m < 1 {
		m = 1
	}
	gamma := in.Gamma
	if gamma <= 0 {
		gamma = 1.5
	}

	p0 := 1 - float64(prior-1)/99
	pHat := (float64(a) + float64(m)*p0) / float64(n+m)
	target := int(math.Round(1 + 99*math.Pow(1-pHat, gamma)))

	return FormulaResult{
		PriorPassRate:    p0,
		SmoothedPassRate: pHat,
		Target:           clamp(target, 1, 100),
	}
}

// Step moves current toward target by at most maxStep and clamps the result to
// 1..100. A non-positive maxStep keeps the current score, as does an already
// reached target.
func Step(current, target, maxStep int) int {
	current = clamp(current, 1, 100)
	target = clamp(target, 1, 100)

	if maxStep <= 0 || current == target {
		return current
	}

	if current < target {
		delta := target - current
		if maxStep < delta {
			delta = maxStep
		}
		current += delta
	} else {
		delta := current - target
		if maxStep < delta {
			delta = maxStep
		}
		current -= delta
	}

	return clamp(current, 1, 100)
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
