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

// TestCheckProblemTestcaseTotal covers the actual-imported-bytes guard directly.
// Normal zip fixtures cannot reach this branch: archive/zip rejects entries that
// read past their declared UncompressedSize64, so declared sizes always bound
// the bytes AddTestcase can store. The guard protects against metadata that is
// not authoritative (and future reader changes).
func TestCheckProblemTestcaseTotal(t *testing.T) {
	const limit = 6 << 20
	assert.NoError(t, checkProblemTestcaseTotal(limit, limit))
	assert.NoError(t, checkProblemTestcaseTotal(limit-1, limit))

	err := checkProblemTestcaseTotal(limit+1, limit)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "单题测试数据不能超过 6 MiB")
	}
}
