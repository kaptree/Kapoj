# Phase C：评测记录、AC 金币与提交统计 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付评测记录重测/高亮/测试点分数、首次 AC 3 金币，以及提交统计图表和可排序的他人指标。

**Architecture:** 先扩展 judge 落库的逐测试点分数与重测所需计数幂等字段，再实现管理员重测 API；随后在游戏化 AC 事件中按“用户+题目首次 AC”发 3 金币；最后增加 `code_length`、全局代码可见性（本人同题 AC 例外）和统计接口/页面。

**Tech Stack:** Go + Gin + GORM + PostgreSQL、独立 judge（pgx）、Vue 3 + PrimeVue 4、Vitest。

**Spec:** [`docs/持续开发需求.md`](../../持续开发需求.md)

## Global Constraints

- 重测仅拥有 `submission.rejudge` 的管理员可操作；普通用户不可重测。
- 重测不改变代码/语言/题目/用户/比赛；`submit_count` 不变；`accept_count` 不重复计数。
- AC 金币：同一用户同一题首次 AC 发 3 金币；不追溯历史 AC；重测把 AC 改 WA 不扣回；所有题型/比赛适用。
- 代码可见性：全站默认隐藏他人代码；查看者同题有本人 AC 后可看；比赛进行中赛时保护优先；统计页永远不展示代码。
- 统计页只展示指标，不返回提交 ID，不提供代码入口。
- 所有新增接口更新 OpenAPI 并生成前端类型；每个任务先写失败测试。

## Review Focus

- 重测 AC 提交时 `accept_count` 不能 +2；重测 WA→AC 要正确 +1；重测不能重复发 AC 金币。
- 历史 `judge_result_json` 没有 score 时不能伪造为 0；前端显示“—”。
- 代码可见性必须由后端计算，不能只靠前端隐藏；比赛进行中不能因本人 AC 放宽赛时保护。
- 统计排序只接受白名单字段，不能拼接任意 SQL。
- 统计页不返回 code/提交 ID，避免从页面绕到代码详情。

---

### Task 1: judge 逐测试点分数与重测计数幂等

**Files:**
- Modify: `apps/judge/internal/model/model.go`
- Modify: `apps/judge/internal/db/repository.go`
- Modify: `apps/judge/internal/db/repository_test.go`
- Modify: `apps/server/internal/submission/model.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/records_test.go`
- Modify: `apps/server/internal/db/db.go`（AutoMigrate 字段）

**Interfaces:**
- `JudgeCaseResult.score` 落库；服务端 DTO `Score *int` 区分历史缺失。
- `submissions.accepted_counted bool`；`submissions.code_length int`（R07 同迁移）。
- judge `Finish` 只在新 AC 且 `accepted_counted=false` 时递增 `problems.accept_count`，并置 true。

- [x] **Step 1: 写失败测试**

judge 测试：AC 提交的 `judge_result_json` 包含每个 case 的 `score`；重测 WA→AC 只 +1；重测 AC→AC 不重复 +1。
服务端测试：`GET /submissions/:id` 返回 `judgeResult[].score`；历史缺失时省略/为空。

- [x] **Step 2: 实现 judge 落库与计数**

- `model.JudgeCaseResult.Score` 改为 `json:"score,omitempty"`。
- `Finish` 在事务内读取旧 `accepted_counted`，仅当新状态 accepted 且旧值为 false 时递增并置 true；重测重置时由服务端把 `accepted_counted=false` 并回退旧 AC 计数。

- [x] **Step 3: 实现服务端 DTO/迁移**

- `submission.JudgeCaseResult` 增加 `Score *int json:"score,omitempty"`。
- `submissions` 增加 `accepted_counted`、`code_length`。
- `ToDetail` 原样保留。

- [x] **Step 4: 运行测试**

