package contest

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

type ProblemInput struct {
	ProblemID int64  `json:"problemId"`
	DisplayID string `json:"displayId"`
	Score     int    `json:"score"`
}

type Input struct {
	Title         string         `json:"title"`
	DescriptionMD string         `json:"descriptionMd"`
	StartTime     time.Time      `json:"startTime"`
	EndTime       time.Time      `json:"endTime"`
	Mode          Mode           `json:"mode"`
	IsRated       bool           `json:"isRated"`
	IsPublic      bool           `json:"isPublic"`
	DisableAI     *bool          `json:"disableAi"`
	RegisterType  RegisterType   `json:"registerType"`
	Password      string         `json:"password"`
	InviteCode    string         `json:"inviteCode"`
	Problems      []ProblemInput `json:"problems"`
}

var displayIDPattern = regexp.MustCompile(`^[A-Z]$`)

func invalid(format string, args ...any) error {
	return httpx.ErrValidation(fmt.Sprintf(format, args...))
}

func (in Input) Normalize() (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > 100 {
		return in, invalid("比赛标题长度需为 1–100 个字符")
	}
	if utf8.RuneCountInString(in.DescriptionMD) > 100000 {
		return in, invalid("比赛描述不能超过 100000 个字符")
	}
	if in.StartTime.IsZero() || in.EndTime.IsZero() {
		return in, invalid("比赛开始与结束时间不能为空")
	}
	in.StartTime, in.EndTime = in.StartTime.UTC(), in.EndTime.UTC()
	if !in.StartTime.Before(in.EndTime) {
		return in, invalid("比赛结束时间必须晚于开始时间")
	}
	if !in.Mode.Valid() {
		return in, invalid("赛制必须是 icpc、ioi 或 oi")
	}
	if in.RegisterType == "" {
		in.RegisterType = RegisterOpen
	}
	if !in.RegisterType.Valid() {
		return in, invalid("报名方式必须是 open、password 或 invite")
	}
	in.Password = strings.TrimSpace(in.Password)
	in.InviteCode = strings.TrimSpace(in.InviteCode)
	if in.Password != "" && (len(in.Password) < 4 || len(in.Password) > 64) {
		return in, invalid("比赛密码长度需为 4–64 字节")
	}
	if in.InviteCode != "" && (len(in.InviteCode) < 4 || len(in.InviteCode) > 64) {
		return in, invalid("邀请码长度需为 4–64 字节")
	}
	if len(in.Problems) > 26 {
		return in, invalid("比赛最多 26 道题")
	}
	seenProblem := map[int64]bool{}
	seenDisplay := map[string]bool{}
	for i := range in.Problems {
		p := &in.Problems[i]
		p.DisplayID = strings.ToUpper(strings.TrimSpace(p.DisplayID))
		if p.ProblemID <= 0 {
			return in, invalid("题目 ID 必须是正整数")
		}
		if !displayIDPattern.MatchString(p.DisplayID) {
			return in, invalid("题目编号必须是单个大写字母 A–Z")
		}
		if seenProblem[p.ProblemID] {
			return in, invalid("同一比赛不能重复添加题目")
		}
		if seenDisplay[p.DisplayID] {
			return in, invalid("同一比赛的题目编号不能重复")
		}
		if p.Score < 1 || p.Score > 1000 {
			return in, invalid("题目分数需为 1–1000")
		}
		seenProblem[p.ProblemID] = true
		seenDisplay[p.DisplayID] = true
	}
	if in.Problems == nil {
		in.Problems = []ProblemInput{}
	}
	return in, nil
}
