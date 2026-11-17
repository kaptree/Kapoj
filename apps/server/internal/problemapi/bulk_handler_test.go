package problemapi_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func readBulkPackage(t *testing.T, data []byte) (problem.ProblemsManifest, map[string]string) {
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
	require.True(t, ok, "batch package must contain a root manifest.json")
	var manifest problem.ProblemsManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
	return manifest, files
}

func TestBulkExportHandler(t *testing.T) {
	f := newFx(t)
	first := f.create(body("bulk-one"))
	f.addCase(first)
	second := f.create(body("bulk-two"))
	f.addCase(second)

	endpoint := "/api/problems/bulk/export"
	w, resp := f.do(http.MethodPost, endpoint, f.token("user"), map[string]any{
		"problemIds": []int64{first, second},
	})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 40300, resp["code"])

	f.createRole("bulk_manager", rbac.PermProblemManageAll)
	manager := f.userWithRole("bulk_manager")
	w, _ = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"problemIds": []int64{first, second},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "problems-export-")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".zip")
	assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))

	manifest, files := readBulkPackage(t, w.Body.Bytes())
	assert.Equal(t, problem.PackageFormatProblems, manifest.Format)
	require.Len(t, manifest.Problems, 2)
	assert.Contains(t, files, "problems/bulk-one/manifest.json")
	assert.Contains(t, files, "problems/bulk-two/manifest.json")
	assert.Contains(t, files, "problems/bulk-one/statement.md")

	var audits int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_type = ?", rbac.AuditActionProblemExport, "problem").
		Count(&audits).Error)
	assert.EqualValues(t, 1, audits)

	// One missing problem rejects the whole batch before any ZIP bytes are sent.
	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"problemIds": []int64{first, 999999},
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40400, resp["code"])
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	ids := make([]int64, problem.MaxPackageProblems+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{"problemIds": ids})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

func TestBulkActionsHandler(t *testing.T) {
	f := newFx(t)
	first := f.create(body("action-one"))
	f.addCase(first)
	second := f.create(body("action-two"))
	f.addCase(second)
	endpoint := "/api/problems/bulk/actions"

	w, resp := f.do(http.MethodPost, endpoint, f.token("user"), map[string]any{
		"action": "publish", "problemIds": []int64{first, second},
	})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 40300, resp["code"])

	f.createRole("action_manager", rbac.PermProblemManageAll)
	manager := f.userWithRole("action_manager")

	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"action": "publish", "problemIds": []int64{first, second, 999999},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	results := resp["data"].(map[string]any)["results"].([]any)
	require.Len(t, results, 3)
	assert.Equal(t, "success", results[0].(map[string]any)["status"])
	assert.Equal(t, "success", results[1].(map[string]any)["status"])
	assert.Equal(t, "failed", results[2].(map[string]any)["status"])

	for _, id := range []int64{first, second} {
		var p problem.Problem
		require.NoError(t, f.db.First(&p, id).Error)
		assert.True(t, p.IsPublic)
	}

	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"action": "hide", "problemIds": []int64{first, second},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	results = resp["data"].(map[string]any)["results"].([]any)
	for _, result := range results {
		assert.Equal(t, "success", result.(map[string]any)["status"])
	}

	require.NoError(t, f.db.Model(&problem.Problem{}).Where("id = ?", first).
		Update("submit_count", 2).Error)
	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"action": "delete", "problemIds": []int64{first, second},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	results = resp["data"].(map[string]any)["results"].([]any)
	require.Len(t, results, 2)
	assert.Equal(t, true, results[0].(map[string]any)["offlined"])
	assert.Equal(t, false, results[1].(map[string]any)["offlined"])

	var firstCount, secondCount int64
	require.NoError(t, f.db.Model(&problem.Problem{}).Where("id = ?", first).Count(&firstCount).Error)
	require.NoError(t, f.db.Model(&problem.Problem{}).Where("id = ?", second).Count(&secondCount).Error)
	assert.EqualValues(t, 1, firstCount, "a problem with submissions is only taken offline")
	assert.Zero(t, secondCount, "a problem without submissions is deleted")

	var audits int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_type = ?", rbac.AuditActionProblemBulk, "problem").
		Count(&audits).Error)
	assert.EqualValues(t, 3, audits)

	w, resp = f.do(http.MethodPost, endpoint, manager, map[string]any{
		"action": "archive", "problemIds": []int64{first},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
}
