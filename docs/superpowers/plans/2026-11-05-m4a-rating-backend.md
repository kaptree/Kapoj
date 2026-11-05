# M4-A Rating、等级、历史与 Rating 榜 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M4 第一阶段的 Rating 后端：Rated 比赛结算、8 级颜色、Rating 历史、Rating 榜单与 Redis 缓存。

**Architecture:** 新增 `rating` 包：等级映射、Elo v1 算法、结算事务、周期扫描器、Rating 榜单/历史；比赛服务补 `rating_settled_at`、开赛冻结 `isRated`、ICPC `LastSubmissionAt`；结算用 PostgreSQL 全局 advisory lock 串行化，`rating_history` 唯一约束保证幂等。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、Asynq（已有）、testify。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-leaderboard-rating-learning-design.md`

## Global Constraints

- 同一用户同一比赛只写一条 `rating_history`，`users.rating` 只更新一次。
- 所有比赛结算串行：全局 advisory lock + 按 `end_time, id` 顺序；多实例安全。
- 只结算比赛期间至少 1 次提交的报名者；0 提交不参与。
- 评测中的提交默认等 10 分钟；超过宽限按已判结果结算，不无限等待。
- 开赛后不能修改 `isRated`；比赛开始时间/赛制/题目列表的现有冻结保持。
- 等级边界、K=32、delta ±100、rating ≥0 必须由测试固定。
- Redis 只缓存榜单基础行；Redis 故障降级数据库，不返回 5xx。
- 本计划不实现活跃度/共享榜/签到（M4-B）、学习路线、社交或前端页面（M4-C）。
- 所有用户可见文案中文；不推送、不合并分支。

## Review Focus

1. **不同比赛并发结算**：同一用户在两场重叠比赛中被同时结算时不能丢更新；测试放 Task 2。
2. **评测未完成**：截止后仍有 pending/judging 时不能永久漏结算；测试放 Task 2。
3. **ICPC 参与判定**：ICPC 行必须有 `LastSubmissionAt`，否则参与者被排除；测试放 Task 1。
4. **Rated 标记变更**：开赛后改 `isRated` 必须被拒绝；测试放 Task 1。
5. **缓存一致性/降级**：结算后榜单缓存失效，Redis 故障仍能读数据库；测试放 Task 3。

---

### Task 1: RatingHistory 模型、等级、比赛标记与 ICPC 参与字段

**Files:**
- Create: `apps/server/internal/rating/model.go`
- Create: `apps/server/internal/rating/level.go`
- Create: `apps/server/internal/rating/level_test.go`
- Modify: `apps/server/internal/contest/model.go`
- Modify: `apps/server/internal/contest/ranklist.go`
- Modify: `apps/server/internal/contest/ranklist_test.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/service_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Consumes: 现有 `contest.RankRow`/`RankSubmission`、`db.Migrate`。
- Produces:
  - `rating.HistoryKind`，常量 `HistoryContest`。
  - `rating.RatingHistory{ID, UserID, ContestID *int64, Kind, Rank, RatingBefore, RatingAfter, CreatedAt}`；唯一部分索引 `(user_id, contest_id) WHERE contest_id IS NOT NULL`。
  - `rating.Level{Key, Name, Color string; AccentColor string}`（JSON `key/name/color/accentColor`）。
  - `func LevelFor(rating int) Level`。
  - `Contest.RatingSettledAt *time.Time`，列 `rating_settled_at`。
  - `ErrRatedFrozen`；开赛后修改 `isRated` 返回 40001。
  - ICPC 榜单 `LastSubmissionAt` 为该用户比赛窗口内最后一次任意提交时间。

- [ ] **Step 1: 写失败等级测试**

