package site

import (
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
)

const maxCompanionNameLen = 20

// companionActivityLevels mirrors the quiet | normal | lively enum used by the
// per-user companion settings.
var companionActivityLevels = map[string]struct{}{
	"quiet":  {},
	"normal": {},
	"lively": {},
}

// validateCompanion trims the name and enforces the admin companion rules.
func validateCompanion(in *CompanionSettings) error {
	in.Name = strings.TrimSpace(in.Name)
	in.DefaultActivityLevel = strings.TrimSpace(in.DefaultActivityLevel)
	if err := requireText("伙伴名称", in.Name, 1, maxCompanionNameLen); err != nil {
		return err
	}
	if !validCompanionActivity(in.DefaultActivityLevel) {
		return httpx.ErrValidation("默认活跃度只能是 quiet、normal 或 lively")
	}
	return nil
}

// normalizeCompanion fills defaults for historical rows. A row written before
// companion_json existed scans as an all-zero document, so it returns the
// canonical defaults; partially written documents have blank name or activity
// clamped without disturbing explicit boolean choices.
func normalizeCompanion(in CompanionSettings) CompanionSettings {
	if in == (CompanionSettings{}) {
		return DefaultCompanionSettings()
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = DefaultCompanionSettings().Name
	}
	if !validCompanionActivity(in.DefaultActivityLevel) {
		in.DefaultActivityLevel = DefaultCompanionSettings().DefaultActivityLevel
	}
	return in
}

func validCompanionActivity(level string) bool {
	_, ok := companionActivityLevels[level]
	return ok
}
