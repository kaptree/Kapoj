# M2-B 比赛提交、赛时保护与榜单 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 报名用户可以在比赛进行中提交比赛题目；比赛进行中他人只能看到通过与否，看不到代码、耗时、内存与测试点详情；三种赛制的榜单可实时计算。

**Architecture:** 复用 M1 的提交/评测管线，提交记录增加 `contest_id` 并在创建时校验比赛时间、报名状态与题目归属。榜单由 `contest` 包直接读取比赛提交并按 ICPC/IOI/OI 规则在内存中聚合；比赛题目在开赛后不可整体替换，已有提交的比赛不可物理删除。OI 赛时只返回本人行，赛后公布完整榜单。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL；前端类型由 OpenAPI 生成。前端比赛页面在 M2-C。

**Spec:** `DEVELOPMENT.md` 3.3、3.4、4.4、4.6；`docs/任务清单.md` M2-3、M2-4。

## Global Constraints

- 比赛提交必须：比赛存在、当前时间在 `[start,end)`、提交者是报名用户（比赛管理员除外）、题目属于该比赛。
- 比赛进行中，非本人/非管理员查看他人比赛提交时：不返回代码、错误信息、测试点详情、耗时、内存、得分；保留状态（通过与否）与语言。
- 比赛结束后恢复完整详情；练习提交始终完整可见。
- ICPC：通过题数降序、总罚时升序；单题罚时 = 首次 AC 相对开赛分钟数 + 首次 AC 之前的未通过提交数 × 20；未通过题不计罚时。
- IOI：每题取最高分，总分降序；OI 计分同上，但赛时非管理员只看到自己的行，赛后公布完整榜单。
- 榜单包含所有报名用户（含 0 提交）；同分并列名次（1,2,2,4）。
- 比赛开始后不允许整体替换题目列表；已有比赛提交时禁止删除比赛。
- 日志不得包含代码、测试数据或比赛凭证。

## Review Focus

- 比赛提交绕过：未报名、赛前/赛后、题目不属于比赛、隐藏题非参赛者。
- 赛时保护是否同时覆盖列表、详情与耗时/内存/得分字段。
- ICPC 罚时、IOI/OI 最高分、并列名次与 0 提交用户。
- OI 赛时是否只返回本人行，赛后是否恢复完整榜单。
- 开赛后题目列表变更、已有提交比赛删除的数据一致性。

---

### Task 1: 比赛提交、访问控制与赛时保护

**Files:**
- Modify: `apps/server/internal/submission/validate.go`
- Modify: `apps/server/internal/submission/model.go`
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/service_test.go`
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Test: `apps/server/internal/submission/service_test.go`
- Test: `apps/server/internal/submission/records_test.go`

**Interfaces:**
- `submission.Input` 增加 `ContestID int64`（0 表示练习）。
- `submission.ContestLookup`：
  ```go
  FindByID(ctx, id) (*contest.Contest, error)
  IsParticipant(ctx, contestID, userID) (bool, error)
  FindProblem(ctx, contestID, problemID) (*contest.ContestProblem, error)
  ```
- `submission.NewService(repo, problems, contests, queue, limiter, publisher)`。
- `contest.Repository` 增加：
  ```go
  FindProblem(ctx, contestID, problemID int64) (*ContestProblem, error)
  HasSubmissions(ctx, contestID int64) (bool, error)
  CanViewProblem(ctx, userID, problemID int64, now time.Time) (bool, error)
  ```
- `contest.ErrHasSubmissions`；`Delete` 在存在 `submissions.contest_id` 时返回该错误，服务映射 40001。
- `contest.Service.Update`：若 `now >= existing.StartTime` 且题目集合变化，返回 40001「比赛开始后不能修改题目列表」。
- `problem.Viewer` 增加 `CanViewHidden bool`；`Service.Get` 对隐藏题允许 `CanViewHidden`。
- `problemapi.RegisterRoutes` 增加 `ContestAccess` 接口；详情对隐藏题查询 `CanViewProblem(ctx, userID, problemID, now)`。
- `submission.ListRow` 增加 `ContestStartTime/ContestEndTime *time.Time`（LEFT JOIN contests）。
- 保护判定：`contestID != nil && !v.CanManage && v.UserID != row.UserID && start <= now < end`。
  - `ListItem`：保护时 `TimeUsedMS/MemoryUsedKB/Score = 0`。
  - `Detail`：保护时 `Code=""`、`CodeHidden=true`、`ErrorMessage=""`、`JudgeResult=[]`、`TimeUsedMS/MemoryUsedKB/Score=0`。
- 创建校验顺序：比赛存在 404 → 时间 40001 → 报名 40001（管理员跳过）→ 题目归属 40001 → 原题目可见性/交互/checker/测试点校验。
- `submissions.contest_id` 写入 `*int64`；`ToCreated` 可不变。

- [ ] **Step 1: 写失败测试**

```go
func TestContestSubmissionRules(t *testing.T) {
    // 赛前/赛后、未报名、非比赛题目 -> 40001；报名后进行中 -> 201 且 contest_id 写入。
}

func TestContestSubmissionProtection(t *testing.T) {
    // 进行中：他人列表/详情无 code/time/memory/score/judgeResult；
    // 本人/管理员/赛后恢复。
}

