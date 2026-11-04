package submission

import (
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const maxCodeBytes = 65536

// Input is the body of POST /submissions.
type Input struct {
	ProblemID int64    `json:"problemId"`
	ContestID int64    `json:"contestId"`
	Language  Language `json:"language"`
	Code      string   `json:"code"`
}

// Normalize validates the input and returns the canonical values. Code is
// preserved exactly, including leading and trailing whitespace.
func (in Input) Normalize() (Input, error) {
	if in.ProblemID <= 0 {
		return in, httpx.ErrValidation("problemId 必须是正整数")
	}
	if in.ContestID < 0 {
		return in, httpx.ErrValidation("contestId 不能为负数")
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
	return in, nil
}