```go
func TestLevelBoundaries(t *testing.T) {
    cases := []struct{ rating int; key string }{
        {0,"gray"},{1199,"gray"},{1200,"green"},{1399,"green"},
        {1400,"cyan"},{1599,"cyan"},{1600,"blue"},{1899,"blue"},
        {1900,"purple"},{2099,"purple"},{2100,"orange"},{2399,"orange"},
        {2400,"red"},{2599,"red"},{2600,"black_red"},
    }
    for _, c := range cases { assert.Equal(t, c.key, LevelFor(c.rating).Key) }
}
func TestBlackRedLevelHasAccent(t *testing.T) {
    l := LevelFor(2600)
    assert.Equal(t, "#000000", l.Color)
    assert.Equal(t, "#FF0000", l.AccentColor)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/rating/ -run TestLevel -v`
Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现模型、等级与迁移**

`RatingHistory` GORM 标签：

```go
ContestID *int64 `gorm:"uniqueIndex:uniq_rating_history_user_contest,priority:2,where:contest_id IS NOT NULL"`
UserID    int64  `gorm:"uniqueIndex:uniq_rating_history_user_contest,priority:1"`
Kind      HistoryKind `gorm:"size:20;not null"`
```

`LevelFor` 使用设计文档阈值；`AccentColor` 对黑红名返回 `#FF0000`，其他为空字符串。

`Contest` 增加 `RatingSettledAt *time.Time`；`db.Migrate` 追加 `RatingHistory`。`db_test.go` 断言表和部分唯一索引存在。

- [ ] **Step 4: 写失败 ICPC/冻结测试**

在 `contest/ranklist_test.go` 增加：ICPC 比赛参与者有 WA 提交时，`RankRow.LastSubmissionAt` 非 nil 且等于最后一次提交时间；无提交者为 nil。

在 `contest/service_test.go` 增加：比赛开始前可改 `isRated`；开始后修改返回 40001，其他可编辑字段仍可改。

- [ ] **Step 5: 实现 `LastSubmissionAt` 与 `isRated` 冻结**

- `computeICPC` 在遍历每个 submission 时更新 `state.row.LastSubmissionAt`（无论 AC/WA/CE），取最大 `CreatedAt`。
- `repository.Update` 的行锁校验中增加：`!now.Before(existing.StartTime)` 时若 `c.IsRated != existing.IsRated` 返回 `ErrRatedFrozen`；服务层映射为“比赛开始后不能修改是否计 Rating”。

- [ ] **Step 6: 运行测试、gofmt、提交**

Run: `cd apps/server && gofmt -w internal/rating internal/contest internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/contest/ ./internal/db/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rating apps/server/internal/contest apps/server/internal/db
git commit -m "feat(server): rating history and levels"
```

---

### Task 2: Elo v1 算法、结算事务与扫描器

**Files:**
- Create: `apps/server/internal/rating/algorithm.go`
- Create: `apps/server/internal/rating/algorithm_test.go`
- Create: `apps/server/internal/rating/repository.go`
- Create: `apps/server/internal/rating/service.go`
- Create: `apps/server/internal/rating/worker.go`
- Create: `apps/server/internal/rating/service_test.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/router/router.go`（如需暴露 Service，可先不暴露）
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 `RatingHistory`、`Contest.RatingSettledAt`、`contest.Ranklist`。
- Produces:
  - `type Participant struct { UserID int64; Rank int; Rating int }`。
  - `type Change struct { UserID int64; Rank, Before, After, Delta int }`。
  - `func Calculate(participants []Participant) []Change`。
  - `type RanklistProvider interface { Ranklist(ctx context.Context, v contest.Viewer, id int64) (*contest.Ranklist, error) }`。
  - `type Service struct { ... }`；`NewService(db *gorm.DB, contests RanklistProvider, rdb *redis.Client, opts Options) *Service`。
  - `type Options struct { JudgeGrace, ScanInterval time.Duration }`。
  - `func (s *Service) SettleContest(ctx context.Context, contestID int64) (bool, error)`：true 表示本次完成结算/设置 marker；false 表示跳过。
  - `func (s *Service) Run(ctx context.Context)`：周期扫描。
  - `Repository.PendingContestIDs(ctx, now, limit) ([]int64, error)`。
  - `Config.RatingScanInterval`/`RatingJudgeGrace`，env `KAPOJ_RATING_SCAN_INTERVAL`（10s–10m，默认 60s）、`KAPOJ_RATING_JUDGE_GRACE`（1m–60m，默认 10m）。

