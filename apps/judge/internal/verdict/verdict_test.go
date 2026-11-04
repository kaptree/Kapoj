package verdict_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
	"github.com/kaptree/kapoj/apps/judge/internal/verdict"
)

func TestCompareIgnoresLineEndWhitespace(t *testing.T) {
	tests := []struct {
		name   string
		output string
		answer string
		want   bool
	}{
		{"identical", "1 2\n3\n", "1 2\n3\n", true},
		{"crlf", "1 2\r\n3\r\n", "1 2\n3\n", true},
		{"trailing spaces", "1 2  \n3\t\n", "1 2\n3\n", true},
		{"trailing blank lines", "1\n2\n\n\n", "1\n2\n", true},
		{"different number", "1\n4\n", "1\n3\n", false},
		{"extra line", "1\n2\n3\n", "1\n2\n", false},
		{"missing line", "1\n", "1\n2\n", false},
		{"empty and whitespace", " \n\t\n", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, verdict.Compare([]byte(tt.output), []byte(tt.answer)))
		})
	}
}

func TestAggregate(t *testing.T) {
	cases := []model.JudgeCaseResult{
		{Status: model.StatusAccepted, TimeMS: 10, MemoryKB: 100, Score: 10},
		{Status: model.StatusAccepted, TimeMS: 30, MemoryKB: 80, Score: 20},
	}
	status, timeMS, memoryKB, score := verdict.Aggregate(cases)
	assert.Equal(t, model.StatusAccepted, status)
	assert.Equal(t, 30, timeMS)
	assert.Equal(t, 100, memoryKB)
	assert.Equal(t, 30, score)

	failed := append(cases, model.JudgeCaseResult{Status: model.StatusWrongAnswer, TimeMS: 5, MemoryKB: 50, Score: 10})
	status, timeMS, memoryKB, score = verdict.Aggregate(failed)
	assert.Equal(t, model.StatusWrongAnswer, status)
	assert.Equal(t, 30, timeMS)
	assert.Equal(t, 100, memoryKB)
	assert.Equal(t, 30, score)

	status, _, _, score = verdict.Aggregate(nil)
	assert.Equal(t, model.StatusSystemError, status)
	assert.Zero(t, score)
}
