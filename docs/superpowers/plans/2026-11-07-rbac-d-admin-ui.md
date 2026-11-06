# RBAC-D 管理后台前端 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付统一 `/admin` 管理后台前端：权限驱动菜单/路由/按钮、仪表盘、用户管理、角色权限矩阵、审计日志，并把现有游戏化/题库/比赛入口迁移到 RBAC 权限判断。

**Architecture:** 前端新增集中权限常量与 `auth.can/canAny`；路由 meta 改为 `permission`/`anyPermission`；`AdminLayout` 提供独立管理壳；管理页面只通过 `/api/admin/*` 和现有内容 API 操作。登录/注册/资料保存后刷新或保留 RBAC 字段，避免权限被空值覆盖。后端无新增接口。

**Tech Stack:** Vue 3、TypeScript、Pinia、Vue Router 5、PrimeVue 4、Vitest、Vite。

**Spec:** `docs/superpowers/specs/2026-11-06-admin-rbac-design.md` 8.3、12、14.2、14.3。

## Global Constraints

- 前端所有菜单、路由、按钮按权限显示；后端仍是最终授权边界。
- `super_admin` 前端视为拥有全部权限。
- 权限码使用后端稳定目录，不在前端自定义新权限。
- `users.role`/JWT role 仅展示兼容；前端授权不再读取 `user.role`。
- 登录/注册后必须立即获取 `/users/me` 的 roles/permissions/isSuperAdmin；资料保存不得用空 RBAC 字段覆盖当前权限。
- 管理页面使用 PrimeVue 4 与 `AppSurface`/`AppPageHeader`/`AppEmptyState`/`AppErrorState`/`AppLoadingState`。
- 管理壳独立于 `DefaultLayout`；移动端使用 Drawer。
- 所有用户可见文案中文；不访问 CDN。
- 不新增后端接口；不修改 OpenAPI（除非发现契约错误，需单独说明）。
- 不推送、不合并分支。

## Review Focus

1. **权限即时性**：登录后、资料保存后、刷新后，`auth.user.permissions` 不被清空；超级管理员始终通过；测试放 Task 1。
2. **路由边界**：无权限直接访问 `/admin/users`、`/admin/roles`、`/admin/audit`、`/admin/gamification` 到 403；测试放各页面/guard 任务。
3. **按钮最小权限**：只读审计员看不到编辑/封禁/调币按钮；只有单一游戏化权限时只显示对应 tab；测试放 Task 1/3/4/5。
4. **敏感操作 UX**：角色权限矩阵系统角色保护、最后 super/自封禁后端错误能正确展示；金币/封禁/角色变更后刷新列表；测试放 Task 3/4。
5. **详情资料完整性**：管理员编辑用户时昵称/简介/头像都能预填并保存，不因 AdminUserItem 缺少 bio/avatar 而丢字段；测试放 Task 3。

---

### Task 1: 权限模型、路由守卫与现有管理 UI 迁移

**Files:**
- Create: `apps/web/src/permissions.ts`
- Create: `apps/web/src/permissions.spec.ts`
- Modify: `apps/web/src/stores/auth.ts`
- Modify: `apps/web/src/stores/auth.spec.ts`
- Modify: `apps/web/src/router/guard.ts`
- Modify: `apps/web/src/router/guard.spec.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AppNav.vue`
- Modify: `apps/web/src/components/layout/AppNav.spec.ts`
- Modify: `apps/web/src/views/ProblemListView.vue`
- Modify: `apps/web/src/views/ProblemListView.spec.ts`
- Modify: `apps/web/src/views/ProblemDetailView.vue`
- Modify: `apps/web/src/views/ProblemDetailView.spec.ts`
- Modify: `apps/web/src/views/ContestListView.vue`
- Modify: `apps/web/src/views/ContestListView.spec.ts`
- Modify: `apps/web/src/views/ContestDetailView.vue`
- Modify: `apps/web/src/views/ContestDetailView.spec.ts`
- Modify: `apps/web/src/views/GamificationAdminView.vue`
- Modify: `apps/web/src/views/GamificationAdminView.spec.ts`
- Modify: `apps/web/src/views/ProfileEditView.vue`
- Modify: `apps/web/src/views/ProfileEditView.spec.ts`

