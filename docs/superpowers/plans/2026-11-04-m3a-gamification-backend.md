# M3-A 金币、事件、成就与任务 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 M3 的后端基础：金币账本、Asynq 游戏化事件、成就自动解锁与任务自动结算。

**Architecture:** 新增 `gamification` 领域包，统一处理金币、条件求值、成就、任务与登录日；Web 进程运行 Asynq `game` worker，judge 在 AC 终态后投递事件；所有奖励靠唯一约束与行锁幂等，读取成就/任务时补算以兜底丢失事件。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、Asynq、testify。

**Spec:** `docs/superpowers/specs/2026-11-04-m3-gamification-design.md`

## Global Constraints

- 所有文档、注释与用户可见文案使用中文；代码标识符使用英文。
- 金币余额变动必须与 `coin_transactions` 在同一数据库事务内完成。
- 自动奖励必须幂等：重复事件、并发请求不能重复发金币。
- 成就/任务/金币管理接口仅 `super_admin` 可用。
- 任务完成只统计 `claimed_at <= submissions.judged_at < tasks.end_time` 的 AC 提交。
- 已领取任务即使被下线，截止前仍可完成并发奖；已解锁成就不因下线消失。
- M3-A 不支持成就徽章奖励：`rewardBadgeId` 必须为空；徽章奖励在 M3-B 实现。
- 不新增前端页面；M3-A 只生成 OpenAPI 与前端类型，页面留给 M3-C。
- 每个任务结束前运行 `gofmt`、`go vet`、对应测试；提交信息使用 `feat(server): ...` 或 `test(server): ...`。
- 不推送、不合并分支，除非用户明确确认。

## Review Focus

1. **重复/并发 AC 事件**：同一成就或任务只能发一次金币，余额与流水必须一致；测试放在 Task 3/4。
2. **任务时间边界**：领取前 AC 不计数，截止后 AC 不计数，截止前领取但过期后不能完成；测试放在 Task 4。
3. **金币并发扣减**：余额不能为负，两个并发消费只能一个成功；测试放在 Task 1。
4. **非法条件配置**：嵌套 `all_of`、零阈值、未知类型必须被管理员接口拒绝，不能静默永不解锁；测试放在 Task 2/3。
5. **下线内容可见性**：已解锁成就、已领取任务仍对本人可见且可完成；测试放在 Task 3/4。

---

### Task 1: 数据模型、迁移、金币账本与管理员调币

**Files:**
- Create: `apps/server/internal/dbtypes/json.go`
- Create: `apps/server/internal/dbtypes/json_test.go`
- Create: `apps/server/internal/gamification/model.go`
- Create: `apps/server/internal/gamification/repository.go`
- Create: `apps/server/internal/gamification/coin_repository.go`
- Create: `apps/server/internal/gamification/coin_repository_test.go`
- Modify: `apps/server/internal/db/db.go`
- Test: `apps/server/internal/db/db_test.go`

**Interfaces:**
- Consumes: 现有 `testutil.PG(t)`、`db.Migrate(g)`。
- Produces:
  - `dbtypes.JSON[T]`：jsonb 值类型，`Value()`/`Scan()`/`GormDataType()`。
  - `gamification.CoinTransaction`、`gamification.CoinType`。
  - `gamification.NewRepository(db *gorm.DB) *Repository`。
  - `(*Repository).CreditCoins(ctx, userID, amount int64, reason, refType string, refID int64) (bool, error)`：插入流水成功才加余额，重复 ref 返回 `false,nil`。
  - `(*Repository).SpendCoins(ctx, userID, amount int64, reason, refType string, refID int64) error`：锁用户行、校验余额、扣款并写流水；余额不足返回 `httpx.ErrValidation("金币不足")`。
  - `(*Repository).AdminAdjustCoins(ctx, userID, amount int64, note string) error`：正负均可，余额不得为负。
  - `(*Repository).Balance(ctx, userID int64) (int64, error)`。
  - `(*Repository).ListCoinTransactions(ctx, userID int64, offset, limit int) ([]CoinTransaction, int64, int64, error)`：返回流水、总数、当前余额。

- [ ] **Step 1: 写失败测试 `dbtypes/json_test.go`**

