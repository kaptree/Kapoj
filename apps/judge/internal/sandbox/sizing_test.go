package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEffectiveOutputLimit(t *testing.T) {
	assert.Equal(t, defaultOutputLimit, effectiveOutputLimit(0))
	assert.Equal(t, defaultOutputLimit, effectiveOutputLimit(-1))
	assert.Equal(t, 4096, effectiveOutputLimit(4096))
	assert.Equal(t, 16<<20, effectiveOutputLimit(16<<20))
}

func TestOutTmpfsSize(t *testing.T) {
	// 1 MiB + overhead still floors at the 8 MiB minimum.
	assert.Equal(t, "size=8m", outTmpfsSize(1<<20))
	// 6 MiB + 4 MiB overhead = 10 MiB.
	assert.Equal(t, "size=10m", outTmpfsSize(6<<20))
	// 16 MiB + 4 MiB overhead = 20 MiB.
	assert.Equal(t, "size=20m", outTmpfsSize(16<<20))
}

func TestOutputFileBlocks(t *testing.T) {
	// 8 MiB floor -> ceil(8 MiB / 512) blocks.
	assert.Equal(t, 16384, outputFileBlocks(1<<20))
	// 10 MiB -> ceil(10 MiB / 512) blocks.
	assert.Equal(t, 20480, outputFileBlocks(6<<20))
	// A 1024-byte-block shell gives 2x the bytes, which is still >= the limit.
	for _, limit := range []int{1 << 20, 6 << 20, 16 << 20} {
		blocks := outputFileBlocks(limit)
		assert.GreaterOrEqual(t, blocks*512, outputFileLimit(limit))
	}
}
