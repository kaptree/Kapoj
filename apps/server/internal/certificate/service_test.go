package certificate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type env struct {
	svc *certificate.Service
	db  *gorm.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	return &env{
		svc: certificate.NewService(certificate.NewGormRepository(g), store),
		db:  g,
	}
}

func requireAppErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var appErr *httpx.AppError
	require.True(t, errors.As(err, &appErr), "应为 AppError，实际为 %T: %v", err, err)
	assert.Equal(t, code, appErr.Code, appErr.Message)
}

func boolPtr(b bool) *bool { return &b }

func TestSeedTypesOnlyWhenEmpty(t *testing.T) {
	g := testutil.PG(t)
	ctx := context.Background()

	require.NoError(t, certificate.SeedTypes(ctx, g))

	var names []string
	require.NoError(t, g.Model(&certificate.CertificateType{}).Order("sort_order").Pluck("name", &names).Error)
	require.Equal(t, []string{"蓝桥杯", "ACM-ICPC", "CCCC 天梯赛", "CCPC", "NOI", "NOIP", "USACO"}, names)

	var count int64
	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)

	// 再次 seed 不重复。
	require.NoError(t, certificate.SeedTypes(ctx, g))
	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)

	// 已有自定义类型时 seed 不覆盖管理员修改：删除一条默认类型、新增自定义类型后
	// 再次 seed 既不恢复默认也不重复插入。
	custom := &certificate.CertificateType{Name: "自定义竞赛", IsActive: true, SortOrder: 8}
	require.NoError(t, g.Create(custom).Error)
	require.NoError(t, g.Where("name = ?", "USACO").Delete(&certificate.CertificateType{}).Error)
	require.NoError(t, certificate.SeedTypes(ctx, g))

	require.NoError(t, g.Model(&certificate.CertificateType{}).Count(&count).Error)
	assert.Equal(t, int64(7), count)
	require.NoError(t, g.Model(&certificate.CertificateType{}).Order("sort_order").Pluck("name", &names).Error)
	assert.NotContains(t, names, "USACO")
	assert.Contains(t, names, "自定义竞赛")
}

func TestListActiveAndAllTypes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))

	// 停用一个类型。
	require.NoError(t, e.db.Model(&certificate.CertificateType{}).
		Where("name = ?", "NOIP").Update("is_active", false).Error)

	active, err := e.svc.ListActiveTypes(ctx)
	require.NoError(t, err)
	require.Len(t, active, 6)
	for _, v := range active {
		assert.True(t, v.IsActive)
	}
	// 公开列表按 sort_order 升序。
	assert.Equal(t, "蓝桥杯", active[0].Name)
	assert.Equal(t, "USACO", active[len(active)-1].Name)

	all, err := e.svc.ListAllTypes(ctx)
	require.NoError(t, err)
	require.Len(t, all, 7)
	foundInactive := false
	for _, v := range all {
		if !v.IsActive {
			foundInactive = true
			assert.Equal(t, "NOIP", v.Name)
		}
	}
	assert.True(t, foundInactive, "includeInactive 应返回停用类型")
}

