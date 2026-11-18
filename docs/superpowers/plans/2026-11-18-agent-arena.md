# KAPOJ Agent 擂台 V1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 每个用户可创建并管理 1 个外部 AI Agent；平台发放 Agent token，提供随机领题、提交、查询、放弃 API；每题每 Agent 最多 20 次；平台记录通过/尝试题数、平均用时，并用难度加权 Elo 维护独立 Agent Rating。

**Architecture:** 新增 `agent` 领域包（模型、token、Rating、领题/提交/结算）与 `agentapi` HTTP 层；Agent 提交复用现有 `submissions` 和 judge，通过 `agent_id/agent_attempt_id/agent_settled` 与人类提交区分；服务端结算 worker 扫描终态 Agent 提交并幂等写入 `agent_rating_history`；所有人类统计查询增加 `agent_id IS NULL`。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、Redis（限流）、Asynq judge 队列；Vue 3.5、PrimeVue 4.5、Vitest、OpenAPI TypeScript 生成。

**Spec:** `docs/superpowers/specs/2026-11-18-agent-arena-design.md`

## Global Constraints

- 每个用户最多 1 个 Agent；名称全局唯一；`agents.user_id` 与 `agents.name` 唯一索引。
- 每个 Agent 同时最多 1 个 active token；明文只在生成/轮换响应返回一次；数据库只存 SHA-256 hash + 前缀。
- Agent token 格式 `kapoj_agent_` + base64url(32 random bytes)；请求头 `Authorization: Bearer <token>`。
- 每题每 Agent 最多 20 次提交，**包含**最终 AC 的那次；第 20 次 AC 仍算通过；第 20 次仍非 AC → `exhausted`，错误文案“错误次数耗尽”。
- 领题有效期 30 分钟；同一 Agent 同时最多 1 个 active attempt；后台结算每 30 秒、超时清扫每 60 秒。
- 有 `pending/judging` 提交时：禁止放弃、禁止超时结算、禁止同一 attempt 再提交；等待评测结束。
- 只有首次 AC 加分；同一题失败（放弃/超时/耗尽）最多扣一次；普通 WA/CE/RE/MLE 只消耗次数。
- Agent 提交不填 `contest_id`；不能参加比赛；随机题排除 `problems.is_public=false`、被未结束比赛锁定的题、已 `solved/exhausted` 的题、不可评测题（交互题、无测试点传统题、缺 checker 特判题）。
- Agent 提交不计入人类 AC、学习路线、游戏化、题目 `submit_count/accept_count`、题目统计、难度重算、比赛榜单。
- Rating 公式（唯一权威）：
  ```text
  D_p = 800 + (difficulty - 1) * 1600 / 99      // 800..2400
  E   = 1 / (1 + 10^((D_p - R) / 400))
  solve:   delta = clamp(round(32 * (1 - E)), 1, 32)
  failure: delta = -clamp(round(32 * E / 2), 1, 32)
  R_new = max(0, R + delta)
  ```
- 初始 Rating 1500；`delta` 存实际变化值（下限 0 导致绝对值可能小于公式值）。
- 所有新错误文案中文；OpenAPI 先改，`corepack pnpm gen:api` 生成类型。
- 后端 DB 测试用 `-p 1` 并 source 根 `.env`；前端用 Vitest。
- V1 不做平台代跑 AI、多 Agent、Agent 对战、Rating 衰减/赛季、按模型归一化、Agent 账号备份。

## Review Focus

1. **Token 安全**：只存 hash、明文一次性、轮换立即吊销旧 token、停用/封禁/吊销后拒绝；测试 `TestAgentTokenLifecycle` / `TestAgentTokenRejectsDisabledOrBanned`（Task 3）。
2. **20 次生命周期**：第 20 次 AC 通过、第 20 次失败 `exhausted`、pending 阻止放弃/提交、超时等待评测；测试 `TestAgentSubmitLimitAndExhaustion`（Task 5）。
3. **结算幂等**：重复扫描不重复加减分；同一题失败只扣一次；锁顺序统一避免死锁；测试 `TestAgentSettlementIdempotent` / `TestAgentFailurePenaltyOncePerProblem`（Task 6）。
4. **人类统计隔离**：submission 列表/AC 数/学习/游戏化/题目统计/难度/比赛/备份重算全部排除 Agent；测试 Task 2。
5. **随机题隔离**：比赛锁定、非公开、已解决/耗尽、不可评测题排除；测试 `TestAgentClaimExcludesUnavailableProblems`（Task 4）。

---

## File Structure

**新增后端**

- `apps/server/internal/agent/model.go` — `Agent`、`AgentToken`、`AgentProblemAttempt`、`AgentRatingHistory`、状态/类型常量。
- `apps/server/internal/agent/token.go` — 生成、SHA-256、前缀、掩码。
- `apps/server/internal/agent/rating.go` — 难度映射、Elo、delta、结算/扣分 helper。
- `apps/server/internal/agent/service.go` — Agent CRUD、token 生命周期、token 鉴权、公开列表/详情/历史、管理操作。
- `apps/server/internal/agent/arena.go` — 随机领题、提交、查询提交、放弃、attempt 视图。
- `apps/server/internal/agent/worker.go` — 终态结算扫描、超时清扫、后台循环。
- `apps/server/internal/agent/limiter.go` — Redis 固定窗口限流。
- `apps/server/internal/agentapi/handler.go` — 用户/公开 API。
- `apps/server/internal/agentapi/agent_handler.go` — Agent token 鉴权中间件与 `/api/agent/*`。
- `apps/server/internal/adminapi/agent_handler.go` — 管理后台 Agent API。
- 测试：`agent/*_test.go`、`agentapi/*_test.go`、`adminapi/agent_handler_test.go`、`judge/internal/db/repository_test.go`、`judge/internal/pipeline/pipeline_test.go`。

**修改的后端**

- `apps/server/internal/submission/model.go` — 增加 `agent_id`、`agent_attempt_id`、`agent_settled`。
- `apps/server/internal/submission/repository.go` — Agent 提交不增 `submit_count`；人类查询排除 Agent。
- `apps/server/internal/submission/service.go` — 保持人类提交默认 `AgentID=nil`。
- `apps/server/internal/db/db.go` — AutoMigrate + 部分唯一索引。
- `apps/server/internal/rbac/catalog.go`、`seed.go`、`audit.go` — `agent.manage`、seed v5、审计常量。
- `apps/server/internal/gamification/repository.go`、`learning/repository.go`、`difficulty/recalculate.go`、`problem/stats.go`、`contest/repository.go`、`backup/account.go` — 排除 Agent。
- `apps/server/internal/router/router.go` — 挂载用户/公开/Agent token/管理路由，`Deps.Agent`。
- `apps/server/cmd/server/main.go` — 构造 Agent service、启动结算/清扫循环。
- `apps/server/internal/httpx/errors.go` — Agent 错误码。
- `apps/judge/internal/model/model.go`、`internal/db/repository.go`、`internal/pipeline/pipeline.go` — 识别 Agent 提交，跳过 `accept_count` 与游戏化事件。
- `packages/api/openapi.yaml` — Agent 契约。

**前端**

- `apps/web/src/views/AgentSettingsView.vue`、`.spec.ts` — 用户 Agent 管理/token。
- `apps/web/src/views/AgentsView.vue`、`.spec.ts` — 公开榜单。
- `apps/web/src/views/AgentDetailView.vue`、`.spec.ts` — 公开详情/历史。
- `apps/web/src/views/AdminAgentsView.vue`、`.spec.ts` — 管理后台。
- `apps/web/src/api/agents.ts`、`api/types.ts`、`api/schema.d.ts` — API 客户端/类型。
- `apps/web/src/router/index.ts`、`permissions.ts`、`components/UserMenu.vue`、`components/layout/AppNav.vue`、`components/layout/AdminNav.vue` — 路由/入口/权限。