**Interfaces:**
- Consumes: 后端 `Me.permissions`、`Me.isSuperAdmin`、现有 `/users/me`。
- Produces:
  - `PERM` 常量对象、`GAMIFICATION_MANAGE_PERMISSIONS`、`PROBLEM_MANAGE_PERMISSIONS`、`CONTEST_MANAGE_PERMISSIONS`、`ADMIN_ACCESS_PERMISSIONS`。
  - `auth.can(code: string): boolean`、`auth.canAny(codes: readonly string[]): boolean`、`auth.isSuperAdmin` computed。
  - `auth.updateProfile(input: UpdateMeRequest): Promise<void>`：保存资料并保留当前 RBAC 字段。
  - `AuthState{isLoggedIn:boolean; permissions:string[]; isSuperAdmin:boolean}`。
  - `RouteMeta.permission?: string`、`RouteMeta.anyPermission?: string[]`；移除 `roles`。
  - 现有路由 meta：
    - `problems/new` → `problem.create`
    - `problems/:id/edit` → `problem.update`
    - `contests/new` → `contest.create`
    - `contests/:id/edit` → `contest.update`
    - `admin/gamification` → `anyPermission: GAMIFICATION_MANAGE_PERMISSIONS`
  - `AppNav` 管理入口用 `auth.canAny(GAMIFICATION_MANAGE_PERMISSIONS)`，暂时仍链接 `/admin/gamification`（Task 2 改为 `/admin`）。
  - 题库/比赛/游戏化页面按钮与 tab 全部改用 `auth.can`/`auth.canAny`。

- [ ] **Step 1: 写失败测试**

```ts
// permissions.spec.ts / auth.spec.ts
it('can/canAny respects permissions and super admin')
it('login/register refresh Me after session')
it('updateProfile preserves roles/permissions/isSuperAdmin')

// guard.spec.ts
it('permission route requires login then checks permission')
it('anyPermission allows one match; super passes')
it('no permission redirects to forbidden')
it('removes role-based authorization')

// ProblemListView.spec.ts
it('problem.create shows create; problem.view_hidden shows hidden toggle')
it('contest participant/view_hidden cannot see create button')

// ContestListView.spec.ts
it('splits create/edit/delete/clone visibility by permission')
it('contest.view_hidden only controls hidden filter')

// ContestDetailView.spec.ts
it('participant.read/edit/clone/delete/announcement buttons use separate permissions')
it('contest.update permits unregistered test submit UI; view_hidden does not')

// GamificationAdminView.spec.ts
it('shows only permitted domain tabs')
it('no gamification permission shows no-permission state')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/permissions.spec.ts src/stores/auth.spec.ts src/router/guard.spec.ts src/views/ProblemListView.spec.ts src/views/ContestListView.spec.ts src/views/ContestDetailView.spec.ts src/views/GamificationAdminView.spec.ts`
Expected: FAIL（新常量/方法不存在，旧测试仍按 roles）。

- [ ] **Step 3: 实现权限基础与现有 UI 迁移**

- `permissions.ts` 定义全部前端使用的权限码与分组数组；`ADMIN_ACCESS_PERMISSIONS` 覆盖所有管理/内容/游戏化/学习路线权限码。
- `auth` store：
  - `isSuperAdmin = computed(() => user.value?.isSuperAdmin === true)`
  - `can(code)`：super 或 `permissions.includes(code)`；旧缓存无 permissions 时按空数组。
  - `canAny(codes)`：super 或任一命中。
  - `login/register` 在 `setSession` 后 `await fetchMe()`（失败不阻断登录，但保留 session）。
  - `updateProfile` 调 `usersApi.updateMe`，合并当前 `roles/permissions/isSuperAdmin` 后 `setUser`。
