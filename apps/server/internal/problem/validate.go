package problem

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

// Input is the editable part of a problem as sent by clients.
type Input struct {
	Title          string   `json:"title"`
	Slug           string   `json:"slug"`
	StatementMD    string   `json:"statementMd"`
	InputFormatMD  string   `json:"inputFormatMd"`
	OutputFormatMD string   `json:"outputFormatMd"`
	Samples        []Sample `json:"samples"`
	HintMD         string   `json:"hintMd"`
	SolutionMD     string   `json:"solutionMd"`
	SolutionCode   string   `json:"solutionCode"`
	Type           Type     `json:"type"`
	Tags           []string `json:"tags"`
	Difficulty     int      `json:"difficulty"`
	TimeLimitMS    int      `json:"timeLimitMs"`
	MemoryLimitMB  int      `json:"memoryLimitMb"`
	IsPublic       bool     `json:"isPublic"`
	Source         string   `json:"source"`
}

const (
	defaultTimeLimitMS   = 1000
	defaultMemoryLimitMB = 256
	maxTags              = 10
	maxSamples           = 10
	maxSampleBytes       = 16384
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)

func runes(s string) int { return utf8.RuneCountInString(s) }

func invalid(format string, args ...any) error {
	return httpx.ErrValidation(fmt.Sprintf(format, args...))
}

// Normalize trims and canonicalizes the input, applies defaults and checks
// every limit. The returned copy is safe to store.
func (in Input) Normalize() (Input, error) {
	in.Title = strings.TrimSpace(in.Title)
	if n := runes(in.Title); n < 1 || n > 100 {
		return in, invalid("标题长度需为 1–100 个字符")
	}

	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugPattern.MatchString(in.Slug) {
		return in, invalid("题目标识需为 2–63 位小写字母、数字或连字符，且不能以连字符开头")
	}

	if strings.TrimSpace(in.StatementMD) == "" {
		return in, invalid("题面不能为空")
	}
	for _, f := range []struct {
		name  string
		value string
		max   int
	}{
		{"题面", in.StatementMD, 100000},
		{"输入格式", in.InputFormatMD, 20000},
		{"输出格式", in.OutputFormatMD, 20000},
		{"提示", in.HintMD, 50000},
		{"解题思路", in.SolutionMD, 50000},
		{"来源", in.Source, 200},
	} {
		if runes(f.value) > f.max {
			return in, invalid("%s不能超过 %d 个字符", f.name, f.max)
		}
	}
	if len(in.SolutionCode) > 65536 {
		return in, invalid("解题代码不能超过 65536 字节")
	}

	if in.Type == "" {
		in.Type = TypeTraditional
	}
	if !in.Type.Valid() {
		return in, invalid("题目类型必须是 traditional、special 或 interactive")
	}
	if in.Difficulty < 1 || in.Difficulty > 5 {
		return in, invalid("难度需为 1–5")
	}

	if in.TimeLimitMS == 0 {
		in.TimeLimitMS = defaultTimeLimitMS
	}
	if in.TimeLimitMS < 100 || in.TimeLimitMS > 10000 {
		return in, invalid("时间限制需为 100–10000 毫秒")
	}
	if in.MemoryLimitMB == 0 {
		in.MemoryLimitMB = defaultMemoryLimitMB
	}
	if in.MemoryLimitMB < 16 || in.MemoryLimitMB > 1024 {
		return in, invalid("内存限制需为 16–1024 MB")
	}

	tags, err := normalizeTags(in.Tags)
	if err != nil {
		return in, err
	}
	in.Tags = tags

	if len(in.Samples) > maxSamples {
		return in, invalid("样例最多 %d 组", maxSamples)
	}
	for _, s := range in.Samples {
		if len(s.Input) > maxSampleBytes || len(s.Output) > maxSampleBytes {
			return in, invalid("单个样例的输入或输出不能超过 %d 字节", maxSampleBytes)
		}
	}
	if in.Samples == nil {
		in.Samples = []Sample{}
	}
	return in, nil
}

func normalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if n := runes(t); n < 1 || n > 20 {
			return nil, invalid("每个标签长度需为 1–20 个字符")
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) > maxTags {
		return nil, invalid("标签最多 %d 个", maxTags)
	}
	return out, nil
}
