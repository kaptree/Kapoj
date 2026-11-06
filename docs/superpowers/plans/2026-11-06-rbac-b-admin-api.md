# RBAC-B 角色、用户、审计与统计 API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付管理后台后端 API：角色/权限矩阵、用户角色分配与封禁、管理员编辑用户、审计日志、仪表盘统计；所有接口使用 `RequirePermission`。

**Architecture:** 在 `rbac` 包增加角色/用户管理/审计/统计服务；新增 `adminapi` 挂载 `/api/admin/*`；所有敏感变更在同一事务写审计；系统角色、`super_admin`、最后一个 active super_admin、自封禁做保护。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、testify。

**Spec:** `docs/superpowers/specs/2026-11-06-admin-rbac-design.md` 6–10、12–14。

## Global Constraints

- 所有 `/api/admin/*` 使用 `RequirePermission`；super_admin 绕过但仍审计。
- 角色/权限变更、用户角色分配、封禁/解封、管理员编辑用户在同一事务写审计。
- 系统角色不可删除、code/name 不可改；`super_admin` 不可改权限。
- 只有 super_admin 能分配/移除/封禁/解封 super_admin；禁止封自己；禁止移除/封禁最后一个 active super_admin。
- 自定义角色 code 创建后不可改；权限用 code 整体替换。
- 审计不记录密码、token、代码、测试数据；摘要中文。
- 不实现 RBAC-C 的既有接口迁移；硬币调整端点迁移留给 RBAC-C（RBAC-B 不改现有 `/admin/users/:id/coins`）。
- 不实现管理前端（RBAC-D）。
- 所有文案中文；不推送、不合并分支。

## Review Focus

1. **最后 super 保护**：并发移除/封禁不能把系统变成无 super；测试放 Task 2。
2. **系统角色保护**：删除/改名/改 code/改 super 权限均拒绝；测试放 Task 1。
3. **权限矩阵整体替换**：非法 code/重复/空处理；测试放 Task 1。
4. **审计事务性**：敏感操作失败不留下审计，成功必有审计；测试放 Task 1/2/3。
5. **权限边界**：每个 API 需要正确权限码，普通用户/无权限角色 403；测试放 Task 3。

---

### Task 1: 角色/权限服务与 API

