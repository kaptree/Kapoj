package submission_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func TestRepositoryCreateIncrementsSubmitCount(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("repo")
	require.NoError(t, problems.Create(context.Background(), p))
	createUser(t, g, 3)

	sub := &submission.Submission{
		UserID: 3, ProblemID: p.ID, Language: submission.LanguageGo,
		Code: "package main", Status: submission.StatusPending,
	}
	require.NoError(t, repo.Create(context.Background(), sub))
	assert.NotZero(t, sub.ID)

	got, err := repo.FindByID(context.Background(), sub.ID)
	require.NoError(t, err)
	assert.Equal(t, submission.StatusPending, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
	assert.NotNil(t, got.JudgeResult)

	var count int
	require.NoError(t, g.Model(&problem.Problem{}).Where("id = ?", p.ID).
		Select("submit_count").Scan(&count).Error)
	assert.Equal(t, 1, count)
}

func TestRepositoryCreateMissingProblem(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	err := repo.Create(context.Background(), &submission.Submission{
		UserID: 1, ProblemID: 999999, Language: submission.LanguageC,
		Code: "int main(){}", Status: submission.StatusPending,
	})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestRepositoryMarkSystemErrorOnlyMovesOpenSubmissions(t *testing.T) {
	g := testutil.PG(t)
	repo := submission.NewGormRepository(g)
	problems := problem.NewGormRepository(g)
	p := publicProblem("mark")
	require.NoError(t, problems.Create(context.Background(), p))
	createUser(t, g, 1)

	open := &submission.Submission{UserID: 1, ProblemID: p.ID, Language: submission.LanguageC, Code: "x", Status: submission.StatusPending}
	done := &submission.Submission{UserID: 1, ProblemID: p.ID, Language: submission.LanguageC, Code: "x", Status: submission.StatusAccepted}
	require.NoError(t, repo.Create(context.Background(), open))
	require.NoError(t, repo.Create(context.Background(), done))

	require.NoError(t, repo.MarkSystemError(context.Background(), open.ID, "boom"))
	require.NoError(t, repo.MarkSystemError(context.Background(), done.ID, "boom"))

	gotOpen, _ := repo.FindByID(context.Background(), open.ID)
	assert.Equal(t, submission.StatusSystemError, gotOpen.Status)
	assert.Equal(t, "boom", gotOpen.ErrorMessage)
	assert.NotNil(t, gotOpen.JudgedAt)

	gotDone, _ := repo.FindByID(context.Background(), done.ID)
	assert.Equal(t, submission.StatusAccepted, gotDone.Status)
	assert.Empty(t, gotDone.ErrorMessage)
}
