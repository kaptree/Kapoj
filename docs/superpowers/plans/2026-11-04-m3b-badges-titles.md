# M3-B 徽章、称号与公开身份 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M3 的第二阶段：徽章商城/装扮、称号解锁/购买/装备，以及个人主页、测评记录、比赛榜单的装备展示。

**Architecture:** 在现有 `gamification` 包增加 Badge/UserBadge/Title/UserTitle 模型与领域服务；徽章图片走本地 `storage.Storage`，通过公开图片接口读取；公开身份由 `Service.Identities` 批量查询，userapi/submissionapi/contestapi 在 DTO 上装饰，不改变原有权限与赛时保护逻辑。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、本地文件存储、Asynq、testify。

**Spec:** `docs/superpowers/specs/2026-11-04-m3-gamification-design.md` 5.2–5.6、6–11。

## Global Constraints

- 徽章/称号管理接口仅 `super_admin`；商城/商店公开，登录用户含拥有、解锁、购买、装备状态。
- 金币扣减与 `coin_transactions` 写入必须同一事务；购买徽章/称号先锁用户行，限量徽章先锁徽章行。
- 徽章装备上限读 `KAPOJ_MAX_EQUIPPED_BADGES`（默认 3，1–10）；称号同时只能装备 1 个。
- 已拥有但下线的徽章仍可见、可装备/卸下；已解锁但下线的称号仍可购买/装备；未拥有/未解锁的下线内容仅管理员可见。
- 已解锁/已购买的装备不因管理员下线而消失；公开身份展示已装备内容，即使后来下线。
- 成就奖励徽章不占用商城库存；即使徽章下线或库存为 0 也照常发放；配置成就时徽章必须存在且上架。
- 徽章图片仅支持 png/jpeg/webp/gif、≤ 2 MiB；存储 key 固定 `badges/<id>`，公开接口为 `GET /api/badges/:id/image`。
- 不实现 M3-C 前端页面；本计划只做后端、OpenAPI 与生成类型。
- 所有文档、用户可见文案使用中文；不推送、不合并分支。

## Review Focus

1. **限量徽章并发购买**：库存只能被扣到 0，不能超卖；测试放 Task 2。
2. **金币扣减与购买幂等**：同一徽章只能购买一次，余额不能为负；测试放 Task 2。
3. **装备上限并发**：并发装备不能超过 3 个；测试放 Task 2。
4. **称号装备互斥**：装备新称号必须自动卸下旧称号；未购买不能装备；测试放 Task 3。
5. **公开身份不破坏赛时保护**：测评记录/榜单加装备字段时不能泄漏被隐藏的代码或测试点；测试放 Task 4。

---

### Task 1: 徽章/称号模型、配置与公开身份类型

**Files:**
- Modify: `apps/server/internal/gamification/model.go`
- Create: `apps/server/internal/gamification/badge_repository.go`
- Create: `apps/server/internal/gamification/title_repository.go`
- Create: `apps/server/internal/gamification/public.go`
- Create: `apps/server/internal/gamification/model_test.go`
- Modify: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Consumes: Task 1–6 of M3-A 的 `Repository`、`Service`、`dbtypes.JSON`。
- Produces:
  - `Badge`、`UserBadge`、`Title`、`UserTitle` GORM 模型。
  - `type BadgeSummary struct { ID int64; Name, ImageURL string }`（JSON `id/name/imageUrl`）。
  - `type TitleSummary struct { ID int64; Name, Color string }`（JSON `id/name/color`）。
  - `type Identity struct { Badges []BadgeSummary; Title *TitleSummary }`（JSON `equippedBadges/equippedTitle`）。
  - `type ServiceOptions struct { MaxEquippedBadges int; Storage storage.Storage }`。
  - `func NewServiceWithOptions(repo *Repository, opts ServiceOptions, publisher ...EventPublisher) *Service`；`NewService` 保持默认上限 3、无 storage。
  - `(*Service).MaxEquippedBadges() int` 仅用于测试/说明，可不暴露。
  - `Config.MaxEquippedBadges int`，`KAPOJ_MAX_EQUIPPED_BADGES` 默认 3，范围 1–10。

**Model 字段：**

