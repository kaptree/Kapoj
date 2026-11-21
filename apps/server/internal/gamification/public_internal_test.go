package gamification

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyIdentityMarshalsEmptyBadges(t *testing.T) {
	raw, err := json.Marshal(emptyIdentity())
	require.NoError(t, err)
	assert.JSONEq(t, `{"equippedBadges":[],"equippedTitle":null,"level":{"key":"gray","name":"灰名","color":"#808080","accentColor":""}}`, string(raw))
}
