package site

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const (
	maxEyebrowLen          = 40
	maxTitleLen            = 60
	maxTitleHighlightLen   = 30
	maxDescriptionLen      = 300
	maxCTATitleLen         = 20
	maxLinkTitleLen        = 40
	maxLinkDescriptionLen  = 120
	maxSectionTitleLen     = 20
	maxHomeLinks           = 8
	maxGuestTitleLen       = 60
	maxGuestDescriptionLen = 300

	maxFooterICPLen    = 100
	maxFooterPoliceLen = 100
	maxFooterURLLen    = 500
	maxFooterCopyLen   = 200
	maxFooterTextLen   = 1000
)

// validateHome trims all text and enforces the home page rules from the
// design document. The receiver is updated in place with normalized values.
func validateHome(in *HomeContent) error {
	trimHome(in)

	if err := requireMax("首页眉标", in.Eyebrow, maxEyebrowLen, false); err != nil {
		return err
	}
	if err := requireMax("首页标题", in.Title, maxTitleLen, false); err != nil {
		return err
	}
	if err := requireMax("首页标题高亮", in.TitleHighlight, maxTitleHighlightLen, false); err != nil {
		return err
	}
	if err := requireMax("首页描述", in.Description, maxDescriptionLen, false); err != nil {
		return err
	}
	if err := validateLink("主按钮", &in.PrimaryCTA, maxCTATitleLen); err != nil {
		return err
	}
	if err := validateLink("次按钮", &in.SecondaryCTA, maxCTATitleLen); err != nil {
		return err
	}
	if err := requireMax("快速开始标题", in.QuickTitle, maxSectionTitleLen, false); err != nil {
		return err
	}
	if err := requireMax("成长中心标题", in.GrowthTitle, maxSectionTitleLen, false); err != nil {
		return err
	}
	if err := validateLinks("快速开始", in.QuickLinks); err != nil {
		return err
	}
	if err := validateLinks("成长中心", in.GrowthLinks); err != nil {
		return err
	}
	if err := requireMax("游客提示标题", in.GuestTitle, maxGuestTitleLen, false); err != nil {
		return err
	}
	if err := requireMax("游客提示描述", in.GuestDescription, maxGuestDescriptionLen, false); err != nil {
		return err
	}
	return nil
}

// validateFooter trims all text and enforces the footer rules from the design
// document. StartedAt is normalized to UTC.
func validateFooter(in *FooterInput) error {
	in.ICPNumber = strings.TrimSpace(in.ICPNumber)
	in.PoliceNumber = strings.TrimSpace(in.PoliceNumber)
	in.PoliceURL = strings.TrimSpace(in.PoliceURL)
	in.Copyright = strings.TrimSpace(in.Copyright)
	in.Text = strings.TrimSpace(in.Text)
	in.StartedAt = in.StartedAt.UTC()

	if err := requireMax("ICP 备案号", in.ICPNumber, maxFooterICPLen, false); err != nil {
		return err
	}
	if err := requireMax("公安备案号", in.PoliceNumber, maxFooterPoliceLen, false); err != nil {
		return err
	}
	if err := requireMax("公安备案链接", in.PoliceURL, maxFooterURLLen, false); err != nil {
		return err
	}
	if in.PoliceURL != "" && !strings.HasPrefix(in.PoliceURL, "https://") {
		return httpx.ErrValidation("公安备案链接必须以 https:// 开头")
	}
	if err := requireMax("版权信息", in.Copyright, maxFooterCopyLen, false); err != nil {
		return err
	}
	if err := requireMax("页脚说明", in.Text, maxFooterTextLen, true); err != nil {
		return err
	}
	if in.StartedAt.IsZero() {
		return httpx.ErrValidation("网站运行起始时间不能为空")
	}
	if in.StartedAt.After(time.Now().UTC().Add(24 * time.Hour)) {
		return httpx.ErrValidation("网站运行起始时间不能晚于当前时间")
	}
	return nil
}

