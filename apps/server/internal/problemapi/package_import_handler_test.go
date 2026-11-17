package problemapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
)

func kapOJPackageZip(t *testing.T, slug, title string, testcases bool) []byte {
	t.Helper()
	manifest := problem.PackageManifest{
		Format:        problem.PackageFormatProblem,
		FormatVersion: problem.PackageFormatVersion,
		Problem: problem.PackageProblem{
			Slug: slug, Title: title, Type: problem.TypeTraditional,
			Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
			Tags: []string{}, IsPublic: false,
		},
		Files: problem.PackageFileManifest{
			Statement: "statement.md", InputFormat: "input_format.md",
			OutputFormat: "output_format.md", Hint: "hint.md",
			Samples: "samples.json", Testdata: "testdata/meta.json",
		},
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	entries := []struct{ name, body string }{
		{"manifest.json", string(raw)},
		{"statement.md", "题面 " + title},
		{"input_format.md", ""},
		{"output_format.md", ""},
		{"hint.md", ""},
		{"samples.json", "[]"},
	}
	if testcases {
		entries = append(entries,
			struct{ name, body string }{"testdata/meta.json", `{"testcases":[{"sortOrder":1,"score":100,"isSample":false,"input":"testdata/001.in","output":"testdata/001.out"}]}`},
			struct{ name, body string }{"testdata/001.in", "1"},
			struct{ name, body string }{"testdata/001.out", "2"},
		)
	} else {
		entries = append(entries, struct{ name, body string }{"testdata/meta.json", `{"testcases":[]}`})
	}
	return hydroZipBytes(t, entries...)
}

func TestImportPackageHandlerKapOJ(t *testing.T) {
	f := newFx(t)
	endpoint := "/api/problems/import/package"
	data := kapOJPackageZip(t, "handler-import", "导入题", true)

	w, _ := f.multipart(endpoint, "", map[string]string{"publish": "true"},
		filePart{"file", "p.zip", bytes.NewReader(data)})
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())

	w, _ = f.multipart(endpoint, f.token("user"), map[string]string{"publish": "true"},
		filePart{"file", "p.zip", bytes.NewReader(data)})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	w, resp := f.multipart(endpoint, f.token("problem_admin"), map[string]string{
		"conflictPolicy": "skip", "publish": "true",
	}, filePart{"file", "p.zip", bytes.NewReader(data)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	imported := resp["data"].(map[string]any)["imported"].([]any)
	require.Len(t, imported, 1)
	item := imported[0].(map[string]any)
	assert.Equal(t, true, item["published"])
	id := int64(item["id"].(float64))
	var published problem.Problem
	require.NoError(t, f.db.First(&published, id).Error)
	assert.True(t, published.IsPublic)

	var importAudits int64
	require.NoError(t, f.db.Table("admin_audit_logs").
		Where("action = ? AND target_type = ? AND target_id IS NULL",
			rbac.AuditActionProblemImport, "problem").
		Count(&importAudits).Error)
	assert.EqualValues(t, 1, importAudits, "each import request must record a summary audit")

	// A user with problem.create but not problem.publish cannot request
	// publish=true, and nothing is imported.
	f.createRole("import_creator", rbac.PermProblemCreate)
	creator := f.userWithRole("import_creator")
	denied := kapOJPackageZip(t, "creator-import", "无发布权限", true)
	w, resp = f.multipart(endpoint, creator, map[string]string{"publish": "true"},
		filePart{"file", "p.zip", bytes.NewReader(denied)})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 40300, resp["code"])
	var count int64
	require.NoError(t, f.db.Model(&problem.Problem{}).Where("slug = ?", "creator-import").Count(&count).Error)
	assert.Zero(t, count)

	w, _ = f.multipart(endpoint, creator, map[string]string{"conflictPolicy": "skip"},
		filePart{"file", "p.zip", bytes.NewReader(denied)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, f.db.Model(&problem.Problem{}).Where("slug = ?", "creator-import").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestImportPackageHandlerDetectsHydroAndPublishes(t *testing.T) {
	f := newFx(t)
	data := validHydroZip(t)
	w, resp := f.multipart("/api/problems/import/package", f.token("problem_admin"),
		map[string]string{"publish": "true"},
		filePart{"file", "h.zip", bytes.NewReader(data)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	imported := resp["data"].(map[string]any)["imported"].([]any)
	require.Len(t, imported, 1)
	item := imported[0].(map[string]any)
	assert.Equal(t, true, item["published"])
	id := int64(item["id"].(float64))
	var published problem.Problem
	require.NoError(t, f.db.First(&published, id).Error)
	assert.True(t, published.IsPublic)

	w, resp = f.multipart("/api/problems/import/package", f.token("problem_admin"), nil,
		filePart{"file", "bad.zip", bytes.NewReader([]byte("not a zip"))})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
}

func TestImportHydroHandlerPublishOption(t *testing.T) {
	f := newFx(t)
	data := hydroZipBytes(t,
		struct{ name, body string }{"7761/problem.yaml", "title: Hydro 发布题\n"},
		struct{ name, body string }{"7761/problem_zh.md", "题面"},
		struct{ name, body string }{"7761/testdata/1.in", "1"},
		struct{ name, body string }{"7761/testdata/1.out", "2"},
	)
	w, resp := f.multipart("/api/problems/import/hydro", f.token("problem_admin"),
		map[string]string{"publish": "true"},
		filePart{"file", "h.zip", bytes.NewReader(data)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	imported := resp["data"].(map[string]any)["imported"].([]any)
	require.Len(t, imported, 1)
	item := imported[0].(map[string]any)
	assert.Equal(t, true, item["published"])
	assert.Equal(t, "hydro-7761", item["slug"])
}