- [ ] **Step 1: 写失败算法测试**

至少：

```go
func TestCalculateThreePlayers(t *testing.T) {
    got := Calculate([]Participant{
        {UserID: 1, Rank: 1, Rating: 1500},
        {UserID: 2, Rank: 2, Rating: 1500},
        {UserID: 3, Rank: 3, Rating: 1500},
    })
    assert.Equal(t, 32, got[0].Delta)  // 32*(2 - 1) = 32
}
func TestCalculateSinglePlayerNoChange(t *testing.T)
func TestCalculateTiesShareActualScore(t *testing.T)
func TestCalculateClampsDeltaAndRatingFloor(t *testing.T)
func TestCalculateKnownFivePlayerCase(t *testing.T) // 固定期望值，覆盖公式
```

说明：`n=3`、三人同分时 `E_i=1`，第一 `A=2`，delta=32；第二 `A=1`，delta=0；第三 `A=0`，delta=-32。测试按此固定，若实现采用不同 K 必须改设计而不是改测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/rating/ -run TestCalculate -v`
Expected: FAIL（`Calculate` 未定义）。

- [ ] **Step 3: 实现算法**

- `E_i` 用 `math.Pow(10, (R_j-R_i)/400)`；`A_i = n - rank`；`delta = math.Round(32*(A-E))`，clamp ±100；`After=max(0,Before+Delta)`。
- 返回顺序与输入一致；`n<=1` 返回 delta=0。
- 输入 rank 必须 ≥1；重复/无效 rank 由调用方保证，算法不 panic。

- [ ] **Step 4: 写失败结算测试**

`service_test.go` 用真实 PostgreSQL + fake/real `contest.Service`：

```go
func TestSettleContestIdempotent(t *testing.T)
// 创建 rated 比赛、报名、提交；SettleContest 两次 → rating_history 1 条、rating 只变一次
func TestSettleContestOnlySubmittedParticipants(t *testing.T)
// 报名 3 人，只有 2 人有提交；第 3 人 rating 不变且无 history
func TestSettleContestNoParticipantsMarksSettled(t *testing.T)
func TestSettleContestWaitsForJudgingWithinGrace(t *testing.T)
// 插入 pending submission，end 刚过；SettleContest 返回 false，marker 仍 NULL
func TestSettleContestAfterGraceSettlesJudgedOnly(t *testing.T)
// 宽限已过，pending 不参与，已判参与者结算，marker 设置
func TestSettleDifferentContestsSerialized(t *testing.T)
// 两场比赛重叠用户并发 SettleContest；两场都结算，最终 rating 基于正确顺序
```

- [ ] **Step 5: 实现结算事务**

- `SettleContest`:
  1. `db.Transaction`；`SELECT pg_advisory_xact_lock(<constant>)`。
  2. `SELECT contests FOR UPDATE`，校验 `is_rated && end_time < now && rating_settled_at IS NULL`。
  3. 查询该比赛 `pending/judging` 提交数；若 >0 且 `now < end_time+grace` → 返回 false,nil。
  4. 调用 `contests.Ranklist(ctx, contest.Viewer{CanManage:true}, id)`；过滤 `row.LastSubmissionAt != nil`。
  5. `SELECT id,rating FROM users WHERE id IN ? FOR UPDATE` 取赛前快照。
  6. `Calculate`；逐人 `INSERT rating_history ... ON CONFLICT DO NOTHING`，仅插入成功才 `UPDATE users SET rating=?`。
  7. `UPDATE contests SET rating_settled_at=now()`。
- `PendingContestIDs`：`is_rated=true AND end_time < now AND rating_settled_at IS NULL ORDER BY end_time,id LIMIT ?`。
- `Run`：ticker（ScanInterval）调用 PendingContestIDs + SettleContest；失败 `slog.Warn`，继续。
- `main.go`：创建 rating.Service，`go ratingSvc.Run(ctx)`；关闭时随 context 停止。Config 增加两个时长并测试。

- [ ] **Step 6: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/rating internal/config cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/config/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rating apps/server/internal/config apps/server/cmd/server
git commit -m "feat(server): rating settlement worker"
```

