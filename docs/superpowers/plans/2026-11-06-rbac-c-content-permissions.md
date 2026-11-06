# RBAC-C 内容权限迁移 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把题目、比赛、测评、游戏化、学习路线的现有管理授权从 legacy role 迁移到 RBAC 权限码，并补内容变更审计与金币调整审计。

**Architecture:** 新增 `LoadAccess` 为需要“权限影响可见性/管理语义”的接口加载实时 Access；管理写接口全部使用 `RequirePermission`；各领域 Viewer 拆成具体能力字段；内容写操作通过 `AuditAdminAction` best-effort 记录，金币调整在业务事务内审计。`users.role`/JWT role 在 RBAC-C 完成后不再参与任何授权。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、testify、Vue 3 + PrimeVue（仅更新生成类型）。

**Spec:** `docs/superpowers/specs/2026-11-06-admin-rbac-design.md` 5–11、14。

## Global Constraints

- 权限码稳定，格式 `domain.action`；本阶段不新增权限码。
- 管理写接口只使用 `RequirePermission`；`super_admin` 仍绕过但写审计。
- `users.role`/JWT role 只保留展示兼容；RBAC-C 之后任何管理授权不得读取 `claims.Role`。
- 题目隐藏读取用 `problem.view_hidden`；比赛隐藏读取用 `contest.view_hidden`；赛时提交查看用 `submission.view_live_contest`。
- 修改题目 `isPublic` 需要 `problem.update` + `problem.publish`；仅 `problem.update` 不能发布/下线。
- 游戏化每个子域独立权限：成就、任务、徽章、称号、配置、金币。
- 学习路线：`learning_path.official.manage` 允许管理官方路线及学习路线管理操作；`learning_path.admin.read` 只读查看全部/下架路线。普通用户自定义路线仍按作者规则。
- 内容管理审计 best-effort，失败只记日志，不影响业务；敏感的用户/角色/金币调整审计仍在事务内，失败回滚。
- 审计不记录密码、token、代码、测试数据、请求体；内容审计只记录 method/path/status/target。
- 现有 `problem_admin` / `contest_admin` / `super_admin` 账号经 Seed 映射后行为保持不变。
- 所有用户可见文案中文；不推送、不合并分支。

## Review Focus

1. **旧角色不再授权**：任何管理接口传 legacy `problem_admin`/`contest_admin`/`super_admin` 的 JWT 但没有 RBAC 角色时，应当 403；测试放各领域任务的权限反例。
2. **隐藏资源不泄漏**：无 `problem.view_hidden`/`contest.view_hidden` 时，列表、详情、统计、榜单、提交列表都不能暴露隐藏数据；测试放题目、比赛、测评任务。
3. **发布权限边界**：仅 `problem.update` 的账号改标题成功，改 `isPublic` 返回 403；同时有 `problem.publish` 才允许；测试放题目任务。
4. **审计完整性与失败隔离**：内容审计失败不影响操作；金币调整审计失败回滚；测试放基础设施/游戏化任务。
5. **各域权限独立**：游戏化五个权限、学习路线 manage/read 不能互相越权；测试放对应任务。

---

### Task 1: Access 加载、权限复用与审计基础设施

