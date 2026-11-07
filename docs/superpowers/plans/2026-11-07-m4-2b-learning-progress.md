# M4-2B 学习路线进度、事件与题目守卫 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 完成 M4-6 剩余部分：服务端按 AC 实时计算学习路线进度与节点解锁、AC 事件触发重算、被活跃路线引用的题目禁止下线/删除。

**Architecture:** `learning` 域新增 `EvaluateUser`，按用户可见的活跃路线计算每个节点的去重 AC 数并 upsert `user_learning_progress`；已完成节点锁存，未完成节点按最新配置重算。`GET /learning-paths` 与 `GET /learning-paths/:id/tree` 对登录用户先重算再读取。`learning.Service.HandleGameEvent` 只处理 `submission.accepted`，与 gamification 通过 `gamequeue` 组合 handler 串联。`problem.Service` 增加可选 `LearningGuard`，在题目从公开改为非公开或删除前校验活跃路线引用。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、Redis、Asynq、Vue 3（本计划不改前端）。

**Spec:** `docs/superpowers/specs/2026-11-05-m4-learning-paths-design.md`

## Global Constraints

- 节点 `solved_count` = 用户在该节点 `problem_ids_json` 中去重 AC 的题数；AC 为全时间、不限比赛/任务。
- `completed = solved_count >= required_count`；根节点未完成时 `active`，子节点仅在父节点 `completed` 时 `active`，否则 `locked`。
- 已完成节点锁存：已有 `status=completed` 的进度行保持 `completed` 与完成时 `solved_count`；修改题目或提高 `required_count` 不回退。未完成/进行中节点按最新配置重算。
- 游客不写进度：根 `active`、其余 `locked`、`solvedCount=0`。
- `EvaluateUser` 只处理 `is_active=true` 且用户可见（官方、公开、自己创建）的路线；私有路线对其他用户不可见。
- 读取树/列表时对登录用户先重算；不新增 Redis 缓存。
- AC 事件：`learning.HandleGameEvent` 只处理 `gamequeue.EventSubmissionAccepted`，调用 `EvaluateUser`；其余事件忽略。组合 handler 依次调用 gamification 与 learning，任一失败返回错误触发重试；两者幂等。
- 题目守卫：被任一活跃学习路线节点引用的题目不能从公开改为非公开，也不能删除（含只下线不物理删除的分支）；移除引用后允许。错误文案固定为「题目被学习路线引用，不能下线/删除」。
- 不修改前端；`docs/任务清单.md` 中 M4-7 学习路线前端仍待办。
- 所有用户文案中文；不推送、不合并当前 `main`。

## Review Focus

1. **解锁链正确性**：根 active、子 locked、父完成后子 active、子完成后孙 active；多根节点互不影响；`solvedCount` 去重且只统计节点题目集合内的 AC。
2. **完成锁存**：已完成节点在改题/提高 `required_count` 后保持 completed 与旧 solvedCount；未完成用户按新配置重算；不得把已完成节点降级。
3. **可见性**：游客不写行；私有路线只对作者重算；下架路线进度保留且不在读取时被改写；公开/官方路线对登录用户重算。
4. **事件幂等**：重复 `submission.accepted` 不产生错误或重复行；组合 handler 顺序执行，gamification 成功而 learning 失败时返回错误供 Asynq 重试。
5. **守卫完整性**：公开→非公开、物理删除、有提交时的下线分支都被拦截；活跃路线（含私有/官方）引用生效，下架路线不阻塞；移除节点后可下线。
6. **读取一致性**：`GET tree` 与 `GET /learning-paths` 返回的 status/solvedCount/myCompletedCount 在首次读取即为最新值。

---

### Task 1: 进度计算核心与仓储

**Files:**
- Modify: `apps/server/internal/learning/repository.go`
- Create: `apps/server/internal/learning/progress.go`
- Create: `apps/server/internal/learning/progress_test.go`
- Test: `apps/server/internal/learning/progress_test.go`