---

### Task 1: Agent schema、submissions 扩展、RBAC 权限

**Files:**
- Create: `apps/server/internal/agent/model.go`
- Modify: `apps/server/internal/submission/model.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/rbac/catalog.go`, `apps/server/internal/rbac/seed.go`, `apps/server/internal/rbac/audit.go`
- Test: `apps/server/internal/db/db_test.go`, `apps/server/internal/rbac/seed_test.go`

**Interfaces:**
```go
package agent

type Status string
const (
    StatusActive   Status = "active"
    StatusDisabled Status = "disabled"
)

type AttemptStatus string
const (
    AttemptActive    AttemptStatus = "active"
    AttemptSolved    AttemptStatus = "solved"
    AttemptExhausted AttemptStatus = "exhausted"
    AttemptAbandoned AttemptStatus = "abandoned"
    AttemptExpired   AttemptStatus = "expired"
)

type RatingKind string
const (
    RatingSolve     RatingKind = "solve"
    RatingAbandon   RatingKind = "abandon"
    RatingTimeout   RatingKind = "timeout"
    RatingExhausted RatingKind = "exhausted"
    RatingReset     RatingKind = "reset"
)

type Agent struct {
    ID             int64
    UserID         int64
    Name           string
    ModelName      string
    ModelIntro     string
    Status         Status
    Rating         int
    PassedCount    int
    AttemptedCount int
    AvgSolveTimeMS int
    CreatedAt, UpdatedAt time.Time
}

type AgentToken struct {
    ID          int64
    AgentID     int64
    TokenHash   string
    TokenPrefix string
    CreatedAt   time.Time
    LastUsedAt  *time.Time
    RevokedAt   *time.Time
}

type AgentProblemAttempt struct {
    ID               int64
    AgentID          int64
    ProblemID        int64
    AttemptsUsed     int
    Status           AttemptStatus
    ClaimedAt        time.Time
    DeadlineAt       time.Time
    LastSubmissionAt *time.Time
    SolvedAt         *time.Time
    SolveTimeMS      int
    CreatedAt, UpdatedAt time.Time
}

type AgentRatingHistory struct {
    ID            int64
    AgentID       int64
    ProblemID     int64
    SubmissionID  *int64
    Kind          RatingKind
    Difficulty    int
    RatingBefore  int
    RatingAfter   int
    Delta         int
    SolveTimeMS   int
    CreatedAt     time.Time
}
```

- GORM tags:
  - `agents.user_id uniqueIndex:uniq_agents_user`; `agents.name uniqueIndex:uniq_agents_name`; `status size:20 default:active`; `rating default:1500`; counts default 0; `avg_solve_time_ms`.
  - `agent_tokens.token_hash char(64) uniqueIndex`; `token_prefix varchar(24)`; `agent_id index`.
  - `agent_problem_attempts` unique `(agent_id, problem_id)`; index `(agent_id, status)`.
  - `agent_rating_history` index `(agent_id, created_at)`; `problem_id`, `submission_id` nullable.
- `submission.Submission` adds:
  ```go
  AgentID        *int64 `gorm:"index:idx_submissions_agent_settled,priority:1;index"`
  AgentAttemptID *int64 `gorm:"index"`
  AgentSettled   bool   `gorm:"not null;default:false;index:idx_submissions_agent_settled,priority:2"`
  ```
- `db.Migrate` AutoMigrate `&agent.Agent{}`, `&agent.AgentToken{}`, `&agent.AgentProblemAttempt{}`, `&agent.AgentRatingHistory{}` after `&submission.Submission{}`.
- Raw idempotent partial unique indexes:
  ```sql
  CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_tokens_active
    ON agent_tokens (agent_id) WHERE revoked_at IS NULL;
  CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_attempts_active
    ON agent_problem_attempts (agent_id) WHERE status = 'active';
  CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_solve
    ON agent_rating_history (agent_id, problem_id) WHERE kind = 'solve';
  CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_fail
    ON agent_rating_history (agent_id, problem_id)
    WHERE kind IN ('abandon', 'timeout', 'exhausted');
  CREATE UNIQUE INDEX IF NOT EXISTS uniq_agent_rating_submission
    ON agent_rating_history (submission_id) WHERE submission_id IS NOT NULL;
  CREATE INDEX IF NOT EXISTS idx_submissions_agent_unsettled
    ON submissions (agent_id)
    WHERE agent_id IS NOT NULL AND agent_settled = false;
  ```
- RBAC:
  - `PermAgentManage PermissionCode = "agent.manage"`; group `PermissionGroupAgent = "Agent"`; catalog 名称“管理 Agent”，描述“停用/恢复 Agent、查看 Agent 列表与重置 Rating”。
  - `SeedVersion = "5"`; `if stored < 5 { migrateSeedV5(tx) }`；`migrateSeedV5` 为空（super_admin 自动拥有，其他角色不自动授予）。
  - `AuditActionAgentManage = "agent.manage"`、`AuditTargetAgent = "agent"`。

- [ ] **Step 1: 写失败测试**

```go
func TestMigrateCreatesAgentSchema(t *testing.T) {
    // Migrate; assert tables agents/agent_tokens/agent_problem_attempts/agent_rating_history;
    // assert submissions has agent_id, agent_attempt_id, agent_settled;
    // assert pg_get_indexdef contains the five partial unique indexes.
}

func TestAgentSchemaConstraints(t *testing.T) {
    // create user; insert agent; second agent same user fails; same name other user fails;
    // second active token for agent fails; second active attempt fails;
    // duplicate solve history and duplicate fail history fail.
}
```

`rbac/seed_test.go`：expected permission codes 加 `agent.manage`，并断言 group/name、`SeedVersion == "5"`。

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/db -run 'TestMigrateCreatesAgentSchema|TestAgentSchemaConstraints' -v
cd apps/server && go test -p 1 ./internal/rbac -run 'TestPermissionCatalogExactCodes|TestAgentPermissionCatalog' -v
```
Expected: FAIL，表和字段不存在。

- [ ] **Step 3: 实现模型、迁移、索引、权限**

- `db_test.go` 是 `package db`，**不要** import `testutil`；用 `open(dsn, io.Discard)` + `Migrate`，用固定唯一 ID/名称清理测试行，不 truncate 全库。
- 不新增数据回填；人类提交 `agent_id` 为 NULL。
- 权限只进目录，不修改既有角色授权。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/model.go apps/server/internal/submission/model.go apps/server/internal/db/db.go apps/server/internal/db/db_test.go apps/server/internal/rbac
git commit -m "feat(agent): add agent arena schema and permission"
```

---

### Task 2: 人类统计与 judge 隔离 Agent 提交

**Files:**
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/gamification/repository.go`
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/difficulty/recalculate.go`
- Modify: `apps/server/internal/problem/stats.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/companion/repository.go`
- Modify: `apps/server/internal/backup/account.go`
- Modify: `apps/judge/internal/model/model.go`, `apps/judge/internal/db/repository.go`, `apps/judge/internal/pipeline/pipeline.go`
- Test: 上列包对应 `_test.go`

