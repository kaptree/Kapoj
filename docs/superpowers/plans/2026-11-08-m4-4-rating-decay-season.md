# M4-4 Rating 衰减与赛季重置 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付可配置的高分段 Rating 衰减、赛季软重置、运行幂等、管理设置页、公开赛季信息与三类 Rating 历史展示。

**Architecture:** `rating` 域新增 `rating_settings` 单例与 `rating_runs` 运行标记；设置/赛季计算放在 `rating/settings.go`，衰减/重置任务放在 `rating/adjustments.go`，复用现有 Rating worker 与全局 advisory lock。`adminapi` 挂载设置 GET/PUT（`rating.settings.manage` 权限 + 审计）；`ratingapi` 增加公开赛季接口并扩展历史接口返回 `kind/periodKey`。前端新增 `/admin/rating` 设置页、Rating 历史类型标签与排行榜赛季信息。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、Vue 3、TypeScript、PrimeVue 4、Vitest、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-07-m4-4-rating-decay-season-design.md`

## Global Constraints

- 衰减仅对 `status=active` 且 `rating > decayThreshold`（默认 2400）的用户生效；最近一次 `kind=contest` 历史距当前 ≥ `decayInactiveDays`（默认 60 天）。
- 衰减公式 `raw=math.Round((rating-threshold)*percent/100)`，`decay=min(max, max(1, raw))`，`new=max(threshold, rating-decay)`；默认 3%、上限 100。
- 赛季默认 6 个月；结束后所有 `rating != 1500` 用户 `new=max(0, math.Round(1500+(rating-1500)*(100-resetPercent)/100))`，默认 30%；计算值不变的用户不写历史。
- 首次创建默认设置与修改 `seasonAnchor`/`seasonMonths` 时，预标记新的“上一赛季”运行键为已跳过，不立即触发重置；跨多赛季只处理最近结束的上一赛季。
- 月度衰减键 `decay:YYYY-MM`；赛季键 `season:YYYY-MM-DD`（上一赛季开始日）；`rating_runs` 唯一键 + advisory lock 保证幂等。
- `rating_history` 新增 `period_key`，部分唯一索引 `(user_id,kind,period_key) WHERE period_key <> ''`；比赛结算 `period_key=''`。
- 新增权限码 `rating.settings.manage`（第 35 个），分组“Rating”，默认只给 super_admin（super 绕过）；设置修改写审计 `rating.settings.manage` / target `rating_settings`。
- 历史接口返回所有 kind：`contest|decay|season_reset`；非比赛行 `contestId=0`、`rank=0`、标题为“Rating 衰减”/“赛季重置”；私密比赛仍脱敏。
- 不新增独立 worker 进程；不提供手动全站重置按钮；不改 Elo 比赛结算公式；不推送/不合并分支。
- 所有用户文案中文；PrimeVue 4；不新增 UI 依赖。

## Review Focus

1. **衰减边界与取整**：2400 不衰减、2401 降 1、2450 降 2、2600 降 6；未活跃 59/60 天边界；上限/floor；`math.Round` 不得退化为整数截断。
2. **赛季语义**：全用户软重置、1500/零变化跳过、0% 仍标记已处理、首次默认不立即重置、改锚点/月数不立即重置、跨多赛季只处理上一赛季。
3. **幂等与并发**：重复 tick、多实例、失败重试不重复扣分/重置；`rating_runs` 与历史唯一索引配合；advisory lock 与比赛结算串行。
4. **历史契约**：kind/periodKey/标签/rank=0/contestId=0；私密比赛脱敏；分页 total 含所有 kind；前端链接仅比赛行。
5. **权限与审计**：`rating.settings.manage` 授权/403/401；super 绕过；设置修改审计；角色权限矩阵显示新分组。
6. **设置校验与默认**：范围校验；空库首次读取自动创建默认设置且不立即重置；修改设置即时影响下一次运行，不追溯。
7. **榜单缓存**：衰减/重置成功后版本键递增；缓存故障不回滚业务。

---

### Task 1: 设置、权限、管理 API 与赛季计算

**Files:**
- Modify: `apps/server/internal/rating/model.go`
- Modify: `apps/server/internal/rating/service.go`（`Options.Now` 与 `now()`）
- Create: `apps/server/internal/rating/settings.go`
- Create: `apps/server/internal/rating/settings_test.go`
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Modify: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/rbac/roles_test.go`
- Create: `apps/server/internal/adminapi/rating_handler.go`
- Create: `apps/server/internal/adminapi/rating_handler_test.go`
- Modify: `apps/server/internal/adminapi/coin_handler.go`（`Options.Rating`）
- Modify: `apps/server/internal/adminapi/role_handler.go`（注册路由/handler 字段）
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Test: 上述测试文件

