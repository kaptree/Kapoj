package similarity_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
)

var csvTestHeader = []string{
	"比赛ID", "比赛标题", "题目ID", "题号", "题目标题",
	"原提交ID", "原提交用户ID", "原提交用户名", "原提交昵称", "原提交时间", "原提交语言",
	"疑似提交ID", "疑似用户ID", "疑似用户名", "疑似昵称", "疑似提交时间", "疑似语言",
	"最高重复率", "Dice相似度", "阈值", "计算完成时间", "说明",
}

const csvTestReviewNotice = "疑似抄袭，需人工复核"

// seedCSVCompletedRun creates a completed run with one match and returns it.
func seedCSVCompletedRun(t *testing.T, e *svcEnv, problemID int64, threshold int, similarityPercent, dicePercent float64, originalSubmissionID, suspectSubmissionID int64) *similarity.Run {
	t.Helper()
	finished := time.Now().UTC().Truncate(time.Microsecond)
	run := e.createRun(t, &similarity.Run{
		ContestID: e.contestID, ProblemID: problemID, Status: similarity.RunStatusCompleted,
		ThresholdPercent: threshold, IgnoreCommonCode: true, Engine: "winnowing-v1", EngineVersion: "1",
		FinishedAt: &finished,
	})
	require.NoError(t, e.repo.ReplaceMatches(context.Background(), run.ID, []similarity.Match{{
		ContestID: e.contestID, ProblemID: problemID,
		OriginalSubmissionID: originalSubmissionID, SuspectSubmissionID: suspectSubmissionID,
		OriginalUserID: e.userA, SuspectUserID: e.userB,
		Language: submission.LanguageCPP, SimilarityPercent: similarityPercent, DicePercent: dicePercent,
		SharedFingerprints: 15, OriginalFingerprints: 20, SuspectFingerprints: 20,
		OriginalCreatedAt: e.start.Add(time.Minute), SuspectCreatedAt: e.start.Add(2 * time.Minute),
	}}))
	return run
}

func parseCSVBody(t *testing.T, data []byte) [][]string {
	t.Helper()
	require.True(t, bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}), "CSV must start with a UTF-8 BOM")
	records, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	require.NoError(t, err)
	return records
}

func TestExportCSVUsesBOMAndHeaders(t *testing.T) {
	e := newServiceEnv(t)

	data, err := e.svc.ExportCSV(context.Background(), managerViewer, e.contestID, 0)
	require.NoError(t, err)

	records := parseCSVBody(t, data)
	require.NotEmpty(t, records)
	assert.Equal(t, csvTestHeader, records[0])
	assert.Contains(t, string(data), "最高重复率")
	assert.Contains(t, string(data), "说明")
}

func TestExportCSVEscapesFormula(t *testing.T) {
	e := newServiceEnv(t)
	require.NoError(t, e.db.Table("users").Where("id = ?", e.userA).Update("nickname", "=cmd").Error)

	seedCSVCompletedRun(t, e, e.problemA, 80, 95, 90, 1, 2)

	data, err := e.svc.ExportCSV(context.Background(), managerViewer, e.contestID, 0)
	require.NoError(t, err)

	records := parseCSVBody(t, data)
	require.Len(t, records, 2)
	assert.Equal(t, "'=cmd", records[1][8], "formula-like nickname must be prefixed")
	assert.Equal(t, csvTestReviewNotice, records[1][len(csvTestHeader)-1])
}

func TestExportCSVEscapesCSVQuotes(t *testing.T) {
	e := newServiceEnv(t)
	title := `Contest "A", B`
	require.NoError(t, e.db.Table("contests").Where("id = ?", e.contestID).Update("title", title).Error)

	seedCSVCompletedRun(t, e, e.problemA, 80, 95, 90, 1, 2)

	data, err := e.svc.ExportCSV(context.Background(), managerViewer, e.contestID, 0)
	require.NoError(t, err)

	records := parseCSVBody(t, data)
	require.Len(t, records, 2)
	assert.Equal(t, title, records[1][1], "commas and quotes must be parsed back as one field")
}

func TestExportCSVUsesLatestCompletedRun(t *testing.T) {
	e := newServiceEnv(t)

	seedCSVCompletedRun(t, e, e.problemA, 80, 90, 80, 1, 101)
	seedCSVCompletedRun(t, e, e.problemA, 85, 95, 90, 2, 102)

	data, err := e.svc.ExportCSV(context.Background(), managerViewer, e.contestID, 0)
	require.NoError(t, err)

	records := parseCSVBody(t, data)
	require.Len(t, records, 2, "only the latest completed run must be exported")
	assert.Equal(t, "2", records[1][5])
	assert.Equal(t, "85", records[1][19])
}
