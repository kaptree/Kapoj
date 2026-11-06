# 平台管理后台与细粒度 RBAC 设计

> 目标：把现有写死的 `users.role` 枚举升级为可配置的“多角色 + 全局权限”体系，并提供统一 `/admin` 管理后台。
> 本文覆盖第一阶段：RBAC 核心、用户/角色管理、审计日志、管理壳与仪表盘；现有题目/比赛/游戏化页面先迁移权限校验并沿用现有入口。

## 1. 背景与现状

- 当前 `users.role` 是单一枚举：`user` / `problem_admin` / `contest_admin` / `super_admin`。
- JWT 中携带 role，`middleware.RequireRole` 直接比较；角色变更后旧 token 仍带旧角色。
- 管理入口分散：题目、比赛、游戏化、用户列表各自用角色判断，没有统一后台。
- 没有权限表、角色表、用户多角色关系、审计日志。
- 前端 `hasRole` 只支持枚举角色，管理员入口写死 `super_admin`。

## 2. 目标

- **权限目录**：每个管理功能拆成稳定权限码，按资源分组。
- **动态角色**：管理员可创建/编辑角色并勾选权限；账号可拥有多个角色，权限取并集。
- **统一管理后台**：`/admin` 独立布局，含仪表盘、用户管理、角色权限、审计日志；内容模块沿用现有页面。
- **权限即时生效**：角色/权限变更后，下一次后端请求立即按新权限判断，不依赖旧 JWT。
- **审计**：记录管理员敏感操作（角色、权限、用户封禁、金币、内容变更等）。
- **兼容迁移**：现有 `super_admin`/`problem_admin`/`contest_admin` 账号平滑映射到新角色。
- **前端权限驱动**：菜单、路由、按钮按 `permissions` 显示/隐藏，后端始终最终校验。

## 3. 非目标（第一阶段）

- 不实现资源范围权限（如“只能管理自己创建的比赛”）；权限全局生效。
- 不把题目/比赛/游戏化/学习路线页面重做成新的管理页面；它们先沿用现有入口，只迁移权限判断。统一管理壳提供链接。
- 不实现 M5 模块（证书、AI、题包导入）的管理 UI；权限目录为其预留扩展，但第一阶段不启用。
- 不实现审批流、双人复核、权限申请/到期时间。
- 不实现权限缓存层；第一阶段每次管理请求查库，管理流量低。
- 不实现审计日志导出/归档/自动清理。
- 不实现社交内容审核（后续 M4 阶段）。

## 4. 方案选择

### 4.1 角色模型：多角色 + 全局权限（已确认）

- 账号可同时拥有多个角色，权限取并集。
- 权限全平台生效，不做资源范围。
- 被否决：
  - 单角色：组合权限需要不断新建合并角色，账号调整不灵活。
  - 多角色 + 资源范围：需要资源归属上下文、范围 UI 与每条查询的额外过滤，第一阶段成本过高，后续可扩展。

### 4.2 权限存储：数据库实时查询，不写 JWT

- JWT 只保留用户 ID 与旧版 role（兼容），不写权限列表。
- `RequirePermission` 中间件按 user ID 查询用户状态、角色、权限；变权限后下一次请求立即生效。
- 被否决：
  - 权限写 JWT：角色变更后旧 token 仍有效，必须等过期。
  - Redis 缓存权限：第一阶段管理流量低，增加失效复杂度；后续可按 `permissions_version` 加缓存。

### 4.3 管理后台布局：独立 `/admin` 壳

- 独立 `AdminLayout`：左侧管理菜单、顶部面包屑/用户菜单、内容区。
- 用户端仍用现有 `DefaultLayout`。
- 现有 `/admin/gamification` 移入或链接到管理壳；题目/比赛管理页先链接到现有页面。
- 被否决：把管理菜单塞进现有侧栏——管理功能多、权限差异大，独立壳更清晰。

### 4.4 审计：敏感操作显式 + 管理变更中间件

- RBAC/用户角色/封禁/金币等敏感操作在服务事务内写审计，失败回滚。
- 题目/比赛/游戏化等内容管理变更通过管理中间件记录方法/路径/目标/状态，best-effort 写审计。
- 不记录请求体，避免密码、测试数据、代码泄露。

## 5. 权限目录

权限码格式：`domain.action`，稳定不可改名。权限按组展示。

### 5.1 管理后台