**Interfaces:**
- Produces:
  - `rating.HistoryDecay HistoryKind = "decay"`、`rating.HistorySeasonReset HistoryKind = "season_reset"`。
  - `rating.History.PeriodKey`（字段 `rating_history.period_key`）。
  - `rating.RatingSettings`（`rating_settings`）与 `rating.RatingRun`（`rating_runs`）模型。
  - `rating.SettingsInput`、`rating.SettingsView`、`rating.SeasonView`。
  - `rating.Options.Now func() time.Time`（可选；默认 `time.Now`）；`Service.now()` 返回 `Options.Now` 或系统时间。
  - `Service.Settings(ctx) (*SettingsView, error)`、`Service.UpdateSettings(ctx, actorID int64, in SettingsInput) (*SettingsView, error)`、`Service.CurrentSeason(ctx) (SeasonView, error)`。
  - 内部 `ensureSettings(ctx)`：不存在则按默认值创建并预标记上一赛季运行键为已跳过。
  - `rbac.PermRatingSettingsManage = "rating.settings.manage"`、`rbac.PermissionGroupRating = "Rating"`、`rbac.AuditActionRatingSettingsManage`、`rbac.AuditTargetRatingSettings`。
  - `adminapi.Options.Rating *rating.Service`；`GET/PUT /api/admin/rating/settings`。
  - 默认值：decayEnabled=true, threshold=2400, inactiveDays=60, percent=3, max=100, seasonEnabled=true, months=6, resetPercent=30, anchor=读取时当月 1 日（UTC）。
  - 校验范围：threshold 1500–4000；inactiveDays 1–365；decayPercent 1–100；decayMax 1–1000；seasonMonths 1–24；resetPercent 0–100；anchor `YYYY-MM-DD`。
- Consumes: 现有 `Service`/`Options`/`RatingHistory`、`rbac` 权限目录、`adminapi` handler。

- [ ] **Step 1: 写失败测试**

`settings_test.go`：
```go
func TestSettingsDefaultsAndLazyCreate(t *testing.T)
// 空库首次 Settings 创建默认行；anchor 为固定 now 的当月 1 日；
// rating_runs 含上一赛季跳过行，SeasonView 正确
func TestUpdateSettingsValidationAndScheduleSkip(t *testing.T)
// 非法 threshold/days/percent/max/months/reset/anchor -> 40001
// 修改 anchor/months 插入新的上一赛季跳过行；未修改不插入
func TestCurrentSeason(t *testing.T)
// anchor=2026-11-01, months=6, now=2026-12-15 -> current 2026-11-01..2027-05-01,
// nextResetAt=2027-05-01；now=2027-06-01 -> current 2027-05-01..2027-11-01
```

`adminapi/rating_handler_test.go`：
```go
func TestAdminRatingSettingsPermission(t *testing.T)
// 无权限 403；super 200；有 rating.settings.manage 角色 200
func TestAdminRatingSettingsUpdateAndAudit(t *testing.T)
// PUT 校验/保存/返回 currentSeason；审计 action=rating.settings.manage target=rating_settings
```

`rbac` 测试：权限总数 34→35，目录包含 `rating.settings.manage` 且分组“Rating”。

