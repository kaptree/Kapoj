# AI 管理开关与权限设计

> 目标：在管理后台增加 AI 功能启用/关闭开关，新增独立权限 `ai.settings.manage`；AI 路由按运行时设置统一 404 关闭，默认启用以保留现有行为。

## 1. 背景与现状

- M5-A 的 AI 路由（`/api/ai/configs`、`complete`、`format`、`optimize`、`hint`）只在配置 `KAPOJ_AI_ENCRYPTION_KEY` 时挂载；未配置时路由不存在（404），前端显示“AI 功能未启用”。
- 没有运行时开关：配置密钥后 AI 必然可用，管理员无法临时关闭；也没有单独的 AI 管理权限。
- `system.settings.manage` 是预留的平台设置权限，但本次按用户要求新增独立 `ai.settings.manage`，不复用它。

## 2. 目标与非目标

### 目标

- 新增管理后台 AI 开关：启用/关闭 AI 功能；关闭后所有 `/api/ai/*` 路由返回 404（与未配置密钥一致）。
- 新增权限码 `ai.settings.manage`（第 36 个），分组“AI”，默认只授予 `super_admin`（super 天然绕过）；可后续分配给角色。
- 管理页 `/admin/ai`：显示当前状态、是否已配置加密密钥、保存开关；写审计。
- 默认启用（`enabled=true`）以保留现有“配置密钥即可用”的行为；管理员可关闭。
- 未配置 `KAPOJ_AI_ENCRYPTION_KEY` 时：
  - AI 路由不挂载（保持现状）；
  - 管理页显示“未配置 AI 加密密钥”，不能启用（启用请求返回 40001）。
- 关闭/启用立即生效（下一个请求即 404/正常），无需重启。

### 非目标

- 不在管理后台配置 `KAPOJ_AI_ENCRYPTION_KEY`、provider base_url 或平台共享 API key；密钥仍由部署环境提供。
- 不做按 provider/按用户/按题目的 AI 开关；不改变 M5-A provider seed、用户配置、限流、提示缓存。
- 不做 AI 使用统计、费用、审计查询以外的运营面板。
- 不改变 `/api/admin/*` 权限体系的其他部分；不推送/不合并。

## 3. 数据模型

```text
ai_settings (
  id          smallint primary key,   -- 固定 1
  enabled     bool not null default true,
  updated_by  bigint not null default 0,
  updated_at  timestamptz not null
)
```

- 单例 `id=1`；首次读取时懒创建（`ON CONFLICT DO NOTHING`），默认 `enabled=true`。
- 迁移加入 `db.Migrate`；无环境变量。
- 不删除/重置现有 AI 数据。

## 4. 权限与审计

- 新增 `rbac.PermAISettingsManage = "ai.settings.manage"`；分组 `PermissionGroupAI = "AI"`；权限目录第 36 个。
- 默认只给 `super_admin`（super 绕过）；content_admin 不自动获得。
- 管理 API 使用 `RequirePermission`；PUT 成功后写审计：
  - action `ai.settings.manage`
  - target type `ai_settings`, target id 1
  - summary“启用/关闭 AI 功能”
- 失败（未配置密钥）不写成功审计。
- 前端 AdminNav/route 使用 `PERM.ai.settingsManage`；`ADMIN_ACCESS_PERMISSIONS` 增加该权限。

## 5. API

