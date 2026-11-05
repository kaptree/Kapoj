# M4-B 活跃度、签到金币、共享积分与三个榜单 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M4 第二阶段：活跃度（提交/参赛/签到）、签到金币、共享积分、活跃度榜/共享榜，并完成三个榜单 API。

**Architecture:** 新增 `activity` 包管理 `activity_logs`/`shared_items`、每日上限/去重、签到金币、共享回填、活跃度/共享榜与 Redis 版本缓存；提交/参赛/比赛创建/克隆通过 handler Options 钩子写入，失败只日志；榜单行在 API 层批量装饰 gamification 身份。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、testify。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-leaderboard-rating-learning-design.md` 5、6。

## Global Constraints

- 活跃度规则固定：submission +2（每日上限 40）、contest_join +10（每日上限 50）、signin +5（每天一次）。
- 签到额外 +5 金币，写 `coin_transactions`（`reason=signin`, `ref_type=day`, `ref_id=YYYYMMDD`），与活跃度/余额同一事务；重复签到不发币。
- 去重唯一约束 `(user_id, action, ref_type, ref_id)`；达到每日上限返回 no-op，不影响主操作。
- 提交/参赛/比赛创建/克隆钩子失败只记录日志，不失败主请求。
- 共享积分：创建/克隆比赛 +20；启动时幂等回填已有比赛。
- 榜单缓存只缓存基础行；Redis 版本 key + TTL 60s；Redis 故障降级数据库，不因缓存失败返回 5xx。
- 活跃度/共享榜公开；签到/摘要需要登录。
- 所有用户可见文案中文；不推送、不合并分支。
- 不实现前端页面（M4-C）、学习路线、社交、Rating 衰减/赛季。
- 不改变 Rating 结算/榜单行为。

## Review Focus

1. **每日上限与并发**：并发提交/签到时不能超过上限、不能重复发币；测试放 Task 1。
2. **签到金币幂等**：重复签到/重复请求只发一次金币与活跃度；测试放 Task 1。
3. **钩子失败隔离**：活动写入失败不能导致提交/报名/比赛创建失败；测试放 Task 2。
4. **共享回填/克隆一致性**：已有比赛启动回填、克隆比赛都计入；测试放 Task 2。
5. **榜单缓存一致性**：活跃度/共享写入后版本失效，Redis 故障降级；测试放 Task 3。

---

### Task 1: ActivityLog/SharedItem 模型、签到金币与每日上限

**Files:**
- Create: `apps/server/internal/activity/model.go`
- Create: `apps/server/internal/activity/repository.go`
- Create: `apps/server/internal/activity/service.go`
- Create: `apps/server/internal/activity/service_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Modify: `apps/server/internal/httpx/errors.go`（如需要新错误码；优先复用 40001/429）

**Interfaces:**
- Consumes: 现有 `coin_transactions`、`users.coins`、`user_login_days`/本地日期、`testutil.PG`。
- Produces:
  - `type Action string`：`submission`、`contest_join`、`signin`。
  - `ActivityLog{ID, UserID, Action, Points, RefType, RefID, CreatedAt}`；唯一 `(user_id, action, ref_type, ref_id)`；索引 `(user_id, created_at)`、`(created_at)`。
  - `SharedItem{ID, UserID, Type, RefID, LikeCount, CreatedAt}`；唯一 `(type, ref_id)`。
  - `type SignInResult struct { Awarded bool; ActivityPoints int; Coins int; Date string }`。
  - `type Summary struct { SignedInToday bool; TodayPoints, TotalPoints int64 }`。
  - `type Service struct { db, rdb }`；`NewService(db *gorm.DB, rdb *redis.Client) *Service`。
  - `RecordSubmission(ctx,userID,submissionID int64) error`。
  - `RecordContestJoin(ctx,userID,contestID int64) error`。
  - `SignIn(ctx,userID int64) (*SignInResult,error)`。
  - `Summary(ctx,userID int64) (*Summary,error)`。
  - `BackfillSharedItems(ctx) error`。
  - `ActivityLeaderboardVersionKey = "kapoj:leaderboard:activity:version"`。
  - `SharingLeaderboardVersionKey = "kapoj:leaderboard:sharing:version"`。

- [ ] **Step 1: 写失败模型/签到测试**

