# 游客权限与成长板块可见性实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在后台角色管理中增加系统「游客」角色和 `guest.growth.view` 开关；默认游客只看到核心导航，开启后才显示成长板块和首页成长中心，登录用户不受影响。

**Architecture:** 复用 RBAC：新增内置 guest 角色与游客访问权限，角色权限矩阵控制开关；公开 `GET /api/guest/permissions` 返回游客权限码；前端 Pinia guest store + 异步路由守卫控制侧栏、首页成长区块和成长路由访问。

**Tech Stack:** Go + Gin + GORM + PostgreSQL；Vue 3 + Pinia + PrimeVue；OpenAPI 生成类型。

**Spec:** `docs/superpowers/specs/2026-11-12-guest-permissions-design.md`

## Global Constraints

- guest 角色 `code='guest'`，系统角色，默认无权限，不可删除、名称不可改、不可分配给用户。
- 权限 `guest.growth.view` 仅允许配置给 guest 角色；guest 角色只允许配置游客访问权限。
- 默认游客只看到核心导航；首页不显示成长中心；直接访问成长路由到 `/forbidden`。
- 开启后游客看到成长组（成就/任务/徽章商城/称号商店；装扮管理仍仅登录用户）和首页成长中心。
- 登录用户完全不受游客开关影响。
- 游客访问接口公开；前端 store 加载失败视为空权限（默认隐藏）。
- 不改变普通用户/管理员角色权限，不新增数据表。
- 现有 Seed/角色数量/权限目录测试需同步更新（7→8 角色，38→39 权限）。
- 后端测试 `go test -p 1`；前端 test/lint/build。
- 不提交 `Export.zip` 或 `.superpowers` 临时文件。

## Review Focus

- guest 角色默认无权限、Seed 幂等、不会恢复管理员已删除的权限（Task 1）。
- 游客角色/游客权限隔离：不能给用户分配 guest，不能给 guest 配管理员权限，不能给普通角色配 guest 权限（Task 1）。
- 公开接口在未登录下可访问，空权限默认隐藏；rbac 未装配时返回空数组（Task 2）。
- 路由守卫异步加载游客权限，失败默认拒绝；登录用户跳过；直接 URL 被拦截（Task 3）。
- 侧栏/首页成长区块使用同一开关，默认隐藏，开启后显示且不暴露装扮管理（Task 3）。
- 后台游客角色只展示游客权限组，用户角色分配不出现游客（Task 4）。

---

### Task 1: 后端 guest 角色、权限与隔离规则

**Files:**
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/roles.go`
- Modify: `apps/server/internal/rbac/users.go`
- Test: `apps/server/internal/rbac/seed_test.go`
- Test: `apps/server/internal/rbac/roles_test.go`
- Test: `apps/server/internal/rbac/users_test.go`
- Test: `apps/server/internal/adminapi/role_handler_test.go`

**Interfaces:**
- Produces:
  - `rbac.RoleCodeGuest = "guest"`
  - `rbac.PermissionGroupGuest = "游客访问"`
  - `rbac.PermGuestGrowthView = "guest.growth.view"`
  - `func (s *Service) GuestPermissions(ctx context.Context) ([]string, error)`
  - `ReplaceRolePermissions` / `CreateRole` / `ReplaceUserRoles` guest isolation rules.

- [ ] **Step 1: Write failing tests**

`rbac/seed_test.go`：

- builtinRoleCodes 加 `guest`；期望角色数 8。
- `PermissionCodes` 长度 39，最后一项 `PermGuestGrowthView`。
- guest 角色 isSystem=true，默认无 role_permissions。
- 启动 Seed 两次仍为 8 角色/39 权限，guest 默认仍无权限。

`rbac/roles_test.go`：

- `ListRoles` 返回 8，含 guest，userCount 0。
- `GuestPermissions` 默认空；给 guest 添加 `guest.growth.view` 后返回该码。
- `ReplaceRolePermissions`：
  - guest + `guest.growth.view` 成功；
  - guest + `user.read` → 40001；
  - 普通角色 + `guest.growth.view` → 40001；
  - 普通角色原行为不变。
- `CreateRole` 携带 `guest.growth.view` → 40001。

`rbac/users_test.go`：

- `ReplaceUserRoles` 给用户分配 guest 角色 → 40001，用户角色不变。
- 正常角色分配仍成功。

`adminapi/role_handler_test.go`：

- 角色列表长度为 8 且包含 guest；
- guest 角色权限接口只接受游客权限；
- 用户角色分配接口拒绝 guest。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/rbac/ ./internal/adminapi/
```