**Files:**
- Create: `apps/server/internal/middleware/access.go`
- Create: `apps/server/internal/middleware/audit.go`
- Create: `apps/server/internal/middleware/access_test.go`
- Create: `apps/server/internal/middleware/audit_test.go`
- Modify: `apps/server/internal/middleware/permission.go`
- Modify: `apps/server/internal/rbac/access.go`
- Modify: `apps/server/internal/rbac/audit.go`
- Create: `apps/server/internal/testutil/rbac.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: `rbac.Service.Access`、`rbac.Access`、`rbac.AuditMeta`、`middleware.CurrentClaims`、`middleware.CurrentAccess`。
- Produces:
  - `(*rbac.Access).Has(code string) bool`：super 永远 true；否则查权限集合；nil 返回 false。
  - `rbac.WriteAuditTx(tx *gorm.DB, actor AuditMeta, action, targetType string, targetID int64, summary string) error`：导出的事务内审计写入。
  - `(*rbac.Service).RecordAudit(ctx context.Context, actor AuditMeta, action, targetType string, targetID int64, summary string) error`：独立事务 best-effort 审计写入。
  - `middleware.LoadAccess(resolver PermissionResolver) gin.HandlerFunc`：有 claims 时解析 Access 并放入 context；游客跳过；解析失败不拒绝请求。
  - `middleware.HasPermission(c *gin.Context, code string) bool`：从 `CurrentAccess` 判断。
  - `middleware.AuditAdminAction(s *rbac.Service, action, targetType string) gin.HandlerFunc`：响应 2xx 且 actor 有 action 权限（或 super）时 best-effort 写审计；失败只 `slog.Warn`。
  - `testutil.SeedRBAC(t *testing.T, g *gorm.DB) *rbac.Service`：执行 `rbac.Seed` 并返回服务。
  - `testutil.AssignRole(t *testing.T, g *gorm.DB, userID int64, code string)`：按角色 code 关联用户。
  - `testutil.AssignLegacyRole(t *testing.T, g *gorm.DB, userID int64, role user.Role)`：映射 `super_admin→super_admin`、`problem_admin→content_admin`、`contest_admin→contest_admin`、`user→无`。

- [ ] **Step 1: 写失败测试**

```go
func TestAccessHasSuperAndPermission(t *testing.T)
// super + empty permissions -> Has(any)=true
// normal + single permission -> true for that code, false for another
// nil Access -> false

func TestLoadAccessStoresAuthenticatedAccess(t *testing.T)
// guest -> resolver not called, no CurrentAccess
// authenticated -> resolver called once, CurrentAccess present
// resolver ErrInactive/generic -> no access, request continues

func TestRequirePermissionReusesLoadedAccess(t *testing.T)
// LoadAccess + RequirePermission -> resolver calls == 1
// no LoadAccess -> resolver called as before

func TestAuditAdminActionWritesOnSuccess(t *testing.T)
// POST 2xx, actor has permission -> admin_audit_logs row with action/target/IP/UA
// actor without permission -> no row
// 4xx -> no row
// admin_audit_logs dropped -> response remains 2xx and log warning

func TestRecordAuditUsesTransaction(t *testing.T)
// RecordAudit inserts a row; targetID=0 -> NULL

func TestSeedRBACAndAssignLegacyRole(t *testing.T)
// SeedRBAC -> roles present; AssignLegacyRole maps problem_admin to content_admin
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/middleware/ ./internal/rbac/ ./internal/testutil/ -run 'TestAccessHas|TestLoadAccess|TestRequirePermissionReuses|TestAuditAdminAction|TestRecordAudit|TestSeedRBAC' -v`
Expected: FAIL（新接口不存在）。

- [ ] **Step 3: 实现基础设施**

- `Access.Has` 先判断 nil/super，再查 `Permissions`。
- `LoadAccess` 在无 claims 时直接 `c.Next()`；解析成功把 Access 放入 context（使用独立 key，由 `CurrentAccess` 读取）；失败只记录 unexpected error 日志，继续。
- `RequirePermission` 优先复用 `CurrentAccess`；只有 context 无 access 时才调用 resolver。resolver nil 时返回 403，不 panic。
- `AuditAdminAction` 在 `c.Next()` 后检查 `c.Writer.Status()` 是否 2xx；从 `CurrentAccess` 检查 action 权限；targetID 从路径 `id` 解析，非法为 0；摘要 `METHOD path -> status`；调用 `RecordAudit`，错误只日志。
- `testutil.AssignRole` 先按 code 查 roles.id；不存在时 `t.Fatalf`；插入 `user_roles` 使用 `ON CONFLICT DO NOTHING`。
- `testutil.SeedRBAC` 用 `context.Background()` 调 `rbac.Seed`。

- [ ] **Step 4: 运行测试、全量检查、提交**

Run: `cd apps/server && gofmt -w internal/middleware internal/rbac internal/testutil && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/middleware/ ./internal/rbac/ ./internal/testutil/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/middleware apps/server/internal/rbac apps/server/internal/testutil
git commit -m "feat(server): add rbac access and audit infrastructure"
```

---

### Task 2: 题库权限、发布保护与审计

**Files:**
- Modify: `apps/server/internal/problem/service.go`
- Modify: `apps/server/internal/problem/service_test.go`
- Modify: `apps/server/internal/problemapi/handler.go`
- Modify: `apps/server/internal/problemapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 的 `LoadAccess`/`HasPermission`/`AuditAdminAction`/`testutil.SeedRBAC`。
- Produces:
  - `problem.Viewer{CanViewHidden bool; CanViewSolution bool}`。
  - `problemapi.Options{RBAC *rbac.Service}`；`RegisterRoutes(api, problemJSON, uploads, s, accepted, contests, opts ...Options)`。
  - 权限接线：
    - `POST /problems` → `problem.create`
    - `PUT /problems/:id` → `problem.update`；修改 `isPublic` 额外要求 `problem.publish`
    - `DELETE /problems/:id` → `problem.delete`
    - 测试数据 GET/PUT/reorder/DELETE/upload → `problem.testcase.manage`
    - checker upload → `problem.checker.manage`
    - `GET /problems`、`GET /problems/:id`、`GET /problems/:id/statistics` 挂 `LoadAccess`
  - 内容审计 action 使用对应权限码，target type `problem`。

