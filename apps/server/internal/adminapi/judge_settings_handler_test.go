package adminapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func TestJudgeSettingsAPI(t *testing.T) {
	f := newFixture(t)
	path := "/api/admin/judge/settings"

	// 未登录 401。
	w, resp := f.do(http.MethodGet, path, "", "")
	requireStatus(t, w, resp, http.StatusUnauthorized)
	assert.EqualValues(t, 40100, resp["code"])

	// 无权限 403。
	_, plain := f.user(t, "plain_judge_settings")
	w, resp = f.do(http.MethodGet, path, plain, "")
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	w, resp = f.do(http.MethodPut, path, plain, `{}`)
	requireStatus(t, w, resp, http.StatusForbidden)

	// 仅有 system.settings.manage 的自定义角色可以访问。
	settingsRole, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "judge_settings_manager", Name: "评测设置管理员",
		PermissionCodes: []string{string(rbac.PermSystemSettingsManage)},
	})
	require.NoError(t, err)
	_, manager := f.user(t, "judge_settings_manager_user", settingsRole.Code)

	// 超级管理员读取默认值；GET 不写审计。
	actorID, super := f.user(t, "super_judge_settings", rbac.RoleCodeSuperAdmin)
	for _, token := range []string{manager, super} {
		w, resp = f.do(http.MethodGet, path, token, "")
		requireStatus(t, w, resp, http.StatusOK)
		data := dataMap(t, resp)
		assert.EqualValues(t, 1<<20, data["outputFloorBytes"])
		assert.EqualValues(t, 1<<20, data["outputSlackBytes"])
		assert.EqualValues(t, 16<<20, data["maxOutputBytes"])
		assert.EqualValues(t, 16<<20, data["maxInputFileBytes"])
		assert.EqualValues(t, 16<<20, data["maxOutputFileBytes"])
		assert.EqualValues(t, 200<<20, data["maxBatchTotalBytes"])
		assert.EqualValues(t, 200<<20, data["maxProblemTotalBytes"])
		assert.NotEmpty(t, data["updatedAt"])
	}

	var settingsAuditCount int64
	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).Where(
		"action = ? AND target_type = ?",
		string(rbac.PermSystemSettingsManage), rbac.AuditTargetJudgeSettings,
	).Count(&settingsAuditCount).Error)
	assert.Zero(t, settingsAuditCount, "GET 不应写评测设置审计")

	// 更新全部字段并写审计。
	w, resp = f.do(http.MethodPut, path, super, `{
		"outputFloorBytes": 2097152,
		"outputSlackBytes": 524288,
		"maxOutputBytes": 8388608,
		"maxInputFileBytes": 33554432,
		"maxOutputFileBytes": 8388608,
		"maxBatchTotalBytes": 314572800,
		"maxProblemTotalBytes": 419430400
	}`)
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 2<<20, data["outputFloorBytes"])
	assert.EqualValues(t, 512<<10, data["outputSlackBytes"])
	assert.EqualValues(t, 8<<20, data["maxOutputBytes"])
	assert.EqualValues(t, 32<<20, data["maxInputFileBytes"])
	assert.EqualValues(t, 8<<20, data["maxOutputFileBytes"])
	assert.EqualValues(t, 300<<20, data["maxBatchTotalBytes"])
	assert.EqualValues(t, 400<<20, data["maxProblemTotalBytes"])

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where(
		"action = ? AND target_type = ?",
		string(rbac.PermSystemSettingsManage), rbac.AuditTargetJudgeSettings,
	).First(&audit).Error)
	assert.EqualValues(t, actorID, audit.ActorID)
	assert.Contains(t, audit.Summary, "最大输出")
	assert.NotContains(t, audit.Summary, "super_judge_settings")

	// 再次 GET 读到更新后的值。
	w, resp = f.do(http.MethodGet, path, super, "")
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 8<<20, data["maxOutputBytes"])
	assert.EqualValues(t, 400<<20, data["maxProblemTotalBytes"])

	// 关系校验失败 -> 40001，且不写审计。
	w, resp = f.do(http.MethodPut, path, super, `{
		"outputFloorBytes": 16777216,
		"outputSlackBytes": 1048576,
		"maxOutputBytes": 1048576,
		"maxInputFileBytes": 16777216,
		"maxOutputFileBytes": 1048576,
		"maxBatchTotalBytes": 209715200,
		"maxProblemTotalBytes": 209715200
	}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	require.NoError(t, f.db.Model(&rbac.AdminAuditLog{}).Where(
		"action = ? AND target_type = ?",
		string(rbac.PermSystemSettingsManage), rbac.AuditTargetJudgeSettings,
	).Count(&settingsAuditCount).Error)
	assert.EqualValues(t, 1, settingsAuditCount, "失败的更新不应写审计")
}