**Interfaces:**
- Produces:
  - `Repository.ListActivePathIDsForUser(ctx context.Context, userID int64) ([]int64, error)`：`is_active=true AND (is_official=true OR is_public=true OR created_by=userID)`。
  - `Repository.ListNodesByPathIDs(ctx context.Context, pathIDs []int64) ([]LearningPathNode, error)`：按 `path_id, sort_order, id` 升序。
  - `Repository.ListProgressForPaths(ctx context.Context, userID int64, pathIDs []int64) (map[int64]UserLearningProgress, error)`：key 为 nodeID；userID≤0 或 pathIDs 为空时返回空 map。
  - `Repository.AcceptedProblemIDs(ctx context.Context, userID int64) (map[int64]struct{}, error)`：`submissions.status='accepted'` 去重 `problem_id`。
  - `Repository.UpsertProgress(ctx context.Context, rows []UserLearningProgress) error`：按唯一约束 `(user_id,node_id)` upsert `status/solved_count/completed_at/updated_at`。
  - `Service.EvaluateUser(ctx context.Context, userID int64) error`：计算并写入进度；userID≤0 时 no-op。
- Consumes: 现有 `LearningPath`、`LearningPathNode`、`UserLearningProgress` 模型与 `ProgressLocked/Active/Completed` 常量。

- [ ] **Step 1: 写失败测试**

`progress_test.go` 至少覆盖：

```go
func TestEvaluateUserUnlockChain(t *testing.T)
// 根(required 2, p1+p2) -> 子(required 1, p3) -> 孙(required 1, p4)
// 无 AC: root active/0, child locked/0, grandchild locked/0
// AC p1: root active/1；AC p1+p2: root completed/2, child active/0
// AC p3: child completed/1, grandchild active/0；AC p4: grandchild completed/1

func TestEvaluateUserDeduplicatesAcceptedSubmissions(t *testing.T)
// 同一题多条 accepted 只计 1；非 accepted 不计

func TestEvaluateUserCompletedLatch(t *testing.T)
// root required 2 完成；改题/提高 required 后仍 completed + 旧 solvedCount
// 另一未完成用户按新配置重算

func TestEvaluateUserVisibilityAndInactivePath(t *testing.T)
// 作者私有路线被作者重算；其他用户不重算
// 下架路线已有进度保持不变，重新上架后继续

func TestEvaluateUserGuestNoop(t *testing.T)
// userID=0 不写任何行
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -run 'TestEvaluateUser' -v`

Expected: FAIL（方法不存在）。

- [ ] **Step 3: 实现仓储与 EvaluateUser**

- `ListActivePathIDsForUser` 使用 `Pluck("id", &ids)`。
- `ListNodesByPathIDs` 空切片直接返回 nil。
- `ListProgressForPaths` 用 `user_id=? AND path_id IN ?` 查询，map key `NodeID`。
- `AcceptedProblemIDs` 用 `Table("submissions").Where("user_id = ? AND status = ?", userID, "accepted").Distinct("problem_id").Pluck(...)`。
- `UpsertProgress` 用 `clause.OnConflict{Columns: []clause.Column{{Name:"user_id"},{Name:"node_id"}}, DoUpdates: clause.AssignmentColumns([]string{"status","solved_count","completed_at","updated_at"})}`。
- `EvaluateUser`：
  1. 取可见活跃 pathIDs、nodes、existing progress、accepted set。
  2. 按 pathID 分组；对每个节点递归 `eval(node)`：
     - 已有 `status=completed`：直接返回 completed，保留旧 solvedCount，不写 upsert。
     - 否则 `solved = 节点 ProblemIDs 与 accepted 的交集数`（去重）；`solved >= RequiredCount` 时写 completed、`CompletedAt=now`、返回 completed。
     - 未完成：parent 为 nil 或缺失 → active；parent eval 为 completed → active；否则 locked。写 active/locked、`CompletedAt=nil`。
  3. 用 memo 避免重复评估；环防御按 locked 处理且不写行。
  4. `UpsertProgress` 一次批量写入；`UpdatedAt=now`。
- 仅对 `pathIDs` 对应节点评估；不触碰下架路线。

- [ ] **Step 4: 运行测试并通过**