- `guard` 用 `permission`/`anyPermission`；未登录先 login redirect；已登录无权限 → forbidden；super 直接通过。`hasRole` 删除。
- Router meta 完成映射；`admin/gamification` 用游戏化 any 权限。
- `AppNav` 管理入口改为 `auth.canAny(GAMIFICATION_MANAGE_PERMISSIONS)`。
- 题库：`ProblemListView` 新建用 `problem.create`，显示未公开用 `problem.view_hidden`，请求 `includeHidden` 只由 view_hidden 决定；`ProblemDetailView` 编辑用 `problem.update`。
- 比赛：`ContestListView` 拆分 `canViewHidden/canCreate/canEdit/canDelete`；`ContestDetailView` 使用 `contest.participant.read`、`contest.update`、`contest.create`、`contest.delete`、`contest.announcement.manage`，免报名测试提交 UI 用 `contest.update`。
- 游戏化：`GamificationAdminView` 按四个 manage 权限生成 tabs；无权限显示空状态；不再判断 `super_admin`。
- `ProfileEditView` 使用 `auth.updateProfile`，不再 `setUser(updateMe(...))`。保留现有表单校验与 toast。
- 所有测试 fixture 的 `Me` 使用 permissions/isSuperAdmin 驱动，不再用 `role` 断言前端授权。

- [ ] **Step 4: 运行测试、lint、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/permissions.ts apps/web/src/stores apps/web/src/router apps/web/src/components/layout/AppNav.vue apps/web/src/components/layout/AppNav.spec.ts apps/web/src/views/ProblemListView.vue apps/web/src/views/ProblemListView.spec.ts apps/web/src/views/ProblemDetailView.vue apps/web/src/views/ProblemDetailView.spec.ts apps/web/src/views/ContestListView.vue apps/web/src/views/ContestListView.spec.ts apps/web/src/views/ContestDetailView.vue apps/web/src/views/ContestDetailView.spec.ts apps/web/src/views/GamificationAdminView.vue apps/web/src/views/GamificationAdminView.spec.ts apps/web/src/views/ProfileEditView.vue apps/web/src/views/ProfileEditView.spec.ts
git commit -m "feat(web): add permission-driven auth and navigation"
```

---

### Task 2: 管理壳、管理导航与仪表盘

**Files:**
- Create: `apps/web/src/layouts/AdminLayout.vue`
- Create: `apps/web/src/components/layout/AdminNav.vue`
- Create: `apps/web/src/views/AdminDashboardView.vue`
- Create: `apps/web/src/layouts/AdminLayout.spec.ts`
- Create: `apps/web/src/components/layout/AdminNav.spec.ts`
- Create: `apps/web/src/views/AdminDashboardView.spec.ts`
- Create: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/components/layout/AppNav.vue`
- Modify: `apps/web/src/components/layout/AppNav.spec.ts`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/components/UserMenu.spec.ts`

**Interfaces:**
- Consumes: Task 1 权限基础、`/admin/stats`、`AdminStats`/`AdminAuditItem` schema。
- Produces:
  - `api/admin.ts` 的 `getAdminStats(): Promise<AdminStats>`。
  - `AdminLayout.vue`：侧栏 + 顶部标题/主题/用户菜单 + 移动 Drawer；无 `DefaultLayout` 依赖。
  - `AdminNav.vue`：概览、游戏化管理（任一 `gamification.*.manage`）、内容管理链接（任一 problem/contest manage）、返回站点。
  - `/admin` 父路由（`requiresAuth` + `anyPermission: ADMIN_ACCESS_PERMISSIONS`）；子路由：
    - `''` → `admin-dashboard`（AdminDashboardView）
    - `'gamification'` → `gamification-admin`（GamificationAdminView，any 游戏化权限）
  - `AppNav` 管理入口改为“管理后台” → `/admin`，显示条件改为 `auth.canAny(ADMIN_ACCESS_PERMISSIONS)`。
  - `UserMenu` 增加“管理后台”菜单项（同一条件）。
  - 仪表盘：有 `admin.dashboard.view` 时请求 stats，显示计数卡片；有 `admin.audit.view` 时显示最近审计；无 dashboard 权限时显示欢迎/快捷入口，不调用 stats。

- [ ] **Step 1: 写失败测试**

```ts
// AdminNav.spec.ts
it('shows overview/gamification/content by permission')
it('hides content links without problem/contest manage permissions')

