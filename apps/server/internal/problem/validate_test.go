package problem

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

func validInput() Input {
	return Input{
		Title: "A+B", Slug: "a-plus-b", StatementMD: "add two numbers",
		Type: TypeTraditional, Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
	}
}

func assertInvalid(t *testing.T, in Input, name string) {
	t.Helper()
	_, err := in.Normalize()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "%s: want AppError, got %v", name, err) {
		assert.Equal(t, 40001, appErr.Code, name)
	}
}

func TestNormalizeAcceptsValidInput(t *testing.T) {
	got, err := validInput().Normalize()
	require.NoError(t, err)
	assert.Equal(t, "a-plus-b", got.Slug)
	assert.NotNil(t, got.Tags)
	assert.NotNil(t, got.Samples)
}

func TestNormalizeSlug(t *testing.T) {
	in := validInput()
	in.Slug = "  A-B  "
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, "a-b", got.Slug, "slug is trimmed and lowercased")

	in.Slug = strings.Repeat("a", 63)
	_, err = in.Normalize()
	assert.NoError(t, err)

	for _, bad := range []string{"", "a", "-ab", strings.Repeat("a", 64), "has space", "under_score", "中文"} {
		in.Slug = bad
		assertInvalid(t, in, "slug "+bad)
	}
}

func TestNormalizeTitle(t *testing.T) {
	in := validInput()
	in.Title = "  spaced  "
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, "spaced", got.Title)

	in.Title = strings.Repeat("字", 100)
	_, err = in.Normalize()
	assert.NoError(t, err)

	for _, bad := range []string{"", "   ", strings.Repeat("字", 101)} {
		in.Title = bad
		assertInvalid(t, in, "title")
	}
}

func TestNormalizeTextLimits(t *testing.T) {
	cases := map[string]func(*Input){
		"statement empty":    func(i *Input) { i.StatementMD = "" },
		"statement too long": func(i *Input) { i.StatementMD = strings.Repeat("字", 100001) },
		"input format":       func(i *Input) { i.InputFormatMD = strings.Repeat("a", 20001) },
		"output format":      func(i *Input) { i.OutputFormatMD = strings.Repeat("a", 20001) },
		"hint":               func(i *Input) { i.HintMD = strings.Repeat("a", 50001) },
		"solution":           func(i *Input) { i.SolutionMD = strings.Repeat("a", 50001) },
		"solution code":      func(i *Input) { i.SolutionCode = strings.Repeat("a", 65537) },
		"source":             func(i *Input) { i.Source = strings.Repeat("a", 201) },
	}
	for name, mutate := range cases {
		in := validInput()
		mutate(&in)
		assertInvalid(t, in, name)
	}

	ok := validInput()
	ok.StatementMD = strings.Repeat("字", 100000)
	ok.SolutionCode = strings.Repeat("a", 65536)
	_, err := ok.Normalize()
	assert.NoError(t, err)
}

func TestNormalizeLimitsAndDefaults(t *testing.T) {
	in := validInput()
	in.TimeLimitMS, in.MemoryLimitMB = 0, 0
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, 1000, got.TimeLimitMS)
	assert.Equal(t, 256, got.MemoryLimitMB)

	for _, v := range []int{100, 10000} {
		in.TimeLimitMS = v
		_, err := in.Normalize()
		assert.NoError(t, err, "time %d", v)
	}
	for _, v := range []int{-1, 99, 10001} {
		in = validInput()
		in.TimeLimitMS = v
		assertInvalid(t, in, "time")
	}
	for _, v := range []int{16, 1024} {
		in = validInput()
		in.MemoryLimitMB = v
		_, err := in.Normalize()
		assert.NoError(t, err, "memory %d", v)
	}
	for _, v := range []int{-1, 15, 1025} {
		in = validInput()
		in.MemoryLimitMB = v
		assertInvalid(t, in, "memory")
	}
}

func TestNormalizeDifficultyAndType(t *testing.T) {
	for _, d := range []int{0, 6, -1} {
		in := validInput()
		in.Difficulty = d
		assertInvalid(t, in, "difficulty")
	}
	in := validInput()
	in.Type = ""
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, TypeTraditional, got.Type, "empty type defaults to traditional")

	in.Type = "weird"
	assertInvalid(t, in, "type")
}

func TestNormalizeTags(t *testing.T) {
	in := validInput()
	in.Tags = []string{"a", " a ", "b", "字"}
	got, err := in.Normalize()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "字"}, got.Tags, "trimmed and de-duplicated, order kept")

	in.Tags = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	_, err = in.Normalize()
	assert.NoError(t, err)

	in.Tags = append(in.Tags, "11")
	assertInvalid(t, in, "11 tags")
	in.Tags = []string{""}
	assertInvalid(t, in, "empty tag")
	in.Tags = []string{strings.Repeat("字", 21)}
	assertInvalid(t, in, "long tag")
}

func TestNormalizeSamples(t *testing.T) {
	in := validInput()
	in.Samples = make([]Sample, 10)
	_, err := in.Normalize()
	assert.NoError(t, err)

	in.Samples = make([]Sample, 11)
	assertInvalid(t, in, "11 samples")

	in.Samples = []Sample{{Input: strings.Repeat("a", 16385)}}
	assertInvalid(t, in, "big sample input")
	in.Samples = []Sample{{Output: strings.Repeat("a", 16385)}}
	assertInvalid(t, in, "big sample output")
	in.Samples = []Sample{{Input: strings.Repeat("a", 16384), Output: strings.Repeat("a", 16384)}}
	_, err = in.Normalize()
	assert.NoError(t, err)
}
