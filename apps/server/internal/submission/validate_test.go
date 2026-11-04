package submission_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

func validInput() submission.Input {
	return submission.Input{ProblemID: 1, Language: submission.LanguageCPP, Code: "int main() {}"}
}

func TestInputNormalize(t *testing.T) {
	in, err := validInput().Normalize()
	assert.NoError(t, err)
	assert.Equal(t, "int main() {}", in.Code)
}

func TestInputNormalizeRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(*submission.Input)
	}{
		{"missing problem", func(in *submission.Input) { in.ProblemID = 0 }},
		{"bad language", func(in *submission.Input) { in.Language = "rust" }},
		{"empty code", func(in *submission.Input) { in.Code = " \n\t " }},
		{"code too large", func(in *submission.Input) { in.Code = strings.Repeat("x", 65537) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.edit(&in)
			_, err := in.Normalize()
			assert.Error(t, err)
		})
	}
}

func TestInputNormalizeAllowsExactCodeLimit(t *testing.T) {
	in := validInput()
	in.Code = strings.Repeat("x", 65536)
	_, err := in.Normalize()
	assert.NoError(t, err)
}