- [ ] **Step 1: 写失败测试**

```go
func TestProblemListAndDetailRespectViewHiddenPermission(t *testing.T)
// 无 view_hidden：隐藏题 404，includeHidden 不返回隐藏题
// 有 view_hidden：可看隐藏题与 includeHidden
// contest participant fallback 仍只放开题目内容，不放开题解

func TestProblemSolutionRequiresManagementCapability(t *testing.T)
// view_hidden-only -> 无 solutionMd/solutionCode
// content_admin -> 有 solutionMd/solutionCode

func TestProblemRoutesRequirePermissions(t *testing.T)
// legacy problem_admin token 但无 RBAC 角色 -> create/update/delete/testcase/checker 403
// content_admin -> 200/201
// view_hidden-only -> create/update/testcase 403，但 GET 隐藏题 200

func TestProblemPublishRequiresExtraPermission(t *testing.T)
// update-only 角色：改 title 成功；改 isPublic=true/false 返回 403
// update+publish 角色：改 isPublic 成功
// 已公开题改为隐藏、隐藏题改为公开都覆盖

func TestProblemWritesAudit(t *testing.T)
// create/update/delete/testcase/checker 成功 -> 对应 action 审计
// 失败请求不写审计
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ -run 'TestProblemListAndDetail|TestProblemSolution|TestProblemRoutes|TestProblemPublish|TestProblemWritesAudit' -v`
Expected: FAIL。

- [ ] **Step 3: 实现题库权限**

- `problem.Service.Get` 用 `v.CanViewHidden`；`List` 的 `IncludeHidden` 用 `v.CanViewHidden`；`Statistics` 沿用 `Get`。
- `problemapi.viewer(c)` 从 `CurrentAccess` 取：
  - `CanViewHidden = access.Has(PermProblemViewHidden)`
  - `CanViewSolution = access.Has(PermProblemViewHidden) || access.Has(PermProblemUpdate) || access.Has(PermProblemDelete) || access.Has(PermProblemPublish) || access.Has(PermProblemTestcaseManage) || access.Has(PermProblemCheckerManage)`