```go
func TestJSONRoundTrip(t *testing.T) {
    in := JSON[struct {
        Threshold int `json:"threshold"`
    }]{Threshold: 3}
    v, err := in.Value()
    require.NoError(t, err)
    var out JSON[struct {
        Threshold int `json:"threshold"`
    }]
    require.NoError(t, out.Scan(v))
    assert.Equal(t, 3, out.Threshold)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/dbtypes/ -run TestJSONRoundTrip -v`
Expected: FAIL，`undefined: JSON`。

- [ ] **Step 3: 实现 `dbtypes.JSON[T]`**

在 `json.go` 定义与 `dbtypes.List[T]` 相同风格的 `Value/Scan/GormDataType`；`nil` 源数据扫描为零值，不报错。

- [ ] **Step 4: 写失败测试 `coin_repository_test.go`**

```go
func TestCreditCoinsIsIdempotent(t *testing.T) {
    e := newCoinEnv(t)
    ok, err := e.repo.CreditCoins(context.Background(), userID, 50, "achievement", "achievement", 7)
    require.NoError(t, err)
    assert.True(t, ok)
    ok, err = e.repo.CreditCoins(context.Background(), userID, 50, "achievement", "achievement", 7)
    require.NoError(t, err)
    assert.False(t, ok)
    balance, err := e.repo.Balance(context.Background(), userID)
    require.NoError(t, err)
    assert.EqualValues(t, 50, balance)
}

func TestSpendCoinsRejectsInsufficientBalance(t *testing.T) {
    e := newCoinEnv(t)
    require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), userID, 10, "测试"))
    err := e.repo.SpendCoins(context.Background(), userID, 11, "badge_purchase", "badge", 1)
    assertCode(t, err, 40001)
    balance, _ := e.repo.Balance(context.Background(), userID)
    assert.EqualValues(t, 10, balance)
}

func TestAdminAdjustCoinsAllowsRepeatedNotes(t *testing.T) {
    e := newCoinEnv(t)
    require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), userID, 5, "活动"))
    require.NoError(t, e.repo.AdminAdjustCoins(context.Background(), userID, 5, "活动"))
    balance, _ := e.repo.Balance(context.Background(), userID)
    assert.EqualValues(t, 10, balance)
}
```

`newCoinEnv` 用 `testutil.PG(t)` 建一个用户（id=1），返回 `NewRepository(g)`；`assertCode` 复制现有 contest 测试的 `httpx.AppError` 断言。

- [ ] **Step 5: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run 'TestCreditCoins|TestSpendCoins|TestAdminAdjustCoins' -v`
Expected: FAIL，`gamification` 或 `CreditCoins` 未定义。

- [ ] **Step 6: 实现模型、迁移与金币仓储**

`model.go` 先定义：

```go
type CoinType string

const (
    CoinEarn        CoinType = "earn"
    CoinSpend       CoinType = "spend"
    CoinAdminAdjust CoinType = "admin_adjust"
)

