# 站点管理、首页内容与品牌配置实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把站点 Logo/favicon、首页欢迎区/快速开始/成长中心、页脚备案与运行/访问数据集中到后台，新增独立 `site.settings.manage` 权限，并让公开站点在配置缺失时回退到当前内容。

**Architecture:** 后端新增 `site` 域：`site_settings` 单例存品牌/页脚/JSONB 首页内容，`site_visit_counters` 单例原子累计访问量；`siteapi` 提供公开读写与管理读写。前端新增 `site` Pinia store，统一驱动品牌、首页、页脚；`/admin/site` 用三个 Tab 编辑品牌/首页/页脚。权限由 RBAC 目录新增，写操作写审计。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + PrimeVue 4 + Pinia；OpenAPI 生成前端类型；本地 storage 存图片。

**Spec:** `docs/superpowers/specs/2026-11-09-site-management-design.md`

## Global Constraints

- 不修改现有首页路由、导航结构或认证逻辑；只把硬编码内容数据化。
- 首页配置是结构化纯文本，不接受任意 HTML/脚本；所有 `to` 只能是站内 `/` 开头路径。
- Logo/favicon 只允许 PNG/JPEG/WebP/GIF，Logo ≤2 MiB、favicon ≤1 MiB，magic bytes 校验；拒绝 SVG。
- 公开配置读取无需登录；管理读写必须 `site.settings.manage`；未登录 401、无权限 403。
- 新增第 38 个权限 `site.settings.manage`（分组「站点」），默认仅 super；Seed 不自动给已有角色授权。
- `site.settings.manage` 与 `system.settings.manage` 独立；页面编辑权限只认前者。
- 访问量按浏览器会话 +1；前端 `sessionStorage` 去重；后端原子递增；`showVisits` 只控制页脚展示，不停止计数。
- 首页默认内容以后端 `DefaultHomeContent()` 为唯一权威；API 永远返回完整结构；前端兜底仅用于请求失败。
- 页脚运行天数由服务端 `startedAt` 计算；`startedAt` 不能晚于当前时间 + 1 天。
- 图片先写新文件 → 更新 DB → best-effort 删除旧文件；DB 失败删除新文件。
- 所有中文文案保持现有风格；不新增依赖。

## Review Focus

- 站点配置读取失败时首页/页脚/品牌必须回退到当前内容，不出现空白或崩溃（Task 4）。
- Logo/favicon 替换后公开 URL 必须带新版本，旧文件被清理，未上传/删除后回退静态品牌与 `/favicon.svg`（Task 1、Task 4）。
- 首页链接/图标/文本校验必须阻止 `javascript:`、外链、HTML 注入和超长字段（Task 1）。
- 权限必须真正独立：没有 `site.settings.manage` 的普通用户/内容管理员不能读写管理接口，super 可访问且写审计（Task 2）。
- 访问量同一标签页会话只 +1，刷新不增加；`showVisits=false` 只隐藏不停止计数（Task 1、Task 4）。
- 页脚字段为空时不出现多余分隔符；运行天数/访问量格式化正确；移动端换行（Task 4）。

---

### Task 1: 后端 site 域（模型、默认内容、仓储、服务、图片、访问量）

**Files:**
- Create: `apps/server/internal/site/model.go`
- Create: `apps/server/internal/site/dto.go`
- Create: `apps/server/internal/site/defaults.go`
- Create: `apps/server/internal/site/validate.go`
- Create: `apps/server/internal/site/repository.go`
- Create: `apps/server/internal/site/image.go`
- Create: `apps/server/internal/site/service.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/site/service_test.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Consumes: `dbtypes.JSON`, `storage.Storage`, `testutil.PG`, `storage.NewLocal`.
- Produces:
  - `type Settings struct`、`type VisitCounter struct`
  - `type Link struct { Icon, IconPos, Title, Description, To string }`
  - `type HomeContent struct { Eyebrow, Title, TitleHighlight, Description string; PrimaryCTA, SecondaryCTA Link; QuickTitle string; QuickLinks []Link; GrowthTitle string; GrowthLinks []Link; GuestTitle, GuestDescription string }`
  - `type FooterInput struct { ICPNumber, PoliceNumber, PoliceURL, Copyright, Text string; StartedAt time.Time; ShowVisits bool }`
  - `type PublicView struct { LogoURL, FaviconURL string; Home HomeContent; Footer FooterView; StartedAt time.Time; RunningDays int; VisitCount int64 }`
  - `type AdminView = PublicView`（管理页使用同一完整视图）
  - `type Image struct { ContentType string; Data []byte }`
  - `func DefaultHomeContent() HomeContent`
  - `func NewService(repo Repository, store storage.Storage) *Service`
  - `func (s *Service) Public(ctx context.Context) (*PublicView, error)`
  - `func (s *Service) Admin(ctx context.Context) (*AdminView, error)`
  - `func (s *Service) UpdateHome(ctx context.Context, actorID int64, in HomeContent) (*AdminView, error)`
  - `func (s *Service) UpdateFooter(ctx context.Context, actorID int64, in FooterInput) (*AdminView, error)`
  - `func (s *Service) UploadLogo(ctx context.Context, actorID int64, r io.Reader) (*AdminView, error)`
  - `func (s *Service) DeleteLogo(ctx context.Context, actorID int64) (*AdminView, error)`
  - `func (s *Service) UploadFavicon(ctx context.Context, actorID int64, r io.Reader) (*AdminView, error)`
  - `func (s *Service) DeleteFavicon(ctx context.Context, actorID int64) (*AdminView, error)`
  - `func (s *Service) RecordVisit(ctx context.Context) (int64, error)`
  - `func (s *Service) Logo(ctx context.Context) (*Image, error)`
  - `func (s *Service) Favicon(ctx context.Context) (*Image, error)`
  - `type Repository interface { Get(ctx) (*Settings, error); Update(ctx, *Settings) error; IncrementVisit(ctx) (int64, error); VisitCount(ctx) (int64, error) }`
  - `func NewGormRepository(db *gorm.DB) Repository`

- [ ] **Step 1: Write the failing tests**

在 `apps/server/internal/site/service_test.go` 添加：

```go
package site_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/testutil"
)