// AdminLayout.spec.ts
it('renders admin nav and route title')
it('opens mobile drawer with admin nav')

// AdminDashboardView.spec.ts
it('loads stats and recent audit with permissions')
it('without dashboard permission shows existing quick links and does not call stats')
it('without audit permission hides recent audit')

// AppNav.spec.ts
it('shows 管理后台 for any admin permission, not for normal users')

// UserMenu.spec.ts
it('shows 管理后台 menu item for admins')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/components/layout/AdminNav.spec.ts src/layouts/AdminLayout.spec.ts src/views/AdminDashboardView.spec.ts src/components/layout/AppNav.spec.ts src/components/UserMenu.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现管理壳与仪表盘**

- `AdminLayout` 结构复用 `DefaultLayout` 的响应式模式，但侧栏使用 `AdminNav`；顶部标题取 `route.meta.title`；包含 `AppThemeToggle`、`UserMenu`；“返回站点”链接 `/`。
- `AdminNav` 用 `auth.can`/`canAny` 过滤；当前路由 active；移动端可关闭。
- `AdminDashboardView`：
  - `auth.can('admin.dashboard.view')` 才调用 `getAdminStats`。
  - 卡片：用户（总/活跃/封禁）、题目（总/公开）、比赛、提交（总/今日）、角色。
  - 最近审计仅在 `auth.can('admin.audit.view')` 时显示，取 `stats.recentAudit`。
  - 无 dashboard 权限时显示快捷入口：游戏化/题库/比赛，只显示有权限的；用户/角色/审计入口在后续任务页面落地后加入。
- `api/admin.ts` 只加入 stats；后续任务追加。
- 路由移动 `/admin/gamification` 到 AdminLayout 下；删除 DefaultLayout 中的旧路由。保留 name/path 兼容。
- `AppNav` 和 `UserMenu` 管理入口改为 `/admin` + 管理后台文案。
- 所有测试 fixture 用 permissions/isSuperAdmin。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/layouts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/views/AdminDashboardView.vue apps/web/src/views/AdminDashboardView.spec.ts apps/web/src/api/admin.ts apps/web/src/api/types.ts apps/web/src/router/index.ts apps/web/src/components/layout/AppNav.vue apps/web/src/components/layout/AppNav.spec.ts apps/web/src/components/UserMenu.vue apps/web/src/components/UserMenu.spec.ts
git commit -m "feat(web): add admin shell and dashboard"
```

---

### Task 3: 用户管理页面

**Files:**
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/gamification.ts`（移除旧 `adminAdjustCoins`）
- Create: `apps/web/src/views/AdminUsersView.vue`
- Create: `apps/web/src/views/AdminUsersView.spec.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/views/AdminDashboardView.vue`
- Modify: `apps/web/src/views/AdminDashboardView.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Consumes: `/admin/users`、`/admin/users/:id`、`/admin/users/:id/roles`、`/admin/users/:id/ban|unban`、`/admin/users/:id/coins`、`/admin/roles`、`/users/:id/profile`。
- Produces:
  - `api/admin.ts`：
    - `listAdminUsers(q)` → `Page<AdminUserItem>`
    - `getAdminUser(id)` → `AdminUserDetail`
    - `updateAdminUser(id, input)` → `AdminUserDetail`
    - `replaceAdminUserRoles(id, roleIds)` → `AdminUserDetail`
    - `banAdminUser(id, reason)` / `unbanAdminUser(id)`
    - `adjustAdminUserCoins(id, input)` → `AdminCoinAdjustResult`
    - `listAdminRoles()` → `AdminRoleItem[]`（用户角色分配/筛选）
  - `AdminUsersView.vue`：
    - 筛选 keyword/status/roleId；分页。
    - 角色标签、状态、Rating/金币、注册时间。
    - 操作按权限显示：编辑 `user.update`、角色 `user.roles.assign`+`role.read`、金币 `user.coins.adjust`、封禁/解封 `user.ban`。
    - 编辑对话框：`getAdminUser` + `getProfile` 预填 nickname/bio/avatarUrl；保存 `updateAdminUser`。
    - 角色对话框：MultiSelect 角色；角色列表含 `super_admin` 时，非 super 禁用该选项；提交只发 roleIds。
    - 金币对话框：amount≠0、note 1–200；提交后刷新。
    - 封禁：自封禁禁用；目标含 super_admin 且 actor 非 super 禁用；确认后刷新。
  - 路由 `/admin/users`，`permission: user.read`。
  - AdminNav 增加“用户管理”（`user.read`）；Dashboard 增加用户快捷入口（页面已落地）。

- [ ] **Step 1: 写失败测试**

```ts
// AdminUsersView.spec.ts
it('loads users with filters and pagination')
it('read-only auditor sees no edit/role/coin/ban actions')
it('edits nickname/bio/avatar with profile prefill')
it('assigns roles and disables super_admin for non-super actor')
it('adjusts coins with validation and refreshes')
it('bans/unbans with confirm; disables self and super target as needed')
it('shows API error messages (last super, self-ban, forbidden)')