Expected: FAIL，guest 角色/权限/GuestPermissions 未定义。

- [ ] **Step 3: Implement**

`catalog.go`：

```go
const (
    PermissionGroupGuest = "游客访问"
    PermGuestGrowthView PermissionCode = "guest.growth.view"
    RoleCodeGuest = "guest"
)

{Code: PermGuestGrowthView, Name: "游客可见成长板块", GroupName: PermissionGroupGuest,
 Description: "允许未登录访客查看成就、任务、徽章商城与称号商店"},

{
    Code: RoleCodeGuest, Name: "游客",
    Description: "未登录访客的公开可见范围；不可分配给用户",
},
```

- guest 的 `Permissions` 留空。
- 权限目录追加在最后，保持现有权限索引不变。

`roles.go`：

```go
func (s *Service) GuestPermissions(ctx context.Context) ([]string, error)
```

- 查 guest 角色；不存在返回 `[]string{}`。
- join `role_permissions` + `permissions`，按 code 排序；空结果返回非 nil 空切片。

`ReplaceRolePermissions`：

- 在加载 role 后：
  ```go
  if role.Code == RoleCodeGuest {
      for _, code := range normalized {
          if code != string(PermGuestGrowthView) {
              return invalidRole("游客角色只能配置游客访问权限")
          }
      }
  } else {
      for _, code := range normalized {
          if code == string(PermGuestGrowthView) {
              return invalidRole("游客访问权限只能配置给游客角色")
          }
      }
  }
  ```

`CreateRole`：

- 在 normalize 后拒绝任何 `PermGuestGrowthView`。

`users.go` `ReplaceUserRoles`：

- `roleCodesForIDs` 得到 newCodes 后：
  ```go
  if containsString(newCodes, RoleCodeGuest) {
      return invalidUser("游客角色不可分配给用户")
  }
  ```

- 不要修改 `Access`；guest 不是用户角色。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/rbac/ ./internal/adminapi/ ./internal/db/ ./internal/router/
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/rbac apps/server/internal/adminapi/role_handler_test.go
git commit -m "feat(rbac): add guest role and growth visibility permission"
```

---

### Task 2: 公开游客权限接口、OpenAPI 与客户端

**Files:**
- Modify: `apps/server/internal/userapi/handler.go`
- Test: `apps/server/internal/userapi/handler_test.go`
- Modify: `packages/api/openapi.yaml`
- Generated: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/api/guest.ts`
- Create: `apps/web/src/api/guest.spec.ts`

**Interfaces:**
- Produces:
  - `GET /api/guest/permissions` public `{ permissions: string[] }`
  - `getGuestPermissions(): Promise<GuestPermissions>`
  - `GuestPermissions` generated type.

- [ ] **Step 1: Write failing tests**

`userapi/handler_test.go`：

- 无 rbac 依赖时 `GET /api/guest/permissions` 200 `{permissions: []}`。
- 注入 fake RBAC/guest lookup 返回 `["guest.growth.view"]` 时 200。
- 游客 token 为空可访问；错误时 500。

`api/guest.spec.ts`：

- `getGuestPermissions()` → GET `/guest/permissions`，返回 `{permissions: []}`。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/userapi/
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/guest.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement**

`userapi/handler.go`：

```go
if h.rbac != nil {
    r.GET("/guest/permissions", h.guestPermissions)
}
```

