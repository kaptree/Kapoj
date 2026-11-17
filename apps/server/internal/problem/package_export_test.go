package problem_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func packageEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		out[f.Name] = body
	}
	return out
}

func decodePackageJSON[T any](t *testing.T, data []byte) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestExportProblemPackage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	in := input("package-export")
	in.Type = problem.TypeSpecial
	in.StatementMD = "初始题面"
	in.InputFormatMD = "两个整数"
	in.OutputFormatMD = "一个整数"
	in.HintMD = "注意范围"
	in.Samples = []problem.Sample{{Input: "1 2", Output: "3"}}
	in.Tags = []string{"数组", "模拟"}
	in.Difficulty = 3
	in.TimeLimitMS = 1500
	in.MemoryLimitMB = 512
	in.Source = "自编"
	p := e.create(t, in)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).Updates(map[string]any{
		"solution_md":          "官方题解正文",
		"solution_code":        "int main() { return 0; }",
		"solution_price_coins": 25,
	}).Error)

	img, err := e.svc.AddStatementImage(ctx, p.ID, 1, problem.AttachmentUpload{
		Filename: "示意图.png", Reader: strings.NewReader("\x89PNG\r\n\x1a\nimage"),
	})
	require.NoError(t, err)
	statement := fmt.Sprintf("题面 ![示意](/api/problems/%d/assets/%d)", p.ID, img.ID)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Update("statement_md", statement).Error)

	att, err := e.svc.AddAttachment(ctx, p.ID, 1, problem.AttachmentUpload{
		Filename: "说明.txt", ContentType: "text/plain", Reader: strings.NewReader("附件内容"),
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("int main() {}")))
	_, err = e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: strings.NewReader("1 2"), Output: strings.NewReader("3"), Score: 30, IsSample: true,
	})
	require.NoError(t, err)
	_, err = e.svc.AddTestcase(ctx, p.ID, problem.TestcaseUpload{
		Input: strings.NewReader("4 5"), Output: strings.NewReader("9"), Score: 70,
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	manifest, err := e.svc.ExportProblem(ctx, p.ID, &buf)
	require.NoError(t, err)

	assert.Equal(t, problem.PackageFormatProblem, manifest.Format)
	assert.Equal(t, problem.PackageFormatVersion, manifest.FormatVersion)
	assert.WithinDuration(t, time.Now(), manifest.ExportedAt, time.Minute)
	assert.Equal(t, p.Slug, manifest.Problem.Slug)
	assert.Equal(t, p.Title, manifest.Problem.Title)
	assert.Equal(t, problem.TypeSpecial, manifest.Problem.Type)
	assert.Equal(t, 3, manifest.Problem.Difficulty)
	assert.Equal(t, 1500, manifest.Problem.TimeLimitMS)
	assert.Equal(t, 512, manifest.Problem.MemoryLimitMB)
	assert.Equal(t, "自编", manifest.Problem.Source)
	assert.ElementsMatch(t, []string{"数组", "模拟"}, manifest.Problem.Tags)
	assert.EqualValues(t, 25, manifest.Problem.SolutionPriceCoins)

	entries := packageEntries(t, buf.Bytes())
	require.Contains(t, entries, "manifest.json")
	require.Contains(t, entries, "statement.md")
	require.Contains(t, entries, "testdata/meta.json")
	require.Contains(t, entries, manifest.Files.Checker)
	require.Contains(t, entries, manifest.Files.Solution)
	require.Contains(t, entries, manifest.Files.SolutionCode)
	require.Len(t, manifest.Files.Images, 1)
	require.Len(t, manifest.Files.Attachments, 1)

	assert.Equal(t, "官方题解正文", string(entries[manifest.Files.Solution]))
	assert.Equal(t, "int main() { return 0; }", string(entries[manifest.Files.SolutionCode]))
	assert.Equal(t, "int main() {}", string(entries[manifest.Files.Checker]))
	assert.Equal(t, att.Filename, manifest.Files.Attachments[0].Filename)
	assert.Equal(t, "附件内容", string(entries[manifest.Files.Attachments[0].File]))
	assert.Equal(t, "示意图.png", manifest.Files.Images[0].Filename)

	exportedStatement := string(entries["statement.md"])
	assert.Contains(t, exportedStatement, "kapoj-package://images/")
	assert.NotContains(t, exportedStatement, fmt.Sprintf("/api/problems/%d/assets/%d", p.ID, img.ID),
		"statement images must be replaced with a package-relative placeholder")

	testdata := decodePackageJSON[problem.TestdataManifest](t, entries["testdata/meta.json"])
	require.Len(t, testdata.Testcases, 2)
	assert.Equal(t, 1, testdata.Testcases[0].SortOrder)
	assert.Equal(t, 30, testdata.Testcases[0].Score)
	assert.True(t, testdata.Testcases[0].IsSample)
	assert.Contains(t, entries, testdata.Testcases[0].Input)
	assert.Contains(t, entries, testdata.Testcases[0].Output)
	assert.Equal(t, 2, testdata.Testcases[1].SortOrder)
	assert.Equal(t, 70, testdata.Testcases[1].Score)
	assert.False(t, testdata.Testcases[1].IsSample)
	assert.Equal(t, "9", string(entries[testdata.Testcases[1].Output]))
}

func TestExportProblemPackageIncludesObjectiveAnswers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	in := input("objective-export")
	in.Type = problem.TypeObjective
	in.Answers = []string{"A", "B"}
	p := e.create(t, in)

	var buf bytes.Buffer
	manifest, err := e.svc.ExportProblem(ctx, p.ID, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Files.Answers)

	entries := packageEntries(t, buf.Bytes())
	var answers []string
	require.NoError(t, json.Unmarshal(entries[manifest.Files.Answers], &answers))
	assert.Equal(t, []string{"A", "B"}, answers)
}

func TestExportProblemPackageMissingProblem(t *testing.T) {
	e := newEnv(t)
	var buf bytes.Buffer
	_, err := e.svc.ExportProblem(context.Background(), 999999, &buf)
	assertCode(t, err, 40400)
}