Run:
```bash
cd apps/judge && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/db ./internal/pipeline -count=1
cd ../server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/submission -count=1
```
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add apps/judge/internal apps/server/internal/submission apps/server/internal/db/db.go
git commit -m "feat: persist per-testcase scores and accepted count idempotency"
```

---

### Task 2: 管理员重测 API 与前端详情增强

**Files:**
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submission/service_test.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`（如需）
- Modify: `packages/api/openapi.yaml`
- Create: `apps/web/src/components/CodeViewer.vue`
- Create: `apps/web/src/components/CodeViewer.spec.ts`
- Modify: `apps/web/src/views/SubmissionsView.vue`
- Modify: `apps/web/src/views/SubmissionsView.spec.ts`
- Modify: `apps/web/src/api/submissions.ts`

**Interfaces:**
- `POST /api/submissions/:id/rejudge`，权限 `submission.rejudge`（新增 RBAC 权限，管理员默认授予）。
- 前端 `rejudgeSubmission(id)`；`CodeViewer` 按语言高亮、行号、复制。

- [x] **Step 1: 写失败测试**

- 普通用户/无权限管理员 403；有权限管理员 200。
- pending/judging 返回 40001；30 秒内重复重测返回 40001。
- 重测重置状态/结果并重新入队；AC 旧提交重测前正确回退 accept_count；金币不重复。
- 前端：重测按钮仅权限可见；题目标题链接；测试点分数列/总分；代码高亮；codeHidden 不渲染。

- [x] **Step 2: 实现后端权限与服务**

- 新增 RBAC `submission.rejudge`，授予 `content_admin`/`contest_admin`（super 绕过）。
- 服务方法 `Rejudge(ctx, viewer, id)`：校验权限、终态、30 秒频控；事务回退旧 AC 计数、清空结果、置 pending；入队失败恢复/标记 system_error。
- `POST /submissions/:id/rejudge` 返回 `{id,status}`。

- [x] **Step 3: 实现前端**

- `CodeViewer.vue` 使用本地 highlight.js，语言映射；行号/复制。
- 详情抽屉：题目 RouterLink、重测按钮/确认、分数列/总分、CodeViewer。
- 失败 Toast；重测后依赖 SSE。

- [x] **Step 4: 运行测试**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/submission ./internal/submissionapi ./internal/rbac -count=1
corepack pnpm --filter web test src/components/CodeViewer.spec.ts src/views/SubmissionsView.spec.ts
corepack pnpm gen:api
```
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add apps/server/internal/submission apps/server/internal/submissionapi apps/server/internal/rbac packages/api/openapi.yaml \
  apps/web/src/api apps/web/src/components/CodeViewer.vue apps/web/src/components/CodeViewer.spec.ts \
  apps/web/src/views/SubmissionsView.vue apps/web/src/views/SubmissionsView.spec.ts
git commit -m "feat: add admin rejudge and richer submission details"
```

---

### Task 3: 首次 AC 奖励 3 金币

**Files:**
- Modify: `apps/server/internal/gamification/events.go`
- Modify: `apps/server/internal/gamification/events_test.go`
- Modify: `apps/server/internal/gamification/coin_repository.go`（如需）
- Modify: `apps/web/src/utils/gamification.ts`
- Modify: `apps/web/src/utils/gamification.spec.ts`

**Interfaces:**
- `AcceptedSubmission` 增加 `ProblemID`、`CreatedAt`。
- `CreditCoins(userID, 3, "problem_accepted", "problem", problemID)`。

- [x] **Step 1: 写失败测试**

- 首次 AC +3、流水 reason；同题二次 AC 不加；并发只一条；重测 AC 不重复；历史 AC 不追溯。
- 前端流水文案“通过题目奖励”。

- [x] **Step 2: 实现**

- `FindAcceptedSubmission` 读取 problem_id/created_at。
- 事件处理先判断是否更早的 AC，再 `CreditCoins`，再 `EvaluateUser`；唯一约束兜底。

- [x] **Step 3: 运行测试**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/gamification -count=1`
Expected: PASS。

- [x] **Step 4: 提交**

```bash
git add apps/server/internal/gamification apps/web/src/utils/gamification.ts apps/web/src/utils/gamification.spec.ts
git commit -m "feat: award three coins on first accepted problem"
```

---

### Task 4: 全站代码可见性与 code_length

**Files:**
- Modify: `apps/server/internal/submission/model.go`
- Modify: `apps/server/internal/submission/repository.go`
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/dto.go`
- Modify: `apps/server/internal/submission/records_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/web/src/views/SubmissionsView.vue`（codeHidden 文案）
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `code_length` 在创建提交时写入，历史回填 `octet_length(code)+octet_length(answer)`。
- `GET /submissions/:id` 的 `codeHidden`：非本人/非管理员/非 live viewer 且查看者在该题无 AC 时 true；仅隐藏代码，不隐藏测试点指标。
- 比赛进行中继续完整赛时保护。