Run: `cd apps/server && gofmt -w internal/learning && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -run 'TestEvaluateUser' -v`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/learning
git commit -m "feat(learning): evaluate user learning progress"
```

---

### Task 2: 树与列表接入真实进度

**Files:**
- Modify: `apps/server/internal/learning/path_service.go`
- Modify: `apps/server/internal/learning/node_service.go`
- Modify: `apps/server/internal/learning/tree_test.go`
- Test: `apps/server/internal/learning/tree_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Service.EvaluateUser`、`Repository.ListProgressForPaths`。
- Produces:
  - `Service.ListPaths` 对 `v.UserID > 0` 先 `EvaluateUser`，再列路线。
  - `Service.Tree` 对 `v.UserID > 0` 先 `EvaluateUser`，再读路线/节点/进度；游客不重算。
  - `buildNodeTree(nodes, summaries, fullProblemIDs, progress map[int64]UserLearningProgress)` 与 `newNodeItem(n, summaries, fullProblemIDs, progress)`：有进度行时用 `SolvedCount/Status`，否则保持当前默认（根 active、子 locked、solved 0）。
  - 节点写接口的 `nodeItem` 传 `nil` 进度，保持编辑响应默认状态。

- [ ] **Step 1: 写失败测试**

在 `tree_test.go` 新增/调整：

```go
func TestTreeUsesEvaluatedProgress(t *testing.T)
// 用户 AC 根节点 required 题目后调用 Tree：
// root completed、child active、grandchild locked；solvedCount 正确

func TestTreeGuestStaysLocked(t *testing.T)
// 游客 root active、其余 locked、solved 0，且不产生 user_learning_progress

func TestListPathsEvaluatesMyCompletedCount(t *testing.T)
// 用户完成根节点后 GET 列表（Service.ListPaths）myCompletedCount=1

func TestTreeCompletedLatchVisible(t *testing.T)
// 已完成节点改题/提高 required 后 Tree 仍返回 completed + 旧 solvedCount
```

保留并调整现有 `TestTreeStructureAndGuestStatus`：删除「本阶段即使有已完成进度也保持游客态」的旧断言，改为游客与登录用户分别断言。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -run 'TestTree|TestListPaths' -v`

Expected: FAIL（Tree 未使用进度）。

- [ ] **Step 3: 实现接入**

- `ListPaths`：`if v.UserID > 0 { if err := s.EvaluateUser(ctx, v.UserID); err != nil { return nil, err } }`。
- `Tree`：同样先重算；随后 `progress, err := s.repo.ListProgressForPaths(ctx, v.UserID, []int64{path.ID})`；`buildNodeTree(..., progress)`。
- 修改 `buildNodeTree`/`newNodeItem` 签名；仅当 `progress[n.ID]` 存在时覆盖 `SolvedCount/Status`。游客传空 map。
- 不改变 `fullProblemIDs` 规则、节点排序、孤儿节点忽略逻辑。

- [ ] **Step 4: 运行测试并通过**

Run: `cd apps/server && gofmt -w internal/learning && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ -v`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/learning
git commit -m "feat(learning): show real progress in trees and lists"
```

---

### Task 3: AC 事件与组合 worker

**Files:**
- Create: `apps/server/internal/learning/events.go`
- Create: `apps/server/internal/learning/events_test.go`
- Create: `apps/server/internal/gamequeue/chain.go`
- Create: `apps/server/internal/gamequeue/chain_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 `Service.EvaluateUser`、现有 `gamequeue.Event`/`EventSubmissionAccepted`/`Handler`。
- Produces:
  - `Service.HandleGameEvent(ctx context.Context, ev gamequeue.Event) error`：仅 `EventSubmissionAccepted` 调用 `EvaluateUser`，`UserID<=0` 或未知类型 no-op。
  - `gamequeue.NewChain(handlers ...Handler) Handler`：按顺序调用，跳过 nil，遇错立即返回。
  - `main.go` 用 `gamequeue.NewChain(gameSvc, learningSvc)` 注册 game worker。

- [ ] **Step 1: 写失败测试**

```go
func TestLearningHandleGameEventAccepted(t *testing.T)
// 插入 accepted 提交，HandleGameEvent(submission.accepted) 后进度完成

func TestLearningHandleGameEventIgnoresOtherTypes(t *testing.T)
// user.login / contest.join / 未知类型不改变进度

func TestChainRunsInOrderAndStopsOnError(t *testing.T)
// 记录调用顺序；第一个返回错误时第二个不被调用

func TestChainSkipsNilHandlers(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/gamequeue/ -run 'TestLearningHandleGameEvent|TestChain' -v`

Expected: FAIL。

- [ ] **Step 3: 实现事件与组合**

