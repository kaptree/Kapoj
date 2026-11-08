package problemapi_test

import (
	"net/http"
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
