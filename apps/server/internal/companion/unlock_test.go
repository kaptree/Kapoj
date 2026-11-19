package companion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/companion"
)

func TestLevelUnlocks(t *testing.T) {
	assert.Equal(t, []string{"outfit-1"}, companion.LevelUnlocks(0))
	assert.Equal(t, []string{"outfit-1", "outfit-2"}, companion.LevelUnlocks(20))
	assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3"}, companion.LevelUnlocks(50))
	assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3", "outfit-4"}, companion.LevelUnlocks(170))
	assert.Equal(t, []string{"outfit-1", "outfit-2", "outfit-3", "outfit-4", "outfit-5"}, companion.LevelUnlocks(380))
}
