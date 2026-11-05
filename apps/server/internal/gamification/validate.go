package gamification

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

var achievementCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

const maxAchievementRewardCoins = 1_000_000

// normalizeAchievementInput 校验并规范化管理员输入。update 时 code 允许为空，
// 表示保持原值；创建时必须提供合法 code。
func normalizeAchievementInput(in AchievementInput, codeRequired bool) (AchievementInput, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.IconURL = strings.TrimSpace(in.IconURL)

	if in.Code == "" {
		if codeRequired {
			return in, httpx.ErrValidation("成就 code 不能为空")
		}
	} else if !achievementCodePattern.MatchString(in.Code) {
		return in, httpx.ErrValidation("成就 code 格式不正确")
	}

	if err := validateText("成就名称", in.Name, 1, 100); err != nil {
		return in, err
	}
	if err := validateText("成就描述", in.Description, 0, 500); err != nil {
		return in, err
	}
	if err := validateText("成就图标地址", in.IconURL, 0, 512); err != nil {
		return in, err
	}
	if in.RewardCoins < 0 {
		return in, httpx.ErrValidation("奖励金币不能为负数")
	}
	if in.RewardCoins > maxAchievementRewardCoins {
		return in, httpx.ErrValidation("奖励金币不能超过 " + strconv.Itoa(maxAchievementRewardCoins))
	}
	if err := ValidateCondition(in.ConditionType, in.ConditionConfig); err != nil {
		return in, err
	}

	if in.ConditionType == ConditionAllOf {
		in.ConditionConfig.Threshold = 0
	} else {
		in.ConditionConfig.Conditions = nil
	}
	return in, nil
}

func validateText(field, value string, minLen, maxLen int) error {
	if !utf8.ValidString(value) {
		return httpx.ErrValidation(field + "包含无效字符")
	}
	length := utf8.RuneCountInString(value)
	if length < minLen {
		return httpx.ErrValidation(field + "不能为空")
	}
	if length > maxLen {
		return httpx.ErrValidation(field + "不能超过 " + strconv.Itoa(maxLen) + " 个字符")
	}
	return nil
}