**Interfaces:**
- `submission.Repository.Create`: 仅当 `sub.AgentID == nil` 时 `problems.submit_count + 1`；Agent 提交只验证 problem 存在。
- `submission.Repository.List` / `FindDetail`: 始终 `s.agent_id IS NULL`（公开提交记录不展示 Agent）。
- `HasAccepted`、`CountUserActivity`、`ListCodeSubmissionTimes`: 增加 `agent_id IS NULL`。
- judge `model.Submission` 增加 `AgentID *int64`；`Claim` 查询/RETURNING 增加 `agent_id`。
- judge `Finish`：`SELECT accepted_counted, agent_id`；只有 `agent_id IS NULL` 且新 AC 且未计数时 `accept_count + 1`；Agent 提交永远不把 `accepted_counted` 置 true。
- judge `Pipeline.finish`：`if in.Status == Accepted && sub.AgentID == nil { publishGameAccepted }`。
- `submission.ResetForRejudge`：`sub.AgentID != nil` 直接返回 `ErrNotRejudgeable`，防止管理员按 ID 重测 Agent 提交时错误回退人类 `accept_count`。
- 人类统计 SQL 全部加 `agent_id IS NULL`：
  - `gamification.CountAcceptedProblems`、`FindAcceptedSubmission`、`HasEarlierAccepted`。
  - `learning.AcceptedProblemIDs`。
  - `difficulty.RecalculateService.aggregate`。
  - `problem.StatusCounts`、`ListSubmissionMetrics`。
  - `contest.ProblemStats`、`UserProblemStats`（防御性；Agent 本无 contest_id）。
  - `companion.HasEarlierAccepted`。
  - `backup/account.go` 账号恢复后的 `submit_count/accept_count` 重算。

- [ ] **Step 1: 写失败测试**

```go
func TestHumanSubmissionQueriesExcludeAgent(t *testing.T) {
    // 同一用户 + 同一题：人类 WA、Agent AC；人类 List/HasAccepted/Activity 都看不到 Agent；
    // Agent 提交不增加 problems.submit_count。
}
func TestJudgeAgentSubmissionSkipsAcceptCountAndGameEvent(t *testing.T) {
    // 插入 agent_id 的 pending 提交；Finish(accepted) 后 accept_count 不变、无 game 事件；
    // 人类提交仍 +1 并发事件。
}
func TestDifficultyAggregateExcludesAgentSubmissions(t *testing.T) {
    // 100 个 Agent AC + 1 个人类 WA 不改变人类样本通过率。
}
func TestProblemAndGamificationStatsExcludeAgentSubmissions(t *testing.T) {
    // 题目状态统计、个人 AC 数、学习进度不包含 Agent。
}
func TestAgentSubmissionCannotBeRejudged(t *testing.T) {
    // 直接按 ID 调用 ResetForRejudge 返回 ErrNotRejudgeable，accept_count 不变。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/submission ./internal/gamification ./internal/learning ./internal/difficulty ./internal/problem ./internal/contest ./internal/backup -run 'TestHumanSubmissionQueriesExcludeAgent|TestDifficultyAggregateExcludesAgent|TestProblemAndGamificationStatsExcludeAgent|TestAgentSubmissionCannotBeRejudged' -v
cd apps/judge && go test -p 1 ./internal/db ./internal/pipeline -run 'TestJudgeAgentSubmission' -v
```
Expected: FAIL，Agent 提交被统计。

- [ ] **Step 3: 实现隔离**

- 不要修改人类提交的行为；Agent 只是额外过滤。
- judge 仍正常评测 Agent 提交并写状态/时间/内存/分数，只跳过计数和游戏化事件。
- 备份重算必须排除 Agent，避免恢复账号后污染题目计数。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + 两个模块全量回归：
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/submission ./internal/gamification ./internal/learning ./internal/difficulty ./internal/problem ./internal/contest ./internal/backup
cd apps/judge && go test -p 1 ./...
```
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal apps/judge/internal
git commit -m "feat(agent): isolate agent submissions from human stats and judge counts"
```

---

### Task 3: Token 生命周期、Agent CRUD、用户 API

**Files:**
- Create: `apps/server/internal/agent/token.go`, `apps/server/internal/agent/service.go`
- Create: `apps/server/internal/agentapi/handler.go`
- Modify: `apps/server/internal/httpx/errors.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/agent/service_test.go`, `apps/server/internal/agentapi/handler_test.go`

**Interfaces:**
```go
const (
    TokenPrefix = "kapoj_agent_"
    MaxAttempts = 20
    ClaimTimeout = 30 * time.Minute
)

func GenerateToken() (plain, hash, prefix string, err error)
func HashToken(raw string) string

type ProblemLookup interface {
    FindByID(ctx context.Context, id int64) (*problem.Problem, error)
    CountTestcases(ctx context.Context, problemID int64) (int, error)
}
type ContestLookup interface {
    IsProblemLocked(ctx context.Context, problemID int64, now time.Time) (bool, error)
}
type Limiter interface {
    Allow(ctx context.Context, agentID int64, action string) (bool, error)
}

type Option func(*Service)
func WithClock(now func() time.Time) Option
func WithClaimTimeout(d time.Duration) Option

type AttemptView struct {
    ID                int64         `json:"id"`
    ProblemID         int64         `json:"problemId"`
    ProblemTitle      string        `json:"problemTitle"`
    ProblemSlug       string        `json:"problemSlug"`
    Difficulty        int           `json:"difficulty"`
    Status            AttemptStatus `json:"status"`
    AttemptsUsed      int           `json:"attemptsUsed"`
    AttemptsRemaining int           `json:"attemptsRemaining"`
    ClaimedAt         time.Time     `json:"claimedAt"`
    DeadlineAt        time.Time     `json:"deadlineAt"`
    SolvedAt          *time.Time    `json:"solvedAt"`
    SolveTimeMS       int           `json:"solveTimeMs"`
}

type AgentInput struct {
    Name       string `json:"name"`
    ModelName  string `json:"modelName"`
    ModelIntro string `json:"modelIntro"`
}

type AgentAuthor struct {
    ID       int64  `json:"id"`
    Username string `json:"username"`
    Nickname string `json:"nickname"`
}

type AgentView struct {
    ID             int64       `json:"id"`
    UserID         int64       `json:"userId"`
    Name           string      `json:"name"`
    ModelName      string      `json:"modelName"`
    ModelIntro     string      `json:"modelIntro"`
    Status         Status      `json:"status"`
    Rating         int         `json:"rating"`
    PassedCount    int         `json:"passedCount"`
    AttemptedCount int         `json:"attemptedCount"`
    AvgSolveTimeMS int         `json:"avgSolveTimeMs"`
    Author         AgentAuthor `json:"author"`
    CreatedAt      time.Time   `json:"createdAt"`
    UpdatedAt      time.Time   `json:"updatedAt"`
}

type AgentTokenView struct {
    ID          int64      `json:"id"`
    TokenPrefix string     `json:"tokenPrefix"`
    CreatedAt   time.Time  `json:"createdAt"`
    LastUsedAt  *time.Time `json:"lastUsedAt"`
}

type TokenIssue struct {
    Token string         `json:"token"`
    Meta  AgentTokenView `json:"meta"`
}

type MyAgentView struct {
    Agent          *AgentView       `json:"agent"`
    Token          *AgentTokenView  `json:"token"`
    CurrentAttempt *AttemptView     `json:"currentAttempt"`
}

type AuthAgent struct {
    Agent   Agent
    TokenID int64
}

func NewService(db *gorm.DB, problems ProblemLookup, contests ContestLookup, queue submission.Enqueuer, limiter Limiter, opts ...Option) *Service
func (s *Service) CreateAgent(ctx context.Context, userID int64, in AgentInput) (*AgentView, error)
func (s *Service) MyAgent(ctx context.Context, userID int64) (*MyAgentView, error)
func (s *Service) UpdateAgent(ctx context.Context, userID int64, in AgentInput) (*AgentView, error)
func (s *Service) GenerateToken(ctx context.Context, userID int64) (*TokenIssue, error)
func (s *Service) RevokeToken(ctx context.Context, userID int64) error
func (s *Service) AuthenticateToken(ctx context.Context, raw string) (*AuthAgent, error)
```

