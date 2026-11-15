package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyWithinPrefixUsesSegmentBoundaries(t *testing.T) {
	assert.True(t, keyWithinPrefix("generated/1/a.in", "generated/1"))
	assert.True(t, keyWithinPrefix("generated/1", "generated/1/"))
	assert.False(t, keyWithinPrefix("generated/10/a.in", "generated/1"))
	assert.True(t, keyWithinPrefix("badges/1", ""))
	assert.True(t, keyWithinPrefix("badges/1", "badges/"))
}