func TestCreateUpdateDeleteType(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// 合法创建；isActive 缺省 true，字符串两侧空白被去除。
	created, err := e.svc.CreateType(ctx, certificate.TypeInput{
		Name: " 蓝桥杯 ", Description: " 算法竞赛 ", SortOrder: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "蓝桥杯", created.Name)
	assert.Equal(t, "算法竞赛", created.Description)
	assert.True(t, created.IsActive)
	assert.Equal(t, 1, created.SortOrder)
	assert.Positive(t, created.ID)

	// 重复名称 40001。
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "蓝桥杯", SortOrder: 2})
	requireAppErrorCode(t, err, 40001)

	// 字段校验：空名称、超长名称、超长描述、排序越界。
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "  ", SortOrder: 1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: strings.Repeat("赛", 51), SortOrder: 1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{
		Name: "X", Description: strings.Repeat("长", 201), SortOrder: 1,
	})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "X", SortOrder: -1})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.CreateType(ctx, certificate.TypeInput{Name: "X", SortOrder: 10001})
	requireAppErrorCode(t, err, 40001)

	// 更新：isActive 缺省保留原值。
	updated, err := e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{
		Name: "蓝桥杯（改）", Description: "更新描述", SortOrder: 9,
	})
	require.NoError(t, err)
	assert.Equal(t, "蓝桥杯（改）", updated.Name)
	assert.True(t, updated.IsActive, "isActive 缺省应保留原值")

	// 更新：显式停用。
	updated, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{
		Name: "蓝桥杯（改）", SortOrder: 9, IsActive: boolPtr(false),
	})
	require.NoError(t, err)
	assert.False(t, updated.IsActive)

	// 更新为已存在的名称 40001。
	other, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "另一个", SortOrder: 3})
	require.NoError(t, err)
	_, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{Name: "另一个", SortOrder: 9})
	requireAppErrorCode(t, err, 40001)
	assert.Positive(t, other.ID)

	// 更新不存在的类型 404。
	_, err = e.svc.UpdateType(ctx, 999999, certificate.TypeInput{Name: "X", SortOrder: 1})
	requireAppErrorCode(t, err, 40400)

	// 删除：无引用可删，删后读取 404。
	require.NoError(t, e.svc.DeleteType(ctx, created.ID))
	_, err = e.svc.UpdateType(ctx, created.ID, certificate.TypeInput{Name: "X", SortOrder: 1})
	requireAppErrorCode(t, err, 40400)
}

func TestDeleteTypeInUse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	typ, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "NOI", SortOrder: 5})
	require.NoError(t, err)

	// 插入引用该类型的证书。
	require.NoError(t, e.db.Create(&certificate.Certificate{
		UserID:              1,
		CertificateTypeID:   typ.ID,
		CertificateTypeName: "NOI",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}).Error)

	// 被引用时删除返回 40001 且带中文提示。
	err = e.svc.DeleteType(ctx, typ.ID)
	requireAppErrorCode(t, err, 40001)
	assert.Contains(t, err.Error(), "已有证书使用该类型，请先停用")

	// 删除不存在的类型 404。
	requireAppErrorCode(t, e.svc.DeleteType(ctx, 999999), 40400)

	// 无引用的类型可删除。
	free, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "USACO", SortOrder: 7})
	require.NoError(t, err)
	require.NoError(t, e.svc.DeleteType(ctx, free.ID))
}

func TestTypeRenameKeepsCertificateSnapshot(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	typ, err := e.svc.CreateType(ctx, certificate.TypeInput{Name: "蓝桥杯", SortOrder: 1})
	require.NoError(t, err)

	cert := &certificate.Certificate{
		UserID:              1,
		CertificateTypeID:   typ.ID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}
	require.NoError(t, e.db.Create(cert).Error)

	// 类型改名后，历史证书仍使用提交时快照的名称。
	_, err = e.svc.UpdateType(ctx, typ.ID, certificate.TypeInput{Name: "蓝桥杯（改名）", SortOrder: 1})
	require.NoError(t, err)

	var got certificate.Certificate
	require.NoError(t, e.db.First(&got, cert.ID).Error)
	assert.Equal(t, "蓝桥杯", got.CertificateTypeName)
}

// testPNG 以合法 PNG magic bytes 开头，http.DetectContentType 会识别为 image/png。
var testPNG = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}

func createUser(t *testing.T, g *gorm.DB, username, nickname string) int64 {
	t.Helper()
	u := &user.User{
		Username:     username,
		Nickname:     nickname,
		Email:        username + "@example.com",
		PasswordHash: "x",
	}
	require.NoError(t, g.Create(u).Error)
	return u.ID
}

func firstActiveTypeID(t *testing.T, e *env) int64 {
	t.Helper()
	active, err := e.svc.ListActiveTypes(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, active)
	return active[0].ID
}

