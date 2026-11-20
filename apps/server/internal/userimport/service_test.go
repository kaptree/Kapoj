package userimport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/password"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func seedUser(t *testing.T, g *gorm.DB, username, email, hash, salt string) *user.User {
	t.Helper()
	u := &user.User{
		Username: username, Nickname: username, Email: email,
		PasswordHash: hash, Salt: salt, Role: user.RoleUser, Status: user.StatusActive,
	}
	require.NoError(t, g.Create(u).Error)
	return u
}

func mustUser(t *testing.T, g *gorm.DB, username string) *user.User {
	t.Helper()
	var u user.User
	require.NoError(t, g.Where("username = ?", username).First(&u).Error)
	return &u
}

func countUsers(t *testing.T, g *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, g.Model(&user.User{}).Count(&count).Error)
	return count
}

func TestProcessPreviewAndImport(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := csvText(
		"2,Alice,Alice@Example.com,"+hash+","+salt+",hydro",
		"6,让我睡觉,me@example.com,"+hash+","+salt+",hydro",
	)

	preview, err := svc.Process(context.Background(), strings.NewReader(csv), true, nil)
	require.NoError(t, err)
	assert.True(t, preview.DryRun)
	assert.Equal(t, 2, preview.Summary.Total)
	assert.Equal(t, 2, preview.Summary.Valid)
	assert.Equal(t, 0, preview.Summary.Conflicts)
	assert.Equal(t, 1, preview.Summary.GeneratedUsernames)
	require.Len(t, preview.Mapping, 1)
	assert.Equal(t, "hydro_6", preview.Mapping[0].Username)
	assert.Zero(t, countUsers(t, g), "preview must not write")

	result, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.False(t, result.DryRun)
	assert.Equal(t, 2, result.Summary.Imported)
	assert.Zero(t, result.Summary.Skipped)

	alice := mustUser(t, g, "alice")
	assert.Equal(t, "Alice", alice.Nickname)
	assert.NotNil(t, alice.EmailVerifiedAt)
	assert.Equal(t, user.InitialRating, alice.Rating)
	assert.Zero(t, alice.Coins)
	assert.Equal(t, user.RoleUser, alice.Role)
	assert.Equal(t, user.StatusActive, alice.Status)
	assert.True(t, password.Verify("old-password", alice.Salt, alice.PasswordHash))
	assert.Equal(t, "hydro_6", mustUser(t, g, "hydro_6").Username)
}

func TestProcessSkipsExistingUser(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	currentHash, currentSalt, err := password.Hash("current-password")
	require.NoError(t, err)
	seedUser(t, g, "kaptree", "current@example.com", currentHash, currentSalt)

	newHash, newSalt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := csvText(
		"2,Kaptree,kaptree@example.com,"+newHash+","+newSalt+",hydro",
		"3,Bob,bob@example.com,"+newHash+","+newSalt+",hydro",
	)

	preview, err := svc.Process(context.Background(), strings.NewReader(csv), true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.Summary.Conflicts)
	assert.Equal(t, 1, preview.Summary.Valid)
	require.Len(t, preview.Issues, 1)
	assert.Equal(t, "conflict", preview.Issues[0].Status)
	assert.Contains(t, preview.Issues[0].Reason, "用户名")

	result, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Summary.Imported)
	assert.Equal(t, 1, result.Summary.Skipped)
	assert.Equal(t, int64(2), countUsers(t, g))

	existing := mustUser(t, g, "kaptree")
	assert.Equal(t, currentHash, existing.PasswordHash)
	assert.Equal(t, currentSalt, existing.Salt)
	assert.Equal(t, "current@example.com", existing.Email)
	assert.Nil(t, existing.EmailVerifiedAt)
}