`db_test.go`：`rating_settings`/`rating_runs` 表、`rating_history.period_key` 列、`uniq_rating_history_user_period` 索引存在。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/adminapi/ ./internal/rbac/ ./internal/db/ -run 'TestSettings|TestUpdateSettings|TestCurrentSeason|TestAdminRatingSettings|TestPermissionCatalog|TestMigrateCreatesRating' -v`

Expected: FAIL。

- [ ] **Step 3: 实现模型、设置、权限、管理 API**

- `model.go`：新增 kind 常量与 `PeriodKey`；`RatingSettings`/`RatingRun` 模型与 `TableName`。
- `settings.go`：
  - `ensureSettings` 事务：`INSERT ... ON CONFLICT DO NOTHING`；默认 anchor `time.Date(now.Year(), now.Month(), 1, 0,0,0,0, time.UTC)`；对新 anchor 计算“上一赛季”并把 `rating_runs(key, kind, processed_at)` 预标记为 `now`（`ON CONFLICT DO NOTHING`）。
  - `Settings`/`UpdateSettings`/`CurrentSeason`；`UpdateSettings` 行锁 `id=1`；如 anchor/months 变化，预标记新上一赛季；校验错误用 `httpx.ErrValidation` 中文。
  - `seasonFor(anchor, months, now)`：当前赛季与上一赛季；`SeasonView{Key,Name,StartsAt,EndsAt,NextResetAt}`；Name 为 `YYYY-MM-DD ~ YYYY-MM-DD`（结束日 inclusive）。
- `rbac/catalog.go`：权限码/分组/目录条目；`seed_test.go` 34→35；`roles_test.go` 34→35。
- `rbac/audit.go`：action/target 常量。
- `adminapi`：`Options.Rating`、handler 字段、路由：
  ```go
  api.GET("/admin/rating/settings", append(requirePermission(s, string(rbac.PermRatingSettingsManage)), h.getRatingSettings)...)
  api.PUT("/admin/rating/settings", append(requirePermission(s, string(rbac.PermRatingSettingsManage)), h.updateRatingSettings)...)
  ```
  - PUT 使用 `auditActor(c)` + `h.svc.RecordAudit(...)` best-effort；日志失败只记录。
  - 只在 `h.rating != nil` 时挂载。
- `router.go`：`adminapi.RegisterRoutes(api, d.RBAC, adminapi.Options{Game: d.Game, Rating: d.Rating})`。
- `db.go`：`AutoMigrate` 加两个模型；`db_test.go` 断言。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/rating internal/adminapi internal/rbac internal/db internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/adminapi/ ./internal/rbac/ ./internal/db/ ./internal/router/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/rating apps/server/internal/rbac apps/server/internal/adminapi apps/server/internal/router apps/server/internal/db
git commit -m "feat(rating): add decay settings and admin API"
```

---

### Task 2: 高分段月度衰减任务

**Files:**
- Create: `apps/server/internal/rating/adjustments.go`
- Create: `apps/server/internal/rating/adjustments_test.go`
- Modify: `apps/server/internal/rating/worker.go`
- Test: `apps/server/internal/rating/adjustments_test.go`

**Interfaces:**
- Consumes: Task 1 的 `ensureSettings`/`Settings`、`HistoryDecay`、`RatingRun`、`Options.Now`、现有 `LeaderboardVersionKey`。
- Produces:
  - `Service.RunAdjustments(ctx context.Context, now time.Time) error`：Task 2 只执行衰减；Task 3 扩展赛季重置。
  - 内部 `runDecay(ctx, now, settings)`。
  - `rating_runs` 键 `decay:YYYY-MM`；处理成功/无符合用户均写 `processed_at`。
  - 衰减历史 `kind=decay`、`period_key=YYYY-MM`、`rank=0`、`contest_id=NULL`。
  - 调整成功修改 Rating 后递增 `LeaderboardVersionKey`（best-effort）。

- [ ] **Step 1: 写失败测试**