```go
type Badge struct {
    ID          int64
    Name        string `gorm:"size:100;not null"`
    Description string `gorm:"size:500;not null;default:''"`
    ImageURL    string `gorm:"size:512;not null;default:''"`
    PriceCoins  int64  `gorm:"not null;default:0"`
    Category    string `gorm:"size:20;not null;default:'其他'"`
    IsLimited   bool   `gorm:"not null;default:false"`
    Stock       int64  `gorm:"not null;default:0"`
    SortOrder   int    `gorm:"not null;default:0"`
    IsActive    bool   `gorm:"not null"`
    CreatedAt, UpdatedAt time.Time
}

type UserBadge struct {
    ID          int64
    UserID      int64 `gorm:"not null;uniqueIndex:uniq_user_badges,priority:1"`
    BadgeID     int64 `gorm:"not null;uniqueIndex:uniq_user_badges,priority:2"`
    IsEquipped  bool  `gorm:"not null;default:false"`
    PurchasedAt time.Time `gorm:"type:timestamptz;not null"`
}

type Title struct {
    ID                    int64
    Name                  string `gorm:"size:100;not null"`
    Color                 string `gorm:"size:7;not null"`
    UnlockConditionType   string `gorm:"size:30;not null"`
    UnlockConditionConfig dbtypes.JSON[map[string]any] `gorm:"column:unlock_condition_json;type:jsonb;not null;default:'{}'"`
    PriceCoins            int64 `gorm:"not null;default:0"`
    SortOrder             int   `gorm:"not null;default:0"`
    IsActive              bool  `gorm:"not null"`
    CreatedAt, UpdatedAt  time.Time
}

type UserTitle struct {
    ID          int64
    UserID      int64 `gorm:"not null;uniqueIndex:uniq_user_titles,priority:1"`
    TitleID     int64 `gorm:"not null;uniqueIndex:uniq_user_titles,priority:2"`
    IsEquipped  bool  `gorm:"not null;default:false"`
    UnlockedAt  time.Time  `gorm:"type:timestamptz;not null"`
    PurchasedAt *time.Time `gorm:"type:timestamptz"`
}
```

- `Badge.IsActive`/`Title.IsActive` 不写 DB default；创建时必须显式赋值。
- `db.Migrate` 追加四个模型。

- [ ] **Step 1: 写失败测试 `model_test.go`**

```go
func TestBadgeAndTitleModelsMigrate(t *testing.T) {
    g := testutil.PG(t)
    // 用 GORM 创建 Badge{IsActive:false}、Title{IsActive:false}
    // 回读断言 false；创建 UserBadge/UserTitle 断言唯一约束生效
}
func TestMaxEquippedBadgesConfig(t *testing.T) {
    // t.Setenv("KAPOJ_MAX_EQUIPPED_BADGES", "5") → cfg.MaxEquippedBadges == 5
    // 0/11 非数字 → error
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/config/ -run 'TestBadgeAndTitle|TestMaxEquipped' -v`
Expected: FAIL（类型/字段未定义）。

- [ ] **Step 3: 实现模型、迁移、配置与 ServiceOptions**

- `config.Config` 增加 `MaxEquippedBadges`，`Load` 默认 3，校验 1–10；`config_test.go` 覆盖默认、合法、非法。
- `Service` 增加 `maxEquippedBadges int`、`store storage.Storage`；`NewService` 调 `NewServiceWithOptions(repo, ServiceOptions{MaxEquippedBadges: 3}, publisher...)`；`NewServiceWithOptions` 对 `MaxEquippedBadges<=0` 回落 3，对 `>10` 截到 10。
- `public.go` 定义 `BadgeSummary/TitleSummary/Identity`，并提供 `emptyIdentity()` 返回空切片。
- `badge_repository.go`/`title_repository.go` 先放 `New...` 不需要，直接把方法挂在 `Repository` 上；本任务只实现 `FindBadge/FindTitle` 与 `List...` 的占位签名，供后续任务填充。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/db internal/config && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/config/ ./internal/db/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/db apps/server/internal/config
git commit -m "feat(server): badge and title models"
```

---

### Task 2: 徽章商城、购买、装备、图片与成就徽章奖励

**Files:**
- Create: `apps/server/internal/gamification/badge.go`
- Create: `apps/server/internal/gamification/badge_repository.go`（本任务填满）
- Create: `apps/server/internal/gamification/badge_test.go`
- Create: `apps/server/internal/gamificationapi/badge_handler.go`（或并入现有 handler.go）
- Modify: `apps/server/internal/gamification/dto.go`
- Modify: `apps/server/internal/gamification/achievement.go`
- Modify: `apps/server/internal/gamification/achievement_repository.go`
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Modify: `apps/server/internal/gamificationapi/handler_test.go`
- Modify: `apps/server/internal/gamification/api_helpers.go`（如需要）
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/gamification/badge_test.go`