- `events.go` 只导入 `gamequeue` 与 `context`；switch 仅处理 accepted。
- `chain.go` 定义 `type chain []Handler` 与 `func NewChain(handlers ...Handler) Handler`；`HandleGameEvent` 顺序执行。
- `main.go`：
  ```go
  gameHandlers := []gamequeue.Handler{gameSvc}
  if learningSvc != nil { gameHandlers = append(gameHandlers, learningSvc) }
  gamequeue.Register(gameMux, gamequeue.NewChain(gameHandlers...))
  ```
- 不改 gamification 的结算逻辑。

- [ ] **Step 4: 运行测试并通过**

Run: `cd apps/server && gofmt -w internal/learning internal/gamequeue cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/gamequeue/ ./internal/gamification/ -v`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/learning apps/server/internal/gamequeue apps/server/cmd/server/main.go
git commit -m "feat(learning): recompute progress on accepted submissions"
```

---

### Task 4: 题目下线/删除守卫

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/learning/repository.go`
- Modify: `apps/server/internal/learning/path_service_test.go`
- Modify: `apps/server/internal/router/router.go`
- Test: `apps/server/internal/problem/service_test.go`、`apps/server/internal/learning/path_service_test.go`

**Interfaces:**
- Consumes: Task 1 的 learning repository。
- Produces:
  - `problem.LearningGuard` 接口：`IsProblemInActivePath(ctx context.Context, problemID int64) (bool, error)`。
  - `problem.WithLearningGuard(g LearningGuard) Option`；`problem.NewService(repo, store, opts ...Option)`（保持现有两参调用兼容）。
  - `learning.Service.IsProblemInActivePath(ctx, problemID) (bool, error)`：活跃路线节点中 `problem_ids_json` 包含该题即 true；下架路线不算。
  - `problem.Service.Update`：仅当 `wasPublic && !p.IsPublic` 时检查；命中返回 `httpx.ErrValidation("题目被学习路线引用，不能下线/删除")`。
  - `problem.Service.Delete`：删除/下线前检查；命中返回同一错误。
  - `router.New` 将 `d.Learning` 注入 problem service（`d.Learning != nil` 时）。

- [ ] **Step 1: 写失败测试**

`problem/service_test.go` 用 fake guard：

```go
func TestProblemUpdateBlocksUnpublishWhenReferenced(t *testing.T)
// wasPublic=true, in.IsPublic=false, blocked=true -> 40001；仓库未变
// in.IsPublic=true 时即使 blocked=true 也允许
// wasPublic=false 时允许

func TestProblemDeleteBlocksWhenReferenced(t *testing.T)
// blocked=true -> 40001，题目仍存在；blocked=false -> 删除/下线正常

func TestProblemGuardErrorPropagates(t *testing.T)
// guard 返回数据库错误时原样返回
```

`learning/path_service_test.go`：

```go
func TestIsProblemInActivePath(t *testing.T)
// 官方/公开/私有活跃路线引用 -> true
// 仅下架路线引用 -> false
// 删除节点后 -> false
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/learning/ -run 'TestProblemUpdateBlocks|TestProblemDeleteBlocks|TestProblemGuard|TestIsProblemInActivePath' -v`

Expected: FAIL。

- [ ] **Step 3: 实现守卫**

- `problem/service.go`：增加 `learningGuard LearningGuard` 字段、`Option`/`WithLearningGuard`、变参构造。`Update` 在 `FindByID` 后记录 `wasPublic`，`apply` 后检查。`Delete` 在调用 repo 前检查。
- `learning/repository.go`：用 SQL `EXISTS` 查询：
  ```sql
  SELECT EXISTS (
    SELECT 1 FROM learning_path_nodes AS n
    JOIN learning_paths AS p ON p.id = n.path_id
    WHERE p.is_active = true
      AND n.problem_ids_json @> jsonb_build_array(?::bigint)
  )
  ```
- `learning.Service.IsProblemInActivePath` 委托 repo。
- `router.go`：`problem.NewService(problemRepo, d.Storage, problem.WithLearningGuard(d.Learning))` 仅在 `d.Learning != nil`；否则用现有两参构造。

- [ ] **Step 4: 运行测试并通过**

