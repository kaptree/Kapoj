# 游客权限调整与成长板块可见性设计

> 目标：在后台「角色权限」页增加一个系统内置的「游客」角色，用权限开关控制未登录访客是否能看到「成长」板块。默认关闭，游客只看到核心导航；登录用户不受影响。

## 1. 背景与现状

- RBAC 目前以 `user_roles` + `role_permissions` 为用户授权；未登录访客没有用户 ID，也没有权限集合。
- `AppNav.vue` 对游客和登录用户都渲染「成长」组（成就、任务、徽章商城、称号商店、装扮管理）。
- 成就、任务、徽章商城、称号商城路由本身是公开的；游客可以直接访问。装扮管理需要登录。
- 站点首页的「成长中心」区块对游客也显示。
- 后台角色管理只能管理可分配给用户的角色，没有游客配置入口。

## 2. 目标与非目标

### 目标

- 在后台角色管理列表新增系统内置「游客」角色，默认无成长权限。
- 新增游客访问权限 `guest.growth.view`；管理员可在角色矩阵中开关。
- 默认游客：
  - 侧栏只显示「核心」组：首页、题库、测评记录、比赛、学习路线、排行榜。
  - 首页不显示「成长中心」区块。
  - 直接访问成就/任务/徽章商城/称号商城路由被拦截到无权限页。
- 管理员打开游客权限后：
  - 游客侧栏显示「成长」组（成就、任务、徽章商城、称号商店；装扮管理仍仅登录用户可见）。
  - 首页显示站点配置的成长链接。
  - 上述公开成长路由允许游客访问。
- 登录用户行为完全不变，仍然看到全部成长内容。
- 游客角色不可分配给用户，不出现在用户角色选择器中。
- 角色管理页对游客角色只展示「游客访问」权限组，避免把管理员权限误配给游客。

### 非目标

- 不做游客的后端 API 访问控制。成长页面本身是公开内容，本次是导航/页面可见性配置，不当作安全边界。
- 不做按单个成长页面（成就/任务/徽章/称号）拆分权限；本次是一个「成长板块」总开关。
- 不改变登录/注册、普通用户角色、管理员权限或现有 RBAC 权限目录语义。
- 不新增站点设置字段；游客权限复用 RBAC 表。
- 不做游客角色成员管理（游客不是真实用户）。

### 反目标

- 管理员把自己的管理员权限误配给游客角色并生效。
- 游客角色被分配给真实用户，导致用户意外获得/失去成长内容。
- 登录用户被游客开关影响。
- 默认配置仍显示成长板块（必须默认关闭）。
- 只在侧栏隐藏，首页成长区块仍暴露。

## 3. 约束

- 后端 Go + Gin + GORM + PostgreSQL；前端 Vue 3 + Pinia + PrimeVue。
- 游客角色 `code = "guest"`，`is_system = true`，不可删除、名称不可改。
- 新权限 `guest.growth.view`，权限组「游客访问」。
- 游客角色默认权限为空；`Seed` 只补齐角色和权限目录，不自动授予成长权限。
- 游客权限只允许配置给游客角色；游客角色只允许配置游客访问权限。
- 游客角色不参与 `user_roles` 分配；`ReplaceUserRoles` 拒绝该角色。
- 游客访问信息通过公开接口读取；未登录用户可访问。
- 前端游客权限 store 默认空集合，加载失败也视为空（默认隐藏成长）。
- 路由守卫对未登录用户检查 `guest.growth.view`；登录用户跳过。
- 侧栏和首页成长区块使用同一开关，避免两处不一致。
- 系统内置角色数量/权限目录数量变化需要更新现有 Seed/角色测试。
- 中文文案、现有视觉风格、响应式和无障碍保持。

## 4. 方案选择

### 4.1 游客权限存储

- **方案 A：内置系统角色 `guest` + 现有 role_permissions（选用）**。复用角色矩阵、权限目录、Seed 和管理 API；后台新增一个角色行即可。
- 方案 B：站点设置 JSON 字段。只适合单开关，无法复用权限矩阵，后续扩展差。
- 方案 C：新建 `guest_permissions` 表。需要另建管理接口和权限目录，重复 RBAC。
选择 A。

