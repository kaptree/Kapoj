package agentapi_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func (f *apiFixture) createAgentWithToken(t *testing.T, username string) (int64, string, string) {
	t.Helper()
	userID := f.user(username, "active")
	jwt := f.token(userID)

	w, resp := f.do(http.MethodPost, "/api/agents", jwt,
		fmt.Sprintf(`{"name":"%s","modelName":"test-model"}`, username))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	agentID := int64(dataObject(t, resp)["id"].(float64))

	w, resp = f.do(http.MethodPost, "/api/agents/me/token", jwt, "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	token, ok := dataObject(t, resp)["token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, token)
	return agentID, token, jwt
}

func (f *apiFixture) seedAPIProblem(slug string) *problem.Problem {
	f.t.Helper()
	p := &problem.Problem{
		Title:          "Agent Problem",
		Slug:           slug,
		StatementMD:    "statement",
		InputFormatMD:  "input format",
		OutputFormatMD: "output format",
		Samples:        dbtypes.List[problem.Sample]{{Input: "1 2", Output: "3"}},
		Tags:           dbtypes.List[string]{"math", "easy"},
		Type:           problem.TypeTraditional,
		Difficulty:     50,
		TimeLimitMS:    1000,
		MemoryLimitMB:  256,
		IsPublic:       true,
		Source:         "unit-test",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(f.t, f.db.Create(p).Error)
	require.NoError(f.t, f.db.Create(&problem.Testcase{
		ProblemID: p.ID, InputFile: "1.in", OutputFile: "1.out",
		InputSize: 1, OutputSize: 1, Score: 100, SortOrder: 1,
	}).Error)
	return p
}

func TestAgentTokenRoutes(t *testing.T) {
	f := newAPIFixture(t)
	agentID, token, jwt := f.createAgentWithToken(t, "machine-owner")

	for _, raw := range []string{"", "not-a-real-token", jwt} {
		w, resp := f.do(http.MethodGet, "/api/agent/me", raw, "")
		require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		assert.EqualValues(t, 40102, resp["code"])
	}

	w, resp := f.do(http.MethodGet, "/api/agent/me", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := dataObject(t, resp)
	agentData, ok := data["agent"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, agentID, agentData["id"])
	assert.Equal(t, "machine-owner", agentData["name"])
	author, ok := agentData["author"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "machine-owner", author["username"])
	assert.Nil(t, data["currentAttempt"])

	// No public problem exists yet.
	w, resp = f.do(http.MethodPost, "/api/agent/problems/random", token, "")
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.EqualValues(t, 40401, resp["code"])

	p := f.seedAPIProblem("api-claimable")
	w, resp = f.do(http.MethodPost, "/api/agent/problems/random", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data = dataObject(t, resp)
	attemptData, ok := data["attempt"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, p.ID, attemptData["problemId"])
	assert.Equal(t, "active", attemptData["status"])
	assert.EqualValues(t, 0, attemptData["attemptsUsed"])
	assert.EqualValues(t, agent.MaxAttempts, attemptData["attemptsRemaining"])
	require.NotEmpty(t, attemptData["deadlineAt"])
	firstDeadline := attemptData["deadlineAt"]

	problemData, ok := data["problem"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, p.ID, problemData["id"])
	assert.Equal(t, "Agent Problem", problemData["title"])
	assert.Equal(t, "api-claimable", problemData["slug"])
	assert.Equal(t, "traditional", problemData["type"])
	assert.Equal(t, "statement", problemData["statementMd"])
	assert.Equal(t, "input format", problemData["inputFormatMd"])
	assert.Equal(t, "output format", problemData["outputFormatMd"])
	assert.EqualValues(t, 50, problemData["difficulty"])
	assert.EqualValues(t, 1000, problemData["timeLimitMs"])
	assert.EqualValues(t, 256, problemData["memoryLimitMb"])
	assert.Equal(t, "unit-test", problemData["source"])
	samples, ok := problemData["samples"].([]any)
	require.True(t, ok)
	require.Len(t, samples, 1)
	tags, ok := problemData["tags"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"math", "easy"}, tags)
	for _, hidden := range []string{"answers", "solutionMd", "testcases", "checkerPath"} {
		assert.NotContains(t, problemData, hidden)
	}

	w, resp = f.do(http.MethodPost, "/api/agent/problems/random", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	second := dataObject(t, resp)["attempt"].(map[string]any)
	assert.EqualValues(t, attemptData["id"], second["id"])
	assert.Equal(t, firstDeadline, second["deadlineAt"])

	w, resp = f.do(http.MethodGet, "/api/agent/me", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	current, ok := dataObject(t, resp)["currentAttempt"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, attemptData["id"], current["id"])

	// Disabled Agents are rejected, and re-enabling does not need a new token.
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", agentID).
		Update("status", agent.StatusDisabled).Error)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/agent/me"},
		{http.MethodPost, "/api/agent/problems/random"},
	} {
		w, resp = f.do(call.method, call.path, token, "")
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		assert.EqualValues(t, 40304, resp["code"])
	}
	require.NoError(t, f.db.Model(&agent.Agent{}).Where("id = ?", agentID).
		Update("status", agent.StatusActive).Error)
	w, _ = f.do(http.MethodGet, "/api/agent/me", token, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Revoking the token kills access immediately.
	w, resp = f.do(http.MethodDelete, "/api/agents/me/token", jwt, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w, resp = f.do(http.MethodGet, "/api/agent/me", token, "")
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.EqualValues(t, 40102, resp["code"])
}
