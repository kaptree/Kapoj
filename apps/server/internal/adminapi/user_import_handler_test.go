package adminapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/password"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func importCSV(hash, salt string) []byte {
	return []byte(fmt.Sprintf(
		"_id,uname,mail,hash,salt,hashType\n2,Alice,a@example.com,%s,%s,hydro\n",
		hash, salt,
	))
}

func summaryOf(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	summary, ok := dataMap(t, resp)["summary"].(map[string]any)
	require.True(t, ok, "summary should be an object: %v", resp)
	return summary
}

func TestAdminUserImportRequiresPermission(t *testing.T) {
	f := newFixture(t)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := importCSV(hash, salt)

	_, plainToken := f.user(t, "plain_import")
	w, resp := f.multipart("/api/admin/users/import?dryRun=true", plainToken, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusForbidden)
	assert.EqualValues(t, 40300, resp["code"])

	_, superToken := f.user(t, "super_import", rbac.RoleCodeSuperAdmin)
	w, resp = f.multipart("/api/admin/users/import?dryRun=true", superToken, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusOK)

	// The preview includes emails, so the endpoint requires user.read as well
	// as user.import.
	importOnly, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "import_only_role", Name: "仅导入用户", PermissionCodes: []string{string(rbac.PermUserImport)},
	})
	require.NoError(t, err)
	_, importOnlyToken := f.user(t, "import_only", importOnly.Code)
	w, resp = f.multipart("/api/admin/users/import?dryRun=true", importOnlyToken, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusForbidden)

	importReader, err := f.svc.CreateRole(context.Background(), rbac.AuditMeta{ActorID: 1}, rbac.RoleInput{
		Code: "import_reader_role", Name: "查看并导入用户",
		PermissionCodes: []string{string(rbac.PermUserRead), string(rbac.PermUserImport)},
	})
	require.NoError(t, err)
	_, importReaderToken := f.user(t, "import_reader", importReader.Code)
	w, resp = f.multipart("/api/admin/users/import?dryRun=true", importReaderToken, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusOK)
}

func TestAdminUserImportPreviewAndCommit(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_import_commit", rbac.RoleCodeSuperAdmin)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := importCSV(hash, salt)

	w, resp := f.multipart("/api/admin/users/import?dryRun=true", token, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusOK)
	data := dataMap(t, resp)
	assert.Equal(t, true, data["dryRun"])
	preview := summaryOf(t, resp)
	assert.EqualValues(t, 1, preview["valid"])
	assert.EqualValues(t, 0, preview["imported"])
	var count int64
	require.NoError(t, f.db.Model(&user.User{}).Where("username = ?", "alice").Count(&count).Error)
	assert.Zero(t, count, "preview must not write")
	assert.NotContains(t, w.Body.String(), hash)
	assert.NotContains(t, w.Body.String(), salt)

	w, resp = f.multipart("/api/admin/users/import", token, "users.csv", csv)
	requireStatus(t, w, resp, http.StatusOK)
	result := summaryOf(t, resp)
	assert.EqualValues(t, 1, result["imported"])
	assert.EqualValues(t, 0, result["skipped"])
	assert.NotContains(t, w.Body.String(), hash)
	assert.NotContains(t, w.Body.String(), salt)

	var imported user.User
	require.NoError(t, f.db.Where("username = ?", "alice").First(&imported).Error)
	assert.NotNil(t, imported.EmailVerifiedAt)
	assert.True(t, password.Verify("old-password", imported.Salt, imported.PasswordHash))

	var audit rbac.AdminAuditLog
	require.NoError(t, f.db.Where("action = ?", rbac.AuditActionUserImport).First(&audit).Error)
	assert.Contains(t, audit.Summary, "成功 1")
	assert.NotContains(t, audit.Summary, "a@example.com")
	assert.NotContains(t, audit.Summary, hash)
}

func TestAdminUserImportRejectsBadCSV(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_import_bad", rbac.RoleCodeSuperAdmin)

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"missing header", []byte("_id,uname\n2,Alice\n")},
		{"non utf8", []byte{0xff, 0xfe, 0xfd}},
		{"empty", nil},
		{"too large", bytes.Repeat([]byte("a"), (5<<20)+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, resp := f.multipart("/api/admin/users/import?dryRun=true", token, "users.csv", tc.data)
			requireStatus(t, w, resp, http.StatusBadRequest)
			assert.EqualValues(t, 40001, resp["code"])
		})
	}
}

func TestAdminUserImportRouteNotShadowedByUserID(t *testing.T) {
	f := newFixture(t)
	_, token := f.user(t, "super_import_route", rbac.RoleCodeSuperAdmin)

	w, resp := f.multipart("/api/admin/users/import?dryRun=true", token, "users.csv", []byte("bad"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestPermissionCatalogIncludesUserImport(t *testing.T) {
	found := false
	for _, permission := range rbac.AllPermissions() {
		if permission.Code == rbac.PermUserImport {
			found = true
			assert.Equal(t, "用户与角色", permission.GroupName)
			assert.Equal(t, "批量导入用户", permission.Name)
		}
	}
	assert.True(t, found, "permission catalog must include user.import")
}
