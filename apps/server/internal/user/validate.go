package user

import (
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

// Normalize trims and lowercases usernames and emails so uniqueness ignores case.
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func ValidateUsername(s string) error {
	if !usernamePattern.MatchString(s) {
		return httpx.ErrValidation("用户名需为 3–32 位字母、数字或下划线")
	}
	return nil
}

func ValidateEmail(s string) error {
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > 254 {
		return httpx.ErrValidation("邮箱格式不正确")
	}
	return nil
}

// ValidatePassword enforces 8–72 bytes; 72 is bcrypt's input limit.
func ValidatePassword(s string) error {
	if len(s) < 8 || len(s) > 72 {
		return httpx.ErrValidation("密码长度需为 8–72 个字节")
	}
	return nil
}

func ValidateNickname(s string) error {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	if n < 1 || n > 32 {
		return httpx.ErrValidation("昵称长度需为 1–32 个字符")
	}
	return nil
}

func ValidateBio(s string) error {
	if utf8.RuneCountInString(s) > 200 {
		return httpx.ErrValidation("个人简介不能超过 200 个字符")
	}
	return nil
}

func ValidateAvatarURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(s) > 512 {
		return httpx.ErrValidation("头像地址需为 http 或 https 链接，且不超过 512 个字符")
	}
	return nil
}
