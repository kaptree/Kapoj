# M2-C 比赛前端与列表增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供可用的比赛前端：比赛列表/详情、倒计时、报名、题目入口、公告、实时榜单、比赛管理与比赛提交入口；并在列表中区分公开比赛与我报名的比赛。

**Architecture:** 后端增加比赛列表 `mine` 过滤和 `registered` 字段；前端新增 `ContestListView`、`ContestDetailView`、`ContestEditView`、`ContestRanklist` 组件与比赛工具函数，提交页接受 `contestId` 查询参数。SSE 复用现有 `/api/submissions/stream`，收到事件后重新拉取榜单/详情。

**Tech Stack:** Go/Gin/GORM、Vue 3、Element Plus、Monaco、SSE。

**Spec:** `DEVELOPMENT.md` 4.4、4.6；`docs/任务清单.md` M2-6。

## Global Constraints

- 比赛管理入口仅对 `contest_admin` / `super_admin` 显示；普通用户只能看到公开比赛和已报名比赛。
- 比赛详情倒计时以服务端 `serverTime` 校准，避免客户端时钟偏差。
- OI 赛时非管理员榜单只显示本人行；前端显示明确提示。
- 比赛提交必须带 `contestId`；提交页显示当前比赛并提供返回比赛链接。
- Markdown 内容统一经 `MarkdownView` 净化；不访问 CDN。
- 页面文案中文；移动端至少可正常滚动和阅读。

## Review Focus

- 未报名/未公开比赛是否从列表与详情泄露。
- 倒计时在开赛/结束边界是否正确刷新状态。
- 报名密码/邀请码、管理员测试与 OI 榜单可见性。
- 比赛编辑的必填字段、题目列表替换与凭证留空语义。
- SSE 重连/卸载与榜单刷新是否泄漏连接。

---

### Task 1: 比赛列表 `mine` 与报名状态

**Files:**
- Modify: `apps/server/internal/contest/query.go`（ListQuery/ListFilter）
- Modify: `apps/server/internal/contest/repository.go`
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/dto.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Test: `apps/server/internal/contest/service_test.go`
- Test: `apps/server/internal/contestapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`

**Interfaces:**
- `ListQuery` 增加 `Mine bool`；`ListFilter` 增加 `ParticipantUserID int64`、`ViewerID int64`。
- `ListRow`/`ListItem` 增加 `Registered bool`（JSON `registered`）。
- Repository：
  - `Mine` 时按 `EXISTS(contest_participants ...)` 过滤，不再要求 `is_public`；
  - 否则保持公开列表规则；
  - 登录用户查询时 SELECT 子查询计算 `registered`。
- Handler `GET /contests?mine=true`；游客 `mine=true` 返回 40001。
- OpenAPI：增加 `mine` 参数；`ContestListItem.required` 加 `registered`。

- [ ] **Step 1: 写失败测试**：公开比赛不显示在 mine；已报名私有比赛显示在 mine；游客 mine 40001；普通列表 `registered` 正确。
- [ ] **Step 2: 运行测试确认失败**。
- [ ] **Step 3: 实现过滤与字段**。
- [ ] **Step 4: 运行测试、生成类型、构建**。
- [ ] **Step 5: 提交** `feat(server): contest mine filter and registered flag`。

### Task 2: 比赛工具、列表页与管理入口

**Files:**
- Create: `apps/web/src/utils/contest.ts`
- Create: `apps/web/src/utils/contest.spec.ts`
- Create: `apps/web/src/views/ContestListView.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/layouts/DefaultLayout.vue`
- Modify: `apps/web/src/api/contests.ts`
- Test: `apps/web/src/utils/contest.spec.ts`

**Interfaces:**
- `utils/contest.ts`：`contestStatusLabel/tagType`、`contestModeLabel`、`registerTypeLabel`、`formatCountdown(targetISO, serverTimeISO, nowMs)`、`contestProblemCell` 等。
- `ContestListView`：全部/我报名的切换（登录后）、关键词/赛制/状态筛选、分页、状态标签、报名方式、时间、题目/报名人数；管理员“新建比赛”、行内“编辑/克隆/删除”。
- 路由 `/contests`，导航去掉 M2 禁用标记；`/contests/new`、`/contests/:id/edit` 需要 contest_admin。
- 列表 API `listContests` 支持 `mine`。

