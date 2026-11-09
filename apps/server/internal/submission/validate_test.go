package submission_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

func validInput() submission.Input {
	return submission.Input{ProblemID: 1, Language: submission.LanguageCPP, Code: "int main() {}"}
}

func validateFor(in submission.Input, typ problem.Type) (submission.Input, error) {
	in, err := in.Normalize()
	if err != nil {
		return in, err
	}
	return in.ValidateForProblem(typ)
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

func TestValidateForTraditionalRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(*submission.Input)
	}{
		{"bad language", func(in *submission.Input) { in.Language = "rust" }},
		{"empty code", func(in *submission.Input) { in.Code = " \n\t " }},
		{"code too large", func(in *submission.Input) { in.Code = strings.Repeat("x", 65537) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.edit(&in)
			_, err := validateFor(in, problem.TypeTraditional)
			assert.Error(t, err)
		})
	}
}

func TestValidateForTraditionalRejectsAnswerLanguage(t *testing.T) {
	in := submission.Input{ProblemID: 1, Language: submission.LanguageAnswer, Code: "int main() {}"}
	in, err := in.Normalize()
	require.NoError(t, err)
	_, err = in.ValidateForProblem(problem.TypeTraditional)
	assertCode(t, err, 40001)
}

func TestValidateForTraditionalAllowsExactCodeLimit(t *testing.T) {
	in := validInput()
	in.Code = strings.Repeat("x", 65536)
	_, err := validateFor(in, problem.TypeTraditional)
	assert.NoError(t, err)
}

func TestInputValidateForObjective(t *testing.T) {
	in := submission.Input{ProblemID: 1, Answer: "  A  "}
	got, err := in.Normalize()
	require.NoError(t, err)
	got, err = got.ValidateForProblem(problem.TypeObjective)
	require.NoError(t, err)
	assert.Equal(t, "A", got.Answer)
	assert.Equal(t, submission.LanguageAnswer, got.Language)
	assert.Empty(t, got.Code)
}

func TestObjectiveRejectsEmptyAnswer(t *testing.T) {
	in := submission.Input{ProblemID: 1, Answer: "  "}
	in, err := in.Normalize()
	require.NoError(t, err)
	_, err = in.ValidateForProblem(problem.TypeObjective)
	assertCode(t, err, 40001)
}

func TestObjectiveRejectsOverlongAnswer(t *testing.T) {
	in := submission.Input{ProblemID: 1, Answer: strings.Repeat("答", 2001)}
	in, err := in.Normalize()
	require.NoError(t, err)
	_, err = in.ValidateForProblem(problem.TypeObjective)
	assertCode(t, err, 40001)
}

func TestObjectiveIgnoresCode(t *testing.T) {
	in := submission.Input{ProblemID: 1, Language: submission.LanguageCPP, Code: "int main() {}", Answer: "B"}
	in, err := in.Normalize()
	require.NoError(t, err)
	got, err := in.ValidateForProblem(problem.TypeObjective)
	require.NoError(t, err)
	assert.Equal(t, submission.LanguageAnswer, got.Language)
	assert.Empty(t, got.Code)
}
