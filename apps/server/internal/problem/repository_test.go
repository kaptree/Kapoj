package problem_test

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func setup(t *testing.T) (problem.Repository, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	return problem.NewGormRepository(g), g
}

func newProblem(slug string) *problem.Problem {
	return &problem.Problem{
		Title:         "Problem " + slug,
		Slug:          slug,
		StatementMD:   "statement",
		Type:          problem.TypeTraditional,
		Difficulty:    1,
		TimeLimitMS:   1000,
		MemoryLimitMB: 256,
		CreatedBy:     1,
	}
}

func mustCreate(t *testing.T, repo problem.Repository, p *problem.Problem) *problem.Problem {
	t.Helper()
	require.NoError(t, repo.Create(context.Background(), p))
	return p
}

func newCase(name string) *problem.Testcase {
	return &problem.Testcase{
		InputFile: name + ".in", OutputFile: name + ".out",
		InputSize: 1, OutputSize: 1, Score: 10,
	}
}

func slugs(items []problem.Problem) []string {
	out := make([]string, len(items))
	for i, p := range items {
		out[i] = p.Slug
	}
	return out
}

func TestCreateAndFindRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()

	p := newProblem("two-sum")
	p.Tags = []string{"array", "hash"}
	p.Samples = []problem.Sample{{Input: "1 2", Output: "3"}}
	mustCreate(t, repo, p)
	require.NotZero(t, p.ID)

	got, err := repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "two-sum", got.Slug)
	assert.Equal(t, []string{"array", "hash"}, []string(got.Tags))
	assert.Equal(t, []problem.Sample{{Input: "1 2", Output: "3"}}, []problem.Sample(got.Samples))
	assert.False(t, got.IsPublic)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestEmptyTagsAndSamplesAreStoredAsEmptyArrays(t *testing.T) {
	repo, g := setup(t)
	p := mustCreate(t, repo, newProblem("empty"))

	var tags, samples string
	require.NoError(t, g.Raw("SELECT tags_json::text, samples_json::text FROM problems WHERE id = ?", p.ID).
		Row().Scan(&tags, &samples))
	assert.Equal(t, "[]", tags)
	assert.Equal(t, "[]", samples)
}

func TestCreateDuplicateSlugMapsToErr(t *testing.T) {
	repo, _ := setup(t)
	mustCreate(t, repo, newProblem("dup"))
	assert.ErrorIs(t, repo.Create(context.Background(), newProblem("dup")), problem.ErrSlugTaken)
}

func TestFindMissing(t *testing.T) {
	repo, _ := setup(t)
	_, err := repo.FindByID(context.Background(), 999999)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestUpdatePublishChecksTestcasesUnderLock(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("publish-empty"))
	p.IsPublic = true

	err := repo.Update(ctx, p)
	assert.ErrorIs(t, err, problem.ErrPublishNeedsTestcase)

	got, err := repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, got.IsPublic, "a rejected publish must leave the problem hidden")
}

func TestDeleteTestcaseGuardsLastPublicTestcase(t *testing.T) {
	repo, g := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("guarded-delete"))
	tc := newCase("guarded")
	tc.ProblemID = p.ID
	require.NoError(t, repo.AddTestcase(ctx, tc))
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).Update("is_public", true).Error)

	_, err := repo.DeleteTestcase(ctx, p.ID, tc.ID)
	assert.ErrorIs(t, err, problem.ErrLastTestcase)

	n, err := repo.CountTestcases(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestPublishAndDeleteLastTestcaseStayConsistent(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()

	for i := range 10 {
		p := mustCreate(t, repo, newProblem(fmt.Sprintf("race-%d", i)))
		tc := newCase(fmt.Sprintf("race-%d", i))
		tc.ProblemID = p.ID
		require.NoError(t, repo.AddTestcase(ctx, tc))

		publish := *p
		publish.IsPublic = true
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- repo.Update(ctx, &publish)
		}()
		go func() {
			defer wg.Done()
			_, err := repo.DeleteTestcase(ctx, p.ID, tc.ID)
			errs <- err
		}()
		wg.Wait()
		close(errs)

		failed := 0
		for err := range errs {
			if err != nil {
				failed++
			}
		}
		require.Equal(t, 1, failed, "exactly one of publish/delete must lose the race")

		got, err := repo.FindByID(ctx, p.ID)
		require.NoError(t, err)
		n, err := repo.CountTestcases(ctx, p.ID)
		require.NoError(t, err)
		if got.IsPublic {
			assert.GreaterOrEqual(t, n, 1, "a public problem must keep at least one testcase")
		}
	}
}