- `get` 的 contest fallback 仍临时设 `v.CanViewHidden = true`，但不要设置 `CanViewSolution`；`includeSolution := v.CanViewSolution`。
- `update` 先用 `h.svc.Get(ctx, problem.Viewer{CanViewHidden: true}, id)` 读取当前 `IsPublic`；若 `in.IsPublic != existing.IsPublic && !middleware.HasPermission(c, string(rbac.PermProblemPublish))`，返回 `httpx.ErrForbidden`。
- `problemapi` 所有测试 fixture 改为 `testutil.SeedRBAC` + `testutil.AssignLegacyRole`；`token("problem_admin")` 同时写 legacy role 和 `content_admin`；另加不分配 RBAC 角色的 legacy token helper，用于证明旧 role 不再授权。
- Router 传 `problemapi.Options{RBAC: d.RBAC}`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/problem internal/problemapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/problem/ ./internal/problemapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/problem apps/server/internal/problemapi apps/server/internal/router
git commit -m "feat(server): migrate problem permissions"
```

---

### Task 3: 比赛权限与审计

**Files:**
- Modify: `apps/server/internal/contest/service.go`
- Modify: `apps/server/internal/contest/ranklist.go`
- Modify: `apps/server/internal/contest/dto.go`（如 `CanManage` 派生）
- Modify: `apps/server/internal/contest/service_test.go`
- Modify: `apps/server/internal/contestapi/handler.go`
- Modify: `apps/server/internal/contestapi/handler_test.go`
- Modify: `apps/server/internal/rating/service.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 基础设施、Task 2 的 fixture 模式。
- Produces:
  - `contest.Viewer{UserID int64; CanViewHidden, CanCreate, CanUpdate, CanDelete, CanReadParticipants, CanManageAnnouncements bool}`。
  - `func (v Viewer) CanManageAny() bool`：任一管理能力为 true。
  - `contestapi.Options{RBAC *rbac.Service}`（已有 Options，增加字段）。
  - 权限接线：
    - `POST /contests`、`POST /contests/:id/clone` → `contest.create`
    - `PUT /contests/:id` → `contest.update`
    - `DELETE /contests/:id` → `contest.delete`
    - `GET /contests/:id/participants` → `contest.participant.read`
    - 公告 POST/PUT/DELETE → `contest.announcement.manage`
    - `GET /contests`、`GET /contests/:id`、`GET /contests/:id/ranklist`、`GET /contests/:id/announcements` 挂 `LoadAccess`
  - 内容审计 action 使用对应权限码，target type `contest`。

- [ ] **Step 1: 写失败测试**

```go
func TestContestVisibilityUsesViewHidden(t *testing.T)
// 无 view_hidden：非公开比赛 404，includeHidden 不返回
// 有 view_hidden：可看非公开、开赛前题目、榜单
// 已报名用户仍可看自己报名的非公开比赛

func TestContestRoutesRequirePermissions(t *testing.T)
// legacy contest_admin token 但无 RBAC 角色 -> create/update/delete/participants/announcement 403
// contest_admin 角色 -> 成功
// view_hidden-only -> 可读隐藏比赛，但不能 create/update/delete

func TestContestGranularPermissions(t *testing.T)
// 只有 contest.update：可更新，不能 create/delete/participants/announcement
// 只有 contest.participant.read：可看报名，不能更新
// 只有 contest.announcement.manage：可管理公告，不能更新比赛
// 只有 contest.create：可创建/克隆，不能更新

func TestContestCanManageDerivation(t *testing.T)
// 详情 canManage 对任一管理权限为 true；纯读/参赛者为 false

func TestContestWritesAudit(t *testing.T)
// create/update/delete/announcement 成功 -> 对应 action 审计
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/contestapi/ -run 'TestContestVisibility|TestContestRoutes|TestContestGranular|TestContestCanManage|TestContestWritesAudit' -v`
Expected: FAIL。

- [ ] **Step 3: 实现比赛权限**

- 替换 `CanManage` 判断：
  - Create/Clone → `CanCreate`
  - Update → `CanUpdate`
  - Delete → `CanDelete`
  - Participants → `CanReadParticipants`
  - Announcement 写 → `CanManageAnnouncements`
  - `findAccessible`、`List IncludeHidden`、`Detail` 开赛前题目、`Ranklist` upcoming/selfOnly → `CanViewHidden`
- `Detail.CanManage = v.CanManageAny()`。
- `rating.Service` 内部 `contest.Viewer{CanManage: true}` 改为 `contest.Viewer{CanViewHidden: true}`。
- `contestapi.viewer` 从 `CurrentAccess` 映射六个字段。
- 测试 fixture 加 SeedRBAC/AssignLegacyRole，`token("contest_admin")` 同时写 legacy role 和 RBAC 角色；另加不分配 RBAC 角色的 legacy token helper，用于证明旧 role 不再授权；直接 `contestapi.RegisterRoutes` 的 hook 测试传 `Options{RBAC: rbacSvc}`。
- Router 传 `contestapi.Options{RBAC: d.RBAC}`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/contest internal/contestapi internal/rating internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/contest/ ./internal/contestapi/ ./internal/rating/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/contest apps/server/internal/contestapi apps/server/internal/rating apps/server/internal/router
git commit -m "feat(server): migrate contest permissions"
```

---

### Task 4: 测评记录权限

**Files:**
- Modify: `apps/server/internal/submission/service.go`
- Modify: `apps/server/internal/submission/query.go`
- Modify: `apps/server/internal/submission/service_test.go`
- Modify: `apps/server/internal/submission/records_test.go`
- Modify: `apps/server/internal/submissionapi/handler.go`
- Modify: `apps/server/internal/submissionapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 基础设施、Task 2/3 的权限映射。
- Produces:
  - `submission.Viewer{UserID int64; CanViewHiddenProblems, CanViewLiveContest, CanManageContest bool}`。
  - `submissionapi.Options{RBAC *rbac.Service}`（已有 Options，增加字段）。
  - `GET /submissions`、`GET /submissions/:id` 挂 `LoadAccess`。
  - 权限映射：
    - 隐藏题提交列表/详情/提交 → `problem.view_hidden`
    - 赛时保护绕过 → `submission.view_live_contest`
    - 免报名比赛测试提交 → `contest.update`
  - 不新增管理路由；无内容审计。