func TestSubmitCertificate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	userID := createUser(t, e.db, "submit-user", "提交用户")

	v, err := e.svc.Submit(ctx, userID, typeID, " 省一 ", bytes.NewReader(testPNG))
	require.NoError(t, err)
	assert.Equal(t, certificate.StatusPending, v.Status)
	assert.Equal(t, "蓝桥杯", v.Type)
	assert.Equal(t, "省一", v.Level, "level 应去除两侧空白")
	assert.Equal(t, fmt.Sprintf("/api/certificates/%d/image", v.ID), v.ImageURL)
	assert.Empty(t, v.VerifyCode)

	var cert certificate.Certificate
	require.NoError(t, e.db.First(&cert, v.ID).Error)
	assert.Equal(t, "蓝桥杯", cert.CertificateTypeName)
	assert.Equal(t, fmt.Sprintf("certificates/%d", cert.ID), cert.ImageKey)
	assert.Equal(t, "image/png", cert.ImageContentType)
	assert.Equal(t, certificate.StatusPending, cert.Status)
	assert.Equal(t, userID, cert.UserID)

	// 我的列表可见。
	page, err := e.svc.ListMine(ctx, userID, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, v.ID, page.Items[0].ID)

	// owner 可读取图片字节与内容类型。
	img, err := e.svc.Image(ctx, v.ID, userID, false)
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.ContentType)
	assert.Equal(t, testPNG, img.Data)
}

func TestSubmitValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	userID := createUser(t, e.db, "val-user", "校验用户")

	// 停用类型 40001。
	require.NoError(t, e.db.Model(&certificate.CertificateType{}).
		Where("id = ?", typeID).Update("is_active", false).Error)
	_, err := e.svc.Submit(ctx, userID, typeID, "省一", bytes.NewReader(testPNG))
	requireAppErrorCode(t, err, 40001)

	// 类型不存在 404。
	_, err = e.svc.Submit(ctx, userID, 999999, "省一", bytes.NewReader(testPNG))
	requireAppErrorCode(t, err, 40400)

	// 重新启用。
	require.NoError(t, e.db.Model(&certificate.CertificateType{}).
		Where("id = ?", typeID).Update("is_active", true).Error)

	// level 空/超 50、空图、超 5 MiB、非图片 -> 40001。
	_, err = e.svc.Submit(ctx, userID, typeID, "  ", bytes.NewReader(testPNG))
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.Submit(ctx, userID, typeID, strings.Repeat("级", 51), bytes.NewReader(testPNG))
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.Submit(ctx, userID, typeID, "省一", bytes.NewReader(nil))
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.Submit(ctx, userID, typeID, "省一", bytes.NewReader(make([]byte, 5<<20+1)))
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.Submit(ctx, userID, typeID, "省一", strings.NewReader("hello world"))
	requireAppErrorCode(t, err, 40001)

	// 校验失败不留行。
	var count int64
	require.NoError(t, e.db.Model(&certificate.Certificate{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestListMinePagination(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	userID := createUser(t, e.db, "page-user", "分页用户")

	for i := 0; i < 3; i++ {
		_, err := e.svc.Submit(ctx, userID, typeID, fmt.Sprintf("等级%d", i), bytes.NewReader(testPNG))
		require.NoError(t, err)
	}

	page, err := e.svc.ListMine(ctx, userID, 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	require.Len(t, page.Items, 2)
	// 按提交时间倒序：后提交的在前。
	assert.Equal(t, "等级2", page.Items[0].Level)
	assert.Equal(t, "等级1", page.Items[1].Level)

	page2, err := e.svc.ListMine(ctx, userID, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2.Items, 1)
	assert.Equal(t, "等级0", page2.Items[0].Level)
}

func TestCertificateImageAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)

	ownerID := createUser(t, e.db, "img-owner", "图片主人")
	otherID := createUser(t, e.db, "img-other", "他人")

	v, err := e.svc.Submit(ctx, ownerID, typeID, "省一", bytes.NewReader(testPNG))
	require.NoError(t, err)

	// owner 200。
	img, err := e.svc.Image(ctx, v.ID, ownerID, false)
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.ContentType)
	assert.Equal(t, testPNG, img.Data)

	// 其他用户 404。
	_, err = e.svc.Image(ctx, v.ID, otherID, false)
	requireAppErrorCode(t, err, 40400)

	// 管理员（canManage=true）200。
	img, err = e.svc.Image(ctx, v.ID, otherID, true)
	require.NoError(t, err)
	assert.Equal(t, testPNG, img.Data)

	// 不存在 404。
	_, err = e.svc.Image(ctx, 999999, ownerID, false)
	requireAppErrorCode(t, err, 40400)
}

func TestVerifyApprovedOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)

	userID := createUser(t, e.db, "verify-user", "核验昵称")
	code := "KAPOJ-ABCD-EFGH"
	approvedAt := time.Now()

	approved := &certificate.Certificate{
		UserID:              userID,
		CertificateTypeID:   typeID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusApproved,
		VerifyCode:          &code,
		SubmittedAt:         time.Now(),
		ReviewedAt:          &approvedAt,
	}
	require.NoError(t, e.db.Create(approved).Error)

	// pending / rejected 即使带 verify_code 也不会命中。
	pendingCode := "KAPOJ-PEND-PEND"
	require.NoError(t, e.db.Create(&certificate.Certificate{
		UserID:              userID,
		CertificateTypeID:   typeID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/2",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		VerifyCode:          &pendingCode,
		SubmittedAt:         time.Now(),
	}).Error)

	// approved 返回 valid + 类型/等级/昵称/通过时间。
	v, err := e.svc.Verify(ctx, "kapoj-abcd-efgh")
	require.NoError(t, err)
	assert.True(t, v.Valid)
	assert.Equal(t, "蓝桥杯", v.Type)
	assert.Equal(t, "省一", v.Level)
	assert.Equal(t, "核验昵称", v.Nickname)
	require.NotNil(t, v.ApprovedAt)

	// 大小写/空格/无连字符/去前缀容错。
	for _, c := range []string{" KAPOJ ABCD EFGH ", "abcd-efgh", "ABCDEFGH", "kapoj abcd efgh"} {
		v, err = e.svc.Verify(ctx, c)
		require.NoError(t, err)
		assert.True(t, v.Valid, "code %q 应核验通过", c)
		assert.Equal(t, "核验昵称", v.Nickname)
	}

	// pending / rejected / unknown 统一 {valid:false}。
	for _, c := range []string{pendingCode, "KAPOJ-ZZZZ-ZZZZ"} {
		v, err = e.svc.Verify(ctx, c)
		require.NoError(t, err)
		assert.False(t, v.Valid)
	}

	// 无效格式直接 false。
	v, err = e.svc.Verify(ctx, "short")
	require.NoError(t, err)
	assert.False(t, v.Valid)
}