Run: `cd apps/server && gofmt -w internal/problem internal/learning internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/learning/ ./internal/router/ -v`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/learning apps/server/internal/router
git commit -m "feat(problem): guard referenced problems from unpublish"
```

---

### Task 5: OpenAPI、文档、全量检查与真实冒烟

**Files:**
- Modify: `packages/api/openapi.yaml`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、评测、前端

**Interfaces:**
- Consumes: Task 1–4。
- Produces: 学习路线进度/事件/守卫契约描述、M4-6 状态文档、真实冒烟记录。

- [ ] **Step 1: OpenAPI 描述**

- 更新 `GET /learning-paths` 与 `GET /learning-paths/:id/tree`：登录用户读取前服务端重算进度；`solvedCount/status/myCompletedCount` 为服务端值；游客 root active/其余 locked。
- 更新题目 `PUT /problems/:id` 与 `DELETE /problems/:id` 的 40001 描述：被活跃学习路线引用时不能下线/删除。
- 不新增 schema；若生成类型无需变化，不运行 `gen:api` 或确认无 diff。

- [ ] **Step 2: 文档**

- `DEVELOPMENT.md` 4.5：补 M4-2B 进度语义、AC 事件、守卫与错误文案。
- `docs/任务清单.md`：M4-6 改为 ✅（进度/守卫完成），M4-7 学习路线前端保持 🚧；更新 M4 进度说明。
- `docs/开发日志.md`：追加 2026-11-07 M4-2B 条目（实现、验证、冒烟）。
- `README.md`：当前进度补 M4-2B。
- `运行手册.md`：补学习路线进度/守卫验证命令或说明。

- [ ] **Step 3: 全量检查**

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

- [ ] **Step 4: 真实 API/浏览器冒烟**

- 用临时用户创建官方/私有路线，根节点 required 2、子节点；普通用户 AC 根节点题目后 `GET tree` 显示 root completed、child active；完成子节点后 completed。
- 已完成节点改题/提高 required 后再次读取，已完成用户不回退；另一未完成用户按新配置重算。
- 游客读取：root active、其余 locked、无进度行。
- 下架路线进度保留；私有路线非作者 404。
- 被活跃路线引用的临时题目：`PUT isPublic=false` 与 `DELETE` 均 40001；删除节点后允许下线/删除。
- 触发一次 `submission.accepted` 事件（真实提交或 API 冒烟），确认 worker 重算；重复事件幂等。
- 清理临时用户/路线/节点/进度/提交/题目；不删除既有数据。
- 浏览器如可用，验证树接口返回结构；前端页面属 M4-7，不在本任务实现。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record learning progress and problem guard"
```

---

## Self-Review

**Spec coverage:**

- 进度计算/解锁 → Task 1 + Task 2。
- 完成锁存/改题重算 → Task 1 + Task 2。
- 游客不写进度 → Task 1 + Task 2。
- AC 事件与读取补算 → Task 3。
- 组合 worker 幂等/重试 → Task 3。
- 题目下线/删除守卫 → Task 4。
- OpenAPI/文档/全量检查/冒烟 → Task 5。

**Type consistency:**

- `EvaluateUser(ctx, userID int64) error` 在 Task 1 定义，Task 2/3 复用。
- `ListProgressForPaths(ctx, userID int64, pathIDs []int64) (map[int64]UserLearningProgress, error)` 在 Task 1 定义，Task 2 复用。
- `LearningGuard`/`WithLearningGuard` 在 Task 4 定义，router 使用。
- `NewChain(handlers ...Handler) Handler` 在 Task 3 定义，main 使用。

**Review Focus mapping:**

1. 解锁链 → Task 1 `TestEvaluateUserUnlockChain` + Task 2 `TestTreeUsesEvaluatedProgress`。
2. 完成锁存 → Task 1 `TestEvaluateUserCompletedLatch` + Task 2 `TestTreeCompletedLatchVisible`。
3. 可见性 → Task 1 `TestEvaluateUserVisibilityAndInactivePath` + Task 2 guest test。
4. 事件幂等 → Task 3 `TestLearningHandleGameEventAccepted` / `TestChain...`。
5. 守卫完整性 → Task 4 problem tests + `TestIsProblemInActivePath`。
6. 读取一致性 → Task 2 `TestListPathsEvaluatesMyCompletedCount` + Task 5 冒烟。
