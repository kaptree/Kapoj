package storage

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasMountBoundary(t *testing.T) {
	devices := map[string]uint64{
		"/":            1,
		"/data":        2,
		"/data/badges": 2,
		"/app":         1,
		"/app/data":    1,
	}
	device := func(path string) (uint64, error) {
		value, ok := devices[path]
		if !ok {
			return 0, fmt.Errorf("unexpected path %q", path)
		}
		return value, nil
	}

	persistent, err := hasMountBoundary("/data/badges", device)
	require.NoError(t, err)
	assert.True(t, persistent, "a directory below /data must be recognized as persistent")

	persistent, err = hasMountBoundary("/data", device)
	require.NoError(t, err)
	assert.True(t, persistent, "the mount point itself must be recognized as persistent")

	persistent, err = hasMountBoundary("/app/data", device)
	require.NoError(t, err)
	assert.False(t, persistent, "the container writable layer has no mount boundary")

	persistent, err = hasMountBoundary("/", device)
	require.NoError(t, err)
	assert.False(t, persistent)
}
