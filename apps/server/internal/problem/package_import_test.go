package problem_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
)

func readZipEntry(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	require.NoError(t, err)
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	return body
}

// rewritePackage runs rewrite over every entry and rebuilds the ZIP.
func rewritePackage(t *testing.T, data []byte, rewrite func(name string, body []byte) (string, []byte)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		name, body := rewrite(f.Name, readZipEntry(t, f))
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func renamePackageSlug(t *testing.T, data []byte, slug string) []byte {
	t.Helper()
	return rewritePackage(t, data, func(name string, body []byte) (string, []byte) {
		if name != "manifest.json" {
			return name, body
		}
		var manifest problem.PackageManifest
		require.NoError(t, json.Unmarshal(body, &manifest))
		manifest.Problem.Slug = slug
		updated, err := json.Marshal(manifest)
		require.NoError(t, err)
		return name, updated
	})
}

func replacePackageFile(t *testing.T, data []byte, file, content string) []byte {
	t.Helper()
	return rewritePackage(t, data, func(name string, body []byte) (string, []byte) {
		if name == file {
			return name, []byte(content)
		}
		return name, body
	})
}

// rewritePackageManifest rewrites the single-problem manifest.json.
func rewritePackageManifest(t *testing.T, data []byte, mutate func(*problem.PackageProblem)) []byte {
	t.Helper()
	return rewritePackage(t, data, func(name string, body []byte) (string, []byte) {
		if name != "manifest.json" {
			return name, body
		}
		var manifest problem.PackageManifest
		require.NoError(t, json.Unmarshal(body, &manifest))
		mutate(&manifest.Problem)
		updated, err := json.Marshal(manifest)
		require.NoError(t, err)
		return name, updated
	})
}

func difficultyHistory(t *testing.T, e *env, problemID int64) []problem.DifficultyHistory {
	t.Helper()
	items, total, err := e.repo.ListDifficultyHistory(context.Background(), problemID, 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(len(items)), total)
	return items
}

func TestImportKapOJPackageRoundTrip(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	in := input("import-source")
	in.Type = problem.TypeSpecial
	in.StatementMD = "初始题面"
	in.InputFormatMD = "两个整数"
	in.OutputFormatMD = "一个整数"
	in.HintMD = "注意范围"
	in.Samples = []problem.Sample{{Input: "1 2", Output: "3"}}
	in.Tags = []string{"数组"}
	in.Difficulty = 4
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
	_, err = e.svc.AddAttachment(ctx, p.ID, 1, problem.AttachmentUpload{
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

	var exported bytes.Buffer
	_, err = e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := renamePackageSlug(t, exported.Bytes(), "imported-copy")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)), problem.PackageImportOptions{
		ConflictPolicy: "skip",
	})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	imported := result.Imported[0]
	assert.Equal(t, "imported-copy", imported.Slug)
	assert.False(t, imported.Overwritten)
	assert.False(t, imported.Published)
	assert.Empty(t, imported.PublishError)

	copied, err := e.repo.FindBySlug(ctx, "imported-copy")
	require.NoError(t, err)
	require.NotNil(t, copied)
	assert.NotEqual(t, p.ID, copied.ID)
	assert.Equal(t, int64(1), copied.CreatedBy)
	assert.False(t, copied.IsPublic)
	assert.Equal(t, "Problem import-source", copied.Title)
	assert.Equal(t, problem.TypeSpecial, copied.Type)
	assert.Equal(t, 4, copied.Difficulty)
	assert.Equal(t, 1500, copied.TimeLimitMS)
	assert.Equal(t, 512, copied.MemoryLimitMB)
	assert.Equal(t, "自编", copied.Source)
	assert.Equal(t, []string{"数组"}, []string(copied.Tags))
	assert.Equal(t, []problem.Sample{{Input: "1 2", Output: "3"}}, []problem.Sample(copied.Samples))
	assert.Equal(t, "官方题解正文", copied.SolutionMD)
	assert.Equal(t, "int main() { return 0; }", copied.SolutionCode)
	assert.EqualValues(t, 25, copied.SolutionPriceCoins)
	assert.Contains(t, copied.StatementMD, fmt.Sprintf("/api/problems/%d/assets/", copied.ID))
	assert.NotContains(t, copied.StatementMD, "kapoj-package://")

	cases, err := e.svc.ListTestcases(ctx, copied.ID)
	require.NoError(t, err)
	require.Len(t, cases, 2)
	assert.Equal(t, 30, cases[0].Score)
	assert.True(t, cases[0].IsSample)
	assert.Equal(t, 1, cases[0].SortOrder)
	assert.Equal(t, 70, cases[1].Score)
	assert.False(t, cases[1].IsSample)
	assert.Equal(t, 2, cases[1].SortOrder)
	content, err := e.svc.GetTestcaseContent(ctx, copied.ID, cases[1].ID)
	require.NoError(t, err)
	assert.Equal(t, "4 5", content.Input)
	assert.Equal(t, "9", content.Output)

	attachments, err := e.repo.ListAttachments(ctx, copied.ID, problem.AttachmentKindFile)
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	assert.Equal(t, "说明.txt", attachments[0].Filename)
	images, err := e.repo.ListAttachments(ctx, copied.ID, problem.AttachmentKindStatementImage)
	require.NoError(t, err)
	require.Len(t, images, 1)

	require.NotEmpty(t, copied.CheckerPath)
	checker, err := e.store.Open(ctx, copied.CheckerPath)
	require.NoError(t, err)
	checkerBody, err := io.ReadAll(checker)
	require.NoError(t, err)
	require.NoError(t, checker.Close())
	assert.Equal(t, "int main() {}", string(checkerBody))

	// The import decision records the importer and the import source.
	history := difficultyHistory(t, e, copied.ID)
	require.Len(t, history, 1)
	assert.Equal(t, 0, history[0].OldScore)
	assert.Equal(t, 4, history[0].NewScore)
	assert.Equal(t, 4, history[0].Prior)
	assert.Equal(t, problem.DifficultySourceImport, history[0].Source)
	assert.Equal(t, int64(1), history[0].CreatedBy)
	assert.Equal(t, problem.DifficultyPriorSourceImport, copied.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceImport, copied.DifficultySource)
	assert.True(t, copied.DifficultyAuto)
}

