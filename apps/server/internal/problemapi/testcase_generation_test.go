package problemapi_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
)

func TestCreateTestcaseGeneration(t *testing.T) {
	f := newFx(t)
	id := f.create(body("gen"))
	admin := f.token("problem_admin")
	url := path("/api/problems/%d/testcase-generations", id)

	w, resp := f.do(http.MethodPost, url, admin, map[string]any{
		"language": "cpp", "sourceCode": "int main(){}", "testcaseCount": 5,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "pending", data["status"])
	assert.EqualValues(t, 5, data["testcaseCount"])
	assert.Equal(t, "print(1)", data["generatorCode"])

	// 无权限 403、未登录 401。
	w, _ = f.do(http.MethodPost, url, f.token("user"), map[string]any{"language": "cpp", "sourceCode": "x", "testcaseCount": 1})
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodPost, url, "", map[string]any{"language": "cpp", "sourceCode": "x", "testcaseCount": 1})
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 非传统题 400。
	special := body("gen-spj")
	special["type"] = "special"
	sid := f.create(special)
	w, resp = f.do(http.MethodPost, path("/api/problems/%d/testcase-generations", sid), admin,
		map[string]any{"language": "cpp", "sourceCode": "x", "testcaseCount": 1})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 非法 count 400。
	w, resp = f.do(http.MethodPost, url, admin,
		map[string]any{"language": "cpp", "sourceCode": "x", "testcaseCount": 11})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestGetTestcaseGeneration(t *testing.T) {
	f := newFx(t)
	id := f.create(body("gen-get"))
	admin := f.token("problem_admin")
	base := path("/api/problems/%d/testcase-generations", id)

	w, resp := f.do(http.MethodPost, base, admin, map[string]any{
		"language": "python3", "sourceCode": "print(1)", "testcaseCount": 3,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	genID := idOf(resp)

	require.NoError(t, f.db.Create(&testcasegen.GenerationCase{
		GenerationID: genID, SortOrder: 1, InputFile: "p/1/g/1.in", OutputFile: "p/1/g/1.out", InputSize: 12, OutputSize: 6,
	}).Error)

	w, resp = f.do(http.MethodGet, path("%s/%d", base, genID), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "pending", data["status"])
	cases := data["cases"].([]any)
	require.Len(t, cases, 1)

	w, resp = f.do(http.MethodGet, path("%s/999999", base), admin, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.EqualValues(t, 40400, resp["code"])
}

// seedGeneration inserts a generation (and cases) directly, bypassing the AI
// create flow so tests can exercise preview/confirm/discard on terminal states.
func (f *fx) seedGeneration(problemID int64, status testcasegen.Status, count int) (genID int64, caseIDs []int64) {
	f.t.Helper()
	gen := &testcasegen.Generation{
		ProblemID: problemID, UserID: 1, Status: status, Language: "cpp",
		StandardCode: "int main(){}", GeneratorCode: "print(1)", TestcaseCount: count,
		TimeLimitMS: 1000, MemoryLimitMB: 256,
	}
	require.NoError(f.t, f.db.Create(gen).Error)
	caseIDs = make([]int64, count)
	for i := 0; i < count; i++ {
		c := testcasegen.GenerationCase{
			GenerationID: gen.ID, SortOrder: i + 1,
			InputFile:  f.genKey(problemID, gen.ID, i+1, "in"),
			OutputFile: f.genKey(problemID, gen.ID, i+1, "out"),
			InputSize:  10, OutputSize: 4,
		}
		require.NoError(f.t, f.db.Create(&c).Error)
		caseIDs[i] = c.ID
	}
	return gen.ID, caseIDs
}

func (f *fx) genKey(problemID, genID int64, n int, ext string) string {
	return fmt.Sprintf("problems/%d/generated/%d/%d.%s", problemID, genID, n, ext)
}

func (f *fx) writeGenFile(problemID, genID int64, n int, ext, content string) {
	f.t.Helper()
	key := f.genKey(problemID, genID, n, ext)
	full := filepath.Join(f.root, filepath.FromSlash(key))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(f.t, os.WriteFile(full, []byte(content), 0o644))
}

func TestTestcaseGenerationPreview(t *testing.T) {
	f := newFx(t)
	id := f.create(body("gen-preview"))
	admin := f.token("problem_admin")
	base := path("/api/problems/%d/testcase-generations", id)

	genID, caseIDs := f.seedGeneration(id, testcasegen.StatusReady, 1)
	f.writeGenFile(id, genID, 1, "in", "1 2")

	previewURL := path("%s/%d/cases/%d/preview", base, genID, caseIDs[0])
	w, resp := f.do(http.MethodGet, previewURL+"?kind=input", admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	assert.Equal(t, "input", data["kind"])
	assert.Equal(t, "1 2", data["content"])
	assert.Equal(t, false, data["truncated"])

	// 超 64 KiB → truncated=true。
	big := make([]byte, 70*1024)
	for i := range big {
		big[i] = 'a'
	}
	f.writeGenFile(id, genID, 1, "in", string(big))
	w, resp = f.do(http.MethodGet, previewURL+"?kind=input", admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, true, data["truncated"])

	// 非法 kind → 400。
	w, resp = f.do(http.MethodGet, previewURL+"?kind=stderr", admin, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestTestcaseGenerationConfirm(t *testing.T) {
	f := newFx(t)
	id := f.create(body("gen-confirm"))
	admin := f.token("problem_admin")
	base := path("/api/problems/%d/testcase-generations", id)

	genID, _ := f.seedGeneration(id, testcasegen.StatusReady, 2)
	confirmURL := path("%s/%d/confirm", base, genID)

	w, resp := f.do(http.MethodPost, confirmURL, admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["count"])

	var n int64
	require.NoError(t, f.db.Table("problem_testcases").Where("problem_id = ?", id).Count(&n).Error)
	assert.Equal(t, int64(2), n)

	// 重复确认：count 不变、不新增测试点。
	w, resp = f.do(http.MethodPost, confirmURL, admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["count"])
	require.NoError(t, f.db.Table("problem_testcases").Where("problem_id = ?", id).Count(&n).Error)
	assert.Equal(t, int64(2), n)

	// 非 ready → 400。
	gen2, _ := f.seedGeneration(id, testcasegen.StatusPending, 2)
	w, resp = f.do(http.MethodPost, path("%s/%d/confirm", base, gen2), admin, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
}

func TestTestcaseGenerationDiscard(t *testing.T) {
	f := newFx(t)
	id := f.create(body("gen-discard"))
	admin := f.token("problem_admin")
	base := path("/api/problems/%d/testcase-generations", id)

	genID, _ := f.seedGeneration(id, testcasegen.StatusReady, 2)
	f.writeGenFile(id, genID, 1, "in", "1")
	f.writeGenFile(id, genID, 1, "out", "1")
	f.writeGenFile(id, genID, 2, "in", "2")
	f.writeGenFile(id, genID, 2, "out", "2")

	w, _ := f.do(http.MethodDelete, path("%s/%d", base, genID), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var gen testcasegen.Generation
	require.NoError(t, f.db.First(&gen, genID).Error)
	assert.Equal(t, testcasegen.StatusDiscarded, gen.Status)
	assert.Empty(t, f.storedFiles())

	// confirmed → 400 且文件保留。
	gen2, _ := f.seedGeneration(id, testcasegen.StatusConfirmed, 1)
	f.writeGenFile(id, gen2, 1, "in", "x")
	f.writeGenFile(id, gen2, 1, "out", "y")
	w, resp := f.do(http.MethodDelete, path("%s/%d", base, gen2), admin, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])
	assert.Len(t, f.storedFiles(), 2)

	// 无权限 → 403。
	w, _ = f.do(http.MethodDelete, path("%s/%d", base, genID), f.token("user"), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestTestcaseGenerationAudit(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	id := f.create(body("gen-audit"))
	base := path("/api/problems/%d/testcase-generations", id)

	auditCount := func() int64 {
		var n int64
		require.NoError(t, f.db.Table("admin_audit_logs").Where("action = ?", "problem.testcase.manage").Count(&n).Error)
		return n
	}

	// create → 1 条审计。
	w, _ := f.do(http.MethodPost, base, admin, map[string]any{
		"language": "cpp", "sourceCode": "int main(){}", "testcaseCount": 1,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(1), auditCount())

	// confirm（直接 seed ready）→ 第 2 条。
	genID, _ := f.seedGeneration(id, testcasegen.StatusReady, 1)
	w, _ = f.do(http.MethodPost, path("%s/%d/confirm", base, genID), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(2), auditCount())

	// discard（直接 seed ready）→ 第 3 条。
	gen2, _ := f.seedGeneration(id, testcasegen.StatusReady, 1)
	w, _ = f.do(http.MethodDelete, path("%s/%d", base, gen2), admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(3), auditCount())
}
