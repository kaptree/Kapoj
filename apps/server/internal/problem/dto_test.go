package problem

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullProblem() *Problem {
	return &Problem{
		ID: 3, Title: "T", Slug: "t", StatementMD: "s", InputFormatMD: "i", OutputFormatMD: "o",
		Samples: []Sample{{Input: "1", Output: "2"}}, HintMD: "hint", SolutionMD: "secret idea",
		SolutionCode: "secret code", Type: TypeSpecial, Tags: []string{"x"}, Difficulty: 2,
		TimeLimitMS: 1000, MemoryLimitMB: 256, CheckerPath: "problems/3/checker.cpp",
		IsPublic: true, Source: "src", SubmitCount: 4, AcceptCount: 1, CreatedBy: 9,
		CreatedAt: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
	}
}

func keys(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestDetailDTOHidesManagerFields(t *testing.T) {
	user := keys(t, ToDetail(fullProblem(), false, false, nil))
	for _, k := range []string{"solutionMd", "solutionCode", "checkerUploaded", "createdBy", "checkerPath", "checker_path"} {
		assert.NotContains(t, user, k, "non-managers must not see %s", k)
	}
	assert.Equal(t, "hint", user["hintMd"], "hint is public")
	assert.Equal(t, "s", user["statementMd"])
	assert.EqualValues(t, 3, user["id"])
	assert.NotContains(t, user, "checkerPath")

	mgr := keys(t, ToDetail(fullProblem(), true, true, nil))
	assert.Equal(t, "secret idea", mgr["solutionMd"])
	assert.Equal(t, "secret code", mgr["solutionCode"])
	assert.Equal(t, true, mgr["checkerUploaded"])
	assert.EqualValues(t, 9, mgr["createdBy"])
	assert.NotContains(t, mgr, "checkerPath", "the storage path is never exposed")
}

func TestDetailDTOManagerEmptyValuesStillPresent(t *testing.T) {
	p := fullProblem()
	p.SolutionMD, p.SolutionCode, p.CheckerPath = "", "", ""
	mgr := keys(t, ToDetail(p, true, true, nil))
	assert.Contains(t, mgr, "solutionMd")
	assert.Contains(t, mgr, "solutionCode")
	assert.Equal(t, false, mgr["checkerUploaded"])
}

func TestListItemHasNoSecretsOrBodies(t *testing.T) {
	item := keys(t, ToListItem(fullProblem()))
	for _, k := range []string{"solutionMd", "solutionCode", "checkerUploaded", "createdBy", "statementMd", "hintMd"} {
		assert.NotContains(t, item, k)
	}
	assert.Equal(t, "t", item["slug"])
	assert.EqualValues(t, 4, item["submitCount"])
}

func TestDTOArraysNeverNull(t *testing.T) {
	p := &Problem{ID: 1}
	for name, m := range map[string]map[string]any{
		"list":   keys(t, ToListItem(p)),
		"detail": keys(t, ToDetail(p, false, false, nil)),
	} {
		assert.Equal(t, []any{}, m["tags"], name)
	}
	assert.Equal(t, []any{}, keys(t, ToDetail(p, false, false, nil))["samples"])
}

func TestDetailDTOAnswersOnlyWhenIncluded(t *testing.T) {
	p := fullProblem()
	p.Answers = []string{"A", "B"}

	assert.NotContains(t, keys(t, ToDetail(p, false, false, nil)), "answers",
		"answers are hidden when not explicitly included")

	d := keys(t, ToDetail(p, false, true, nil))
	assert.Equal(t, []any{"A", "B"}, d["answers"])

	p.Answers = nil
	d = keys(t, ToDetail(p, false, true, nil))
	assert.Equal(t, []any{}, d["answers"], "included answers are never null")
}

func TestDetailAuthorOptional(t *testing.T) {
	author := &AuthorSummary{ID: 9, Username: "alice", Nickname: "Alice"}
	with := keys(t, ToDetail(fullProblem(), false, false, author))
	assert.Equal(t, map[string]any{
		"id": float64(9), "username": "alice", "nickname": "Alice",
	}, with["author"], "author is public and carries only id/username/nickname")

	without := keys(t, ToDetail(fullProblem(), false, false, nil))
	assert.NotContains(t, without, "author", "a nil author omits the key entirely")
}
