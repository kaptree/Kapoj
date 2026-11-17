package problemapi_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func readProblemPackage(t *testing.T, data []byte) (problem.PackageManifest, map[string]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files[f.Name] = string(body)
	}
	raw, ok := files["manifest.json"]
	require.True(t, ok, "package must contain manifest.json")
	var manifest problem.PackageManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
	return manifest, files
}

func TestProblemPackageHandlerPermissions(t *testing.T) {
	f := newFx(t)
	owner := f.token("problem_admin")
	id := f.create(body("package-owner"))
	f.addCase(id)
	endpoint := fmt.Sprintf("/api/problems/%d/package", id)

	w, _ := f.do(http.MethodGet, endpoint, "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())

	w, _ = f.do(http.MethodGet, endpoint, owner, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "package-owner.zip")
	assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	packageBody := append([]byte(nil), w.Body.Bytes()...)

	w, infoResp := f.do(http.MethodGet, endpoint+"/info", owner, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	info := infoResp["data"].(map[string]any)
	assert.Equal(t, problem.PackageFormatProblem, info["format"])
	assert.GreaterOrEqual(t, int(info["fileCount"].(float64)), 8)
	assert.Greater(t, int64(info["sizeBytes"].(float64)), int64(0))
	assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))

	manifest, files := readProblemPackage(t, packageBody)
	assert.Equal(t, problem.PackageFormatProblem, manifest.Format)
	assert.Equal(t, "package-owner", manifest.Problem.Slug)
	assert.Contains(t, files, "statement.md")
	assert.Contains(t, files, manifest.Files.Testdata)

	var audits int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_type = ? AND target_id = ?",
			rbac.AuditActionProblemExport, "problem", id).
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits, "a successful export must be audited")

	f.createRole("package_other", rbac.PermProblemUpdate)
	other := f.userWithRole("package_other")
	w, _ = f.do(http.MethodGet, endpoint, other, nil)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"a non-owner without problem.manage_all must not download a hidden package")

	f.createRole("package_manager", rbac.PermProblemManageAll)
	manager := f.userWithRole("package_manager")
	w, _ = f.do(http.MethodGet, endpoint, manager, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Body.Bytes())

	w, _ = f.do(http.MethodGet, "/api/problems/999999/package", manager, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProblemPackageHandlerMissingStorageReturnsJSONError(t *testing.T) {
	f := newFx(t)
	owner := f.token("problem_admin")
	id := f.create(body("package-missing-file"))
	f.addCase(id)
	files := f.storedFiles()
	require.NotEmpty(t, files)
	require.NoError(t, os.Remove(files[0]))

	w, resp := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/package", id), owner, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var audits int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_id = ?", rbac.AuditActionProblemExport, id).
		Count(&audits).Error)
	assert.Zero(t, audits, "a failed export must not be audited as a successful download")
}