- [x] **Step 1: 写失败测试**

- 练习提交：他人无 AC 看不到代码；他人有 AC 可看；本人/管理员可看。
- 比赛进行中即使有 AC 仍看不到他人代码/指标。
- `code_length` 新提交写入、历史回填。

- [x] **Step 2: 实现**

- `Submission` 增加 `CodeLength`。
- 创建时计算 `len(code)`/`len(answer)`。
- `Detail` 用 repository 查询查看者同题 AC，设置 `CodeHidden`；不要复用赛时保护的完整隐藏分支。
- 历史回填迁移。

- [x] **Step 3: 运行测试**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/submission ./internal/submissionapi -count=1`
Expected: PASS。

- [x] **Step 4: 提交**

```bash
git add apps/server/internal/submission apps/server/internal/db/db.go packages/api/openapi.yaml apps/web/src/views/SubmissionsView.vue
git commit -m "feat: gate others code behind same-problem acceptance"
```

---

### Task 5: 提交统计接口与图表页面

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/repository.go`
- Modify: `apps/server/internal/problem/dto.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/web/src/api/problems.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/views/ProblemStatisticsView.vue`
- Modify: `apps/web/src/views/ProblemStatisticsView.spec.ts`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `GET /problems/:id/statistics` 增加 `statusCounts: [{status,count,percentage}]`。
- `GET /problems/:id/submission-stats?sort=time|memory|code_length|created_at&order=asc|desc&page=&page_size=`；返回指标，不含代码/提交 ID。
- 前端饼图/柱状图（本地 SVG/CSS）+ 可排序明细表。

- [x] **Step 1: 写失败测试**

- 状态分布统计正确；排序白名单/分页；比赛进行中排除受保护提交；响应不含 code/codeHidden/id。
- 前端图表/排序/分页/空态/移动端。

- [x] **Step 2: 实现后端**

- `Stats` 增加 `StatusCounts`。
- 新查询聚合 `status` 计数；明细查询按白名单排序，返回 userId/nickname/language/status/time/memory/codeLength/createdAt。
- 隐藏题/赛时保护沿用现有可见性规则。

- [x] **Step 3: 实现前端**

- 扩展统计页：摘要卡 + 饼图 + 柱状图 + 图例 + 明细表 + 排序/分页；不显示代码列/入口。
- 使用本地 SVG/CSS，不新增 CDN。

- [x] **Step 4: 运行测试**

Run:
```bash
cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test ./internal/problem ./internal/problemapi -count=1
corepack pnpm --filter web test src/views/ProblemStatisticsView.spec.ts
corepack pnpm --filter web build
```
Expected: PASS。

- [x] **Step 5: 提交**

```bash
git add apps/server/internal/problem apps/server/internal/problemapi packages/api/openapi.yaml \
  apps/web/src/api/problems.ts apps/web/src/api/types.ts apps/web/src/api/schema.d.ts \
  apps/web/src/views/ProblemStatisticsView.vue apps/web/src/views/ProblemStatisticsView.spec.ts
git commit -m "feat: add submission status charts and sortable metrics"
```

---

## Self-Review

- **Spec coverage:** R01 由 Task 1–2 覆盖；R06 由 Task 3 覆盖；R07 由 Task 4–5 覆盖。
- **重测/AC 计数/金币幂等** 在 Task 1、2、3 之间交叉测试。
- **代码可见性** 在 Task 4 同时覆盖练习 AC 例外与赛时保护。
- **Review Focus** 的五个风险均有对应测试。
- **Proportion:** 计划覆盖 3 个需求，按独立可验收交付物拆分；实现细节留给执行时结合现有模式补齐。