---

### Task 3: Rating 榜、Rating 历史 API 与 Redis 缓存

**Files:**
- Create: `apps/server/internal/rating/leaderboard.go`
- Create: `apps/server/internal/rating/history.go`
- Create: `apps/server/internal/rating/leaderboard_test.go`
- Create: `apps/server/internal/rating/history_test.go`
- Create: `apps/server/internal/ratingapi/handler.go`
- Create: `apps/server/internal/ratingapi/handler_test.go`
- Modify: `apps/server/internal/rating/service.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/dto` 无；DTO 放 rating 包
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 2 的 Service/Redis、`gamification.Service.Identities`。
- Produces:
  - `rating.LeaderboardRow{ Rank int; UserID int64; Username, Nickname string; Score int; Level Level }`（基础行）。
  - `rating.LeaderboardPage{ Total int64; Items []LeaderboardRow }` 或复用 `httpx.Page`。
  - `(*Service).Leaderboard(ctx, page, size) (httpx.Page[LeaderboardRow], error)`：版本化 Redis 缓存。
  - `rating.HistoryItem{ ContestID, ContestTitle string; Rank, RatingBefore, RatingAfter, Delta int; CreatedAt time.Time }`。
  - `(*Service).RatingHistory(ctx, userID int64, page, size) (httpx.Page[HistoryItem], error)`。
  - API：`GET /api/leaderboard/rating`、`GET /api/users/:id/rating-history`。
  - `ratingapi.Options{ IdentityLookup IdentityLookup }`；router 在 `d.Game != nil` 时传入，用于给榜单行补 `equippedBadges/equippedTitle`。
  - 缓存 key：`kapoj:leaderboard:rating:v<version>:<page>:<size>`，TTL 60s；版本 key `kapoj:leaderboard:rating:version`；结算后 `INCR`；Redis 读失败降级数据库。

- [ ] **Step 1: 写失败榜单/缓存测试**

```go
func TestRatingLeaderboardOrderAndPagination(t *testing.T)
// 三个用户 rating 1700/1700/1500；按 rating desc,id asc；rank 1,2,3；level 正确
func TestRatingLeaderboardCacheHitAndVersion(t *testing.T)
// 第一次查库写缓存；修改用户 rating 并 INCR 版本后，下一次读到新值
func TestRatingLeaderboardRedisDownFallsBack(t *testing.T)
// 关闭 miniredis 后仍返回数据库数据
func TestRatingHistoryOrderAnd404(t *testing.T)
// 两场历史按时间倒序；不存在用户 404；分页 total/items 正确
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ -run 'TestRatingLeaderboard|TestRatingHistory' -v`
Expected: FAIL。

- [ ] **Step 3: 实现榜单与历史**

- 榜单查询 `users` 中 `status='active'`，`ORDER BY rating DESC, id ASC`，分页；rank = offset+index+1；`LevelFor(user.Rating)`。
- 缓存序列化 `httpx.Page[LeaderboardRow]`；版本 key 读取失败视为 0；缓存失败只日志。
- 历史查询 `rating_history JOIN contests`，`kind='contest'`，按 `created_at DESC, id DESC`；用户不存在返回 404（先查 users）。
- `Delta = RatingAfter - RatingBefore`；`ContestTitle` 从 contests 取。
- 结算成功后 `INCR` 版本 key（在 `SettleContest` 里）。

- [ ] **Step 4: 实现 API、身份装饰与 OpenAPI**

