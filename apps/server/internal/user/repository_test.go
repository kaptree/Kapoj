package user_test

import (
	"context"
	"fmt"
	"sync"
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

func TestUpdateProfileDoesNotOverwriteOtherColumns(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()
	u := newUser("frank")
	require.NoError(t, repo.Create(ctx, u))

	// An admin changes coins and status after the profile request read the row.
	admin, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	admin.Coins, admin.Status = 50, user.StatusBanned
	require.NoError(t, repo.Update(ctx, admin))

	bio := "new bio"
	require.NoError(t, repo.UpdateProfile(ctx, u.ID, user.ProfileChanges{Bio: &bio}))

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "new bio", got.Bio)
	assert.Equal(t, "frank", got.Nickname, "unchanged fields stay")
	assert.Equal(t, int64(50), got.Coins)
	assert.Equal(t, user.StatusBanned, got.Status)
}

func TestUpdateProfileMissingUser(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	bio := "x"
	err := repo.UpdateProfile(context.Background(), 999999, user.ProfileChanges{Bio: &bio})
	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestConcurrentCreateSameUsername(t *testing.T) {
	repo := user.NewGormRepository(testutil.PG(t))
	ctx := context.Background()

	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := newUser("bob")
			u.Email = fmt.Sprintf("bob%d@example.com", i)
			errs <- repo.Create(ctx, u)
		}(i)
	}
	wg.Wait()
	close(errs)

	ok := 0
	for err := range errs {
		if err == nil {
			ok++
			continue
		}
		assert.ErrorIs(t, err, user.ErrUsernameTaken)
	}
	assert.Equal(t, 1, ok)
}