```go
func TestDecayFormulaAndThreshold(t *testing.T)
// 用户 2400 不变；2401->2400；2450->2448；2600->2594
func TestDecayInactiveBoundary(t *testing.T)
// 59 天不衰减；60 天衰减；无 contest 历史不衰减；非 active 不衰减
func TestDecayCapAndFloor(t *testing.T)
// decayMax=10 时 3000->2990；threshold 边界不跌破
func TestDecayIdempotentSameMonth(t *testing.T)
// 同 now 连续 RunAdjustments 两次：Rating 只扣一次、只有一条 history、run processed
func TestDecayCrossMonthNotBackfilled(t *testing.T)
// 跨月停机只处理当前月，不补历史月
func TestDecayCacheVersion(t *testing.T)
// 有实际变化时 Redis 版本键 +1；无变化不递增
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ -run 'TestDecay' -v`

Expected: FAIL。

- [ ] **Step 3: 实现衰减任务**

- `RunAdjustments(ctx, now)`：`now=now.UTC()`；加载设置；若 `decayEnabled` 调 `runDecay`。
- `runDecay` 事务：
  1. `pg_advisory_xact_lock(settlementAdvisoryLockKey)`。
  2. 重新读取设置（事务内）。
  3. 键 `decay:<YYYY-MM>`；查询 `rating_runs`；`processed_at != nil` 直接返回；不存在则插入 `(key,kind,created_at)`。
  4. 查询符合条件用户并 `FOR UPDATE`：
     ```sql
     SELECT u.id, u.rating FROM users u
     WHERE u.status = 'active' AND u.rating > ?
       AND EXISTS (SELECT 1 FROM rating_history rh WHERE rh.user_id=u.id AND rh.kind='contest')
       AND NOT EXISTS (SELECT 1 FROM rating_history rh
                       WHERE rh.user_id=u.id AND rh.kind='contest' AND rh.created_at >= ?)
     FOR UPDATE
     ```
     `?` 为 `now.AddDate(0,0,-inactiveDays)`。
  5. 逐人：`above=rating-threshold`；`raw=int(math.Round(float64(above)*float64(percent)/100))`；`decay=min(max, max(1, raw))`；`new=max(threshold, rating-decay)`；若 `new==rating` 跳过；`INSERT ... ON CONFLICT (user_id,kind,period_key) WHERE period_key <> '' DO NOTHING`；RowsAffected=1 才 `UPDATE users SET rating=?`。
  6. `UPDATE rating_runs SET processed_at=? WHERE key=?`；提交。
- 提交后若有变化，`rdb.Incr(LeaderboardVersionKey)`；Redis 错误仅日志。
- `worker.go` 在每次 `scan` 后调用 `s.RunAdjustments(ctx, s.now())`（`now` 取自 Options；Task 1 已加）。
- 失败回滚 run 行/历史/用户更新，下一 tick 重试。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/rating && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/rating
git commit -m "feat(rating): add monthly high-rating decay"
```

---

### Task 3: 赛季重置、公开赛季与多 kind 历史

**Files:**
- Modify: `apps/server/internal/rating/adjustments.go`
- Modify: `apps/server/internal/rating/adjustments_test.go`
- Modify: `apps/server/internal/rating/history.go`
- Modify: `apps/server/internal/rating/history_test.go`
- Modify: `apps/server/internal/ratingapi/handler.go`
- Modify: `apps/server/internal/ratingapi/handler_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 `seasonFor`/`ensureSettings`、Task 2 的 `RunAdjustments` 事务模式。
- Produces:
  - `RunAdjustments` 在衰减后执行 `runSeasonReset(ctx, now, settings)`。
  - 运行键 `season:<上一赛季开始日>`；全用户 `rating != 1500` 软重置；`kind=season_reset`、`period_key=<上一赛季开始日>`、`rank=0`。
  - `GET /api/rating/season/current` → `rating.SeasonView`（公开）。
  - `rating.HistoryItem` 增加 `Kind HistoryKind`、`PeriodKey string`；查询所有 kind；非比赛行 `ContestID=0`、`Rank=0`、标题标签。
  - 私密比赛仍返回 `contestId=0`/“私密比赛”。

