package rating

import "math"

// Elo v1 constants. The formula and limits are part of the rating design:
// delta = round(32 * (actual - expected)), clamped to [-100, 100].
const (
	eloK        = 32.0
	eloMaxDelta = 100
)

// Participant is one rated player before settlement. Rank is 1-based and may
// repeat for ties; the caller must also provide a valid rank.
type Participant struct {
	UserID int64
	Rank   int
	Rating int
}

// Change is one player's computed rating update.
type Change struct {
	UserID int64
	Rank   int
	Before int
	After  int
	Delta  int
}

// Calculate applies Elo v1 to a settlement snapshot. Expected score is
// E_i = sum_{j != i} 1 / (1 + 10^((R_j - R_i)/400)); actual score is n-rank.
// Results keep the input order. With fewer than two players every delta is 0.
func Calculate(participants []Participant) []Change {
	changes := make([]Change, len(participants))
	n := len(participants)
	if n <= 1 {
		for i, p := range participants {
			changes[i] = Change{UserID: p.UserID, Rank: p.Rank, Before: p.Rating, After: p.Rating}
		}
		return changes
	}

	for i, p := range participants {
		expected := 0.0
		for j, other := range participants {
			if i == j {
				continue
			}
			expected += 1 / (1 + math.Pow(10, float64(other.Rating-p.Rating)/400))
		}
		actual := float64(n - p.Rank)
		delta := int(math.Round(eloK * (actual - expected)))
		if delta > eloMaxDelta {
			delta = eloMaxDelta
		}
		if delta < -eloMaxDelta {
			delta = -eloMaxDelta
		}
		after := p.Rating + delta
		if after < 0 {
			after = 0
		}
		changes[i] = Change{
			UserID: p.UserID, Rank: p.Rank,
			Before: p.Rating, After: after, Delta: delta,
		}
	}
	return changes
}