### 5.1 管理设置

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/admin/ai/settings` | `ai.settings.manage` | `{enabled, keyConfigured, updatedAt}` |
| PUT | `/api/admin/ai/settings` | `ai.settings.manage` | `{enabled}`；启用但无密钥 → 40001 |

- `keyConfigured` 由进程启动时是否成功构造 AI 服务决定（即是否配置了合法密钥）；只读。
- `PUT` 允许关闭即使无密钥（保持关闭）；启用需要 `keyConfigured=true`。
- 记录 `updated_by=当前用户`、`updated_at=now`。
- 审计 best-effort 还是事务内？与设置更新同事务写 `rbac.WriteAuditTx`，失败回滚；与角色/用户敏感设置一致。

### 5.2 AI 路由开关

- 新增 `ai.SettingsService`（DB）：
  - `Get(ctx) (*SettingsView,error)`：懒创建默认行。
  - `Update(ctx, actorID, enabled) (*SettingsView,error)`。
  - `Enabled(ctx) (bool,error)`：读取设置（不存在则默认 true）。
- `aiapi` 所有路由加中间件 `RequireAIEnabled(settingsSvc)`：
  - 设置关闭 → `httpx.ErrNotFound`（404），与未挂载一致；不暴露“AI 已关闭”。
  - 设置读取 DB 错误 → 500（不静默放行）。
- `router` 构造 settings service（即使 `d.AI==nil` 也构造，供管理页显示 keyConfigured=false）；`aiapi` 仅在 `d.AI!=nil` 时挂载，并传入 settings。
- 管理 API 在 `adminapi`，不受 AI 开关影响，始终可访问（有权限时）。

## 6. 前端

### 6.1 管理页 `/admin/ai`

- 路由 `admin-ai`，`requiresAuth` + `permission: ai.settings.manage`。
- AdminNav 内容组（或新“AI”组）增加“AI 设置”；仅权限可见。
- 页面：
  - `AppPageHeader` + `AppSurface`；
  - 当前状态 Tag（已启用/已关闭）；
  - `ToggleSwitch` 启用 AI；
  - 未配置密钥时 `Message` 警告“未配置 AI 加密密钥，无法启用”，开关禁用；
  - 保存按钮；成功 Toast；失败中文；加载/错误态；
  - 说明：关闭后所有 AI 接口返回 404，用户侧显示“AI 功能未启用”；加密密钥仍由部署环境配置。
- 不新增用户侧入口；用户 AI 设置/提交页/提示通过 404 自动降级。

### 6.2 类型/契约

- OpenAPI：`AdminAISettingsView`、`AdminAISettingsInput`、`AdminAISettingsResponse`；paths GET/PUT `/admin/ai/settings`。
- 生成 `schema.d.ts`；`types.ts` 别名；`api/admin.ts` `getAdminAISettings`/`updateAdminAISettings`。
- 不改变现有 AI 用户接口契约。

## 7. 测试与验收

### 7.1 后端

- `ai_settings` 懒创建默认 enabled=true；重复读取幂等；db 迁移表。
- 管理 GET/PUT：权限 401/403/200；超管/授权角色；启用无密钥 40001；关闭允许；审计 action/target/summary；事务回滚。
- `Enabled`: 默认 true；关闭 false；DB 错误。
- AI 路由中间件：enabled=true 正常；false 全部 404；未挂载（无密钥）仍 404；管理 API 不受影响。
- 权限目录 36、分组“AI”、seed/roles 测试。

### 7.2 前端

- AdminAiView：加载/启用/关闭/密钥缺失禁用/保存/Toast/错误/权限路由。
- AdminNav 入口权限；`admin-rbac`/权限计数 36。
- 用户侧 AI 设置/提交/提示在开关关闭时显示“AI 功能未启用”。
- 生成类型/lint/build。

### 7.3 真实冒烟

- 配置密钥的临时 server：管理页关闭 → `/api/ai/configs` 404、用户侧显示未启用；开启 → 200/可用；审计记录；无权限账号 403。
- 无密钥 server：管理页显示未配置，启用返回 40001，关闭可保存；AI 路由仍 404。
- 清理临时设置/用户/审计；恢复默认启用；不删除既有数据。

## 8. 交付拆分

- AI-A：`ai_settings` 模型/服务/迁移、权限、管理 API、AI 路由中间件。
- AI-B：OpenAPI/生成类型。
- AI-C：管理页/导航/路由/前端测试。
- AI-D：文档、全量检查、真实冒烟。

## 9. 留给实施计划的决定

- `SettingsService` 放 `ai` 包的文件拆分与构造参数。
- 管理页开关即时保存还是“保存”按钮（建议保存按钮，避免误触）。
- 审计 summary 的具体中文文案。