- [ ] **Step 1: 写失败测试**

```go
func TestSubmissionHiddenProblemUsesViewHidden(t *testing.T)
// 无 view_hidden：他人隐藏题提交列表/详情 404
// 有 view_hidden：可见他人隐藏题提交

func TestSubmissionLiveContestUsesPermission(t *testing.T)
// 无 submission.view_live_contest：赛时他人代码/成绩/测试点被隐藏
// 有该权限：可见
// 本人仍可见自己的提交

func TestSubmissionContestSubmitRequiresRegistrationUnlessCanUpdate(t *testing.T)
// 普通用户未报名 -> 40001
// contest.update 用户未报名 -> 可提交
// problem.view_hidden 不绕过报名

func TestSubmissionRoutesStillPublic(t *testing.T)
// 游客 GET /submissions 200；未登录 POST 401
// legacy admin token 但无 RBAC 角色不获得隐藏/赛时能力
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submission/ ./internal/submissionapi/ -run 'TestSubmissionHidden|TestSubmissionLive|TestSubmissionContestSubmit|TestSubmissionRoutesStillPublic' -v`
Expected: FAIL。

- [ ] **Step 3: 实现测评权限**

- `Viewer` 字段替换；`query.go` 的 `CanSeeHidden` 用 `CanViewHiddenProblems`。
- `Detail` 隐藏题判断用 `CanViewHiddenProblems`。
- `contestProtected` 用 `CanViewLiveContest`。
- `Create`：
  - 比赛免报名用 `CanManageContest`（`contest.update`）。
  - 非公开题无比赛提交用 `CanViewHiddenProblems`。
