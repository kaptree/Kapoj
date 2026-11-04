package httpx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePageClamps(t *testing.T) {
	cases := []struct {
		query      string
		page, size int
	}{
		{"", 1, 20},
		{"page=0&page_size=0", 1, 20},
		{"page=3&page_size=500", 3, 100},
		{"page=x&page_size=y", 1, 20},
	}
	for _, tc := range cases {
		page, size := ParsePage(tc.query)
		assert.Equal(t, tc.page, page, tc.query)
		assert.Equal(t, tc.size, size, tc.query)
	}
}