- 错误码：
  ```go
  ErrInvalidAgentToken     = newErr(401, 40102, "Agent token 无效或已吊销")
  ErrAgentDisabled         = newErr(403, 40304, "Agent 已停用")
  ErrNoAgentProblem        = newErr(404, 40401, "没有可领取的题目")
  ErrTooManyAgentRequests  = newErr(429, 42906, "Agent 请求过于频繁，请稍后再试")
  ```
- 校验：name trim 后 1–100 rune；modelName 1–100；modelIntro ≤500；名称重复 40001“Agent 名称已存在”；每用户已有 Agent 40001“每个用户只能创建一个 Agent”。
- 创建 Agent 初始 `status=active, rating=1500, passed_count=0, attempted_count=0, avg_solve_time_ms=0`。
- token 生成：`plain = "kapoj_agent_" + base64.RawURLEncoding(32 random bytes)`；`hash = hex(sha256(plain))`；`prefix = plain[:20]`；事务内锁定 Agent 行 → 若 Agent 已停用拒绝 → 吊销现有 active token (`revoked_at=now`) → 插入新 token；返回一次明文。
- 鉴权：hash 查 `agent_tokens JOIN agents JOIN users`；要求 token 未吊销、Agent `active`、用户未 `banned`；`last_used_at` 仅在空或超过 1 分钟时更新（SQL 条件更新，避免每次写库）。
- API（挂在既有 `/api` JWT group，`RequireAuth()` 单独加）：
  - `GET /api/agents/me` → `MyAgentView`；无 Agent 时 200 `{agent:null,token:null,currentAttempt:null}`。
  - `POST /api/agents` → 201 `AgentView`。
  - `PUT /api/agents/me` → `AgentView`。
  - `POST /api/agents/me/token` → 201 `TokenIssue`。
  - `DELETE /api/agents/me/token` → 200 `null`。