或始终注册，`rbac == nil` 返回空数组。Handler：

```go
func (h *handler) guestPermissions(c *gin.Context) {
    codes := []string{}
    if h.rbac != nil {
        result, err := h.rbac.GuestPermissions(c.Request.Context())
        if err != nil { httpx.Fail(c, err); return }
        codes = result
    }
    httpx.OK(c, gin.H{"permissions": codes})
}
```

- `GuestPermissions` 结果为空时确保 `[]` 而不是 `null`。

OpenAPI：

- `GET /guest/permissions` 公开，无 security 或 `[{}, { bearer: [] }]`。
- `GuestPermissions` schema：`permissions: string[]`，required。

前端：

```ts
export interface GuestPermissions { permissions: string[] }
export const getGuestPermissions = () => http.get<GuestPermissions>('/guest/permissions')
```

- `types.ts` 增加 `GuestPermissions` 别名。
- `pnpm gen:api`。

- [ ] **Step 4: Run tests/typecheck**

```bash
cd /Users/admin/codespace/kapoj/apps/server
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./internal/userapi/
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/api/guest.spec.ts
corepack pnpm --filter web exec vue-tsc -b --force
```

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/userapi/handler.go apps/server/internal/userapi/handler_test.go packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts apps/web/src/api/guest.ts apps/web/src/api/guest.spec.ts
git commit -m "feat(api): expose guest permissions"
```

---

### Task 3: 前端 guest store、路由守卫、侧栏与首页

**Files:**
- Modify: `apps/web/src/permissions.ts`
- Create: `apps/web/src/stores/guest.ts`
- Create: `apps/web/src/stores/guest.spec.ts`
- Modify: `apps/web/src/router/guard.ts`
- Modify: `apps/web/src/router/guard.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AppNav.vue`
- Modify: `apps/web/src/components/layout/AppNav.spec.ts`
- Modify: `apps/web/src/views/HomeView.vue`
- Modify: `apps/web/src/views/HomeView.spec.ts`

**Interfaces:**
- Consumes: `getGuestPermissions()`
- Produces:
  - `GUEST_PERM.growth = 'guest.growth.view'`
  - `useGuestStore()` with `permissions`, `loaded`, `ensureLoaded`, `can`
  - `guestPermission` route meta
  - guest-aware nav/home.

- [ ] **Step 1: Write failing tests**

`guest.spec.ts`：

- 初始空/未加载。
- `ensureLoaded` 调一次 API；并发调用共享同一个 Promise。
- 成功保存 permissions；失败标记 loaded 且保持空。
- `can` 正确。

`guard.spec.ts`：

- 游客无 guest 权限访问 `guestPermission` 路由 → forbidden。
- 游客有权限 → true。
- 登录用户无 guest 权限仍 true。
- `guestPermissions` 缺省 → forbidden（默认关闭）。

`AppNav.spec.ts`：

- 默认游客：只有核心组，无成长。
- 游客 + guest.growth.view：有成长组，无装扮管理。
- 登录用户：成长组含装扮管理，管理员入口逻辑不变。
- mock `useGuestStore` 和 auth `isLoggedIn`。

`HomeView.spec.ts`：

- 默认游客：无成长中心链接。
- 游客 + guest.growth.view：显示成长链接，不含 `/appearance`。
- 登录用户：显示成长链接 + 装扮管理。
- mock guest store。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/stores/guest.spec.ts src/router/guard.spec.ts src/components/layout/AppNav.spec.ts src/views/HomeView.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement**

`permissions.ts`：

```ts
export const GUEST_PERM = {
  growth: 'guest.growth.view',
} as const
```

`stores/guest.ts`：

- `permissions = ref<string[]>([])`, `loaded = ref(false)`, `inFlight`.
- `ensureLoaded`：已加载直接返回；in-flight 复用；成功设置；失败设置 `[]` 并 loaded；finally 清 in-flight。
- `can(code)`。

`router/guard.ts`：

- `AuthState` 增加 `guestPermissions?: string[]`。
- 在 guestOnly 之后：
  ```ts
  const guestPermission = typeof to.meta.guestPermission === 'string' ? to.meta.guestPermission : ''
  if (!state.isLoggedIn && guestPermission && !(state.guestPermissions ?? []).includes(guestPermission)) {
      return { name: 'forbidden' }
  }
  ```

`router/index.ts`：

- 导入 `GUEST_PERM`、`useGuestStore`。
- `RouteMeta` 增加 `guestPermission?: string`。
- 给 achievements/tasks/badge-shop/title-shop meta 加 `guestPermission: GUEST_PERM.growth`。
- `beforeEach` 改 async：
  ```ts
  router.beforeEach(async (to) => {
    const auth = useAuthStore()
    const guest = useGuestStore()
    if (!auth.isLoggedIn) await guest.ensureLoaded()
    return authGuard(to, {
      isLoggedIn: auth.isLoggedIn,
      permissions: auth.user?.permissions ?? [],
      isSuperAdmin: auth.isSuperAdmin,
      guestPermissions: guest.permissions,
    })
  })
  ```

`AppNav.vue`：

- import `GUEST_PERM`, `useGuestStore`.
- `growthVisible` computed; guest growth items exclude appearance.
- `groups` includes成长 only when visible.
- 模板可保持 `v-for groups`.

`HomeView.vue`：

- import `GUEST_PERM`, `useGuestStore`.
- `visibleGrowthLinks`：登录用户 = growthLinks + appearance；游客 = 有权限 ? growthLinks : []。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/stores/guest.spec.ts src/router/guard.spec.ts src/components/layout/AppNav.spec.ts src/views/HomeView.spec.ts
corepack pnpm --filter web test
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/permissions.ts apps/web/src/stores/guest.ts apps/web/src/stores/guest.spec.ts apps/web/src/router/guard.ts apps/web/src/router/guard.spec.ts apps/web/src/router/index.ts apps/web/src/components/layout/AppNav.vue apps/web/src/components/layout/AppNav.spec.ts apps/web/src/views/HomeView.vue apps/web/src/views/HomeView.spec.ts
git commit -m "feat(web): gate guest growth navigation by permission"
```