```go
func TestSignInAwardsOnce(t *testing.T) {
    // 首次 SignIn -> Awarded=true, ActivityPoints=5, Coins=5
    // 再次 -> Awarded=false, 0/0；user.coins 只加 5；activity_logs=1；coin_transactions=1
}
func TestSignInConcurrent(t *testing.T) {
    // 8 个并发 SignIn -> 只有一次 awarded，余额+5，日志/流水各 1
}
func TestActivityDailyCaps(t *testing.T) {
    // submission 23 次（每次 2 分）→ 40 分封顶，第 21 次起 no-op
    // contest_join 6 次（每次 10 分）→ 50 分封顶
}
func TestActivityDedupe(t *testing.T) {
    // 同一 submission/contest ref 重复记录只生效一次
}
func TestSummary(t *testing.T) {
    // 今天有签到/提交，返回 signedInToday/todayPoints/totalPoints
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ -v`
Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现模型、迁移与活跃度基础**

- `ActivityLog`/`SharedItem` 字段按设计；`db.Migrate` 追加；`db_test.go` 断言表/唯一索引。
- `recordActivity` 事务：锁用户行 `FOR UPDATE`；先按唯一键查重；计算 `(user_id, action, created_at::date=today)` 的当日总分；若达到上限返回 false,nil；插入日志；活动版本 `INCR`（best-effort）。
- `RecordSubmission` 分值 2、上限 40；`RecordContestJoin` 分值 10、上限 50；重复 ref no-op。
- 日期使用 `time.Now().In(time.Local).Format("2006-01-02")`，SQL 日期比较用 `created_at >= 当天零点 AND created_at < 次日零点`，避免时区/DateStyle 依赖。

- [ ] **Step 4: 实现签到与摘要**

`SignIn` 事务：
1. 锁用户行；查当天 `action=signin, ref_type=day, ref_id=YYYYMMDD`。
2. 已存在 → 返回 `Awarded=false` 与当天分数（0/已记录的分）。
3. 插入 activity_log（5）；
4. 插入 `coin_transactions`（reason `signin`, refType `day`, refID YYYYMMDD, amount 5），唯一冲突则整体回滚/返回已签到；
5. `users.coins = coins + 5`；
6. 提交后 `INCR` 活跃度版本。
- 返回 `ActivityPoints=5, Coins=5, Date=YYYY-MM-DD`。
- `Summary` 查询当天分数、总分、是否签到。
- 若 `rdb` 为 nil，跳过版本 INCR。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/activity internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ ./internal/db/ -v`
Expected: PASS。

```bash
git add apps/server/internal/activity apps/server/internal/db
git commit -m "feat(server): activity logs and sign-in coins"
```

---

### Task 2: 提交/参赛/比赛创建钩子与共享回填

**Files:**
- Create: `apps/server/internal/activity/sharing.go`
- Create: `apps/server/internal/activity/sharing_test.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 `RecordSubmission/RecordContestJoin`、`BackfillSharedItems`、`SharedItem`。
- Produces:
  - `submissionapi.ActivityHook{RecordSubmission(ctx,userID,submissionID int64) error}`；`Options.ActivityHook`。
  - `contestapi.ActivityHook{RecordContestJoin(ctx,userID,contestID int64) error; RecordContestCreated(ctx,userID,contestID int64) error}`；`Options.ActivityHook`。
  - `activity.Service.RecordContestCreated(ctx,userID,contestID) error`：插入 `shared_items(type=contest)`，唯一冲突 no-op；成功后 `INCR sharing version`。
  - `activity.Service.BackfillSharedItems(ctx)`：`INSERT ... SELECT created_by,'contest',id,0,created_at FROM contests WHERE created_by>0 ON CONFLICT DO NOTHING`。
  - main 在启动时调用 `BackfillSharedItems`（失败日志，不阻止启动）。

- [ ] **Step 1: 写失败共享/钩子测试**

```go
func TestRecordContestCreatedAndBackfill(t *testing.T) {
    // 直接创建两条 contests + 一条已有 shared_item
    // BackfillSharedItems 后 shared_items 数量正确且幂等
    // RecordContestCreated 新比赛增加一条；重复 no-op
}
func TestSubmissionActivityHookFailureDoesNotFailRequest(t *testing.T)
func TestContestActivityHookFailureDoesNotFailRegisterOrCreate(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ ./internal/submissionapi/ ./internal/contestapi/ -run 'TestRecordContest|TestBackfill|TestSubmissionActivity|TestContestActivity' -v`
Expected: FAIL。