func TestAdminListFiltersAndPagination(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)

	u1 := createUser(t, e.db, "admin-list-a", "列表用户甲")
	u2 := createUser(t, e.db, "admin-list-b", "列表用户乙")

	base := time.Now()
	mk := func(userID int64, status certificate.Status, level string, at time.Time) certificate.Certificate {
		c := certificate.Certificate{
			UserID:              userID,
			CertificateTypeID:   typeID,
			CertificateTypeName: "蓝桥杯",
			Level:               level,
			ImageKey:            "certificates/1",
			ImageContentType:    "image/png",
			Status:              status,
			SubmittedAt:         at,
		}
		require.NoError(t, e.db.Create(&c).Error)
		return c
	}
	c1 := mk(u1, certificate.StatusPending, "省一", base)
	c2 := mk(u2, certificate.StatusApproved, "国一", base.Add(time.Second))
	c3 := mk(u1, certificate.StatusRejected, "省二", base.Add(2*time.Second))

	// 空 status（all）返回全部，按 submitted_at 倒序、id 倒序。
	page, err := e.svc.ListAdmin(ctx, "", 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, page.Total)
	require.Len(t, page.Items, 3)
	assert.Equal(t, c3.ID, page.Items[0].ID)
	assert.Equal(t, c2.ID, page.Items[1].ID)
	assert.Equal(t, c1.ID, page.Items[2].ID)

	// 用户摘要：id/username/nickname。
	assert.Equal(t, u1, page.Items[0].User.ID)
	assert.Equal(t, "admin-list-a", page.Items[0].User.Username)
	assert.Equal(t, "列表用户甲", page.Items[0].User.Nickname)
	assert.Equal(t, u2, page.Items[1].User.ID)

	// status 筛选。
	pending, err := e.svc.ListAdmin(ctx, certificate.StatusPending, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, pending.Total)
	assert.Equal(t, c1.ID, pending.Items[0].ID)

	approved, err := e.svc.ListAdmin(ctx, certificate.StatusApproved, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, approved.Total)
	assert.Equal(t, c2.ID, approved.Items[0].ID)
	assert.Equal(t, "国一", approved.Items[0].Level)

	rejected, err := e.svc.ListAdmin(ctx, certificate.StatusRejected, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, rejected.Total)
	assert.Equal(t, c3.ID, rejected.Items[0].ID)

	// 分页。
	p1, err := e.svc.ListAdmin(ctx, "", 1, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, p1.Total)
	require.Len(t, p1.Items, 2)
	p2, err := e.svc.ListAdmin(ctx, "", 2, 2)
	require.NoError(t, err)
	require.Len(t, p2.Items, 1)

	// 非法 status 40001。
	_, err = e.svc.ListAdmin(ctx, certificate.Status("bogus"), 1, 20)
	requireAppErrorCode(t, err, 40001)
}

func TestReviewApprove(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	ownerID := createUser(t, e.db, "review-owner", "审核主人")
	adminID := createUser(t, e.db, "review-admin", "审核员")

	v, err := e.svc.Submit(ctx, ownerID, typeID, "省一", bytes.NewReader(testPNG))
	require.NoError(t, err)

	got, err := e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "approve"})
	require.NoError(t, err)
	assert.Equal(t, certificate.StatusApproved, got.Status)
	assert.NotEmpty(t, got.VerifyCode)
	require.NotNil(t, got.ReviewedAt)
	// 验证码格式 KAPOJ-XXXX-XXXX，字母表去除 I/L/O/U/0/1。
	assert.Regexp(t, `^KAPOJ-[A-HJKMNP-TV-Z2-9]{4}-[A-HJKMNP-TV-Z2-9]{4}$`, got.VerifyCode)

	var cert certificate.Certificate
	require.NoError(t, e.db.First(&cert, v.ID).Error)
	assert.Equal(t, certificate.StatusApproved, cert.Status)
	require.NotNil(t, cert.ReviewedBy)
	assert.Equal(t, adminID, *cert.ReviewedBy)
	require.NotNil(t, cert.ReviewedAt)
	require.NotNil(t, cert.VerifyCode)
	assert.Equal(t, got.VerifyCode, *cert.VerifyCode)

	// 公开 Verify 变为 valid。
	pv, err := e.svc.Verify(ctx, got.VerifyCode)
	require.NoError(t, err)
	assert.True(t, pv.Valid)
	assert.Equal(t, "审核主人", pv.Nickname)

	// 重复审核 40001。
	_, err = e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "approve"})
	requireAppErrorCode(t, err, 40001)
}