**Files:**
- Create: `apps/server/internal/rbac/roles.go`
- Create: `apps/server/internal/rbac/roles_test.go`
- Create: `apps/server/internal/rbac/audit.go`（审计写入 helper）
- Create: `apps/server/internal/rbac/audit_test.go`
- Create: `apps/server/internal/adminapi/role_handler.go`
- Create: `apps/server/internal/adminapi/role_handler_test.go`
- Modify: `apps/server/internal/adminapi/`（如已有目录则复用）
- Modify: `apps/server/internal/router/router.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: RBAC-A models/Service/Access/middleware、`user` roles。
- Produces:
  - `rbac.PermissionItem{Code,Name,GroupName,Description string; SortOrder int}`。
  - `rbac.RoleItem{ID int64; Code,Name,Description string; IsSystem bool; Permissions []string; UserCount int64; CreatedAt,UpdatedAt time.Time}`。
  - `rbac.RoleInput{Code,Name,Description string; PermissionCodes []string}`（create）；`RoleUpdateInput{Name,Description string}`。
  - `rbac.AuditMeta{ActorID int64; IP,UserAgent string}`。
  - `(*Service).ListPermissions(ctx) []PermissionItem`。
  - `(*Service).ListRoles(ctx) ([]RoleItem,error)`。
  - `(*Service).CreateRole(ctx, actor AuditMeta, in RoleInput) (*RoleItem,error)`。
  - `(*Service).UpdateRole(ctx, actor AuditMeta, id int64, in RoleUpdateInput) (*RoleItem,error)`。
  - `(*Service).DeleteRole(ctx, actor AuditMeta, id int64) error`。
  - `(*Service).ReplaceRolePermissions(ctx, actor AuditMeta, id int64, codes []string) (*RoleItem,error)`。
  - `writeAudit(tx *gorm.DB, actor AuditMeta, action, targetType string, targetID int64, summary string) error`。
  - API：`GET /api/admin/permissions`（`role.read`）、`GET /api/admin/roles`（`role.read`）、`POST /api/admin/roles`（`role.create`）、`PUT /api/admin/roles/:id`（`role.update`）、`DELETE /api/admin/roles/:id`（`role.delete`）、`PUT /api/admin/roles/:id/permissions`（`role.permissions.update`）。

- [ ] **Step 1: 写失败角色/权限测试**

```go
func TestListPermissionsAndRoles(t *testing.T)
// 34 permissions grouped; 7 built-in roles; role permissions/userCount correct
func TestCreateCustomRole(t *testing.T)
// code regex, name length, permission code validation, custom is_system=false
func TestSystemRoleProtection(t *testing.T)
// delete system -> 40001; update name/code -> 40001; super permissions -> 40001
func TestReplaceRolePermissions(t *testing.T)
// replace set, dedup invalid code 40001, audit row exists
func TestDeleteCustomRoleCleansJoins(t *testing.T)
// delete custom role removes role_permissions/user_roles; audit
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ -run 'Test.*Role|TestListPermissions' -v`
Expected: FAIL（类型/方法不存在）。

- [ ] **Step 3: 实现角色/权限服务**

- `ListPermissions` 从 `AllPermissions()` 返回，按 GroupName/SortOrder 排序；无需 DB。
- 角色 code 正则 `^[a-z][a-z0-9_]{2,63}$`；name 1–100；description ≤500；权限 codes 必须全部存在于目录、去重（重复可拒绝或去重，计划固定为去重）。
- `CreateRole`：事务；插入 Role；校验/插入 role_permissions；写审计 `role.create`；返回 RoleItem。
- `UpdateRole`：系统角色 code/name 不可改（若输入不同 → 40001）；自定义角色 code 不可改（输入 code 不应存在于 update input）；描述可改；审计 `role.update`。
- `DeleteRole`：系统角色 40001；事务内删除 user_roles/role_permissions/role；审计 `role.delete`。
- `ReplaceRolePermissions`：super_admin 40001；校验 codes；事务删除旧 role_permissions，插入新；审计 `role.permissions.update`；返回角色。
- `ListRoles`：roles + permission codes + userCount（`COUNT(user_roles)` 子查询或分组）。
- `audit.go`：`writeAudit` 插入 AdminAuditLog；Summary 中文；空 IP/UA 允许。
- `adminapi`：helper `requirePermission(s, code)` 组合 `RequireAuth` + `RequirePermission`；actor 从 claims + c.ClientIP + UA；错误统一 `httpx.Fail`。

- [ ] **Step 4: 实现路由与测试、提交**

- Router `Deps.RBAC` 已有；挂载 `adminapi.RegisterRoutes(api, d.RBAC)`（若 `d.RBAC != nil`）。
- **重要**：Task 2 增加 `GET /api/admin/users` 前，必须从 `userapi` 移除旧的 `GET /admin/users` 路由和 `listUsers` handler，否则 Gin 会因重复路由 panic。旧 super_admin 测试迁移到 `adminapi` 的 `user.read` 权限测试。
- API handler 测试：403 无权限、super 通过、角色创建/更新/删除/权限替换、系统角色保护、审计存在。

Run: `cd apps/server && gofmt -w internal/rbac internal/adminapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rbac apps/server/internal/adminapi apps/server/internal/router
git commit -m "feat(server): rbac role management api"
```

---

### Task 2: 用户管理、角色分配、封禁与最后 super 保护

**Files:**
- Create: `apps/server/internal/rbac/users.go`
- Create: `apps/server/internal/rbac/users_test.go`
- Create: `apps/server/internal/adminapi/user_handler.go`
- Create: `apps/server/internal/adminapi/user_handler_test.go`
- Modify: `apps/server/internal/rbac/roles.go`（共享 RBAC advisory lock 常量）
- Modify: `apps/server/internal/adminapi/` 路由注册
- Modify: `apps/server/internal/userapi/handler.go`（移除旧 `GET /admin/users` 与 listUsers，避免与 adminapi 重复路由）
- Modify: `apps/server/internal/userapi/handler_test.go`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 audit/actor/role helpers、user 模型/校验。
- Produces:
  - `rbac.AdminUserItem{ID,Username,Nickname,Email string; Role user.Role; Rating int; Coins int64; Status user.Status; Roles []RoleSummary; CreatedAt time.Time}`。
  - `rbac.AdminUserDetail{AdminUserItem; IsSuperAdmin bool}`（可选 effective permissions）。
  - `rbac.AdminUserInput{Keyword string; Status user.Status; RoleID int64}` + page/size list.
  - `rbac.AdminUserUpdateInput{Nickname,Bio,AvatarURL *string}`。
  - `(*Service).AdminListUsers(ctx, page,size, filter AdminUserFilter) (httpx.Page[AdminUserItem],error)`。
  - `(*Service).AdminUser(ctx,id) (*AdminUserDetail,error)`。
  - `(*Service).AdminUpdateUser(ctx, actor, id, in) error`。
  - `(*Service).ReplaceUserRoles(ctx, actor, userID, roleIDs) error`。
  - `(*Service).SetUserBanned(ctx, actor, userID int64, banned bool, reason string) error`。
  - API：`GET /api/admin/users`（`user.read`）、`GET/PUT /api/admin/users/:id`（`user.read`/`user.update`）、`PUT /api/admin/users/:id/roles`（`user.roles.assign`）、`POST /api/admin/users/:id/ban|unban`（`user.ban`）。

- [ ] **Step 1: 写失败用户管理/保护测试**

```go
func TestAdminListUsersFilters(t *testing.T)
// keyword/status/roleId; roles included; page/total
func TestAdminUpdateUser(t *testing.T)
// nickname/bio/avatar validation; audit
func TestReplaceUserRoles(t *testing.T)
// multi-role assign; audit; unknown role 40001
func TestOnlySuperAssignsSuper(t *testing.T)
// non-super actor assigns/removes super -> 403; super can
func TestLastSuperProtection(t *testing.T)
// remove/ban last active super -> 40001; with two supers allowed
func TestBanRules(t *testing.T)
// self-ban 40001; only super can ban/unban super; audit
func TestConcurrentLastSuperRemoval(t *testing.T)
// two concurrent removals of two supers -> one succeeds, one rejected
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ -run 'TestAdmin|TestReplaceUserRoles|TestOnlySuper|TestLastSuper|TestBan|TestConcurrent' -v`
Expected: FAIL。

- [ ] **Step 3: 实现用户管理**

- `AdminListUsers`：查询 users；keyword 对 username/nickname/email 做 ILIKE（转义 `%`/`_`）；status/roleId 过滤；分页；批量查 roles；返回 `httpx.Page[AdminUserItem]`。
- 从 `userapi` 移除旧 `/admin/users` 路由/`listUsers`；新的 adminapi 端点使用 `user.read`。
- `AdminUser`：用户 + roles；不存在 404。
- `AdminUpdateUser`：validate nickname/bio/avatar（复用 user 校验）；UpdateProfile；审计 `user.update`。
- `ReplaceUserRoles`：
  - 事务；获取 `pg_advisory_xact_lock`（RBAC 管理锁）；加载目标用户及当前角色；校验 role IDs 存在；
  - actor 非 super 且目标当前/新角色含 super_admin → 403；
  - 若移除 super，统计 active super 数量；若目标是最后一个 → 40001；
  - 删除 user_roles，插入新 user_roles（assigned_by=actor）；审计 `user.roles.assign`。
- `SetUserBanned`：
  - 事务 + advisory lock；用户不存在 404；自封禁 40001；
  - 目标当前 super 且 actor 非 super → 403；
  - 封禁时若目标是最后一个 active super → 40001；
  - 更新 status；审计 `user.ban`/`user.unban`（reason 非空写入 summary）。
- `adminapi` handlers：actor 从 claims/IP/UA；输入校验；403/404/40001 中文。

- [ ] **Step 4: 契约/路由测试、运行、提交**

Run: `cd apps/server && gofmt -w internal/rbac internal/adminapi && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rbac apps/server/internal/adminapi apps/server/internal/router
git commit -m "feat(server): rbac user administration"
```

---

### Task 3: 审计查询、仪表盘统计与权限接线

**Files:**
- Create: `apps/server/internal/rbac/audit_query.go`
- Create: `apps/server/internal/rbac/audit_query_test.go`
- Create: `apps/server/internal/rbac/stats.go`
- Create: `apps/server/internal/rbac/stats_test.go`
- Create: `apps/server/internal/adminapi/audit_handler.go`
- Create: `apps/server/internal/adminapi/stats_handler.go`
- Create: `apps/server/internal/adminapi/admin_handler_test.go`
- Modify: `apps/server/internal/adminapi/` 路由注册
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1/2 audit rows、models、DB。
- Produces:
  - `rbac.AuditFilter{ActorID int64; Action,TargetType string; From,To *time.Time}`。
  - `rbac.AuditItem{ID,ActorID; Action,TargetType string; TargetID *int64; Summary,IP,UserAgent string; CreatedAt time.Time}`。
  - `(*Service).ListAudit(ctx, filter, page,size) (httpx.Page[AuditItem],error)`。
  - `rbac.Stats{Users, ActiveUsers, BannedUsers, Problems, PublicProblems, Contests, Submissions, TodaySubmissions, Roles int64; RecentAudit []AuditItem}`。
  - `(*Service).Stats(ctx, includeAudit bool) (*Stats,error)`。
  - API：`GET /api/admin/audit`（`admin.audit.view`）、`GET /api/admin/stats`（`admin.dashboard.view`）。
  - Admin route registration all under `adminapi.RegisterRoutes(api, s)`.
  - Existing `/admin/users/:id/coins` remains; no migration in RBAC-B.

- [ ] **Step 1: 写失败审计/统计测试**

```go
func TestListAuditFiltersAndPagination(t *testing.T)
// actor/action/target/date; order desc; total
func TestStatsCounts(t *testing.T)
// users active/banned; problems public; contests; submissions today; roles
func TestStatsAuditPermission(t *testing.T)
// includeAudit false omits RecentAudit; true includes
func TestAdminRoutesRequirePermissions(t *testing.T)
// each endpoint without permission 403; with permission/super 200
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ -run 'TestListAudit|TestStats|TestAdminRoutes' -v`
Expected: FAIL。

- [ ] **Step 3: 实现审计查询与统计**

- `ListAudit`：query `admin_audit_logs`；filters；order `created_at DESC,id DESC`；page/size；返回 items/total。
- `Stats`：单次或多次 count 查询（低流量可接受）；用户 total/active/banned，problems total/public，contests total，submissions total/today，roles total；`includeAudit` 时取最近 10 条审计。
- `adminapi` handlers：
  - `GET /admin/audit` parse actorId/action/targetType/from/to/page/page_size；日期用 RFC3339/日期。
  - `GET /admin/stats` 从 `CurrentAccess` 判断 `admin.audit.view` 是否包含 RecentAudit。
- `adminapi.RegisterRoutes` 集中挂载所有 RBAC-B 路由，使用 `RequirePermission`。
- 路由测试覆盖每个权限码正反。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/rbac internal/adminapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/adminapi/ ./internal/router/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rbac apps/server/internal/adminapi apps/server/internal/router
git commit -m "feat(server): rbac audit and admin stats"
```