- `router.Deps` 增加 `Agent *agent.Service`；`router.New` 在 `d.Agent != nil` 时调用 `agentapi.RegisterUserRoutes(api, d.Agent)`。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateAgentOncePerUserAndUniqueName(t *testing.T) { /* ... */ }
func TestAgentTokenLifecycle(t *testing.T) {
    // generate: plaintext prefix/hash; second generate revokes old; old auth fails, new works;
    // revoke: auth fails; DB stores no plaintext.
}
func TestAgentTokenRejectsDisabledOrBanned(t *testing.T) { /* agent disabled -> 40304; user banned -> 40301 */ }
func TestAgentUserAPI(t *testing.T) { /* 401; me null; create; update; token issue/rotate/revoke */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/agent ./internal/agentapi -run 'TestCreateAgent|TestAgentToken|TestAgentUserAPI' -v
```
Expected: FAIL，类型/路由不存在。

- [ ] **Step 3: 实现 token/CRUD/API**

- 不实现随机题/提交/结算（Task 4–6）。
- 不修改旧用户 API。
- 所有写操作带 `user_id` 条件，防越权。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/agent ./internal/agentapi ./internal/router`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/token.go apps/server/internal/agent/service.go apps/server/internal/agent/*_test.go apps/server/internal/agentapi apps/server/internal/httpx/errors.go apps/server/internal/router/router.go
git commit -m "feat(agent): add agent CRUD and token lifecycle"
```

---

### Task 4: Rating 引擎、随机领题、Agent 鉴权路由

**Files:**
- Create: `apps/server/internal/agent/rating.go`, `apps/server/internal/agent/arena.go`, `apps/server/internal/agent/limiter.go`
- Create: `apps/server/internal/agentapi/agent_handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/agent/rating_test.go`, `arena_test.go`, `agentapi/agent_handler_test.go`

**Interfaces:** `ProblemLookup`、`ContestLookup`、`Limiter`、`AttemptView` 已在 Task 3 定义；本任务新增：
```go
type AgentProblemView struct {
    ID             int64    `json:"id"`
    Title          string   `json:"title"`
    Slug           string   `json:"slug"`
    Type           problem.Type `json:"type"`
    StatementMD    string   `json:"statementMd"`
    InputFormatMD  string   `json:"inputFormatMd"`
    OutputFormatMD string   `json:"outputFormatMd"`
    Samples        []problem.Sample `json:"samples"`
    Tags           []string `json:"tags"`
    Difficulty     int      `json:"difficulty"`
    TimeLimitMS    int      `json:"timeLimitMs"`
    MemoryLimitMB  int      `json:"memoryLimitMb"`
    Source         string   `json:"source"`
}

type ClaimResult struct {
    Attempt AttemptView      `json:"attempt"`
    Problem AgentProblemView `json:"problem"`
}

type AgentMeView struct {
    Agent          AgentView    `json:"agent"`
    CurrentAttempt *AttemptView `json:"currentAttempt"`
}

func DifficultyRating(difficulty int) float64
func ExpectedScore(rating, difficulty int) float64
func SolveDelta(rating, difficulty int) int
func FailureDelta(rating, difficulty int) int
func (s *Service) Claim(ctx context.Context, agentID int64) (*ClaimResult, error)
func (s *Service) AgentMe(ctx context.Context, agentID int64) (*AgentMeView, error)
```

- `DifficultyRating` clamp difficulty 1–100（越界按 1/100）。
- `SolveDelta` / `FailureDelta` 用 `math.Round`，clamp 如全局约束。
- `applySolve(tx, agent, attempt, submissionID, now)`、`applyFailure(tx, agent, attempt, submissionID *int64, kind, now)`：
  - 调用方已锁 `agents` 行。
  - solve 仅在无 solve history 时插入；`passed_count +=1`；平均用时 `round((oldAvg*oldPassed + solveTime)/(oldPassed+1))`。
  - failure 仅在无该题 failure history 时插入；`rating = max(0, rating+delta)`；`delta` 存实际差值。
  - 唯一索引兜底。
- `Claim` 事务：
  - `SELECT ... FROM agents WHERE id=? FOR UPDATE`。
  - 有 active attempt：未过期直接返回；已过期且有 pending/judging → 50001? 用 `httpx.ErrValidation("当前题目正在评测，请等待结果")`；已过期且无 pending → `expireAttemptTx(kind=timeout)` 后继续。
  - 随机 SQL：
    ```sql
    SELECT p.* FROM problems p
    WHERE p.is_public = true
      AND p.type <> 'interactive'
      AND (p.type = 'objective' OR EXISTS (
        SELECT 1 FROM problem_testcases tc WHERE tc.problem_id = p.id))
      AND (p.type <> 'special' OR p.checker_path <> '')
      AND NOT EXISTS (
        SELECT 1 FROM contest_problems cp JOIN contests c ON c.id = cp.contest_id
        WHERE cp.problem_id = p.id AND c.end_time > now())
      AND NOT EXISTS (
        SELECT 1 FROM agent_problem_attempts a
        WHERE a.agent_id = ? AND a.problem_id = p.id
          AND a.status IN ('solved','exhausted'))
    ORDER BY random() LIMIT 1
    ```
  - 无题 → `httpx.ErrNoAgentProblem`。
  - upsert attempt：`ON CONFLICT (agent_id, problem_id) DO UPDATE SET status='active', claimed_at=now, deadline_at=now+30m, solved_at=NULL, solve_time_ms=0, updated_at=now`；保留 `attempts_used`。
  - 返回 attempt + 公开题面。
- 限流：`RedisLimiter` 固定窗口 key `kapoj:agent:rl:<action>:<agentID>:<unixMinute>`，claim/submit 10/min；nil limiter 时放行。
- `agentapi.RegisterAgentRoutes(public, svc)` 挂 `/api/agent`：
  - `GET /agent/me`
  - `POST /agent/problems/random`
- `requireAgentToken(svc)`：解析 Bearer，调用 `AuthenticateToken`；成功后 `c.Set("agent_auth", authAgent)`；失败 401/403。

- [ ] **Step 1: 写失败测试**

```go
func TestAgentRatingFormula(t *testing.T) {
    // difficulty 10/30/50/70/90 at 1500 => +1,+7,+20,+29,+32; -15,-13,-6,-1,-1.
    // R=2400 difficulty 1 => solve +1; R=0 difficulty 100 => failure delta 0? -> -1 but floor 0 -> actual 0.
}
func TestAgentClaimReturnsActiveAttempt(t *testing.T) { /* second claim returns same attempt, deadline unchanged */ }
func TestAgentClaimExcludesUnavailableProblems(t *testing.T) {
    // hidden, locked contest, solved, exhausted, interactive, no-testcase -> excluded.
}
func TestAgentClaimExpiresStaleAttemptWithTimeoutPenalty(t *testing.T) { /* deadline past, no pending */ }
func TestAgentClaimWaitsForPendingSubmission(t *testing.T) { /* expired + pending -> 40001 */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/agent -run 'TestAgentRating|TestAgentClaim' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现 Rating/claim/限流/Agent 路由**

- 锁顺序：`agents` → `agent_problem_attempts` → `submissions`；所有写入遵守。
- 随机题只返回公开字段，绝不返回测试数据/题解/answers。
- 不实现提交/结算 worker（Task 5–6）。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/agent ./internal/agentapi`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/rating.go apps/server/internal/agent/arena.go apps/server/internal/agent/limiter.go apps/server/internal/agent/*_test.go apps/server/internal/agentapi apps/server/internal/router/router.go
git commit -m "feat(agent): add rating engine and random problem claiming"
```

---

### Task 5: 提交、查询、放弃、20 次限制

**Files:**
- Modify: `apps/server/internal/agent/arena.go`
- Modify: `apps/server/internal/agentapi/agent_handler.go`
- Test: `apps/server/internal/agent/arena_test.go`, `apps/server/internal/agentapi/agent_handler_test.go`

**Interfaces:**
```go
type SubmissionInput struct {
    Language submission.Language `json:"language"`
    Code     string              `json:"code"`
    Answer   string              `json:"answer"`
}

type AgentSubmitResult struct {
    SubmissionID     int64               `json:"submissionId"`
    Status           submission.Status   `json:"status"`
    AttemptsUsed     int                 `json:"attemptsUsed"`
    AttemptsRemaining int                `json:"attemptsRemaining"`
    Exhausted        bool                `json:"exhausted"`
}

type AgentSubmissionView struct {
    ID                int64             `json:"id"`
    ProblemID         int64             `json:"problemId"`
    Status            submission.Status `json:"status"`
    TimeUsedMS        int               `json:"timeUsedMs"`
    MemoryUsedKB      int               `json:"memoryUsedKb"`
    Score             int               `json:"score"`
    ErrorMessage      string            `json:"errorMessage"`
    AttemptsUsed      int               `json:"attemptsUsed"`
    AttemptsRemaining int               `json:"attemptsRemaining"`
    Exhausted         bool              `json:"exhausted"`
    RatingDelta       *int              `json:"ratingDelta"`
    CreatedAt         time.Time         `json:"createdAt"`
    JudgedAt          *time.Time        `json:"judgedAt"`
}

type AbandonResult struct {
    Status            AttemptStatus `json:"status"`
    RatingDelta       *int          `json:"ratingDelta"`
    AttemptsUsed      int           `json:"attemptsUsed"`
    AttemptsRemaining int           `json:"attemptsRemaining"`
}

func (s *Service) Submit(ctx context.Context, agentID, attemptID int64, in SubmissionInput) (*AgentSubmitResult, error)
func (s *Service) Submission(ctx context.Context, agentID, submissionID int64) (*AgentSubmissionView, error)
func (s *Service) Abandon(ctx context.Context, agentID, attemptID int64) (*AbandonResult, error)
```

- `Submit` 事务：
  1. 锁 `agents` 行，再锁 attempt `FOR UPDATE`；校验 `agent_id` 匹配（否则 404）。
  2. attempt 必须 `active`；`deadline_at > now`；`attempts_used < 20`；无 `pending/judging` 提交。
  3. 读取 problem；重新校验 `is_public`、`!IsProblemLocked(now)`、可评测；`in.ValidateForProblem(p.Type)`。
  4. `attempts_used += 1`；若旧值为 0，`agents.attempted_count += 1`；`last_submission_at=now`。
  5. 插入 `submission.Submission{UserID: agent.UserID, ProblemID, AgentID:&agent.ID, AgentAttemptID:&attempt.ID, AgentSettled:false, Status:pending, ...}`；不设 contest。
  6. 提交后只 `queue.Enqueue`；**不**调用公共 `submissionevents` publisher（Agent 提交不进入人类实时提交流）。入队失败标记 `system_error`（与人类提交一致），不退还次数。
- 第 20 次非 AC 的结算由 worker 设置 `exhausted`（Task 6）；提交接口在 `attempts_used == 20` 后拒绝继续。
- `Submission`：按 `(id, agent_id)` 查询；未结算 `ratingDelta=nil`；已结算从 `agent_rating_history.submission_id` 取。
- `Abandon`：
  - 锁 agent → attempt；必须 active；`deadline_at <= now` → 调 `expireAttemptTx(timeout)` 并返回 40001 “题目已超时，请重新领取”。
  - 有 pending/judging → 40001 “当前题目正在评测，请等待结果”。
  - 设置 `status=abandoned`；`applyFailure(kind=abandon, submissionID=nil)`；返回实际 delta。
- API 路由：
  - `POST /api/agent/attempts/:id/submissions`
  - `GET /api/agent/submissions/:id`
  - `POST /api/agent/attempts/:id/abandon`
- HTTP 状态：提交成功 201；查询 200；放弃 200。

- [ ] **Step 1: 写失败测试**

```go
func TestAgentSubmitLimit(t *testing.T) {
    // 19 WA -> remaining 1; 20th WA -> 201, attemptsUsed 20, remaining 0;
    // 21st submit rejected; exhausted 状态与扣分留到 Task 6 验证。
}
func TestAgentSubmitRequiresActiveAttemptAndNoPending(t *testing.T) { /* foreign/expired/pending rejected */ }
func TestAgentAbandonPenaltyAndPendingGuard(t *testing.T) { /* abandon once -> delta; second abandon 400; pending -> 400 */ }
func TestAgentSubmissionViewBeforeSettlement(t *testing.T) { /* pending/terminal 且未结算 -> ratingDelta nil */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/agent ./internal/agentapi -run 'TestAgentSubmit|TestAgentAbandon|TestAgentSubmissionView' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现提交/查询/放弃**

- Agent 提交直接写 `submissions`（不复用 `submission.Service.Create` 的 contest/limiter 路径），但复用 `Input.ValidateForProblem` 和 problem/contest 校验语义。
- `submission.Status` 使用现有常量；`JudgeResult` 初始化为空。
- `CodeLength`：代码取 `len(Code)`，客观题取 `len(Answer)`。
- 不修改 judge 行为（Task 2 已完成隔离）。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/agent ./internal/agentapi ./internal/submission`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/arena.go apps/server/internal/agent/arena_test.go apps/server/internal/agentapi
git commit -m "feat(agent): add agent submissions and abandon flow"
```

---

### Task 6: 结算 worker、超时清扫、启动接入

**Files:**
- Create: `apps/server/internal/agent/worker.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/agent/worker_test.go`

**Interfaces:**
```go
func (s *Service) SettleOnce(ctx context.Context, limit int) (int, error)
func (s *Service) SweepOnce(ctx context.Context, limit int) (int, error)
func (s *Service) RunSettler(ctx context.Context, interval time.Duration)
func (s *Service) RunSweeper(ctx context.Context, interval time.Duration)
```

- `RunSettler` 默认 `30 * time.Second`；启动立即执行一次；`RunSweeper` 默认 `time.Minute`。
- `SettleOnce` 查询：
  ```sql
  SELECT id FROM submissions
  WHERE agent_id IS NOT NULL AND agent_settled = false
    AND status IN ('accepted','wrong_answer','time_limit_exceeded',
                   'memory_limit_exceeded','runtime_error','compile_error','system_error')
  ORDER BY id LIMIT ?
  ```
- 每条结算事务：
  1. 读取 submission 的 agent_id/attempt_id/problem_id/status/judged_at；锁 `agents` → attempt → submission (`FOR UPDATE`)。
  2. 已 settled 直接返回。
  3. AC 且 attempt ≠ solved：`solveTime = max(0, judgedAt-or-now - claimedAt)`；attempt → solved；`applySolve`；写 solve history（`submission_id=sub.ID`）。
  4. AC 且 attempt 已 solved：只标记 settled。
  5. 非 AC 且 attempt 为 active 且 `attempts_used >= 20`：attempt → exhausted；`applyFailure(kind=exhausted, submission_id=sub.ID)`。
  6. 非 AC 且 attempt 为 active 且 `<20`：只标记 settled，保持 active。
  7. 非 AC 且 attempt 已 abandoned/expired：只标记 settled（失败分已在放弃/超时时扣过）。
  8. 标记 `submissions.agent_settled=true`。
- `SweepOnce` 查询 `agent_problem_attempts WHERE status='active' AND deadline_at < now() ORDER BY id LIMIT ?`；每条事务锁 agent → attempt：
  - 重读 status/deadline；非 active 跳过。
  - 存在 `pending/judging` 提交 → 跳过（等待评测）。
  - 设置 expired；`applyFailure(kind=timeout, submission_id=nil)`。
- `main.go`：
  - 构造 `agent.NewService(g, problemRepo, contestRepo, queue, agent.NewRedisLimiter(rdb), ...)`。
  - `router.Deps.Agent = agentSvc`。
  - `go agentSvc.RunSettler(workCtx, 30*time.Second)`、`go agentSvc.RunSweeper(workCtx, time.Minute)`。
- 锁顺序保持 `agents` → `agent_problem_attempts` → `submissions`。

- [ ] **Step 1: 写失败测试**

```go
func TestAgentSettlementAcceptedOnce(t *testing.T) {
    // AC -> rating + delta, passed_count 1, avg time, history solve; second SettleOnce no change.
}
func TestAgentTwentiethAcceptedCounts(t *testing.T) { /* 19 WA + 20th AC -> solved, passed_count +1 */ }
func TestAgentSettlementExhaustedAfter20Failures(t *testing.T) { /* 20th WA -> exhausted + one failure penalty + "错误次数耗尽" */ }
func TestAgentSubmissionViewIncludesDeltaAfterSettlement(t *testing.T) { /* settled solve/exhausted -> delta from history */ }
func TestAgentFailurePenaltyOncePerProblem(t *testing.T) {
    // abandon then re-claim then timeout -> only one failure history/penalty.
}
func TestAgentSweepSkipsPendingAndExpiresIdle(t *testing.T) { /* pending skipped; idle -> expired + timeout */ }
func TestAgentSettlementRatingFloor(t *testing.T) { /* R=2, failure -> R=0, delta=-2 */ }
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/agent -run 'TestAgentSettlement|TestAgentSweep|TestAgentFailurePenalty' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现 worker 并接入 main**

- worker 幂等：`agent_settled` + 唯一 history 双重保护。
- `SweepOnce` 不处理 `status != active`；不重复扣分。
- 后台循环必须响应 `ctx.Done()`；启动时立即跑一次，失败只记录日志不退出进程。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/agent` + `go build ./...`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/worker.go apps/server/internal/agent/worker_test.go apps/server/cmd/server/main.go apps/server/internal/router/router.go
git commit -m "feat(agent): settle ratings and expire idle attempts"
```

---

### Task 7: 公开榜单/详情、Rating 历史、管理 API

**Files:**
- Modify: `apps/server/internal/agent/service.go`
- Modify: `apps/server/internal/agentapi/handler.go`
- Create: `apps/server/internal/adminapi/agent_handler.go`
- Modify: `apps/server/internal/adminapi/role_handler.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/agent/service_test.go`, `agentapi/handler_test.go`, `adminapi/agent_handler_test.go`

**Interfaces:**
```go
type AgentFilter struct {
    Model string
    Sort  string // rating | passed | avg_time
}

type AgentDetailView struct {
    Agent AgentView `json:"agent"`
}

type AgentRatingRow struct {
    ID           int64      `json:"id"`
    ProblemID    int64      `json:"problemId"`
    ProblemTitle string     `json:"problemTitle"`
    ProblemSlug  string     `json:"problemSlug"`
    Kind         RatingKind `json:"kind"`
    Difficulty   int        `json:"difficulty"`
    RatingBefore int        `json:"ratingBefore"`
    RatingAfter  int        `json:"ratingAfter"`
    Delta        int        `json:"delta"`
    SolveTimeMS  int        `json:"solveTimeMs"`
    CreatedAt    time.Time  `json:"createdAt"`
}

type AdminAgentInput struct {
    Status      Status `json:"status"`
    ResetRating bool   `json:"resetRating"`
}

func (s *Service) ListAgents(ctx context.Context, f AgentFilter, page, size int) (httpx.Page[AgentView], error)
func (s *Service) AgentDetail(ctx context.Context, id int64) (*AgentDetailView, error)
func (s *Service) RatingHistory(ctx context.Context, agentID int64, page, size int) (httpx.Page[AgentRatingRow], error)
func (s *Service) AdminList(ctx context.Context, status, model string, page, size int) (httpx.Page[AgentView], error)
func (s *Service) AdminUpdate(ctx context.Context, id int64, in AdminAgentInput) (*AgentView, error)
```

- 公开 API（`agentapi.RegisterPublicRoutes(api, d.Agent)`）：
  - `GET /api/agents?page=&page_size=&model=&sort=` → 分页榜单；默认 `rating DESC, id ASC`；`sort=passed` → `passed_count DESC`; `sort=avg_time` → `avg_solve_time_ms ASC`，只包含 `passed_count>0`。
  - `GET /api/agents/:id` → `AgentDetailView`；不存在 404。
  - `GET /api/agents/:id/rating-history` → 分页历史；`problem_title/slug` join；`reset` 的 problem 字段为空。
  - `GET /api/agents/me/rating-history`（需登录）→ 当前用户的 Agent 历史；无 Agent 404。
- 管理 API（`agent.manage`）：
  - `GET /api/admin/agents?status=&model=&page=&page_size=` → `Page[AgentView]`。
  - `PUT /api/admin/agents/:id` body `{status:"active|disabled", resetRating:false}`：
    - `status` 为空时保持当前状态；`disabled` → 事务内设置 status 并吊销所有 active token；`active` 只恢复状态，不恢复旧 token。
    - `resetRating` → `rating=1500`，写 `kind=reset, problem_id=0` 的 history，`delta=1500-old`；管理事务先锁 `agents` 行。
  - 路由使用 `requirePermission(..., string(rbac.PermAgentManage))` + `middleware.AuditAdminAction(..., string(rbac.PermAgentManage), rbac.AuditTargetAgent)`。
- `adminapi.Options` 增加 `Agent *agent.Service`；`router.Deps.Agent` 同时传给 adminapi。

- [ ] **Step 1: 写失败测试**

```go
func TestAgentLeaderboardFiltersAndSorts(t *testing.T) { /* model, sort, pagination, author */ }
func TestAgentPublicDetailAndHistory(t *testing.T) { /* 404; history includes difficulty/delta; no hidden data */ }
func TestAdminAgentUpdateDisablesAndResets(t *testing.T) {
    // disable revokes tokens and blocks API; enable does not restore token;
    // reset rating -> 1500 + reset history; audit permission required.
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
set -a; . "$PWD/.env"; set +a; cd apps/server && go test -p 1 ./internal/agent ./internal/agentapi ./internal/adminapi -run 'TestAgentLeaderboard|TestAgentPublic|TestAdminAgent' -v
```
Expected: FAIL。

- [ ] **Step 3: 实现公开/管理 API**

- 公开详情不返回 token、当前题目、测试数据、题解。
- 停用 Agent 立即阻止 Agent token；恢复后必须重新生成 token。
- 管理列表包含作者信息；`resetRating` 只在显式 true 时执行。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `go test -p 1 ./internal/agent ./internal/agentapi ./internal/adminapi ./internal/router`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent apps/server/internal/agentapi apps/server/internal/adminapi apps/server/internal/router/router.go
git commit -m "feat(agent): add leaderboard, detail, history and admin API"
```

---

### Task 8: OpenAPI、类型与前端 API 客户端

**Files:**
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/agents.ts`

**Interfaces:**
- Paths：
  - `GET /agents`（公开榜单，`page/page_size/model/sort`）
  - `GET /agents/{id}`
  - `GET /agents/{id}/rating-history`
  - `GET /agents/me/rating-history`
  - `GET /agents/me`
  - `POST /agents`
  - `PUT /agents/me`
  - `POST /agents/me/token`
  - `DELETE /agents/me/token`
  - `POST /agent/problems/random`
  - `GET /agent/me`
  - `POST /agent/attempts/{id}/submissions`
  - `GET /agent/submissions/{id}`
  - `POST /agent/attempts/{id}/abandon`
  - `GET /admin/agents`
  - `PUT /admin/agents/{id}`
- Schemas：
  - `AgentInput`、`AgentView`、`AgentTokenView`、`AgentTokenIssue`、`MyAgentView`、`AgentDetailView`、`AgentRatingRow`、`AgentProblemView`、`AgentAttemptView`、`AgentClaimResult`、`AgentMeView`、`AgentSubmitInput`、`AgentSubmitResult`、`AgentSubmissionView`、`AgentAbandonResult`、`AdminAgentInput`。
  - 复用现有 `Page`/envelope 风格；错误码 40001/40102/40304/40400/42906。
- API functions：
  ```ts
  listAgents(params?); getAgent(id); getAgentRatingHistory(id, params?);
  getMyAgentRatingHistory(params?); getMyAgent(); createAgent(input); updateMyAgent(input);
  generateAgentToken(); revokeAgentToken();
  claimAgentProblem(); getAgentMe(); submitAgentAttempt(id, input);
  getAgentSubmission(id); abandonAgentAttempt(id);
  listAdminAgents(params?); updateAdminAgent(id, input);
  ```

- [ ] **Step 1: 基线构建**

Run: `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 2: 修改 OpenAPI 并生成**

Run: `corepack pnpm gen:api`；在 `types.ts` 添加生成类型别名；新增 `api/agents.ts`。

- [ ] **Step 3: 构建确认**

Run: `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 4: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/agents.ts
git commit -m "feat(api): add agent arena contracts and client"
```

---

### Task 9: 用户 Agent 管理页

**Files:**
- Create: `apps/web/src/views/AgentSettingsView.vue`, `AgentSettingsView.spec.ts`
- Modify: `apps/web/src/router/index.ts`, `apps/web/src/components/UserMenu.vue`, `apps/web/src/components/UserMenu.spec.ts`

**Interfaces / behavior:**
- Route `/settings/agent` name `agent-settings`，`requiresAuth: true`，标题“我的 Agent”；UserMenu 增加“我的 Agent”入口。
- 无 Agent：创建表单（名称、所属模型、模型简介），提交后显示详情。
- 有 Agent：
  - 基本信息编辑（名称/模型/简介）。
  - 状态、作者、Rating、通过题数/尝试题数、平均用时。
  - token 卡片：无 token 显示“生成 Token”；生成后一次性明文 + 复制 + “只显示一次”警告；已有 token 显示前缀、最近使用、轮换、吊销（二次确认）。
  - 接入说明：base `/api`、`Authorization: Bearer`、`POST /agent/problems/random`、提交/查询示例。
  - 当前 attempt（若有）：题目、难度、剩余次数、截止时间；只展示，不提供用户侧放弃。
- 错误中文；加载/提交中状态；token 明文不落 localStorage。
- 使用 `api/agents.ts`，不直接拼 axios。

- [ ] **Step 1: 写失败测试**

```ts
it('creates an agent when none exists', ...)
it('shows stats and rating history', ...)
it('generates a token once and warns it will not be shown again', ...)
it('rotates and revokes tokens with confirmation', ...)
it('edits agent metadata', ...)
it('shows validation and server errors in Chinese', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AgentSettingsView.spec.ts src/components/UserMenu.spec.ts`，Expected: FAIL。

- [ ] **Step 3: 实现页面**

- 复用现有 PrimeVue 表单/卡片/Toast 风格。
- 不新增全局状态；页面本地状态即可。
- 历史分页；token 明文只保存在组件内存，刷新/离开即丢弃。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AgentSettingsView.vue apps/web/src/views/AgentSettingsView.spec.ts apps/web/src/router/index.ts apps/web/src/components/UserMenu.vue apps/web/src/components/UserMenu.spec.ts
git commit -m "feat(web): add agent settings and token management"
```

---

### Task 10: 公开 Agent 榜单与详情页

**Files:**
- Create: `apps/web/src/views/AgentsView.vue`, `AgentsView.spec.ts`
- Create: `apps/web/src/views/AgentDetailView.vue`, `AgentDetailView.spec.ts`
- Modify: `apps/web/src/router/index.ts`, `apps/web/src/components/layout/AppNav.vue`, `AppNav.spec.ts`

**Interfaces / behavior:**
- 路由：`/agents` name `agents`（公开），`/agents/:id` name `agent-detail`（公开）。
- 主导航增加“Agents”（位置在排行榜之后），公开可见。
- 榜单：排名、名称、模型、作者、Rating、通过/尝试、平均用时、状态；排序 Rating/通过数/平均用时；模型筛选；分页；点击进入详情。
- 详情：名称/模型/简介/作者/状态、Rating、通过/尝试、平均用时、Rating 历史（题目、难度、delta、前后分、用时、时间）；不显示 token/当前题目。
- 空状态、加载、错误中文；移动端可用。

- [ ] **Step 1: 写失败测试**

```ts
it('lists agents and filters by model', ...)
it('sorts by rating, passed count and average time', ...)
it('opens an agent detail with rating history', ...)
it('shows an empty state when no agents exist', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AgentsView.spec.ts src/views/AgentDetailView.spec.ts`，Expected: FAIL。

- [ ] **Step 3: 实现页面与导航**

- 不展示隐藏题/测试数据/题解。
- 公开页不要求登录；错误不泄露内部信息。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AgentsView.vue apps/web/src/views/AgentsView.spec.ts apps/web/src/views/AgentDetailView.vue apps/web/src/views/AgentDetailView.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AppNav.vue apps/web/src/components/layout/AppNav.spec.ts
git commit -m "feat(web): add public agent leaderboard and detail"
```

---

### Task 11: 管理后台 Agent 页面与权限入口

**Files:**
- Create: `apps/web/src/views/AdminAgentsView.vue`, `AdminAgentsView.spec.ts`
- Modify: `apps/web/src/router/index.ts`, `apps/web/src/permissions.ts`, `apps/web/src/permissions.spec.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`, `AdminNav.spec.ts`

**Interfaces / behavior:**
- `PERM.agent = { manage: 'agent.manage' }`；加入 `ADMIN_ACCESS_PERMISSIONS`。
- 路由 `/admin/agents` name `admin-agents`，`permission: PERM.agent.manage`，标题“Agent 管理”。
- AdminNav 新增“Agent”，位置在“AI”之后或“内容”组；仅 `agent.manage` 显示。
- 列表：Agent/作者/模型/状态/Rating/通过/尝试/平均用时；筛选状态/模型；分页。
- 操作：停用（二次确认，提示立即吊销 token）、恢复（提示需重新生成 token）、重置 Rating（二次确认，写 1500）。
- 操作后刷新列表；错误中文。

- [ ] **Step 1: 写失败测试**

```ts
it('shows the agent admin entry only with agent.manage', ...)
it('lists and filters agents', ...)
it('disables an agent and warns that tokens are revoked', ...)
it('restores an agent without restoring tokens', ...)
it('resets rating with confirmation', ...)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web exec vitest run src/views/AdminAgentsView.spec.ts src/components/layout/AdminNav.spec.ts`，Expected: FAIL。

- [ ] **Step 3: 实现管理页面/权限/导航**

- 不改变现有管理页行为。
- 重置 Rating 只调用一次 `PUT /admin/agents/:id`，body `{resetRating:true}`（不传 status，保持当前状态）。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2 + `corepack pnpm --filter web build`，Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/AdminAgentsView.vue apps/web/src/views/AdminAgentsView.spec.ts apps/web/src/router/index.ts apps/web/src/permissions.ts apps/web/src/permissions.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts
git commit -m "feat(web): add agent administration"
```

---

### Task 12: 文档、全量验证与真实冒烟

**Files:**
- Modify: `DEVELOPMENT.md`, `运行手册.md`, `README.md`, `docs/任务清单.md`, `docs/开发日志.md`
- Test: 全部后端/前端套件。

**Docs 内容：**
- Agent 擂台概念、每用户 1 个 Agent、token 生成/轮换/吊销、Agent API 流程（领题→提交→轮询→放弃）。
- 20 次限制、第 20 次 AC 规则、超时 30 分钟、pending 禁止放弃。
- Rating 公式、难度映射、首次 AC/失败扣分一次、初始 1500、下限 0。
- 随机题隔离与人类统计隔离。
- 管理后台权限 `agent.manage` 与停用/恢复/重置。
- 外部 Agent 接入示例（curl：生成 token、领题、提交、查询）。
- 安全：token 只显示一次、只存 hash、不要提交到代码库。

- [ ] **Step 1: 更新文档**

- 保持现有文档结构，新增章节而不是重写无关部分。
- README 功能列表加 Agent 擂台。
- 任务清单/开发日志加一条已完成记录。

- [ ] **Step 2: 全量验证**

Run:
```bash
set -a; . "$PWD/.env"; set +a; test_db="$KAPOJ_TEST_DATABASE_URL"
cd apps/server && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$test_db" go test -count=1 -p 1 ./...
cd apps/judge && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" KAPOJ_TEST_DATABASE_URL="$test_db" go test -count=1 -p 1 ./...
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```
Expected: PASS。

- [ ] **Step 3: 真实冒烟**

临时 DB/Redis/server + 假 Agent 客户端（不触碰主库/主 server）：
1. 创建用户 A、用户 B；用户 A 创建 Agent，重复创建被拒；用户 B 同名被拒。
2. 生成 token → 轮换 → 旧 token 401、新 token 可用；吊销后 401。
3. 用 token 领取随机公开题；重复领取返回同一 attempt；deadline 约 30 分钟。
4. 提交错误代码 → 轮询终态 → 次数减少；提交正确代码 → `accepted`，Rating 加分、`passed_count/avg_solve_time_ms` 更新。
5. 另一题连续 20 次错误 → `exhausted`，显示“错误次数耗尽”，无法再提交/领取。
6. 放弃 → 扣分一次；再次放弃/超时不重复扣分；有 pending 时放弃被拒。
7. 把 attempt deadline 改到过去 → 清扫后 `expired` + 一次 timeout 扣分。
8. 人类统计：Agent 提交不出现在 `/submissions`、不增加用户 AC/学习/游戏化/题目 `submit_count/accept_count`、不影响难度重算。
9. 管理后台停用 Agent → token 立即 403；恢复后旧 token 仍不可用；重置 Rating → 1500 + reset history。
10. 清理临时数据。

如某项无法执行，报告中明确列出未执行项与原因；不得声称已执行。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md 运行手册.md README.md docs/任务清单.md docs/开发日志.md
git commit -m "docs: document agent arena V1"
```

---

## Self-Review

- Spec coverage: 数据模型/迁移（T1）、人类隔离/judge（T2）、token/CRUD（T3）、Rating/领题（T4）、提交/20 次/放弃（T5）、结算/超时（T6）、榜单/管理（T7）、OpenAPI/客户端（T8）、用户页（T9）、公开页（T10）、管理页（T11）、文档/冒烟（T12）。
- Type consistency: `AgentView/AttemptView/AgentSubmitResult/AgentSubmissionView` 在 T3–T5 定义并在 T7–T9 使用；`applySolve/applyFailure` 在 T4 定义、T5/T6 使用；`AuthAgent` 在 T3 定义、T4 中间件使用；`AgentFilter/AdminAgentInput` 在 T7 定义、T8 契约使用。
- Review Focus tests: T3 token；T5 20 次；T6 幂等/扣分一次；T2 隔离；T4 随机池。
- 20 次与结算规则：第 20 次 AC 在 T5 测试；第 20 次失败 `exhausted` 在 T5/T6 测试。
- 未引入平台 AI 解题、Agent 比赛、Rating 衰减、模型归一化。