- `admin.dashboard.view`：访问管理仪表盘
- `admin.audit.view`：查看审计日志
- `system.settings.manage`：系统设置（预留给后续配置页）

### 5.2 用户与角色

- `user.read`：查看用户列表/详情
- `user.update`：编辑用户资料（管理员）
- `user.ban`：封禁/解封
- `user.coins.adjust`：调整金币
- `user.roles.assign`：给用户分配/移除角色
- `role.read`：查看角色与权限目录
- `role.create`：创建自定义角色
- `role.update`：编辑角色名称/描述
- `role.delete`：删除自定义角色
- `role.permissions.update`：修改角色权限

### 5.3 题库

- `problem.view_hidden`：查看未公开题目
- `problem.create`
- `problem.update`
- `problem.delete`（含下线）
- `problem.publish`：公开/下线（通过 `PUT /problems/:id` 修改 `isPublic` 时额外要求；只有 `problem.update` 不能发布/下线）
- `problem.testcase.manage`：测试数据上传/编辑/排序/删除
- `problem.checker.manage`：checker 上传

### 5.3.1 测评记录

- `submission.view_live_contest`：比赛进行中查看他人代码/成绩/测试点（绕过赛时保护）
- `problem.view_hidden` 同时用于隐藏题目的提交列表/详情与隐藏题提交

### 5.4 比赛

- `contest.view_hidden`：查看非公开比赛
- `contest.create`
- `contest.update`
- `contest.delete`
- `contest.participant.read`：查看报名用户
- `contest.announcement.manage`：公告 CRUD

### 5.5 游戏化

- `gamification.achievement.manage`
- `gamification.task.manage`
- `gamification.badge.manage`
- `gamification.title.manage`
- `gamification.config.manage`（如装备上限等系统配置）

### 5.6 学习路线

- `learning_path.official.manage`：管理官方路线（题管/超管）
- `learning_path.admin.read`：查看所有/下架路线（管理后台）
- 用户自定义路线仍由“登录用户 + 作者”规则控制，不占用管理权限。

### 5.7 预留（本阶段不启用）

- `certificate.review`、`ai.provider.manage`、`problem.import`、`social.content.moderate`
- 预留权限不出现在第一阶段管理 UI；M5 接入时再加入。

## 6. 内置角色

| code | 名称 | 说明 | 系统角色 |
|------|------|------|----------|
| super_admin | 超级管理员 | 绕过所有权限检查；不可删除/不可改权限 | 是 |
| content_admin | 内容管理员 | problem.* + learning_path.official.manage | 是 |
| contest_admin | 比赛管理员 | contest.* | 是 |
| gamification_admin | 游戏化管理员 | gamification.* | 是 |
| user_admin | 用户管理员 | user.read/update/ban/coins.adjust | 是 |
| security_admin | 安全管理员 | role.* + user.roles.assign + admin.audit.view + dashboard | 是 |
| auditor | 审计员 | dashboard + audit + user.read + role.read（只读） | 是 |

- 系统角色不可删除、code/name 不可改；除 `super_admin` 外，权限可由 `role.permissions.update` 调整。
- 自定义角色可创建/改名称/描述/权限/删除。
- 删除自定义角色时移除用户关联；审计记录。
- 内置角色权限明确集合：
  - `super_admin`：绕过所有检查。
  - `content_admin`：`admin.dashboard.view`、`problem.view_hidden/create/update/delete/publish/testcase.manage/checker.manage`、`learning_path.official.manage`、`learning_path.admin.read`。
  - `contest_admin`：`admin.dashboard.view`、`contest.view_hidden/create/update/delete/participant.read/announcement.manage`、`submission.view_live_contest`。
  - `gamification_admin`：`admin.dashboard.view`、`gamification.achievement/task/badge/title/config.manage`。
  - `user_admin`：`admin.dashboard.view`、`user.read/update/ban/coins.adjust`、`role.read`。
  - `security_admin`：`admin.dashboard.view`、`admin.audit.view`、`user.read`、`role.*`、`user.roles.assign`。
  - `auditor`：`admin.dashboard.view`、`admin.audit.view`、`user.read`、`role.read`（只读）。
- 现有角色映射：
  - `users.role=super_admin` → `super_admin`
  - `problem_admin` → `content_admin`
  - `contest_admin` → `contest_admin`
  - `user` → 不分配管理角色
