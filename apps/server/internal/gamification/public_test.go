package gamification_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/dbtypes"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

func newPublicEnv(t *testing.T) (*gamification.Service, *gorm.DB) {
	t.Helper()
	g := testutil.PG(t)
	return gamification.NewService(gamification.NewRepository(g)), g
}

func publicUser(t *testing.T, g *gorm.DB, name string) *user.User {
	t.Helper()
	u := &user.User{
		Username:     name,
		Nickname:     name,
		Email:        name + "@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	return u
}

func publicBadge(t *testing.T, g *gorm.DB, name string, sortOrder int, active bool) *gamification.Badge {
	t.Helper()
	b := &gamification.Badge{
		Name:      name,
		Category:  "测试",
		ImageURL:  "https://example.com/badge.png",
		SortOrder: sortOrder,
		IsActive:  active,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, g.Create(b).Error)
	return b
}

func equipBadge(t *testing.T, g *gorm.DB, userID, badgeID int64) {
	t.Helper()
	require.NoError(t, g.Create(&gamification.UserBadge{
		UserID: userID, BadgeID: badgeID, IsEquipped: true, PurchasedAt: time.Now(),
	}).Error)
}

func publicTitle(t *testing.T, g *gorm.DB, name, color string, sortOrder int, active bool) *gamification.Title {
	t.Helper()
	title := &gamification.Title{
		Name:                name,
		Color:               color,
		UnlockConditionType: string(gamification.TitleUnlockACCount),
		UnlockConditionConfig: dbtypes.JSON[map[string]any]{
			Data: map[string]any{"threshold": 1},
		},
		SortOrder: sortOrder,
		IsActive:  active,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, g.Create(title).Error)
	return title
}

func equipTitle(t *testing.T, g *gorm.DB, userID, titleID int64) {
	t.Helper()
	require.NoError(t, g.Create(&gamification.UserTitle{
		UserID: userID, TitleID: titleID, IsEquipped: true, UnlockedAt: time.Now(),
	}).Error)
}

func TestIdentitiesBatch(t *testing.T) {
	svc, g := newPublicEnv(t)
	ctx := context.Background()

	first := publicUser(t, g, "identity-first")
	second := publicUser(t, g, "identity-second")
	third := publicUser(t, g, "identity-third")

	b1 := publicBadge(t, g, "徽章一", 2, true)
	b2 := publicBadge(t, g, "徽章二", 1, true)
	equipBadge(t, g, first.ID, b1.ID)
	equipBadge(t, g, first.ID, b2.ID)
	equipBadge(t, g, second.ID, b1.ID)

	title := publicTitle(t, g, "卷王", "#7C3AED", 1, true)
	equipTitle(t, g, first.ID, title.ID)

	got, err := svc.Identities(ctx, []int64{first.ID, second.ID, third.ID, first.ID, 0, -1})
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Contains(t, got, first.ID)
	require.Contains(t, got, second.ID)
	require.Contains(t, got, third.ID)

	require.Len(t, got[first.ID].Badges, 2)
	assert.Equal(t, []gamification.BadgeSummary{
		{ID: b2.ID, Name: "徽章二", ImageURL: "https://example.com/badge.png"},
		{ID: b1.ID, Name: "徽章一", ImageURL: "https://example.com/badge.png"},
	}, got[first.ID].Badges)
	require.NotNil(t, got[first.ID].Title)
	assert.Equal(t, gamification.TitleSummary{ID: title.ID, Name: "卷王", Color: "#7C3AED"}, *got[first.ID].Title)

	require.Len(t, got[second.ID].Badges, 1)
	assert.Equal(t, b1.ID, got[second.ID].Badges[0].ID)
	assert.Nil(t, got[second.ID].Title)

	assert.NotNil(t, got[third.ID].Badges, "无装备用户的 Badges 必须是空切片而非 nil")
	assert.Empty(t, got[third.ID].Badges)
	assert.Nil(t, got[third.ID].Title)

	raw, err := json.Marshal(got[third.ID])
	require.NoError(t, err)
	assert.JSONEq(t, `{"equippedBadges":[],"equippedTitle":null}`, string(raw))
}

func TestIdentitiesOrderAndInactive(t *testing.T) {
	svc, g := newPublicEnv(t)
	ctx := context.Background()

	userWithBadges := publicUser(t, g, "identity-order")
	userWithTitle := publicUser(t, g, "identity-title")

	late := publicBadge(t, g, "后置徽章", 2, true)
	earlyLow := publicBadge(t, g, "前置徽章甲", 1, true)
	earlyHigh := publicBadge(t, g, "前置徽章乙", 1, true)
	equipBadge(t, g, userWithBadges.ID, late.ID)
	equipBadge(t, g, userWithBadges.ID, earlyHigh.ID)
	equipBadge(t, g, userWithBadges.ID, earlyLow.ID)

	// 装备后下线，仍应出现在公开身份中。
	require.NoError(t, g.Model(&gamification.Badge{}).Where("id = ?", late.ID).
		Update("is_active", false).Error)
	require.NoError(t, g.Model(&gamification.Badge{}).Where("id = ?", earlyLow.ID).
		Update("is_active", false).Error)

	activeTitle := publicTitle(t, g, "上架称号", "#112233", 1, true)
	inactiveTitle := publicTitle(t, g, "下线称号", "#445566", 2, false)
	equipTitle(t, g, userWithBadges.ID, activeTitle.ID)
	equipTitle(t, g, userWithBadges.ID, inactiveTitle.ID)

	// 只有已下线的称号被装备时同样展示。
	equipTitle(t, g, userWithTitle.ID, inactiveTitle.ID)

	got, err := svc.Identities(ctx, []int64{userWithBadges.ID, userWithTitle.ID})
	require.NoError(t, err)

	require.Len(t, got[userWithBadges.ID].Badges, 3)
	assert.Equal(t, []int64{earlyLow.ID, earlyHigh.ID, late.ID},
		[]int64{
			got[userWithBadges.ID].Badges[0].ID,
			got[userWithBadges.ID].Badges[1].ID,
			got[userWithBadges.ID].Badges[2].ID,
		}, "徽章按 sort_order, id 排序")

	require.NotNil(t, got[userWithBadges.ID].Title)
	assert.Equal(t, activeTitle.ID, got[userWithBadges.ID].Title.ID,
		"同一用户只返回一个称号，按 sort_order, id 取第一个")

	require.NotNil(t, got[userWithTitle.ID].Title)
	assert.Equal(t, inactiveTitle.ID, got[userWithTitle.ID].Title.ID,
		"已装备但下线的称号仍展示")
	assert.NotNil(t, got[userWithTitle.ID].Badges)
	assert.Empty(t, got[userWithTitle.ID].Badges)
}
