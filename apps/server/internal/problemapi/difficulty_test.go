package problemapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/problemapi"
	"github.com/kaptree/kapoj/apps/server/internal/router"
)

// fakeAssessor is the AI difficulty assessment seam used by the API tests.
// configured models PlatformModelConfigured; calls counts assessments.
type fakeAssessor struct {
	score      int
	confidence float64
	reason     string
	err        error
	configured bool
	calls      int
	lastInput  ai.DifficultyAssessmentInput
}

func (f *fakeAssessor) PlatformModelConfigured(context.Context) (bool, error) {
	return f.configured, nil
}

func (f *fakeAssessor) AssessDifficulty(_ context.Context, in ai.DifficultyAssessmentInput) (*ai.DifficultyAssessment, error) {
	f.calls++
	f.lastInput = in
	if f.err != nil {
		return nil, f.err
	}
	return &ai.DifficultyAssessment{Score: f.score, Confidence: f.confidence, Reason: f.reason}, nil
}

// fakeAISettings is the AI feature toggle seam.
type fakeAISettings struct {
	enabled bool
	err     error
	calls   int
}

func (f *fakeAISettings) Enabled(context.Context) (bool, error) {
	f.calls++
	return f.enabled, f.err
}

// newDifficultyFixture builds the normal handler fixture plus the difficulty
// settings service and the fake AI seams. Settings are created lazily by the
// service with the production defaults (default score 50, AI assessments on).
func newDifficultyFixture(t *testing.T, assessor *fakeAssessor, aiSettings *fakeAISettings) *fx {
	t.Helper()
	return newFx(t, func(d *router.Deps) {
		d.Difficulty = difficulty.NewSettingsService(d.DB)
		d.DifficultyAI = assessor
		d.DifficultyAISettings = aiSettings
	})
}

func boolPtr(v bool) *bool { return &v }

func difficultyBody(slug string, score int, auto *bool) map[string]any {
	b := map[string]any{
		"title":         "Problem " + slug,
		"slug":          slug,
		"statementMd":   "statement",
		"type":          "traditional",
		"difficulty":    score,
		"timeLimitMs":   1000,
		"memoryLimitMb": 256,
	}
	if auto != nil {
		b["difficultyAuto"] = *auto
	}
	return b
}

func detailData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok, "response has no data object: %v", resp)
	return data
}

func historyItems(t *testing.T, resp map[string]any) []map[string]any {
	t.Helper()
	data := detailData(t, resp)
	raw, ok := data["items"].([]any)
	require.True(t, ok, "response has no items array: %v", resp)
	items := make([]map[string]any, len(raw))
	for i := range raw {
		items[i] = raw[i].(map[string]any)
	}
	return items
}