---

### Task 4: 后台游客角色矩阵与用户分配过滤

**Files:**
- Modify: `apps/web/src/views/AdminRolesView.vue`
- Modify: `apps/web/src/views/AdminRolesView.spec.ts`
- Modify: `apps/web/src/views/AdminUsersView.vue`
- Modify: `apps/web/src/views/AdminUsersView.spec.ts`

**Interfaces:**
- Consumes: guest role in `listAdminRoles`, `游客访问` permission group in `listAdminPermissions`.
- Produces:
  - guest role matrix only `游客访问` group.
  - non-guest matrix excludes `游客访问`.
  - guest hint.
  - assign options exclude guest.

- [ ] **Step 1: Write failing tests**

`AdminRolesView.spec.ts`：

- `directory()` 增加 `guest.growth.view`（组「游客访问」）。
- 角色 fixture 含 guest system 角色。
- 打开 guest 编辑：groupNames 只有 `['游客访问']`，hint 文案存在。
- 勾选 `guest.growth.view` 保存 → `replaceAdminRolePermissions(guestId, ['guest.growth.view'])`。
- 打开普通角色编辑：groupNames 不含 `游客访问`。
- guest 行无删除按钮；权限按钮存在。

`AdminUsersView.spec.ts`：

- roles fixture 含 guest；角色分配选项不包含游客。
- 角色筛选仍可包含游客（如有对应测试）。
- 现有分配/筛选测试不回归。

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/AdminRolesView.spec.ts src/views/AdminUsersView.spec.ts
```

Expected: FAIL。

- [ ] **Step 3: Implement**

`AdminRolesView.vue`：

- `const isGuestRole = computed(() => editingRole.value?.code === 'guest')`
- `const dialogPermissionGroups = computed(() => isGuestRole.value
    ? permissionGroups.value.filter(g => g.groupName === '游客访问')
    : permissionGroups.value.filter(g => g.groupName !== '游客访问'))`
- 模板 `v-for="group in dialogPermissionGroups"`。
- 在权限矩阵上方显示 guest 提示：
  「游客默认只能看到核心导航；开启后额外显示成长板块（装扮管理仍需登录）。」
- 保存 guest 权限走现有 `replaceAdminRolePermissions`；不需要额外 store 刷新。

`AdminUsersView.vue`：

```ts
const roleAssignOptions = computed(() =>
  roles.value
    .filter((role) => role.code !== 'guest')
    .map((role) => ({ ... })),
)
```

- `roleFilterOptions` 保留全部角色。

- [ ] **Step 4: Run tests**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm --filter web exec vitest run src/views/AdminRolesView.spec.ts src/views/AdminUsersView.spec.ts
corepack pnpm --filter web test
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/views/AdminRolesView.vue apps/web/src/views/AdminRolesView.spec.ts apps/web/src/views/AdminUsersView.vue apps/web/src/views/AdminUsersView.spec.ts
git commit -m "feat(web): add guest permission controls"
```