- [ ] **Step 3: 实现共享与钩子**

- `RecordContestCreated`: transaction insert `SharedItem{UserID, Type:"contest", RefID:contestID, LikeCount:0, CreatedAt:now}`; `OnConflict DoNothing`; only bump version if inserted.
- `BackfillSharedItems`: raw INSERT SELECT; `ON CONFLICT (type,ref_id) DO NOTHING`; bump version after any insert? Simpler always bump once after backfill.
- `submissionapi.handler.create`: after `svc.Create` success, if `activityHook != nil` call `RecordSubmission(ctx, claims.UserID, created.ID)`, log warning on error, still return 201.
- `contestapi.handler.register`: after successful `svc.Register` and existing game hook, call `RecordContestJoin`; log on error.
- `contestapi.handler.create`/`clone`: after success, call `RecordContestCreated(ctx, claims.UserID, created.ID)`; log on error. The create/clone handlers currently use viewer(c) with user ID; ensure claims available.
- Router: create `activitySvc := activity.NewService(d.DB, d.Redis)`, pass to `submissionapi.Options{ActivityHook: activitySvc}` and `contestapi.Options{ActivityHook: activitySvc}` (alongside existing hooks), only when non-nil. Main creates activity service for backfill; router can construct its own or Deps add `Activity *activity.Service`. Prefer `Deps.Activity` to share and avoid duplicate. Main creates and passes.
- Backfill in main after migrate, before serving; log warning on failure.

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/activity internal/submissionapi internal/contestapi internal/router cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ ./internal/submissionapi/ ./internal/contestapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/activity apps/server/internal/submissionapi apps/server/internal/contestapi apps/server/internal/router apps/server/cmd/server
git commit -m "feat(server): activity hooks and shared contests"
```

---

### Task 3: 活跃度/共享榜、签到/摘要 API 与 Redis 缓存

**Files:**
- Create: `apps/server/internal/activity/leaderboard.go`
- Create: `apps/server/internal/activity/leaderboard_test.go`
- Create: `apps/server/internal/activityapi/handler.go`
- Create: `apps/server/internal/activityapi/handler_test.go`
- Modify: `apps/server/internal/activity/service.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1/2 的 activity service、gamification identities、rating.Leaderboard pattern。
- Produces:
  - `activity.LeaderboardRow{ Rank int; UserID int64; Username, Nickname string; Score int64 }`（活跃度/共享榜共用；无 level）。
  - `(*Service).ActivityLeaderboard(ctx,page,size) (httpx.Page[LeaderboardRow], error)`。
  - `(*Service).SharingLeaderboard(ctx,page,size) (httpx.Page[LeaderboardRow], error)`。
  - 缓存 key `kapoj:leaderboard:activity:v<version>:<page>:<size>` / `...:sharing:...`，TTL 60s；版本 key 为 Task 1 常量；Redis 故障降级。
  - `activityapi.Options{ IdentityLookup IdentityLookup }`；row DTO 加 `equippedBadges`/`equippedTitle`。
  - API：
    - `GET /api/leaderboard/activity`
    - `GET /api/leaderboard/sharing`
    - `GET /api/activity/summary`（登录）
    - `POST /api/activity/signin`（登录）
  - Router `Deps.Activity` optional；main 共享 service。

- [ ] **Step 1: 写失败榜单/缓存测试**

```go
func TestActivityLeaderboardOrderAndPagination(t *testing.T)
// 3 用户总分 20/10/10；按总分 desc,user_id asc；rank/分页正确
func TestSharingLeaderboardFromSharedItems(t *testing.T)
// 2 场比赛=40 分；solution like_count 预留但本阶段为 0
func TestLeaderboardCacheVersionAndFallback(t *testing.T)
// 首次写缓存；记录活动后 INCR 版本 → 新值；关闭 miniredis → 仍返回数据库
func TestSignInAPIIIdempotentAndSummary(t *testing.T)
// 登录用户签到 200 awarded=true；再次 awarded=false；summary signedInToday=true
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ ./internal/activityapi/ -run 'TestActivityLeaderboard|TestSharing|TestLeaderboardCache|TestSignInAPI' -v`
Expected: FAIL。

