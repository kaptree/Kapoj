package learning

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxTitleRunes       = 100
	maxDescriptionRunes = 50000
	maxCoverURLBytes    = 512
)

func invalid(format string, args ...any) error {
	return httpx.ErrValidation(fmt.Sprintf(format, args...))
}

// Normalize 清理并校验路线输入。
func (in PathInput) Normalize() (PathInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n < 1 || n > maxTitleRunes {
		return in, invalid("标题长度需为 1–100 个字符")
	}
	if utf8.RuneCountInString(in.DescriptionMD) > maxDescriptionRunes {
		return in, invalid("描述不能超过 50000 个字符")
	}

	in.CoverURL = strings.TrimSpace(in.CoverURL)
	if in.CoverURL != "" {
		u, err := url.Parse(in.CoverURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" || len(in.CoverURL) > maxCoverURLBytes {
			return in, invalid("封面地址需为 http 或 https 链接，且不超过 512 个字符")
		}
	}
	return in, nil
}