---

### Task 5: 全量验证、真实冒烟与文档

**Files:**
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`（当前进度一行，可选）

- [ ] **Step 1: Backend checks**

```bash
cd /Users/admin/codespace/kapoj/apps/server
test -z "$(gofmt -l .)"
go vet ./...
/private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
  go test -p 1 -count=1 ./...
```

- [ ] **Step 2: Frontend checks**

```bash
cd /Users/admin/codespace/kapoj
corepack pnpm gen:api
corepack pnpm --filter web test
corepack pnpm --filter web lint
corepack pnpm --filter web build
```

- [ ] **Step 3: Real API smoke**

隔离临时库/端口：

- 新库启动：角色 8、权限 39；guest 角色存在且无权限。
- 游客 `GET /api/guest/permissions` → `{permissions: []}`。
- 用 super 管理员 `PUT /api/admin/roles/:guestId/permissions` `["guest.growth.view"]`。
- 游客再取 → `["guest.growth.view"]`。
- 尝试给普通角色配 `guest.growth.view` → 40001。
- 尝试给用户分配 guest → 40001。
- 清理临时服务/DB/Redis/storage。

- [ ] **Step 4: Browser smoke**

- 默认游客：侧栏只有核心；首页无成长中心；直接访问 `/achievements` → `/forbidden`。
- 管理员在角色管理打开游客「成长板块」并保存。
- 新游客会话：侧栏出现成长（无装扮管理）；首页出现成长中心；`/achievements` 可访问。
- 登录普通用户：成长全量不受影响。
- 375px/1440px 无横向溢出。
- 若浏览器不可用，记录延后，不声称已验证。

- [ ] **Step 5: Update docs**

- `docs/任务清单.md`：新增「跨里程碑：游客可见性与成长板块权限 ✅」，记录交付摘要/验证/取舍。
- `docs/开发日志.md`：追加 `2026-11-12：游客可见性与成长板块权限`。
- README 当前进度可选补一行。

- [ ] **Step 6: Commit**

```bash
git add docs/任务清单.md docs/开发日志.md README.md
git commit -m "docs: record guest permission delivery"
```

---

## Self-Review

- **Spec coverage:** guest role/permission/isolation（Task 1）、public endpoint/contract（Task 2）、guest store/guard/nav/home（Task 3）、admin matrix/assignment（Task 4）、verification/docs（Task 5）。
- **Type consistency:** `RoleCodeGuest`、`PermGuestGrowthView`、`GUEST_PERM.growth`、`GuestPermissions`、`guestPermission` 在任务间一致。
- **Review Focus mapping:** seed/default/isolation（Task 1）、public endpoint（Task 2）、guard/nav/home（Task 3）、admin UI（Task 4）。
- **Proportion:** 任务按可独立验收的交付物拆分；计划给出签名、默认值、权限规则和测试断言。