func TestPackageImportOldScale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	source := e.create(t, input("old-scale-source"))
	e.addCase(t, source.ID)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, source.ID, &exported)
	require.NoError(t, err)

	// Simulate a package written by the pre-1-100 application: only the
	// legacy 1–5 field is present.
	data := renamePackageSlug(t, rewritePackageManifest(t, exported.Bytes(),
		func(p *problem.PackageProblem) {
			p.Difficulty = 3
			p.DifficultyScore = nil
			p.DifficultyScale = ""
		}), "old-scale-copy")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)

	copied, err := e.repo.FindBySlug(ctx, "old-scale-copy")
	require.NoError(t, err)
	require.NotNil(t, copied)
	assert.Equal(t, 50, copied.Difficulty, "legacy band 3 maps to 50")
	assert.Equal(t, 50, copied.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceImport, copied.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceImport, copied.DifficultySource)
	assert.True(t, copied.DifficultyAuto)

	history := difficultyHistory(t, e, copied.ID)
	require.Len(t, history, 1)
	assert.Equal(t, 0, history[0].OldScore)
	assert.Equal(t, 50, history[0].NewScore)
	assert.Equal(t, 50, history[0].Prior)
	assert.Equal(t, problem.DifficultySourceImport, history[0].Source)
	assert.Equal(t, int64(1), history[0].CreatedBy)
}

func TestPackageImportRejectsInvalidDifficultyFields(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	source := e.create(t, input("invalid-difficulty-source"))
	e.addCase(t, source.ID)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, source.ID, &exported)
	require.NoError(t, err)
	base := exported.Bytes()

	cases := []struct {
		name   string
		slug   string
		mutate func(*problem.PackageProblem)
	}{
		{
			name: "unknown scale",
			slug: "invalid-scale",
			mutate: func(p *problem.PackageProblem) {
				p.DifficultyScale = "0-10"
			},
		},
		{
			name: "legacy difficulty out of range",
			slug: "invalid-legacy",
			mutate: func(p *problem.PackageProblem) {
				p.Difficulty = 9
				p.DifficultyScore = nil
				p.DifficultyScale = ""
			},
		},
		{
			name: "score too low",
			slug: "invalid-low",
			mutate: func(p *problem.PackageProblem) {
				zero := 0
				p.DifficultyScore = &zero
			},
		},
		{
			name: "score too high",
			slug: "invalid-high",
			mutate: func(p *problem.PackageProblem) {
				high := 101
				p.DifficultyScore = &high
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := renamePackageSlug(t, rewritePackageManifest(t, base, tc.mutate), tc.slug)
			_, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
				problem.PackageImportOptions{ConflictPolicy: "skip"})
			assertCode(t, err, 40001)
		})
	}
}

func TestPackageImportOverwriteWritesHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	existing := e.create(t, input("overwrite-difficulty"))
	e.addCase(t, existing.ID)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", existing.ID).
		Update("difficulty_auto", false).Error)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, existing.ID, &exported)
	require.NoError(t, err)
	data := rewritePackageManifest(t, exported.Bytes(), func(p *problem.PackageProblem) {
		score := 73
		p.Difficulty = 4
		p.DifficultyScore = &score
		p.DifficultyScale = "1-100"
	})

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "overwrite"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	assert.True(t, result.Imported[0].Overwritten)

	updated, err := e.repo.FindByID(ctx, existing.ID)
	require.NoError(t, err)
	assert.Equal(t, 73, updated.Difficulty)
	assert.Equal(t, 73, updated.DifficultyPrior)
	assert.Equal(t, problem.DifficultyPriorSourceImport, updated.DifficultyPriorSource)
	assert.Equal(t, problem.DifficultySourceImport, updated.DifficultySource)
	assert.True(t, updated.DifficultyAuto, "package overwrite resets the problem to automatic")

	history := difficultyHistory(t, e, existing.ID)
	require.Len(t, history, 2)
	// Newest first: the import row, then the original manual creation row.
	assert.Equal(t, 1, history[0].OldScore, "old_score is the score before the overwrite")
	assert.Equal(t, 73, history[0].NewScore)
	assert.Equal(t, 73, history[0].Prior)
	assert.Equal(t, problem.DifficultySourceImport, history[0].Source)
	assert.Equal(t, int64(1), history[0].CreatedBy)
	assert.Equal(t, problem.DifficultySourceManual, history[1].Source)
}

func TestImportKapOJObjectiveAnswers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("objective-source")
	in.Type = problem.TypeObjective
	in.Answers = []string{"A", "B"}
	p := e.create(t, in)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := renamePackageSlug(t, exported.Bytes(), "objective-copy")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)

	copied, err := e.repo.FindBySlug(ctx, "objective-copy")
	require.NoError(t, err)
	require.NotNil(t, copied)
	assert.Equal(t, problem.TypeObjective, copied.Type)
	assert.Equal(t, []string{"A", "B"}, []string(copied.Answers))
}

func TestImportKapOJOverwritePreservesIdentity(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("overwrite-source"))
	e.addCase(t, p.ID)
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).Updates(map[string]any{
		"is_public": true, "submit_count": 7, "accept_count": 3,
	}).Error)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := replacePackageFile(t, exported.Bytes(), "testdata/001.out", "99")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "overwrite"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	imported := result.Imported[0]
	assert.True(t, imported.Overwritten)
	assert.Equal(t, p.ID, imported.ID)

	updated, err := e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, updated.ID)
	assert.Equal(t, int64(1), updated.CreatedBy)
	assert.Equal(t, 7, updated.SubmitCount)
	assert.Equal(t, 3, updated.AcceptCount)
	assert.False(t, updated.IsPublic, "overwrite import must force the problem offline")

	cases, err := e.svc.ListTestcases(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	content, err := e.svc.GetTestcaseContent(ctx, p.ID, cases[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "99", content.Output, "old testdata must be replaced")
}

func TestImportKapOJPublishFailureIsPerProblem(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("publish-empty"))

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := renamePackageSlug(t, exported.Bytes(), "publish-empty-copy")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip", Publish: true, CanPublish: true})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	assert.False(t, result.Imported[0].Published)
	assert.Contains(t, result.Imported[0].PublishError, "测试点")

	created, err := e.repo.FindBySlug(ctx, "publish-empty-copy")
	require.NoError(t, err)
	require.NotNil(t, created, "a publish failure must keep the imported problem")
	assert.False(t, created.IsPublic)

	denied := renamePackageSlug(t, exported.Bytes(), "publish-denied")
	_, err = e.svc.ImportPackage(ctx, 1, bytes.NewReader(denied), int64(len(denied)),
		problem.PackageImportOptions{ConflictPolicy: "skip", Publish: true})
	assertCode(t, err, 40300)
	missing, err := e.repo.FindBySlug(ctx, "publish-denied")
	require.NoError(t, err)
	assert.Nil(t, missing, "publishing without permission must not import anything")
}