- [ ] **Step 1: 写工具函数测试**（状态/赛制标签、倒计时边界）。
- [ ] **Step 2: 运行测试确认失败**。
- [ ] **Step 3: 实现工具、列表页与路由**。
- [ ] **Step 4: 测试/lint/build**。
- [ ] **Step 5: 提交** `feat(web): contest list`.

### Task 3: 比赛详情、报名、公告、实时榜单

**Files:**
- Create: `apps/web/src/views/ContestDetailView.vue`
- Create: `apps/web/src/components/ContestRanklist.vue`
- Modify: `apps/web/src/views/SubmitView.vue`（读取 `contestId` 并提交）
- Modify: `apps/web/src/views/ProblemDetailView.vue`（可选：显示比赛返回链接不实现）
- Test: `apps/web/src/utils/contest.spec.ts`

**Interfaces:**
- 详情：服务端时间校准倒计时；报名对话框（密码/邀请码）；题目列表（displayId/title/score），开赛后可“提交”跳转 `/problems/:id/submit?contestId=id`；公告列表 + 管理员 CRUD；管理员编辑/克隆/删除/报名用户。
- `ContestRanklist`：props `contestId`；按 mode 展示 solved/penalty 或 totalScore，题目单元格显示 AC 时间/尝试次数/分数；OI `selfOnly` 提示；SSE 事件 300ms 节流刷新并 30 秒兜底；卸载关闭。
- `SubmitView`：`contestId` 查询参数传入 `createSubmission`，页面显示“比赛提交”标签和返回比赛链接；本地草稿 key 包含 contestId 以避免练习/比赛串稿。

- [ ] **Step 1: 补充倒计时/榜单单元格测试**。
- [ ] **Step 2: 运行测试确认失败**。
- [ ] **Step 3: 实现详情、榜单、报名、公告与提交集成**。
- [ ] **Step 4: 真实浏览器验证**：列表→详情→报名→提交→榜单 SSE→OI 提示。
- [ ] **Step 5: 提交** `feat(web): contest detail, registration and ranklist`.

### Task 4: 比赛编辑/克隆/删除与问题配置

**Files:**
- Create: `apps/web/src/views/ContestEditView.vue`
- Modify: `apps/web/src/views/ContestListView.vue`（新建/编辑/克隆/删除入口）
- Modify: `apps/web/src/views/ContestDetailView.vue`（编辑/克隆/删除）
- Test: 通过构建和真实浏览器验证。

**Interfaces:**
- 表单：标题、描述（MarkdownEditor）、开始/结束时间、赛制、isRated/isPublic、registerType、password/inviteCode（编辑留空保留）、题目列表（problemId/displayId/score，添加/删除/上移/下移，最多 26）。
- 创建/更新使用 `ContestInput` 全字段；克隆调用 `cloneContest(id,{})` 后跳转编辑页；删除确认。
- 编辑页加载公开题目作为候选项，已有题目即使不在候选中也保留显示。

- [ ] **Step 1: 实现表单与题目编辑器**。
- [ ] **Step 2: 浏览器验证创建、编辑、克隆、删除**。
- [ ] **Step 3: 测试/lint/build**。
- [ ] **Step 4: 提交** `feat(web): contest admin editor`.

### Task 5: 文档与验收

- 更新 `DEVELOPMENT.md` 4.4 实现状态、`docs/任务清单.md` M2-6 ✅ 与 M2 ✅、`docs/开发日志.md`、`README.md`、`运行手册.md`。
- E2E：创建比赛→列表→详情倒计时→报名→比赛提交→赛时保护→榜单 SSE→公告→编辑/克隆/删除。
- 全量检查：后端 19 包、judge 10 包、前端测试/lint/build、浏览器无控制台错误/无 CDN。

## Self-Review

**Spec coverage:** 4.4 的比赛列表、详情、倒计时、报名、公告、实时榜单、管理与提交入口由 Task 1–5 覆盖；后端榜单/提交已在 M2-B 完成。

**Type consistency:** `mine`/`registered` 从后端 OpenAPI 到前端类型；`contestId` 查询参数在详情、提交页、SubmissionInput 一致；榜单字段与 M2-B DTO 一致。

**Review Focus mapping:** 列表可见性 → Task 1/2；倒计时 → Task 2/3；报名与 OI → Task 3；编辑凭证/题目 → Task 4；SSE 生命周期 → Task 3。