type CoinTransaction struct {
    ID        int64     `gorm:"primaryKey"`
    UserID    int64     `gorm:"not null;index:idx_coin_transactions_user_created,priority:1;uniqueIndex:uniq_coin_transactions_idem,priority:1"`
    Amount    int64     `gorm:"not null"`
    Type      CoinType  `gorm:"size:20;not null"`
    Reason    string    `gorm:"size:40;not null;uniqueIndex:uniq_coin_transactions_idem,priority:2"`
    RefType   string    `gorm:"size:20;not null;default:'';uniqueIndex:uniq_coin_transactions_idem,priority:3,where:ref_type <> ''"`
    RefID     int64     `gorm:"not null;default:0;uniqueIndex:uniq_coin_transactions_idem,priority:4"`
    Note      string    `gorm:"size:200;not null;default:''"`
    CreatedAt time.Time `gorm:"type:timestamptz;not null;index:idx_coin_transactions_user_created,priority:2"`
}
```

同时定义 `Achievement`、`UserAchievement`、`Task`、`UserTaskRecord`、`UserLoginDay`（字段按设计文档 5.2、5.3、5.6）：`UserLoginDay.LoginDate` 使用 `gorm:"type:date;not null;uniqueIndex:uniq_user_login_days,priority:2"`，`UserID` 同索引 `priority:1`。

在 `db.Migrate` 追加全部上述模型。`repository.go` 提供 `NewRepository` 与 `db` 字段。`coin_repository.go` 用 `db.Transaction` 实现四个方法；消费时用 `clause.Locking{Strength: "UPDATE"}` 锁 `users` 行，插入流水冲突返回已处理，扣款前检查 `coins >= amount`。

- [ ] **Step 7: 运行金币与迁移测试**

Run: `cd apps/server && gofmt -w internal/dbtypes internal/gamification internal/db && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/dbtypes/ ./internal/gamification/ ./internal/db/ -v`
Expected: PASS。

- [ ] **Step 8: 并发扣减测试**

在 `coin_repository_test.go` 增加 `TestSpendCoinsConcurrent`：余额 10，两个 goroutine 各消费 8，最终只有一个成功，余额 2 且流水 1 条。用 `sync.WaitGroup` + 错误通道收集结果。

Run: 同一测试命令，Expected: PASS。

- [ ] **Step 9: 提交**

```bash
cd /Users/admin/codespace/kapoj
git add apps/server/internal/dbtypes apps/server/internal/gamification apps/server/internal/db
git commit -m "feat(server): coin ledger and gamification models"
```

---

### Task 2: 条件求值与用户补算骨架

**Files:**
- Create: `apps/server/internal/gamification/condition.go`
- Create: `apps/server/internal/gamification/condition_test.go`
- Create: `apps/server/internal/gamification/login_repository.go`
- Create: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/gamification/repository.go`
- Test: `apps/server/internal/gamification/condition_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Repository`、`Achievement`。
- Produces:
  - `type ConditionType string`，常量 `ConditionACCount`、`ConditionContestCount`、`ConditionLoginStreak`、`ConditionCreatedContest`、`ConditionAllOf`。
  - `type Condition struct { Type ConditionType; Threshold int64 }`。
  - `type ConditionConfig struct { Threshold int64; Conditions []Condition }`，JSON 字段 `threshold`、`conditions`。
  - `type ConditionProgress struct { Type ConditionType; Current int64; Target int64; Done bool }`。
  - `func ValidateCondition(t ConditionType, c ConditionConfig) error`。
  - `func (s *Service) EvaluateCondition(ctx context.Context, userID int64, t ConditionType, c ConditionConfig) (bool, []ConditionProgress, error)`。
  - `func (s *Service) EvaluateUser(ctx context.Context, userID int64) error`（本任务只做登录日/占位，Task 3/4 填充成就与任务）。
  - `(*Repository).RecordLoginDay(ctx, userID int64, date string) error`。
  - `(*Repository).CurrentLoginStreak(ctx, userID int64, today string) (int64, error)`。

- [ ] **Step 1: 写失败条件测试**

覆盖：

```go
func TestConditionACCount(t *testing.T) // 插入 accepted + wrong_answer，期望 current=1, done=true（threshold=1）
func TestConditionAllOf(t *testing.T)   // ac_count>=1 且 contest_count>=1；只满足一个时 done=false
func TestConditionLoginStreak(t *testing.T) // 今天、昨天、前天 =3；中间缺一天则只算连续段
func TestValidateConditionRejectsBadConfig(t *testing.T) // threshold<=0、未知类型、all_of 只有 1 项、嵌套 all_of、超过 8 项均返回 40001
```

登录 streak 测试直接插入 `user_login_days` 行，`today` 用 `time.Now().In(time.Local).Format("2006-01-02")`。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run 'TestCondition|TestValidateCondition' -v`
Expected: FAIL，类型/方法未定义。

- [ ] **Step 3: 实现条件求值**

计数查询：

- AC：`SELECT COUNT(DISTINCT problem_id) FROM submissions WHERE user_id = ? AND status = 'accepted'`。
- 参赛：`SELECT COUNT(*) FROM contest_participants WHERE user_id = ?`。
- 创建比赛：`SELECT COUNT(*) FROM contests WHERE created_by = ?`。
- 登录 streak：取最近 400 条 `login_date::text` 倒序，从 `today` 开始逐日回退比较；今天不存在返回 0。
- `all_of`：逐个调用单条件求值并汇总 progress；任一 false 则整体 false。

- [ ] **Step 4: 实现 `Service` 与登录日写入**

```go
type Service struct {
    repo *Repository
}
func NewService(repo *Repository) *Service
func (s *Service) EvaluateUser(ctx context.Context, userID int64) error { return nil } // 后续任务填充
func (s *Service) OnLogin(ctx context.Context, userID int64) error
```

`OnLogin` 调用 `RecordLoginDay`（`ON CONFLICT DO NOTHING`），事件发布在 Task 5 接入。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification
git commit -m "feat(server): gamification condition evaluation"
```