func TestReviewReject(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	ownerID := createUser(t, e.db, "reject-owner", "驳回主人")
	adminID := createUser(t, e.db, "reject-admin", "驳回审核员")

	v, err := e.svc.Submit(ctx, ownerID, typeID, "省一", bytes.NewReader(testPNG))
	require.NoError(t, err)

	// 缺原因 / 超 500 字 -> 40001。
	_, err = e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "reject"})
	requireAppErrorCode(t, err, 40001)
	_, err = e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "reject", Reason: strings.Repeat("长", 501)})
	requireAppErrorCode(t, err, 40001)

	// 合法驳回（两侧空白去除）。
	got, err := e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "reject", Reason: "  证书信息不符  "})
	require.NoError(t, err)
	assert.Equal(t, certificate.StatusRejected, got.Status)
	assert.Equal(t, "证书信息不符", got.RejectReason)
	assert.Empty(t, got.VerifyCode)

	var cert certificate.Certificate
	require.NoError(t, e.db.First(&cert, v.ID).Error)
	assert.Equal(t, certificate.StatusRejected, cert.Status)
	assert.Equal(t, "证书信息不符", cert.RejectReason)
	assert.Nil(t, cert.VerifyCode, "驳回后 verify_code 应为 NULL")
	require.NotNil(t, cert.ReviewedBy)
	assert.Equal(t, adminID, *cert.ReviewedBy)

	// 驳回原因仅本人/管理员可见：本人列表可见，公开核验 invalid 且无原因。
	mine, err := e.svc.ListMine(ctx, ownerID, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, "证书信息不符", mine.Items[0].RejectReason)
	pv, err := e.svc.Verify(ctx, "KAPOJ-AAAA-BBBB")
	require.NoError(t, err)
	assert.False(t, pv.Valid)

	admin, err := e.svc.ListAdmin(ctx, certificate.StatusRejected, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, "证书信息不符", admin.Items[0].RejectReason)
}

func TestReviewNotFoundAndTerminal(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	ownerID := createUser(t, e.db, "term-owner", "终态主人")
	adminID := createUser(t, e.db, "term-admin", "终态审核员")

	// 不存在 404。
	_, err := e.svc.Review(ctx, adminID, 999999, certificate.ReviewInput{Action: "approve"})
	requireAppErrorCode(t, err, 40400)

	// 已通过证书再审核 -> 40001 terminal。
	code := "KAPOJ-TERM-TERM"
	require.NoError(t, e.db.Create(&certificate.Certificate{
		UserID:              ownerID,
		CertificateTypeID:   typeID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/1",
		ImageContentType:    "image/png",
		Status:              certificate.StatusApproved,
		VerifyCode:          &code,
		SubmittedAt:         time.Now(),
	}).Error)
	var approved certificate.Certificate
	require.NoError(t, e.db.Where("status = ?", certificate.StatusApproved).First(&approved).Error)
	_, err = e.svc.Review(ctx, adminID, approved.ID, certificate.ReviewInput{Action: "approve"})
	requireAppErrorCode(t, err, 40001)

	// 非法 action 40001。
	pending := &certificate.Certificate{
		UserID:              ownerID,
		CertificateTypeID:   typeID,
		CertificateTypeName: "蓝桥杯",
		Level:               "省一",
		ImageKey:            "certificates/2",
		ImageContentType:    "image/png",
		Status:              certificate.StatusPending,
		SubmittedAt:         time.Now(),
	}
	require.NoError(t, e.db.Create(pending).Error)
	_, err = e.svc.Review(ctx, adminID, pending.ID, certificate.ReviewInput{Action: "banana"})
	requireAppErrorCode(t, err, 40001)
}