- [ ] **Step 1: 写失败测试**

`adjustments_test.go`：
```go
func TestSeasonResetFormulaAndScope(t *testing.T)
// 2600->2270；1200->1290；1500 跳过；0->450；不写零 delta
func TestSeasonResetScheduleSkip(t *testing.T)
// 默认设置/改 anchor 后首次 RunAdjustments 不重置；下一赛季结束后才重置
func TestSeasonResetAcrossMissedSeasons(t *testing.T)
// 跨多个赛季只处理最近结束的上一赛季
func TestSeasonResetIdempotent(t *testing.T)
// 重复 RunAdjustments 不重复重置/写历史
func TestSeasonResetCacheVersion(t *testing.T)
```

`history_test.go` 更新：
```go
// 原“decay 不出现”改为：contest/decay/season_reset 均返回；
// 非比赛行 contestId=0、rank=0、contestTitle 标签、periodKey；
// 分页 total 包含所有 kind；私密比赛仍脱敏。
```

`ratingapi/handler_test.go`：
```go
// GET /api/rating/season/current 200；无设置时懒创建默认；响应字段正确。
// GET /api/users/:id/rating-history 返回 kind/periodKey/非比赛标签。
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/ratingapi/ -run 'TestSeason|TestRatingHistory|TestRatingSeason' -v`

Expected: FAIL。

- [ ] **Step 3: 实现赛季与历史**

- `runSeasonReset`：
  - `seasonEnabled=false` 返回；`seasonFor` 得 current/previous；若 previous 不存在或 `previous.EndsAt.After(now)` 返回（未结束）。
  - 键 `season:<previous.StartsAt.Format("2006-01-02")>`；run 已处理返回；未认领则插入。
  - 事务内 advisory lock；`SELECT id,rating FROM users WHERE rating <> 1500 FOR UPDATE`；逐人 `new=max(0,int(math.Round(1500+float64(rating-1500)*float64(100-resetPercent)/100)))`；`new==rating` 跳过；插入 history `ON CONFLICT ... DO NOTHING`；RowsAffected=1 才更新；标记 run；提交。
  - 有变化则 bump 榜单版本。
- `history.go`：
  - `HistoryItem` 加 `Kind`/`PeriodKey`；SQL `LEFT JOIN contests`；`COALESCE(rh.contest_id,0)`；`CASE` 标题：
    - `decay` → `Rating 衰减`
    - `season_reset` → `赛季重置`
    - `contest` + `c.is_public` → `c.title`
    - `contest` + 非公开 → `私密比赛`
  - 去掉 `rh.kind = contest` 过滤；`rank` 非比赛为 0；`period_key` 返回。
- `ratingapi`：
  - `GET /rating/season/current` 调 `svc.CurrentSeason`，`httpx.OK`。
  - 历史 handler 不变，DTO 新字段随服务返回。
- 前端类型/契约在 Task 4。

- [ ] **Step 4: 运行测试、gofmt、lint**

Run: `cd apps/server && gofmt -w internal/rating internal/ratingapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/ratingapi/ ./internal/router/ -v && /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/rating apps/server/internal/ratingapi
git commit -m "feat(rating): add season reset and season history"
```

---

### Task 4: OpenAPI、类型与管理/历史/榜单前端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/leaderboard.ts`
- Modify: `apps/web/src/permissions.ts`
- Create: `apps/web/src/views/AdminRatingView.vue`
- Create: `apps/web/src/views/AdminRatingView.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/components/RatingHistoryCard.vue`
- Add/Modify: `apps/web/src/components/RatingHistoryCard.spec.ts`（若不存在则创建）
- Modify: `apps/web/src/views/LeaderboardView.vue`
- Modify: `apps/web/src/views/LeaderboardView.spec.ts`
- Modify: `apps/web/src/views/AdminRolesView.spec.ts`（34→35）
- Modify: `apps/web/e2e/admin-rbac.spec.ts`（34→35）
- Test: 上述 spec

