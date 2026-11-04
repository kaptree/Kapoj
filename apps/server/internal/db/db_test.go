package db

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerHidesQueryParameters(t *testing.T) {
	dsn := os.Getenv("KAPOJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KAPOJ_TEST_DATABASE_URL not set; skipping database test")
	}
	var logs bytes.Buffer
	g, err := open(dsn, &logs)
	require.NoError(t, err)
	require.NoError(t, Migrate(g))

	marker := "SECRET_QUERY_PARAMETER"
	require.NoError(t, g.Exec("SELECT pg_sleep(0.25), ?", marker).Error)
	assert.Contains(t, logs.String(), "SLOW SQL", "the slow-query path must be exercised")
	assert.NotContains(t, logs.String(), marker)
}
