# RBAC-A 权限模型、Seed 与权限中间件 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付细粒度 RBAC 后端基础：权限目录、角色/权限/用户角色表、首次 Seed、Access 解析、`RequirePermission` 中间件，以及 `/users/me` 的 roles/permissions。

**Architecture:** 新增 `rbac` 包；`users.role`/JWT role 只保留为 legacy 展示兼容，RBAC-C 前未迁移的管理接口仍按 legacy role 授权；RBAC-A 新管理授权通过 `rbac.Service.Access(userID)` 实时查询角色与权限，`middleware.RequirePermission` 做最终校验。RBAC-B/C/D 再做角色管理 API、接口迁移与管理后台。

**Tech Stack:** Go 1.26、Gin、GORM、PostgreSQL 17、testify。

**Spec:** `docs/superpowers/specs/2026-11-06-admin-rbac-design.md` 4–8、13、14。

## Global Constraints

- 一个账号可拥有多个角色，权限取并集；权限全局生效，不做资源范围。
- `super_admin` 角色绕过所有权限检查；不可删除/不可改权限；最后一个 active super_admin 保护在 RBAC-B 实现。
- `users.role`/JWT role 只做 legacy 展示兼容；新 `RequirePermission` 授权只认 `user_roles`；RBAC-C 前未迁移的管理接口仍按 legacy role 授权。
- 权限码稳定，格式 `domain.action`；不允许 UI 自定义新权限码。
- Seed 只在首次执行（`rbac_meta.seed_version`），不覆盖管理员后续权限调整。
- 权限实时查库；不写 JWT，不做 Redis 缓存。
- 封禁用户的 Access 解析返回拒绝。
- 所有管理授权后续必须迁移到 `RequirePermission`；本阶段不改现有接口，只提供基础设施。
- 所有用户可见文案中文；不推送、不合并分支。

## Review Focus

1. **多角色权限并集**：两个角色权限合并、super 绕过；测试放 Task 2。
2. **旧角色映射/Seed 幂等**：重复启动不重复插入、管理员删除的权限不被重加；测试放 Task 1。
3. **权限变更即时生效**：角色变更后同一 token 下一次请求权限已更新；测试放 Task 2。
4. **封禁用户**：Access 拒绝，管理请求 403；测试放 Task 2。
5. **中间件 401/403/权限码**：未登录/无权限/单权限/任一权限；测试放 Task 2。

---

### Task 1: RBAC 模型、权限目录与首次 Seed

**Files:**
- Create: `apps/server/internal/rbac/model.go`
- Create: `apps/server/internal/rbac/catalog.go`
- Create: `apps/server/internal/rbac/seed.go`
- Create: `apps/server/internal/rbac/seed_test.go`
- Modify: `apps/server/internal/db/db.go`
- Modify: `apps/server/internal/db/db_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/rbac/seed_test.go`

**Interfaces:**
- Consumes: `user.Role` legacy values、`db.Migrate`、`testutil.PG`。
- Produces:
  - `rbac.Role{ID,Code,Name,Description,IsSystem,CreatedAt,UpdatedAt}`。
  - `rbac.Permission{ID,Code,Name,GroupName,Description,SortOrder,CreatedAt}`。
  - `rbac.RolePermission{ID,RoleID,PermissionID}` 唯一 `(role_id,permission_id)`。
  - `rbac.UserRole{ID,UserID,RoleID,AssignedBy *int64,AssignedAt}` 唯一 `(user_id,role_id)`。
  - `rbac.AdminAuditLog{ID,ActorID,Action,TargetType string,TargetID *int64,Summary,IP,UserAgent string,CreatedAt time.Time}`。
  - `rbac.Meta{Key,Value string}`（`rbac_meta`）。
  - `rbac.PermissionCodes` 常量/目录与 `AllPermissions() []PermissionDef`。
  - `rbac.Seed(ctx, db *gorm.DB) error`：首次 seed、映射旧角色。
  - `main.go` 在 `db.Migrate` 后调用 `rbac.Seed`。

**权限目录（代码常量）：**

```text
admin.dashboard.view, admin.audit.view, system.settings.manage
user.read, user.update, user.ban, user.coins.adjust, user.roles.assign
role.read, role.create, role.update, role.delete, role.permissions.update
problem.view_hidden, problem.create, problem.update, problem.delete, problem.publish,
problem.testcase.manage, problem.checker.manage
submission.view_live_contest
contest.view_hidden, contest.create, contest.update, contest.delete,
contest.participant.read, contest.announcement.manage
gamification.achievement.manage, gamification.task.manage,
gamification.badge.manage, gamification.title.manage, gamification.config.manage
learning_path.official.manage, learning_path.admin.read
```

**内置角色权限：**