---

### Task 4: OpenAPI、文档、全量检查与冒烟

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
- Consumes: Task 1–3。
- Produces: RBAC-B 状态文档与前端可用类型。

- [ ] **Step 1: OpenAPI**

- Schemas：`AdminPermissionItem`、`AdminRoleItem`、`AdminRoleInput`、`AdminRoleUpdateInput`、`AdminRolePermissionsInput`、`AdminUserItem`、`AdminUserPage`、`AdminUserUpdateInput`、`AdminUserRolesInput`、`AdminAuditItem/Page`、`AdminStats`。
- Paths：`/admin/permissions`、`/admin/roles` CRUD/permissions、`/admin/users` list/detail/update/roles/ban/unban、`/admin/audit`、`/admin/stats`。
- 权限/错误/分页描述；`gen:api`。

- [ ] **Step 2: 文档**

- `DEVELOPMENT.md` 管理后台章节：RBAC-B 完成（角色/用户/审计/统计 API）；RBAC-C/D 待办。
- `docs/任务清单.md`：RBAC-B ✅，RBAC-C/D ⬜；计划链接。
- `docs/开发日志.md`：追加 RBAC-B 条目（角色矩阵、用户保护、审计、统计、验证、复审修复）。
- `README.md`、`运行手册.md`：管理 API 已可用但管理 UI 未上线；内置角色/权限矩阵、最后 super/封禁规则、审计查询。
- 不修改 `docs/primevue-migration.md`（管理 UI 未做）。

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

