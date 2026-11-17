package problem

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceStatementAssetURLsDoesNotUsePrefixMatch(t *testing.T) {
	statement := "a /api/problems/7/assets/4 b /api/problems/7/assets/45"
	got := replaceStatementAssetURLs(statement, 7, map[int64]string{
		4:  "kapoj-package://images/four.png",
		45: "kapoj-package://images/forty-five.png",
	})
	assert.Equal(t,
		"a kapoj-package://images/four.png b kapoj-package://images/forty-five.png",
		got)
}

func TestRewriteImagePlaceholdersUsesLongestMatch(t *testing.T) {
	statement := "a kapoj-package://images/a.png b kapoj-package://images/a.png.bak"
	got := rewriteImagePlaceholders(statement, map[string]string{
		"images/a.png":     "/api/problems/9/assets/1",
		"images/a.png.bak": "/api/problems/9/assets/2",
	})
	assert.Equal(t,
		"a /api/problems/9/assets/1 b /api/problems/9/assets/2",
		got)
}