- `users.role` 列保留为 legacy/展示字段，不再作为权限来源；新账号默认 `user`。第一阶段完成后，任何管理授权不得再调用 `RequireRole`；旧字段/JWT role 只用于兼容展示，不参与授权。

## 7. 数据模型

```text
roles (
  id, code, name, description,
  is_system, created_at, updated_at
)
-- code 唯一

permissions (
  id, code, name, group_name, description, sort_order, created_at
)
-- code 唯一

role_permissions (
  id, role_id, permission_id
)
-- 唯一：(role_id, permission_id)

user_roles (
  id, user_id, role_id, assigned_by, assigned_at
)
-- 唯一：(user_id, role_id)

admin_audit_logs (
  id, actor_id, action, target_type, target_id,
  summary, ip, user_agent, created_at
)
-- action 为权限码或内部动作；target_type/target_id 可空

rbac_meta (
  key, value
)
-- seed_version 等初始化标记
```

- `user_roles.assigned_by` 可为空（种子迁移）。
- 审计 `summary` 只写非敏感摘要；不写密码、token、代码、测试数据内容。
- 删除角色/用户使用软删除或保留审计；用户本身已有 status，角色删除为物理删除（关联先删）。

## 8. 权限解析与中间件

### 8.1 解析

`rbac.PermissionResolver`：

```go
type Access struct {
  UserID      int64
  IsSuper     bool
  Roles       []RoleSummary
  Permissions map[string]struct{}
}
func (s *Service) Access(ctx, userID) (*Access, error)
```

- 查询 `users`：必须存在且 `status=active`；封禁用户管理接口返回 403。
- join `user_roles/roles`；若包含 `super_admin`，`IsSuper=true`。
- join `role_permissions/permissions` 得到权限集合。
- 每次管理请求实时查询；无缓存。
- 超级管理员绕过所有 `RequirePermission`，但仍写审计。

### 8.2 中间件

- `middleware.RequirePermission(code string)`：
  - 未登录 401；
  - 解析失败/封禁 403；
  - 非 super 且无权限 403；
  - 通过后把 Access 放入 context，后续 handler 可读取角色/权限。
- `middleware.RequireAnyPermission(codes...)` 用于管理壳入口/游戏化管理页。
- 现有 `RequireRole` 保留给未迁移的代码；第一阶段迁移所有管理写接口与隐藏资源读取到权限码。
- `RequireAuth` 不变。

### 8.3 旧 role 兼容

- JWT 继续带 role（旧前端/旧逻辑可用），但权限判断不再使用 `claims.Role`。
- `MeDTO` 增加：
  - `roles: [{id, code, name}]`
  - `permissions: string[]`（前端菜单/按钮用；不含 super 的通配）
  - `isSuperAdmin: boolean`
- `role` 字段保留为 legacy 主角色（展示/兼容）。
- 前端 `hasRole` 逐步替换为 `auth.can(permission)`；路由 meta 增加 `permission`/`anyPermission`。