**Interfaces:**
- Consumes: Task 1–3 的 API/DTO。
- Produces:
  - OpenAPI：`RatingSettingsView`、`RatingSettingsInput`、`RatingSeasonView`、`RatingHistoryItem.kind/periodKey`；`/admin/rating/settings` GET/PUT；`/rating/season/current` GET。
  - `api/types.ts` 类型；`api/admin.ts` `getAdminRatingSettings`/`updateAdminRatingSettings`；`api/leaderboard.ts` `getRatingSeason`。
  - `permissions.ts` `PERM.rating.settings.manage` + `ADMIN_ACCESS_PERMISSIONS`。
  - 路由 `admin-rating` → `/admin/rating`，`requiresAuth` + `permission: PERM.rating.settings.manage`；AdminNav 内容组或新分组“Rating”入口。
  - `AdminRatingView` 表单：衰减开关/阈值/未活跃天数/比例/上限；赛季开关/月数/重置比例/锚点；只读当前赛季与下次重置；保存 Toast/错误。
  - `RatingHistoryCard`：类型标签（比赛/衰减/赛季重置）；非比赛无链接；`rank` 显示“—”或 0；图表包含所有记录；标题“最近 20 条 Rating 变化”。
  - `LeaderboardView`：顶部显示当前赛季名称与下次重置；请求失败静默降级。

- [ ] **Step 1: OpenAPI + 生成类型**

- 更新 schemas/paths，`corepack pnpm gen:api`；检查 `RatingHistoryItem` 新增 required `kind`/`periodKey`；`RatingSettingsView.currentSeason`。
- 不允许手写重复类型。

- [ ] **Step 2: 写失败前端测试**

`AdminRatingView.spec.ts`：
```ts
it('loads settings and current season')
it('saves valid settings and shows toast')
it('shows validation errors for out-of-range values')
it('hidden/forbidden without rating.settings.manage')
```
`RatingHistoryCard.spec.ts`：
```ts
it('renders contest, decay and season_reset rows with labels')
it('links only contest rows')
it('updates chart label/count for all kinds')
```
`LeaderboardView.spec.ts`：
```ts
it('shows current season and next reset')
it('ignores season fetch failure')
```
`AdminNav.spec.ts`：有 `rating.settings.manage` 显示 Rating 设置，无权限不显示。

- [ ] **Step 3: 实现前端**

- `AdminRatingView`：`AppPageHeader`/`AppSurface`/`AppFormField`；`ToggleSwitch`、`InputNumber`、`InputText`（日期 `YYYY-MM-DD`）；加载/错误/保存；当前赛季只读；权限由路由保证。
- `RatingHistoryCard`：`kind` 标签用 `Tag`/自定义；`contestName` 改为直接用 `item.contestTitle`；链接条件 `kind==='contest' && contestId>0`；排名非比赛显示“—”；图表 label “共 N 条 Rating 变化”。
- `LeaderboardView`：`season = ref<RatingSeasonView|null>`；`onMounted` 拉取；header 显示 `赛季 <name> · 下次重置 <date>`；失败不显示。
- `AdminRolesView.spec`/`admin-rbac.spec` 34→35；`AdminNav` 入口/路由。
- `permissions.ts` 新权限；`ADMIN_ACCESS_PERMISSIONS` 增加。