- `super_admin`：system，绕过；Seed 时可不插权限行。
- `content_admin`：dashboard + problem.*（含 view_hidden/publish/testcase/checker）+ learning_path.official.manage/admin.read。
- `contest_admin`：dashboard + contest.* + submission.view_live_contest。
- `gamification_admin`：dashboard + gamification.*。
- `user_admin`：dashboard + user.read/update/ban/coins.adjust + role.read。
- `security_admin`：dashboard + audit + user.read + role.* + user.roles.assign。
- `auditor`：dashboard + audit + user.read + role.read。

- [ ] **Step 1: 写失败 Seed/迁移测试**

```go
func TestSeedCreatesCatalogAndRoles(t *testing.T) {
    // rbac.Seed -> permissions 数量/role codes/role_permissions 非空
}
func TestSeedMapsLegacyRoles(t *testing.T) {
    // super_admin -> super_admin role; problem_admin -> content_admin; contest_admin -> contest_admin; user -> none
}
func TestSeedIsIdempotent(t *testing.T) {
    // Seed twice -> no duplicate roles/permissions/user_roles; meta.seed_version == current
}
func TestSeedDoesNotRestoreRemovedPermission(t *testing.T) {
    // Seed -> delete a content_admin role_permission -> Seed again -> still missing
}
func TestMigrateCreatesRBACTables(t *testing.T)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ -run 'TestSeed|TestMigrate' -v`
Expected: FAIL（包/类型不存在）。

- [ ] **Step 3: 实现模型、目录、迁移与 Seed**

- `db.Migrate` 追加 Role/Permission/RolePermission/UserRole/AdminAuditLog/Meta。
- `rbac_meta` 主键 `key`；首次无 `seed_version` 时执行完整 seed，写 `seed_version=1`。
- Seed 事务：
  1. 插入缺失 permissions（按 code）。
  2. 插入内置 roles（按 code）。
  3. 首次 seed 时为内置角色插入 role_permissions；`super_admin` 不插（或插全部但 Access 仍 bypass）。
  4. 按 `users.role` 插入 user_roles：
     - super_admin→super_admin
     - problem_admin→content_admin
     - contest_admin→contest_admin
     - user→跳过
     `ON CONFLICT DO NOTHING`。
  5. 写 meta。