func TestProcessGeneratedNameCollision(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	seedUser(t, g, "hydro_6", "six@example.com", hash, salt)

	csv := csvText("6,让我睡觉,me@example.com," + hash + "," + salt + ",hydro")
	result, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Summary.Imported)
	require.Len(t, result.Mapping, 1)
	assert.Equal(t, "hydro_6_2", result.Mapping[0].Username)
	assert.Equal(t, "让我睡觉", mustUser(t, g, "hydro_6_2").Nickname)
}

func TestProcessDuplicateRowsAllInvalid(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := csvText(
		"2,Alice,a@example.com,"+hash+","+salt+",hydro",
		"3,Bob,a@example.com,"+hash+","+salt+",hydro",
	)

	preview, err := svc.Process(context.Background(), strings.NewReader(csv), true, nil)
	require.NoError(t, err)
	assert.Zero(t, preview.Summary.Valid)
	assert.Equal(t, 2, preview.Summary.Invalid)
	require.Len(t, preview.Issues, 2)
	assert.Contains(t, preview.Issues[0].Reason, "重复")

	_, err = svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.Zero(t, countUsers(t, g))
}

func TestProcessEmailConflict(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	seedUser(t, g, "someone", "a@example.com", hash, salt)

	preview, err := svc.Process(context.Background(), strings.NewReader(
		csvText("5,Alice,a@example.com,"+hash+","+salt+",hydro"),
	), true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.Summary.Conflicts)
	require.Len(t, preview.Issues, 1)
	assert.Contains(t, preview.Issues[0].Reason, "邮箱")
}

func TestProcessSecondImportIsIdempotent(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := csvText(
		"2,Alice,a@example.com,"+hash+","+salt+",hydro",
		"6,让我睡觉,me@example.com,"+hash+","+salt+",hydro",
	)

	first, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, first.Summary.Imported)

	second, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	assert.Zero(t, second.Summary.Imported)
	assert.Equal(t, 2, second.Summary.Skipped)
	assert.Equal(t, int64(2), countUsers(t, g))
}

func TestProcessAuditFailureRollsBack(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	csv := csvText("2,Alice,a@example.com," + hash + "," + salt + ",hydro")

	_, err = svc.Process(context.Background(), strings.NewReader(csv), false,
		func(*gorm.DB, Summary) error { return errors.New("audit failed") })
	require.Error(t, err)
	assert.Zero(t, countUsers(t, g))
}

func TestProcessMappingOnlyImportedGeneratedRows(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	seedUser(t, g, "blocked", "blocked@example.com", hash, salt)
	csv := csvText(
		"6,让我睡觉,blocked@example.com,"+hash+","+salt+",hydro",
		"7,小助手,ok@example.com,"+hash+","+salt+",hydro",
	)

	preview, err := svc.Process(context.Background(), strings.NewReader(csv), true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.Summary.Conflicts)
	require.Len(t, preview.Mapping, 1)
	assert.Equal(t, "hydro_7", preview.Mapping[0].Username)

	result, err := svc.Process(context.Background(), strings.NewReader(csv), false, nil)
	require.NoError(t, err)
	require.Len(t, result.Mapping, 1)
	assert.Equal(t, "hydro_7", result.Mapping[0].Username)
}

func TestProcessIssuesAreCapped(t *testing.T) {
	g := testutil.PG(t)
	svc := NewService(g)
	hash, salt, err := password.Hash("old-password")
	require.NoError(t, err)
	var b strings.Builder
	b.WriteString("_id,uname,mail,hash,salt,hashType\n")
	for i := 1; i <= 501; i++ {
		fmt.Fprintf(&b, "%d,u%d,not-an-email-%d,%s,%s,hydro\n", i, i, i, hash, salt)
	}

	preview, err := svc.Process(context.Background(), strings.NewReader(b.String()), true, nil)
	require.NoError(t, err)
	assert.Equal(t, 501, preview.Summary.Invalid)
	assert.Len(t, preview.Issues, 500)
	assert.True(t, preview.IssuesTruncated)
}