**Interfaces:**
- Consumes: Task 1 的 Badge/UserBadge、`ServiceOptions.Storage`、M3-A 的 `Repository` 与 `addUserCoins`。
- Produces:
  - `type BadgeInput struct { Name, Description, ImageURL, Category string; PriceCoins int64; IsLimited bool; Stock *int64; SortOrder int; IsActive *bool }`（更新时 Stock 为 nil 表示保留当前库存，ImageURL 为空表示保留现有图片）。
  - `type BadgeItem struct { ... BadgeInput fields; Owned bool; Equipped bool; PurchasedAt *time.Time }`。
  - `type BadgeImage struct { Data []byte; ContentType string }`。
  - `(*Service).ListBadges(ctx, v Viewer, includeInactive bool) ([]BadgeItem, error)`。
  - `CreateBadge/UpdateBadge/DeleteBadge`。
  - `UploadBadgeImage(ctx, v Viewer, id int64, r io.Reader) error`。
  - `BadgeImage(ctx, id int64) (*BadgeImage, error)`。
  - `PurchaseBadge(ctx, userID, id int64) error`。
  - `EquipBadge/UnequipBadge`。
  - `ListUserBadges(ctx, userID int64) ([]BadgeItem, error)`。
  - API：`GET/POST /api/badges`、`PUT/DELETE /api/badges/:id`、`POST /api/badges/:id/image`、`GET /api/badges/:id/image`、`POST /api/badges/:id/purchase|equip|unequip`、`GET /api/users/:id/badges`。
  - `main.go` 使用 `gamification.NewServiceWithOptions(repo, ServiceOptions{MaxEquippedBadges: cfg.MaxEquippedBadges, Storage: d.Storage}, gamePublisher)`，并把 `uploads` 路由组传给 `gamificationapi.RegisterRoutes`。

- [ ] **Step 1: 写失败服务测试**

```go
func TestBadgePurchaseAndEquipLimit(t *testing.T) {
    // 创建 4 个免费/低价徽章；购买全部；装备 3 个成功，第 4 个 40001
    // 卸下 1 个后第 4 个可装备
}
func TestLimitedBadgeConcurrentPurchase(t *testing.T) {
    // 限量 stock=1，两个用户并发购买：一个成功、一个 40001，stock=0
}
func TestBadgePurchaseIdempotentAndBalance(t *testing.T) {
    // 价格 100，余额 100；第一次购买成功、余额 0、流水 -100
    // 第二次购买 40001；不重复扣款
}
func TestBadgeImageUploadAndServe(t *testing.T) {
    // 上传 png magic bytes → ImageURL=/api/badges/:id/image
    // BadgeImage 返回 image/png；>2MiB、text/plain 被拒绝
}
func TestAchievementRewardsBadgeWithoutStock(t *testing.T) {
    // 限量库存 0 的徽章作为成就奖励；解锁后 UserBadge 存在、库存不变
    // 重复 EvaluateUser 不重复插入；成就更新省略 rewardBadgeId 时保留原值
}
func TestInactiveOwnedBadgeVisibleAndEquippable(t *testing.T) { ... }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run 'TestBadge|TestLimited|TestAchievementRewardsBadge' -v`
Expected: FAIL。

- [ ] **Step 3: 实现徽章领域逻辑**