- 已存在 `seed_version` 时只确保 permissions/roles 存在（不重加 role_permissions），保证管理员删除的权限不被恢复。
- 名称/分组中文。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/rbac internal/db cmd/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/db/ -v`
Expected: PASS。

```bash
git add apps/server/internal/rbac apps/server/internal/db apps/server/cmd/server
git commit -m "feat(server): rbac models and seed"
```

---

### Task 2: Access 解析、权限中间件与 Me 权限

**Files:**
- Create: `apps/server/internal/rbac/access.go`
- Create: `apps/server/internal/rbac/access_test.go`
- Create: `apps/server/internal/middleware/permission.go`
- Create: `apps/server/internal/middleware/permission_test.go`
- Modify: `apps/server/internal/user/dto.go`
- Modify: `apps/server/internal/userapi/handler.go`
- Modify: `apps/server/internal/userapi/handler_test.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/userapi/handler.go`（Options）
- Modify: `packages/api/openapi.yaml`（Me 字段）
- Regenerate: `apps/web/src/api/schema.d.ts`
- Test: 上述测试文件

**Interfaces:**
- Consumes: Task 1 模型/Seed。
- Produces:
  - `rbac.RoleSummary{ID,Code,Name}`。
  - `rbac.Access{UserID int64; IsSuper bool; Roles []RoleSummary; Permissions map[string]struct{}}`。
  - `rbac.Service`/`NewService(db)`，`Access(ctx,userID) (*Access,error)`。
  - `rbac.ErrInactive`（封禁/不存在）。
  - `middleware.PermissionResolver` 接口；`RequirePermission(resolver, codes ...string) gin.HandlerFunc`；`CurrentAccess(c)`。
  - `user.MeDTO` 增加 `Roles []RoleSummary`、`Permissions []string`、`IsSuperAdmin bool`；`user.RoleSummary{ID,Code,Name}`。
  - `userapi.Options{RBAC *rbac.Service}`；`GET /users/me` 填充权限。
  - Router `Deps.RBAC *rbac.Service` optional。

- [ ] **Step 1: 写失败 Access/中间件测试**

```go
func TestAccessUnionRoles(t *testing.T)
// user 两个角色：problem.view_hidden + contest.view_hidden -> 都有；Roles 两条
func TestAccessSuperAdmin(t *testing.T)
// super_admin role -> IsSuper true（即使 role_permissions 为空）
func TestAccessBannedOrMissing(t *testing.T)
// status=banned -> ErrInactive；不存在 -> ErrInactive/ErrNotFound
func TestPermissionMiddleware(t *testing.T)
// 无 token 401；无权限 403；有权限 200；RequireAnyPermission 任一通过；super 通过
func TestMeIncludesPermissions(t *testing.T)
// GET /users/me 返回 roles/permissions/isSuperAdmin；无角色为空数组
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/middleware/ ./internal/userapi/ -run 'TestAccess|TestPermission|TestMeIncludes' -v`
Expected: FAIL。

- [ ] **Step 3: 实现 Access 与中间件**

- `Access` 查询：
  - `users.status` 必须 active；否则 `ErrInactive`。
  - join user_roles/roles；若含 code `super_admin`，`IsSuper=true`。
  - join role_permissions/permissions 去重；`Permissions` 非 nil（无权限为空 map）。
  - 每次请求实时查询，无缓存。
- `RequirePermission`：
  - 未登录 → 401；
  - resolver 错误/封禁 → 403；
  - `IsSuper` 通过；
  - 至少一个 code 在权限集合中通过；
  - 否则 403；
  - 将 Access 放入 context，`CurrentAccess` 读取。
- `RequireAnyPermission` 可用同一函数（codes 任一），单独命名供可读性。
- `userapi`:
  - `Options{RBAC *rbac.Service}`；`getMe` 填 roles/permissions/isSuperAdmin；无 RBAC 时返回空数组/false（保持现有测试兼容）。
  - 登录/注册返回的 `MeDTO` 仍可为空权限；前端登录后调用 `/users/me` 获取，文档注明。
- OpenAPI `Me` 增加字段并生成类型。

- [ ] **Step 4: 运行测试、提交**

Run: `cd apps/server && gofmt -w internal/rbac internal/middleware internal/user internal/userapi internal/router && KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./internal/rbac/ ./internal/middleware/ ./internal/userapi/ ./internal/router/ -v`
`cd /Users/admin/codespace/kapoj && corepack pnpm gen:api && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/server/internal/rbac apps/server/internal/middleware apps/server/internal/user apps/server/internal/userapi apps/server/internal/router packages/api/openapi.yaml apps/web/src/api/schema.d.ts
git commit -m "feat(server): rbac access and permission middleware"
```

---

### Task 3: 文档、全量检查与权限验收

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端、judge、前端

**Interfaces:**
- Consumes: Task 1–2。
- Produces: RBAC-A 状态文档与验证记录。

- [ ] **Step 1: 更新文档**

- `DEVELOPMENT.md` 3.1/4.1：新增 RBAC 实现状态；说明 `users.role` 为 legacy，权限只认 roles；M4/管理后台章节新增权限模型/中间件。
- `docs/任务清单.md`：新增管理后台/RBAC 任务条目（如 M4-8 或独立“平台管理后台”章节），RBAC-A 标 ✅，RBAC-B/C/D ⬜；加计划链接。
- `docs/开发日志.md`：追加 RBAC-A 条目（模型、Seed、旧角色映射、Access、中间件、验证、复审修复）。
- `README.md`、`运行手册.md`：说明权限模型、内置角色、权限码、如何用现有超管账号；管理后台 UI 尚未上线。
- 不修改 `docs/primevue-migration.md`（管理 UI 未做）。

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

- [ ] **Step 3: 权限冒烟**

- 启动 server 后验证 Seed：内置角色/权限/旧账号映射。
- 用 `rbac.Service.Access` 或现有测试账号检查 `m2badmin` 为 super（通过新 `super_admin` role），普通用户无管理权限。
- 验证 `GET /users/me` 返回 roles/permissions/isSuperAdmin。
- 验证权限变更即时性（临时在 DB 加/删 user_role 或 role_permission，再请求 `/users/me`）。
- 不删除现有数据，报告临时变更并恢复。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
git commit -m "docs: record rbac-a permission foundation"
```

---

## Self-Review

**Spec coverage:**

- 模型/迁移/权限目录/内置角色/Seed → Task 1。
- Access/中间件/Me 权限 → Task 2。
- 文档/全量检查/冒烟 → Task 3。
- 角色管理 API/用户角色分配/审计/管理 UI/现有接口迁移 → RBAC-B/C/D。

**Type consistency:**

- `Access.Permissions map[string]struct{}` 与中间件 code 校验一致。
- `MeDTO.Roles []RoleSummary`/`Permissions []string`/`IsSuperAdmin bool` 从 schema 到前端。
- `rbac_meta.seed_version` 控制 seed 一次；`user_roles` 唯一约束防重复。
- `RequirePermission` 支持单权限/任一权限；super bypass。

**Review Focus mapping:**

1. 多角色并集 → Task 2 `TestAccessUnionRoles`。
2. Seed 幂等/不恢复删除权限 → Task 1 tests。
3. 权限即时性 → Task 2/3 DB mutation + request。
4. 封禁 → Task 2 `TestAccessBannedOrMissing`。
5. 中间件 401/403 → Task 2 `TestPermissionMiddleware`。

**Proportion:** 计划固定模型、权限目录、Seed 语义、中间件与测试；具体角色管理 API 留给 RBAC-B。