func trimHome(in *HomeContent) {
	in.Eyebrow = strings.TrimSpace(in.Eyebrow)
	in.Title = strings.TrimSpace(in.Title)
	in.TitleHighlight = strings.TrimSpace(in.TitleHighlight)
	in.Description = strings.TrimSpace(in.Description)
	trimLink(&in.PrimaryCTA)
	trimLink(&in.SecondaryCTA)
	in.QuickTitle = strings.TrimSpace(in.QuickTitle)
	for i := range in.QuickLinks {
		trimLink(&in.QuickLinks[i])
	}
	in.GrowthTitle = strings.TrimSpace(in.GrowthTitle)
	for i := range in.GrowthLinks {
		trimLink(&in.GrowthLinks[i])
	}
	in.GuestTitle = strings.TrimSpace(in.GuestTitle)
	in.GuestDescription = strings.TrimSpace(in.GuestDescription)
}

func trimLink(l *Link) {
	l.Icon = strings.TrimSpace(l.Icon)
	l.IconPos = strings.TrimSpace(l.IconPos)
	l.Title = strings.TrimSpace(l.Title)
	l.Description = strings.TrimSpace(l.Description)
	l.To = strings.TrimSpace(l.To)
}

func validateLinks(label string, links []Link) error {
	if len(links) > maxHomeLinks {
		return httpx.ErrValidation(fmt.Sprintf("%s最多只能有 %d 张卡片", label, maxHomeLinks))
	}
	seen := make(map[string]struct{}, len(links))
	for i := range links {
		item := fmt.Sprintf("%s第 %d 项", label, i+1)
		if err := validateLink(item, &links[i], maxLinkTitleLen); err != nil {
			return err
		}
		if _, ok := seen[links[i].To]; ok {
			return httpx.ErrValidation(fmt.Sprintf("%s存在重复链接：%s", label, links[i].To))
		}
		seen[links[i].To] = struct{}{}
	}
	return nil
}

func validateLink(label string, l *Link, maxTitle int) error {
	if err := requireText(label+"标题", l.Title, 1, maxTitle); err != nil {
		return err
	}
	if err := requireMax(label+"描述", l.Description, maxLinkDescriptionLen, false); err != nil {
		return err
	}
	if l.To == "" || !strings.HasPrefix(l.To, "/") || strings.HasPrefix(l.To, "//") {
		return httpx.ErrValidation(label + "链接必须是站内路径（以 / 开头）")
	}
	if hasControl(l.To, false) {
		return httpx.ErrValidation(label + "链接包含非法字符")
	}
	if !strings.HasPrefix(l.Icon, "pi pi-") || len(l.Icon) == len("pi pi-") {
		return httpx.ErrValidation(label + "图标必须使用 PrimeIcons（pi pi- 前缀）")
	}
	if hasControl(l.Icon, false) {
		return httpx.ErrValidation(label + "图标包含非法字符")
	}
	switch l.IconPos {
	case "", "left", "right":
	default:
		return httpx.ErrValidation(label + "图标位置只能是 left 或 right")
	}
	return nil
}

func requireText(label, value string, minLen, maxLen int) error {
	n := utf8.RuneCountInString(value)
	if n < minLen {
		return httpx.ErrValidation(label + "不能为空")
	}
	return requireMax(label, value, maxLen, false)
}

func requireMax(label, value string, maxLen int, allowNewlines bool) error {
	if hasControl(value, allowNewlines) {
		return httpx.ErrValidation(label + "包含非法字符")
	}
	if utf8.RuneCountInString(value) > maxLen {
		return httpx.ErrValidation(fmt.Sprintf("%s不能超过 %d 个字符", label, maxLen))
	}
	return nil
}

// hasControl reports whether s contains control characters. Newlines, carriage
// returns and tabs are allowed only when allowNewlines is true, so multi-line
// footer text keeps its line breaks.
func hasControl(s string, allowNewlines bool) bool {
	for _, r := range s {
		if allowNewlines && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