### 4.2 权限粒度

- **方案 A：一个 `guest.growth.view` 总开关（选用）**。用户要求的是「成长这部分内容」；默认关闭，配置简单。
- 方案 B：按成就/任务/徽章/称号分别配置。更细，但后台矩阵和前端过滤复杂，当前需求未要求。
- 方案 C：按页面路由动态权限。实现最重，不适合本阶段。
选择 A。

### 4.3 前端权限获取

- **方案 A：公开 `GET /api/guest/permissions` + Pinia `guest` store（选用）**。未登录时由路由守卫首次加载，侧栏/首页共用；失败默认隐藏。
- 方案 B：塞进 `GET /api/site`。少一次请求，但把 RBAC 混进站点配置，且管理后台更新后刷新语义模糊。
- 方案 C：登录页/App 内联请求。状态分散，难测试。
选择 A。

## 5. 设计

### 5.1 权限目录与内置角色

`rbac/catalog.go`：

```go
const (
    PermissionGroupGuest = "游客访问"
    PermGuestGrowthView PermissionCode = "guest.growth.view"
    RoleCodeGuest = "guest"
)
```

权限定义：

```go
{Code: PermGuestGrowthView, Name: "游客可见成长板块", GroupName: PermissionGroupGuest,
 Description: "允许未登录访客查看成就、任务、徽章商城与称号商店"}
```

内置角色：

```go
{
    Code: RoleCodeGuest, Name: "游客",
    Description: "未登录访客的公开可见范围；不可分配给用户",
}
```

- 默认 `Permissions` 为空。
- `ensureRoles` 已按 `ON CONFLICT DO NOTHING` 补齐，因此已有数据库启动后会自动新增 guest 角色。
- `ensurePermissions` 自动新增权限。
- `SeedVersion` 不变；新权限/角色通过幂等补齐。

### 5.2 游客权限服务

`rbac/roles.go`：

```go
func (s *Service) GuestPermissions(ctx context.Context) ([]string, error)
```

- 查 `roles.code='guest'`；不存在返回空切片 + nil。
- 查 `role_permissions` join `permissions`，按 code 排序。
- 只返回游客角色实际拥有的权限码。

`ReplaceRolePermissions`：

- 若角色是 `guest`：
  - 只允许 `guest.growth.view`；
  - 其他权限码返回 40001「游客角色只能配置游客访问权限」。
- 若角色不是 guest：
  - 不允许包含 `guest.growth.view`；
  - 返回 40001「游客访问权限只能配置给游客角色」。

`CreateRole`：

- 自定义角色不允许包含游客访问权限（同上）。
- code `guest` 已存在，不需要额外保留逻辑；唯一索引会拒绝重复。

`ReplaceUserRoles`：

- `newCodes` 包含 `guest` 时返回 40001「游客角色不可分配给用户」。

### 5.3 公开接口

`GET /api/guest/permissions`（公开）：

```json
{ "permissions": ["guest.growth.view"] }
```

- 挂在 `userapi.RegisterRoutes`（已接收 `Options.RBAC`）；`h.rbac == nil` 时返回空数组。
- `rbac.Service` 查 guest 角色权限。
- OpenAPI 增加 `GuestPermissions`、`GuestPermissionsResponse`。
- 前端 `api/guest.ts` 增加 `getGuestPermissions()`。

### 5.4 前端游客 store

`apps/web/src/stores/guest.ts`：

```ts
export const useGuestStore = defineStore('guest', () => {
  const permissions = ref<string[]>([])
  const loaded = ref(false)
  let inFlight: Promise<void> | null = null

  async function ensureLoaded() { ... }
  async function refresh() { loaded.value = false; await ensureLoaded() }
  function can(code: string) { return permissions.value.includes(code) }
  return { permissions, loaded, ensureLoaded, refresh, can }
})
```