## 9. 管理接口

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/admin/stats` | `admin.dashboard.view` | 用户/题目/比赛/提交等计数与最近审计 |
| GET | `/api/admin/permissions` | `role.read` | 权限目录（按分组） |
| GET | `/api/admin/roles` | `role.read` | 角色列表（含权限、用户数） |
| POST | `/api/admin/roles` | `role.create` | 创建自定义角色 |
| PUT | `/api/admin/roles/:id` | `role.update` | 改名称/描述（系统角色 code/name 不可改） |
| DELETE | `/api/admin/roles/:id` | `role.delete` | 删除自定义角色；系统角色 40001 |
| PUT | `/api/admin/roles/:id/permissions` | `role.permissions.update` | 整体替换权限；super_admin 不可改 |
| GET | `/api/admin/users` | `user.read` | 用户列表（现有扩展角色/权限摘要） |
| GET | `/api/admin/users/:id` | `user.read` | 用户详情（状态、角色、金币、统计摘要） |
| PUT | `/api/admin/users/:id` | `user.update` | 管理员编辑昵称/简介/头像链接 |
| PUT | `/api/admin/users/:id/roles` | `user.roles.assign` | 整体替换用户角色 |
| POST | `/api/admin/users/:id/ban` | `user.ban` | 封禁 |
| POST | `/api/admin/users/:id/unban` | `user.ban` | 解封 |
| POST | `/api/admin/users/:id/coins` | `user.coins.adjust` | 现有金币调整 |
| GET | `/api/admin/audit` | `admin.audit.view` | 审计日志分页/筛选 |

规则：

- `super_admin` 角色只能由 super_admin 分配/移除；普通 `user.roles.assign` 不能授予超级管理员。
- 禁止移除最后一个 active super_admin 的 `super_admin` 角色。
- 禁止封禁自己；只有 super_admin 能封禁/解封 super_admin 账号；禁止封禁最后一个 active super_admin。封禁检查与角色变更使用同一事务/锁。
- 角色分配/权限变更在事务内写审计。
- 系统角色不可删除；`super_admin` 不可改权限。
- 自定义角色 code 创建后不可改；名称/描述可改。
- 用户角色整体替换时，空数组表示移除所有管理角色（不能移除最后一个 super_admin）。
- 封禁用户：管理请求 403；现有 JWT 仍可能访问普通接口（沿用现有行为），但管理权限解析拒绝。

## 10. 审计

- 记录字段：actor、action、target、summary、IP、user-agent、时间。
- 敏感操作事务内写：
  - 角色创建/更新/删除/权限替换
  - 用户角色分配
  - 封禁/解封
  - 金币调整
  - 管理员编辑用户资料
- 内容管理变更：中间件 best-effort 记录 `method path status target`，不记录 body。
- 审计查询支持：actorId、action、targetType、日期范围、分页；按时间倒序。
- 审计内容不包含密码、token、代码、测试数据、金币备注之外的敏感信息。

## 11. 现有接口迁移

### 11.1 题库

- `POST /problems` → `problem.create`
- `PUT /problems/:id` → `problem.update`
- `DELETE /problems/:id` → `problem.delete`
- 测试点接口 → `problem.testcase.manage`
- checker 上传 → `problem.checker.manage`
- 未公开题目读取 → `problem.view_hidden`
- `PUT /problems/:id` 修改 `isPublic` 需要额外 `problem.publish`；仅 `problem.update` 只能改题面/元数据。
- Handler 构造 `problem.Viewer` 时按当前 Access 设置对应能力；Service 的权限字段拆成 `CanCreate/CanUpdate/CanDelete/CanPublish/CanManageTestcases/CanViewHidden`（或等价）。
- 现有 `problem_admin` 映射的 `content_admin` 拥有全部 problem.*，行为不变。

### 11.2 比赛

- `POST /contests` → `contest.create`
- `PUT /contests/:id` → `contest.update`
- `DELETE /contests/:id` → `contest.delete`
- 报名用户列表 → `contest.participant.read`
- 公告 CRUD → `contest.announcement.manage`
- 非公开比赛读取、开赛前题目/榜单读取 → `contest.view_hidden`
- 比赛服务 Viewer 增加对应能力字段；`contest_admin` 映射后行为不变。
- 赛时查看他人代码/成绩/测试点 → `submission.view_live_contest`；比赛管理员默认拥有。
- 比赛管理员测试提交（免报名）使用 `contest.update` 能力判定。

### 11.3 游戏化

- 成就 CRUD → `gamification.achievement.manage`
- 任务 CRUD → `gamification.task.manage`
- 徽章 CRUD/图片 → `gamification.badge.manage`
- 称号 CRUD → `gamification.title.manage`
- 配置接口 → `gamification.config.manage`
- 管理员调币 → `user.coins.adjust`
- 前端游戏化管理页路由改用 `RequireAnyPermission(gamification.*)`。

### 11.3.1 测评记录

- `submission.Viewer.CanManageProblems` → `problem.view_hidden`（隐藏题提交列表/详情/提交）
- `submission.Viewer.CanManage` 拆分：
  - 赛时保护绕过 → `submission.view_live_contest`
  - 免报名比赛测试提交 → `contest.update`
- 普通用户仍按本人/管理员/赛后规则；无新增公开字段。

### 11.4 学习路线

- 官方路线管理 → `learning_path.official.manage`
- 管理后台查看所有路线/下架 → `learning_path.admin.read`
- 用户自定义路线仍按作者规则，不要求管理权限。
- 第一阶段学习路线前端未完成时，只迁移后端权限。

## 12. 前端管理后台

### 12.1 布局与导航

- 新增 `AdminLayout.vue`：左侧管理菜单、顶部标题/用户菜单/返回站点；与 `DefaultLayout` 分离。
- 菜单分组：
  - 概览：`/admin`
  - 用户与权限：`/admin/users`、`/admin/roles`
  - 审计：`/admin/audit`
  - 内容：题目管理 `/problems`、比赛管理 `/contests`（链接现有页面）
  - 游戏化：`/admin/gamification`（现有页面）
  - 系统设置：预留
- 菜单项按权限显示；无 `admin.dashboard.view` 但有任一管理权限时，默认进入第一个可访问页面。
- 路由 meta：`permission` 或 `anyPermission`；守卫从 `auth.can` 判断。

### 12.2 页面

- **仪表盘**：用户数、题目数、比赛数、提交数、最近审计。计数为聚合数据，`admin.dashboard.view` 可看；最近审计卡片额外要求 `admin.audit.view`。计数包含未公开/封禁记录但不返回明细。
- **用户管理**：用户列表（搜索/状态/角色筛选）、角色标签、封禁/解封、金币调整、分配角色对话框。
- **角色权限**：角色列表；新建/编辑自定义角色；权限矩阵按资源分组勾选；系统角色保护提示；显示角色用户数。
- **审计日志**：actor/action/target/日期筛选、分页、摘要。
- **内容/游戏化**：管理菜单链接现有页面；现有页面按钮改为 `auth.can` 判断。
- 所有页面使用 PrimeVue 4、AppSurface/AppPageHeader/AppEmptyState/AppErrorState/AppLoadingState。

### 12.3 权限驱动 UI

- `auth.user.permissions: string[]`、`auth.user.isSuperAdmin`；新增 `auth.can(code)`、`auth.canAny(codes)`。
- `DefaultLayout` 管理入口：有 `admin.dashboard.view` 或任一管理权限时显示“管理后台”。
- 现有题目/比赛编辑按钮、游戏化管理入口改用 `auth.can`；路由守卫支持 `permission`。
- 角色/权限变更后，受影响账号刷新 `/users/me` 或重新进入页面生效；后台不推送实时权限。

## 13. 初始化与迁移

- 启动时调用 `rbac.Seed`（幂等，带版本标记）：
  1. 读取/写入 `rbac_meta.seed_version`；只在首次部署（无标记）执行完整 seed。
  2. 插入权限目录、内置角色、内置角色权限。
  3. 按 `users.role` 给现有用户分配角色（`ON CONFLICT DO NOTHING`）。
  4. 写入当前 seed_version。
- Seed 只在首次执行；管理员后续对内置角色权限的删除/调整不会被重启重新加回。新版本新增权限时通过显式迁移/新 seed_version 处理，不静默重加旧权限。
- `users.role` 保留；新账号默认 user，不自动获得管理角色。
- 迁移脚本/回滚：项目使用 GORM AutoMigrate；新增表只增不删；回滚通过数据迁移/手工处理。

## 14. 测试与验收

### 14.1 后端

- 权限解析：多角色并集、super 绕过、封禁拒绝、无角色空权限。
- 中间件：401/403、单权限/任一权限、权限变更后旧 token 立即生效。
- 角色：系统角色不可删/改名，super 不可改权限；自定义角色 CRUD、权限替换。
- 用户角色：多角色分配、super 只能 super 分配、最后一个 super 保护。
- 审计：敏感操作事务内写入；内容变更中间件记录；查询筛选/分页。
- 现有接口迁移：每个权限码的正反用例；旧 problem_admin/contest_admin 行为不变。
- Seed 幂等；旧角色映射正确；管理员删除内置角色权限后重启不会被重新加回。
- 封禁：不能封自己、只有 super 能操作 super、最后 super 保护。
- 权限读取：赛时保护/隐藏题/发布/学习路线 inactive 读取均按权限码。

### 14.2 前端

- `auth.can/canAny`。
- 管理路由守卫、菜单过滤、无权限 403。
- 用户管理/角色权限/审计页面组件测试。
- 权限矩阵勾选/保存/系统角色保护。
- lint/type-check/build。

### 14.3 真实浏览器 E2E

1. 超管创建自定义角色（只有题目查看/编辑），分配给测试用户；该用户只能看到题目管理入口，不能进用户/角色/审计。
2. 修改角色权限后，同一账号刷新页面，菜单/按钮即时变化；旧 JWT 不阻止新权限生效。
3. 普通用户直接访问 `/admin/users`、`/admin/roles`、`/admin/audit` → 403。
4. 封禁/解封、金币调整、角色分配写入审计；审计页可筛选。
5. 删除最后一个 super_admin 角色分配被拒绝；系统角色删除被拒绝。
6. 现有题目/比赛/游戏化管理在拥有对应权限时行为不变；无权限时隐藏且后端 403。
7. 移动端管理菜单/表格可用；无 CDN/控制台错误。

## 15. 交付拆分

### RBAC-A：权限模型、Seed、解析与中间件（后端）

- 表/模型/迁移、权限目录、内置角色、Seed/旧角色映射。
- Access 解析、RequirePermission/RequireAnyPermission。
- `MeDTO` roles/permissions/isSuperAdmin。
- 后端测试。

### RBAC-B：角色/用户/审计/统计 API（后端）

- 角色 CRUD/权限矩阵、用户角色分配、封禁/解封、审计、仪表盘统计。
- 最后 super 保护、系统角色保护、事务审计。
- OpenAPI/后端测试。

### RBAC-C：现有管理接口迁移（后端）

- 题库/比赛/游戏化/学习路线的权限码替换；Viewer 能力拆分。
- 内容管理审计中间件。
- 回归测试，保证旧角色映射后行为不变。

### RBAC-D：管理后台前端与文档/E2E

- AdminLayout、仪表盘、用户管理、角色权限、审计页面。
- `auth.can`、菜单/路由/按钮权限化；现有管理入口迁移。
- 文档、全量检查、浏览器 E2E。

## 16. 风险与取舍

- **权限即时查询数据库**：管理请求每次 2–3 个 join；管理流量低，可接受；后续可加 permissions_version 缓存。
- **保留 users.role**：仅展示/兼容，不参与授权；legacy role 与 roles 表可能不一致，文档明确“权限只认 roles 表”。第一阶段结束后管理授权不再使用 RequireRole。
- **系统角色权限可改**：除 super_admin 外允许调整，保留灵活性；审计记录。
- **内容管理页沿用现有页面**：第一阶段不做统一内容页，管理壳用链接；后续可迁移。
- **审计 best-effort**：内容变更审计失败不阻断业务；敏感 RBAC/用户操作事务内审计。
- **最后 super 保护**：只保护 active 用户；并发分配/移除需在事务中计数并锁。
- **前端权限可能短暂过期**：角色变更后用户需刷新 `/users/me`；后端权限始终实时。

## 17. 留给实施计划的决定

- `rbac`/`adminapi` 包的具体文件拆分。
- 权限矩阵前端的具体分组/勾选组件。
- 审计中间件的路由标注方式（逐路由 vs 路由组）。
- 统计接口的缓存/查询优化。
- OpenAPI 字段描述与示例。

以上不改变本文确认的角色模型、权限语义、管理范围和审计边界。


## 18. Builder check 决议

1. **旧 role/JWT**：`users.role` 与 JWT role 冻结为 legacy/展示；第一阶段移除所有管理授权中的 `RequireRole`，权限只认 `user_roles`。
2. **测评记录权限**：新增 `submission.view_live_contest`；隐藏题提交使用 `problem.view_hidden`；免报名测试提交使用 `contest.update`。
3. **特权读取**：隐藏题 `problem.view_hidden`；非公开/开赛前比赛与榜单 `contest.view_hidden`；`includeInactive` 由各域 manage 权限控制；`canManage` 字段由任一相关管理权限派生，具体按钮由前端 `auth.can` 控制。
4. **内置角色**：按 6 节明确集合；`user_admin` 补 `role.read`，`security_admin` 补 `user.read`，`content_admin` 补 `learning_path.admin.read`，所有管理角色补 `admin.dashboard.view`。
5. **最后 super/封禁**：禁止封自己；只有 super 能封/解封 super；禁止封最后一个 active super；与角色移除共用事务/锁。
6. **problem.publish**：`PUT /problems/:id` 修改 `isPublic` 需要 `problem.update` + `problem.publish`；仅 update 不能发布/下线。
7. **Seed 语义**：首次 seed + `rbac_meta.seed_version`；管理员调整不会被重启覆盖，后续新增权限走显式迁移。
8. **用户详情/编辑**：第一阶段增加 `GET/PUT /api/admin/users/:id`，`user.update` 编辑昵称/简介/头像链接并审计。
9. **仪表盘**：计数统一由 `admin.dashboard.view` 访问，聚合不含 PII；最近审计额外要求 `admin.audit.view`。