func TestListFilters(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()

	a := newProblem("two-sum")
	a.Title, a.Tags, a.Difficulty, a.IsPublic = "Two Sum", []string{"array", "hash"}, 1, true
	b := newProblem("graph-paths")
	b.Title, b.Tags, b.Difficulty, b.IsPublic = "Graph Paths", []string{"graph"}, 3, true
	c := newProblem("array-hidden")
	c.Title, c.Tags, c.Difficulty, c.IsPublic = "Array Hidden", []string{"array"}, 1, false
	for _, p := range []*problem.Problem{a, b, c} {
		mustCreate(t, repo, p)
	}

	list := func(f problem.Filter) []string {
		items, _, err := repo.List(ctx, f, 0, 50)
		require.NoError(t, err)
		return slugs(items)
	}

	assert.Equal(t, []string{"two-sum", "graph-paths"}, list(problem.Filter{}), "hidden excluded by default")
	assert.Equal(t, []string{"two-sum", "graph-paths", "array-hidden"}, list(problem.Filter{IncludeHidden: true}))
	assert.Equal(t, []string{"two-sum"}, list(problem.Filter{Keyword: "SUM"}), "case-insensitive title match")
	assert.Equal(t, []string{"graph-paths"}, list(problem.Filter{Keyword: "graph-p"}), "slug match")
	assert.Equal(t, []string{"two-sum"}, list(problem.Filter{Tag: "array"}))
	assert.Equal(t, []string{"two-sum", "array-hidden"}, list(problem.Filter{Tag: "array", IncludeHidden: true}))
	assert.Empty(t, list(problem.Filter{Tag: "arr"}), "tags match whole elements only")
	assert.Equal(t, []string{"graph-paths"}, list(problem.Filter{Difficulty: 3}))
	assert.Equal(t, []string{"two-sum"}, list(problem.Filter{Keyword: "sum", Tag: "hash", Difficulty: 1}))
}

