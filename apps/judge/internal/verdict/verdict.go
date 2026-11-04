// Package verdict compares program output and aggregates case results.
package verdict

import (
	"strings"

	"github.com/kaptree/kapoj/apps/judge/internal/model"
)

// Compare reports whether output matches answer, ignoring trailing whitespace
// on each line, CRLF differences and trailing blank lines.
func Compare(output, answer []byte) bool {
	a := normalizedLines(output)
	b := normalizedLines(answer)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizedLines(b []byte) []string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Aggregate returns the overall status, maximum time and memory, and the
// score of accepted cases. Cases are expected in execution order; the first
// non-AC case decides the overall status.
func Aggregate(cases []model.JudgeCaseResult) (status model.Status, timeMS, memoryKB, score int) {
	if len(cases) == 0 {
		return model.StatusSystemError, 0, 0, 0
	}
	status = model.StatusAccepted
	for _, c := range cases {
		if c.TimeMS > timeMS {
			timeMS = c.TimeMS
		}
		if c.MemoryKB > memoryKB {
			memoryKB = c.MemoryKB
		}
		if c.Status == model.StatusAccepted {
			score += c.Score
		} else if status == model.StatusAccepted {
			status = c.Status
		}
	}
	return status, timeMS, memoryKB, score
}