- `ratingapi`：
  - `GET /leaderboard/rating` 公开，分页参数 `httpx.ParsePage`。
  - `GET /users/:id/rating-history` 公开，分页。
  - 榜单行 DTO 增加 `EquippedBadges []gamification.BadgeSummary`、`EquippedTitle *gamification.TitleSummary`；handler 用可选 lookup 批量装饰，空数组初始化。
- OpenAPI 增加 `RatingLevel`、`RatingLeaderboardRow`、`RatingLeaderboardPage`、`RatingHistoryItem`、`RatingHistoryPage` 与两个路径；`corepack pnpm gen:api`。
- API 测试：公开 200、分页、历史 404、身份装饰、Redis 降级不 500。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/rating internal/ratingapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rating/ ./internal/ratingapi/ ./internal/router/ -v`
`cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/server/internal/rating apps/server/internal/ratingapi apps/server/internal/router packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(server): rating leaderboard and history"
```

---

### Task 4: 文档、全量检查与真实结算冒烟

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–3。
- Produces: M4-A 状态文档与验证记录。

- [ ] **Step 1: 更新文档**

- `DEVELOPMENT.md` 5.5 实现状态：M4-A 已完成（Rating 结算、等级、历史、Rating 榜）；M4-B/C 与学习路线/社交/衰减赛季仍待办。3.1 `rating_history` 不再标“未实现”；4.4 比赛 `isRated` 开赛冻结说明。
- `docs/任务清单.md`：M4-1、M4-2 标 ✅；M4-3/M4-4/M4-5 仍部分待办（M4-5 的 Rating 榜已完成，三榜未全）；M4-6/M4-7 ⬜。新增 M4-A 计划链接。
- `docs/开发日志.md`：追加 M4-A 条目（Rating 算法 v1、advisory lock、评测宽限、等级、历史、榜单缓存、验证、复审修复）。
- `README.md`、`运行手册.md`：当前版本 M4-A 完成；补充 Rating 结算扫描配置 `KAPOJ_RATING_SCAN_INTERVAL`、`KAPOJ_RATING_JUDGE_GRACE` 与 Rating 榜/历史接口。
- 明确 M4 后续阶段（学习路线、社交、衰减/赛季）未完成。

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

- [ ] **Step 3: 真实结算冒烟**

- 用本地 server + DB 创建/选择一场 `isRated=true` 已结束比赛与至少 2 名有提交的报名者。
- 触发扫描（等待 60s 或临时缩短 interval），验证 `GET /api/leaderboard/rating` 返回新 Rating、等级；`GET /api/users/:id/rating-history` 有一条记录。
- 重复触发不新增历史、不重复改 Rating。
- 确认 `rating_settled_at` 已设置。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m4-a rating backend"
```

---

## Self-Review

**Spec coverage:**

- Rating 结算、等级、历史、Rating 榜 → Task 1–3。
- 全局串行、评测宽限、ICPC 参与 → Task 1/2。
- Redis 缓存/降级 → Task 3。
- 文档/冒烟 → Task 4。
- 活跃度/共享榜/签到 → M4-B；学习路线/社交/衰减赛季 → 后续 M4 阶段。

**Type consistency:**

- `RatingHistory.ContestID *int64` 与部分唯一索引一致。
- `Level.AccentColor` 仅黑红名非空。
- `LeaderboardRow.Score` 在 Rating 榜为 Rating。
- `SettleContest` 返回 `(bool,error)`；worker 忽略 false。
- `RatingScanInterval`/`RatingJudgeGrace` 从 config 到 main 到 service 一致。

**Review Focus mapping:**

1. 不同比赛并发 → Task 2 `TestSettleDifferentContestsSerialized`。
2. 评测未完成 → Task 2 grace tests。
3. ICPC 参与 → Task 1 `LastSubmissionAt` tests。
4. Rated 冻结 → Task 1 service test。
5. 缓存/降级 → Task 3 cache tests。

**Proportion:** 计划固定接口、公式、时间边界和测试；具体查询/缓存封装按现有风格完成。