- 默认空集合；请求失败也标记 loaded，避免重复请求；权限保持为空（隐藏成长）。
- `permissions.ts` 增加：

```ts
export const GUEST_PERM = {
  growth: 'guest.growth.view',
} as const
```

### 5.5 路由守卫

`AuthState` 增加可选 `guestPermissions?: string[]`：

```ts
if (!state.isLoggedIn && to.meta.guestPermission &&
    !(state.guestPermissions ?? []).includes(to.meta.guestPermission)) {
  return { name: 'forbidden' }
}
```

- `router/index.ts` 为 `achievements`、`tasks`、`badge-shop`、`title-shop` 增加：

```ts
meta: { guestPermission: GUEST_PERM.growth, title: ... }
```

- 登录用户不受 `guestPermission` 限制。
- `appearance` 仍是 `requiresAuth`，不设置 guestPermission。
- `router.beforeEach` 改为 async：
  ```ts
  if (!auth.isLoggedIn) await useGuestStore().ensureLoaded()
  return authGuard(to, { ..., guestPermissions: guest.permissions })
  ```
- 游客直接访问被关闭的成长路由 → `/forbidden`；不显示内容。

### 5.6 侧栏与首页

`AppNav.vue`：

```ts
const guest = useGuestStore()
const growthVisible = computed(() => auth.isLoggedIn || guest.can(GUEST_PERM.growth))
const guestGrowthItems = growthItems.filter((item) => item.key !== 'appearance')
const groups = computed(() => {
  const list = [{ key: 'core', label: '核心', items: primaryItems }]
  if (growthVisible.value) {
    list.push({
      key: 'growth',
      label: '成长',
      items: auth.isLoggedIn
        ? (showAdminEntry.value ? [...growthItems, adminItem] : growthItems)
        : guestGrowthItems,
    })
  }
  return list
})
```

- 游客关闭时只渲染核心组。
- 游客开启时渲染成长组，但隐藏「装扮管理」（要求登录）。
- 登录用户保持原样。
- 测试 mock guest store；默认 false，测试包含默认隐藏和开启显示。

`HomeView.vue`：

```ts
const visibleGrowthLinks = computed(() => {
  if (auth.isLoggedIn) return [...site.home.growthLinks, appearanceLink]
  return guest.can(GUEST_PERM.growth) ? site.home.growthLinks : []
})
```

- 游客关闭时成长中心区块不渲染。
- 游客开启时显示站点配置成长链接；不显示「装扮管理」。
- 测试覆盖默认隐藏/开启显示/登录用户不变。

### 5.7 后台角色管理

`AdminRolesView.vue`：

- 角色列表自动出现「游客」系统角色（`isSystem=true`，userCount 0）。
- 游客角色可编辑权限；名称/描述受系统角色规则限制（名称不可改，描述可改）。
- 打开游客角色时：
  - 权限矩阵只展示「游客访问」组；
  - 非游客角色隐藏「游客访问」组；
  - 增加提示：「游客默认只能看到核心导航；开启后额外显示成长板块（装扮管理仍需登录）。」
- 新建/编辑非游客角色时，游客访问权限不参与展示；后端仍兜底拒绝非法配置。
- 保存游客权限后，管理员刷新页面或下一个游客请求生效。可选：保存后调用 guest store refresh；本次不做，避免跨管理员状态耦合。

`AdminUsersView.vue`：

- `roleAssignOptions` 过滤 `code === 'guest'`，避免把游客角色分配给用户。
- 角色筛选可以保留游客（筛选结果为空），不影响现有筛选。

### 5.8 后端默认与迁移

- 已有数据库启动 `rbac.Seed` 时自动插入 `guest` 角色和 `guest.growth.view` 权限。
- guest 角色默认无 `role_permissions`，因此默认关闭。
- 不修改已有角色权限。
- 不需要新表/列。

### 5.9 测试

后端：