func TestCreateProblemAutoDifficulty(t *testing.T) {
	assessor := &fakeAssessor{score: 62, confidence: 0.8, reason: "常见最短路建模", configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)

	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"),
		difficultyBody("auto-ai", 50, boolPtr(true)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := detailData(t, resp)
	assert.EqualValues(t, 62, data["difficulty"])
	assert.Equal(t, "ai", data["difficultySource"])
	assert.Equal(t, "ai", data["difficultyPriorSource"])
	assert.EqualValues(t, 62, data["difficultyPrior"])
	assert.Equal(t, true, data["difficultyAuto"])
	assert.Equal(t, "常见最短路建模", data["difficultyReason"])
	assert.Equal(t, 1, assessor.calls)
	assert.Equal(t, "Problem auto-ai", assessor.lastInput.Title)

	id := int64(data["id"].(float64))
	w, hist := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history", id),
		f.token("problem_admin"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := historyItems(t, hist)
	require.Len(t, items, 1)
	assert.EqualValues(t, 0, items[0]["oldScore"])
	assert.EqualValues(t, 62, items[0]["newScore"])
	assert.Equal(t, "ai_initial", items[0]["source"])
}

func TestCreateProblemAutoFallbackOnAIError(t *testing.T) {
	assessor := &fakeAssessor{err: context.DeadlineExceeded, configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)

	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"),
		difficultyBody("auto-fallback", 50, boolPtr(true)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := detailData(t, resp)
	assert.EqualValues(t, 50, data["difficulty"])
	assert.Equal(t, "default", data["difficultySource"])
	assert.Equal(t, "default", data["difficultyPriorSource"])
	assert.Equal(t, true, data["difficultyAuto"])
	assert.Equal(t, "AI 评估超时，使用默认难度", data["difficultyReason"])
	assert.Equal(t, 1, assessor.calls, "a failed assessment still counts as one attempt")

	id := int64(data["id"].(float64))
	_, hist := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history", id),
		f.token("problem_admin"), nil)
	items := historyItems(t, hist)
	require.Len(t, items, 1)
	assert.Equal(t, "default", items[0]["source"])
	assert.EqualValues(t, 50, items[0]["newScore"])
	assert.Equal(t, "AI 评估超时，使用默认难度", items[0]["reason"])
}

func TestCreateProblemAutoFallbackReasonsAreCategorized(t *testing.T) {
	assessor := &fakeAssessor{configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)
	admin := f.token("problem_admin")

	cases := []struct {
		name     string
		prepare  func()
		want     string
		wantCall int
	}{
		{
			name: "format",
			prepare: func() {
				assessor.err = ai.ErrAssessmentFormat
			},
			want:     "AI 返回格式无效，使用默认难度",
			wantCall: 1,
		},
		{
			name: "upstream",
			prepare: func() {
				assessor.err = errors.New("dial tcp: connection refused")
			},
			want:     "AI 服务调用失败，使用默认难度",
			wantCall: 2,
		},
		{
			name: "not configured",
			prepare: func() {
				assessor.err = nil
				assessor.configured = false
			},
			want:     "AI 评估未启用或未配置，使用默认难度",
			wantCall: 2,
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.prepare()
			slug := fmt.Sprintf("auto-reason-%d", i)
			w, resp := f.do(http.MethodPost, "/api/problems", admin,
				difficultyBody(slug, 50, boolPtr(true)))
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			data := detailData(t, resp)
			assert.EqualValues(t, 50, data["difficulty"])
			assert.Equal(t, "default", data["difficultySource"])
			assert.Equal(t, tc.want, data["difficultyReason"])
			assert.Equal(t, tc.wantCall, assessor.calls)
		})
	}
}

func TestCreateProblemManualSkipsAI(t *testing.T) {
	assessor := &fakeAssessor{score: 62, configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)

	w, resp := f.do(http.MethodPost, "/api/problems", f.token("problem_admin"),
		difficultyBody("manual-create", 80, boolPtr(false)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	data := detailData(t, resp)
	assert.EqualValues(t, 80, data["difficulty"])
	assert.Equal(t, "manual", data["difficultySource"])
	assert.Equal(t, "manual", data["difficultyPriorSource"])
	assert.EqualValues(t, 80, data["difficultyPrior"])
	assert.Equal(t, false, data["difficultyAuto"])
	assert.Zero(t, assessor.calls, "manual creates must not call the assessor")
}

func TestUpdateProblemManualLock(t *testing.T) {
	assessor := &fakeAssessor{score: 62, configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)
	admin := f.token("problem_admin")

	b := difficultyBody("update-lock", 50, boolPtr(true))
	w, resp := f.do(http.MethodPost, "/api/problems", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := int64(detailData(t, resp)["id"].(float64))

	b = difficultyBody("update-lock", 75, boolPtr(false))
	w, resp = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := detailData(t, resp)
	assert.EqualValues(t, 75, data["difficulty"])
	assert.Equal(t, "manual", data["difficultySource"])
	assert.EqualValues(t, 75, data["difficultyPrior"])
	assert.Equal(t, false, data["difficultyAuto"])

	_, hist := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history", id), admin, nil)
	items := historyItems(t, hist)
	require.Len(t, items, 2)
	assert.EqualValues(t, 62, items[0]["oldScore"])
	assert.EqualValues(t, 75, items[0]["newScore"])
	assert.Equal(t, "manual", items[0]["source"])
	assert.Greater(t, items[0]["createdBy"].(float64), float64(0),
		"the API must record the acting user in history")
}

func TestAssessProblemDifficulty(t *testing.T) {
	assessor := &fakeAssessor{score: 62, configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)
	admin := f.token("problem_admin")

	b := difficultyBody("assess", 50, boolPtr(true))
	w, resp := f.do(http.MethodPost, "/api/problems", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := int64(detailData(t, resp)["id"].(float64))
	assessPath := fmt.Sprintf("/api/problems/%d/difficulty/assess", id)
	historyPath := fmt.Sprintf("/api/problems/%d/difficulty/history", id)

	assessor.score, assessor.confidence, assessor.reason = 88, 0.9, "模型重新评估"
	w, resp = f.do(http.MethodPost, assessPath, admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := detailData(t, resp)
	assert.EqualValues(t, 88, data["difficulty"])
	assert.Equal(t, "ai", data["difficultySource"])
	assert.Equal(t, true, data["difficultyAuto"], "reassessment must keep the problem automatic")
	assert.Equal(t, "模型重新评估", data["difficultyReason"])

	_, hist := f.do(http.MethodGet, historyPath, admin, nil)
	items := historyItems(t, hist)
	require.Len(t, items, 2)
	assert.Equal(t, "ai_reassess", items[0]["source"])
	assert.EqualValues(t, 62, items[0]["oldScore"])
	assert.EqualValues(t, 88, items[0]["newScore"])
	assert.InDelta(t, 0.9, items[0]["confidence"], 0.001)

	// Lock the problem and reassess again: the lock state must not change.
	b = difficultyBody("assess", 88, boolPtr(false))
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assessor.score = 61
	w, resp = f.do(http.MethodPost, assessPath, admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = detailData(t, resp)
	assert.EqualValues(t, 61, data["difficulty"])
	assert.Equal(t, false, data["difficultyAuto"], "reassessment must not unlock a manual problem")

	_, hist = f.do(http.MethodGet, historyPath, admin, nil)
	items = historyItems(t, hist)
	assert.Equal(t, "ai_reassess", items[0]["source"])
	assert.EqualValues(t, 61, items[0]["newScore"])
	assert.Equal(t, 3, assessor.calls)

	// AI disabled: 40001 and the problem is left unchanged.
	aiSettings.enabled = false
	w, resp = f.do(http.MethodPost, assessPath, admin, nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])
	_, detail := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", id), admin, nil)
	assert.EqualValues(t, 61, detailData(t, detail)["difficulty"])

	// Model not configured: also 40001.
	aiSettings.enabled = true
	assessor.configured = false
	w, resp = f.do(http.MethodPost, assessPath, admin, nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.EqualValues(t, 40001, resp["code"])

	// Upstream failure: 50200.
	assessor.configured = true
	assessor.err = errors.New("upstream timeout")
	w, resp = f.do(http.MethodPost, assessPath, admin, nil)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assert.EqualValues(t, 50200, resp["code"])
}

func TestListProblemsDifficultyRange(t *testing.T) {
	assessor := &fakeAssessor{configured: false}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)
	admin := f.token("problem_admin")

	for _, score := range []int{10, 50, 90} {
		slug := fmt.Sprintf("range-%d", score)
		w, _ := f.do(http.MethodPost, "/api/problems", admin,
			difficultyBody(slug, score, boolPtr(false)))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}

	list := func(query string) []map[string]any {
		t.Helper()
		w, resp := f.do(http.MethodGet, "/api/problems?includeHidden=true&"+query, admin, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return historyItems(t, resp)
	}

	items := list("difficultyMin=40&difficultyMax=80")
	require.Len(t, items, 1)
	assert.EqualValues(t, 50, items[0]["difficulty"])

	items = list("difficulty=90")
	require.Len(t, items, 1)
	assert.EqualValues(t, 90, items[0]["difficulty"])

	for _, query := range []string{
		"difficultyMin=0",
		"difficultyMax=101",
		"difficultyMin=80&difficultyMax=40",
		"difficultyMin=abc",
	} {
		w, resp := f.do(http.MethodGet, "/api/problems?includeHidden=true&"+query, admin, nil)
		require.Equal(t, http.StatusBadRequest, w.Code, query+": "+w.Body.String())
		assert.EqualValues(t, 40001, resp["code"], query)
	}
}

func TestDifficultyHistoryAndManagerDetail(t *testing.T) {
	assessor := &fakeAssessor{score: 62, configured: true}
	aiSettings := &fakeAISettings{enabled: true}
	f := newDifficultyFixture(t, assessor, aiSettings)
	admin := f.token("problem_admin")

	b := difficultyBody("detail-meta", 50, boolPtr(true))
	w, resp := f.do(http.MethodPost, "/api/problems", admin, b)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := int64(detailData(t, resp)["id"].(float64))
	f.addCaseAs(id, admin)

	// Publish while preserving the automatic state.
	b = difficultyBody("detail-meta", 62, boolPtr(true))
	b["isPublic"] = true
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	metaKeys := []string{
		"difficultyAuto", "difficultySource", "difficultyPrior",
		"difficultyPriorSource", "difficultyReason", "difficultyUpdatedAt",
	}
	_, adminDetail := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", id), admin, nil)
	adminData := detailData(t, adminDetail)
	for _, key := range metaKeys {
		assert.Contains(t, adminData, key, "managers see %s", key)
	}

	// An accepted solver sees the solution but never the difficulty metadata.
	solverToken, solverID := f.registerUser("user")
	require.NoError(t, f.db.Exec(
		`INSERT INTO submissions (user_id, problem_id, language, code, status, created_at)
		 VALUES (?, ?, 'cpp', 'int main(){}', 'accepted', now())`, solverID, id).Error)
	w, solverDetail := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d", id), solverToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	solverData := detailData(t, solverDetail)
	assert.Contains(t, solverData, "solutionMd", "accepted users can read solutions")
	for _, key := range metaKeys {
		assert.NotContains(t, solverData, key, "accepted users must not see %s", key)
	}

	// History: newest first, paginated, manager-only, full JSON contract.
	b = difficultyBody("detail-meta", 70, boolPtr(false))
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	b = difficultyBody("detail-meta", 80, boolPtr(false))
	w, _ = f.do(http.MethodPut, fmt.Sprintf("/api/problems/%d", id), admin, b)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	historyPath := fmt.Sprintf("/api/problems/%d/difficulty/history?page=1&page_size=2", id)
	w, hist := f.do(http.MethodGet, historyPath, admin, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	page := detailData(t, hist)
	assert.EqualValues(t, 3, page["total"])
	items := historyItems(t, hist)
	require.Len(t, items, 2)
	assert.EqualValues(t, 80, items[0]["newScore"])
	assert.EqualValues(t, 70, items[1]["newScore"])
	for _, key := range []string{
		"oldScore", "newScore", "prior", "source", "sampleUsers", "acceptedUsers",
		"passRate", "smoothedPassRate", "targetScore", "confidence", "reason",
		"runId", "createdBy", "createdAt",
	} {
		assert.Contains(t, items[0], key, "history item carries %s", key)
	}

	_, second := f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history?page=2&page_size=2", id), admin, nil)
	secondItems := historyItems(t, second)
	require.Len(t, secondItems, 1)
	assert.Equal(t, "ai_initial", secondItems[0]["source"])

	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history", id), solverToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = f.do(http.MethodGet, fmt.Sprintf("/api/problems/%d/difficulty/history", id), "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

var _ problemapi.DifficultyAssessor = (*fakeAssessor)(nil)
var _ problemapi.AISettings = (*fakeAISettings)(nil)