func renameBatchSlugs(t *testing.T, data []byte, mapping map[string]string) []byte {
	t.Helper()
	return rewritePackage(t, data, func(name string, body []byte) (string, []byte) {
		newName := name
		for oldSlug, newSlug := range mapping {
			oldDir := "problems/" + oldSlug
			if name == oldDir || strings.HasPrefix(name, oldDir+"/") {
				newName = "problems/" + newSlug + strings.TrimPrefix(name, oldDir)
			}
		}
		if name == "manifest.json" {
			var root problem.ProblemsManifest
			require.NoError(t, json.Unmarshal(body, &root))
			for i := range root.Problems {
				if newSlug, ok := mapping[root.Problems[i].Slug]; ok {
					root.Problems[i].Slug = newSlug
					root.Problems[i].Directory = "problems/" + newSlug
				}
			}
			updated, err := json.Marshal(root)
			require.NoError(t, err)
			return newName, updated
		}
		if strings.HasSuffix(name, "/manifest.json") {
			var manifest problem.PackageManifest
			require.NoError(t, json.Unmarshal(body, &manifest))
			if newSlug, ok := mapping[manifest.Problem.Slug]; ok {
				manifest.Problem.Slug = newSlug
				updated, err := json.Marshal(manifest)
				require.NoError(t, err)
				return newName, updated
			}
		}
		return newName, body
	})
}

func TestImportKapOJBatchPackage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first := e.create(t, input("batch-source-one"))
	e.addCase(t, first.ID)
	second := e.create(t, input("batch-source-two"))
	e.addCase(t, second.ID)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblems(ctx, []int64{first.ID, second.ID}, &exported)
	require.NoError(t, err)

	conflicts, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(exported.Bytes()),
		int64(exported.Len()), problem.PackageImportOptions{ConflictPolicy: "ask"})
	require.NoError(t, err)
	assert.Empty(t, conflicts.Imported)
	assert.Len(t, conflicts.Conflicts, 2)

	data := renameBatchSlugs(t, exported.Bytes(), map[string]string{
		"batch-source-one": "batch-copy-one",
		"batch-source-two": "batch-copy-two",
	})
	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 2)
	assert.Equal(t, "batch-copy-one", result.Imported[0].Slug)
	assert.Equal(t, "batch-copy-two", result.Imported[1].Slug)

	for _, slug := range []string{"batch-copy-one", "batch-copy-two"} {
		copied, err := e.repo.FindBySlug(ctx, slug)
		require.NoError(t, err)
		require.NotNil(t, copied)
		cases, err := e.svc.ListTestcases(ctx, copied.ID)
		require.NoError(t, err)
		assert.Len(t, cases, 1)
	}
}

func prefixPackageDir(t *testing.T, data []byte, dir string) []byte {
	t.Helper()
	return rewritePackage(t, data, func(name string, body []byte) (string, []byte) {
		return dir + "/" + name, body
	})
}

func TestImportKapOJNestedPackage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("nested-source"))
	e.addCase(t, p.ID)

	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := prefixPackageDir(t, renamePackageSlug(t, exported.Bytes(), "nested-copy"), "wrapped")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	assert.Equal(t, "nested-copy", result.Imported[0].Slug)
	copied, err := e.repo.FindBySlug(ctx, "nested-copy")
	require.NoError(t, err)
	require.NotNil(t, copied)
}