- `GuestPermissions`：默认空；授予 `guest.growth.view` 后返回；不存在角色返回空；排序稳定。
- `ReplaceRolePermissions`：
  - 游客角色只能设置 `guest.growth.view`；
  - 游客角色设置其他权限 40001；
  - 非游客角色设置游客权限 40001；
  - 正常角色原行为不变。
- `ReplaceUserRoles`：包含 guest 角色 40001；其他角色正常。
- Seed：角色数 8、权限数 39、guest 默认无权限、guest 为系统角色。
- 公开接口：游客 200、权限数组、rbac nil 空数组。
- 角色列表 API 包含 guest；AdminRolesView/AdminUsersView 测试更新。

前端：

- guest store：加载成功/失败/只请求一次/refresh。
- AppNav：游客默认无成长组；游客开启后有成长组且无装扮管理；登录用户有全部成长；管理员入口不变。
- HomeView：游客默认无成长中心；开启后显示；登录用户显示成长+装扮。
- Router guard：游客无权限访问成长路由 → forbidden；有权限允许；登录用户不受限；权限加载失败默认拒绝。
- AdminRolesView：游客行存在；游客权限矩阵只有「游客访问」；勾选保存调用 `replaceAdminRolePermissions`；非游客矩阵不含游客组。
- AdminUsersView：角色分配选项不含游客。
- 全量 test/lint/build。

### 5.10 文件影响

后端：

- `apps/server/internal/rbac/catalog.go`
- `apps/server/internal/rbac/roles.go`
- `apps/server/internal/rbac/users.go`
- `apps/server/internal/rbac/*_test.go`
- `apps/server/internal/userapi/handler.go`
- `apps/server/internal/userapi/handler_test.go`
- `packages/api/openapi.yaml`

前端：

- `apps/web/src/permissions.ts`
- `apps/web/src/stores/guest.ts`（新增）
- `apps/web/src/stores/guest.spec.ts`
- `apps/web/src/api/guest.ts`（新增）
- `apps/web/src/api/types.ts`
- `apps/web/src/api/schema.d.ts`（生成）
- `apps/web/src/router/index.ts`
- `apps/web/src/router/guard.ts`
- `apps/web/src/router/guard.spec.ts`
- `apps/web/src/components/layout/AppNav.vue`
- `apps/web/src/components/layout/AppNav.spec.ts`
- `apps/web/src/views/HomeView.vue`
- `apps/web/src/views/HomeView.spec.ts`
- `apps/web/src/views/AdminRolesView.vue`
- `apps/web/src/views/AdminRolesView.spec.ts`
- `apps/web/src/views/AdminUsersView.vue`
- `apps/web/src/views/AdminUsersView.spec.ts`

文档：

- `docs/任务清单.md`：新增「跨里程碑：游客可见性权限」。
- `docs/开发日志.md`：交付后追加。

## 6. 验收标准

- 默认数据库/新库：游客侧栏只有核心组；首页无成长中心；直接访问成长路由到无权限页。
- 后台角色管理出现「游客」角色；默认权限为空；只显示游客访问组。
- 勾选「游客可见成长板块」并保存后，新游客会话能看到成长组和首页成长中心；登录用户不变。
- 游客角色不能分配给用户；尝试分配返回 40001。
- 管理员权限不能被误配到游客角色；游客权限不能配给其他角色。
- 现有角色管理/用户管理/导航/首页/路由权限测试全部通过。
- 全量前端 test/lint/build 通过；后端 RBAC/userapi 包测试通过。

## 7. 已知取舍

- 这是可见性配置，不是后端安全边界；公开成长 API 仍可被直接调用。
- 一个总开关，不能单独隐藏成就/任务/徽章/称号；后续可按页面拆分权限。
- 游客权限变更对已打开页面不会实时生效，需要刷新；新请求按最新配置。
- 游客开启成长后仍不显示「装扮管理」（需要登录），避免把登录专属入口暴露给游客。
- 游客角色使用系统角色机制，名称不可改；描述和权限可改。
- 角色列表多一个系统角色，现有后台测试的数量断言需要更新。