func newSite(t *testing.T) (*site.Service, site.Repository, storage.Storage) {
	t.Helper()
	g := testutil.PG(t)
	store, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	repo := site.NewGormRepository(g)
	return site.NewService(repo, store), repo, store
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestSiteSettingsLazyDefaults(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	view, err := svc.Public(ctx)
	require.NoError(t, err)
	assert.Equal(t, "KAPOJ // ONLINE JUDGE", view.Home.Eyebrow)
	assert.Equal(t, "欢迎来到", view.Home.Title)
	assert.Equal(t, "kapOJ", view.Home.TitleHighlight)
	assert.Len(t, view.Home.QuickLinks, 4)
	assert.True(t, view.Footer.ShowVisits)
	assert.Equal(t, int64(0), view.VisitCount)
	assert.NotZero(t, view.StartedAt)

	again, err := svc.Public(ctx)
	require.NoError(t, err)
	assert.Equal(t, view.StartedAt, again.StartedAt)
}

func TestSiteHomeValidation(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	bad := site.DefaultHomeContent()
	bad.PrimaryCTA.To = "javascript:alert(1)"
	_, err := svc.UpdateHome(ctx, 1, bad)
	assertCode(t, err, 40001)

	bad = site.DefaultHomeContent()
	bad.QuickLinks = append(bad.QuickLinks, bad.QuickLinks[0])
	_, err = svc.UpdateHome(ctx, 1, bad)
	assertCode(t, err, 40001)
}

func TestSiteFooterValidation(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	in := site.DefaultFooterInput()
	in.PoliceURL = "http://example.com"
	_, err := svc.UpdateFooter(ctx, 1, in)
	assertCode(t, err, 40001)

	in = site.DefaultFooterInput()
	in.StartedAt = time.Now().Add(48 * time.Hour)
	_, err = svc.UpdateFooter(ctx, 1, in)
	assertCode(t, err, 40001)

	in = site.DefaultFooterInput()
	in.StartedAt = time.Now().Add(-72 * time.Hour)
	view, err := svc.UpdateFooter(ctx, 1, in)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, view.RunningDays, 2)
}

