package storageaudit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/storageaudit"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestBadgeKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
		ok    bool
	}{
		{name: "internal", input: "/api/badges/13/image", key: "badges/13", ok: true},
		{name: "external", input: "https://example.com/badge.png"},
		{name: "missing id", input: "/api/badges//image"},
		{name: "leading zero", input: "/api/badges/013/image"},
		{name: "extra path", input: "/api/badges/13/other/image"},
		{name: "empty", input: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, ok := storageaudit.BadgeKey(test.input)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.key, key)
		})
	}
}

func TestMissingReferences(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()
	_, err = store.Put(ctx, "badges/1", strings.NewReader("present"), 100)
	require.NoError(t, err)

	missing, err := storageaudit.MissingReferences(ctx, store, []storageaudit.Reference{
		{Kind: "badge", ID: 1, Key: "badges/1"},
		{Kind: "badge", ID: 2, Key: "badges/2"},
		{Kind: "avatar", ID: 1, Key: "avatars/1/photo"},
	})
	require.NoError(t, err)
	assert.Equal(t, []storageaudit.Missing{
		{Kind: "avatar", ID: 1, Key: "avatars/1/photo"},
		{Kind: "badge", ID: 2, Key: "badges/2"},
	}, missing)
}

func TestCollectFindsBadgeAndAvatarReferences(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()

	badge := &gamification.Badge{Name: "missing image", IsActive: true}
	require.NoError(t, g.Create(badge).Error)
	badge.ImageURL = fmt.Sprintf("/api/badges/%d/image", badge.ID)
	require.NoError(t, g.Model(badge).Update("image_url", badge.ImageURL).Error)

	avatarUser := &user.User{
		Username: "audit-avatar", Nickname: "audit-avatar",
		Email: "audit-avatar@example.com", PasswordHash: "x",
		AvatarKey: "avatars/1/photo",
	}
	require.NoError(t, g.Create(avatarUser).Error)

	problemRow := &problem.Problem{
		Title: "audit-problem", Slug: "audit-problem", Type: problem.TypeTraditional,
		Difficulty: 1, TimeLimitMS: 1000, MemoryLimitMB: 256,
		CheckerPath: "problems/1/checker.cpp", Answers: dbtypes.List[string]{},
		CreatedBy: avatarUser.ID,
	}
	require.NoError(t, g.Create(problemRow).Error)
	testcase := &problem.Testcase{
		ProblemID: problemRow.ID, InputFile: "problems/1/tc.in",
		OutputFile: "problems/1/tc.out", SortOrder: 1,
	}
	require.NoError(t, g.Create(testcase).Error)

	refs, err := storageaudit.Collect(ctx, g)
	require.NoError(t, err)
	assert.Contains(t, refs, storageaudit.Reference{
		Kind: "badge", ID: badge.ID, Key: fmt.Sprintf("badges/%d", badge.ID),
	})
	assert.Contains(t, refs, storageaudit.Reference{
		Kind: "avatar", ID: avatarUser.ID, Key: "avatars/1/photo",
	})
	assert.Contains(t, refs, storageaudit.Reference{
		Kind: "problem-checker", ID: problemRow.ID, Key: "problems/1/checker.cpp",
	})
	assert.Contains(t, refs, storageaudit.Reference{
		Kind: "problem-testcase-input", ID: testcase.ID, Key: "problems/1/tc.in",
	})
	assert.Contains(t, refs, storageaudit.Reference{
		Kind: "problem-testcase-output", ID: testcase.ID, Key: "problems/1/tc.out",
	})
}