func TestImportKapOJRejectsDuplicateAttachmentReference(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("dup-attachment"))
	_, err := e.svc.AddAttachment(ctx, p.ID, 1, problem.AttachmentUpload{
		Filename: "dup.txt", ContentType: "text/plain", Reader: strings.NewReader("dup"),
	})
	require.NoError(t, err)
	var exported bytes.Buffer
	_, err = e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)

	data := rewritePackage(t, exported.Bytes(), func(name string, body []byte) (string, []byte) {
		if name != "manifest.json" {
			return name, body
		}
		var manifest problem.PackageManifest
		require.NoError(t, json.Unmarshal(body, &manifest))
		require.NotEmpty(t, manifest.Files.Attachments)
		manifest.Files.Attachments = append(manifest.Files.Attachments, manifest.Files.Attachments[0])
		updated, err := json.Marshal(manifest)
		require.NoError(t, err)
		return name, updated
	})
	data = renamePackageSlug(t, data, "dup-attachment-copy")

	result, err := e.svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	require.NoError(t, err)
	require.Empty(t, result.Imported)
	require.Len(t, result.Failed, 1)
	assert.Contains(t, result.Failed[0].Error, "附件")
	missing, err := e.repo.FindBySlug(ctx, "dup-attachment-copy")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestImportKapOJAttachmentBudget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("budget-attachment"))
	_, err := e.svc.AddAttachment(ctx, p.ID, 1, problem.AttachmentUpload{
		Filename: "big.bin", Reader: bytes.NewReader(bytes.Repeat([]byte("x"), 5000)),
	})
	require.NoError(t, err)
	var exported bytes.Buffer
	_, err = e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := renamePackageSlug(t, exported.Bytes(), "budget-attachment-copy")

	limited := problem.NewService(e.repo, e.store, problem.WithHydroUncompressedLimit(1500))
	_, err = limited.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	assertCode(t, err, 40001)
	missing, err := e.repo.FindBySlug(ctx, "budget-attachment-copy")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestImportKapOJOverwriteStorageFailurePreservesOriginal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := input("overwrite-safe")
	in.Type = problem.TypeSpecial
	in.StatementMD = "旧题面"
	p := e.create(t, in)
	e.addCase(t, p.ID)
	_, err := e.svc.AddAttachment(ctx, p.ID, 1, problem.AttachmentUpload{
		Filename: "old.txt", ContentType: "text/plain", Reader: strings.NewReader("old attachment"),
	})
	require.NoError(t, err)
	require.NoError(t, e.svc.UploadChecker(ctx, p.ID, strings.NewReader("old checker")))
	require.NoError(t, e.db.Model(&problem.Problem{}).Where("id = ?", p.ID).Updates(map[string]any{
		"solution_md":          "old solution",
		"solution_code":        "old code",
		"solution_price_coins": 5,
	}).Error)

	var exported bytes.Buffer
	_, err = e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)

	fs := &failingStore{Storage: e.store, failOnPut: 1}
	svc := problem.NewService(e.repo, fs)
	result, err := svc.ImportPackage(ctx, 1, bytes.NewReader(exported.Bytes()),
		int64(exported.Len()), problem.PackageImportOptions{ConflictPolicy: "overwrite"})
	require.NoError(t, err)
	require.Empty(t, result.Imported)
	require.Len(t, result.Failed, 1)

	got, err := e.repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "旧题面", got.StatementMD)
	assert.Equal(t, "old solution", got.SolutionMD)
	assert.Equal(t, "old code", got.SolutionCode)
	assert.EqualValues(t, 5, got.SolutionPriceCoins)

	cases, err := e.repo.ListTestcases(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	content, err := e.svc.GetTestcaseContent(ctx, p.ID, cases[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "3", content.Output)

	attachments, err := e.repo.ListAttachments(ctx, p.ID, problem.AttachmentKindFile)
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	assert.Equal(t, "old.txt", attachments[0].Filename)

	checker, err := e.store.Open(ctx, got.CheckerPath)
	require.NoError(t, err)
	checkerBody, err := io.ReadAll(checker)
	require.NoError(t, err)
	require.NoError(t, checker.Close())
	assert.Equal(t, "old checker", string(checkerBody))
}

type failingFindRepo struct {
	problem.Repository
	err error
}

func (r failingFindRepo) FindBySlug(context.Context, string) (*problem.Problem, error) {
	return nil, r.err
}

func TestImportPackageInternalErrorIsNotValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.create(t, input("internal-import"))
	var exported bytes.Buffer
	_, err := e.svc.ExportProblem(ctx, p.ID, &exported)
	require.NoError(t, err)
	data := renamePackageSlug(t, exported.Bytes(), "internal-import-copy")

	svc := problem.NewService(failingFindRepo{
		Repository: e.repo, err: errors.New("database unavailable"),
	}, e.store)
	_, err = svc.ImportPackage(ctx, 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{ConflictPolicy: "skip"})
	var appErr *httpx.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, 50000, appErr.Code)
}

func TestImportPackageHonorsRequestZipLimit(t *testing.T) {
	e := newEnv(t)
	data := buildZip(t, hydroProblem("7992", "包限制题", 1)...)
	_, err := e.svc.ImportPackage(context.Background(), 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{MaxZipBytes: int64(len(data)) - 1})
	requireImportErrorCode(t, err, 40001)
}

func TestImportPackageHonorsRequestUncompressedLimit(t *testing.T) {
	e := newEnv(t)
	data := buildZip(t, hydroProblem("7995", "包解压限制题", 1)...)
	_, err := e.svc.ImportPackage(context.Background(), 1, bytes.NewReader(data), int64(len(data)),
		problem.PackageImportOptions{MaxUncompressedBytes: 10})
	requireImportErrorCode(t, err, 40001)
}