- `ListBadges`：管理员 `includeInactive` 返回全部；游客返回 active；登录用户额外返回已拥有的 inactive。每项填 `owned/equipped/purchasedAt`。
- `Create/Update/Delete`：super_admin；name 1–100、description ≤500、category 1–20、price 0–1,000,000、提交的 stock 必须 ≥0（更新省略时保留当前库存，图片留空时保留现有图片）、imageUrl 为空或 http(s)/内部路径；删除软删除。
- `PurchaseBadge`：事务内 `SELECT badge FOR UPDATE` → 校验 active → 限量扣库存前检查 `stock>0` → 检查 `user_badges` 是否已拥有 → 锁用户行校验余额 → 插入 user_badges → 写 `coin_transactions`（reason `badge_purchase`, refType `badge`, refID badgeID，amount=-price）→ 更新余额。价格 0 也写 amount 0 流水以保留记录。
- `EquipBadge`：事务内锁用户行 → 校验拥有 → 已装备则幂等返回 → 统计 `is_equipped=true` 数量 → 达到上限返回 40001 → 更新目标 true。
- `UnequipBadge`：锁用户行 → 校验拥有 → 更新 false。
- `UploadBadgeImage`：管理员；读取至多 2MiB+1，超过返回 40001；`http.DetectContentType` 仅允许 png/jpeg/webp/gif；`store.Put("badges/"+id)`；更新 `image_url` 为 `/api/badges/<id>/image`。
- `BadgeImage`：读取 `badges/<id>`，≤2MiB，返回检测到的 content type；不存在返回 404。
- `ListUserBadges`：公开；用户不存在返回 404；返回全部拥有徽章及装备状态。

- [ ] **Step 4: 接入成就徽章奖励**

- 修改 `Achievement` 服务创建/更新校验：`RewardBadgeID != nil` 时 `FindBadge` 必须存在且 `IsActive`；M3-A 的“非空拒绝”改为真实校验。
- 同步更新 M3-A 的成就测试与 API 测试：原来断言 `rewardBadgeId` 非空返回 40001 的用例改为“徽章不存在/已下线返回 40001，合法徽章成功”。
- 更新成就时如果输入 `RewardBadgeID == nil`，保留 `existing.RewardBadgeID`；只有显式传 0？JSON 无法区分 null 与 omit；规则：nil 保留，若需要清除提供 `clearRewardBadge=true` 字段（新增到 Input）。为降低复杂度，M3-B 只允许 nil=保留，不支持清除；文档写明。
- `GrantAchievement` 事务内，若 `a.RewardBadgeID != nil`，插入 `user_badges`（`ON CONFLICT DO NOTHING`），不检查库存/active，不扣库存。

- [ ] **Step 5: 实现 API、图片路由与权限**

- `gamificationapi` 注册：
  - `api.GET("/badges", ...)`、`api.GET("/badges/:id/image", ...)`、`api.GET("/users/:id/badges", ...)`
  - `uploads.POST("/badges/:id/image", admin...)`（复用 40MB group，service 内限制 2MB）
  - 管理 CRUD/purchase/equip 路由。
- 图片响应：`c.Data(200, contentType, data)` + `Cache-Control: public, max-age=3600`。
- 增加 handler 测试：游客 200、普通用户管理 403、super_admin CRUD、购买/装备、图片上传/读取、错误参数 40001。