- [ ] **Step 4: 真实 API 冒烟**

- 重启 server；确认 Seed 后 `m2badmin` super。
- 用 super 创建自定义角色（如只有 `problem.view_hidden`），分配给 `m2bsolver1`；该用户 `/users/me` 出现权限；无 `admin.dashboard.view` 时 `/admin/stats` 403。
- 给角色添加 `admin.dashboard.view`/`user.read`，验证 `/admin/stats`/`/admin/users` 200。
- 验证系统角色删除 40001、`super_admin` 权限修改 40001。
- 验证最后一个 super 不能被移除/封禁；自封禁 40001；第二个 super 存在时允许移除一个。
- 验证审计记录；最后清理临时角色/分配（保留审计或清理测试数据并报告）。

- [ ] **Step 5: 提交**

```bash
git add packages/api/openapi.yaml apps/web/src/api/schema.d.ts DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record rbac-b admin api"
```

---

## Self-Review

**Spec coverage:**

- 角色/权限矩阵 → Task 1。
- 用户管理/角色分配/封禁/最后 super → Task 2。
- 审计/统计/权限接线 → Task 3。
- OpenAPI/文档/冒烟 → Task 4。
- RBAC-C/D 仍待办。

**Type consistency:**

- `RoleItem.Permissions []string` 使用 code；`RoleInput.PermissionCodes` 同 code。
- `AdminUserItem.Roles []RoleSummary` 与 RBAC-A 一致。
- 所有 admin 路由用 `RequirePermission`；super bypass。
- `AuditMeta` 从 claims + IP/UA 构造；审计 action 稳定。

**Review Focus mapping:**

1. 最后 super → Task 2 tests（含并发）。
2. 系统角色保护 → Task 1 tests。
3. 权限矩阵整体替换 → Task 1 tests。
4. 审计事务性 → Task 1/2 audit tests。
5. 权限边界 → Task 3 route tests。

**Proportion:** 计划固定 API、权限码、事务/保护规则与测试；具体 SQL/查询按 RBAC-A/M4 风格完成。