- [ ] **Step 4: 运行测试、lint、build**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api apps/web/src/permissions.ts apps/web/src/router/index.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/views/AdminRatingView.vue apps/web/src/views/AdminRatingView.spec.ts apps/web/src/components/RatingHistoryCard.vue apps/web/src/components/RatingHistoryCard.spec.ts apps/web/src/views/LeaderboardView.vue apps/web/src/views/LeaderboardView.spec.ts apps/web/src/views/AdminRolesView.spec.ts apps/web/e2e/admin-rbac.spec.ts
git commit -m "feat(web): add rating settings and season history"
```

---

### Task 5: 文档、全量检查与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Test: 全量后端、评测、前端、真实浏览器/API

**Interfaces:**
- Consumes: Task 1–4。
- Produces: M4-4 状态文档、权限计数 35、冒烟记录。

- [ ] **Step 1: 文档**

- `DEVELOPMENT.md`：§4.7 权限目录第 35 个与 Rating 分组；§5.5 Rating 衰减/赛季；§3.8/4.x 新表 `rating_settings`/`rating_runs`/`rating_history.period_key`；Worker 语义。
- `docs/任务清单.md`：M4-4 ✅；M4 整体仍 🚧（社交未完成）；更新 M4 进度说明。
- `docs/开发日志.md`：2026-11-08 M4-4 条目（设置/衰减/赛季/历史/前端/测试/冒烟）。
- `README.md`：当前进度与 Rating 赛季说明。
- `运行手册.md`：设置页、权限、衰减/赛季验证命令。
- `docs/primevue-migration.md`：管理页新增 Rating 设置（不新增 UI 依赖）。

- [ ] **Step 2: 全量检查**

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

- [ ] **Step 3: 真实冒烟**

- 使用当前后端（必要时重建/重启）与前端；管理员设置 `rating.settings.manage` 权限用临时角色/账号验证 GET/PUT/审计；普通用户 403。
- 临时用户 Rating 2500、最后 Rated 比赛 61 天前：设置阈值 2400/3%/上限 100，等待 Worker tick 或重建重启触发；验证 2500→2497（`above=100→3`）、历史 `kind=decay`、榜单缓存/排名更新；重复 tick 不重复扣分。
- 边界：2400 用户不变；2401→2400；2450→2448；未活跃 59 天不变。
- 赛季：**使用临时数据库/临时 server（如 `kapoj_m44_smoke`，监听 `:18081`）**，不重置开发库既有用户；设置 anchor 为已结束赛季，验证 2600→2270、1200→1290、1500 跳过、零变化不写历史；重复 tick 幂等；跨多赛季只处理上一赛季；默认设置/改 anchor 不立即重置。
- 历史 API/前端：三类记录标签、非比赛无链接、私密比赛脱敏、分页 total；管理页保存/校验；排行榜赛季信息。
- 清理临时用户/设置/运行标记/历史/审计/缓存版本；恢复开发库默认设置；不删除既有数据。
- 浏览器如可用，验证 `/admin/rating`、用户主页 Rating 历史、排行榜赛季；375px/690px 可用。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record rating decay and season reset"
```

---

## Self-Review

**Spec coverage:**

- 设置/权限/管理 API → Task 1。
- 衰减任务 → Task 2。
- 赛季重置/公开赛季/多 kind 历史 → Task 3。
- OpenAPI/前端 → Task 4。
- 文档/检查/冒烟 → Task 5。

**Type consistency:**

- `rating.HistoryKind` 新增 `decay`/`season_reset`；`HistoryItem.Kind/PeriodKey` 在 Task 3 定义，Task 4 类型生成复用。
- `SettingsView/Input/SeasonView` 在 Task 1 定义，Task 4 前端类型复用。
- `RunAdjustments(ctx, now)` 在 Task 2 定义，Task 3 扩展赛季；worker 使用 `Options.Now`。
- `rating.settings.manage` 在 Task 1 定义，Task 4 前端权限/路由复用。

**Review Focus mapping:**

1. 衰减边界/取整 → Task 2 tests。
2. 赛季语义 → Task 3 tests。
3. 幂等/并发 → Task 2/3 tests。
4. 历史契约 → Task 3 tests + Task 4 card tests。
5. 权限/审计 → Task 1 tests + Task 5 smoke。
6. 设置默认/校验 → Task 1 tests + Task 5 smoke。
7. 榜单缓存 → Task 2/3 tests。