// AdminNav.spec.ts / AdminDashboardView.spec.ts
it('shows users entry/link only with user.read')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminUsersView.spec.ts src/components/layout/AdminNav.spec.ts src/views/AdminDashboardView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现用户管理**

- 新建 `AdminUsersView`，使用 PrimeVue DataTable/Dialog/InputText/Select/MultiSelect/InputNumber/Textarea/Tag/Button/Confirm/Toast。
- 常量 `PAGE_SIZE=20`；筛选点击后 page=1；分页。
- 权限按钮显隐；无动作权限时不显示操作列。
- 编辑对话框使用 `Promise.all([getAdminUser, getProfile])`；保存后重新加载当前页。
- 角色对话框使用 `listAdminRoles`；super 选项 `optionDisabled`；提交后刷新。
- 金币/封禁使用确认对话框；API 错误原样 toast/消息。
- `api/gamification.ts` 移除 `adminAdjustCoins`（已迁移到 admin API）。
- 路由与导航/仪表盘入口。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/api/admin.ts apps/web/src/api/types.ts apps/web/src/api/gamification.ts apps/web/src/views/AdminUsersView.vue apps/web/src/views/AdminUsersView.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/views/AdminDashboardView.vue apps/web/src/views/AdminDashboardView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add admin user management"
```

---

### Task 4: 角色权限矩阵页面

**Files:**
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/views/AdminRolesView.vue`
- Create: `apps/web/src/views/AdminRolesView.spec.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/views/AdminDashboardView.vue`
- Modify: `apps/web/src/views/AdminDashboardView.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Consumes: `/admin/permissions`、`/admin/roles` CRUD、`/admin/roles/:id/permissions`。
- Produces:
  - `api/admin.ts`：
    - `listAdminPermissions()` → `AdminPermissionItem[]`
    - `createAdminRole(input)` / `updateAdminRole(id,input)` / `deleteAdminRole(id)`
    - `replaceAdminRolePermissions(id, permissionCodes)` → `AdminRoleItem`
  - `AdminRolesView.vue`：
    - 角色列表：名称/code/系统标记/权限数/用户数/描述。
    - 新建自定义角色；编辑名称/描述；删除自定义角色；权限矩阵按 `groupName` 分组。
    - 系统角色 code/name 不可编辑；`super_admin` 权限不可编辑并显示“绕过全部权限检查”。
    - 按钮按 `role.create/update/delete/permissions.update` 显隐。
    - 保存：创建 = `POST` 带 permissionCodes；编辑 = `PUT`（name/description）+ 有 `role.permissions.update` 时 `PUT permissions`。
  - 路由 `/admin/roles`，`permission: role.read`。
  - AdminNav 增加“角色权限”（`role.read`）；Dashboard 增加角色快捷入口（页面已落地）。

- [ ] **Step 1: 写失败测试**

```ts
// AdminRolesView.spec.ts
it('groups permissions by group and shows role user counts')
it('creates a custom role with selected permissions')
it('edits custom role name/description and permissions')
it('protects system role code/name and super_admin permissions')
it('deletes only custom roles; read-only user sees no mutation buttons')
it('shows backend errors such as duplicate code / system role delete')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminRolesView.spec.ts src/components/layout/AdminNav.spec.ts src/views/AdminDashboardView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现角色权限页面**