func TestReviewNoSensitivePublicFields(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)
	ownerID := createUser(t, e.db, "sensitive-owner", "敏感主人")
	adminID := createUser(t, e.db, "sensitive-admin", "敏感审核员")

	v, err := e.svc.Submit(ctx, ownerID, typeID, "省一", bytes.NewReader(testPNG))
	require.NoError(t, err)
	_, err = e.svc.Review(ctx, adminID, v.ID, certificate.ReviewInput{Action: "approve"})
	require.NoError(t, err)

	page, err := e.svc.ListAdmin(ctx, certificate.StatusApproved, 1, 20)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)

	b, err := json.Marshal(page.Items[0])
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	userObj, ok := m["user"].(map[string]any)
	require.True(t, ok, "管理员列表应包含 user 摘要")
	for _, key := range []string{"email", "passwordHash", "password", "avatarUrl", "bio", "coins", "rating", "role", "status"} {
		_, exists := userObj[key]
		assert.False(t, exists, "管理员用户摘要不应包含 %s", key)
	}
	for _, key := range []string{"id", "username", "nickname"} {
		_, exists := userObj[key]
		assert.True(t, exists, "管理员用户摘要应包含 %s", key)
	}

	// 公开核验不含敏感字段。
	pv, err := e.svc.Verify(ctx, page.Items[0].VerifyCode)
	require.NoError(t, err)
	assert.True(t, pv.Valid)
	pb, err := json.Marshal(pv)
	require.NoError(t, err)
	var pm map[string]any
	require.NoError(t, json.Unmarshal(pb, &pm))
	for _, key := range []string{"imageUrl", "userId", "email", "rejectReason", "reviewedBy", "verifyCode"} {
		_, exists := pm[key]
		assert.False(t, exists, "公开核验不应包含 %s", key)
	}
}

func TestListApprovedByUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	require.NoError(t, certificate.SeedTypes(ctx, e.db))
	typeID := firstActiveTypeID(t, e)

	ownerID := createUser(t, e.db, "profile-owner", "主页主人")
	otherID := createUser(t, e.db, "profile-other", "其他用户")
	reviewerID := createUser(t, e.db, "profile-reviewer", "审核员")

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	codeOld, codeNew, codeTie := "KAPOJ-OLD1-OLD1", "KAPOJ-NEW1-NEW1", "KAPOJ-TIE1-TIE1"
	codeOther := "KAPOJ-OTHR-OTHR"

	// mk 插入一条证书；reviewedAt 为 nil 表示未审核。
	mk := func(userID int64, status certificate.Status, level string, code *string, reviewedAt *time.Time) certificate.Certificate {
		c := certificate.Certificate{
			UserID:              userID,
			CertificateTypeID:   typeID,
			CertificateTypeName: "蓝桥杯",
			Level:               level,
			ImageKey:            "certificates/1",
			ImageContentType:    "image/png",
			Status:              status,
			RejectReason:        "不应泄露的驳回原因",
			VerifyCode:          code,
			SubmittedAt:         base.Add(-24 * time.Hour),
			ReviewedAt:          reviewedAt,
			ReviewedBy:          &reviewerID,
		}
		require.NoError(t, e.db.Create(&c).Error)
		return c
	}
	at := func(d time.Duration) *time.Time { v := base.Add(d); return &v }

	older := mk(ownerID, certificate.StatusApproved, "省三", &codeOld, at(0))
	newer := mk(ownerID, certificate.StatusApproved, "省二", &codeNew, at(time.Hour))
	tie := mk(ownerID, certificate.StatusApproved, "省一", &codeTie, at(time.Hour))
	mk(ownerID, certificate.StatusPending, "国三", nil, at(2*time.Hour))
	mk(ownerID, certificate.StatusRejected, "国二", nil, at(2*time.Hour))
	mk(otherID, certificate.StatusApproved, "国一", &codeOther, at(3*time.Hour))

	// 只返回 approved，按 reviewed_at DESC, id DESC（同时间用 id 倒序兜底）。
	items, total, err := e.svc.ListApprovedByUser(ctx, ownerID, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 3)
	assert.Equal(t, tie.ID, items[0].ID)
	assert.Equal(t, newer.ID, items[1].ID)
	assert.Equal(t, older.ID, items[2].ID)

	// 字段：type 使用提交时快照的类型名，approvedAt 取 reviewed_at。
	first := items[0]
	assert.Equal(t, "蓝桥杯", first.Type)
	assert.Equal(t, "省一", first.Level)
	assert.Equal(t, codeTie, first.VerifyCode)
	require.NotNil(t, first.ApprovedAt)
	assert.True(t, first.ApprovedAt.Equal(base.Add(time.Hour)))

	// 公开摘要只包含 id/type/level/verifyCode/approvedAt，绝不泄露图片、状态、
	// 驳回原因、审核人等敏感字段。
	b, err := json.Marshal(first)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "type", "level", "verifyCode", "approvedAt"}, keys)
	for _, key := range []string{"imageUrl", "status", "rejectReason", "reviewedBy", "reviewedAt", "userId", "email", "submittedAt"} {
		_, exists := m[key]
		assert.False(t, exists, "公开证书摘要不应包含 %s", key)
	}

	// 无证书用户返回非 nil 空切片 + total 0。
	emptyID := createUser(t, e.db, "profile-empty", "空用户")
	items, total, err = e.svc.ListApprovedByUser(ctx, emptyID, 50)
	require.NoError(t, err)
	assert.Zero(t, total)
	require.NotNil(t, items, "无证书应返回空切片而非 nil")
	assert.Empty(t, items)

	// limit 生效：limit=1 只取第一条，total 仍是全部 approved 数。
	items, total, err = e.svc.ListApprovedByUser(ctx, ownerID, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 1)
	assert.Equal(t, tie.ID, items[0].ID)

	// limit<=0 取默认 50；超过 50 截断为 50。
	items, _, err = e.svc.ListApprovedByUser(ctx, ownerID, 0)
	require.NoError(t, err)
	assert.Len(t, items, 3)

	bulkID := createUser(t, e.db, "profile-bulk", "批量用户")
	bulk := make([]certificate.Certificate, 51)
	for i := range bulk {
		code := fmt.Sprintf("KAPOJ-BULK-%04d", i)
		reviewed := base.Add(time.Duration(i) * time.Minute)
		bulk[i] = certificate.Certificate{
			UserID:              bulkID,
			CertificateTypeID:   typeID,
			CertificateTypeName: "蓝桥杯",
			Level:               "省一",
			ImageKey:            "certificates/1",
			ImageContentType:    "image/png",
			Status:              certificate.StatusApproved,
			VerifyCode:          &code,
			SubmittedAt:         base,
			ReviewedAt:          &reviewed,
		}
	}
	require.NoError(t, e.db.Create(&bulk).Error)
	items, total, err = e.svc.ListApprovedByUser(ctx, bulkID, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 51, total)
	assert.Len(t, items, 50, "limit 应上限为 50")

	// 非法用户 ID -> 404。
	_, _, err = e.svc.ListApprovedByUser(ctx, 0, 10)
	requireAppErrorCode(t, err, 40400)
}

type failingStorage struct {
	putErr  error
	deleted []string
}

func (f *failingStorage) Put(context.Context, string, io.Reader, int64) (int64, error) {
	return 0, f.putErr
}

func (f *failingStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}

func (f *failingStorage) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

func TestSubmitStorageFailureCleansRow(t *testing.T) {
	g := testutil.PG(t)
	require.NoError(t, certificate.SeedTypes(context.Background(), g))
	var typ certificate.CertificateType
	require.NoError(t, g.Where("is_active = ?", true).First(&typ).Error)

	userID := createUser(t, g, "fail-user", "失败用户")

	fs := &failingStorage{putErr: errors.New("disk full")}
	svc := certificate.NewService(certificate.NewGormRepository(g), fs)

	_, err := svc.Submit(context.Background(), userID, typ.ID, "省一", bytes.NewReader(testPNG))
	require.Error(t, err)

	// 失败后不留行，且尝试删除文件。
	var count int64
	require.NoError(t, g.Model(&certificate.Certificate{}).Where("user_id = ?", userID).Count(&count).Error)
	assert.Zero(t, count)
	assert.NotEmpty(t, fs.deleted, "清理时应尝试删除对象")
}
