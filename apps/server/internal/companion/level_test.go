package companion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevelForBond(t *testing.T) {
	cases := []struct {
		bond  int
		level int
		name  string
		next  int
	}{
		{0, 0, "陌生", 20},
		{19, 0, "陌生", 20},
		{20, 1, "认识", 50},
		{100, 3, "搭档", 170},
		{380, 6, "只对你", 380},
	}
	for _, tc := range cases {
		got := LevelForBond(tc.bond)
		assert.Equal(t, tc.level, got.Level)
		assert.Equal(t, tc.name, got.Name)
		assert.Equal(t, tc.next, got.Next)
	}
}