- 使用 DataTable/Dialog/Checkbox/InputText/Textarea/Tag/Button/Confirm/Toast。
- 权限矩阵：`groupBy(permissions, 'groupName')`，保留 API 顺序；每组全选/清空可选；复选框绑定 `permissionCodes` 数组。
- 系统角色：code/name disabled；删除按钮不显示；`super_admin` 权限区 disabled。
- 编辑保存顺序：先 `PUT` 角色信息，成功后再 `PUT permissions`（仅在允许且有变化时）；刷新列表。
- 删除自定义角色二次确认；后端 40001 显示。
- 路由/导航/仪表盘入口。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/api/admin.ts apps/web/src/api/types.ts apps/web/src/views/AdminRolesView.vue apps/web/src/views/AdminRolesView.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/views/AdminDashboardView.vue apps/web/src/views/AdminDashboardView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add admin role permission matrix"
```

---

### Task 5: 审计日志页面

**Files:**
- Modify: `apps/web/src/api/admin.ts`
- Modify: `apps/web/src/api/types.ts`
- Create: `apps/web/src/views/AdminAuditView.vue`
- Create: `apps/web/src/views/AdminAuditView.spec.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/components/layout/AdminNav.spec.ts`
- Modify: `apps/web/src/views/AdminDashboardView.vue`
- Modify: `apps/web/src/views/AdminDashboardView.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Consumes: `/admin/audit`。
- Produces:
  - `api/admin.ts`：`listAdminAudit(q)` → `Page<AdminAuditItem>`；`q` 包含 `actorId?/action?/targetType?/from?/to?/page?/pageSize?`。
  - `AdminAuditView.vue`：筛选 actorId/action/targetType/from/to；分页；展示时间、actor、action、target、summary、IP/UA。
  - 日期使用 `YYYY-MM-DD`（API 语义为闭区间）；`to` 覆盖整天。
  - 路由 `/admin/audit`，`permission: admin.audit.view`。
  - AdminNav 增加“审计日志”（`admin.audit.view`）；Dashboard 增加审计快捷入口（页面已落地）。

- [ ] **Step 1: 写失败测试**

```ts
// AdminAuditView.spec.ts
it('loads audit with actor/action/target/date filters and pagination')
it('formats YYYY-MM-DD from/to and omits empty filters')
it('renders summary/target/IP and empty state')
it('shows API error with retry')
it('no write/edit controls')
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm --filter web test -- src/views/AdminAuditView.spec.ts src/components/layout/AdminNav.spec.ts src/views/AdminDashboardView.spec.ts`
Expected: FAIL。

- [ ] **Step 3: 实现审计页面**

- 使用 DataTable/Column/InputText/Select/DatePicker/Button/Paginator/App*。
- 筛选表单：actorId（正整数）、action、targetType、from/to；提交重置 page=1。
- 查询参数使用 `page_size`；空值不发送。
- 表格列：时间、操作者、action、目标、摘要、IP/UA（UA 可截断 title）。
- 空状态/错误/加载状态；无任何编辑按钮。
- 路由/导航/仪表盘入口。

- [ ] **Step 4: 运行测试、lint、build、提交**

Run: `cd /Users/admin/codespace/kapoj && corepack pnpm test:web && corepack pnpm lint:web && corepack pnpm build:web`
Expected: PASS。

```bash
git add apps/web/src/api/admin.ts apps/web/src/api/types.ts apps/web/src/views/AdminAuditView.vue apps/web/src/views/AdminAuditView.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/components/layout/AdminNav.spec.ts apps/web/src/views/AdminDashboardView.vue apps/web/src/views/AdminDashboardView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add admin audit log"
```