- `submissionapi.viewer` 从 `CurrentAccess` 映射。
- `GET /submissions`、`GET /submissions/:id`、`POST /submissions` 都挂 `LoadAccess(s)`。
- 测试 fixture 里需要权限的场景用 `testutil.SeedRBAC` + `AssignRole`；普通用户测试保持无角色。
- Router 传 `submissionapi.Options{RBAC: d.RBAC}`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/submission internal/submissionapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/submission/ ./internal/submissionapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/submission apps/server/internal/submissionapi apps/server/internal/router
git commit -m "feat(server): migrate submission permissions"
```

---

### Task 5: 游戏化权限、金币调整迁移与审计

**Files:**
- Modify: `apps/server/internal/gamification/dto.go`
- Modify: `apps/server/internal/gamification/achievement.go`
- Modify: `apps/server/internal/gamification/task.go`
- Modify: `apps/server/internal/gamification/badge.go`
- Modify: `apps/server/internal/gamification/title.go`
- Modify: `apps/server/internal/gamification/service.go`
- Modify: `apps/server/internal/gamification/coin_repository.go`
- Modify: `apps/server/internal/gamification/*_test.go`
- Modify: `apps/server/internal/gamificationapi/handler.go`
- Modify: `apps/server/internal/gamificationapi/badge_handler.go`
- Modify: `apps/server/internal/gamificationapi/handler_test.go`
- Modify: `apps/server/internal/adminapi/role_handler.go`（Options/路由注册）
- Create: `apps/server/internal/adminapi/coin_handler.go`
- Create: `apps/server/internal/adminapi/coin_handler_test.go`
- Modify: `apps/server/internal/adminapi/user_handler_test.go`（fixture Options）
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 基础设施、`rbac.AuditActionUserCoinsAdjust`。
- Produces:
  - `gamification.Viewer{UserID int64; CanManageAchievements, CanManageTasks, CanManageBadges, CanManageTitles, CanAdjustCoins bool}`。
  - `gamificationapi.Options{RBAC *rbac.Service}`；`RegisterRoutes(r, uploads, s, opts ...Options)`。
  - `gamification.Service.AdminAdjustCoinsWithAudit(ctx, v Viewer, userID, amount int64, note string, audit func(tx *gorm.DB) error) (int64, error)`。
  - `gamification.Repository.AdminAdjustCoinsWithAudit(...)`（审计回调在事务内、加币后执行）。
  - `adminapi.Options{Game *gamification.Service}`；`RegisterRoutes(api, s, opts ...Options)`。
  - `POST /api/admin/users/:id/coins` 从 `gamificationapi` 移到 `adminapi`，权限 `user.coins.adjust`，事务内审计。
  - 权限接线：
    - 成就 CRUD/`includeInactive` → `gamification.achievement.manage`
    - 任务 CRUD/`includeInactive` → `gamification.task.manage`
    - 徽章 CRUD/图片上传/`includeInactive` → `gamification.badge.manage`
    - 称号 CRUD/`includeInactive` → `gamification.title.manage`
    - 金币调整 → `user.coins.adjust`
    - `gamification.config` 仍公开只读；`gamification.config.manage` 暂无写接口。
  - 内容审计 action 使用对应权限码，target type `gamification_*`。

- [ ] **Step 1: 写失败测试**

```go
func TestGamificationRoutesRequireDomainPermissions(t *testing.T)
// legacy super_admin token 但无 RBAC 角色 -> 成就/任务/徽章/称号/金币 403
// gamification_admin -> 成就/任务/徽章/称号 成功；金币 403
// user_admin -> 金币 200；成就 403

func TestGamificationIncludeInactiveUsesDomainPermission(t *testing.T)
// achievement-only 角色 includeInactive 成就 200，任务 403
// task-only、badge-only、title-only 同理
// 普通用户 includeInactive 仍 403

func TestGamificationWritesAudit(t *testing.T)
// 成就/任务/徽章/称号成功 -> 对应 action 审计
// 失败请求不写审计

func TestAdminCoinAdjustRequiresPermissionAndAudits(t *testing.T)
// 无 user.coins.adjust -> 403
// 有权限 -> 200，余额变化，审计 action=user.coins.adjust target=user
// 审计失败 -> 500 且余额回滚（用可注入 audit 错误测试 service 层）
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ ./internal/adminapi/ -run 'TestGamificationRoutes|TestGamificationInclude|TestGamificationWritesAudit|TestAdminCoinAdjust' -v`
Expected: FAIL。

- [ ] **Step 3: 实现游戏化权限与金币迁移**

- `gamification` 各子域把 `IsAdmin` 替换为对应 `CanManage*`；`AdminAdjustCoins` 用 `CanAdjustCoins`。
- `gamificationapi.viewer` 从 `CurrentAccess` 映射五个字段。
- 每个管理路由加 `RequireAuth + RequirePermission + AuditAdminAction`；读路由加 `LoadAccess`。
- 删除 `gamificationapi` 的 `admin` wrapper、`/admin/users/:id/coins` 路由和 `adminAdjustCoins` handler。
- 新增 `adminapi` coin handler：
  - `auditActor(c)` 取得 actor。
  - `AdminAdjustCoinsWithAudit` 的 audit 回调调用 `rbac.WriteAuditTx(tx, actor, rbac.AuditActionUserCoinsAdjust, rbac.AuditTargetUser, id, fmt.Sprintf("调整用户金币 %+d", amount))`。
  - 只记录金额，不记录备注。
- 从 `adminapi` 的 `Options.Game` 注入游戏化服务；Router 传 `adminapi.Options{Game: d.Game}`。
- `rbac/audit.go` 新增 `AuditActionUserCoinsAdjust = "user.coins.adjust"`。
- 测试 fixture 加 SeedRBAC/AssignRole；`token("super_admin")` 同时映射 RBAC 角色；另加不分配 RBAC 角色的 legacy token helper，用于证明旧 role 不再授权。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/gamification internal/gamificationapi internal/adminapi internal/router internal/rbac && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/gamification/ ./internal/gamificationapi/ ./internal/adminapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/gamification apps/server/internal/gamificationapi apps/server/internal/adminapi apps/server/internal/router apps/server/internal/rbac
git commit -m "feat(server): migrate gamification permissions and coin audit"
```

---

### Task 6: 学习路线权限与审计

**Files:**
- Modify: `apps/server/internal/learning/dto.go`
- Modify: `apps/server/internal/learning/path_service.go`
- Modify: `apps/server/internal/learning/node_service.go`
- Modify: `apps/server/internal/learning/*_test.go`
- Modify: `apps/server/internal/learningapi/handler.go`
- Modify: `apps/server/internal/learningapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 基础设施、Task 2 fixture 模式。
- Produces:
  - `learning.Viewer{UserID int64; CanManageOfficial, CanViewAll bool}`。
  - `learningapi.Options{RBAC *rbac.Service}`；`RegisterRoutes(r, s, opts ...Options)`。
  - 读路由 `GET /learning-paths`、`GET /learning-paths/:id`、`GET /learning-paths/:id/tree` 挂 `LoadAccess`。
  - 权限语义：
    - `CanManageOfficial`（`learning_path.official.manage`）：可创建官方路线、管理官方路线、管理任意路线的上下架/恢复/节点（保持现有内容管理员行为）。
    - `CanViewAll`（`learning_path.admin.read`）：只读查看全部路线、下架路线、完整题目 ID；不能创建/编辑/删除/写节点。
    - 普通用户自定义路线仍按作者规则。
  - 学习路线写操作加 `AuditAdminAction`，action `learning_path.official.manage`，target type `learning_path`；普通作者操作因无该权限不写审计。

- [ ] **Step 1: 写失败测试**

```go
func TestLearningPathViewerPermissions(t *testing.T)
// content_admin（official.manage+admin.read）保持现有官方/自定义路线管理行为
// admin.read-only：includeInactive/详情/tree 200，创建官方/更新/删除/节点 403 或 404
// 普通作者：自己的自定义路线 CRUD 成功，官方路线 403，他人路线 404

func TestLearningPathInactiveVisibility(t *testing.T)
// 无权限看不到 inactive；admin.read 可列/查 inactive；official.manage 可按 id 查 inactive 官方路线

func TestLearningPathTreeUsesAdminRead(t *testing.T)
// admin.read 看到完整 problemIds；普通非作者只看到公开题 ID 摘要

func TestLearningPathWritesAudit(t *testing.T)
// official.manage 创建/更新/删除/节点 -> 审计
// 普通作者写自定义路线 -> 不写管理审计
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/learningapi/ -run 'TestLearningPathViewer|TestLearningPathInactive|TestLearningPathTreeUses|TestLearningPathWritesAudit' -v`
Expected: FAIL。

- [ ] **Step 3: 实现学习路线权限**

- `CanManage` 替换为 `CanManageOfficial`（写/管理）和 `CanViewAll`（读全部）。
- `ListPaths` 的 `IncludeInactive` 用 `CanViewAll || CanManageOfficial`。
- `FindPath` inactive 用 `CanViewAll || CanManageOfficial`；私有自定义路线非作者可见仅 `CanViewAll`。
- `UpdatePath`/`DeletePath`/`writablePath`：作者规则保留；`CanManageOfficial` 可管理任意路线；`CanViewAll` 不参与写权限。
- `Tree` 完整 problemIds：作者或 `CanViewAll` 或 `CanManageOfficial`。
- `learningapi.viewer` 从 `CurrentAccess` 映射两个字段。
- 所有学习路线路由都挂 `LoadAccess`；写路由不强制权限（保留普通作者/管理员语义），但加 `AuditAdminAction`。
- 测试 fixture 加 SeedRBAC/AssignLegacyRole，`token(..., RoleProblemAdmin)` 同时映射 `content_admin`；另加不分配 RBAC 角色的 legacy token helper，用于证明旧 role 不再授权。
- Router 传 `learningapi.Options{RBAC: d.RBAC}`。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/learning internal/learningapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/learning/ ./internal/learningapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/learning apps/server/internal/learningapi apps/server/internal/router
git commit -m "feat(server): migrate learning path permissions"
```

---

### Task 7: OpenAPI、文档、全量检查与冒烟

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–6。
- Produces: RBAC-C 状态文档、OpenAPI 权限描述与前端类型。

- [ ] **Step 1: 更新 OpenAPI**

- 每个管理接口 description/summary 写明权限码，不再写“仅超级管理员”。
- `includeInactive` 描述改为对应域 manage 权限（成就/任务/徽章/称号）。
- 题目 `isPublic` 描述写明需要 `problem.update` + `problem.publish`。
- 隐藏题/非公开比赛/赛时提交描述写明对应权限码。
- 学习路线描述区分 `learning_path.official.manage` 与 `learning_path.admin.read`。
- 金币调整描述改为 `user.coins.adjust`，审计说明。
- 运行 `corepack pnpm gen:api`。

- [ ] **Step 2: 更新文档**

- `DEVELOPMENT.md`：RBAC-C 完成；所有内容管理接口已迁移；legacy role 不再授权；列出各域权限映射与 Viewer 拆分。
- `docs/任务清单.md`：RBAC-C ✅，RBAC-D ⬜；链接计划。
- `docs/开发日志.md`：追加 RBAC-C 条目（Access 加载、审计、题目/比赛/测评/游戏化/学习路线、金币审计、验证）。
- `README.md`、`运行手册.md`：更新权限矩阵、内容管理权限、审计行为、旧角色兼容说明。
- 不修改 `docs/primevue-migration.md`（管理 UI 仍 RBAC-D）。
- 不修改 `users.role` 字段/迁移；只声明不再授权。

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

额外检查（不应有输出）：

```bash
grep -RIn "RequireRole(\|claims\.Role" apps/server/internal --include='*.go' | grep -v "_test.go" | grep -v "internal/middleware/auth.go"
```

- [ ] **Step 4: 真实 API 冒烟**

- 重建并重启本地 server（:18080），确认 Seed 和 `m2badmin` super。
- 创建自定义角色并验证：
  - 只有 `problem.view_hidden`：可看隐藏题，不能创建/发布；`/users/me` 出现权限。
  - `problem.update`：可改标题，改 `isPublic` 403；加 `problem.publish` 后可发布。
  - 只有 `contest.view_hidden`：可看非公开比赛，不能创建比赛。
  - 只有 `gamification.achievement.manage`：可建成就，不能建任务/徽章。
  - `learning_path.admin.read`：可看下架路线，不能写节点。
  - `user.coins.adjust`：可调金币；审计出现 `user.coins.adjust`。
- 验证内容审计：题目/比赛/游戏化/学习路线写操作产生审计；普通作者写自定义路线不产生管理审计。
- 清理临时角色/分配/测试数据；保留或清理审计并按报告说明。
- 所有临时变更必须记录并恢复。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record rbac-c content permissions"
```

---

## Self-Review

**Spec coverage:**

- 权限加载/复用/审计基础设施 → Task 1。
- 题库权限、发布保护、内容审计 → Task 2。
- 比赛权限/隐藏读取/公告/报名用户 → Task 3。
- 测评隐藏题/赛时保护/免报名 → Task 4。
- 游戏化五子域权限、金币迁移与审计 → Task 5。
- 学习路线 official.manage/admin.read/作者规则 → Task 6。
- OpenAPI/文档/全量检查/真实冒烟 → Task 7。
- 管理 UI、内容管理页面重做 → RBAC-D，不在本计划。

**Type consistency:**

- 所有 Viewer 使用布尔能力字段；不再用 `CanManage` 聚合做授权。
- `middleware.HasPermission` 与 `Access.Has` 使用稳定权限码字符串。
- 金币审计 action `user.coins.adjust` 与权限码一致。
- 内容审计 action 使用权限码；target type 固定字符串。
- 学习路线 `CanManageOfficial` 保留内容管理员现有写行为；`CanViewAll` 只读。

**Review Focus mapping:**

1. 旧角色不授权 → Task 2/3/5 权限反例。
2. 隐藏资源不泄漏 → Task 2/3/4 可见性测试。
3. 发布边界 → Task 2 `TestProblemPublishRequiresExtraPermission`。
4. 审计隔离 → Task 1 `TestAuditAdminAction`、Task 5 金币回滚。
5. 域权限独立 → Task 3/5/6 粒度测试。

**Proportion:** 计划固定权限映射、Viewer 字段、审计边界和测试；具体查询/服务实现留给实现者按现有代码风格完成。
