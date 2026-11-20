package userimport

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	testHash = "c519b45b4e118f223b8edfa027e6b07a3446fa506e634ff3f71608fb6abf5d17"
	testSalt = "aB3xY9mKpQ2rStUvWxYz0123456789AB"
)

func csvText(lines ...string) string {
	return "_id,uname,mail,hash,salt,hashType\n" + strings.Join(lines, "\n") + "\n"
}

func requireValidationError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "expected AppError, got %T: %v", err, err)
	assert.Equal(t, 40001, appErr.Code)
}

func TestParseHydroCSVValidRowAndBOM(t *testing.T) {
	rows, err := parseHydroCSV(strings.NewReader("\ufeff" + csvText(
		"2,Alice,Alice@Example.com,"+testHash+","+testSalt+",hydro",
	)))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 2, rows[0].line)
	assert.Equal(t, int64(2), rows[0].legacyID)
	assert.Equal(t, "Alice", rows[0].uname)
	assert.Equal(t, "alice@example.com", rows[0].email)
	assert.Equal(t, strings.ToLower(testHash), rows[0].hash)
	assert.Equal(t, testSalt, rows[0].salt)
	assert.Equal(t, "hydro", rows[0].hashType)
	assert.Equal(t, rowValid, rows[0].status)
}

func TestParseHydroCSVSkipsBlankLinesAndIgnoresExtraColumns(t *testing.T) {
	csv := "_id,uname,mail,hash,salt,hashType,extra\n" +
		"\n" +
		"2,Alice,a@example.com," + testHash + "," + testSalt + ",hydro,ignored\n" +
		"   \n"
	rows, err := parseHydroCSV(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, rowValid, rows[0].status)
}

func TestParseHydroCSVInvalidRows(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"bad id", "x,Alice,a@example.com," + testHash + "," + testSalt + ",hydro"},
		{"zero id", "0,Alice,a@example.com," + testHash + "," + testSalt + ",hydro"},
		{"empty uname", "2,,a@example.com," + testHash + "," + testSalt + ",hydro"},
		{"bad email", "2,Alice,not-an-email," + testHash + "," + testSalt + ",hydro"},
		{"short hash", "2,Alice,a@example.com,short," + testSalt + ",hydro"},
		{"bad hash hex", "2,Alice,a@example.com," + strings.Repeat("z", 64) + "," + testSalt + ",hydro"},
		{"short salt", "2,Alice,a@example.com," + testHash + ",short,hydro"},
		{"bad hash type", "2,Alice,a@example.com," + testHash + "," + testSalt + ",bcrypt"},
		{"control char", "2,Alice\u0001,a@example.com," + testHash + "," + testSalt + ",hydro"},
		{"missing columns", "2,Alice,a@example.com," + testHash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := parseHydroCSV(strings.NewReader(csvText(tc.line)))
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, rowInvalid, rows[0].status)
			assert.NotEmpty(t, rows[0].reason)
		})
	}
}

func TestParseHydroCSVDuplicateRowsAllInvalid(t *testing.T) {
	rows, err := parseHydroCSV(strings.NewReader(csvText(
		"2,Alice,a@example.com,"+testHash+","+testSalt+",hydro",
		"3,Bob,a@example.com,"+testHash+","+testSalt+",hydro",
	)))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, rowInvalid, rows[0].status)
	assert.Equal(t, rowInvalid, rows[1].status)
	assert.Contains(t, rows[0].reason, "重复")
	assert.Contains(t, rows[1].reason, "重复")
}

func TestParseHydroCSVRejectsBadInput(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, err := parseHydroCSV(strings.NewReader(""))
		requireValidationError(t, err)
	})
	t.Run("missing header", func(t *testing.T) {
		_, err := parseHydroCSV(strings.NewReader("_id,uname\n2,Alice\n"))
		requireValidationError(t, err)
	})
	t.Run("non utf8", func(t *testing.T) {
		_, err := parseHydroCSV(strings.NewReader(string([]byte{0xff, 0xfe, 0xfd})))
		requireValidationError(t, err)
	})
	t.Run("too many rows", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("_id,uname,mail,hash,salt,hashType\n")
		for i := 1; i <= maxCSVRows+1; i++ {
			fmt.Fprintf(&b, "%d,u%d,u%d@example.com,%s,%s,hydro\n", i, i, i, testHash, testSalt)
		}
		_, err := parseHydroCSV(strings.NewReader(b.String()))
		requireValidationError(t, err)
	})
	t.Run("too large", func(t *testing.T) {
		_, err := parseHydroCSV(strings.NewReader(strings.Repeat("a", maxCSVBytes+1)))
		requireValidationError(t, err)
	})
}

func TestApplyUsernames(t *testing.T) {
	rows := []*row{
		{line: 2, legacyID: 2, uname: "Alice", status: rowValid},
		{line: 3, legacyID: 6, uname: "让我睡觉", status: rowValid},
		{line: 4, legacyID: 7, uname: "hydro_7", status: rowValid},
		{line: 5, legacyID: 8, uname: "a b", status: rowValid},
		{line: 6, legacyID: 9, uname: "ab", status: rowValid},
	}
	taken := map[string]struct{}{"hydro_9": {}}
	applyUsernames(rows, taken)

	assert.Equal(t, "alice", rows[0].username)
	assert.False(t, rows[0].generated)
	assert.Equal(t, "hydro_6", rows[1].username)
	assert.True(t, rows[1].generated)
	assert.Equal(t, "hydro_7", rows[2].username, "explicit valid names are reserved first")
	assert.False(t, rows[2].generated)
	assert.Equal(t, "hydro_8", rows[3].username)
	assert.True(t, rows[3].generated)
	assert.Equal(t, "hydro_9_2", rows[4].username, "generated collisions get a suffix")
	assert.True(t, rows[4].generated)
	assert.Contains(t, taken, "hydro_6")
	assert.Contains(t, taken, "hydro_8")
	assert.Contains(t, taken, "hydro_9_2")
}

func TestApplyUsernamesLeavesInvalidRowsAlone(t *testing.T) {
	rows := []*row{
		{line: 2, legacyID: 6, uname: "让我睡觉", status: rowInvalid, reason: "邮箱格式不正确"},
	}
	applyUsernames(rows, map[string]struct{}{})
	assert.Empty(t, rows[0].username)
	assert.False(t, rows[0].generated)
}

func TestApplyUsernamesSuffixesPastMultipleCollisions(t *testing.T) {
	rows := []*row{{line: 2, legacyID: 6, uname: "让我睡觉", status: rowValid}}
	applyUsernames(rows, map[string]struct{}{
		"hydro_6":   {},
		"hydro_6_2": {},
		"hydro_6_3": {},
	})
	assert.Equal(t, "hydro_6_4", rows[0].username)
}
