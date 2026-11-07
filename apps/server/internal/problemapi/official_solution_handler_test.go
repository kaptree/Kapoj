package problemapi_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setSolution uses the official solution endpoint to write content owned by the
// problem creator.
func (f *fx) setSolution(id int64, tok string, md, code string, price int64) {
	f.t.Helper()
	w, resp := f.do(http.MethodPut, path("/api/problems/%d/official-solution", id), tok, map[string]any{
		"solutionMd": md, "solutionCode": code, "priceCoins": price,
	})
	require.Equal(f.t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(f.t, "creator", resp["data"].(map[string]any)["access"])
}

func (f *fx) userWithCoins(coins int64) (string, int64) {
	f.t.Helper()
	tok, id := f.registerUser("user")
	require.NoError(f.t, f.db.Exec("UPDATE users SET coins = ? WHERE id = ?", coins, id).Error)
	return tok, id
}

func TestOfficialSolutionEndpointsAuthAndPurchase(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	id := f.publish("official", nil)

	os := path("/api/problems/%d/official-solution", id)

	// GET 公开：游客可访问，返回 locked（价格默认 0 时其实为 free）。
	w, resp := f.do(http.MethodGet, os, "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "free", resp["data"].(map[string]any)["access"])

	// 设置价格与内容（创建者是 content_admin，可直接编辑）。
	f.setSolution(id, admin, "OFFICIAL_MD", "OFFICIAL_CODE", 100)

	// 游客 -> guest，无正文。
	w, resp = f.do(http.MethodGet, os, "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "guest", data["access"])
	assert.NotContains(t, data, "solutionMd")
	assert.NotContains(t, data, "solutionCode")

	// 普通用户 -> locked，可购买。
	userTok, userID := f.userWithCoins(0)
	w, resp = f.do(http.MethodGet, os, userTok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data = resp["data"].(map[string]any)
	assert.Equal(t, "locked", data["access"])
	assert.Equal(t, true, data["canPurchase"])
	assert.NotContains(t, data, "solutionMd")

	// PUT 未登录 -> 401。
	w, _ = f.do(http.MethodPut, os, "", map[string]any{"solutionMd": "x", "priceCoins": 0})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	// POST purchase 未登录 -> 401。
	w, _ = f.do(http.MethodPost, os+"/purchase", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 普通用户 PUT -> 403。
	w, _ = f.do(http.MethodPut, os, userTok, map[string]any{"solutionMd": "x", "priceCoins": 0})
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 金币不足购买 -> 40001，且不写解锁。
	w, resp = f.do(http.MethodPost, os+"/purchase", userTok, nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.EqualValues(t, 40001, resp["code"])

	// 充值后购买成功。
	require.NoError(t, f.db.Exec("UPDATE users SET coins = 100 WHERE id = ?", userID).Error)
	w, resp = f.do(http.MethodPost, os+"/purchase", userTok, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = resp["data"].(map[string]any)
	assert.Equal(t, "purchased", data["access"])
	assert.Equal(t, "OFFICIAL_MD", data["solutionMd"])

	// 余额扣减。
	var coins int64
	require.NoError(t, f.db.Raw(`SELECT coins FROM users WHERE id = ?`, userID).Scan(&coins).Error)
	assert.EqualValues(t, 0, coins)
}

func TestProblemUpdateDoesNotWriteSolution(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")
	id := f.publish("no-sol-write", nil)

	// 通过官方题解接口写入内容。
	f.setSolution(id, admin, "KEEP_MD", "KEEP_CODE", 77)

	// 主更新接口带 solutionMd/solutionCode（忽略），题解内容必须保留。
	b := body("no-sol-write")
	b["isPublic"] = true
	b["solutionMd"] = "SHOULD_BE_IGNORED"
	b["solutionCode"] = "SHOULD_BE_IGNORED"
	w, _ := f.do(http.MethodPut, path("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w, resp := f.do(http.MethodGet, path("/api/problems/%d/official-solution", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "KEEP_MD", data["solutionMd"])
	assert.Equal(t, "KEEP_CODE", data["solutionCode"])
	assert.EqualValues(t, 77, data["priceCoins"])
}

func TestProblemInputNoSolutionFields(t *testing.T) {
	f := newFx(t)
	admin := f.token("problem_admin")

	// 创建时带上 solutionMd/solutionCode，字段应被忽略（不落库）。
	b := body("no-sol-input")
	b["solutionMd"] = "IGNORED_MD"
	b["solutionCode"] = "IGNORED_CODE"
	w, resp := f.do(http.MethodPost, "/api/problems", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := idOf(resp)

	// 详情作为管理员返回的题解字段为空（未被写入）。
	w, resp = f.do(http.MethodGet, path("/api/problems/%d", id), admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "", data["solutionMd"])
	assert.Equal(t, "", data["solutionCode"])
	assert.EqualValues(t, 0, data["solutionPriceCoins"])
}
