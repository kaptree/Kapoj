package submission

import (
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// ListQuery is the user-facing filter set for GET /submissions.
type ListQuery struct {
	ProblemID int64
	ContestID *int64
	UserID    int64
	Language  Language
	Status    Status
	Nickname  string
}

func (q ListQuery) Normalize() (ListQuery, error) {
	if q.ProblemID < 0 {
		return q, httpx.ErrValidation("problemId 不能为负数")
	}
	if q.UserID < 0 {
		return q, httpx.ErrValidation("userId 不能为负数")
	}
	if q.ContestID != nil && *q.ContestID < 0 {
		return q, httpx.ErrValidation("contestId 不能为负数")
	}
	if q.Language != "" && !q.Language.Valid() {
		return q, httpx.ErrValidation("language 必须是 c、cpp、python3、java 或 go")
	}
	if q.Status != "" && !q.Status.Valid() {
		return q, httpx.ErrValidation("status 不是合法的判题状态")
	}
	q.Nickname = strings.TrimSpace(q.Nickname)
	if utf8.RuneCountInString(q.Nickname) > 64 {
		return q, httpx.ErrValidation("nickname 不能超过 64 个字符")
	}
	return q, nil
}

// ListFilter is the repository-level filter set.
type ListFilter struct {
	ProblemID int64
	ContestID *int64
	UserID    int64
	Language  Language
	Status    Status
	Nickname  string
}

func toFilter(q ListQuery) ListFilter { return ListFilter(q) }
