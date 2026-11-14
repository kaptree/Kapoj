package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/db"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func TestMigrateBackfillsSubmissionFieldsOnce(t *testing.T) {
	g := testutil.PG(t) // 已 Migrate 并清空所有表（包括 db_migrations）

	u := &user.User{Username: "legacy-code", Nickname: "旧提交", Email: "legacy-code@example.com", PasswordHash: "x"}
	require.NoError(t, g.Create(u).Error)
	p := &problem.Problem{Title: "旧题", Slug: "legacy-submission"}
	require.NoError(t, g.Create(p).Error)

	legacy := &submission.Submission{
		UserID: u.ID, ProblemID: p.ID, Language: submission.LanguageCPP,
		Code: "你好", Status: submission.StatusAccepted,
	}
	require.NoError(t, g.Create(legacy).Error)

	// 模拟首次升级：标记不存在，Migrate 应回填代码长度与 AC 计数标记。
	require.NoError(t, db.Migrate(g))
	var got submission.Submission
	require.NoError(t, g.First(&got, legacy.ID).Error)
	assert.Equal(t, len("你好"), got.CodeLength, "code_length uses UTF-8 bytes")
	assert.True(t, got.AcceptedCounted)

	// 回填只执行一次；之后的新提交不会被再次标记或改写。
	fresh := &submission.Submission{
		UserID: u.ID, ProblemID: p.ID, Language: submission.LanguageCPP,
		Code: "ab", Status: submission.StatusAccepted,
	}
	require.NoError(t, g.Create(fresh).Error)
	require.NoError(t, db.Migrate(g))
	got = submission.Submission{}
	require.NoError(t, g.First(&got, fresh.ID).Error)
	assert.Zero(t, got.CodeLength)
	assert.False(t, got.AcceptedCounted)
}