---

### Task 3: 成就服务、奖励发放与 API

**Files:**
- Create: `apps/server/internal/gamification/achievement.go`
- Create: `apps/server/internal/gamification/achievement_repository.go`
- Create: `apps/server/internal/gamification/achievement_test.go`
- Create: `apps/server/internal/gamification/dto.go`
- Create: `apps/server/internal/gamification/validate.go`
- Create: `apps/server/internal/gamificationapi/handler.go`
- Create: `apps/server/internal/gamificationapi/handler_test.go`
- Modify: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1/2 的仓储与条件求值。
- Produces:
  - `type Viewer struct { UserID int64; IsAdmin bool }`。
  - `type AchievementInput struct { Code, Name, Description, IconURL string; ConditionType ConditionType; ConditionConfig ConditionConfig; RewardCoins int64; RewardBadgeID *int64; SortOrder int; IsActive *bool }`。
  - `type AchievementItem struct { ID, Code, Name, Description, IconURL, ConditionType, ConditionConfig, RewardCoins, RewardBadgeID, SortOrder, IsActive, Unlocked, UnlockedAt, Progress }`，JSON 字段按设计文档。
  - `(*Service).ListAchievements(ctx, v Viewer, includeInactive bool) ([]AchievementItem, error)`。
  - `(*Service).CreateAchievement(ctx, v Viewer, in AchievementInput) (*Achievement, error)`。
  - `(*Service).UpdateAchievement(ctx, v Viewer, id int64, in AchievementInput) (*Achievement, error)`。
  - `(*Service).DeleteAchievement(ctx, v Viewer, id int64) error`。
  - `(*Repository).GrantAchievement(ctx, userID int64, a Achievement) (bool, error)`。
  - `(*Repository).UnlockedAchievementIDs(ctx, userID int64) (map[int64]time.Time, error)`。
  - API：`GET /api/achievements`、`GET /api/users/me/achievements`、`POST/PUT/DELETE /api/achievements`。

- [ ] **Step 1: 写失败服务测试**

至少包含：

```go
func TestAchievementUnlocksAndRewardsOnce(t *testing.T) {
    // 创建 ac_count=1、reward=50 的成就；插入 accepted 提交
    // EvaluateUser → unlocked, balance=50
    // 再 EvaluateUser → 仍 50，user_achievements=1，coin_transactions=1
}

func TestAchievementListKeepsUnlockedInactive(t *testing.T) {
    // 成就解锁后 DeactivateAchievement；普通用户列表仍能看到 Unlocked=true
    // 另一个未解锁且 inactive 的成就不可见
}

func TestAchievementProgressAndAllOf(t *testing.T) {
    // 两个条件的 all_of；只完成一个时 progress 两项、done=false
}

func TestAchievementValidation(t *testing.T) {
    // code 非法、未知 condition、rewardBadgeId 非空（M3-A）、rewardCoins<0 均 40001
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run TestAchievement -v`
Expected: FAIL。

- [ ] **Step 3: 实现成就服务**

- `EvaluateUser` 直接调用 `repo.ListAchievements(false)` 取 active 成就与 `UnlockedAchievementIDs`，不能调用 `Service.ListAchievements`（后者会调用 `EvaluateUser`，造成递归）；对每个未解锁成就求值，满足则 `GrantAchievement`。
- `GrantAchievement` 在一个事务内：`INSERT user_achievements ... ON CONFLICT DO NOTHING`；若插入成功且 `rewardCoins>0`，`INSERT coin_transactions ... ON CONFLICT DO NOTHING`，仅流水插入成功才 `UPDATE users SET coins = coins + reward`。
- M3-A 若 `RewardBadgeID != nil`，创建/更新时返回 `httpx.ErrValidation("M3-B 才支持徽章奖励")`。
- `ListAchievements`：管理员可 `includeInactive`；登录用户先 `EvaluateUser`，再返回 `is_active=true` 或自己已解锁的成就；游客只返回 active。
- `UpdateAchievement` 不允许修改 `code`（输入非空且不同返回 40001）。
- 删除为 `is_active=false`。