func TestContestDeletionAndProblemChangeGuards(t *testing.T) {
    // 有提交时删除比赛 40001；开赛后改题目列表 40001；赛前可改。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submission/ ./internal/contest/ ./internal/problemapi/`
Expected: FAIL。

- [ ] **Step 3: 实现校验、保护与守卫**

按 Interfaces 实现。提交列表/详情查询增加 `LEFT JOIN contests`；测试用真实比赛/报名/提交数据。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/submission apps/server/internal/submissionapi apps/server/internal/contest apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/router
git commit -m "feat(server): contest submissions and protection"
```

---

### Task 2: ICPC / IOI / OI 榜单

**Files:**
- Create: `apps/server/internal/contest/ranklist.go`
- Create: `apps/server/internal/contest/ranklist_test.go`
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/dto.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/contest.ts`（或 `contests.ts`）
- Modify: `apps/web/src/api/types.ts`

**Interfaces:**
- 仓储：
  ```go
  type RankSubmission struct {
      UserID, ProblemID int64
      Status string
      Score int
      CreatedAt time.Time
  }
  func (r *gormRepository) RankSubmissions(ctx, contestID int64) ([]RankSubmission, error)
  ```
- DTO：
  ```go
  type RankProblem struct {
      ProblemID int64 `json:"problemId"`
      DisplayID string `json:"displayId"`
      Title string `json:"title"`
  }
  type RankCell struct {
      Status string `json:"status,omitempty"`
      Attempts int `json:"attempts"`
      AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
      Score int `json:"score"`
  }
  type RankRow struct {
      Rank int `json:"rank"`
      UserID int64 `json:"userId"`
      Username, Nickname string
      Solved int `json:"solved"`
      Penalty int `json:"penalty"`
      TotalScore int `json:"totalScore"`
      LastAcceptedAt *time.Time `json:"lastAcceptedAt,omitempty"`
      Problems []RankCell `json:"problems"`
  }
  type Ranklist struct {
      ContestID int64; Mode Mode; Status Status; GeneratedAt time.Time
      Problems []RankProblem; Rows []RankRow; SelfOnly bool
  }
  func (s *Service) Ranklist(ctx, v Viewer, id int64) (*Ranklist, error)
  ```
- 访问：复用 `findAccessible`。OI 且 `running` 且 `!v.CanManage`：`Rows` 只保留 `UserID==v.UserID`，`SelfOnly=true`。
- ICPC：只统计终态提交；首次 AC 前的非 AC 次数为 attempts；`penalty = floor(minutes since start) + 20*attempts`；按 solved desc → penalty asc → lastAcceptedAt asc → userID。
- IOI/OI：每题取最高 `score`；`totalScore` 为各题最高分之和；按 totalScore desc → last submission asc → userID；`Solved` 为有 AC 的题数。
- 名次：与上一行关键指标相同则同名次（competition ranking）。
- `GET /contests/:id/ranklist` 公开（可选登录）；OpenAPI 增加 `ContestRanklistResponse`。
- 前端 `contests.ts` 增加 `getContestRanklist(id)` 与类型。

- [ ] **Step 1: 写失败测试**

```go
func TestICPCRanklist(t *testing.T) {
    // A: 1 AC + 1 WA before -> solved 1, penalty 20+20=40；
    // B: 2 AC, 不同罚时；C: 0 提交 -> 包含且 solved 0；并列名次。
}

func TestIOIRanklist(t *testing.T) {
    // 每题取最高分，总分排序；0 提交用户包含。
}

func TestOIRanklistVisibility(t *testing.T) {
    // 赛时非管理员只返回本人行且 selfOnly=true；
    // 管理员/赛后返回完整榜单。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='...' go test -count=1 -p 1 ./internal/contest/ ./internal/contestapi/`
Expected: FAIL。

- [ ] **Step 3: 实现榜单计算与接口**

按 Interfaces 实现。榜单计算在 Go 内存中完成；不要用 N+1 查询。

- [ ] **Step 4: 运行测试确认通过**

Run: 同 Step 2，Expected PASS。
Run: `corepack pnpm gen:api && corepack pnpm build:web`。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/contest apps/server/internal/contestapi packages/api/openapi.yaml apps/web/src/api
git commit -m "feat(server): contest ranklists"
```

---

### Task 3: 文档与端到端验收

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`

**验收（真实 server + judge + PostgreSQL + Redis + Docker）：**
1. 创建比赛、报名、赛前提交被拒、开赛后提交成功、赛后提交被拒。
2. 进行中他人列表/详情无代码/耗时/内存/得分/测试点；本人/管理员/赛后可见。
3. ICPC 榜单罚时、IOI 最高分、OI 赛时仅本人、赛后完整、并列名次、0 提交用户。
4. 有提交删除比赛 40001；开赛后改题目 40001。
5. 全量测试、lint、构建通过。

- [ ] **Step 1: 运行 E2E 并记录结果**
- [ ] **Step 2: 更新文档**（3.3 提交差异、3.4 榜单/删除策略、4.4 实现状态、4.6 保护、任务清单 M2-3/M2-4、开发日志、README/运行手册）
- [ ] **Step 3: 全量检查**
- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: m2-b contest judging and ranklists"
```

---

## Self-Review

**Spec coverage:** 4.4 的比赛提交、三种榜单、赛时保护由 Task 1–2 覆盖；前端页面明确留给 M2-C。3.4 的提交 contest_id 与删除策略在 Task 1 落地。

**Type consistency:** 榜单状态沿用 M1 状态字符串；`contestId` 在提交输入、模型与 OpenAPI 中一致；OI `selfOnly` 由后端返回。

**Review Focus mapping:** 提交绕过 → Task 1；保护字段 → Task 1；榜单规则/并列/OI → Task 2；开赛后变更 → Task 1；文档/验收 → Task 3。