---

### Task 6: 文档、全量检查与浏览器验收

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Modify: `docs/primevue-migration.md`
- Test: 全量后端、judge、前端；真实浏览器验收

**Interfaces:**
- Consumes: Task 1–5。
- Produces: RBAC-D 完成状态、管理后台使用说明、浏览器验收记录。

- [ ] **Step 1: 更新文档**

- `DEVELOPMENT.md`：RBAC-D 完成；`auth.can`、路由 meta、AdminLayout、用户/角色/审计页面；管理后台可用；权限变更后刷新方式。
- `docs/任务清单.md`：RBAC-D ✅；RBAC 全部阶段链接/状态。
- `docs/开发日志.md`：追加 RBAC-D 条目（权限驱动 UI、管理壳、页面、测试、复审修复、浏览器验收）。
- `README.md`、`运行手册.md`：管理后台入口、页面说明、只读审计员/内容管理员/游戏化管理员可用范围、旧角色不再授权。
- `docs/primevue-migration.md`：标记管理壳/仪表盘/用户/角色/审计页面完成；不再写“管理 UI 未上线”。
- 不修改 OpenAPI（无后端契约变化）。

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

- [ ] **Step 3: 真实浏览器验收**

用 `m2badmin`（super）和受限测试角色验证：
1. 登录后 `/admin` 显示仪表盘计数与最近审计。
2. 用户管理：编辑资料、分配非 super 角色、调金币、封禁/解封；自封禁与最后 super 错误提示。
3. 角色权限：新建只含 `user.read` 的角色；权限矩阵保存；系统角色不可删；super 权限不可改。
4. 审计日志：按 action/targetType/日期筛选；角色/用户/金币操作记录可见。
5. 受限账号登录：只看到有权限菜单；直接访问无权限 `/admin/users`/`/admin/roles`/`/admin/audit`/`/admin/gamification` 显示 403。
6. 游戏化：只有 `gamification.achievement.manage` 的账号只看到成就 tab；无任务/徽章/称号 tab。
7. 题库/比赛按钮按权限显示：publish、edit、delete、announcement 等不越权。
8. 移动端宽度下管理 Drawer/表格可用，无控制台错误。
9. 清理临时角色/用户/内容；恢复测试账号权限。
- 如果浏览器工具不可用，至少完成等价 HTTP + 页面组件验证，并在报告中明确未执行的真实浏览器项。

- [ ] **Step 4: 提交**

```bash
git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md docs/primevue-migration.md
git commit -m "docs: record rbac-d admin ui"
```

---

## Self-Review

**Spec coverage:**

- `auth.can/canAny`、路由 meta、菜单/按钮权限 → Task 1。
- AdminLayout、仪表盘、管理导航、移动 Drawer → Task 2。
- 用户管理（编辑/角色/封禁/金币）→ Task 3。
- 角色权限矩阵、系统角色保护 → Task 4。
- 审计日志 → Task 5。
- 文档、全量检查、浏览器验收 → Task 6。
- 未改后端/OpenAPI；RBAC-C 已完成的内容权限作为前端展示依据。

**Type consistency:**

- 权限常量使用后端稳定 code；`AuthState.permissions` 与 `Me.permissions` 一致。
- `api/admin.ts` 类型全部来自 `schema.d.ts`。
- Admin route meta 用 `permission`/`anyPermission`；不再使用 `roles`。
- 页面动作权限与后端路由权限一一对应。

**Review Focus mapping:**

1. 权限即时性 → Task 1 auth/guard/ProfileEdit tests。
2. 路由边界 → Task 1/2/3/4/5 guard/page tests + Task 6 E2E。
3. 按钮最小权限 → Task 1/3/4/5 tests。
4. 敏感操作 UX → Task 3/4 tests + Task 6 E2E。
5. 用户资料完整性 → Task 3 profile prefill tests。

**Proportion:** 计划固定文件、权限映射、组件职责和测试；具体模板/样式按现有 PrimeVue 页面风格实现。