- [ ] **Step 4: 写失败 API 测试**

`handler_test.go` 复用 contestapi 的 `fx` 搭建方式，至少：

```go
func TestAchievementPermissionsAndList(t *testing.T) {
    // 游客 GET 200；普通用户 POST 403；super_admin POST 201
    // 普通用户已解锁 inactive 成就仍返回
    // includeInactive=true 对普通用户 403
}
func TestAdminAdjustCoinsEndpoint(t *testing.T) {
    // super_admin +100 → GET /coins/transactions balance=100
    // 普通用户 403；amount=0 / note 为空 40001
}
```

- [ ] **Step 5: 实现 API 与路由**

在 `gamificationapi` 注册上述成就接口与 `GET /api/coins/transactions`、`POST /api/admin/users/:id/coins`。`router.Deps` 增加可选 `Game *gamification.Service`；为 nil 时跳过游戏化路由，保持现有测试兼容。`router.New` 中：

```go
if d.Game != nil {
    gamificationapi.RegisterRoutes(api, d.Game)
}
```

金币流水响应 `{ balance, total, items }`。管理员调币输入 `{ amount, note }`，`amount != 0`，`note` 1–200 字。

- [ ] **Step 6: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/gamificationapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/gamificationapi apps/server/internal/router
git commit -m "feat(server): achievements and coin APIs"
```

---

### Task 4: 任务服务、领取、自动完成与 API

**Files:**
- Create: `apps/server/internal/gamification/task.go`
- Create: `apps/server/internal/gamification/task_repository.go`
- Create: `apps/server/internal/gamification/task_test.go`
- Modify: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Test: `apps/server/internal/gamificationapi/handler_test.go`

**Interfaces:**
- Consumes: Task 1/2/3 的仓储、条件与 `EvaluateUser`。
- Produces:
  - `type TaskStatus string`，常量 `upcoming/active/claimed/completed/expired/ended/inactive`。
  - `type TaskInput struct { Title, DescriptionMD string; ProblemIDs []int64; RewardCoins int64; StartTime, EndTime time.Time; IsActive *bool }`。
  - `type TaskItem struct { ID, Title, DescriptionMD, ProblemIDs, RewardCoins, StartTime, EndTime, IsActive, Status, ClaimedAt, CompletedAt, Progress, ProblemCount }`。
  - `(*Service).ListTasks(ctx, v Viewer, includeInactive bool) ([]TaskItem, error)`。
  - `(*Service).CreateTask/UpdateTask/DeleteTask`。
  - `(*Service).ClaimTask(ctx, userID, id int64) (*TaskItem, error)`。
  - `(*Service).EvaluateTasks(ctx, userID int64) error`，由 `EvaluateUser` 调用。
  - API：`GET/POST /api/tasks`、`PUT/DELETE /api/tasks/:id`、`POST /api/tasks/:id/claim`。

- [ ] **Step 1: 写失败任务测试**

```go
func TestTaskClaimAndAutoComplete(t *testing.T) {
    // 任务 2 题；领取前先 AC 第 1 题 → progress=0
    // 领取后 AC 两题 → EvaluateUser → completed, reward=80, 流水 1 条
    // 再次 EvaluateUser → 仍 80
}
func TestTaskDeadlineAndPreClaimAC(t *testing.T) {
    // 领取前 AC 不计入；end_time 之后 AC 不计入；截止后 claim 40001
}
func TestTaskFreezeAfterClaim(t *testing.T) {
    // 有人领取后，修改 problemIds 或 rewardCoins 返回 40001；改标题/结束时间成功
}
func TestInactiveClaimedTaskStillCompletes(t *testing.T) {
    // 领取后 is_active=false；列表仍可见；截止前 AC 全部 → 完成并发奖
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ -run TestTask -v`
Expected: FAIL。

- [ ] **Step 3: 实现任务服务**

- 校验：标题 1–100 字，描述 ≤ 50000，题目 1–20 个且不重复、全部存在且 `is_public=true`；奖励 0–1,000,000；`start < end`。
- 领取：任务 active、`start <= now < end`；`INSERT user_task_records ... ON CONFLICT DO NOTHING`；已 completed 返回 40001，已 claimed 返回现有记录。
- 进度：`COUNT(DISTINCT problem_id) FROM submissions WHERE user_id=? AND status='accepted' AND judged_at >= claimed_at AND judged_at < end_time AND problem_id IN (...)`。
- 完成：事务内锁记录行，状态必须 `claimed`；仅当 progress == len(problems) 时更新 completed 并发金币；重复投递返回 `false,nil`。
- `EvaluateTasks` 遍历该用户 claimed 记录；任务即使 inactive 也参与完成判定，但必须 `now` 未超过 end（或 AC 的 `judged_at < end`）。
- 显示状态按设计文档计算，`expired` 不落库。
- 更新：`TaskHasRecords` 为 true 时，`problemIds` 或 `rewardCoins` 变化返回 40001。
- 删除软删除。

- [ ] **Step 4: 实现任务 API 与失败测试**

`GET /api/tasks` 游客返回 active 任务；登录用户返回 active + 自己领取过的任务；`includeInactive=true` 仅 super_admin。领取接口登录可用。

至少测试：游客 200、普通用户不能创建 403、super_admin 创建/更新/删除、普通用户领取、重复领取幂等、已完成再领取 40001。

- [ ] **Step 5: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/gamificationapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/gamificationapi
git commit -m "feat(server): task claiming and auto completion"
```

---

### Task 5: Asynq 事件、登录/参赛钩子与 judge AC 投递

**Files:**
- Create: `apps/server/internal/gamequeue/contract.go`
- Create: `apps/server/internal/gamequeue/client.go`
- Create: `apps/server/internal/gamequeue/worker.go`
- Create: `apps/server/internal/gamequeue/contract_test.go`
- Create: `apps/server/internal/gamification/events.go`
- Create: `apps/server/internal/gamification/events_test.go`
- Create: `apps/judge/internal/gamequeue/contract.go`
- Create: `apps/judge/internal/gamequeue/publisher.go`
- Create: `apps/judge/internal/gamequeue/contract_test.go`
- Modify: `apps/server/internal/auth/handler.go`
- Modify: `apps/server/internal/auth/handler_test.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/judge/cmd/judge/main.go`
- Modify: `apps/judge/internal/pipeline/pipeline.go`
- Modify: `apps/judge/internal/pipeline/pipeline_test.go`

**Interfaces:**
- Consumes: `gamification.Service.EvaluateUser`、`OnLogin`、`RecordLoginDay`。
- Produces:
  - `gamequeue.TypeEvent = "game:event"`、`gamequeue.QueueName = "game"`。
  - `gamequeue.Event{Type EventType; UserID, SubmissionID, ContestID int64; OccurredAt time.Time}`，JSON 字段 `type/userId/submissionId/contestId/occurredAt`。
  - `gamequeue.EventType` 常量：`submission.accepted`、`user.login`、`contest.join`。
  - `gamequeue.NewPublisher(redisAddr string) *Publisher`、`Publish(ctx, Event) error`、`Close()`。
  - `gamequeue.Register(mux *asynq.ServeMux, h Handler)`，`Handler` 接口 `HandleGameEvent(ctx, Event) error`。
  - `gamification.Service.HandleGameEvent(ctx, ev gamequeue.Event) error`。
  - `gamification.ErrSubmissionNotFound`：worker 读取不到提交或提交不是 accepted 时返回，handler 视为无需重试。
  - `gamification.Service.OnContestJoin(ctx, userID, contestID int64) error`。
  - `auth.LoginHook` 接口 `OnLogin(ctx, userID int64) error`；`auth.RegisterRoutes(r, s, hooks ...LoginHook)`。
  - `contestapi.GameHook` 接口 `OnContestJoin(ctx, userID, contestID int64) error`；`contestapi.RegisterRoutes(api, s, hooks ...GameHook)`。
  - judge `pipeline.GamePublisher` 接口 `Publish(ctx, gamequeue.Event) error`；`pipeline.New(..., game ...GamePublisher)`。

- [ ] **Step 1: 写失败契约/worker 测试**

```go
func TestEventJSONContract(t *testing.T) {
    raw := `{"type":"submission.accepted","userId":2,"submissionId":9,"occurredAt":"2026-11-04T00:00:00Z"}`
    var ev Event
    require.NoError(t, json.Unmarshal([]byte(raw), &ev))
    assert.Equal(t, EventSubmissionAccepted, ev.Type)
    assert.EqualValues(t, 9, ev.SubmissionID)
}

func TestWorkerDispatchesEvent(t *testing.T) {
    // 用 fake Handler + asynq.NewTask(TypeEvent, payload)，调用 mux 处理器
    // 断言 handler 收到 type/userId/submissionId
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/gamequeue/ -v`
Expected: FAIL。

- [ ] **Step 3: 实现服务端 gamequeue 与 gamification 事件处理**

- `client.go` 使用 `asynq.NewClient`，`Publish` 编码 JSON，任务选项 `Queue(QueueName)`, `MaxRetry(5)`, `Timeout(30*time.Second)`。
- `worker.go` 的 `Register` 解析任务；JSON 非法或 `UserID<=0` 返回可重试错误；调用 handler。
- `gamification/events.go`：

```go
func (s *Service) HandleGameEvent(ctx, ev gamequeue.Event) error {
    switch ev.Type {
    case gamequeue.EventSubmissionAccepted:
        sub, err := s.repo.FindAcceptedSubmission(ctx, ev.SubmissionID)
        if errors.Is(err, ErrSubmissionNotFound) { return nil }
        if err != nil { return err }
        return s.EvaluateUser(ctx, sub.UserID)
    case gamequeue.EventUserLogin, gamequeue.EventContestJoin:
        return s.EvaluateUser(ctx, ev.UserID)
    }
    return nil
}
```

`FindAcceptedSubmission` 查询 `submissions` 的 `id,user_id,status`；状态不是 accepted 返回 `ErrSubmissionNotFound`。
- `OnLogin`：写登录日（已存在忽略），有 publisher 时投递 `user.login`；发布失败返回错误由 handler 记录日志，不影响登录响应。
- `OnContestJoin`：有 publisher 时投递 `contest.join`；发布失败返回错误由 handler 记录日志。
- `Service` 增加 `publisher EventPublisher` 字段，`EventPublisher` 接口 `Publish(ctx, gamequeue.Event) error`；`NewService(repo, publisher ...EventPublisher)` 保持现有测试兼容。

- [ ] **Step 4: 写失败登录/参赛钩子测试**

在 auth handler 测试中增加 `fakeLoginHook`，调用 `POST /api/auth/login` 后断言 hook 收到 userID 一次。在 contestapi 测试中增加 `fakeGameHook`，报名成功后断言收到 `(userID, contestID)`。

- [ ] **Step 5: 接入 auth/contest/router/main**

- `auth.RegisterRoutes` 增加变参 hook；handler 保存第一个；注册/登录成功后调用 `hook.OnLogin`，失败仅 `slog.Warn`。
- `config` 测试补充 `KAPOJ_GAME_CONCURRENCY` 默认 2、合法范围 1–16、非法值报错。
- `contestapi.RegisterRoutes` 增加变参 hook；报名成功后调用 `OnContestJoin`，失败仅日志。
- `router.Deps` 增加 `Game *gamification.Service`；存在时传给 auth/contest 与 gamificationapi。
- `config.Config` 增加 `GameConcurrency int`，默认 2，`KAPOJ_GAME_CONCURRENCY`，校验 1–16。
- `main.go` 创建 game publisher、gamification service、Asynq server + mux；启动 worker，HTTP 关闭时 `Shutdown`。

- [ ] **Step 6: 写失败 judge 投递测试**

在 pipeline 测试中增加 `fakeGamePublisher`，断言：

- AC 完成时收到一次 `submission.accepted`，`UserID`/`SubmissionID` 正确。
- WA 完成时不投递。
- 投递错误不影响 `Run` 返回 nil。

- [ ] **Step 7: 实现 judge 侧并运行测试**

- judge `gamequeue` 契约与服务端保持一致；`NewPublisher(redisAddr)` 使用 Asynq。
- `pipeline.New` 增加可选 game publisher；`Run` 中把 `sub` 传入 `finish`；`finish` 在 `repo.Finish` 成功后，若 `in.Status == accepted` 投递事件，失败只 `slog.Warn`。
- `judge/main.go` 创建 publisher 并注入 pipeline，退出时 Close。

Run:

```bash
cd apps/server && gofmt -w internal/gamequeue internal/gamification internal/auth internal/contestapi internal/router internal/config cmd/server
cd ../judge && gofmt -w internal/gamequeue internal/pipeline cmd/judge
cd ../server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamequeue/ ./internal/gamification/ ./internal/auth/ ./internal/contestapi/ ./internal/router/ -v
cd ../judge && go test -count=1 ./internal/gamequeue/ ./internal/pipeline/ -v
```

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add apps/server/internal/gamequeue apps/server/internal/gamification apps/server/internal/auth apps/server/internal/contestapi apps/server/internal/router apps/server/internal/config apps/server/cmd/server apps/judge/internal/gamequeue apps/judge/internal/pipeline apps/judge/cmd/judge
git commit -m "feat: gamification events via asynq"
```

---

### Task 6: OpenAPI、前端类型、文档与全量检查

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
- Consumes: Task 1–5 的 API。
- Produces: 前端可直接调用的类型；文档记录 M3-A 状态。

- [ ] **Step 1: 补 OpenAPI**

新增 schema：`CoinType`、`CoinTransactionItem`、`CoinTransactionPage`、`AchievementConditionType`、`AchievementCondition`、`AchievementConditionConfig`、`AchievementConditionProgress`、`AchievementItem`、`AchievementInput`、`TaskStatus`、`TaskItem`、`TaskInput`、`AdminCoinAdjustInput`。

新增/修改路径：

- `GET/POST /achievements`、`PUT/DELETE /achievements/{id}`
- `GET /users/me/achievements`
- `GET/POST /tasks`、`PUT/DELETE /tasks/{id}`、`POST /tasks/{id}/claim`
- `GET /coins/transactions`
- `POST /admin/users/{id}/coins`

所有响应引用统一 `ErrorResponse`；描述写清游客/登录/super_admin 权限与 40001 条件。

- [ ] **Step 2: 生成类型并检查**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web`
Expected: 类型生成成功，构建通过（页面尚未使用新类型）。

- [ ] **Step 3: 更新文档**

- `DEVELOPMENT.md`：3.5 实现状态、5.1/5.2 实现状态、4.1 登录钩子、4.6 后续徽章/称号展示说明；标注 M3-A 完成、M3-B/C 待办。
- `docs/任务清单.md`：M3-A 对应 M3-1/2/3/4 标 ✅，加子项目计划链接。
- `docs/开发日志.md`：追加 M3-A 条目（变更、验证、复审修复、待办）。
- `README.md`、`运行手册.md`：当前版本写“M3-A 已完成；徽章/称号与前端页面待 M3-B/C”。

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

Expected: 后端/ judge 全部包通过，golangci-lint `0 issues`，前端测试/lint/build 通过，`git diff --check` 无输出。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record m3-a gamification backend"
```

---

## Self-Review

**Spec coverage:**

- 金币余额/流水/管理员调币 → Task 1、Task 3 API。
- 事件总线（AC/登录/参赛）→ Task 5。
- 成就配置、条件、奖励、幂等 → Task 2、Task 3。
- 任务领取、AC 自动完成、结算、冻结 → Task 4。
- 登录连续天数 → Task 2。
- 读取补算 → Task 3/4 的列表服务调用 `EvaluateUser`。
- OpenAPI/文档 → Task 6。
- 徽章/称号/公开身份/前端页面 → M3-B/C，不在本计划。

**Type consistency:**

- `ConditionType` 在模型、输入、进度、OpenAPI 中同名。
- `TaskStatus` 的 `expired` 只在 DTO 计算，不写入 `user_task_records.status`（数据库只存 `claimed/completed`）。
- 金币 `reason`/`refType` 固定值在 Task 1/3/4 一致：`achievement/achievement`、`task/task`、`admin_adjust/''`。
- 事件 JSON 字段在 server/judge 两个契约文件中一致。

**Review Focus mapping:**

1. 重复/并发 AC → Task 3 `TestAchievementUnlocksAndRewardsOnce`、Task 4 `TestTaskClaimAndAutoComplete`。
2. 任务时间边界 → Task 4 `TestTaskDeadlineAndPreClaimAC`。
3. 金币并发扣减 → Task 1 `TestSpendCoinsConcurrent`。
4. 非法条件 → Task 2 `TestValidateConditionRejectsBadConfig`、Task 3 validation 测试。
5. 下线内容可见性 → Task 3 `TestAchievementListKeepsUnlockedInactive`、Task 4 `TestInactiveClaimedTaskStillCompletes`。

**Proportion:** 计划只固定接口、规则与测试；具体查询与事务实现留给执行者按现有代码风格完成。