func TestListKeywordEscapesWildcards(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	for i, title := range []string{"100% sure", "a_b", "ab", `back\slash`, "plain"} {
		p := newProblem(fmt.Sprintf("kw-%d", i))
		p.Title, p.IsPublic = title, true
		mustCreate(t, repo, p)
	}

	titles := func(keyword string) []string {
		items, _, err := repo.List(ctx, problem.Filter{Keyword: keyword}, 0, 50)
		require.NoError(t, err)
		out := make([]string, len(items))
		for i, p := range items {
			out[i] = p.Title
		}
		sort.Strings(out)
		return out
	}

	assert.Equal(t, []string{"100% sure"}, titles("%"))
	assert.Equal(t, []string{"a_b"}, titles("a_b"), "underscore must not match any single character")
	assert.Equal(t, []string{`back\slash`}, titles(`\`))
	assert.Equal(t, []string{`back\slash`}, titles(`k\s`))
}

func TestListPaginates(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		p := newProblem(fmt.Sprintf("p%d", i))
		p.IsPublic = true
		mustCreate(t, repo, p)
	}

	items, total, err := repo.List(ctx, problem.Filter{}, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Equal(t, []string{"p2", "p3"}, slugs(items))
}

func TestUpdateKeepsProtectedColumns(t *testing.T) {
	repo, g := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("protected"))
	tc := newCase("protected")
	tc.ProblemID = p.ID
	require.NoError(t, repo.AddTestcase(ctx, tc))

	// Something else changes protected columns after p was read.
	require.NoError(t, g.Exec(
		"UPDATE problems SET checker_path = 'c.cpp', submit_count = 5, accept_count = 2, created_by = 7 WHERE id = ?", p.ID).Error)

	p.Title = "New title"
	p.IsPublic = true
	p.Tags = []string{"x"}
	require.NoError(t, repo.Update(ctx, p))

	got, err := repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "New title", got.Title)
	assert.True(t, got.IsPublic)
	assert.Equal(t, []string{"x"}, []string(got.Tags))
	assert.Equal(t, "c.cpp", got.CheckerPath)
	assert.Equal(t, 5, got.SubmitCount)
	assert.Equal(t, 2, got.AcceptCount)
	assert.Equal(t, int64(7), got.CreatedBy)
}

func TestUpdateMissing(t *testing.T) {
	repo, _ := setup(t)
	p := newProblem("ghost")
	p.ID = 999999
	assert.ErrorIs(t, repo.Update(context.Background(), p), problem.ErrNotFound)
}

func TestUpdateDuplicateSlug(t *testing.T) {
	repo, _ := setup(t)
	mustCreate(t, repo, newProblem("one"))
	two := mustCreate(t, repo, newProblem("two"))
	two.Slug = "one"
	assert.ErrorIs(t, repo.Update(context.Background(), two), problem.ErrSlugTaken)
}

func TestSetCheckerPath(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("chk"))

	require.NoError(t, repo.SetCheckerPath(ctx, p.ID, "problems/1/checker.cpp"))
	got, _ := repo.FindByID(ctx, p.ID)
	assert.Equal(t, "problems/1/checker.cpp", got.CheckerPath)
	assert.ErrorIs(t, repo.SetCheckerPath(ctx, 999999, "x"), problem.ErrNotFound)
}

func TestAddTestcaseAssignsSequentialOrder(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("seq"))

	for i := 1; i <= 3; i++ {
		tc := newCase(fmt.Sprintf("c%d", i))
		tc.ProblemID = p.ID
		require.NoError(t, repo.AddTestcase(ctx, tc))
		assert.Equal(t, i, tc.SortOrder)
		assert.NotZero(t, tc.ID)
	}
	n, err := repo.CountTestcases(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestAddTestcaseMissingProblem(t *testing.T) {
	repo, _ := setup(t)
	tc := newCase("x")
	tc.ProblemID = 999999
	assert.ErrorIs(t, repo.AddTestcase(context.Background(), tc), problem.ErrNotFound)
}

func TestAddTestcaseConcurrentSortOrder(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("conc"))

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tc := newCase(fmt.Sprintf("c%d", i))
			tc.ProblemID = p.ID
			errs <- repo.AddTestcase(ctx, tc)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	cases, err := repo.ListTestcases(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, cases, n)
	for i, tc := range cases {
		assert.Equal(t, i+1, tc.SortOrder)
	}
}

func addCases(t *testing.T, repo problem.Repository, problemID int64, n int) []problem.Testcase {
	t.Helper()
	for i := 1; i <= n; i++ {
		tc := newCase(fmt.Sprintf("p%d-c%d", problemID, i))
		tc.ProblemID = problemID
		require.NoError(t, repo.AddTestcase(context.Background(), tc))
	}
	cases, err := repo.ListTestcases(context.Background(), problemID)
	require.NoError(t, err)
	return cases
}

func ids(cases []problem.Testcase) []int64 {
	out := make([]int64, len(cases))
	for i, c := range cases {
		out[i] = c.ID
	}
	return out
}

func TestReorderTestcases(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("reorder"))
	cases := addCases(t, repo, p.ID, 3)
	a, b, c := cases[0].ID, cases[1].ID, cases[2].ID

	require.NoError(t, repo.ReorderTestcases(ctx, p.ID, []int64{c, a, b}))
	got, err := repo.ListTestcases(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{c, a, b}, ids(got))

	// A plain swap must not trip the unique (problem_id, sort_order) index.
	require.NoError(t, repo.ReorderTestcases(ctx, p.ID, []int64{a, c, b}))
	got, _ = repo.ListTestcases(ctx, p.ID)
	assert.Equal(t, []int64{a, c, b}, ids(got))
}

func TestReorderRejectsBadIDs(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("badorder"))
	other := mustCreate(t, repo, newProblem("other"))
	cases := addCases(t, repo, p.ID, 3)
	foreign := addCases(t, repo, other.ID, 1)
	a, b, c := cases[0].ID, cases[1].ID, cases[2].ID

	for name, order := range map[string][]int64{
		"missing":   {a, b},
		"duplicate": {a, b, b},
		"extra":     {a, b, c, 999999},
		"foreign":   {a, b, foreign[0].ID},
		"empty":     {},
	} {
		assert.ErrorIs(t, repo.ReorderTestcases(ctx, p.ID, order), problem.ErrBadOrder, name)
	}
	got, _ := repo.ListTestcases(ctx, p.ID)
	assert.Equal(t, []int64{a, b, c}, ids(got), "failed reorders change nothing")
}

func TestTestcaseOperationsAreScopedToProblem(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	pa := mustCreate(t, repo, newProblem("scope-a"))
	pb := mustCreate(t, repo, newProblem("scope-b"))
	tc := addCases(t, repo, pa.ID, 1)[0]

	_, err := repo.FindTestcase(ctx, pb.ID, tc.ID)
	assert.ErrorIs(t, err, problem.ErrTestcaseNotFound)
	score := 99
	assert.ErrorIs(t, repo.UpdateTestcase(ctx, pb.ID, tc.ID, &score, nil), problem.ErrTestcaseNotFound)
	_, err = repo.DeleteTestcase(ctx, pb.ID, tc.ID)
	assert.ErrorIs(t, err, problem.ErrTestcaseNotFound)

	still, err := repo.FindTestcase(ctx, pa.ID, tc.ID)
	require.NoError(t, err)
	assert.Equal(t, 10, still.Score)
}

func TestUpdateAndDeleteTestcase(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("tc-ops"))
	tc := addCases(t, repo, p.ID, 2)[0]

	score, sample := 50, true
	require.NoError(t, repo.UpdateTestcase(ctx, p.ID, tc.ID, &score, &sample))
	got, _ := repo.FindTestcase(ctx, p.ID, tc.ID)
	assert.Equal(t, 50, got.Score)
	assert.True(t, got.IsSample)

	only := 7
	require.NoError(t, repo.UpdateTestcase(ctx, p.ID, tc.ID, &only, nil))
	got, _ = repo.FindTestcase(ctx, p.ID, tc.ID)
	assert.Equal(t, 7, got.Score)
	assert.True(t, got.IsSample, "nil fields stay unchanged")

	removed, err := repo.DeleteTestcase(ctx, p.ID, tc.ID)
	require.NoError(t, err)
	assert.Equal(t, tc.InputFile, removed.InputFile)
	n, _ := repo.CountTestcases(ctx, p.ID)
	assert.Equal(t, 1, n)
}

func TestDeleteWithoutSubmissionsCascades(t *testing.T) {
	repo, g := setup(t)
	ctx := context.Background()
	p := mustCreate(t, repo, newProblem("del"))
	require.NoError(t, repo.SetCheckerPath(ctx, p.ID, "problems/x/checker.cpp"))
	cases := addCases(t, repo, p.ID, 2)

	res, err := repo.Delete(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, res.Offlined)

	want := []string{"problems/x/checker.cpp"}
	for _, c := range cases {
		want = append(want, c.InputFile, c.OutputFile)
	}
	assert.ElementsMatch(t, want, res.FileKeys)

	_, err = repo.FindByID(ctx, p.ID)
	assert.ErrorIs(t, err, problem.ErrNotFound)
	var n int64
	require.NoError(t, g.Raw("SELECT count(*) FROM problem_testcases").Scan(&n).Error)
	assert.Zero(t, n, "testcases are removed with the problem")
}

func TestDeleteWithSubmissionsOfflines(t *testing.T) {
	repo, g := setup(t)
	ctx := context.Background()
	p := newProblem("kept")
	p.IsPublic = true
	mustCreate(t, repo, p)
	addCases(t, repo, p.ID, 2)
	require.NoError(t, g.Exec("UPDATE problems SET submit_count = 1 WHERE id = ?", p.ID).Error)

	res, err := repo.Delete(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, res.Offlined)
	assert.Empty(t, res.FileKeys, "files must be kept")

	got, err := repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, got.IsPublic)
	n, _ := repo.CountTestcases(ctx, p.ID)
	assert.Equal(t, 2, n)
}

func TestDeleteMissing(t *testing.T) {
	repo, _ := setup(t)
	_, err := repo.Delete(context.Background(), 999999)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}
