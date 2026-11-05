package rating_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rating"
)

func TestCalculateThreePlayers(t *testing.T) {
	got := rating.Calculate([]rating.Participant{
		{UserID: 1, Rank: 1, Rating: 1500},
		{UserID: 2, Rank: 2, Rating: 1500},
		{UserID: 3, Rank: 3, Rating: 1500},
	})

	require.Len(t, got, 3)
	assert.Equal(t, rating.Change{UserID: 1, Rank: 1, Before: 1500, After: 1532, Delta: 32}, got[0])
	assert.Equal(t, rating.Change{UserID: 2, Rank: 2, Before: 1500, After: 1500, Delta: 0}, got[1])
	assert.Equal(t, rating.Change{UserID: 3, Rank: 3, Before: 1500, After: 1468, Delta: -32}, got[2])
}

func TestCalculateSinglePlayerNoChange(t *testing.T) {
	got := rating.Calculate([]rating.Participant{{UserID: 9, Rank: 1, Rating: 1734}})

	require.Len(t, got, 1)
	assert.Equal(t, 0, got[0].Delta)
	assert.Equal(t, 1734, got[0].After)
}

func TestCalculateEmpty(t *testing.T) {
	assert.Empty(t, rating.Calculate(nil))
}

func TestCalculateTiesShareActualScore(t *testing.T) {
	got := rating.Calculate([]rating.Participant{
		{UserID: 1, Rank: 1, Rating: 1500},
		{UserID: 2, Rank: 1, Rating: 1500},
		{UserID: 3, Rank: 3, Rating: 1500},
	})

	require.Len(t, got, 3)
	assert.Equal(t, 32, got[0].Delta, "tied first place shares A=2")
	assert.Equal(t, 32, got[1].Delta)
	assert.Equal(t, -32, got[2].Delta)
	assert.Equal(t, 1500+32, got[1].After)
}

func TestCalculateKnownFivePlayerCase(t *testing.T) {
	got := rating.Calculate([]rating.Participant{
		{UserID: 1, Rank: 1, Rating: 1600},
		{UserID: 2, Rank: 2, Rating: 1550},
		{UserID: 3, Rank: 3, Rating: 1500},
		{UserID: 4, Rank: 4, Rating: 1450},
		{UserID: 5, Rank: 5, Rating: 1400},
	})

	require.Len(t, got, 5)
	wantDeltas := []int{42, 21, 0, -21, -42}
	wantAfter := []int{1642, 1571, 1500, 1429, 1358}
	for i, want := range wantDeltas {
		assert.Equal(t, want, got[i].Delta, "user %d delta", got[i].UserID)
		assert.Equal(t, wantAfter[i], got[i].After, "user %d after", got[i].UserID)
	}
}

func TestCalculateClampsDelta(t *testing.T) {
	participants := make([]rating.Participant, 20)
	for i := range participants {
		participants[i] = rating.Participant{UserID: int64(i + 1), Rank: i + 1, Rating: 1500}
	}

	got := rating.Calculate(participants)
	require.Len(t, got, 20)
	assert.Equal(t, 100, got[0].Delta, "+304 is clamped to +100")
	assert.Equal(t, 1600, got[0].After)
	assert.Equal(t, -100, got[len(got)-1].Delta, "-304 is clamped to -100")
	assert.Equal(t, 1400, got[len(got)-1].After)
}

func TestCalculateClampsRatingFloor(t *testing.T) {
	got := rating.Calculate([]rating.Participant{
		{UserID: 1, Rank: 3, Rating: 20},
		{UserID: 2, Rank: 1, Rating: 0},
		{UserID: 3, Rank: 2, Rating: 0},
	})

	require.Len(t, got, 3)
	assert.Equal(t, -34, got[0].Delta)
	assert.Equal(t, 0, got[0].After, "rating cannot drop below zero")
}