- [ ] **Step 6: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/gamificationapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/gamificationapi apps/server/internal/router
git commit -m "feat(server): badges, purchases and images"
```

---

### Task 3: 称号解锁、购买、装备与 API

**Files:**
- Create: `apps/server/internal/gamification/title.go`
- Create: `apps/server/internal/gamification/title_repository.go`（本任务填满）
- Create: `apps/server/internal/gamification/title_test.go`
- Modify: `apps/server/internal/gamification/dto.go`
- Modify: `apps/server/internal/gamification/achievement.go`（`EvaluateUser` 调 `EvaluateTitles`）
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Modify: `apps/server/internal/gamificationapi/handler_test.go`
- Test: `apps/server/internal/gamification/title_test.go`

**Interfaces:**
- Consumes: Task 1 的 Title/UserTitle、M3-A 条件计数与 `Repository`、Task 2 的金币事务方法。
- Produces:
  - `type TitleUnlockType string`：`rating`、`ac_count`、`achievement`。
  - `type TitleUnlockConfig struct { Rating int; Threshold int64; AchievementCode string }`。
  - `type TitleInput struct { Name, Color string; UnlockConditionType TitleUnlockType; UnlockConditionConfig TitleUnlockConfig; PriceCoins int64; SortOrder int; IsActive *bool }`。
  - `type TitleProgress struct { Type TitleUnlockType; Current int64; Target int64; Done bool; AchievementCode string }`（JSON `type/current/target/done/achievementCode`）。
  - `type TitleItem struct { ...; Unlocked bool; UnlockedAt *time.Time; Purchased bool; PurchasedAt *time.Time; Equipped bool; Progress TitleProgress }`。
  - `(*Service).ListTitles(ctx, v Viewer, includeInactive bool) ([]TitleItem, error)`。
  - `CreateTitle/UpdateTitle/DeleteTitle`。
  - `PurchaseTitle/EquipTitle/UnequipTitle`。
  - `(*Service).EvaluateTitles(ctx, userID int64) error`。
  - API：`GET/POST /api/titles`、`PUT/DELETE /api/titles/:id`、`POST /api/titles/:id/purchase|equip|unequip`。

- [ ] **Step 1: 写失败服务测试**

```go
func TestTitleUnlockConditions(t *testing.T) {
    // rating 1600：用户 1500 未解锁；UPDATE users SET rating=1600 后 EvaluateUser 解锁
    // ac_count 1：已有 AC 解锁
    // achievement first-ac：用户已解锁该成就时解锁
}
func TestTitlePurchaseAndEquipExclusive(t *testing.T) {
    // 未解锁购买 40001；解锁后余额不足 40001
    // 购买 A 扣款；装备 A；购买/装备 B 自动卸下 A
}
func TestTitleInactiveOwnedVisibility(t *testing.T) {
    // 已解锁未购买的下线称号仍可购买；未解锁下线仅管理员可见
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run TestTitle -v`
Expected: FAIL。

- [ ] **Step 3: 实现称号领域逻辑**

- 校验：name 1–100、color `^#[0-9A-Fa-f]{6}$`、price 0–1,000,000、unlock 类型/配置合法。
- 解锁条件求值：
  - `rating`：`users.rating >= cfg.Rating`。
  - `ac_count`：复用 `CountAcceptedProblems`。
  - `achievement`：`user_achievements` 关联 `achievements.code = cfg.AchievementCode`。
- `EvaluateTitles`：active 称号 + 未解锁 map；满足则 `INSERT user_titles`（唯一约束）。
- `ListTitles`：先 `EvaluateUser`（内含 `EvaluateTitles`）；管理员 includeInactive；游客 active；登录用户 active + 自己已解锁的 inactive；每个返回 `progress`。
- 购买：事务锁用户行 + 读 user_title（`FOR UPDATE`）；未解锁/已购买/余额不足返回 40001；写流水 `title_purchase/title/titleID`（amount=-price，0 也写）；更新 `purchased_at`。
- 装备：事务锁用户行；校验已购买；`UPDATE user_titles SET is_equipped=false WHERE user_id=?`；目标 true。
- 删除软删除。

- [ ] **Step 4: 接入 `EvaluateUser` 与 API**

- `EvaluateUser` 在成就/任务后调用 `EvaluateTitles`。
- API 注册与权限同徽章：公开列表，管理 super_admin，购买/装备登录。
- handler 测试覆盖：游客列表、普通用户管理 403、解锁进度、购买/装备、未解锁/余额不足 40001。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/gamificationapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/gamificationapi
git commit -m "feat(server): titles and equipment"
```

---

### Task 4: 公开身份与个人主页/测评记录/比赛榜单展示

**Files:**
- Modify: `apps/server/internal/gamification/public.go`
- Create: `apps/server/internal/gamification/public_test.go`
- Modify: `apps/server/internal/user/dto.go`
- Modify: `apps/server/internal/userapi/handler.go`
- Modify: `apps/server/internal/userapi/handler_test.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/contest/ranklist.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 2/3 的 `UserBadge`/`UserTitle` 表与 `Service`。
- Produces:
  - `func (s *Service) Identities(ctx context.Context, userIDs []int64) (map[int64]Identity, error)`：返回每个输入用户（包括无装备者）的 `Identity`，Badges 至少为空切片。
  - `type IdentityLookup interface { Identities(ctx, []int64) (map[int64]Identity, error) }`（在 userapi/submissionapi/contestapi 各自定义本地接口）。
  - 三个 handler 包统一用 Options 结构，避免多个不同类型的变参冲突：
    - `userapi.Options{IdentityLookup IdentityLookup}` + `RegisterRoutes(r, s, opts ...Options)`
    - `submissionapi.Options{IdentityLookup IdentityLookup}` + `RegisterRoutes(r, s, streamer, opts ...Options)`
    - `contestapi.Options{GameHook GameHook; IdentityLookup IdentityLookup}` + `RegisterRoutes(r, s, opts ...Options)`
  - router 在 `d.Game != nil` 时传 `Options{GameHook: d.Game, IdentityLookup: d.Game}`；现有测试的无参调用继续兼容。
  - `user.ProfileDTO.EquippedBadges []gamification.BadgeSummary`、`EquippedTitle *gamification.TitleSummary`。
  - `submission.ListItem.EquippedBadges []gamification.BadgeSummary`、`EquippedTitle *gamification.TitleSummary`。
  - `contest.RankRow.EquippedBadges []gamification.BadgeSummary`、`EquippedTitle *gamification.TitleSummary`。
  - 使用上面的 Options 结构接入；`contestapi` 原先的 `GameHook` 变参改为 `Options.GameHook`，router 与测试同步调整。

- [ ] **Step 1: 写失败身份测试**

```go
func TestIdentitiesBatch(t *testing.T) {
    // 3 个用户，1 个装备 2 徽章 + 1 称号，1 个只有徽章，1 个无装备
    // 返回 map 含全部 3 个 key；无装备者 Badges 为空数组而非 nil
}
func TestIdentitiesOrderAndInactive(t *testing.T) {
    // 徽章按 sort_order,id；称号最多 1 个；下线的已装备内容仍返回
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run TestIdentities -v`
Expected: FAIL。

- [ ] **Step 3: 实现批量身份查询**

- 一条查询 `user_badges JOIN badges`：`user_id IN ? AND is_equipped=true`，按 `badges.sort_order, badges.id`；不筛 `badges.is_active`。
- 一条查询 `user_titles JOIN titles`：`user_id IN ? AND is_equipped=true`；不筛 active。
- 初始化 map 中每个输入 id 为空 `Identity{Badges: []BadgeSummary{}}`，再填充。
- 去重、忽略 `userID<=0`。

- [ ] **Step 4: 接入个人主页**

- `userapi` handler 可选 lookup；profile 读取后调用 `Identities(ctx, []int64{id})`，填充 DTO。
- `user.ProfileDTO` 改为 typed 字段；`ToProfile` 默认空切片/nil。
- 测试：有装备的用户返回徽章/称号；无装备返回 `[]` 和 null；游客可读。

- [ ] **Step 5: 接入测评记录**

- `submission.ListItem` 增加字段（`json:"equippedBadges"`/`json:"equippedTitle"`），`Detail` 继承。
- `submissionapi` handler 可选 lookup；列表收集当前页 userID，批量查询后逐项填充；详情查询单个。
- 不得改变 `toListItem` 的赛时隐藏逻辑；装备字段在 service 返回后由 handler 装饰。
- 测试：列表/详情返回装备；赛时保护字段仍隐藏；无装备返回空数组。

- [ ] **Step 6: 接入比赛榜单**

- `contest.RankRow` 增加字段；`Ranklist` 计算完成后不填身份。
- `contestapi` handler 在 `svc.Ranklist` 成功后收集 rows 的 userID，批量查身份并填充；OI `selfOnly` 时只装饰保留下来的行。
- 测试：ICPC/IOI/OI 榜单行含装备；赛时保护/selfOnly 行数不变；空榜单不报错。

- [ ] **Step 7: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/user internal/userapi internal/submission internal/submissionapi internal/contest internal/contestapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/userapi/ ./internal/submissionapi/ ./internal/contestapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/user apps/server/internal/userapi apps/server/internal/submission apps/server/internal/submissionapi apps/server/internal/contest apps/server/internal/contestapi apps/server/internal/router
git commit -m "feat(server): public equipped badges and titles"
```

---

### Task 5: OpenAPI、生成类型、文档与全量检查

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端检查

**Interfaces:**
- Consumes: Task 1–4 的 API/字段。
- Produces: M3-B 可用的 OpenAPI 与前端类型。

- [ ] **Step 1: 补 OpenAPI**

新增 schema：`BadgeItem`、`BadgeInput`、`TitleItem`、`TitleInput`、`TitleUnlockType`、`TitleUnlockConfig`、`TitleProgress`、`EquippedBadge`、`EquippedTitle`、`PublicIdentity`。

新增路径：

- `GET/POST /badges`、`PUT/DELETE /badges/{id}`
- `POST /badges/{id}/image`、`GET /badges/{id}/image`
- `POST /badges/{id}/purchase|equip|unequip`
- `GET /users/{id}/badges`
- `GET/POST /titles`、`PUT/DELETE /titles/{id}`
- `POST /titles/{id}/purchase|equip|unequip`

修改现有 schema：

- `Profile.equippedBadges` → `EquippedBadge[]`，`equippedTitle` → `EquippedTitle|null`。
- `SubmissionListItem` 增加 `equippedBadges`、`equippedTitle` 必需字段。
- `ContestRankRow` 增加同样字段。
- `AchievementInput.rewardBadgeId` 在 M3-B 生效；描述写清“徽章必须存在且上架，奖励不占库存”。
- 图片接口响应 `image/png|jpeg|webp|gif`，2 MiB 限制。

- [ ] **Step 2: 生成类型并构建**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web`
Expected: 生成成功；构建通过（M3-C 尚未使用类型）。

- [ ] **Step 3: 更新文档**

- `DEVELOPMENT.md` 3.5、5.3、5.4、4.1、4.6 标记 M3-B 实现状态；M3 仍 🚧（M3-C 前端待办）。
- `docs/任务清单.md`：M3-5/M3-6/M3-7 ✅，M3-8 ⬜；加 M3-B 计划链接。
- `docs/开发日志.md`：追加 M3-B 条目（变更、验证、复审修复、待办 M3-C）。
- `README.md`、`运行手册.md`：当前版本写“M3-B 已完成；前端页面待 M3-C”，补充徽章图片目录/配置 `KAPOJ_MAX_EQUIPPED_BADGES`。

- [ ] **Step 4: 全量检查**

```bash
cd /Users/admin/codespace/kapoj/apps/server
gofmt -l .
go vet ./...
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run

cd /Users/admin/codespace/kapoj/apps/judge
gofmt -l .
go vet ./...
go test -count=1 ./...

cd /Users/admin/codespace/kapoj
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
git diff --check
```

Expected: 后端 22+ 包、judge 11 包、golangci-lint 0 issues、前端 79 测试/lint/build、diff check 均通过。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m3-b badges and titles"
```

---

## Self-Review

**Spec coverage:**

- 徽章 CRUD、商城、购买、限量库存、装备上限、图片上传/读取 → Task 2。
- 成就奖励徽章、不占库存、配置校验 → Task 2。
- 称号解锁条件、购买、装备互斥、下线可见性 → Task 3。
- 公开身份与个人主页/测评记录/比赛榜单 → Task 4。
- OpenAPI/文档 → Task 5。
- M3-C 前端页面不在本计划。

**Type consistency:**

- `BadgeSummary`/`TitleSummary`/`Identity` 在 gamification、user、submission、contest 中复用同一类型。
- `BadgeItem`/`TitleItem` 的 JSON 字段与 OpenAPI 一致。
- 装备上限 `MaxEquippedBadges` 从 config 到 ServiceOptions 到测试一致。
- 徽章图片 URL 固定 `/api/badges/<id>/image`。

**Review Focus mapping:**

1. 限量并发 → Task 2 `TestLimitedBadgeConcurrentPurchase`。
2. 扣款/购买幂等 → Task 2 `TestBadgePurchaseIdempotentAndBalance`。
3. 装备上限并发 → Task 2 并发装备测试（补充 `TestConcurrentEquipLimit`）。
4. 称号互斥 → Task 3 `TestTitlePurchaseAndEquipExclusive`。
5. 公开身份不破坏赛时保护 → Task 4 submission/contest API 测试。

**Proportion:** 计划固定接口、事务规则和测试；具体查询/事务实现按现有 M3-A 风格完成。
