package submission

import (
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

const (
	maxCodeBytes   = 65536
	maxAnswerChars = 2000
)

// Input is the body of POST /submissions.
type Input struct {
	ProblemID int64    `json:"problemId"`
	ContestID int64    `json:"contestId"`
	Language  Language `json:"language"`
	Code      string   `json:"code"`
	Answer    string   `json:"answer"`
}

// Normalize validates fields shared by every problem type. Code is preserved
// exactly, including leading and trailing whitespace. Type-specific rules live
// in ValidateForProblem.
func (in Input) Normalize() (Input, error) {
	if in.ProblemID <= 0 {
		return in, httpx.ErrValidation("problemId 必须是正整数")
	}
	if in.ContestID < 0 {
		return in, httpx.ErrValidation("contestId 不能为负数")
	}
	return in, nil
}

// ValidateForProblem applies the submission payload rules for the problem type.
// Objective problems require a trimmed answer and ignore language/code; code
// problems require a supported language and non-empty code and ignore answer.
func (in Input) ValidateForProblem(t problem.Type) (Input, error) {
	if t == problem.TypeObjective {
		in.Answer = strings.TrimSpace(in.Answer)
		if in.Answer == "" {
			return in, httpx.ErrValidation("答案不能为空")
		}
		if utf8.RuneCountInString(in.Answer) > maxAnswerChars {
			return in, httpx.ErrValidation("答案不能超过 2000 个字符")
		}
		in.Language, in.Code = LanguageAnswer, ""
		return in, nil
	}
	if !in.Language.Valid() {
		return in, httpx.ErrValidation("语言必须是 c、cpp、python3、java 或 go")
	}
	if strings.TrimSpace(in.Code) == "" {
		return in, httpx.ErrValidation("代码不能为空")
	}
	if len(in.Code) > maxCodeBytes {
		return in, httpx.ErrValidation("代码不能超过 64 KiB")
	}
	in.Answer = ""
	return in, nil
}
