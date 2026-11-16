package similarity

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/contest"
)

const csvPageSize = 100

// csvBOM makes spreadsheet applications detect UTF-8, which is required for
// the Chinese header to render correctly.
var csvBOM = []byte{0xEF, 0xBB, 0xBF}

var csvHeader = []string{
	"比赛ID", "比赛标题", "题目ID", "题号", "题目标题",
	"原提交ID", "原提交用户ID", "原提交用户名", "原提交昵称", "原提交时间", "原提交语言",
	"疑似提交ID", "疑似用户ID", "疑似用户名", "疑似昵称", "疑似提交时间", "疑似语言",
	"最高重复率", "Dice相似度", "阈值", "计算完成时间", "说明",
}

const csvReviewNotice = "疑似抄袭，需人工复核"

// buildMatchesCSV renders match rows as a BOM-prefixed UTF-8 CSV document.
func buildMatchesCSV(c *contest.Contest, problems map[int64]contest.ProblemRow, rows []MatchRow) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(csvBOM)

	w := csv.NewWriter(&buf)
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, row := range rows {
		problem := problems[row.ProblemID]
		record := []string{
			strconv.FormatInt(row.ContestID, 10),
			c.Title,
			strconv.FormatInt(row.ProblemID, 10),
			problem.DisplayID,
			problem.ProblemTitle,
			strconv.FormatInt(row.OriginalSubmissionID, 10),
			strconv.FormatInt(row.OriginalUserID, 10),
			row.OriginalUsername,
			row.OriginalNickname,
			row.OriginalCreatedAt.Format(time.RFC3339),
			string(row.Language),
			strconv.FormatInt(row.SuspectSubmissionID, 10),
			strconv.FormatInt(row.SuspectUserID, 10),
			row.SuspectUsername,
			row.SuspectNickname,
			row.SuspectCreatedAt.Format(time.RFC3339),
			string(row.Language),
			strconv.FormatFloat(row.SimilarityPercent, 'f', 1, 64),
			strconv.FormatFloat(row.DicePercent, 'f', 1, 64),
			strconv.Itoa(row.ThresholdPercent),
			formatCSVTime(row.FinishedAt),
			csvReviewNotice,
		}
		for i := range record {
			record[i] = sanitizeCSVCell(record[i])
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitizeCSVCell neutralizes spreadsheet formula injection by prefixing
// cells that start with a formula trigger with an apostrophe.
func sanitizeCSVCell(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@':
		return "'" + value
	}
	return value
}

// formatCSVTime renders an optional time as RFC3339, leaving nil empty.
func formatCSVTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