func TestSiteVisitCounter(t *testing.T) {
	svc, _, _ := newSite(t)
	ctx := context.Background()
	n, err := svc.RecordVisit(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	n, err = svc.RecordVisit(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestSiteLogoLifecycle(t *testing.T) {
	svc, repo, store := newSite(t)
	ctx := context.Background()
	png := pngBytes(t)
	view, err := svc.UploadLogo(ctx, 1, bytes.NewReader(png))
	require.NoError(t, err)
	assert.Contains(t, view.LogoURL, "/api/site/logo?v=")
	oldURL := view.LogoURL
	settings, err := repo.Get(ctx)
	require.NoError(t, err)
	oldKey := settings.LogoKey

	img, err := svc.Logo(ctx)
	require.NoError(t, err)
	assert.Equal(t, "image/png", img.ContentType)
	assert.Equal(t, png, img.Data)

	view, err = svc.UploadLogo(ctx, 1, bytes.NewReader(png))
	require.NoError(t, err)
	assert.NotEqual(t, oldURL, view.LogoURL)
	_, err = store.Open(ctx, oldKey)
	assert.ErrorIs(t, err, storage.ErrNotFound, "old logo file must be cleaned")

	view, err = svc.DeleteLogo(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, view.LogoURL)
	_, err = svc.Logo(ctx)
	assert.ErrorIs(t, err, site.ErrNotFound)
}

func TestSiteRejectsNonImage(t *testing.T) {
	svc, _, _ := newSite(t)
	_, err := svc.UploadFavicon(context.Background(), 1, bytes.NewBufferString("<svg/>"))
	assertCode(t, err, 40001)
}
```

`assertCode` 复用 `contest_test` 风格；在 `site_test` 内定义：

```go
func assertCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *httpx.AppError
	if assert.True(t, errors.As(err, &appErr), "want AppError %d, got %v", code, err) {
		assert.Equal(t, code, appErr.Code)
	}
}
```

在 `apps/server/internal/db/db_test.go` 追加表/列断言：

```go
assert.True(t, g.Migrator().HasTable(&site.Settings{}))
assert.True(t, g.Migrator().HasTable(&site.VisitCounter{}))
assert.True(t, g.Migrator().HasColumn(&site.Settings{}, "home_json"))
assert.True(t, g.Migrator().HasColumn(&site.Settings{}, "footer_police_url"))
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/site/ ./internal/db/`
Expected: FAIL，`site` 包/类型未定义。

- [ ] **Step 3: Implement**

`model.go`：

```go
type Settings struct {
	ID                   int16                `gorm:"primaryKey"`
	LogoKey              string               `gorm:"column:logo_key;size:255;not null;default:''"`
	LogoContentType      string               `gorm:"column:logo_content_type;size:100;not null;default:''"`
	FaviconKey           string               `gorm:"column:favicon_key;size:255;not null;default:''"`
	FaviconContentType   string               `gorm:"column:favicon_content_type;size:100;not null;default:''"`
	StartedAt            time.Time            `gorm:"column:started_at;type:timestamptz;not null"`
	FooterICP            string               `gorm:"column:footer_icp;size:100;not null;default:''"`
	FooterPolice         string               `gorm:"column:footer_police;size:100;not null;default:''"`
	FooterPoliceURL      string               `gorm:"column:footer_police_url;size:500;not null;default:''"`
	FooterCopyright      string               `gorm:"column:footer_copyright;size:200;not null;default:''"`
	FooterText           string               `gorm:"column:footer_text;type:text;not null;default:''"`
	ShowVisits           bool                 `gorm:"column:show_visits;not null;default:true"`
	Home                 dbtypes.JSON[HomeContent] `gorm:"column:home_json;type:jsonb;not null;default:'{}'"`
	UpdatedBy            int64                `gorm:"column:updated_by;not null;default:0"`
	UpdatedAt            time.Time            `gorm:"type:timestamptz;not null"`
}

func (Settings) TableName() string { return "site_settings" }

type VisitCounter struct {
	ID        int16     `gorm:"primaryKey"`
	Count     int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (VisitCounter) TableName() string { return "site_visit_counters" }
```

`defaults.go`：

- `DefaultHomeContent()` 使用当前 `HomeView.vue` 的文案：
  - eyebrow `KAPOJ // ONLINE JUDGE`
  - title `欢迎来到`，highlight `kapOJ`
  - description 当前欢迎语
  - primary CTA `开始刷题` → `/problems`，icon `pi pi-arrow-right`，iconPos `right`
  - secondary CTA `浏览比赛` → `/contests`，icon `pi pi-trophy`，iconPos `left`
  - quickTitle `快速开始`；4 张卡片：题库 `/problems` `pi pi-book`、比赛 `/contests` `pi pi-trophy`、排行榜 `/leaderboard` `pi pi-chart-line`、测评记录 `/submissions` `pi pi-list-check`
  - growthTitle `成长中心`；4 张卡片：成就墙 `/achievements` `pi pi-star`、任务大厅 `/tasks` `pi pi-check-square`、徽章商城 `/shop/badges` `pi pi-shop`、称号商店 `/shop/titles` `pi pi-crown`
  - guestTitle `登录后记录你的成长`，guestDescription 当前文案
- `DefaultFooterInput()`：`StartedAt: time.Now().UTC()`，`Copyright: "© kapOJ"`，`ShowVisits: true`。

`repository.go`：

- `Get` 在 `site_settings` 不存在时创建 id=1、完整默认 Home、`StartedAt=time.Now().UTC()`、`UpdatedAt=now`；`home_json` 为空/零值时补 `DefaultHomeContent()` 并持久化。
- `Update` 使用事务 + `SELECT ... FOR UPDATE`，写回并更新 `UpdatedAt`。
- `IncrementVisit` 用 `INSERT ... ON CONFLICT (id) DO UPDATE SET count = site_visit_counters.count + 1, updated_at=now() RETURNING count`。
- `VisitCount` 懒创建 id=1 count=0 后读取。

`validate.go`：

- 实现设计 §5.2 的所有长度/协议/图标/重复链接/控制字符校验；`to` 必须 `/` 开头且不是 `//`；`icon` 以 `pi pi-` 开头；`iconPos` 空或 `left`/`right`；`startedAt` 不为零且不晚于 `now+24h`。
- 返回 `httpx.ErrValidation("...")` 中文错误。

`image.go`：

- 读取最多 `maxLogoBytes+1` / `maxFaviconBytes+1`，`io.LimitReader` + `io.ReadAll`；用 `http.DetectContentType` 与 allowlist 校验；写入 `site/logo/<random16>` / `site/favicon/<random16>`。
- 返回 `Image{ContentType, Data}`；`Logo`/`Favicon` 从 settings key + storage 读取；无 key 或对象不存在返回 `ErrNotFound`。
- 上传：先写新 key，再 `repo.Update` 设置 key/contentType；成功后 best-effort 删除旧 key；DB 失败删除新 key。
- 删除：清空 key/contentType，保存后删除旧 key；无 key 仍成功。

`service.go`：

- `Public`/`Admin` 读取 settings + visit count，计算 `RunningDays`，生成 `LogoURL`/`FaviconURL`（空 key 为空串；否则 `/api/site/logo?v=<UpdatedAt.Unix()>`）。
- `UpdateHome`/`UpdateFooter` 校验、写 settings、返回 AdminView。
- `RecordVisit` 调用 repo。
- `AdminView` 与 `PublicView` 同结构；管理页通过 `footer` + `startedAt` + `visitCount` 获取全部字段。

`db.Migrate` 加 `&site.Settings{}, &site.VisitCounter{}`。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/site/ ./internal/db/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/site apps/server/internal/db
git commit -m "feat(site): add site settings and visit counter"
```

---

### Task 2: site API、RBAC 权限、审计与路由

**Files:**
- Create: `apps/server/internal/siteapi/handler.go`
- Create: `apps/server/internal/siteapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/rbac/roles_test.go`
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/permissions.spec.ts`

**Interfaces:**
- Consumes: `site.Service`, `rbac.Service`, `middleware.RequireAuth/RequirePermission`.
- Produces:
  - `siteapi.RegisterRoutes(public, api, uploads gin.IRouter, svc *site.Service, auth *rbac.Service)`
  - `rbac.PermSiteSettingsManage = "site.settings.manage"`
  - `rbac.PermissionGroupSite = "站点"`
  - `rbac.AuditActionSiteSettingsManage = "site.settings.manage"`
  - `rbac.AuditTargetSiteSettings = "site_settings"`
  - `rbac.AuditTargetSiteAsset = "site_asset"`
  - `PERM.site.settings.manage`

- [ ] **Step 1: Write failing tests**

在 `apps/server/internal/siteapi/handler_test.go` 添加：

```go
func TestSitePublicAndAdminHTTP(t *testing.T) {
	f := newSiteFx(t)
	// 公开读取无需登录
	w, resp := f.do(http.MethodGet, "/api/site", "", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotEmpty(t, resp["data"].(map[string]any)["home"])

	// 未登录管理接口 401
	w, _ = f.do(http.MethodGet, "/api/admin/site", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 普通用户没有 site.settings.manage -> 403
	userToken := f.tokenForRole(t, "site_user", nil)
	w, _ = f.do(http.MethodGet, "/api/admin/site", userToken, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 拥有权限的用户可读写
	managerToken := f.tokenForRole(t, "site_manager", []string{"site.settings.manage"})
	w, resp = f.do(http.MethodGet, "/api/admin/site", managerToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	home := resp["data"].(map[string]any)["home"].(map[string]any)
	home["eyebrow"] = "UPDATED"
	w, resp = f.do(http.MethodPut, "/api/admin/site/home", managerToken, home)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	// 公开读取已更新
	w, resp = f.do(http.MethodGet, "/api/site", "", nil)
	assert.Equal(t, "UPDATED", resp["data"].(map[string]any)["home"].(map[string]any)["eyebrow"])
}

func TestSiteUploadAndVisitHTTP(t *testing.T) {
	f := newSiteFx(t)
	token := f.tokenForRole(t, "site_manager", []string{"site.settings.manage"})
	png := pngBytes(t)
	w, resp := f.multipart("/api/admin/site/logo", token, png)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	logoURL := resp["data"].(map[string]any)["logoUrl"].(string)
	assert.Contains(t, logoURL, "/api/site/logo")
	w, _ = f.do(http.MethodGet, logoURL, "", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	w, resp = f.do(http.MethodPost, "/api/site/visit", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 1, resp["data"].(map[string]any)["visitCount"])
	w, resp = f.do(http.MethodPost, "/api/site/visit", "", nil)
	assert.EqualValues(t, 2, resp["data"].(map[string]any)["visitCount"])
}
```

测试 helper 可复用 `problemapi` 测试的 fixture 风格：`testutil.PG` + `testutil.SeedRBAC` + `auth.NewTokenIssuer` + Gin engine；`tokenForRole` 创建自定义角色并通过 `rbac.Service` 或 SQL 分配权限。

RBAC 测试：

- `seed_test.go` 权限数 37→38，期望列表加入 `site.settings.manage`。
- `roles_test.go` 同样 37→38。
- 新增 `TestSiteSettingsPermissionCatalog`：`rbac.AllPermissions()` 包含 group「站点」；`PermissionCodes` 第 38 个；`content_admin` 不含该权限。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/siteapi/ ./internal/rbac/`
Expected: FAIL，路由/权限未定义。

- [ ] **Step 3: Implement**

`siteapi/handler.go`：

- 公开：
  - `GET /site` → `svc.Public`
  - `GET /site/logo`、`GET /site/favicon` → `c.Header("Cache-Control", "public, max-age=31536000, immutable")` + `c.Data(200, image.ContentType, image.Data)`；404。
  - `POST /site/visit` → `svc.RecordVisit`，`httpx.OK(c, gin.H{"visitCount": n})`。
- 管理（`middleware.RequireAuth` + `RequirePermission(site.settings.manage)`）：
  - `GET /admin/site`
  - `PUT /admin/site/home` 绑定 `site.HomeContent`（或包一层 `{home: ...}`；统一选直接 body = HomeContent）
  - `PUT /admin/site/footer` 绑定 `site.FooterInput`
  - `POST/DELETE /admin/site/logo`
  - `POST/DELETE /admin/site/favicon`
- 上传用 `uploads` group（multipart，40 MiB 限制），`c.FormFile("file")`；错误映射复用 `uploadFormError`（中文 40001）。
- 审计：`h.audit.RecordAudit(ctx, actor, rbac.AuditActionSiteSettingsManage, target, 0, summary)`；失败仅 `slog.Warn`，不阻断响应。
- `router.New`：`siteRepo := site.NewGormRepository(d.DB); siteSvc := site.NewService(siteRepo, d.Storage); siteapi.RegisterRoutes(public, api, uploads, siteSvc, d.RBAC)`。
- RBAC：
  - catalog 加 `PermSiteSettingsManage`、`PermissionGroupSite`、目录项（第 38 个，排在证书之后）。
  - audit 加 action/target。
  - 更新测试计数与期望列表；`content_admin` 不自动新增权限。
- `permissions.ts` 加 `site.settings.manage` 与 `ADMIN_ACCESS_PERMISSIONS`；`permissions.spec.ts` 覆盖。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 ./internal/siteapi/ ./internal/rbac/ ./internal/router/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/siteapi apps/server/internal/router apps/server/internal/rbac apps/web/src/permissions.ts apps/web/src/permissions.spec.ts
git commit -m "feat(site): add public/admin site API and permission"
```

---

### Task 3: OpenAPI 契约、生成类型与前端 API 客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/site.ts`

**Interfaces:**
- Consumes: backend paths/DTOs from Tasks 1-2.
- Produces:
  - `SiteLink`、`SiteHomeContent`、`SiteFooterView`、`SitePublicView`、`SiteAdminView`、`SiteFooterInput`
  - `getSite`、`recordSiteVisit`、`getAdminSite`、`updateAdminSiteHome`、`updateAdminSiteFooter`、`uploadAdminSiteLogo`、`deleteAdminSiteLogo`、`uploadAdminSiteFavicon`、`deleteAdminSiteFavicon`

- [ ] **Step 1: Update OpenAPI**

新增 schemas：

```yaml
SiteLink:
  type: object
  required: [icon, title, to]
  properties:
    icon: { type: string, pattern: '^pi pi-' }
    iconPos: { type: string, enum: [left, right], default: left }
    title: { type: string, maxLength: 40 }
    description: { type: string, maxLength: 120 }
    to: { type: string, pattern: '^/' }
SiteHomeContent:
  type: object
  required: [eyebrow, title, titleHighlight, description, primaryCta, secondaryCta, quickTitle, quickLinks, growthTitle, growthLinks, guestTitle, guestDescription]
  properties: ...
SiteFooterView:
  type: object
  required: [icpNumber, policeNumber, policeUrl, copyright, text, showVisits]
  properties: ...
SitePublicView:
  type: object
  required: [logoUrl, faviconUrl, home, footer, startedAt, runningDays, visitCount]
  properties: ...
SiteFooterInput:
  type: object
  required: [icpNumber, policeNumber, policeUrl, copyright, text, startedAt, showVisits]
  properties: ...
```

paths：

- `GET /site` → `SitePublicView`
- `GET /site/logo`、`GET /site/favicon` → `image/*` binary
- `POST /site/visit` → `{visitCount}`
- `GET /admin/site` → `SitePublicView`
- `PUT /admin/site/home` body `SiteHomeContent`
- `PUT /admin/site/footer` body `SiteFooterInput`
- `POST/DELETE /admin/site/logo`
- `POST/DELETE /admin/site/favicon`

- [ ] **Step 2: Regenerate and typecheck**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm --filter web exec vue-tsc -b --force`
Expected: 生成成功；新增 API 客户端未使用时 typecheck 通过。

- [ ] **Step 3: Implement `api/site.ts` and aliases**

`types.ts` 增加别名：

```ts
export type SiteLink = Schemas['SiteLink']
export type SiteHomeContent = Schemas['SiteHomeContent']
export type SiteFooterView = Schemas['SiteFooterView']
export type SitePublicView = Schemas['SitePublicView']
export type SiteFooterInput = Schemas['SiteFooterInput']
```

`api/site.ts` 使用 `http` 实现设计 §5.3/§5.4 的九个函数；上传用 `FormData` + `postForm`。

- [ ] **Step 4: Commit**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/site.ts
git commit -m "docs(api): add site management contract"
```

---

### Task 4: 前端 site store、品牌、页脚与首页数据化

**Files:**
- Create: `apps/web/src/stores/site.ts`
- Create: `apps/web/src/stores/site.spec.ts`
- Create: `apps/web/src/components/layout/SiteFooter.vue`
- Create: `apps/web/src/components/layout/SiteFooter.spec.ts`
- Modify: `apps/web/src/App.vue`
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Modify: `apps/web/src/components/layout/AppShell.vue`
- Modify: `apps/web/src/components/layout/AppBrand.vue`
- Modify: `apps/web/src/components/auth/AuthLayout.vue`
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `apps/web/src/views/HomeView.spec.ts`
- Modify: `apps/web/index.html`

**Interfaces:**
- Consumes: `getSite`, `recordSiteVisit`, `SitePublicView`, `SiteHomeContent`, `SiteFooterView`.
- Produces:
  - `useSiteStore()` with `config`, `home`, `footer`, `logoUrl`, `faviconUrl`, `runningDays`, `visitCount`, `ensureLoaded()`, `recordVisit()`
  - `DEFAULT_HOME`, `DEFAULT_FOOTER`
  - `SiteFooter.vue`

- [ ] **Step 1: Write failing tests**

`site.spec.ts`：

```ts
it('loads public config once and applies favicon', async () => {
  vi.mocked(getSite).mockResolvedValue(siteView({ faviconUrl: '/api/site/favicon?v=1' }))
  const store = useSiteStore()
  await store.ensureLoaded()
  await store.ensureLoaded()
  expect(getSite).toHaveBeenCalledTimes(1)
  expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/api/site/favicon?v=1')
})

it('falls back to defaults when the request fails', async () => {
  vi.mocked(getSite).mockRejectedValue(new Error('network'))
  const store = useSiteStore()
  await store.ensureLoaded()
  expect(store.home.title).toBe('欢迎来到')
  expect(store.footer.copyright).toContain('kapOJ')
})

it('records a visit once per session', async () => {
  sessionStorage.clear()
  vi.mocked(recordSiteVisit).mockResolvedValue({ visitCount: 7 })
  const store = useSiteStore()
  await store.recordVisit()
  await store.recordVisit()
  expect(recordSiteVisit).toHaveBeenCalledTimes(1)
})
```

`HomeView.spec.ts`：mock `useSiteStore` 或真实 store + `getSite`；断言自定义 `home.eyebrow/title/quickLinks[0].title` 渲染，未登录/登录成长卡片正确，默认回退。

`SiteFooter.spec.ts`：断言版权、ICP/公安链接、运行天数、访问量格式化、`showVisits=false` 隐藏访问量、空字段无多余分隔符。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/stores/site.spec.ts src/views/HomeView.spec.ts src/components/layout/SiteFooter.spec.ts`
Expected: FAIL，store/组件不存在。

- [ ] **Step 3: Implement**

`stores/site.ts`：

- 默认常量与后端一致（当前首页文案）。
- `ensureLoaded` 使用单一 in-flight promise；成功设置 `config`，失败保持 `config=null`，可稍后重试。
- `recordVisit` 用 `sessionStorage['kapoj:site-visited']`；先检查，成功/失败后处理标记；成功时更新 `config.visitCount`。
- `watch(faviconUrl, applyFavicon, { immediate: true })`；无 favicon 时 href `/favicon.svg`。

`App.vue`：`onMounted(() => void useSiteStore().ensureLoaded())`。

`DefaultLayout.vue`：`onMounted(() => void useSiteStore().recordVisit())`；`<template #footer><SiteFooter /></template>`。

`AppShell.vue`：在 `<main>` 后加：

```vue
<footer v-if="$slots.footer" class="app-footer">
  <slot name="footer" />
</footer>
```

补 `.app-footer` 样式（公开页面底部、与内容同宽）。

`AppBrand.vue`：读取 `site.logoUrl`；有 Logo 且未加载失败时显示 `<img class="app-brand__logo" :src="logoUrl" alt="kapOJ 首页" @error="logoFailed = true" />`，否则保留现有 `K` + `kapOJ`。`watch(logoUrl, () => logoFailed=false)`。

`AppShell.vue` header：用 `<AppBrand class="app-header__brand" />` 替换现有文字品牌（或复用同一 logo 条件），确保顶部也显示上传 Logo。

`AuthLayout.vue`：用 `<AppBrand />` 替换 `.auth-brand` 区块，删除重复品牌样式；如保留现有结构，则同样加 `logoUrl` 条件。

`HomeView.vue`：

- 删除本地 `quickLinks`/`growthLinks` 常量，改用 `site.home.quickLinks`/`growthLinks`；`visibleGrowthLinks` 仍追加登录后的 `appearanceLink`。
- 欢迎区使用 `site.home.eyebrow/title/titleHighlight/description`；CTA 使用 `primaryCta`/`secondaryCta` 的 `title/to/icon/iconPos`。
- `quickTitle`/`growthTitle` 使用配置；游客条使用 `guestTitle/guestDescription`。
- 空图标/空链接由后端校验保证；前端对空数组显示空态或不渲染。

`index.html`：给 favicon link 加 `id="app-favicon"`。

`SiteFooter.vue`：按设计 §5.8 渲染；`Intl.NumberFormat('zh-CN')` 格式化访问量；`showVisits=false` 隐藏访问量；字段为空不输出分隔符。

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/stores/site.spec.ts src/views/HomeView.spec.ts src/components/layout/SiteFooter.spec.ts src/components/layout/AppShell.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/stores/site.ts apps/web/src/stores/site.spec.ts apps/web/src/components/layout/SiteFooter.vue apps/web/src/components/layout/SiteFooter.spec.ts apps/web/src/App.vue apps/web/src/layouts/DefaultLayout.vue apps/web/src/components/layout/AppShell.vue apps/web/src/components/layout/AppBrand.vue apps/web/src/components/auth/AuthLayout.vue apps/web/src/views/HomeView.vue apps/web/src/views/HomeView.spec.ts apps/web/index.html
git commit -m "feat(web): drive site brand, home and footer from config"
```

---

### Task 5: `/admin/site` 三 Tab 管理页、导航与路由

**Files:**
- Create: `apps/web/src/views/AdminSiteView.vue`
- Create: `apps/web/src/views/AdminSiteView.spec.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Consumes: `getAdminSite`, `updateAdminSiteHome`, `updateAdminSiteFooter`, `uploadAdminSiteLogo`, `deleteAdminSiteLogo`, `uploadAdminSiteFavicon`, `deleteAdminSiteFavicon`, `SiteHomeContent`, `SiteFooterInput`, `PERM.site.settings.manage`.
- Produces: `/admin/site` page with tabs `branding`/`home`/`footer`; AdminNav site group.

- [ ] **Step 1: Write failing tests**

`AdminSiteView.spec.ts`：

```ts
it('loads and saves homepage content', async () => { ... })
it('uploads and removes logo and favicon', async () => { ... })
it('validates and saves footer fields', async () => { ... })
it('shows load error and retries', async () => { ... })
```

`AdminNav.spec.ts`：只有 `site.settings.manage` 时显示 `/admin/site`；没有时隐藏。

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/AdminSiteView.spec.ts src/components/layout/AdminNav.spec.ts`
Expected: FAIL。

- [ ] **Step 3: Implement**

- `AdminSiteView.vue`：
  - `Tabs` + `TabList` + `Tab` + `TabPanels`/`TabPanel`；`tab = ref('branding')`。
  - `load()` → `getAdminSite`；`view` ref；`homeForm`/`footerForm` 从 view 深拷贝。
  - 品牌：Logo/favicon 预览；`input type=file` 隐藏 + Button trigger；`accept="image/png,image/jpeg,image/webp,image/gif"`；上传后 `view = result`；移除确认；显示大小/格式提示。
  - 首页：欢迎区字段；CTA 字段；卡片编辑器（`LinkEditor` 局部组件或内联 `v-for`）：icon `Select` 预设、title/description/to `InputText`、上移/下移/删除/添加；`saveHome()` → `updateAdminSiteHome(homeForm)`。
  - 页脚：ICP/公安/链接/版权/自定义说明 `Textarea`；`DatePicker` 绑定 `startedAt`；`ToggleSwitch` 显示访问量；只读 `view.runningDays`/`view.visitCount`；`saveFooter()` → `updateAdminSiteFooter({...footerForm, startedAt: toISOString})`。
  - 前端校验：标题必填、链接 `/` 开头、图标/URL 格式；服务端错误 Toast。
  - 使用 `AppPageHeader`、`AppSurface`、`AppFormField`、`Button`、`InputText`、`Textarea`、`DatePicker`、`ToggleSwitch`、`Select`、`Tag`、`Message`。
- `AdminNav.vue`：新增 `siteItem`（`/admin/site`，`pi pi-image` 或 `pi pi-desktop`，label `站点设置`）；在 `groups` 中当 `auth.can(PERM.site.settings.manage)` 时加入新分组 `{key:'site', label:'站点', items:[siteItem]}`（放在内容/系统分组之后或之前均可，保持当前顺序风格）。
- `router/index.ts`：admin children 加：
  ```ts
  {
    path: 'site',
    name: 'admin-site',
    component: () => import('@/views/AdminSiteView.vue'),
    meta: { requiresAuth: true, permission: PERM.site.settings.manage, title: '站点设置' },
  }
  ```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web exec vitest run src/views/AdminSiteView.spec.ts src/components/layout/AdminNav.spec.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AdminSiteView.vue apps/web/src/views/AdminSiteView.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add admin site management page"
```

---

### Task 6: 全量验证、真实冒烟与任务清单同步

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`（新增本次交付记录）
- Modify: `README.md`（当前进度一句话，可选）
- No new feature files.

**Interfaces:**
- Consumes: Tasks 1-5.
- Produces: verified branch, updated M6 task list statuses.

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
```

Expected: 全部通过，0 issues。

- [ ] **Step 2: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

Expected: 全部通过。

- [ ] **Step 3: Real smoke**

临时库 `kapoj_site_smoke` + 临时 server `:18081` + 临时 storage；用 SQL 给测试用户分配 `site.settings.manage` 或直接使用 super。验证：

1. 游客 `GET /api/site`：默认首页、`startedAt`、`runningDays`、`visitCount=0`、空 Logo/favicon。
2. 普通用户 `GET/PUT /api/admin/site` 403；管理员 200。
3. 上传 PNG Logo/favicon → 公开 URL 可读、Content-Type 正确；替换后 URL `?v=` 变化；删除后 404 且公开 URL 为空。
4. 保存首页（修改 eyebrow、新增/删除卡片、修改 CTA）→ 公开读取一致；非法 `javascript:`/外链/重复链接返回 40001；`PUT /home` 返回 40001 时数据不变。
5. 保存页脚（ICP/公安/版权/自定义文本/`startedAt`/`showVisits=false`）→ 公开读取 `runningDays` 正确、访问量仍累计但页脚隐藏（前端单测覆盖）。
6. `POST /api/site/visit` 两次 → count 2；论坛/首页刷新不重复由前端 sessionStorage 单测覆盖。
7. 审计表有 `site.settings.manage` 行；无权限请求不写成功审计。
8. 清理临时用户/角色/站点行/图片/审计，删除临时库，停止临时 server。

- [ ] **Step 4: Update docs**

在 `docs/任务清单.md` 新增 M6 小节（若计划阶段尚未添加），把 SITE-A～E 标为 ✅；写交付摘要、测试结果、设计/计划链接。`docs/开发日志.md` 追加本次变更记录。README 当前进度可加一句“站点管理”。

- [ ] **Step 5: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record site management delivery"
```

---

## Self-Review

- **Spec coverage:** 品牌/图片（Task 1/2/4）、首页内容（Task 1/2/4/5）、页脚/运行/访问量（Task 1/2/4/5）、权限/审计（Task 2）、OpenAPI（Task 3）、前端管理页（Task 5）、验证/文档（Task 6）。无遗漏。
- **Type consistency:** `HomeContent`、`FooterInput`、`PublicView`、`SitePublicView`、`SiteFooterInput`、`site.settings.manage` 在任务间名称一致。
- **Review Focus mapping:** 回退（Task 4）、图片替换/清理（Task 1/2）、注入校验（Task 1）、权限独立（Task 2）、访问量去重/显示（Task 1/4）、页脚空字段/移动端（Task 4）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、SQL/校验语义与测试断言，不逐行抄写实现。
