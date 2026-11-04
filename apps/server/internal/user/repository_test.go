package user_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func newUser(name string) *user.User {
	return &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
}

func TestCreateAndFind(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()

	u := newUser("alice")
	require.NoError(t, repo.Create(ctx, u))
	require.NotZero(t, u.ID)

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, 1500, got.Rating)
	assert.Equal(t, int64(0), got.Coins)
	assert.False(t, got.CreatedAt.IsZero())

	byName, err := repo.FindByUsername(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, u.ID, byName.ID)

	byEmail, err := repo.FindByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, byEmail.ID)
}

func TestCreateDuplicateUsernameMapsToErr(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newUser("bob")))

	dup := newUser("bob")
	dup.Email = "other@example.com"
	assert.ErrorIs(t, repo.Create(ctx, dup), user.ErrUsernameTaken)
}

func TestCreateDuplicateEmailMapsToErr(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newUser("carol")))

	dup := newUser("carol2")
	dup.Email = "carol@example.com"
	assert.ErrorIs(t, repo.Create(ctx, dup), user.ErrEmailTaken)
}

func TestFindMissingReturnsErrNotFound(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()

	_, err := repo.FindByID(ctx, 999999)
	assert.ErrorIs(t, err, user.ErrNotFound)
	_, err = repo.FindByUsername(ctx, "nobody")
	assert.ErrorIs(t, err, user.ErrNotFound)
	_, err = repo.FindByEmail(ctx, "nobody@example.com")
	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestUpdatePersistsFields(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	u := newUser("dave")
	require.NoError(t, repo.Create(ctx, u))

	u.Bio = "hello"
	require.NoError(t, repo.Update(ctx, u))
	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Bio)
}

func TestCoinsCannotBeNegative(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	u := newUser("erin")
	require.NoError(t, repo.Create(ctx, u))

	u.Coins = -1
	assert.Error(t, repo.Update(ctx, u))
}

func TestListPaginates(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Create(ctx, newUser(fmt.Sprintf("user%d", i))))
	}

	items, total, err := repo.List(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	require.Len(t, items, 2)
	assert.Equal(t, "user2", items[0].Username)
	assert.Equal(t, "user3", items[1].Username)
}
