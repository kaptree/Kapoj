package adminapi_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func TestAdminProblemImportSettings(t *testing.T) {
	f := newFixture(t)
	path := "/api/admin/problems/import/settings"

	_, plain := f.user(t, "plain_problem_import_settings")
	w, resp := f.do(http.MethodGet, path, plain, "")
	requireStatus(t, w, resp, http.StatusForbidden)

	_, super := f.user(t, "super_problem_import_settings", rbac.RoleCodeSuperAdmin)
	w, resp = f.do(http.MethodGet, path, super, "")
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.EqualValues(t, 200, data["maxPackageMb"])
	assert.EqualValues(t, 200<<20, data["maxZipBytes"])
	assert.EqualValues(t, 1<<30, data["maxUncompressedBytes"])

	w, resp = f.do(http.MethodPut, path, super, `{"maxPackageMb":2048}`)
	requireStatus(t, w, resp, http.StatusOK)
	data = dataMap(t, resp)
	assert.EqualValues(t, 2048, data["maxPackageMb"])
	assert.EqualValues(t, 2048<<20, data["maxZipBytes"])
	assert.EqualValues(t, 4<<30, data["maxUncompressedBytes"])

	w, resp = f.do(http.MethodPut, path, super, `{"maxPackageMb":0}`)
	requireStatus(t, w, resp, http.StatusBadRequest)
	assert.EqualValues(t, 40001, resp["code"])

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionProblemImportSettings).First(&audit).Error)
	assert.Contains(t, audit.Summary, "2048 MB")
	assert.NotContains(t, audit.Summary, "username")
}