- [ ] **Step 3: 实现榜单与缓存**

- 活跃度榜：`SELECT user_id, SUM(points) AS score FROM activity_logs GROUP BY user_id HAVING SUM(points)>0`，join users `status='active'`，order score desc,user_id asc；total 为有分用户数。
- 共享榜：`shared_items` 分 = `20 * count(type='contest') + 5 * sum(like_count WHERE type='solution')`；本阶段 solution 为 0；join active users；order score desc,user_id asc。
- 缓存基础行（不含身份），版本 key 读取/`redis.Nil`/错误降级；TTL 60s；仅在成功插入 activity/shared 后 INCR 版本（Task 1/2 已做）。
- `activityapi` 榜单 handler 批量装饰身份；空数组初始化。
- 签到/摘要 handler 使用 `RequireAuth`；`SignIn` 返回 `{awarded,activityPoints,coins,date}`；`Summary` 返回 `{signedInToday,todayPoints,totalPoints}`。
- OpenAPI schema/paths；`corepack pnpm gen:api`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/activity internal/activityapi internal/router cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/activity/ ./internal/activityapi/ ./internal/router/ -v`
`cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/server/internal/activity apps/server/internal/activityapi apps/server/internal/router apps/server/cmd/server packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(server): activity and sharing leaderboards"
```

---

### Task 4: 文档、全量检查与真实榜单冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–3。
- Produces: M4-B 状态文档与验证记录。

- [ ] **Step 1: 更新文档**

- `DEVELOPMENT.md` 5.5：M4-B 完成（活跃度、签到金币、共享分、三个榜单）；M4-6/M4-7 学习路线与前端仍待办；3.6 表状态更新。
- `docs/任务清单.md`：M4-3/M4-4 ✅；M4-5 ✅（三榜完成）；M4-6/M4-7 ⬜；加 M4-B 计划链接。
- `docs/开发日志.md`：追加 M4-B 条目（规则、签到金币、共享回填、缓存、验证、复审修复）。
- `README.md`、`运行手册.md`：当前版本 M4-B；补充签到 +5 金币、活跃度/共享榜、Redis 版本缓存；说明评论/点赞/关注/题解点赞仍未实现（后续 M4 阶段）。

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

- 登录 `m2bsolver1`，调用 `POST /api/activity/signin` 两次：第一次 `awarded=true, coins=5`，第二次 `awarded=false`；金币只增加 5。
- 提交一次代码、报名一场比赛（若已有可跳过）：活跃度增加 2/10；`GET /api/leaderboard/activity` 分数正确。
- 创建/克隆一场比赛：`GET /api/leaderboard/sharing` 增加 20 分；启动回填后已有比赛也计入。
- 检查 Redis 缓存：重复请求命中，活动写入后版本失效；停止 Redis 后榜单仍 200（降级）。
- 不删除本地测试数据，报告创建内容。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m4-b activity leaderboards"
```

---

## Self-Review

**Spec coverage:**

- 活跃度提交/参赛/签到、上限/去重 → Task 1。
- 签到金币 → Task 1。
- 共享创建/克隆/回填 → Task 2。
- 活跃度/共享榜、三榜完成、缓存 → Task 3。
- 文档/冒烟 → Task 4。
- 前端、学习路线、社交、衰减/赛季 → 后续阶段。

**Type consistency:**

- `ActivityLog` 唯一 `(user_id,action,ref_type,ref_id)`；`signin` 使用 `ref_type=day`, `ref_id=YYYYMMDD`。
- `coin_transactions` `reason=signin` 与唯一约束一致。
- 版本 key 常量与缓存 key 前缀一致。
- 榜单基础行不含身份；API 层统一装饰。

**Review Focus mapping:**

1. 每日上限/并发 → Task 1 cap/concurrent tests。
2. 签到幂等 → Task 1 sign-in tests。
3. 钩子失败隔离 → Task 2 handler tests。
4. 共享回填/克隆 → Task 2 tests。
5. 缓存一致性/降级 → Task 3 tests。

**Proportion:** 计划固定规则、接口、缓存 key 与测试；具体 SQL/事务实现按现有 M3/M4-A 风格完成。
