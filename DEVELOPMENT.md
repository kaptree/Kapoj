# kapOJ 开发文档

> 本文档基于 `plan.md` 细化，是 kapOJ（Kap Online Judge）平台的详细开发规格说明书。
> 目标：**打造出最有趣味的 Online Judge 平台**。
> 文档包含完整的架构设计、数据库设计、模块需求，并为每个模块提供了可直接交给 Codex 的**开发提示词（Prompt）**。
> 开发进度见 [`docs/任务清单.md`](docs/任务清单.md)，每次变更记录见 [`docs/开发日志.md`](docs/开发日志.md)，前端 UI 迁移见 [`docs/primevue-migration.md`](docs/primevue-migration.md)。当前进度：**M4-2A、M4-2B、M4-2C、M4-3、M4-4、RBAC-D、M5-A、M5-4、M5-5、M5-6 与 M5-7 已完成**（M0–M3 已完成；M4 已完成；M5 部分完成。M4-A 交付 Rated 比赛 Rating 结算、8 级等级、Rating 历史与 Rating 榜后端；M4-B 交付活跃度、每日签到金币、共享分与活跃度/共享榜后端；M4-C 交付排行榜页、签到、Rating 历史卡片、导航与首页入口；M4-2A 交付学习路线模型、权限、路线/节点 CRUD 与树结构；M4-2B 交付服务端进度计算（AC 事件补算 + 读取补算、完成锁存）与题目下线/删除守卫；M4-2C 交付学习路线前端（列表/树详情/编辑/官方管理页，PrimeVue TreeTable/DataTable，UI-31）；M4-3 交付轻量社交互动（题解点赞、用户关注、站内通知，回复/点赞/关注接入活跃度与共享分，前端点赞按钮/关注 Dialog/通知铃铛/通知页，UI-32，见 4.8）；M4-4 交付 Rating 衰减与赛季重置（`rating.settings.manage` 权限与 Rating 分组、管理设置 API、`/admin/rating` 设置页、月度高分段衰减、赛季软重置、`decay`/`season_reset` 历史与公开赛季接口）；RBAC-A 交付权限目录、内置角色、首次 Seed/旧角色映射、实时 Access 解析、权限中间件与 `/users/me` 权限字段；RBAC-B 交付角色 CRUD/权限矩阵、用户角色分配、封禁/解封、审计日志与仪表盘统计 API；RBAC-C 交付题目/比赛/测评/游戏化/学习路线管理接口的权限码迁移、Viewer 能力拆分与内容管理审计；RBAC-D 交付 `auth.can`/`canAny`/路由 meta、AdminLayout 管理壳、仪表盘/用户管理/角色权限/审计日志页面与菜单/路由/按钮权限化，以及文档、HTTP 冒烟与浏览器 E2E；M5-A 交付 AI 后端基础（多 provider 配置、用户 API key AES-GCM 加密存储、OpenAI 兼容补全/格式化/优化与题目 AI 提示缓存/限流，AI 路由始终挂载、未配置密钥时返回 40002）；M5-5 交付 AI 前端（提交页 AI 补全/格式化/优化工具栏、题目详情 AI 提示、`/settings/ai` AI 配置页与用户菜单入口，见 §6.5）；M5-6 交付 HydroOJ 题包导入（`POST /api/problems/import/hydro`、大小限制/防路径穿越、slug 冲突不覆盖、默认下线、逐题部分成功与整包回滚，见 §7）与题目详情公开创建者摘要（`author{id,username,nickname}`），前端新增 `/problems/import` 导入页、题目列表「导入题包」入口与详情创建者链接。AI 出题造数据（M5-4）已交付：标程 → AI 纯 Python 3 造数据脚本 → judge 沙盒生成输入/输出 → 预览确认入库，见 §6.4；M5-7 证书认证已交付（证书类型/提交/审核/公开验证码核验，见 §5.6）；头像上传（M5-8）尚未实现）。前端 UI 已完成 UI-00～UI-20 的 PrimeVue 4 迁移（含 UI-19 题库编辑与 UI-20 异常页）、UI-30（M4-C 排行榜/历史）、UI-31（M4-7 学习路线）、UI-32（M4-3 社交互动）与 RBAC-D 管理后台页面直接实现，Element Plus 依赖与过渡样式已移除；该迁移已提交。题目详情页已改造为「题面 + 提示卡片 + 右侧四资源卡片」，并交付官方题解（创建者编辑 + 金币解锁）、解题思路论坛（帖子/回复/软删除）、提交统计页与关联比赛页，详见 4.3；题目主更新不再写入题解字段，官方题解专用接口是唯一写入口。

---

## 目录

1. [项目概述](#1-项目概述)
2. [技术栈与总体架构](#2-技术栈与总体架构)
3. [数据库总体设计](#3-数据库总体设计)
4. [基础功能模块](#4-基础功能模块)
   - 4.1 用户与认证
   - 4.2 代码测评沙盒
   - 4.3 题库与题目模块
   - 4.4 比赛模块
   - 4.5 学习路线模块
   - 4.6 测评记录模块
   - 4.7 平台管理后台与 RBAC
   - 4.8 社交互动模块
5. [核心趣味功能模块](#5-核心趣味功能模块)
   - 5.1 成就模块
   - 5.2 任务模块
   - 5.3 徽章模块
   - 5.4 称号模块
   - 5.5 排行榜模块
   - 5.6 证书认证模块
6. [AI 辅助功能模块](#6-ai-辅助功能模块)
7. [题目导入功能](#7-题目导入功能)
8. [Codex 开发提示词汇总](#8-codex-开发提示词汇总)
9. [开发里程碑建议](#9-开发里程碑建议)
10. [附录：非功能性需求](#附录非功能性需求)

---

## 1. 项目概述

### 1.1 项目定位

kapOJ 是一个融合 **竞技评测 + 游戏化成长体系 + AI 辅助** 的新一代 Online Judge 平台。与传统的 OJ 不同，kapOJ 在保证严谨评测能力的基础上，通过成就、任务、徽章、称号、排行榜等机制，让刷题过程更有趣味性和持续性。

### 1.2 核心目标

1. 提供稳定、安全、多语言的代码在线评测能力。
2. 构建完整的题库与比赛系统，支持主流赛制。
3. 通过游戏化机制（成就/任务/徽章/称号/金币）提升用户粘性。
4. 引入 AI 能力，降低刷题与出题门槛。
5. 支持证书认证，为竞赛选手提供可信的能力背书。

### 1.3 用户角色

| 角色       | 权限说明                                                       |
| ---------- | -------------------------------------------------------------- |
| 游客       | 浏览题目、比赛、排行榜、学习路线；不可提交                     |
| 注册用户   | 提交代码、参与比赛、完成任务、获取成就、购买装扮、使用 AI 辅助 |
| 题目管理员 | 管理题库、测试数据、学习路线                                   |
| 比赛管理员 | 创建/管理比赛、发布公告                                        |
| 超级管理员 | 全部权限，含成就/任务/徽章/称号/证书管理、用户管理             |

> **RBAC 实现说明（2026-11-06）**：上述固定角色已升级为「多角色 + 全局权限」模型。`users.role` 与 JWT 中的 `role` 是 legacy 兼容字段，**自 RBAC-C 起不再参与任何管理授权，仅用于展示/兼容**；管理授权只认 `user_roles` 关联的角色与 `role_permissions`。内置角色、权限目录与映射见 4.7。角色/用户角色/审计/统计管理 API（RBAC-B）、内容管理接口迁移（RBAC-C）与管理后台 UI（RBAC-D）均已完成。

---

## 2. 技术栈与总体架构

### 2.1 推荐技术栈

| 层次     | 技术选型                                                   | 说明                                                                             |
| -------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------- |
| 前端     | Vue 3 + TypeScript + Vite + Pinia + PrimeVue 4 + ECharts | SPA，响应式，榜单/统计用 ECharts；Element Plus 已完全移除 |
| 编辑器   | md-editor-v3（题面 Markdown）、Monaco Editor（代码）       | 统一题面与代码编辑方案；替代早期的 mavon-editor（维护不活跃，对 Vue 3 支持有限） |
| 后端     | Go 1.22+ + Gin + GORM                                      | 高性能、高并发、编译为单一二进制部署（既定方案，不随文档调整而变更）             |
| 数据库   | PostgreSQL 17                                              | 主存储                                                                           |
| 缓存     | Redis                                                      | 会话、排行榜、限流、任务队列                                                     |
| 消息队列 | Asynq（基于 Redis）                                        | 评测任务异步调度                                                                 |
| 评测沙盒 | 独立评测服务 + Docker 容器隔离                             | 多语言编译运行，资源限制                                                         |
| 对象存储 | MinIO / 本地文件系统                                       | 题目数据、徽章图片、证书                                                         |
| AI 对接  | OpenAI 兼容 API                                            | DeepSeek / OpenAI / Qwen / GLM 统一接入                                          |
| 部署     | Docker Compose / K8s                                       | 前后端、评测服务容器化                                                           |

**实现差异（M0）**：`apps/server/go.mod` 要求 Go 1.26，因为最新版 go-redis 与 golang.org/x/* 依赖需要它；仍符合「Go 1.22+」。前端 TypeScript 固定为 5.9，因为 TypeScript 7 不再提供 vue-tsc 与 openapi-typescript 依赖的编译器 API。

**前端 UI 栈（2026-11-08，迁移已完成）**：默认 UI 组件库已切换为 PrimeVue `4.5.5`（`@primeuix/themes 1.2.5`、`primeicons 7.0.0`），并落地暗色优先的科技风 tokens、`AppPageHeader` / `AppSurface` / `AppFormField` 等共享组件、框架无关的 `validation.ts` 与局部中文 PrimeVue locale。现有 UI 任务 UI-00～UI-20 已全部完成迁移，M4-C 新增的排行榜页、`RatingLevelName`、`RatingHistoryCard` 与签到卡片直接使用 PrimeVue 4（UI-30）；Element Plus 依赖、`main.ts` 引导与过渡样式已彻底移除，legacy allowlist 清空。详细 backlog、设计决策与逐项验证见 [`docs/primevue-migration.md`](docs/primevue-migration.md)。下文历史提示词中的 Element Plus 为初始方案草案，当前实现以本段与迁移文档为准。

### 2.2 系统架构图

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Web 前端     │────▶│  API 网关     │────▶│  业务后端     │
│  (Vue3)      │     │  (Go/Gin)    │     │  (Go/Gin)    │
└──────────────┘     └──────────────┘     └──────┬───────┘
                                                 │
                    ┌────────────────────────────┼────────────────────────────┐
                    │                            │                            │
              ┌─────▼─────┐              ┌───────▼───────┐            ┌───────▼───────┐
              │ PostgreSQL │              │     Redis      │            │  MinIO/存储    │
              └───────────┘              └───────┬───────┘            └───────────────┘
                                                 │
                                          ┌──────▼──────┐
                                          │ 评测服务      │
                                          │ (Asynq 消费者)│
                                          └──────┬──────┘
                                                 │ Docker 沙盒
                                        ┌────────▼────────┐
                                        │ 编译运行容器      │
                                        │ C/C++/Py/Java/Go │
                                        └─────────────────┘
```

图中的 API 网关是逻辑层：初期由 `apps/server` 中的 Gin 中间件（鉴权、限流、日志、统一错误处理）承担，不单独部署。

### 2.3 仓库结构（Monorepo）

```
kapoj/
├── apps/
│   ├── web/            # Vue3 前端
│   ├── server/         # Go 后端（Gin + GORM）
│   └── judge/          # 评测服务（Go，独立进程，消费 Asynq 队列）
├── packages/
│   └── api/            # OpenAPI 规范，供前端生成 TypeScript 类型
├── docker/             # Dockerfile、docker-compose
└── docs/               # 文档
```

**当前状态（M4-3）**：已建立 `apps/web`、`apps/server`、`apps/judge`、`packages/api`、`docker`、`docs`。后端按领域分包，位于 `apps/server/internal/`：`config`、`httpx`（统一响应与错误码）、`health`、`db`、`dbtypes`（jsonb 列表类型）、`storage`（文件存储抽象与本地实现）、`user`（模型、仓储、校验、资料服务）、`auth`（令牌与认证服务）、`middleware`、`userapi`（用户 HTTP 接口）、`problem`（题目模型、仓储、校验、服务）、`problemapi`（题目 HTTP 接口）、`submission`（提交模型、仓储、服务、限流）、`submissionapi`（提交 HTTP 接口）、`submissionevents`（Redis→SSE）、`gamification`（金币、成就、任务、徽章、称号与公开身份）、`gamificationapi`（游戏化 HTTP 接口）、`gamequeue`（游戏事件契约与 worker）、`judgequeue`（Asynq 队列契约）、`rating`（Rating 模型、等级、Elo 结算、扫描器、榜单与历史）、`ratingapi`（Rating HTTP 接口）、`activity`（活跃度、签到金币、共享积分与活跃度/共享榜）、`activityapi`（活跃度/共享榜与签到 HTTP 接口）、`social`（关注、通知、点赞服务）、`socialapi`（关注/通知 HTTP 接口）、`router`。`apps/judge` 是独立 Go 模块与进程，包含 `config`、`db`、`model`、`queue`、`events`、`gamequeue`、`reconcile`、`storage`、`verdict`、`sandbox`、`pipeline`、`worker`。本地开发时，docker compose 中的 PostgreSQL 映射到宿主机 5433 端口，Redis 映射到 6380 端口，启动方式见 `README.md`。

---

### 2.4 通用约定

- **主键**：统一使用 bigint 自增主键，接口中的 ID 与数据库保持一致。
- **时间**：数据库使用 `timestamptz`（UTC 存储），接口返回 RFC 3339 格式。时间戳字段统一为 `created_at` / `updated_at`。
- **响应格式**：`{ "code": 0, "message": "ok", "data": ... }`；出错时 `code` 非 0，并返回错误码。
- **分页**：请求参数为 `page`、`page_size`，返回 `total` 与 `items`。
- **金币一致性**：金币余额变动必须与 `coin_transactions` 写入在同一数据库事务中完成。
- **幂等**：事件驱动的奖励（成就、任务、Rating 结算）可能被重复执行，必须依靠唯一约束保证结果只生效一次。
- **删除策略**：已有提交记录的题目、比赛不做物理删除，改为下线（`is_public=false`）或归档。
- **接口前缀**：业务接口统一挂在 `/api` 下（如 `/api/auth/login`），`/health` 不加前缀。本文档各模块「关键接口」表中的路径省略了 `/api`。
- **字段命名**：接口 JSON 字段使用 camelCase（如 `avatarUrl`、`createdAt`），数据库列使用 snake_case。
- **请求体大小**：JSON 请求默认上限 1 MiB；题目创建/更新因为题面与样例可能较大，单独放宽到 4 MiB；测试数据上传接口上限 40 MiB（单个输入/输出文件 16 MiB，checker 256 KiB）。题包、图片等上传接口在各自里程碑单独放宽。

**错误码**（M0 已实现，见 `apps/server/internal/httpx/errors.go`；新模块在此表追加）：

| code  | HTTP | 含义                         |
| ----- | ---- | ---------------------------- |
| 0     | 200  | 成功（创建类接口返回 201）   |
| 40000 | 400  | 请求体格式错误               |
| 40001 | 400  | 参数校验失败（message 说明） |
| 40100 | 401  | 未登录或令牌无效             |
| 40101 | 401  | 账号或密码错误               |
| 40300 | 403  | 权限不足                     |
| 40301 | 403  | 账号已封禁                   |
| 40400 | 404  | 资源不存在                   |
| 40901 | 409  | 用户名已被占用               |
| 40902 | 409  | 邮箱已被注册                 |
| 40903 | 409  | 题目标识已存在               |
| 41300 | 413  | 请求体过大                   |
| 42901 | 429  | 登录失败次数过多             |
| 42902 | 429  | 提交过于频繁                 |
| 50000 | 500  | 服务器内部错误               |
| 50300 | 503  | 依赖服务不可用               |

---

## 3. 数据库总体设计

> 以下为关键表设计（字段示例，具体以代码实现为准）。字段名与枚举值以本文档为准；代码实现偏离时须在本文档中注明「实现差异」。

### 3.1 用户与认证

```sql
users (
  id, username, nickname, email, password_hash, avatar_url,
  role,            -- legacy 兼容字段：user / problem_admin / contest_admin / super_admin；自 RBAC-C 起仅用于展示，不再参与管理授权
  rating,          -- 当前比赛 Rating（由 rating_history 结算得出，此处为缓存）
  coins,           -- 金币余额，不允许为负（CHECK coins >= 0）
  bio, status,     -- active / banned
  created_at, updated_at
)
-- 唯一约束：username、email

-- 实现差异（M0）：username 与 email 统一存小写，因此唯一性不区分大小写；
-- 唯一索引名为 uniq_users_username、uniq_users_email，CHECK 约束名为 chk_users_coins_non_negative；
-- 新用户 rating 默认 1500。
-- 实现差异（M4-A）：rating_history 已由 db.Migrate 创建，唯一索引名为 uniq_rating_history_user_contest，
-- 使用部分唯一索引 (user_id, contest_id) WHERE contest_id IS NOT NULL；contest_id 可空，kind 本阶段只写 contest。
-- 实现差异（M4-4）：rating_history 增加 period_key（默认 ''，比赛结算恒为空串），新增部分唯一索引
-- uniq_rating_history_user_period：(user_id, kind, period_key) WHERE period_key <> ''；
-- kind 扩展为 contest / decay / season_reset，衰减与重置行 contest_id=NULL、rank=0、period_key 非空。

rating_history (
  id, user_id, contest_id NULL, kind, rank, rating_before, rating_after, period_key, created_at
)
-- 唯一约束：(user_id, contest_id) WHERE contest_id IS NOT NULL，保证同一场比赛的 Rating 只结算一次；
--           (user_id, kind, period_key) WHERE period_key <> ''，保证同一用户同一 kind+period 只写一行；
-- kind：contest（比赛结算，period_key=''）/ decay（月度衰减，period_key=YYYY-MM）/ season_reset（赛季重置，period_key=赛季开始日）

-- M4-4 新增：Rating 衰减/赛季设置单例与幂等运行标记
rating_settings (
  id,                -- 固定 1，单例
  decay_enabled, decay_threshold, decay_inactive_days, decay_percent, decay_max,
  season_enabled, season_months, season_reset_percent, season_anchor,
  updated_by, updated_at
)
-- 默认值见 5.5；首次读取时若不存在则按默认值创建（ON CONFLICT DO NOTHING），
-- 并把该锚点下「上一赛季」运行键预标记为已跳过，避免首次部署立即触发历史赛季重置。

rating_runs (
  key         text primary key,   -- decay:YYYY-MM / season:YYYY-MM-DD（赛季开始日）
  kind        text not null,      -- decay / season_reset
  processed_at timestamptz null,  -- 空表示已认领未完成，可被下次扫描重试
  created_at  timestamptz not null
)
-- key 唯一保证幂等；与 rating_history 的部分唯一索引配合，重复 tick/多实例不重复扣分/重置。

-- RBAC-A 新增（2026-11-06）
roles (
  id, code, name, description, is_system, created_at, updated_at
)
-- code 唯一；内置角色 is_system=true，code/name 不可改

permissions (
  id, code, name, group_name, description, sort_order, created_at
)
-- code 唯一；权限码稳定不可改名

role_permissions (
  id, role_id, permission_id
)
-- 唯一约束：(role_id, permission_id)

user_roles (
  id, user_id, role_id, assigned_by NULL, assigned_at
)
-- 唯一约束：(user_id, role_id)；assigned_by 为空表示由 Seed 迁移

admin_audit_logs (
  id, actor_id, action, target_type, target_id NULL, summary, ip, user_agent, created_at
)
-- actor/action/created_at 建索引；审计写入由 RBAC-B（敏感操作）与 RBAC-C（内容管理）实现

rbac_meta (
  key, value
)
-- key 为主键；Seed 写入 key=seed_version、value=1

-- 实现差异（RBAC-A）：
-- users.role 继续保留，是 legacy 兼容字段：自 RBAC-C 起不再参与任何管理授权，仅用于展示；
-- RBAC 权限判断只认 user_roles/role_permissions；
-- 权限来源为 users → user_roles → roles → role_permissions → permissions 的并集；
-- 唯一索引为 uniq_roles_code、uniq_permissions_code、uniq_role_permissions、uniq_user_roles；
-- 首次 Seed 的判定标记为 rbac_meta.seed_version，详细语义见 4.7。
```

### 3.2 题库与题目

```sql
problems (
  id, title, slug, statement_md,        -- 题目描述 Markdown
  input_format_md, output_format_md,    -- 输入输出格式
  samples_json,                          -- 结构化样例（用于展示与导入校验），须与 statement_md 中的样例一致
  hint_md, solution_md, solution_code,   -- 提示/官方题解说明/官方题解代码
  solution_price_coins,                  -- 官方题解解锁价格（金币）；0 表示免费，范围 0..1,000,000
  type,                                  -- traditional / special / interactive
  tags_json, difficulty,
  time_limit_ms, memory_limit_mb,
  checker_path,                          -- special 题的 checker 文件路径（MinIO），其他类型为空
  is_public, source, submit_count, accept_count,
  created_by, created_at, updated_at
)

problem_testcases (
  id, problem_id, input_file, output_file, score, is_sample, sort_order
)
-- 唯一约束：(problem_id, sort_order)

-- 题目资源区新增（2026-11-07，官方题解与解题思路论坛）
problem_solution_unlocks (
  id, user_id, problem_id, price_paid, created_at
)
-- 唯一约束：(user_id, problem_id)；解锁永久有效，创建者改价不影响已解锁用户

problem_solution_posts (
  id, problem_id, user_id, title, content_md,
  created_at, updated_at, deleted_at
)
-- deleted_at 软删除；列表/详情过滤已删除

problem_solution_replies (
  id, post_id, user_id, content_md,
  created_at, updated_at, deleted_at
)
-- deleted_at 软删除；帖子删除后其回复整体不可见

-- 实现差异（M1-A）：
-- 存储：M1 使用本地文件系统（KAPOJ_STORAGE_DIR，默认 data），通过 storage.Storage 接口抽象，MinIO 实现推迟到 M3/M5；
--   checker_path 与测试点的 input_file / output_file 均为存储键（测试点如 problems/<id>/testcases/<hex>.in，
--   checker 如 problems/<id>/checkers/<hex>.cpp），不返回给前端；重新上传 checker 会写新键并在成功后删除旧文件。
-- problem_testcases 增加 input_size、output_size（字节数，供管理页展示）；
-- 唯一索引名为 uniq_testcases_problem_order，problems.slug 唯一索引名为 uniq_problems_slug，
-- tags_json 建 GIN 索引 idx_problems_tags，外键名为 fk_problems_testcases（ON DELETE CASCADE）。
-- samples_json、tags_json 为 jsonb，空值存 []（不是 NULL）。
```

### 3.3 提交与评测

```sql
submissions (
  id, user_id, problem_id, contest_id,   -- contest_id 可空（练习）
  language, code, status,                -- 取值见下方「判题状态」
  time_used_ms, memory_used_kb,
  score, error_message, judge_result_json,
  created_at, judged_at
)
-- 索引：(problem_id, status)、(user_id, created_at)、(contest_id, user_id)

-- 实现差异（M1-B）：
-- contest_id 暂不设外键，M2 建 contests 表后补；
-- 索引名为 idx_submissions_problem_status、idx_submissions_user_created、idx_submissions_contest_user；
-- judge_result_json 为 jsonb 数组，元素形如
--   {"testcaseId":1,"status":"accepted","timeMs":12,"memoryKb":345,"message":"..."}；
-- submit_count 在创建提交的同一事务中 +1；accept_count 只在 judging -> accepted 时 +1，
--   且 Finish 带 status='judging' 条件，重复任务不会重复计数；
-- 提交代码上限 64 KiB；创建提交按用户限流（默认冷却 3 秒）。
-- 实现差异（M2-B）：
-- contest_id 为空/0 表示练习，>0 表示比赛提交；创建时校验比赛进行中、已报名、题目属于比赛；
-- 比赛进行中，非本人/非管理员查询他人比赛提交时不返回代码、错误信息、测试点详情、耗时、内存、得分；
-- 比赛结束后恢复完整详情；OI 赛时非管理员榜单只返回本人行；
-- 比赛已有提交时禁止删除；比赛开始后禁止整体替换题目列表。
```

**判题状态**（全平台统一，`submissions.status` 的取值）：

| 状态值                | 含义                               |
| --------------------- | ---------------------------------- |
| pending               | 已入队，等待评测                   |
| judging               | 评测中                             |
| accepted              | 通过（AC）                         |
| wrong_answer          | 答案错误（WA）                     |
| time_limit_exceeded   | 超时（TLE）                        |
| memory_limit_exceeded | 超内存（MLE）                      |
| runtime_error         | 运行时错误（RE）                   |
| compile_error         | 编译错误（CE）                     |
| system_error          | 评测系统内部错误（SE），可重新评测 |

### 3.4 比赛

```sql
contests (
  id, title, description_md, start_time, end_time,
  mode,                -- icpc / ioi / oi
  is_rated, is_public, password_hash,
  register_type,                         -- open / password / invite
  rating_settled_at,                     -- Rating 结算完成时间，可空
  created_by, created_at, updated_at
)
contest_problems (
  id, contest_id, problem_id, display_id, sort_order, score
)
-- 唯一约束：(contest_id, problem_id)、(contest_id, display_id)
contest_participants (
  id, contest_id, user_id, register_time, extra_info_json
)
-- 唯一约束：(contest_id, user_id)
-- 实现差异（M2-A）：
-- contests 增加 invite_code_hash（邀请码报名）；password_hash / invite_code_hash 只存 bcrypt；
-- contest_problems 唯一约束为 uniq_contest_problems_problem、uniq_contest_problems_display；
-- contest_participants 唯一约束为 uniq_contest_participants_user，extra_info_json 存 []；
-- 比赛无提交时删除级联删除题目关联、报名与公告；已有提交时禁止物理删除（M2-B）。
-- 实现差异（M4-A）：contests 增加 rating_settled_at（timestamptz NULL），结算成功后写入；
-- 比赛开始后 is_rated 冻结；克隆比赛会清空 rating_settled_at，使克隆场次重新具备结算资格。
announcements (
  id, contest_id, title, content_md, created_by, created_at
)
```

### 3.5 游戏化模块

```sql
-- 成就
achievements (
  id, code, name, description, icon_url, condition_type, condition_config_json,
  reward_coins, reward_badge_id, sort_order, is_active
)
user_achievements ( id, user_id, achievement_id, unlocked_at )
-- 唯一约束：(user_id, achievement_id)，保证奖励只发放一次

-- 任务
tasks (
  id, title, description_md, problem_ids_json, reward_coins,
  start_time, end_time, is_active, created_by
)
user_task_records ( id, user_id, task_id, status, claimed_at, completed_at )
-- status: claimed / completed；唯一约束：(user_id, task_id)

-- 徽章
badges (
  id, name, description, image_url, price_coins, category,
  is_limited, stock, sort_order, is_active
)
user_badges ( id, user_id, badge_id, is_equipped, purchased_at )
-- 唯一约束：(user_id, badge_id)；装备数量上限见 5.3

-- 称号
titles (
  id, name, color, unlock_condition_type, unlock_condition_json,
  price_coins, is_active
)
user_titles ( id, user_id, title_id, is_equipped, unlocked_at, purchased_at )
-- 唯一约束：(user_id, title_id)；同一用户同时仅 1 个称号可装备（见 5.4）

-- 金币流水
coin_transactions (
  id, user_id, amount, type,        -- earn/spend/admin_adjust
  reason, ref_type, ref_id, created_at
)
-- 唯一约束：(user_id, reason, ref_type, ref_id)，防止同一奖励被重复结算
```

**实现状态（M3 已完成）**：金币、事件、成就、任务、徽章、称号与公开身份后端已实现，成就/任务/徽章/称号前端页面、游戏化管理后台与真实浏览器验收也已完成。

- 已建表：`coin_transactions`、`achievements`、`user_achievements`、`tasks`、`user_task_records`、`user_login_days`、`badges`、`user_badges`、`titles`、`user_titles`；金币余额沿用 `users.coins`（`CHECK coins >= 0`）。
- `coin_transactions` 增加 `note` 列（管理员调整备注，1–200 字）；唯一约束仍为 `(user_id, reason, ref_type, ref_id)`，仅 `ref_type <> ''` 时生效，管理员调整不参与幂等去重。M3-B 起流水的 `reason` 增加 `badge_purchase`、`title_purchase`，`ref_type` 增加 `badge`、`title`。
- `achievements.reward_badge_id` 自 M3-B 起生效：配置时要求徽章存在且上架；发放时直接写入 `user_badges`，**不占用商城库存**，即使徽章后来下线也照常发放。更新成就时 `rewardBadgeId=null` 表示保持原奖励徽章不变。
- 徽章：管理员 CRUD 与图片上传，用户用金币购买、装备（默认最多 3 个，`KAPOJ_MAX_EQUIPPED_BADGES` 配置 1–10）。限量徽章购买锁定徽章行并扣减库存；装备上限由用户行锁串行化。图片仅支持 PNG/JPEG/WebP/GIF 且不超过 2 MiB，保存为存储键 `badges/<id>`，数据库 `image_url` 写为 `/api/badges/<id>/image`；公开读取响应带 `Cache-Control: public, max-age=3600`。
- 称号：管理员 CRUD；用户满足 `rating`、`ac_count` 或 `achievement` 条件后解锁（读取时补算，不扣金币），解锁后可用金币购买；同一用户同时最多装备 1 个称号，重复装备幂等。
- 公开身份：个人主页、测评记录列表/详情、比赛榜单按页批量查询已装备徽章与称号（默认每用户最多 3 个徽章 + 1 个称号），接口字段分别为 `equippedBadges` 与 `equippedTitle`；已装备但后来下线的徽章/称号仍展示。
- 软删除统一为 `is_active=false`，保留用户已获得的记录与金币流水。
- 事件队列为 Asynq `game`，任务类型 `game:event`，最多重试 5 次、超时 30 秒；Web 端 worker 并发由 `KAPOJ_GAME_CONCURRENCY` 配置（1–16，默认 2）。judge 投递失败只记录警告，不影响评测结果。
- 实现文件：`apps/server/internal/{gamification,gamificationapi,gamequeue}`、`apps/judge/internal/{gamequeue,pipeline}`；`auth`、`contestapi` 通过钩子记录登录日与报名事件，`userapi`、`submissionapi`、`contestapi` 接入公开身份。

### 3.6 排行榜与交互

```sql
-- 活跃度
activity_logs (
  id, user_id, action, points, ref_type, ref_id, created_at
)
-- action: submission / contest_join / signin / solution_reply / solution_like / follow
-- 每类行为的每日积分上限与去重规则见 5.5
-- 共享内容（用户创建比赛/题解）
shared_items (
  id, user_id, type,              -- contest / solution
  ref_id, like_count, created_at
)

-- 社交互动（M4-3，2026-11-08）
problem_solution_post_likes (
  id, user_id, post_id, created_at
)
-- unique(user_id, post_id)、index(post_id)；帖子软删除时清理

user_follows (
  id, follower_id, followee_id, created_at
)
-- unique(follower_id, followee_id)、index(followee_id)、check follower_id <> followee_id

notifications (
  id, user_id, actor_id, type, dedupe_key UNIQUE,
  target_type, target_id, link, read_at, created_at
)
-- type: solution_reply / solution_like / follow
-- index(user_id, created_at DESC)、index(user_id, read_at)
```

**实现状态（M4-B + M4-3）**

- 已建表：`activity_logs`、`shared_items`；唯一约束分别为 `(user_id, action, ref_type, ref_id)` 与 `(type, ref_id)`，索引为 `(user_id, created_at)` 与 `(created_at)`。
- `activity_logs.action` 支持 `submission`、`contest_join`、`signin`（M4-B）与 `solution_reply`、`solution_like`、`follow`（M4-3）；`shared_items.type` 支持 `contest`（创建/克隆比赛 +20）与 `solution`（题解每赞 +5，`like_count` 累计）。
- 评论复用论坛回复 `problem_solution_replies`（见 4.3），不新增通用 `comments` 表；点赞/关注/通知分别使用 `problem_solution_post_likes`、`user_follows`、`notifications` 三张新表（M4-3）。
- 共享项在比赛创建/克隆成功后写入，服务启动时把已有 `contests.created_by>0` 幂等回填到 `shared_items`；题解被赞时 `shared_items(type=solution)` upsert `like_count`，取消赞扣回，帖子删除移除。

### 3.7 证书认证

```sql
certificate_types (
  id bigserial primary key,
  name varchar(50) not null unique,     -- 唯一索引 uniq_certificate_types_name
  description varchar(200) not null default '',
  is_active boolean not null,           -- 无数据库默认值，创建时由服务显式赋值
  sort_order int not null default 0,
  created_at timestamptz not null,
  updated_at timestamptz not null
)
certificates (
  id bigserial primary key,
  user_id bigint not null,
  certificate_type_id bigint not null,
  certificate_type_name varchar(50) not null,  -- 提交时快照，类型改名/停用不回溯
  level varchar(50) not null,
  image_key varchar(255) not null,             -- 仅服务端使用，不通过 DTO 返回
  image_content_type varchar(100) not null,
  status varchar(20) not null default 'pending',  -- pending / approved / rejected
  reject_reason varchar(500) not null default '',
  verify_code varchar(32) null,                -- approved 后生成，唯一索引 uniq_certificates_verify_code
  submitted_at timestamptz not null,
  reviewed_at timestamptz null,
  reviewed_by bigint null,
  created_at timestamptz not null,
  updated_at timestamptz not null
)
-- 索引：idx_certificates_user_submitted(user_id, submitted_at)、idx_certificates_status_submitted(status, submitted_at)、
--       idx_certificates_type(certificate_type_id)、uniq_certificates_verify_code(verify_code)
```

**实现状态（M5-7 ✅，2026-11-08）**

- 已建表 `certificate_types`、`certificates`，字段与索引如上；首次启动且表为空时 seed 7 个内置类型（蓝桥杯、ACM-ICPC、CCCC 天梯赛、CCPC、NOI、NOIP、USACO，sort_order 1..7），已有数据不覆盖。
- `certificate_type_name` 为提交时快照，类型改名/停用/删除不回溯历史证书与公开核验；类型删除仅当无证书引用，被引用删除返回 40001「已有证书使用该类型，请先停用」。
- `verify_code` 为 `*string`，pending/rejected 为 NULL（Postgres 唯一索引允许多个 NULL）；通过审核生成 `KAPOJ-XXXX-XXXX`（crypto/rand，去 I/L/O/U/0/1 的无歧义大写字母/数字），唯一冲突重试。
- 图片仅本地 `storage.Storage` 保存（key `certificates/<id>`），≤5 MiB、magic bytes 校验仅 PNG/JPEG/WebP；`image_key`/`image_content_type` 仅存数据库，不通过 DTO 返回。
- 权限：新增第 37 个权限码 `certificate.manage`（分组「证书」，默认仅 `super_admin`），见 4.7。
- 实现文件：`apps/server/internal/{certificate,certificateapi}`；提交/我的证书/图片/公开核验/管理员审核/类型 CRUD 接口见 5.6。

### 3.8 学习路线

```sql
learning_paths (
  id, title, description_md, cover_url,
  is_official, is_public, is_active, created_by,
  sort_order, created_at, updated_at
)
learning_path_nodes (
  id, path_id, parent_id, title, description_md, problem_ids_json, sort_order,
  required_count, created_at, updated_at
  -- 完成本节点所需通过题数（默认为节点内题目总数）；parent_id 自关联，根节点为 NULL
)
user_learning_progress (
  id, user_id, path_id, node_id, status, solved_count, completed_at, updated_at
)
-- status: locked / active / completed；唯一约束：(user_id, node_id)；进度由服务端根据 AC 事件计算
```

- `is_active=false` 为软删除（下架），路线、节点与用户进度都保留；只有管理员可通过 `includeInactive=true` 查看并恢复。
- `is_official=true` 强制 `is_public=true`；官方标记仅创建时由管理员设置、创建后不可修改，上下架状态仅管理员可设置。
- 节点校验：父节点属于同一路线、不能成环、最大深度 8 层；题目 1–50 道、不重复且存在/公开；`required_count` 为 1–题目数；仅叶子节点可删除。
- **M4-2A 已完成**：模型/迁移、官方/自定义权限、路线与节点 CRUD、树结构接口。**M4-2B 已完成**：服务端进度计算（`EvaluateUser`）、AC 事件补算、读取补算、完成锁存与题目下线守卫；**M4-2C 已完成**：前端列表/树/编辑/管理页（见 4.5）。

### 3.9 AI 辅助

```sql
ai_providers (
  id, name, base_url, model_name, is_active,
  api_key_ref                            -- 密钥引用（环境变量名或密钥服务中的 ID），不存明文
)
ai_hints (
  id, problem_id, content, created_by, created_at   -- 首个用户点击后缓存
)
-- 唯一约束：problem_id（每题只缓存一份提示）
user_ai_configs (
  id, user_id, provider, model_name,
  api_key_encrypted                      -- 用户自行填写的 key，AES 加密后存储
)
```

**实现状态（M5-A ✅，M5-1～M5-3 已完成）**

- 已建表：`ai_providers`（code 唯一 `uniq_ai_providers_code`）、`user_ai_configs`（user_id 唯一 `uniq_user_ai_configs_user`）、`ai_hints`（problem_id 唯一 `uniq_ai_hints_problem`）。
- 平台级 provider 的密钥不写入数据库；`ai_providers.api_key_ref` 保留为环境变量名或密钥服务 ID 引用，当前固定为空字符串，M5-A 不实现平台代付 key。
- 内置 provider seed（幂等，`ON CONFLICT DO NOTHING`，不覆盖已有行）：`deepseek` / DeepSeek / `https://api.deepseek.com/v1` / `deepseek-chat`、`openai` / OpenAI / `https://api.openai.com/v1` / `gpt-4o-mini`、`qwen` / 通义千问 / `https://dashscope.aliyuncs.com/compatible-mode/v1` / `qwen-plus`、`glm` / 智谱 GLM / `https://open.bigmodel.cn/api/paas/v4` / `glm-4-flash`。
- `user_ai_configs.api_key_encrypted` 使用 AES-256-GCM（随机 12 字节 nonce，输出 `v1:`+base64(nonce+ciphertext)），密钥为 base64 编码的 32 字节（来自环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 或密钥文件 `KAPOJ_AI_KEY_FILE`，见 §6.7）；接口只返回脱敏值（长度 >8 显示前 3 字符 + … + 后 4 字符，否则 `****`），不写日志、不写审计摘要。
- 提示缓存：`ai_hints` 每题唯一；首个用户触发后生成，后续用户直接读取缓存，不要求用户 AI 配置、不消耗限流；非公开题对无 `problem.view_hidden` 的用户与不存在一致返回 40400，不泄露存在性。


---

## 4. 基础功能模块

### 4.1 用户与认证

**功能需求**

- 注册、登录（用户名/邮箱 + 密码）、JWT 鉴权、刷新令牌。refresh token 保存在 Redis 中，登出或修改密码后立即吊销。
- 登录失败次数限制（如 5 分钟内最多 10 次），防止暴力破解。
- 个人中心：查看/编辑资料、头像上传、我的成就/徽章/称号/金币、装扮管理。
- 角色与权限（RBAC）：多角色 + 全局权限；`users.role` 保留为 legacy/展示字段，权限由 `roles`/`permissions` 实时解析；自 RBAC-C 起 legacy role 不再授权，只用于展示。内置角色、权限码与迁移语义见 4.7。

**关键接口**

| 方法 | 路径               | 说明                            |
| ---- | ------------------ | ------------------------------- |
| POST | /auth/register     | 注册                            |
| POST | /auth/login        | 登录，返回 access/refresh token |
| POST | /auth/refresh      | 刷新令牌                        |
| POST | /auth/logout       | 登出，吊销 refresh token        |
| GET  | /users/me          | 当前用户信息                    |
| PUT  | /users/me          | 更新资料                        |
| GET  | /users/:id/profile | 查看他人主页                    |
| GET  | /admin/users       | 用户分页列表（`user.read`，见 4.7） |

**实现状态（M0 ✅）**：上表接口均已实现，完整定义见 `packages/api/openapi.yaml`。

**实现差异**：

- 注册成功后直接返回 `{ accessToken, refreshToken, user }`，即自动登录，与登录响应相同。
- 登录请求字段为 `account`（用户名或邮箱）和 `password`。
- 校验规则：用户名 3–32 位字母、数字或下划线；邮箱不超过 254 字节；密码 8–72 字节（72 是 bcrypt 的上限）；昵称 1–32 个字符，为空时默认等于用户名；简介不超过 200 个字符；头像链接为 http(s) 且不超过 512 字符。
- access token 为 HS256 JWT，有效期 15 分钟；refresh token 为随机串，有效期 7 天，每次刷新都会轮换，旧令牌立即失效。Redis 中只保存其 SHA-256。
- 登录尝试限制：每个用户 5 分钟内最多 10 次。每次尝试在校验密码前原子计数，并发请求无法绕过；用户名和邮箱登录共用同一个计数。超过后即使密码正确也返回 42901，登录成功后清零。账号不存在与密码错误返回同一个错误。
- `/auth/*` 接口不校验 Authorization 头，由请求体中的 refresh token 识别用户，因此 access token 过期后仍可登出和刷新。
- 注册与登录成功后触发游戏化登录钩子（M3-A）：按服务端本地日期记录登录日，并投递 `user.login` 事件用于连续登录成就。钩子失败只记录警告，不影响注册或登录响应。
- 前端在多个标签页之间同步登录状态；某个标签页轮换令牌后，其他标签页会改用新令牌，而不是被登出。
- 修改资料只写入被修改的字段，不会覆盖同时发生的封禁、角色或金币变动。
- 被封禁用户无法登录或刷新令牌；已签发的 access token 在过期前（最长 15 分钟）仍然有效。
- `GET /users/:id/profile` 不返回邮箱和角色；M3-B 起返回真实的 `equippedBadges`（数组，最多 `KAPOJ_MAX_EQUIPPED_BADGES` 个）与 `equippedTitle`（无装备时为 null），已装备但后来下线的徽章/称号仍展示。
- M3-C 起个人主页昵称旁用 `IdentityShowcase` 展示装备称号与徽章；用户菜单显示金币余额并提供「装扮管理」入口。金币余额以服务端 `GET /users/me` 为准，购买或装备后刷新。
- `GET /admin/users` 原为超级管理员的用户分页列表；RBAC-B 起改为 `user.read` 权限（super 绕过），并新增 keyword/status/roleId 筛选，详见 4.7。该接口也用于验收「管理员接口对无权限用户返回 403」。
- 头像上传依赖 MinIO，**未实现**，安排在 M5-8；目前只能填写头像链接。修改密码接口**未实现**。

**实现状态（RBAC-A，2026-11-06）**

- `GET /users/me` 增加 `roles`（`[{id, code, name}]`）、`permissions`（权限码字符串数组，按字典序排序）与 `isSuperAdmin`。登录、注册、更新资料等其他响应中的 `roles`/`permissions` 默认空数组、`isSuperAdmin=false`，前端登录后应调用 `/users/me` 获取最新权限。
- 权限解析使用 `rbac.Service.Access`：每次请求实时查询数据库，要求用户存在且 `status=active`；多角色权限取并集；任一角色为 `super_admin` 时 `isSuperAdmin=true`，中间件直接放行。无角色时 `permissions` 为空数组。
- 封禁用户的 `/users/me` 保持普通接口既有行为：仍返回基础资料，但 `roles`/`permissions` 为空、`isSuperAdmin=false`；权限中间件对封禁用户返回 403。
- `middleware.RequirePermission(resolver, codes...)` 与 `RequireAnyPermission` 已实现：未登录 401，解析失败/封禁/权限不足 403，super 绕过；通过后把 `*rbac.Access` 写入 context，可用 `CurrentAccess` 读取。
- **当前范围（RBAC-A，历史记录）**：RBAC-A 只提供权限基础设施；管理接口迁移到权限码、Viewer 能力拆分与内容管理审计随后由 RBAC-C 完成，管理后台 UI 属于 RBAC-D。因此现阶段既有管理接口已全部改为权限码授权，无需再依赖 `users.role` 为对应管理员；`/users/me` 与权限中间件识别角色依赖 `user_roles` 关联（首次 Seed 会按旧角色自动映射，见 4.7）。

**实现状态（RBAC-B，2026-11-06）**

- 新增 `/api/admin/*` 管理接口，全部使用 `RequirePermission`（super 绕过但仍写审计）：角色 CRUD/权限矩阵、权限目录、用户列表/详情/编辑/角色分配/封禁/解封、审计查询、仪表盘统计，接口清单与权限码见 4.7。
- 敏感变更（角色/权限、用户角色分配、封禁/解封、管理员编辑用户）在同一事务内写审计，失败回滚不留下审计；审计摘要为中文，不含密码/token/代码/测试数据。
- 保护规则：系统角色不可删除、code/name 不可改；`super_admin` 权限不可修改；只有 super 能授予/移除/封禁/解封 super；禁止封禁自己；禁止移除/封禁最后一个 active super（事务 + PostgreSQL advisory lock 串行化）。详见 4.7。

**Codex 提示词（用户与认证）**

```text
你是一名资深全栈工程师，请为 kapOJ（一个基于 Go (Gin) + PostgreSQL + Vue3 的 Online Judge 平台）实现「用户与认证」模块。

技术栈：后端 Go + Gin + GORM，前端 Vue3 + TypeScript + Pinia + Element Plus，鉴权使用 JWT（access token + refresh token）。

请完成以下内容：
1. 数据库模型 User：字段包含 id、username（唯一）、nickname、email（唯一）、password_hash、avatar_url、role（枚举：user/problem_admin/contest_admin/super_admin）、rating、coins、bio、status、created_at、updated_at。使用 bcrypt 加密密码。
2. 后端接口：
   - POST /auth/register：注册（校验用户名/邮箱唯一性，密码强度至少 8 位）
   - POST /auth/login：登录，返回 { accessToken, refreshToken, user }
   - POST /auth/refresh：用 refresh token 换取新 access token
   - GET /users/me：返回当前登录用户
   - PUT /users/me：更新 nickname、bio、avatar_url
   - GET /users/:id/profile：返回用户公开信息（含 rating、金币数、装备中的徽章/称号）
3. 实现 JWT 鉴权中间件与角色校验中间件，支持游客/用户/管理员三级权限。
4. 前端实现：登录页、注册页、路由守卫、Pinia 用户状态管理、个人中心资料编辑页。
5. 验收标准：能注册登录，刷新令牌有效，未登录访问受保护接口返回 401，管理员接口对普通用户返回 403。

请直接输出完整可运行代码，包含必要的 GORM 模型定义与数据库迁移。
```

---

### 4.2 代码测评沙盒

**功能需求**

- 支持语言：C/C++、Python3、Java、Go。
- 编译 + 运行 + 测试样例自动比对（含 TLE/MLE/RE/WA/AC/CE 判定）。
- 严格的资源限制：CPU 时间、内存、输出大小、进程数、网络禁用。
- 评测任务异步化：后端只负责入队，评测服务消费队列并在 Docker 沙盒中执行。

**评测流程**

```
提交 → 后端校验 → 写入 submissions(status=pending) → 入队(Asynq)
     → 评测服务取任务 → 拉取题目测试数据 → Docker 沙盒编译/运行
     → 逐样例比对 → 汇总判题结果 → 写回 submissions + 更新题目统计
```

**关键设计**

- 沙盒隔离：每个评测在独立 Docker 容器中运行，限制 `--memory`、`--cpus`、`--network none`、超时 `timeout`。
- 特判（Special Judge）：题目 type 为 `special` 时，调用 `problems.checker_path` 指向的 checker 判定输出。
- 判题状态机：`pending → judging → 终态`，终态取值与 3.3 节「判题状态」一致。
- 失败重试：因系统原因（非用户代码问题）导致的失败由 Asynq 重试，超过次数后标记为 `system_error`。
- 权限隔离：评测服务需要调用 Docker，应部署在独立主机或使用 rootless Docker，不与 Web 服务共享宿主机权限。

**实现状态（M1-B）**

- Web 服务只做校验、写 `submissions(status=pending)` 与入队；`POST /submissions` 要求登录，交互题、隐藏题对普通用户、无测试点或特判题缺 checker 会被拒绝。
- 队列：Asynq，任务类型 `judge:submission`，队列名 `judge`，payload 为 `{"submissionId": <id>}`，最多重试 3 次。入队失败时提交被标记为 `system_error` 并返回 500。
- 评测进程：`apps/judge`，可独立部署；从 PostgreSQL 读取提交/题目/测试点，从共享 `KAPOJ_STORAGE_DIR` 读取输入、答案与 checker。
- 语言与镜像：C/C++ `gcc:14`、Python3 `python:3.12-slim`、Java `eclipse-temurin:21-jdk`、Go `golang:1.26`；镜像可用 `KAPOJ_JUDGE_*_IMAGE` 覆盖。
- 沙盒：每个测试点使用独立容器；`--network none`、`--read-only`、`--cap-drop ALL`、`--pids-limit`、`--cpus 1`、`--memory/--memory-swap`，`/work`、`/tmp`、`/out` 使用 tmpfs。源码、编译产物、输入通过 `docker exec` + tar/cat 流传输，不使用宿主机 bind mount，因此 judge 自身在容器中也能运行。
- 计时防篡改：运行容器的 wrapper 以 root 启动，`/out`、`/work` 归 root 且选手进程不可写；选手程序通过 `setpriv` 降权到 UID 65532，只有 `/tmp` 可写。耗时、内存峰值、OOM 计数由 root wrapper 写入；测量文件缺失或无法解析视为系统错误，不当作 0。
- 资源：编译超时 30 秒、编译内存 1 GiB；运行时间上限为题目限制 + 3 秒容器宽限，实际耗时在容器内测量；输出上限 1 MiB；内存峰值读取 cgroup v2 `memory.peak`，OOM 通过 `memory.events` 判定。若内核不提供 cgroup v2，内存峰值记为 0，但 OOM 仍通过退出码识别；Java/Python/Go/C++ 的 OOM 堆栈（如 `OutOfMemoryError`、`MemoryError`、`std::bad_alloc`）也会判定为 MLE。
- 结论：编译失败 `compile_error`；运行超时 `time_limit_exceeded`；OOM `memory_limit_exceeded`；非零退出 `runtime_error`；输出超限或文本比对失败 `wrong_answer`；全部通过 `accepted`。系统原因（Docker/存储/DB/checker 编译失败）重试，耗尽后 `system_error`。
- 特判：checker 以 C++ 编译，argv 为 `<checker> <输入> <选手输出> <标准答案>`；退出 0 通过，退出 1 答案错误，其他退出码（含信号、126/127）或输出超限属于系统错误；checker 超时/OOM/编译失败也属于系统错误。checker 编译诊断不写入日志或 `error_message`，避免泄露判题逻辑。
- 超时与重试：任务在 Asynq 中设置 10 分钟超时，最多重试 3 次；judge 的 `ShutdownTimeout` 为 2 分钟，进程启动与关闭时都会按实例标签清扫容器。实例标签取 `KAPOJ_JUDGE_INSTANCE`（默认主机名），多实例部署时必须各不相同。
- 崩溃恢复：judge 启动后及每分钟扫描一次：创建超过 1 分钟仍为 `pending`、或超过 15 分钟仍为 `judging` 的提交会重新入队；`Claim`/`Finish` 的幂等性保证不会重复计数。这同时覆盖“数据库已提交但入队前进程崩溃”和异步队列丢失两种情况。
- 日志脱敏：GORM 使用 `ParameterizedQueries`，慢查询/错误日志只出现占位符，不包含源代码或密码哈希。
- 前端提交页：`/problems/:id/submit` 使用 Monaco（本地 worker、无 CDN），支持 c/cpp/python3/java/go，代码草稿按题目与语言保存在浏览器本地；提交后跳转测评记录。
- 统计：`submit_count` 创建提交时递增；`accept_count` 仅在首次把提交从 `judging` 改为 `accepted` 时递增。
- 已知限制：M1 不评测交互题；Java 堆上限按题内存减 64 MB（最低 16 MB）；运行时间按墙钟计时而非 CPU 时间；Docker socket 等价于宿主机 root 权限，生产需独立评测主机或 rootless Docker。

**Codex 提示词（代码测评沙盒）**

```text
你是一名评测系统专家，请为 kapOJ 实现「代码测评沙盒与评测服务」。

要求：
1. 使用 Docker 作为隔离沙盒，支持 C/C++（gcc/g++）、Python3、Java（OpenJDK）、Go 四种语言的编译与运行。
2. 评测服务为独立 Go 进程，使用 Asynq 从 Redis 队列消费评测任务。
3. 每个评测任务流程：
   - 读取提交记录（含 code、language、problem_id）与题目测试数据
   - 编译（如需要），编译失败判定 compile_error，返回编译器错误信息
   - 对每个测试样例：在 Docker 容器内运行程序，输入重定向，限制 CPU 时间、内存、输出大小，禁用网络
   - 比对 stdout 与期望输出（忽略行尾空白），判定 AC/WA/TLE/MLE/RE
   - 汇总结果写回 submissions 表，更新题目的 submit_count / accept_count
4. 实现判题状态枚举与状态机；记录每个样例的耗时与内存峰值。
5. 安全要求：沙盒禁用网络、限制进程数、设置只读文件系统（除必要目录）、防止 fork bomb 与超时僵尸进程。
6. 提供评测服务、Dockerfile 与 docker-compose 集成。
7. 验收标准：四类语言简单题目均能正确判题，TLE/MLE/RE/WA/AC/CE 六类结果可复现。

请输出完整代码、Dockerfile、docker-compose 配置与测试用例。
```

---

### 4.3 题库与题目模块

**功能需求**

- 题目 CRUD：创建、修改、删除（仅题目管理员及以上）。
- 题目信息：名称、编号、类型、标签、时间限制、内存限制；描述/输入输出格式/样例用 Markdown 编辑。
- 题目类型：传统题（traditional）、特判题（special）、交互题（interactive）。
- 题目页面右侧展示：题目提示、解题思路、解题代码、提交统计、关联比赛。
- 支持按标签、难度、关键词筛选与分页。
- 已有提交记录的题目不做物理删除，只能下线（`is_public=false`）。

**关键接口**

| 方法   | 路径                     | 说明                                   |
| ------ | ------------------------ | -------------------------------------- |
| GET    | /problems                | 题目列表（筛选/分页）                  |
| POST   | /problems                | 创建题目（管理员）                     |
| GET    | /problems/:id            | 题目详情                               |
| PUT    | /problems/:id            | 更新题目（管理员）                     |
| DELETE | /problems/:id            | 删除题目（管理员）；有提交记录时仅下线 |
| GET    | /problems/:id/statistics | 提交统计                               |
| GET    | /problems/:id/testcases  | 测试点列表（管理员）                   |
| POST   | /problems/:id/testcases  | 上传测试点（管理员，multipart）        |
| PUT    | /problems/:id/testcases/:tid | 修改分数或样例标记（管理员）       |
| POST   | /problems/:id/testcases/reorder | 调整测试点顺序（管理员）        |
| DELETE | /problems/:id/testcases/:tid | 删除测试点（管理员）               |
| POST   | /problems/:id/checker    | 上传特判 checker（管理员，仅特判题）   |
| GET    | /problems/:id/official-solution          | 官方题解访问视图（价格/访问状态/是否可编辑可购买；有权限时返回正文） |
| PUT    | /problems/:id/official-solution          | 保存官方题解说明/代码/价格（创建者或 super） |
| POST   | /problems/:id/official-solution/purchase | 金币解锁官方题解（幂等） |
| GET    | /problems/:id/solutions                  | 解题思路帖子分页列表 |
| POST   | /problems/:id/solutions                  | 发布帖子（登录） |
| GET    | /problems/:id/solutions/:postId          | 帖子详情 + 回复 |
| PUT/DELETE | /problems/:id/solutions/:postId       | 编辑/删除帖子（作者或管理员） |
| POST/DELETE | /problems/:id/solutions/:postId/like | 点赞/取消赞（登录；自赞 40001） |
| POST   | /problems/:id/solutions/:postId/replies  | 回复帖子（登录） |
| PUT/DELETE | /problems/:id/solutions/:postId/replies/:replyId | 编辑/删除回复（作者或管理员） |

**实现状态（M1-A）**

- 后端题目 CRUD、测试数据管理、checker 上传与前端题目列表、详情、编辑页、测评记录页、Monaco 提交页均已完成；提交、评测与 SSE 实时刷新已在 M1-B/M1-C 交付。
- 权限（RBAC-C）：`POST /problems` 需要 `problem.create`；`PUT /problems/:id` 需要 `problem.update`，修改 `isPublic`（公开/下线）额外要求 `problem.publish`（仅 `problem.update` 不能发布/下线）；`DELETE` 需要 `problem.delete`；测试点与排序需要 `problem.testcase.manage`；checker 上传需要 `problem.checker.manage`。`super_admin` 总是放行。未公开题目对无 `problem.view_hidden` 者返回 40400，与不存在一致；`contest_admin` 不因比赛管理权限看到隐藏题。
- 创建时忽略 `isPublic`，新题一律未公开；发布需至少 1 个测试点，特判题还需已上传 checker；公开题目不能删除最后一个测试点。发布校验与删除保护都在题目行锁事务内完成，并发删除测试点不会让公开题目变成 0 个测试点。
- **更新是整体替换**：`PUT /problems/:id` 必须提交全部可编辑字段；未提交的字符串/数组会清空，`isPublic` 缺省为 `false`（即下线）。前端编辑页会先加载完整题目再提交。题目创建/更新的 JSON 请求体上限为 4 MiB。
- 难度为整数 1–5（入门、普及、提高、省选、NOI）。
- 列表筛选：`keyword` 同时匹配标题与 slug（不区分大小写、按字面量匹配，`%`/`_` 不作通配符）；`tag` 精确匹配整个标签；`difficulty` 精确匹配；`includeHidden` 需要 `problem.view_hidden` 权限（需携带 bearer token）。
- 题解可见性：`solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy` 返回给拥有题目管理权限（`problem.update`/`problem.delete`/`problem.publish`/`problem.testcase.manage`/`problem.checker.manage` 任一）的用户，或本人已在该题有 `accepted` 提交的普通用户；`problem.view_hidden` 只放开题面，不放开题解；`hintMd` 对所有人生效。
- 交互题允许创建与存储，M1 不参与评测；提交时的拒绝逻辑在 M1-B。
- 前端使用 md-editor-v3 编辑与渲染 Markdown，渲染结果统一经 `sanitize-html` 净化；KaTeX 与 highlight.js 使用本地实例，不加载 CDN 资源。
- 关联比赛：M2 之前详情页显示「比赛功能将在 M2 上线」。

**实现状态（题目资源区、官方题解与解题思路论坛，2026-11-07）**

- 详情页右侧的 Tabs 改为四个可点击资源卡片：官方题解、解题思路、提交统计、关联比赛；提示单独放在题面下方。右侧卡片均为整块 `RouterLink`，移动端全宽纵向排列。
- 官方题解：内容沿用 `solution_md`/`solution_code`；`PUT /problems/:id` 不再写入这两列，`ProblemInput`/`ProblemUpdateInput` 已移除 solution 字段；官方题解 `PUT /problems/:id/official-solution` 是唯一写入口，避免整体替换清空题解。`ProblemDetail` 始终返回 `solutionPriceCoins`（用于卡片显示「免费/金币 N」）。
  - 访问状态六态：`creator`（创建者/super）、`accepted`（已通过）、`purchased`（已解锁）、`free`（价格 0，游客也可看）、`locked`（登录未通过/未购买）、`guest`（游客且价格 >0）；正文只在 creator/accepted/purchased/free 返回。
  - 编辑仅创建者或 super；价格 0–1,000,000，说明 ≤50,000 字符，代码 ≤65,536 字节。
  - 购买：价格 0/已通过/创建者/super/已解锁不扣费；否则事务内写 `problem_solution_unlocks` + `gamification.SpendCoinsTx` 扣金币与流水，余额不足（40001）整体回滚；重复购买幂等不重复扣费。解锁永久有效，改价不影响已解锁用户。
  - 隐藏题：无 `problem.view_hidden`/super 返回 404，不沿用比赛参与者题面 fallback。
- 解题思路论坛：帖子列表（`created_at DESC, id DESC`，分页 20）/详情/回复（`created_at ASC`，上限 500）；游客可读，写操作要求登录。标题 1–100 字、帖子正文 1–50,000 字、回复 1–10,000 字；作者可编辑/删除自己的内容，题目创建者或 `problem.update` 管理员可删除他人帖子/回复（`canEdit`/`canDelete` 由后端按 viewer 计算）；删除为软删除，删除帖子后其回复整体不可见。M4-3 起帖子列表/详情返回 `likeCount`/`likedByMe`，登录用户可点赞/取消赞（自赞 40001、隐藏题 404），删除帖子会清理点赞与共享项，详见 4.8。
- 关联比赛：`GET /api/contests?problemId=<正整数>` 返回包含该题且当前用户可见的比赛（公开 OR 本人已报名 OR `contest.view_hidden`/super 联合可见性，不复用 `mine=true` 的互斥语义）；无参数保持原行为。
- 提交统计页复用 `GET /problems/:id/statistics`。

**Codex 提示词（题库与题目）**

```text
你是一名全栈工程师，请为 kapOJ 实现「题库与题目」模块。

技术栈：Go + Gin + GORM + PostgreSQL 后端，Vue3 + Element Plus + md-editor-v3 前端。

请完成：
1. 数据模型 Problem：id、title、slug（唯一）、statement_md、input_format_md、output_format_md、samples_json、hint_md、solution_md、solution_code、type（traditional/special/interactive）、tags_json、difficulty、time_limit_ms、memory_limit_mb、is_public、source、submit_count、accept_count、created_by、created_at、updated_at。
2. 后端接口：
   - GET /problems：支持 keyword、tag、difficulty 筛选与分页
   - POST /problems：创建（仅题目管理员，校验 slug 唯一）
   - GET /problems/:id：详情（题目描述 Markdown + 统计）
   - PUT /problems/:id：更新
   - DELETE /problems/:id：有提交记录时仅下线，否则删除（级联删除测试数据）
3. 前端：题目列表页（搜索/标签/难度筛选）、题目详情页（左侧 Markdown 渲染题目，右侧 Tab：提示/思路/解题代码/提交统计/关联比赛）、题目编辑页（Markdown 编辑器）。
4. 验收标准：管理员可完整增删改查题目，Markdown 正确渲染，筛选分页正常。

请输出完整代码与类型定义。
```

---

### 4.4 比赛模块

**功能需求**

- 比赛 CRUD 与克隆。
- 设置：比赛时间、赛制（ICPC / IOI / OI）、题目、报名用户、是否计 Rating、是否公开、访问密码。
- ICPC 风格榜单：实时排名、题目通过情况、罚时计算。
- 赛时保护：比赛进行中，其他选手的代码和测评详情不可见，只显示通过与否。
- 计入 Rating 的比赛（`is_rated=true`）在结束后触发 Rating 结算，规则见 5.5。
- 公告功能：比赛内发布公告，用于题目勘误。
- 赛制规则：
  - **ICPC**：按通过题数降序，其次总罚时升序。单题罚时 = 首次 AC 用时（分钟）+ 首次 AC 之前的未通过提交次数 × 20 分钟；未通过的题不计罚时。
  - **IOI**：每题多次提交取最高分，总分排名。
  - **OI**：仅赛时可见分数，赛后统一评测并公布。

**实现状态（M2-A）**

- 后端已完成比赛 CRUD、克隆、比赛题目整体替换、报名（open/password/invite）、报名用户列表、公告 CRUD。
- 权限（RBAC-C）：`POST /contests` 与克隆需要 `contest.create`；`PUT` 需要 `contest.update`；`DELETE` 需要 `contest.delete`；报名用户列表需要 `contest.participant.read`；公告写需要 `contest.announcement.manage`；非公开比赛、开赛前题目与榜单读取需要 `contest.view_hidden`。非公开比赛对非报名者且无 `contest.view_hidden` 者返回 40400；`super_admin` 总是放行。
- 密码与邀请码只存 bcrypt 哈希；响应 DTO 不包含哈希；日志只记录请求路径与状态。
- 比赛状态由服务器时间计算为 `upcoming`/`running`/`ended`；`is_rated` 在开赛后冻结（再修改返回 40001），Rating 结算在 M4-A 实现（见 5.5）。
- 比赛题目最多 26 题，`displayId` 为 A–Z 且同一比赛唯一；更新时整体替换题目列表。
- 克隆复制比赛配置、密码/邀请码哈希与题目，不复制报名与公告；可覆盖标题/起止时间。
- 比赛提交、赛时保护、三种榜单与前端页面在 M2-B/M2-C。

**实现状态（M2-B）**

- 比赛提交复用 M1 提交/评测管线：`POST /submissions` 带 `contestId`，校验比赛存在、`[start,end)`、报名状态、题目归属；拥有 `contest.update` 者可跳过报名用于测试。
- 赛时保护覆盖列表与详情：进行中且非本人/非管理员时，只保留状态与语言，代码、错误信息、测试点详情、耗时、内存、得分全部隐藏；本人/管理员/比赛结束后完整可见。
- `GET /contests/:id/ranklist`：
  - ICPC：通过题数降序、总罚时升序（首次 AC 分钟 + 首次 AC 前未通过次数 × 20），并列同名次，包含 0 提交报名者。
  - IOI：每题取最高分，总分降序。
  - OI：计分同 IOI；赛时非管理员只返回本人行（`selfOnly=true`），管理员和赛后返回完整榜单。
- 比赛开始后不能修改开始时间、赛制或整体替换题目列表；已有比赛提交时删除比赛返回 40001；榜单只统计比赛时间窗内且报名之后的提交。
- 实时刷新复用 M1 的 submission SSE：前端收到事件后重新拉取当前榜单。
- Rating 结算已在 M4-A 完成（见下方与 5.5）。

**实现状态（M4-A）**

- 每场 `is_rated=true` 且 `end_time` 已过的比赛由后台扫描器自动结算；结算使用 PostgreSQL 全局 advisory lock 串行、评测宽限与 `rating_history` 唯一约束保证幂等。
- 比赛开始后不允许修改 `isRated`；`rating_settled_at` 只由结算事务写入，管理员整体更新不会覆盖已写入的结算标记；克隆比赛会清空该标记。
- ICPC 榜单补齐内部字段 `LastSubmissionAt`（不返回 API），记录报名用户在比赛窗口内最后一次已判（非 `pending`/`judging`）提交时间，用于判断是否参与 Rating；0 次已判提交者不参与。

**实现状态（M2-C）**

- 前端完成比赛列表（公开/我报名的、关键词/赛制/状态筛选、分页、管理员新建/编辑/克隆/删除）、比赛详情（服务端时间校准倒计时、报名、题目入口、公告、榜单）、比赛编辑页（题目整体替换、密码/邀请码留空保留、Markdown 描述）。
- 比赛提交页接受 `contestId` 查询参数，并在页面显示“比赛提交”、返回比赛链接；草稿按“练习/比赛”作用域隔离。
- 榜单通过 SSE 自动刷新；OI 赛时显示“仅本人成绩”提示。
- 榜单选手列展示装备称号与徽章（`IdentityShowcase`，小尺寸），空装备不渲染占位；窄屏下选手信息纵向排列，表格可横向滚动。
- 比赛列表 `mine=true` 仅返回当前登录用户已报名的比赛（含非公开比赛），`registered` 字段用于显示已报名状态；游客使用 `mine=true` 返回 40001。

**实现状态（M2-B）— 测评记录**

- 比赛进行中，非本人/非管理员查看他人比赛提交时，列表与详情均隐藏代码、错误信息、测试点详情、耗时、内存、得分；比赛结束后恢复。
- 用户报名前（例如比赛管理员测试）产生的比赛提交不计入正式榜单。

**Codex 提示词（比赛）**

```text
你是一名 OJ 系统专家，请为 kapOJ 实现「比赛」模块。

技术栈：Go + Gin + GORM + PostgreSQL + Redis 后端，Vue3 + ECharts 前端。

请完成：
1. 数据模型：Contest、ContestProblem、ContestParticipant、Announcement（字段见开发文档第 3.4 节）。
2. 后端接口：
   - 比赛 CRUD：GET/POST/PUT/DELETE /contests（比赛管理员）
   - POST /contests/:id/clone：克隆比赛（复制题目与配置）
   - POST /contests/:id/register：报名（含密码校验/邀请码）
   - GET /contests/:id/ranklist：榜单（支持 ICPC/IOI/OI 三种赛制实时计算）
   - 公告 CRUD：GET/POST/PUT/DELETE /contests/:id/announcements
3. 榜单计算：
   - ICPC：按 AC 题数降序、总罚时升序；单题罚时 = 首次 AC 用时 + 首次 AC 之前的未通过次数 × 20min
   - IOI：每题取多次提交最高分，按总分降序
   - OI：赛时仅本人可见成绩，赛后统一公布
4. 前端：比赛列表、比赛详情（倒计时、题目列表、提交、公告）、实时榜单页（ECharts 排名变化）。
5. 验收标准：可创建/克隆比赛，三种赛制榜单计算正确，公告可发布，比赛倒计时与访问控制生效。

请输出完整代码、榜单算法实现与类型定义。
```

---

### 4.5 学习路线模块

**功能需求**

- 学习路线 CRUD：官方路线由题目管理员/超级管理员维护；任意登录用户可创建默认私有的自定义路线，作者可公开，管理员可下架/删除违规公开路线（无审核队列）。
- 路线内包含按算法组织的题目，呈树状结构（算法模块 → 阶段 → 题目）。
- 用户按阶段挑战，完成部分算法后可解锁下一个算法模块。
- 点亮分支结构，点进每个算法模块可查看题目清单。
- 数据完整性：被活跃路线引用的题目不能下线/删除；已完成节点锁存，不因改题回退。

**关键设计**

- 节点模型：`learning_path_nodes` 支持 `parent_id` 自关联，形成树；父节点必须属于同一路线，不允许自环/成环，最大深度 8 层。
- 可见性：官方与公开自定义路线对所有人可见；私有自定义路线仅作者与拥有 `learning_path.admin.read` 或 `learning_path.official.manage` 者可见；`is_active=false`（软删除）路线需要 `learning_path.admin.read` 或 `learning_path.official.manage` 才可见/恢复。无权限与不存在统一返回 40400。
- 权限（RBAC-C）：创建路线需登录；编辑/删除路线与节点仅作者或拥有 `learning_path.official.manage` 者；官方路线仅拥有 `learning_path.official.manage` 者可写；`learning_path.admin.read` 只读查看全部/下架/完整题目 ID，不参与写授权。`is_official` 仅创建时由拥有 `learning_path.official.manage` 者设置、创建后不可修改（拥有该权限者也不能提升自定义路线为官方），`is_active` 仅拥有 `learning_path.official.manage` 者可设置，`is_official=true` 强制公开。
- 节点内容：题目 1–50 道、不重复且必须存在/公开；`required_count` 为 1–题目数，默认题目数；仅叶子节点可删除，有子节点返回 40001。
- 解锁规则：节点只有在父节点完成后才可挑战；节点完成 = 通过其 `required_count` 道题（默认为节点内全部题目）。根节点默认解锁。进度由服务端根据 AC 计算，不接受客户端上报。
- 前端用 PrimeVue 4 `TreeTable` 或自定义树形组件可视化。结构是树而非 DAG，每个节点只有一个父节点。

**实现状态（M4-2B）**

- 已完成 `learning_paths`、`learning_path_nodes`、`user_learning_progress` 模型/迁移与 OpenAPI/前端类型。
- 已交付官方/自定义权限、私有/公开/下架可见性、路线 CRUD、软删除/恢复校验、节点 CRUD、父节点/环/深度/题目/`required_count` 校验与叶子删除限制。
- 服务端进度（`learning.Service.EvaluateUser`）：按用户在节点题目集合中去重 AC 的题数计算 `solved_count`，`solved_count >= required_count` 即完成；根节点默认 `active`，子节点仅在父节点 `completed` 后 `active`、否则 `locked`；已完成节点锁存（改题/提高 `required_count` 不回退已完成用户，`solved_count` 保留完成时的值，未完成节点按新配置重算）；游客（`userID<=0`）为 no-op，不写进度；只处理用户可见的活跃路线。`GET /learning-paths` 与 `GET /learning-paths/:id/tree` 对登录用户读取前先 `EvaluateUser`，`solvedCount/status/myCompletedCount` 均为服务端值；游客树根 `active`、其余 `locked`、`solvedCount=0`、无进度行。公开树对非作者且无学习路线管理/只读权限者只返回公开题目的 `problemIds`，作者、拥有 `learning_path.admin.read` 或 `learning_path.official.manage` 者保留完整存储列表供编辑。
- AC 事件（`learning.Service.HandleGameEvent`）：只处理 `submission.accepted`，重新 `EvaluateUser` 该用户；`gamequeue.NewChain` 在 `main.go` 中把 gamification 与 learning 组合为同一个 Asynq handler，依次调用、任一失败返回错误触发重试，二者幂等。
- 题目下线守卫（`problem.LearningGuard`/`WithLearningGuard`，router 注入 `learning.Service`）：`IsProblemInActivePath` 判断题目是否被任一活跃路线节点引用；`Update` 将 `is_public` 由 true 改为 false 与 `Delete` 物理删除前检查，命中返回 40001「题目被学习路线引用，不能下线/删除」；移除引用节点后允许下线/删除。

**实现状态（M4-2C，前端已完成）**

- 学习路线前端四页已交付，复用 `AppPageHeader`/`AppSurface`/`AppFormField`/`MarkdownView`/`MarkdownEditor` 与 PrimeVue 4 组件，不新增 UI 依赖；权限全部以 `auth.can`/后端返回为准，前端不自行推导。
- 路由与页面：
  - `/learning-paths`（`LearningPathListView`，公开）：官方/公开自定义路线卡片，筛选「全部/官方/自定义/我创建的」（游客隐藏“我创建的”），登录用户显示“新建路线”；卡片展示标题、作者、`已完成 myCompletedCount / nodeCount 节点`、官方/公开/私有标签；作者或 `learning_path.official.manage` 显示“编辑”入口。
  - `/learning-paths/:id`（`LearningPathDetailView`，公开）：`TreeTable` 树节点（`data-key="id"`、默认全展开）+ 右侧详情面板；节点列展示标题 + 状态 Tag（`completed` 绿色/`active` 主色/`locked` 灰色锁）、`solvedCount/requiredCount` 进度条、题目数；点击节点展示描述（Markdown）与题目链接；游客根 `active`、其余 `locked`；私有/下架路线返回 40400 时显示“学习路线不存在或无权查看”。
  - `/learning-paths/new`、`/learning-paths/:id/edit`（`LearningPathEditView`，`requiresAuth`）：路线表单（标题/描述/封面/排序/公开，官方开关仅创建时且 `learning_path.official.manage` 可见并强制公开）+ 节点 `TreeTable`（添加根/子节点、编辑、上移/下移、删除叶子；有子节点删除禁用）；`LearningNodeDialog` 校验标题 1–100、描述 ≤50,000、题目 1–50 去重、`requiredCount` 1–题目数、父节点同路线不可成环。
  - `/admin/learning-paths`（`AdminLearningPathsView`，`learning_path.official.manage` 可写 / `learning_path.admin.read` 只读）：`DataTable` 管理全部路线（含 `includeInactive`），列标题/类型/可见性/状态/作者/节点数/我的完成/更新时间/操作；`official.manage` 可新建官方、编辑、下架（软删除）/恢复；`admin.read` 只读不显示写按钮。
- 状态语义：`learningStatusLabel`（`已完成`/`进行中`/`未解锁`）、`learningStatusSeverity`（`success`/`info`/`secondary`）、`nodeProgressPercent`（required≤0 时 0）、`learningPathVisibilityLabel`（`官方`/`公开`/`私有`）集中在 `utils/learning.ts`，列表/树/编辑/管理页复用。
- 导航：主侧栏“学习路线”（`pi pi-sitemap`）与 `AdminNav` 内容组“学习路线”（同权限）已接入。
- 移动端：列表卡片 `<=640px` 单列；树 `TreeTable`/`DataTable` 在容器内横向滚动，对话框宽度 `min(94vw, 640px)`，375px 下可读可操作；无 CDN 请求。
- 验证：前端测试/lint/build 通过；真实浏览器 E2E 覆盖官方/自定义路线、进度重算、完成锁存、私有/公开可见性、下架/恢复、节点编辑与 375px 布局，见 `apps/web/e2e/learning-paths.spec.ts`。

**Codex 提示词（学习路线）**

```text
你是一名全栈工程师，请为 kapOJ 实现「学习路线」模块。

技术栈：Go + Gin + GORM + PostgreSQL 后端，Vue3 + PrimeVue 4（TreeTable/自定义树形可视化）前端。

请完成：
1. 数据模型：LearningPath（含 is_official/is_public/is_active/created_by）、LearningPathNode（含 parent_id 自关联）、UserLearningProgress。
2. 后端接口：
   - 学习路线 CRUD：GET/POST/PUT/DELETE /learning-paths（创建登录；编辑/删除作者或管理员；官方路线仅管理员）
   - 节点 CRUD：POST/PUT/DELETE /learning-paths/:id/nodes...
   - GET /learning-paths/:id/tree：返回完整树结构（节点 + 关联题目 + 用户完成状态）
   - 进度由服务端根据 AC 事件自动计算，不接受客户端上报（防止作弊）；前端只读取树结构与进度
3. 解锁逻辑：节点仅在父节点完成后可挑战；节点完成条件为通过 required_count 道题（默认为该节点全部题目）。
4. 前端：学习路线列表、树状路线详情页（分支可点亮、已完成/进行中/未解锁状态、点击节点查看题目清单）。
5. 验收标准：可创建树状路线，节点按父级依赖解锁，进度正确持久化，前端树状结构正确展示。

请输出完整代码与类型定义。
```

---

### 4.6 测评记录模块

**功能需求**

- 查看全平台所有用户的测评记录，动态刷新。
- 支持按题目 ID、比赛 ID、语言、提交状态、用户 ID、用户昵称筛选。
- 比赛进行中，其他选手的代码和测评详情不可见（与 4.4 赛时保护一致）。
- 单条记录展示：提交者、题目、状态、耗时、内存、语言、时间。

**实现状态（M2-B）**：比赛进行中，其他用户的比赛提交在列表与详情中隐藏代码、错误信息、测试点详情、耗时、内存与得分，只保留状态和语言；本人、管理员和比赛结束后完整可见。OI 赛时非管理员榜单只返回本人行。

**公开身份展示（M3 已完成）**：`GET /submissions`、`GET /submissions/:id` 与 `GET /contests/:id/ranklist` 的每一行返回提交者/选手的 `equippedBadges`（数组，最多 `KAPOJ_MAX_EQUIPPED_BADGES` 个）与 `equippedTitle`（无装备时为 null），按页批量查询避免 N+1，已装备但后来下线的徽章/称号仍展示。测评记录页在提交者昵称下方、榜单页在选手昵称下方用 `IdentityShowcase` 渲染，详情抽屉同样展示；空装备时不显示占位。

**Codex 提示词（测评记录）**

```text
你是一名全栈工程师，请为 kapOJ 实现「测评记录」模块。

技术栈：Go + Gin + GORM + PostgreSQL + Redis（用于发布订阅/SSE）后端，Vue3 + Element Plus 前端。

请完成：
1. 后端接口 GET /submissions：支持分页与多条件筛选（problemId、contestId、language、status、userId、nickname）。比赛进行中，非本人的代码和测评详情不返回（见 4.4 赛时保护）。
2. 动态刷新：使用 SSE（Server-Sent Events）或 WebSocket，在评测状态变化时向前端推送最新提交。
3. 前端：测评记录列表页（筛选栏 + 表格 + 实时刷新 + 状态彩色标签）。
4. 验收标准：可按所有维度筛选，评测完成后列表自动更新，状态显示正确。

请输出完整代码与类型定义。
```

**实现状态（M1-C）**

- 接口：`GET /submissions`（公开，分页 + problemId/contestId/language/status/userId/nickname 筛选）、`GET /submissions/:id`（公开，返回代码与逐测试点结果）、`GET /submissions/stream`（SSE，事件只含 `{"submissionId": id}`）。
- 排序为提交 id 倒序；`contestId=0` 表示练习提交，`contestId>0` 表示指定比赛。比赛提交在 M2 赛时保护完成前，非本人/非管理员只返回 `codeHidden=true`，不返回代码、错误信息与测试点详情。
- 隐藏题目：公开列表只显示 `problems.is_public=true` 的提交（登录用户额外可看到自己的提交）；他人隐藏题的提交列表/详情需要 `problem.view_hidden`，其他用户返回 40400；`contest_admin` 不因比赛管理权限看到隐藏题。比赛进行中他人提交的代码/成绩/测试点需要 `submission.view_live_contest` 才可查看（本人、赛后恢复）。
- 昵称为不区分大小写的模糊匹配，按字面量转义 `%`/`_`；非法 status/language/负 id 返回 40001。
- Web 在创建提交（pending）与入队失败（system_error）、judge 在领取任务（judging）与写入终态后，都会向 Redis 频道 `kapoj:submissions` 发布 submission id；Web 端 SSE 订阅该频道，连接建立时立即发送注释行、每 15 秒 keep-alive，浏览器断线由 EventSource 自动重连。前端在 `onopen` 补拉一次并每 30 秒兜底刷新，避免 Redis/网络短暂中断造成列表长期停留在旧状态。
- 前端：`/submissions` 记录页（筛选、分页、状态彩色标签、详情抽屉、SSE 自动刷新）；`/problems/:id/submit` 提交页（Monaco、语言切换、代码改动 500ms 防抖写入本地草稿，存储异常不影响提交）；题目详情页在本人 AC 后显示「解题思路」「解题代码」。
- Monaco 使用本地打包的 editor worker，不访问 CDN；提交页路由懒加载编辑器，避免影响首屏体积。

---

### 4.7 平台管理后台与 RBAC

**功能需求**

- 把固定 `users.role` 枚举升级为可配置的「多角色 + 全局权限」体系：权限码按资源分组，账号可拥有多个角色，权限取并集。
- 权限变更实时生效：后端每次管理请求都按用户 ID 查询用户状态、角色与权限，不把权限写入 JWT，也不使用权限缓存。
- 统一 `/admin` 管理后台：仪表盘、用户管理、角色权限、审计日志；现有题目/比赛/游戏化页面先迁移权限判断并沿用入口。
- 兼容迁移现有 `super_admin` / `problem_admin` / `contest_admin` 账号；`users.role` 与 JWT role 保留为 legacy/展示字段，自 RBAC-C 起不再参与任何管理授权，只用于展示。

**权限目录（37 个权限码，稳定不可改名）**

| 分组 | 权限码 |
|------|--------|
| 管理后台 | `admin.dashboard.view`、`admin.audit.view`、`system.settings.manage` |
| 用户与角色 | `user.read`、`user.update`、`user.ban`、`user.coins.adjust`、`user.roles.assign`、`role.read`、`role.create`、`role.update`、`role.delete`、`role.permissions.update` |
| 题库 | `problem.view_hidden`、`problem.create`、`problem.update`、`problem.delete`、`problem.publish`、`problem.testcase.manage`、`problem.checker.manage` |
| 测评记录 | `submission.view_live_contest` |
| 比赛 | `contest.view_hidden`、`contest.create`、`contest.update`、`contest.delete`、`contest.participant.read`、`contest.announcement.manage` |
| 游戏化 | `gamification.achievement.manage`、`gamification.task.manage`、`gamification.badge.manage`、`gamification.title.manage`、`gamification.config.manage` |
| 学习路线 | `learning_path.official.manage`、`learning_path.admin.read` |
| Rating | `rating.settings.manage` |
| AI | `ai.settings.manage` |
| 证书 | `certificate.manage` |

> `rating.settings.manage`（第 35 个权限码，M4-4 新增，分组「Rating」）用于管理 Rating 衰减与赛季重置设置；默认只授予 `super_admin`（super 绕过），不自动授予 `content_admin`。修改设置写审计 action=`rating.settings.manage`、target=`rating_settings`。
>
> `ai.settings.manage`（第 36 个权限码，AI 管理开关新增，分组「AI」）用于启用/关闭平台 AI 功能；默认只授予 `super_admin`（super 绕过），不自动授予 `content_admin`。修改设置写审计 action=`ai.settings.manage`、target=`ai_settings`。开关语义见 §6.6。
>
> `certificate.manage`（第 37 个权限码，M5-7 证书认证新增，分组「证书」）用于审核证书认证与维护证书类型；默认只授予 `super_admin`（super 绕过），不自动授予 `content_admin`。审核写审计 action=`certificate.manage`、target=`certificate`，类型 CRUD 写审计 target=`certificate_type`。语义见 §5.6。

**内置角色（7 个，均由首次 Seed 创建，`is_system=true`）**

| code | 名称 | 权限 |
|------|------|------|
| `super_admin` | 超级管理员 | 绕过所有权限检查；不写 `role_permissions` 行，不可删除/不可改权限 |
| `content_admin` | 内容管理员 | `admin.dashboard.view` + 全部 `problem.*` + `learning_path.official.manage`、`learning_path.admin.read` |
| `contest_admin` | 比赛管理员 | `admin.dashboard.view` + 全部 `contest.*` + `submission.view_live_contest` |
| `gamification_admin` | 游戏化管理员 | `admin.dashboard.view` + 全部 `gamification.*` |
| `user_admin` | 用户管理员 | `admin.dashboard.view` + `user.read/update/ban/coins.adjust` + `role.read` |
| `security_admin` | 安全管理员 | `admin.dashboard.view`、`admin.audit.view`、`user.read`、全部 `role.*`、`user.roles.assign` |
| `auditor` | 审计员 | `admin.dashboard.view`、`admin.audit.view`、`user.read`、`role.read` |

> 系统角色保护（不可删除、code/name 不可改、`super_admin` 不可改权限）由 RBAC-B 实现；RBAC-A 只负责创建内置角色。

**旧角色映射（仅首次 Seed）**

| `users.role` | RBAC 角色 |
|--------------|-----------|
| `super_admin` | `super_admin` |
| `problem_admin` | `content_admin` |
| `contest_admin` | `contest_admin` |
| `user` | 不分配管理角色 |

**实现状态（RBAC-A、RBAC-B、RBAC-C 与 RBAC-D 已完成，2026-11-06/07）**

- 已建表：`roles`、`permissions`、`role_permissions`、`user_roles`、`admin_audit_logs`、`rbac_meta`；唯一索引见 3.1。`admin_audit_logs` 的审计写入由 RBAC-B（敏感操作，事务内）与 RBAC-C（内容管理，best-effort）完成。
- 权限目录与内置角色定义在 `apps/server/internal/rbac/catalog.go`；测试锁定权限数量/唯一性与内置角色权限集合，名称/描述/分组仅校验非空，不锁定具体中文文案。
- 启动时在 `db.Migrate` 之后执行 `rbac.Seed`：
  - 仅当 `rbac_meta.seed_version` 不存在时执行完整 seed：插入权限目录、内置角色、内置角色权限（`super_admin` 除外）、按 `users.role` 映射旧账号，最后写入 `seed_version=1`。
  - 后续启动只按 code 自动补齐缺失的权限目录与内置角色（已有行不变），**不会重新添加管理员已删除的 `role_permissions`，也不会重新映射旧角色**；`seed_version` 小于当前版本时同样只补齐目录/角色并更新版本，不会新增或恢复任何角色权限授权。给内置角色新增初始权限必须走显式数据迁移，不能依赖 seed。
  - 映射使用 `ON CONFLICT (user_id, role_id) DO NOTHING`，重复启动不会产生重复关联。
- `rbac.Service.Access(ctx, userID)` 实时读取数据库：用户不存在/非 active 返回 `ErrNotFound`/`ErrInactive`；角色按 code 排序；多角色权限取并集；`super_admin` 设置 `IsSuper=true`。没有缓存。
- `RequirePermission` / `RequireAnyPermission` 语义：未登录 401；解析失败、封禁或无权限 403；super 绕过；通过后 `*rbac.Access` 写入 context，可用 `CurrentAccess` 读取。
- `/users/me` 已返回 `roles`/`permissions`/`isSuperAdmin`；OpenAPI 与前端类型同步更新。登录/注册响应中三个字段默认为空数组/false。

**RBAC-B 管理接口（全部使用 `RequirePermission`，super 绕过但仍写审计）**

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/api/admin/stats` | `admin.dashboard.view` | 用户/题目/比赛/提交/角色计数；super 或 `admin.audit.view` 附带最近 10 条审计 |
| GET | `/api/admin/audit` | `admin.audit.view` | 审计日志分页/筛选（actorId/action/targetType/from/to） |
| GET | `/api/admin/permissions` | `role.read` | 权限目录（按分组与排序） |
| GET | `/api/admin/roles` | `role.read` | 角色列表（含权限、用户数） |
| POST | `/api/admin/roles` | `role.create` | 创建自定义角色 |
| PUT | `/api/admin/roles/:id` | `role.update` | 改名称/描述；系统角色 code/name 不可改 |
| DELETE | `/api/admin/roles/:id` | `role.delete` | 删除自定义角色；系统角色 40001 |
| PUT | `/api/admin/roles/:id/permissions` | `role.permissions.update` | 整体替换权限；`super_admin` 不可改 |
| GET | `/api/admin/users` | `user.read` | 用户列表（keyword/status/roleId 筛选与分页） |
| GET | `/api/admin/users/:id` | `user.read` | 用户详情（含角色、isSuperAdmin） |
| PUT | `/api/admin/users/:id` | `user.update` | 编辑昵称/简介/头像链接 |
| PUT | `/api/admin/users/:id/roles` | `user.roles.assign` | 整体替换用户角色 |
| POST | `/api/admin/users/:id/ban` | `user.ban` | 封禁 |
| POST | `/api/admin/users/:id/unban` | `user.ban` | 解封 |
| GET | `/api/admin/rating/settings` | `rating.settings.manage` | Rating 衰减/赛季设置 + 当前赛季与下次重置（M4-4） |
| PUT | `/api/admin/rating/settings` | `rating.settings.manage` | 整体更新 Rating 设置；校验范围；写审计（M4-4） |
| GET | `/api/admin/ai/settings` | `ai.settings.manage` | AI 开关与密钥配置状态 `{enabled,keyConfigured,updatedAt}`（AI 管理开关） |
| PUT | `/api/admin/ai/settings` | `ai.settings.manage` | 启用/关闭 AI；无密钥时启用返回 40001；与审计同事务（AI 管理开关） |
| GET | `/api/admin/ai/key` | `ai.settings.manage` | AI 加密密钥状态 `{configured,source,masked,fingerprint}`（不含完整密钥） |
| POST | `/api/admin/ai/key/generate` | `ai.settings.manage` | 生成并轮换密钥；完整密钥仅一次性返回 |
| PUT | `/api/admin/ai/key` | `ai.settings.manage` | 手动填写密钥；已有密钥时轮换重加密 |

- **保护规则**：系统角色不可删除、code/name 不可改；`super_admin` 权限不可修改；只有 super 能授予/移除/封禁/解封 super；禁止封禁自己（40001）；禁止移除/封禁最后一个 active super（40001）。角色/权限变更、用户角色分配、封禁/解封、管理员编辑用户在同一事务内写审计，失败回滚。
- 自定义角色 code 创建后不可修改（`^[a-z][a-z0-9_]{2,63}$`），权限用 code 整体替换、重复去重、非法 code 返回 40001。
- `POST /api/admin/users/:id/coins`（`user.coins.adjust`）在 RBAC-C 迁移到 `adminapi`，调整与审计在同一事务内完成，审计失败回滚。

**RBAC-C 内容管理接口迁移（所有管理写接口与隐藏读取已迁移到权限码）**

- 题目：`problem.view_hidden`（隐藏题列表/详情/统计与隐藏题提交）、`problem.create`/`problem.update`/`problem.delete`、`problem.publish`（修改 `isPublic` 需 `problem.update` + `problem.publish`）、`problem.testcase.manage`、`problem.checker.manage`。`problem.Viewer` 拆 `CanViewHidden`/`CanViewSolution`，`problem.view_hidden` 只放开题面不放开题解。
- 比赛：`contest.view_hidden`（非公开/开赛前/榜单读取）、`contest.create`（含克隆）、`contest.update`、`contest.delete`、`contest.participant.read`、`contest.announcement.manage`。`contest.Viewer` 拆六个能力字段，`canManage` 由任一管理能力派生。
- 测评：`problem.view_hidden`（隐藏题提交）、`submission.view_live_contest`（赛时保护绕过）、`contest.update`（免报名比赛测试提交）。读取路由仍公开，`submission.Viewer` 拆 `CanViewHiddenProblems`/`CanViewLiveContest`/`CanManageContest`。
- 游戏化：成就/任务/徽章/称号各用 `gamification.*.manage`（含 `includeInactive`）；`gamification.config` 仍公开只读（`gamification.config.manage` 暂无写接口）。金币调整 → `user.coins.adjust`（事务内审计）。
- 学习路线：`learning_path.official.manage`（写/管理官方与任意路线、上下架/恢复/节点，保留内容管理员既有行为）、`learning_path.admin.read`（只读查看全部/下架/完整题目 ID，不参与写授权）；普通作者自定义路线规则保留。`learning.Viewer` 拆 `CanManageOfficial`/`CanViewAll`。
- 内容管理审计：题目/比赛/游戏化/学习路线写操作经 `AuditAdminAction` best-effort 写审计（仅 2xx 且 actor 拥有对应权限码时），action 为权限码，target type 为 `problem`/`contest`/`gamification_achievement`/`gamification_task`/`gamification_badge`/`gamification_title`/`learning_path`，仅记录 method/path/status/target，失败只记日志不阻断业务。
- **legacy role 不再授权**：RBAC-C 完成后，任何管理授权不再调用 `RequireRole` 或读取 `claims.Role`；`users.role`/JWT role 仅保留展示/兼容。
**RBAC-D 管理后台前端（2026-11-07）**

- 新增统一 `/admin` 管理壳 `AdminLayout.vue` 与 `AdminNav.vue`：独立于 `DefaultLayout`，桌面侧栏 + 移动 Drawer，菜单按权限过滤；`/admin` 父路由 `requiresAuth` + `anyPermission: ADMIN_ACCESS_PERMISSIONS`，子路由 `''`（概览）、`users`（`user.read`）、`roles`（`role.read`）、`audit`（`admin.audit.view`）、`gamification`（任一 `gamification.*.manage`）。
- 新增仪表盘 `AdminDashboardView`（`admin.dashboard.view` 才请求 `/admin/stats`，`admin.audit.view` 才显示最近审计）、用户管理 `AdminUsersView`（编辑/角色/封禁/金币，动作按权限显隐）、角色权限矩阵 `AdminRolesView`（系统角色保护、`super_admin` 权限不可改）、审计日志 `AdminAuditView`（actor/action/targetType/日期筛选与分页）。
- 权限基础：`apps/web/src/permissions.ts` 定义全部前端权限码常量与 `ADMIN_ACCESS_PERMISSIONS`；`auth.can/canAny/isSuperAdmin`；路由 meta 改为 `permission`/`anyPermission`（移除 `roles`）；守卫未登录跳登录、无权限跳 `/forbidden`。
- 现有入口迁移：`AppNav`/`UserMenu` 的“管理后台”入口改为 `/admin`，显示条件 `auth.canAny(ADMIN_ACCESS_PERMISSIONS)`；题库/比赛/游戏化页面按钮与 tab 全部改用 `auth.can/canAny`；游戏化管理 `GamificationAdminView` 按四个 `gamification.*.manage` 权限生成 tab。
- 权限即时性：登录/注册/资料保存后刷新 `/users/me` 并保留 `roles/permissions/isSuperAdmin`；角色/权限变更后刷新页面或重新拉取 `/users/me` 即生效，后端仍实时校验。

**交付拆分**

| 阶段 | 内容 | 状态 |
|------|------|------|
| RBAC-A | 权限模型、目录、首次 Seed/旧角色映射、Access 解析、权限中间件、Me 权限字段（后端） | ✅ |
| RBAC-B | 角色 CRUD/权限矩阵、用户角色分配、封禁/解封、审计、仪表盘统计（后端） | ✅ |
| RBAC-C | 现有管理接口权限码迁移、Viewer 能力拆分、内容管理审计、回归测试 | ✅ |
| RBAC-D | AdminLayout、仪表盘/用户/角色/审计页面、`auth.can`、菜单/路由/按钮权限化、文档、HTTP 冒烟与浏览器 E2E | ✅ |

**Codex 提示词（RBAC 后端核心）**

```text
你是一名后端工程师，请为 kapOJ 实现细粒度 RBAC 的权限模型与中间件。

技术栈：Go + Gin + GORM + PostgreSQL。

请完成：
1. 数据模型：roles、permissions、role_permissions、user_roles、admin_audit_logs、rbac_meta（字段见 DEVELOPMENT.md 3.1）。
2. 权限目录按资源分组，权限码格式 domain.action；内置 super_admin/content_admin/contest_admin/gamification_admin/user_admin/security_admin/auditor 七个角色。
3. 启动时幂等 Seed：首次写入权限、角色、角色权限并按 users.role 映射旧账号；写 rbac_meta.seed_version；后续启动不恢复管理员删除的角色权限。
4. rbac.Service.Access 实时解析用户 active 状态、多角色权限并集与 super 标记；RequirePermission/RequireAnyPermission 未登录 401、无权限/封禁 403、super 绕过。
5. GET /users/me 返回 roles、permissions、isSuperAdmin；users.role 仅作 legacy 兼容展示（RBAC-C 起不再参与管理授权）。
6. 验收：多角色并集、封禁拒绝、权限变更后旧 token 立即生效、Seed 幂等且不恢复已删除权限。
```

---

### 4.8 社交互动模块

**实现状态（M4-3 ✅，2026-11-08）**

在解题思路论坛之上交付题解帖子点赞、用户关注、站内通知，并把论坛回复（评论）、点赞、关注接入活跃度与共享分。数据模型见 3.6（`problem_solution_post_likes`、`user_follows`、`notifications`），设计见 [`docs/superpowers/specs/2026-11-08-m4-3-social-design.md`](docs/superpowers/specs/2026-11-08-m4-3-social-design.md)。

**关键接口**

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| POST/DELETE | /problems/:id/solutions/:postId/like | 登录 | 点赞/取消赞；返回 `{liked, likeCount}`；重复幂等；自赞 40001；隐藏题/不存在帖子 404 |
| POST/DELETE | /users/:id/follow | 登录 | 关注/取消关注；返回 `{following, followersCount}`；重复幂等；自己/不存在/非 active → 40001/404/40001 |
| GET | /users/:id/social | 可选 | `{followersCount, followingCount, isFollowing}`；游客 `isFollowing=false` |
| GET | /users/:id/followers?page=&page_size= | 可选 | 粉丝分页（公开用户摘要，按关注时间倒序、id 倒序） |
| GET | /users/:id/following?page=&page_size= | 可选 | 关注分页（同上） |
| GET | /notifications?page=&page_size= | 登录 | 自己的通知，`created_at DESC, id DESC` |
| GET | /notifications/unread-count | 登录 | `{count}` |
| POST | /notifications/:id/read | 登录 | 标记单条已读；非本人 404 |
| POST | /notifications/read-all | 登录 | 全部标记已读（幂等） |

**语义与规则**

- **点赞**：先复用讨论服务校验题目/帖子可见性（隐藏题/不存在 404），再点赞；自己赞自己的帖子返回 40001「不能点赞自己的帖子」且不产生点赞/通知/活跃度/共享分。事务内 `INSERT problem_solution_post_likes ... ON CONFLICT DO NOTHING` 判定是否新插入：插入成功则 `shared_items(type=solution)` upsert +1、给帖子作者写 `like:<actor>:<post>` 通知（作者 ≠ 点赞者）；提交后 best-effort 记点赞活跃度 +1 与共享榜版本递增。取消赞删除 like 并 `GREATEST(0, like_count-1)`，不写活跃度/通知。帖子列表/详情返回 `likeCount`/`likedByMe`（游客 false）。帖子软删除后清理该帖点赞与共享项（活动日志/通知保留）。
- **关注**：只能关注 active 用户、不能关注自己；首次关注事务内写 `follow:<actor>:<followee>` 通知 + best-effort 记活跃度 +2；取消关注删除关系，不扣活跃度/不删通知。公开 `social`/`followers`/`following` GET 对已存在但封禁/非 active 的用户仍返回计数/列表（与 `/users/:id/profile` 一致），只有写关注要求 active。用户摘要仅 `id/username/nickname/avatarUrl/rating`，不含 email。
- **通知**：类型 `solution_reply`（回复我的帖子）、`solution_like`（赞我的帖子）、`follow`（关注我）；`dedupe_key` 全局唯一分别为 `reply:<replyId>`、`like:<actor>:<post>`、`follow:<actor>:<followee>`，保证重复赞/关注/同一回复不重复通知。自己触发自己的动作不生成通知（自回复不通知，但回复活跃度仍计分）。回复通知在论坛回复创建成功后 best-effort 写入；点赞/关注通知在事务内写入。`link` 为站内相对路径（如 `/problems/1/solutions/9`、`/users/3`），通知标题由前端按 type 生成。通知只读自己；标记已读幂等；帖子软删除后通知保留。
- **活跃度**：回复 +3（`ref_type=solution_reply`/`ref_id=replyID`）、点赞 +1（`solution_post`/`postID`）、关注 +2（`user`/`followeeID`），每类每日上限 20；`activity_logs` 唯一约束 `(user_id, action, ref_type, ref_id)` 去重，取消赞/取消关注不扣分；写活跃度 best-effort，失败只日志不阻断主操作，成功写活跃度后递增活跃度榜缓存版本。
- **共享分**：题解帖子每获得 1 个赞，作者共享分 +5（`shared_items(type=solution)` upsert `like_count`）；取消赞扣回；帖子删除移除共享项并递增共享榜缓存版本。共享分公式 `20 * 比赛共享数 + 5 * 题解点赞数` 的第二项自此生效。
- **前端**：题解列表/详情点赞按钮（已赞高亮、乐观更新失败回滚 + Toast、游客跳登录）；用户主页关注按钮与粉丝/关注 Dialog（两个 Tab、分页、非本人且登录才显示关注）；`NotificationBell` 仅登录显示未读数 badge、60 秒轮询、卸载清理、点击进 `/notifications`；`/notifications` 页分页列表、点击标记已读并跳转 link、全部已读、空/错/载入态；路由 `notifications` 需登录，游客跳登录。不新增 UI 依赖（PrimeVue UI-32）。

---

## 5. 核心趣味功能模块

### 5.1 成就模块

**功能需求**

- 管理员置顶/配置一系列成就，用户完成条件后自动解锁并获得奖励。
- 成就记录奖励（金币、徽章等），在个人中心展示。
- 成就条件示例：首次 AC、AC 100 题、连续登录 30 天、参加 5 场比赛、创建 1 场比赛、获得 100 个赞等。

**触发机制**

- 事件驱动：后端在关键动作处发布事件（如提交 AC、登录、参赛、点赞），成就服务订阅事件并判断是否满足条件。事件可能被重复投递，判定必须幂等。
- 条件可配置：`condition_type` + `condition_config_json` 支持计数类、阈值类、组合类。
- 解锁记录与奖励发放在同一数据库事务中完成，依靠 `user_achievements(user_id, achievement_id)` 唯一约束防止重复发放。

**实现状态（M3-A 后端、M3-C 前端已完成；M3-B 补充徽章奖励）**

- 接口：`GET /achievements`（公开，可选登录，登录时含解锁状态与逐条件进度；`includeInactive=true` 需要 `gamification.achievement.manage`）、`GET /users/me/achievements`（登录）、`POST /achievements`、`PUT /achievements/:id`、`DELETE /achievements/:id`（`gamification.achievement.manage`）。
- 条件类型：`ac_count`（去重 AC 题目数）、`contest_count`（报名比赛数）、`login_streak`（截至今天的连续登录天数）、`created_contest`（创建的比赛数）、`all_of`（2–8 个非 `all_of` 子条件，不允许嵌套）；阈值必须为正整数，非法配置返回 40001。
- 事件：judge 在提交 AC 后投递 `submission.accepted`，注册/登录后投递 `user.login`，比赛报名后投递 `contest.join`；队列 `game`、任务类型 `game:event`，最多重试 5 次。worker 与读取接口都调用同一个 `EvaluateUser`，重复/丢失事件最终收敛。
- 奖励：金币写入 `coin_transactions`（`reason=achievement`、`ref_type=achievement`），与解锁记录同一事务，唯一约束保证只发一次。M3-B 起 `rewardBadgeId` 生效：配置时徽章必须存在且上架，否则返回 40001；发放时直接写入 `user_badges`，不占用商城库存。
- 可见性：已解锁成就下线后仍对本人可见；未解锁且已下线的成就仅拥有 `gamification.achievement.manage` 者通过 `includeInactive=true` 看到。
- 前端成就页（`/achievements`）已完成：卡片网格、全部/已解锁/未解锁筛选、条件进度、奖励与解锁时间；已接入侧边栏「成长」子菜单。

**Codex 提示词（成就）**

```text
你是一名后端工程师，请为 kapOJ 实现「成就」模块（游戏化系统的一部分）。

技术栈：Go + Gin + GORM + PostgreSQL + Redis + Asynq。

请完成：
1. 数据模型：Achievement、UserAchievement（字段见开发文档第 3.5 节）。
2. 事件驱动成就判定：
   - 定义事件类型：submission.accepted、user.login、contest.join、solution.liked 等
   - 使用 Asynq 或 Redis Pub/Sub 发布事件，成就服务订阅并评估条件
   - 条件配置支持：阈值计数（如 AC 数量 ≥ 100）、累计行为计数等
3. 后端接口：
   - 成就 CRUD：GET/POST/PUT/DELETE /achievements（管理员）
   - GET /achievements：成就列表（含用户是否已解锁）
   - GET /users/me/achievements：我的成就（已解锁 + 未解锁）
4. 解锁时：写入 UserAchievement，发放奖励（金币写入 coin_transactions，徽章写入 user_badges）。
5. 验收标准：完成动作后对应成就自动解锁，奖励正确发放且不重复发放。

请输出完整代码、事件定义与类型定义。
```

---

### 5.2 任务模块

**功能需求**

- 管理员发布一系列题目任务；已完成过的用户无法再次领取。
- 用户完成任务后获得金币奖励。
- 金币是系统流通货币，可用于兑换称号、购买徽章等。

**关键规则**

- **领取**：任务在有效期内，且用户尚未领取过该任务。
- **完成**：用户对任务中的全部题目都获得 AC，即视为完成。完成后不可再次领取。
- **结算**：完成由 AC 事件自动触发并发放金币，无需用户手动提交。同一任务只结算一次（依靠唯一约束，见 2.4 幂等约定）。

**实现状态（M3-A 后端、M3-C 前端已完成）**

- 接口：`GET /tasks`（公开，可选登录；`includeInactive=true` 需要 `gamification.task.manage`）、`POST /tasks`、`PUT /tasks/:id`、`DELETE /tasks/:id`（`gamification.task.manage`）、`POST /tasks/:id/claim`（登录）。
- 题目为 1–20 个不重复的公开题目；奖励金币 0–1000000；`start_time < end_time`，领取要求 `start <= now < end`。
- 在领取窗口内重复领取返回当前记录（幂等）；任务已完成、未开始、已截止或已下线返回 40001；任务不存在返回 40400。
- 完成判定只统计 `claimed_at <= judged_at < end_time` 的去重 AC；领取前已 AC 的题目不计入。事件丢失时，读取任务大厅或登录/AC 事件会补算，`user_task_records` 行锁保证 `claimed → completed` 只发生一次，金币只发一次。
- 已有任一领取记录后冻结 `problem_ids_json` 与 `reward_coins`，只允许修改标题、描述、时间与上下架；软删除后已领取用户仍可在截止前完成并领奖。
- 展示状态：`upcoming`、`active`、`claimed`、`completed`、`expired`、`ended`、`inactive`；`expired` 只在读取时按时间计算，不写入数据库。
- 前端任务大厅（`/tasks`）已完成：任务卡片显示奖励、时间、题目数、状态与进度；登录后可领取，领取前二次确认，成功后刷新进度；已接入侧边栏「成长」子菜单。

**Codex 提示词（任务）**

```text
你是一名全栈工程师，请为 kapOJ 实现「任务」模块。

技术栈：Go + Gin + GORM + PostgreSQL 后端，Vue3 + Element Plus 前端。

请完成：
1. 数据模型：Task、UserTaskRecord（字段见开发文档第 3.5 节）。
2. 后端接口：
   - 任务 CRUD：GET/POST/PUT/DELETE /tasks（管理员）
   - GET /tasks：用户可见任务列表（含领取状态）
   - POST /tasks/:id/claim：领取任务（校验未领取、未过期、未完成过）
   - 完成由 AC 事件自动触发，发放金币（写入 coin_transactions），不提供手动完成接口
3. 规则：任务由多个 problem_ids 组成，用户需对全部题目获得 AC 才算完成；已完成该任务的用户不可再次领取。
4. 前端：任务大厅（任务卡片、金币奖励、领取/进度/完成状态）、我的任务。
5. 验收标准：未完成用户可领取并完成后获得金币，已完成用户无法重复领取。

请输出完整代码与类型定义。
```

---

### 5.3 徽章模块

**功能需求**

- 管理员上传徽章（图片 + 名称 + 价格），用户可购买。
- 节假日可上架特殊限量徽章。
- 用户装扮徽章后，在测评记录、排行榜、个人中心等处对所有用户可见。

**关键设计**

- 徽章购买走金币流水（`coin_transactions`），扣减用户 `coins`。
- 装扮：`user_badges.is_equipped`，每位用户最多装备 3 个徽章（上限可在系统配置中调整）。
- 公开接口需返回用户装备徽章列表，供测评记录、排行榜、个人中心展示。
- 限量库存：购买时在同一事务中对徽章行加锁（`SELECT ... FOR UPDATE`）再扣减 `stock`，避免超卖。

**实现状态（M3-B 后端、M3-C 前端已完成）**

- 接口：`GET /badges`（公开，可选登录；登录时含 `owned`/`equipped`；`includeInactive=true` 需要 `gamification.badge.manage`）、`POST /badges`、`PUT /badges/:id`、`DELETE /badges/:id`（`gamification.badge.manage`）、`POST /badges/:id/image`（`gamification.badge.manage`，multipart 字段 `file`）、`GET /badges/:id/image`（公开）、`POST /badges/:id/purchase|equip|unequip`（登录）、`GET /users/:id/badges`（公开）。
- 装备上限由 `KAPOJ_MAX_EQUIPPED_BADGES` 配置（1–10，默认 3）；装备前锁定用户行并重新统计，避免并发超限；已装备时重复装备、未装备时卸下均幂等成功。
- 购买在同一事务内锁定徽章行与用户行，校验上架、未拥有、限量库存与余额，扣库存/扣金币/写 `coin_transactions`（`reason=badge_purchase`、`ref_type=badge`）/写 `user_badges`；已拥有、库存不足、余额不足或已下线返回 40001。
- 管理端更新在同一事务内锁定徽章行并只更新提交字段：`stock` 省略时保留当前库存（包括并发购买扣减），`imageUrl` 留空时保留现有图片，`isActive` 省略时保留上下架状态，避免覆盖并发购买、图片上传或软删除。
- 图片仅支持 PNG/JPEG/WebP/GIF 且不超过 2 MiB；保存为存储键 `badges/<id>`，`image_url` 写为 `/api/badges/<id>/image`，公开读取响应带 `Cache-Control: public, max-age=3600`。外链或文件缺失返回 40400。
- 软删除后已购买用户仍保留并展示；成就奖励徽章直接写入 `user_badges`，不占用商城库存。
- 前端徽章商城（`/shop/badges`）与装扮管理（`/appearance`）已完成：支持分类筛选、购买确认、余额/库存/拥有状态、装备与卸下，并在购买或装备后刷新用户余额；已接入侧边栏「成长」子菜单。

**Codex 提示词（徽章）**

```text
你是一名全栈工程师，请为 kapOJ 实现「徽章」模块。

技术栈：Go + Gin + GORM + PostgreSQL + MinIO（对象存储）后端，Vue3 + Element Plus 前端。

请完成：
1. 数据模型：Badge、UserBadge（字段见开发文档第 3.5 节）。
2. 图片上传：徽章图片上传到 MinIO，存储 URL；支持管理员上传。
3. 后端接口：
   - 徽章 CRUD：GET/POST/PUT/DELETE /badges（管理员）
   - GET /badges：徽章商城（含用户是否已拥有）
   - POST /badges/:id/purchase：购买（校验金币余额，扣款并写入流水）
   - POST /badges/:id/equip / unequip：装扮/取消装扮
   - GET /users/:id/badges：用户徽章（含装备中）
4. 前端：徽章商城（图片墙、价格、购买按钮）、个人中心装扮管理（装备/卸下）。
5. 公开展示：测评记录、排行榜、个人中心展示用户装备的徽章。
6. 验收标准：可上传/购买/装备徽章，金币正确扣减，公开页面正确展示装备徽章。

请输出完整代码与类型定义。
```

---

### 5.4 称号模块

**功能需求**

- 称号可由用户解锁后购买，购买后在用户中心装扮。
- 装扮后在测评记录、排行榜、个人中心等对所有用户可见。
- 解锁条件示例：Rating 达到某等级、AC 题数达到阈值、获得特定成就等。
- 同一用户同时只能装备 1 个称号，装备新称号时自动卸下原称号。

**实现状态（M3-B 后端、M3-C 前端已完成）**

- 接口：`GET /titles`（公开，可选登录；登录时含解锁/购买/装备状态与条件进度；`includeInactive=true` 需要 `gamification.title.manage`）、`POST /titles`、`PUT /titles/:id`、`DELETE /titles/:id`（`gamification.title.manage`）、`POST /titles/:id/purchase|equip|unequip`（登录）。
- 解锁条件：`rating`（当前 Rating ≥ 阈值）、`ac_count`（去重 AC 题目数 ≥ 阈值）、`achievement`（已解锁指定 code 的成就）；读取列表时先补算解锁，不扣金币。M4-A 已实现 Rating 结算，Rating 条件自然可达。
- 购买要求已解锁且未购买，在同一事务内扣金币、写 `coin_transactions`（`reason=title_purchase`、`ref_type=title`）并记录购买时间；已下线但已解锁的称号仍可购买。
- 装备要求已购买；同一事务内先卸下该用户全部称号，再装备目标称号，并发下始终最多 1 个；重复装备幂等成功。
- 软删除后已解锁/已购买记录保留，仍可在“我的称号”和装扮管理中查看、购买（若已解锁未购买）和装备。
- 前端称号商店（`/shop/titles`）与装扮管理已完成：显示解锁条件进度、未解锁/可购买/已购买/装备中状态、彩色名称，并支持购买、装备与卸下；已接入侧边栏「成长」子菜单。

**Codex 提示词（称号）**

```text
你是一名全栈工程师，请为 kapOJ 实现「称号」模块。

技术栈：Go + Gin + GORM + PostgreSQL 后端，Vue3 + Element Plus 前端。

请完成：
1. 数据模型：Title、UserTitle（字段见开发文档第 3.5 节）。
2. 解锁逻辑：称号有 unlock_condition_type（如 rating、ac_count、achievement）与 unlock_condition_json，满足条件后自动解锁；解锁后才能购买。
3. 后端接口：
   - 称号 CRUD：GET/POST/PUT/DELETE /titles（管理员）
   - GET /titles：称号列表（含解锁/购买状态）
   - POST /titles/:id/purchase：购买（校验已解锁 + 金币余额）
   - POST /titles/:id/equip / unequip：装扮/取消装扮
4. 前端：称号商店、个人中心装扮管理（显示称号颜色/样式）。
5. 公开展示：测评记录、排行榜、个人中心展示用户装备的称号。
6. 验收标准：达到条件自动解锁，解锁后可购买并装扮，公开页面正确展示。

请输出完整代码与类型定义。
```

---

### 5.5 排行榜模块

**功能需求**
排行榜分三个模块：

1. **用户比赛 Rating 排行榜**（默认）
   - 按比赛 Rating 排名，计算规则参考 Codeforces。
   - 按 Rating 分等级：灰名、绿名、青名、蓝名、紫名、橙名、红名、黑红名（首字符黑、后续红）。

2. **用户活跃度排行榜**
   - 统计用户所有交互行为积分：提交代码、评论、点赞、关注、参与比赛等。

3. **用户共享排行榜**
   - 针对用户创建的比赛、提供的题解被点赞情况进行积分排名。

**Rating 计算（参考 Codeforces Elo）**

- 新用户初始 Rating（如 1500）。
- 每场 Rated 比赛结束后，根据选手相对排名与对手 Rating，用 Elo 变体算法更新 Rating。
- 结算时机：Rated 比赛结束后由异步任务结算，结果写入 `rating_history`。每人每场只结算一次（唯一约束），任务可安全重试。
- 公式：采用 Codeforces 风格的 Elo 变体。具体公式在实现时确定，并在对应代码注释中记录版本。
- 等级阈值（M4-A 固定阈值）：

| 等级   | 颜色                           | Rating 范围 |
| ------ | ------------------------------ | ----------- |
| 灰名   | #808080                        | 0 ~ 1199    |
| 绿名   | #008000                        | 1200 ~ 1399 |
| 青名   | #03A89E                        | 1400 ~ 1599 |
| 蓝名   | #0000FF                        | 1600 ~ 1899 |
| 紫名   | #AA00AA                        | 1900 ~ 2099 |
| 橙名   | #FF8C00                        | 2100 ~ 2399 |
| 红名   | #FF0000                        | 2400 ~ 2599 |
| 黑红名 | 用户名首字符为黑色、其余为红色 | 2600+       |

**活跃度积分（示例）**

| 行为     | 积分 |
| -------- | ---- |
| 提交代码 | 2    |
| 评论     | 3    |
| 点赞     | 1    |
| 关注     | 2    |
| 参与比赛 | 10   |
| 每日签到 | 5    |

积分防刷规则：每类行为设每日积分上限（如评论、点赞各 20 分），签到每天只计一次；同一对象的重复点赞、重复关注不重复计分。

> M4-B 实现提交 +2（每日上限 40）、参赛 +10（每日上限 50）与签到 +5；M4-3 接入论坛回复 +3、点赞 +1、关注 +2（每类每日上限 20，唯一 ref 去重）。

**共享积分（示例）**

| 行为             | 积分 |
| ---------------- | ---- |
| 创建比赛         | 20   |
| 提供的题解被点赞 | 5/赞 |

> M4-B 实现创建/克隆比赛 +20；M4-3 实现题解每赞 +5（`shared_items(type=solution)` 的 `like_count` 累计），取消赞扣回。

**实现状态（M4-A/B 后端与 M4-C 前端已完成）**

- **Rating 算法 v1**：设参与人数 `n`，赛前 Rating 为 `R_i`，期望得分 `E_i = Σ_{j≠i} 1 / (1 + 10^((R_j - R_i)/400))`，实际得分 `A_i = n - rank_i`（并列同名次），`delta_i = round(32 * (A_i - E_i))` 并限制在 `[-100, 100]`，新 Rating 为 `max(0, R_i + delta_i)`；`n <= 1` 时 delta 为 0，仍写历史。同一场使用结算开始时的 Rating 快照。K=32 与 ±100 固定在 `rating/algorithm.go` 的版本注释和测试中。
- **参与判定**：只统计比赛报名者中在比赛时间窗内至少有 1 次已判（非 `pending`/`judging`）提交的用户；0 次已判提交不参与。结算复用现有 ICPC/IOI/OI 榜单，ICPC 路径补齐内部 `LastSubmissionAt`；宽限结束后仍只统计已判提交。
- **结算与幂等**：扫描器默认每 60 秒处理最多 20 场，按 `end_time, id` 升序；结算事务先获取 PostgreSQL 全局事务级 advisory lock，再锁定比赛行并复查 `is_rated && end_time < now && rating_settled_at IS NULL`。不同比赛不会并发读取同一批用户的旧 Rating。逐人 `INSERT rating_history ... ON CONFLICT DO NOTHING`，只有插入成功才更新 `users.rating`；全部完成后设置 `rating_settled_at`，无参与者也会设置；重复扫描/多实例不会重复写历史或重复加分。
- **评测宽限**：若比赛仍有 `pending`/`judging` 提交且 `now < end_time + KAPOJ_RATING_JUDGE_GRACE`，本轮跳过且不设置标记；超过宽限后按已判结果结算，避免永久卡死。`KAPOJ_RATING_SCAN_INTERVAL` 取值 10s–10m（默认 60s），`KAPOJ_RATING_JUDGE_GRACE` 取值 1m–60m（默认 10m）。
- **等级**：8 档灰/绿/青/蓝/紫/橙/红/黑红，阈值与颜色见上表；接口返回 `level: {key,name,color,accentColor}`。黑红名 `color=#000000`、`accentColor=#FF0000`，其他等级 `accentColor` 为空字符串。
- **Rating 榜**：`GET /api/leaderboard/rating?page=&page_size=`，公开；只含 `status=active` 用户，按 Rating 倒序、用户 ID 升序，`score` 为 Rating，行内附带 `equippedBadges`/`equippedTitle`。基础行 Redis 缓存 60 秒，key 为 `kapoj:leaderboard:rating:v<version>:<page>:<size>`；结算成功后递增 `kapoj:leaderboard:rating:version`；Redis/缓存故障时回退数据库；缓存故障本身不会导致 5xx（身份装饰查询失败仍可能返回 5xx）。
- **Rating 历史**：`GET /api/users/:id/rating-history?page=&page_size=`，公开；按 `created_at DESC, id DESC` 返回所有 kind（`contest`/`decay`/`season_reset`），字段为 `kind`、`contestId`、`contestTitle`、`rank`、`periodKey`、`ratingBefore`、`ratingAfter`、`delta`、`createdAt`；用户不存在返回 40400。非比赛行 `contestId=0`、`rank=0`、标题分别为「Rating 衰减」「赛季重置」，`periodKey` 为 `YYYY-MM`（衰减）或赛季开始日（重置）；比赛行 `periodKey=''`，私密比赛仍脱敏为「私密比赛」且 `contestId=0`。分页 total 包含所有 kind。
- **活跃度（M4-B + M4-3）**：提交创建成功 +2（每日上限 40）、比赛报名成功 +10（每日上限 50）、每日签到 +5（每天一次）、论坛回复 +3、题解点赞 +1、关注 +2（后三类每日上限均 20）；`activity_logs` 唯一约束 `(user_id, action, ref_type, ref_id)` 保证同一提交/比赛/日期/回复/点赞对象/关注对象不重复计分。去重、每日上限与用户行锁在同一事务内完成；达到上限返回 no-op，不影响主流程；取消赞/取消关注不扣分。
- **签到金币（M4-B）**：每个本地自然日首次 `POST /api/activity/signin` 同时奖励 5 活跃度与 5 金币，签到日志、`coin_transactions`（`reason=signin`、`ref_type=day`、`ref_id=YYYYMMDD`）与 `users.coins` 在同一事务内提交；重复签到返回 `awarded=false`、`activityPoints=0`、`coins=0`，不重复发币。`GET /api/activity/summary` 返回 `signedInToday`、`todayPoints`、`totalPoints`。
- **共享积分（M4-B + M4-3）**：创建或克隆比赛成功后写入 `shared_items(type=contest)`，每场 20 分；启动时幂等回填已有 `created_by>0` 的比赛。题解帖子被赞时 `shared_items(type=solution)` upsert `like_count` +1（取消赞 -1 不删除行），作者共享分 +5/赞；帖子删除移除共享项。共享分公式 `20 * 比赛共享数 + 5 * 题解点赞数` 两项均已生效。
- **活跃度/共享榜（M4-B）**：`GET /api/leaderboard/activity`、`GET /api/leaderboard/sharing`，公开；只含 `status=active` 且分数 > 0 的用户，按分数倒序、用户 ID 升序，分页上限 100。行内 `score` 为积分，并返回 `equippedBadges`/`equippedTitle`；这两个榜不返回 Rating `level`。
- **榜单缓存（M4-B）**：活跃度/共享榜基础行 Redis 缓存 60 秒，key 为 `kapoj:leaderboard:activity:v<version>:<page>:<size>` 与 `kapoj:leaderboard:sharing:v<version>:<page>:<size>`；活跃度写入后递增 `kapoj:leaderboard:activity:version`，共享项写入/回填后递增 `kapoj:leaderboard:sharing:version`。只缓存基础行，装备变更无需失效；Redis/缓存故障时回退数据库，缓存故障本身不返回 5xx（身份装饰查询失败仍可能返回 5xx）。
- **排行榜页（M4-C）**：`/leaderboard` 提供 Rating（默认）、活跃度、共享三个 Tab；每个 Tab 独立维护页码、加载、错误与空态，切换不串数据，分页每页 20 条。Rating Tab 展示服务端等级颜色，黑红名首字符黑、其余红；用户列链接个人主页并展示装备称号/徽章；活跃度/共享榜分数使用千分位。导航“排行榜”已启用，首页提供快捷入口。
- **签到入口（M4-C）**：活跃度 Tab 对登录用户展示今日活跃度、总活跃度与“签到 +5 金币”按钮；首次签到按服务端返回值提示 +5 活跃度/+5 金币，成功后刷新登录用户、签到摘要与活跃度榜第 1 页；当日已签到显示禁用按钮与中文状态；游客显示登录 CTA；失败展示服务端中文错误。
- **Rating 历史（M4-C）**：用户主页展示最近 20 条 Rating 历史表格与简单 SVG 折线（无新增图表依赖）；私密比赛 `contestId=0` 显示“私密比赛”且不生成链接；无记录显示“暂无 Rating 记录”。
- **Rating 衰减与赛季重置（M4-4）**：复用 Rating 扫描器 tick（默认 60s，可用 `KAPOJ_RATING_SCAN_INTERVAL` 调整），在比赛结算后调用 `RunAdjustments(now)`，与结算共用同一 PostgreSQL 全局 advisory lock；不新增独立 worker 进程。策略存于单例 `rating_settings`（管理页 `/admin/rating` 或 `GET/PUT /api/admin/rating/settings`，需 `rating.settings.manage`），默认：衰减启用、阈值 2400、未活跃 60 天、月比例 3%、单次上限 100；赛季启用、周期 6 个月、重置比例 30%、锚点为首次读取时的当月 1 日（UTC）。
  - **衰减**：仅 `status=active` 且 `rating > decayThreshold`、最近一次 `kind=contest` 历史距今 ≥ `decayInactiveDays` 的用户；每月最多执行一次（运行键 `decay:YYYY-MM`，UTC）。单次 `above=rating-threshold`、`raw=round(above*percent/100)`、`decay=min(max,max(1,raw))`、`new=max(threshold,rating-decay)`，取整统一 `math.Round`；`rating>threshold` 时至少扣 1。写入 `kind=decay`、`periodKey=YYYY-MM`、`rank=0`；仅插入历史成功（`ON CONFLICT DO NOTHING`）才更新 `users.rating`。跨月停机只处理当前月，不回补。
  - **赛季重置**：赛季 `[anchor + n*months, anchor + (n+1)*months)`（anchor 必须是某月 1 日，UTC）；赛季结束后首次扫描处理「刚结束的上一赛季」（运行键 `season:<上一赛季开始日>`），跨多赛季只处理最近结束的上一赛季。所有 `rating != 1500` 用户 `new=max(0, round(1500+(rating-1500)*(100-resetPercent)/100))`（默认 30%：2600→2270、1200→1290）；`new==rating`（含 1500、0%、四舍五入不变）不写历史、不更新 Rating，但该赛季运行仍标记已处理。写入 `kind=season_reset`、`periodKey=赛季开始日`、`rank=0`。首次创建默认设置与修改 `seasonAnchor`/`seasonMonths` 时，预标记新周期「上一赛季」运行键为已跳过，不立即触发重置。
  - **幂等与缓存**：`rating_runs` 唯一键 + 认领与处理同事务（失败回滚）；成功修改 Rating 后 best-effort 递增 `kapoj:leaderboard:rating:version`，Redis 故障只记日志。公开赛季接口 `GET /api/rating/season/current`（游客可读）返回 `{key,name,startsAt,endsAt,nextResetAt}`；排行榜页顶部展示当前赛季与下次重置（失败静默降级）。
- **尚未实现**：学习路线前端已随 M4-2C 完成；社交互动（题解点赞/关注/通知）已随 M4-3 完成（见 4.8）。排行榜与社交后续无剩余 M4 子任务。

**Codex 提示词（排行榜）**

```text
你是一名后端工程师，请为 kapOJ 实现「排行榜」模块（三个子榜单 + Rating 系统）。

技术栈：Go + Gin + GORM + PostgreSQL + Redis（缓存榜单）后端。

请完成：
1. Rating 系统：
   - 实现 Codeforces 风格 Elo 变体算法：赛后根据选手排名与对手 Rating 更新 Rating
   - 新用户初始 Rating 1500
   - 按 Rating 分 8 个等级（灰/绿/青/蓝/紫/橙/红/黑红），提供等级与颜色映射
2. 活跃度积分：
   - 在关键行为处写 activity_logs（提交/评论/点赞/关注/参赛/签到）
   - 提供 GET /leaderboard/activity：按积分降序排名
3. 共享积分：
   - 统计用户创建的比赛数、题解被点赞数，计算积分
   - 提供 GET /leaderboard/sharing：按积分降序排名
4. 后端接口：
   - GET /leaderboard/rating：Rating 榜（默认）
   - GET /leaderboard/activity：活跃度榜
   - GET /leaderboard/sharing：共享榜
   - 支持分页、返回用户等级颜色与装备徽章/称号
5. 使用 Redis 缓存榜单快照，定时刷新。
6. 验收标准：三个榜单数据正确，Rating 赛后更新正确，等级颜色映射正确。

请输出完整代码、Rating 算法实现与类型定义。
```

---

### 5.6 证书认证模块

**功能需求**

- 用户上传/提交参加比赛的证书进行认证。
- 支持证书类型：蓝桥杯、ACM-ICPC、CCCC 天梯赛、CCPC、NOI、NOIP、USACO 等；管理员可添加其他类型。
- 认证流程：用户提交 → 管理员审核 → 通过/驳回（附原因）。
- 认证通过后在个人中心展示。每张通过的证书生成唯一的验证码和公开验证页面，任何人输入验证码即可核验；证书图片仅本人与管理员可见。

**Codex 提示词（证书认证）**

```text
你是一名全栈工程师，请为 kapOJ 实现「证书认证」模块。

技术栈：Go + Gin + GORM + PostgreSQL + MinIO 后端，Vue3 + Element Plus 前端。

请完成：
1. 数据模型：Certificate、CertificateType（字段见开发文档第 3.7 节）。
2. 后端接口：
   - 证书类型 CRUD：GET/POST/PUT/DELETE /certificate-types（管理员，可扩展添加比赛类型）
   - POST /certificates：用户提交认证（上传证书图片、选择类型、填写等级）
   - GET /certificates/my：我的认证列表
   - GET /certificates：管理员审核列表（按状态筛选）
   - POST /certificates/:id/review：审核通过/驳回（驳回需填写原因）
3. 前端：证书提交页、我的证书、管理员审核页。
4. 验收标准：用户可提交证书，管理员可审核通过/驳回，状态流转正确，证书展示正确。

请输出完整代码与类型定义。
```

**实现状态（M5-7 ✅，2026-11-08）与 API**

- **数据模型**：`certificate_types`（name 唯一、`is_active`、`sort_order`）与 `certificates`（`certificate_type_name` 快照、`status`、`reject_reason`、`verify_code`、`image_key`/`image_content_type`、`reviewed_by`），字段与索引见 3.7；`db.Migrate` 建表，首次启动空表 seed 7 个类型。
- **权限**：新增第 37 个权限码 `certificate.manage`（分组「证书」，默认仅 `super_admin`，super 绕过），用户提交/我的证书/图片只需登录；公开类型列表/公开核验无需登录。
- **图片**：≤5 MiB，magic bytes 校验仅 PNG/JPEG/WebP（`http.DetectContentType`），本地 `storage.Storage` 保存 `certificates/<id>`；`GET /api/certificates/:id/image` 仅本人或 `certificate.manage`（他人/不存在统一 404，未登录 401），不暴露 storage key。
- **提交**：`POST /api/certificates`（multipart `typeId`/`level`/`file`）→ pending；类型不存在/停用、level 空/超 50、空图/超 5 MiB/非图片均 40001；上传或回填失败删除行与对象。
- **审核**：`GET /api/admin/certificates?status=&page=&page_size=`（默认 pending）与 `POST /api/admin/certificates/:id/review`（`{action:"approve"|"reject", reason?}`）；仅 pending 可审，approve 生成唯一验证码，reject 需 1–500 字原因；终态/重复审核 40001。
- **公开核验**：`GET /api/certificates/verify?code=`（大小写/去空格/去连字符/省略 `KAPOJ` 前缀容错）仅 approved 返回 `{valid:true,type,level,nickname,approvedAt}`；未知/pending/rejected 返回 `{valid:false}` 200，绝不返回图片/邮箱/用户 ID/审核人/驳回原因。
- **类型 CRUD**：`GET /api/admin/certificate-types?includeInactive=true`、`POST/PUT/DELETE /api/admin/certificate-types[/:id]`；改名不回溯历史（快照），停用后公开列表不返回，被引用删除 40001。
- **审计**：审核写审计 action=`certificate.manage`、target=`certificate`、targetID=证书 ID、summary「通过证书/驳回证书」（驳回原因不写审计）；类型写操作（POST/PUT/DELETE）经 `AuditAdminAction` 写审计 target=`certificate_type`（GET 列表为只读不写审计）。

**关键接口**

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/certificate-types` | 公开 | 启用的证书类型 |
| POST | `/api/certificates` | 登录 | multipart 提交（`typeId`/`level`/`file`，≤5 MiB PNG/JPEG/WebP） |
| GET | `/api/certificates/my` | 登录 | 我的证书分页 |
| GET | `/api/certificates/:id/image` | 登录（本人/管理员） | 证书图片；他人/不存在 404，未登录 401 |
| GET | `/api/certificates/verify?code=` | 公开 | 公开核验，仅 approved 返回 valid=true |
| GET | `/api/admin/certificates?status=&page=&page_size=` | `certificate.manage` | 审核列表（默认 pending） |
| POST | `/api/admin/certificates/:id/review` | `certificate.manage` | 审核通过/驳回 |
| GET | `/api/admin/certificate-types?includeInactive=` | `certificate.manage` | 全部类型（含停用） |
| POST/PUT/DELETE | `/api/admin/certificate-types[/:id]` | `certificate.manage` | 类型 CRUD |

**前端页面与路由**

- `/settings/certificates`（路由 `certificate-settings`，`requiresAuth`）：我的证书列表（状态 Tag/驳回原因/验证码复制链接）+「申请认证」提交 Dialog（类型/等级/图片 blob 预览），图片通过鉴权 blob 加载。
- `/verify`（路由 `certificate-verify`，公开，支持 `?code=` 自动核验）：输入验证码核验，展示类型/等级/持有者昵称/通过日期，不展示图片。
- `/admin/certificates`（路由 `admin-certificates`，`permission: certificate.manage`）：证书审核 Tab（状态筛选/分页/图片预览/通过/驳回，驳回必填原因）+ 证书类型 Tab（CRUD）。
- 入口：`UserMenu`「我的证书」；`AdminNav` 内容组「证书审核」（`certificate.manage`）。

---

## 6. AI 辅助功能模块

**实现状态（M5-A ✅；M5-1～M5-3、M5-4、M5-5 已完成；M5-7 证书认证已完成；M5-8 头像上传待办）**

- **路由挂载**：AI 路由（`/api/ai/configs` GET/POST、`/api/ai/complete`、`/api/ai/format`、`/api/ai/optimize`、`/api/ai/hint`）**始终挂载**（`ai.Service` 恒非 nil）；管理员在 `/admin/ai` 关闭 AI 后，`RequireAIEnabled` 中间件让所有 `/api/ai/*` 统一返回 404（见 §6.6）。未配置密钥时后端也正常启动，AI 路由仍可访问（`GET /api/ai/configs` 返回 200 + `keyConfigured=false`），保存配置/AI 调用返回 40001（见 §6.7 无密钥语义）。
- **密钥来源**：加密密钥来自环境变量 `KAPOJ_AI_ENCRYPTION_KEY`（优先）或本地密钥文件 `KAPOJ_AI_KEY_FILE`（默认 `data/ai.key`）；env 非法 base64/长度在启动时报错，文件缺失/非法则视为未配置（后台可生成/覆盖）。详见 §6.7。
- **provider**：启动时幂等 seed 四个内置 provider（`deepseek`/`openai`/`qwen`/`glm`，见 3.9）；只有 `is_active=true` 的 provider 可用，未知/停用 provider 被拒绝。
- **加密**：用户 API key 使用 AES-256-GCM 加密存储（随机 nonce，`v1:` 前缀），接口只返回脱敏值，不写日志/审计；平台 provider 的密钥不写入数据库。
- **OpenAI 客户端**：`POST {base_url}/chat/completions`，Bearer key，解析 `choices[0].message.content`；非 2xx / 空 choices / 畸形 JSON / 超时统一映射为 `50200 AI 服务调用失败`（`KAPOJ_AI_REQUEST_TIMEOUT` 控制超时，默认 45s），不泄露 key 与上游响应体敏感内容。
- **限流**：补全/格式化/优化/首次生成提示按用户 Redis 冷却（`kapoj:ai:cool:<userID>`，`SetNX`，`KAPOJ_AI_COOLDOWN` 默认 2s），超限返回 `42903`；Redis 故障返回 `50300`；缓存命中的提示不消耗冷却。
- **提示缓存**：`POST /api/ai/hint` 登录 + `LoadAccess`；缓存命中直接返回 `cached=true`，未命中要求用户配置并调用模型后 `INSERT ... ON CONFLICT (problem_id) DO NOTHING` 写入缓存；非公开题对无 `problem.view_hidden` 的用户返回 40400。
- **范围外**：M5-8（头像上传）尚未实现（M5-4 出题造数据见 §6.4，M5-5 AI 前端见 §6.5，M5-6 HydroOJ 导入见 §7，M5-7 证书认证见 §5.6）；M5-A 不新增管理员权限码。

### 6.1 功能清单

1. **代码自动补全**：编辑器内基于上下文补全代码。
2. **代码自动格式化**：一键格式化代码。
3. **代码自动优化**：AI 分析并给出优化建议/优化后代码。
4. **出题造数据**：创建题目时读取标程，AI 生成纯 Python 3 标准库造数据脚本（stdin 读 seed、stdout 输出输入），judge 在 Docker 沙盒运行脚本生成 input、运行标程生成 output，管理员预览确认后写入题目测试数据（见 §6.4）。
5. **AI 辅助解题**：题目右侧「AI 提示」按钮，点击查看 AI 提示；首个用户点击后提示缓存到题目，其他用户也可查看。
6. **多模型接入**：用户需自行填写 API key，支持 DeepSeek、OpenAI、Qwen、GLM 等模型（OpenAI 兼容协议）。

### 6.2 架构设计

- 后端提供统一 `AIService`，抽象 `chat()` / `complete()` / `optimize()` 等接口。
- 通过 OpenAI 兼容协议接入多模型（`base_url` + `api_key` + `model_name`）。
- 用户 API key 存 `user_ai_configs`（加密存储），请求时按用户选择的路由到对应模型。
- 平台级 provider 的密钥不写入数据库，通过环境变量或密钥服务引用（`ai_providers.api_key_ref`）。
- 提示缓存：`ai_hints` 表，首个用户触发后写入，后续用户直接读取缓存，降低 AI 调用成本。

### 6.3 Codex 提示词（AI 辅助 - 后端）

```text
你是一名 AI 工程专家，请为 kapOJ 实现「AI 辅助」模块的后端部分。

技术栈：Go + Gin + GORM + PostgreSQL + Redis。

请完成：
1. 统一 AIService，基于 OpenAI 兼容协议接入多模型（DeepSeek、OpenAI、Qwen、GLM）。通过 base_url + api_key + model_name 配置不同 provider。
2. 用户 API key 管理：
   - POST /ai/configs：保存用户 AI 配置（provider、api_key、model_name），api_key 使用 AES 加密存储
   - GET /ai/configs：返回配置（api_key 脱敏显示）
3. 接口：
   - POST /ai/complete：代码自动补全（接收 code + cursor 位置，返回补全片段）
   - POST /ai/format：代码格式化（返回格式化后代码）
   - POST /ai/optimize：代码优化（返回优化建议 + 优化后代码）
   - POST /ai/hint：获取题目 AI 提示；若 ai_hints 已有缓存则直接返回，否则调用模型生成并缓存
4. 提示缓存：题目首个用户触发后写入 ai_hints 表，后续用户复用。
5. 错误处理：API key 缺失/无效、模型超时、限流。
6. 验收标准：可配置多模型，四个 AI 接口可用，提示缓存生效。

请输出完整代码与类型定义。
```

### 6.4 AI 出题造数据（M5-4 ✅）

**实现状态（M5-4 ✅）**

> 目标：管理后台在题目编辑中提交标程，AI 生成纯 Python 3 造数据脚本；judge 在 Docker 沙盒运行脚本生成输入、运行标程生成输出；管理员预览确认后写入题目测试数据。设计见 [`docs/superpowers/specs/2026-11-08-m5-4-ai-testdata-design.md`](superpowers/specs/2026-11-08-m5-4-ai-testdata-design.md)。

- **脚本契约（纯 Python 3 标准库，替代 cyaron 旧设想）**：AI 生成的造数据脚本必须是**纯 Python 3 标准库**脚本——沙盒镜像 `python:3.12-slim` 不含 cyaron/第三方包，且沙盒无网络无法安装，故早期「用 cyaron 生成」设想改为纯标准库。脚本从 **stdin 读取一个 1-based 整数 seed**（`1`、`2`…，即组号），建议用 `random.Random(seed)` 生成确定性随机数据，向 **stdout 输出一组输入**；不写文件、不联网、不读取环境变量；输出 ≤ 16 MiB。
- **数据模型**：`testcase_generations`（`id/problem_id/user_id/status/language/standard_code/generator_code/testcase_count/time_limit_ms/memory_limit_mb/error_message/created_at/updated_at`，索引 `(problem_id, created_at)`）与 `testcase_generation_cases`（`id/generation_id/sort_order/input_file/output_file/input_size/output_size`，唯一 `(generation_id, sort_order)`）。表由服务器 `db.Migrate` 创建，judge 只读写。
- **状态机**：`pending → running → ready / failed`；`ready` 可确认（`confirmed`）或丢弃（`discarded`）；`failed` 可丢弃；`confirmed` 不可丢弃；`pending/running` 不能丢弃。judge 任务 `MaxRetry(0)`、超时 30 分钟，失败由管理员重新发起，不自动重试。
- **judge 任务**：`judge:testcase-generation`（队列 `judge`，payload `{"generationId":<id>}`）。judge `Claim` 仅认领 `pending` 并置 `running`；先编译生成器（python3 `py_compile`）、再编译标程（按 language），随后对 `i=1..count` 用 seed 运行生成器得输入、运行标程得输出；任一失败（编译错误/超时/OOM/非 0/输出截断/空输出）→ `failed` + 中文/编译错误摘要（≤8 KiB），并清理半套 cases/文件。
- **文件 key**：`problems/<problemID>/generated/<generationID>/<n>.in|.out`（n 从 1 开始，1-based），写入共享 `KAPOJ_STORAGE_DIR`；确认后复用相同 key 写入 `problem_testcases`，文件继续被测试点引用。
- **预览/确认/丢弃**：`GET .../cases/:caseId/preview?kind=input|output` 最多返回 64 KiB（非法 UTF-8 用替换字符、超长标记 `truncated`）；`POST .../confirm` 仅 `ready`，事务内以 `COALESCE(MAX(sort_order),0)+i` 追加插入 `problem_testcases`（score=10、`is_sample=false`、sort_order 接在已有测试点后），重复确认幂等；`DELETE ...` 丢弃 `ready/failed`（删除 cases 与确定性 `n.in/n.out` 文件，置 `discarded`），`confirmed` 不可丢弃。
- **沙盒限制**：生成器/标程只在 judge Docker 沙盒执行（`network none`、read-only、`cap-drop ALL`、CPU/内存/pids 限制）；生成器 5s/256MB/16 MiB 输出，标程使用题目 time/memory 限制与 16 MiB 输出上限。服务器进程绝不 `os/exec`、绝不执行用户/AI 代码。
- **AI 错误码**：平台密钥缺失 40002、用户未配置 AI 模型 40001、限流 42903、上游失败 50200、Redis 不可用 50300。
- **权限复用**：创建/查看/预览/确认/丢弃均复用 `problem.testcase.manage`，不新增权限码；AI 管理开关仅关闭 `/api/ai/*`，不影响本题管理接口。

**关键接口**

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/problems/:id/testcase-generations` | `problem.testcase.manage` | 创建生成任务（`language`/`sourceCode`/`testcaseCount`，仅传统题） |
| GET | `/api/problems/:id/testcase-generations/:genId` | 同上 | 状态/cases 元数据 |
| GET | `.../cases/:caseId/preview?kind=input\|output` | 同上 | 截断预览 |
| POST | `.../confirm` | 同上 | 确认入库 |
| DELETE | `/api/problems/:id/testcase-generations/:genId` | 同上 | 丢弃/清理 |

**前端（M5-4 ✅）**：新页面 `/problems/:id/testcase-generator`（路由 `problem-testcase-generator`，PrimeVue UI-39）；`ProblemEditView` 测试点区域（`TestcaseManager`，已随 UI-19 迁移为 PrimeVue）增加「AI 造数据」入口。表单：语言 Select（c/cpp/python3/java/go）、标程 CodeEditor（Monaco）、生成组数 InputNumber 1–10；生成中轮询（2s，最长 10 分钟）并展示 AI 脚本预览；完成后 cases 表格 + 输入/输出预览 Dialog + 确认/丢弃；失败显示错误并可返回修改标程重新生成；AI 未配置/密钥缺失提示 40001/40002 与 `/settings/ai` 链接。

### 6.5 AI 辅助前端（M5-5 ✅）

**实现状态（M5-5 ✅）**

- **提交页 AI 工具栏**（`SubmitView.vue`）：在语言 Select 与提交按钮之间新增「AI 补全 / AI 格式化 / AI 优化」三个小尺寸按钮。页面加载时调用 `GET /api/ai/configs`：未配置（`configured=false`）时显示「尚未配置 AI 模型」提示与 `/settings/ai` 链接并禁用按钮；路由 404（未启用）时显示「AI 功能未启用」；其他加载失败静默禁用按钮（不阻塞提交）。三个按钮互斥禁用；补全按当前光标 UTF-8 字节偏移调用 `POST /api/ai/complete` 并把返回片段插入光标处（空片段提示「没有可插入的补全」）；格式化返回代码与当前不同才弹预览 Dialog（「应用格式化/取消」），相同提示「代码已是格式化状态」；优化 Dialog 展示 Markdown 建议与优化代码（「应用优化代码」）；只有确认应用才替换当前语言草稿。代码按 UTF-8 字节 ≤64 KiB 前置拦截（超限提示「代码超过 64 KiB，无法使用 AI」，不发请求）。
- **题目详情 AI 提示**（`ProblemDetailView.vue`）：提示卡片标题右侧新增「AI 提示」按钮；游客跳登录（redirect 当前路径），登录调用 `POST /api/ai/hint`；成功在官方提示下方新增「AI 提示」区块（MarkdownView，缓存命中显示「来自缓存」标签）；40001 且无缓存显示「尚未配置 AI 模型」+ 去配置链接，42903 Toast「操作过于频繁，请稍后再试」，50200/50300 Toast「AI 服务暂时不可用」，404 Toast「题目不存在」；不自动请求，切换题目/重新加载清空本地 AI 状态。
- **AI 配置页 `/settings/ai`**（`AiSettingsView.vue`，路由 `ai-settings`、`requiresAuth`）：加载 `GET /api/ai/configs`；404 显示「AI 功能未启用，请联系管理员」并禁用表单；成功展示 provider Select（`providers`，切换时按 provider 默认模型填充）、模型 InputText（占位默认模型）、API Key `Password`（已配置时留空保留现有密钥、显示脱敏 key）；首次配置 key 必填，保存调用 `POST /api/ai/configs`，成功 Toast 并刷新视图；provider 必选、首次 key 非空校验中文。说明文案：密钥仅加密存储、不会回显明文，AI 调用使用用户自己的 key。移动端表单单列。
- **用户菜单**：`UserMenu.vue` 登录用户新增「AI 设置」菜单项（`pi pi-sparkles`）跳转 `/settings/ai`。
- **API 客户端**：`apps/web/src/api/ai.ts` 提供 `getAIConfig`/`saveAIConfig`/`completeCode`/`formatCode`/`optimizeCode`/`getAIHint`；类型别名在 `api/types.ts`（复用 `schema.d.ts` 的 AI schema，不新增 OpenAPI 改动）。
- **CodeEditor**：`CodeEditor.vue` 暴露 `getCursorByteOffset()`（Monaco UTF-16 offset → UTF-8 字节）与 `insertAtCursor(text)`。
- **错误映射**：40001「尚未配置 AI 模型」、42903「操作过于频繁，请稍后再试」、50200/50300「AI 服务暂时不可用」、404「AI 功能未启用/题目不存在」、代码 >64KiB「代码超过 64 KiB，无法使用 AI」、空补全「没有可插入的补全」、格式化无变化「代码已是格式化状态」。

### 6.6 AI 管理开关（`/admin/ai`）

管理员可在管理后台 `/admin/ai`（路由 `admin-ai`，`requiresAuth` + `ai.settings.manage` 权限，super 绕过）查看并切换 AI 功能开关：

- **设置 API**：`GET/PUT /api/admin/ai/settings`（需 `ai.settings.manage`）返回/更新 `{enabled, keyConfigured, updatedAt}`。设置存单例 `ai_settings`（`id=1`，首次读取懒创建、默认 `enabled=true`，`ON CONFLICT DO NOTHING`），更新与审计在同一事务内（action=`ai.settings.manage`、target=`ai_settings`、summary「启用/关闭 AI 功能」），审计失败回滚。
- **关闭语义**：关闭后所有 `/api/ai/*` 接口立即返回 404（与未配置密钥一致，不暴露「AI 已关闭」），用户侧 AI 设置/提交页工具栏/题目提示自动降级为「AI 功能未启用」；重新开启立即恢复，无需重启。关闭**不删除**用户的 `user_ai_configs`/`ai_hints`，重新开启后用户配置与提示缓存仍在。
- **密钥缺失语义**：`keyConfigured` 为只读，动态取自 `KeyManager.Configured()`（env 或密钥文件，见 §6.7）；未配置时管理页显示「未配置 AI 加密密钥，无法启用」，开关禁用，启用请求返回 40001，关闭仍可保存（此时 AI 路由仍挂载，`GET /api/ai/configs` 返回 `keyConfigured=false`、保存/调用返回 40001）。
- **管理页**：`AdminAiView` 展示当前状态 Tag（已启用/已关闭/不可用）、`ToggleSwitch`、密钥缺失 `Message` 警告、保存按钮与 Toast；下方「加密密钥」板块展示密钥状态 Tag（未配置/已配置（文件）/由环境变量管理）、脱敏值与指纹，提供自动生成（一次性显示完整密钥 + 复制 + 备份警告）、手动填写、重新生成/手动替换（二次确认），环境变量来源只读（见 §6.7）。仅 `ai.settings.manage` 可见保存按钮，`AdminNav`「AI 设置」入口按权限显隐。管理 API 不受 AI 开关影响，始终可访问（有权限时）。
- **权限**：`ai.settings.manage` 默认只授予 `super_admin`，不自动授予 `content_admin`；需要时用 RBAC-B 给自定义角色分配该权限。普通用户/无权限账号访问管理接口返回 403，不影响其正常 AI 使用。

**Codex 提示词（历史参考，已实现）**

```text
你是一名前端工程师，请为 kapOJ 实现「AI 辅助」模块的前端。

技术栈：Vue3 + TypeScript + Pinia + Element Plus + Monaco Editor。

请完成：
1. 代码编辑器集成 Monaco Editor，支持 C/C++/Python/Java/Go 语法高亮。
2. 编辑器工具栏按钮：
   - 「自动补全」：触发 /ai/complete，在光标处插入补全
   - 「格式化」：触发 /ai/format，替换编辑器内容
   - 「优化」：触发 /ai/optimize，弹窗展示优化建议与优化后代码，可一键应用
3. 题目右侧「AI 提示」按钮：点击调用 /ai/hint，展示提示内容（首次生成，后续读取缓存）。
4. 用户 AI 配置页：填写 provider、api_key、model_name（下拉选择 DeepSeek/OpenAI/Qwen/GLM）。
5. 验收标准：编辑器可补全/格式化/优化，题目 AI 提示可查看，配置可保存并生效。

请输出完整代码与类型定义。
```

---

### 6.7 AI 加密密钥管理（密钥文件/生成/手动/轮换）

管理员可在 `/admin/ai` 的「加密密钥」板块查看密钥状态、自动生成或手动填写密钥并写入本地密钥文件，运行时立即生效；已有密钥轮换时自动重加密所有用户 AI Key。环境变量密钥优先且后台只读。

- **密钥文件与优先级**：`KAPOJ_AI_KEY_FILE`（可选，默认 `data/ai.key`，权限 0600，父目录不存在时 0700 创建），文件内容为单行 base64 编码的 32 字节密钥（`openssl rand -base64 32` 等价，允许末尾换行）。环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 优先级高于文件：env 非空时校验 base64/32 字节，非法则启动失败，且忽略文件（来源 `env`）；env 为空时读文件，文件不存在 → 未配置（`none`），格式非法 → 记录警告并视为未配置（后台可覆盖）。
- **KeyManager**（`ai.KeyManager`，`sync.RWMutex`）：启动时由 `ai.NewKeyManager(envKey, filePath)` 构造，提供 `Configured()/Source()（env|file|none）/Masked()（前 3…后 4）/Fingerprint()（SHA-256 hex）/Encrypt/Decrypt/Replace`。`ai.Service` 改为持有 `*KeyManager`，所有加解密走当前密钥；运行时 `Replace` 原子切换新密钥，无需重启。
- **无密钥语义**：AI 路由始终挂载；未配置密钥时 `GET /api/ai/configs` 返回 200 + `keyConfigured=false`（含启用的 provider 列表），用户保存配置、补全/格式化/优化、无缓存的提示返回 40001「AI 加密密钥未配置」，提示缓存命中仍可返回。`ai_settings.keyConfigured` 改为 `KeyManager.Configured()` 动态判断（不再用 `d.AI==nil`）。
- **管理 API**（均需 `ai.settings.manage`，复用第 36 权限，不新增）：
  - `GET /api/admin/ai/key` → `KeyView{configured,source,masked,fingerprint}`（不含完整密钥）。
  - `POST /api/admin/ai/key/generate` → 用 `crypto/rand` 生成 32 字节 base64 并保存，返回 `{...KeyView,key}`（完整密钥仅此一次，供备份）。
  - `PUT /api/admin/ai/key` body `{key}` → 校验 base64/32 字节并保存。
  - 环境变量来源时 POST/PUT 返回 40001「由环境变量管理，只读」；审计 action=`ai.settings.manage`、target=`ai_key`、summary「生成/更新 AI 加密密钥」（不含密钥/指纹）。
- **轮换与重加密**（`ai.SettingsService.GenerateKey/SetKey/rotate`）：已有密钥时重新生成/替换会在事务内 `SELECT ... FOR UPDATE` 全部 `user_ai_configs`，逐行旧密钥解密、新密钥重新加密；任一失败回滚。写密钥文件采用临时文件 + `os.Rename` 原子替换（0600），旧文件备份为 `ai.key.bak`，DB 事务成功后删除备份、失败恢复备份（文件写失败则回滚 DB）；`ai_settings.key_fingerprint` 更新为 SHA-256 指纹（仅诊断/一致性，不存密钥）。
- **保护规则**：无旧密钥但存在 `user_ai_configs` 时返回 40001「存在无法解密的用户 AI 配置，请先恢复旧密钥」，不破坏数据；无旧密钥且无配置时直接保存。管理开关关闭不删除密钥/用户配置，重新开启恢复；启用要求当前 `keyConfigured=true`。
- **用户侧状态**：`AIConfigView.keyConfigured` 区分「AI 功能未启用」（404/开关关闭）与「管理员尚未配置 AI 密钥」（keyConfigured=false）与「尚未配置 AI 模型」（用户未配置 key）；提交页工具栏、题目提示、AI 设置页据此分别降级，缓存命中的提示仍展示。

---

## 7. 题目导入功能

**功能需求**

- 支持 HydroOJ 格式的题目导入。
- HydroOJ 题目格式通常为目录结构：

```
problem/
├── problem.yaml          # 题面元信息
├── problem_zh.md         # 中文题面
├── testdata/
│   ├── 1.in / 1.out
│   └── ...
└── additional_file/
```

- 导入流程：上传压缩包 → 后端解析 → 映射为 kapOJ 题目模型 → 入库（含测试数据）。
- 导入规则：slug 已存在时报错，不覆盖已有题目；压缩包限制大小（如 200MB）并校验路径，防止路径穿越（zip slip）；导入后题目默认为下线状态，由题目管理员确认后上架。
- 字段映射以实际 HydroOJ 题包为准，实现前用 2–3 个真实题包验证。

**实现状态（M5-6 已完成）**

- 接口：`POST /api/problems/import/hydro`（multipart，字段 `file`），需登录且拥有 `problem.create`；路由 body 上限 210 MiB（200 MiB 文件 + multipart 开销）。
- 格式与映射：zip 内每个含 `problem.yaml` 的目录视为一道题；`problem.yaml` 只取 `title`（trim 后 1–100 字符）与 `tag`（字符串数组，走 `normalizeTags`）；`problem_zh.md`（缺失/为空回退 `problem.md`）整体作为 `statement_md`（≤100,000 字符，不拆分 input/output/samples）；`testdata/` 下同名 `.in`/`.out` 成对映射为测试数据（score=10，按文件名数字优先再字典序排序）；slug=`hydro-` + 文件夹名净化（小写、非 `[a-z0-9-]` 替换为 `-`、去首尾 `-`，需满足 2–63 位 slug 规则）。`gen.py`/`std.cpp`/`additional_file/` 等非测试数据忽略，不执行题包内脚本。
- 默认值：导入题目 `is_public=false`（下线）、`created_by`=上传者、`source='HydroOJ'`、`type='traditional'`、`difficulty=1`、`time_limit_ms=1000`、`memory_limit_mb=256`。
- 限制：题包文件 ≤200 MiB、解压总量 ≤1 GiB、条目 ≤5000、题目文件夹 ≤100、单题测试点 ≤500、单个测试文件 ≤16 MiB、单题测试数据 ≤200 MiB、`problem.yaml` ≤1 MiB、题面 ≤100,000 字符。防 zip slip：拒绝绝对路径、反斜杠、`.`/`..` 路径段、符号链接、加密条目、非常规文件，忽略 `__MACOSX/` 与隐藏文件；实际解压字节按读取计数校验，防止伪造 `UncompressedSize64`。整包损坏/加密/无题目文件夹/超限返回 40001，body 超 210 MiB 由中间件返回 413。
- 部分成功：题包可解析且至少发现一个题目文件夹时返回 200，`imported`/`failed` 逐题结果；slug 冲突（`题目标识已存在：hydro-xxxx`）、缺文件、非法 slug、测试数据不配对等单题失败进入 `failed`，不阻断其他题；若整包级错误（如实际解压超限）发生在已导入部分题目之后，回滚本次已导入题目及其测试文件再返回 40001，不残留部分数据。每个成功题目 best-effort 写 `problem.create` 审计。
- 作者摘要：`GET /problems/:id` 公开返回可选 `author {id, username, nickname}`（所有能看到该题的用户，不含 email）；`created_by <= 0` 或用户不存在时省略 author；仅 GET 详情填充，POST/PUT 可省略。

**Codex 提示词（题目导入）**

```text
你是一名后端工程师，请为 kapOJ 实现「HydroOJ 格式题目导入」功能。

技术栈：Go + Gin + GORM + PostgreSQL + MinIO。

请完成：
1. 接口 POST /problems/import/hydro：
   - 接收题目压缩包（zip/tar.gz）
   - 解压并解析 HydroOJ 目录结构（problem.yaml、题面 md、testdata/*.in|*.out、additional_file）
   - 将题面、时间/内存限制、标签、样例、测试数据映射到 kapOJ 的 Problem 与 ProblemTestCase
   - 写入数据库，测试数据上传到 MinIO
2. 解析规则：
   - problem.yaml 中的 title、tags、time/memory limit、difficulty 映射到对应字段
   - 题面 md（problem.md / problem_zh.md）作为 statement_md
   - testdata 目录下成对的 .in/.out 作为测试样例
3. 校验：压缩包结构非法、缺少必要文件时返回明确错误。
4. 验收标准：导入标准 HydroOJ 题目包后，题目与测试数据正确，可被评测使用。

请输出完整代码与类型定义。
```

---

## 8. Codex 开发提示词汇总

> 以下是按开发顺序整理的提示词索引，可复制对应模块提示词直接交给 Codex。

| 序号 | 模块             | 提示词位置 |
| ---- | ---------------- | ---------- |
| 1    | 项目脚手架       | 见下方 P0  |
| 2    | 用户与认证       | 4.1        |
| 3    | 代码测评沙盒     | 4.2        |
| 4    | 题库与题目       | 4.3        |
| 5    | 比赛             | 4.4        |
| 6    | 学习路线         | 4.5        |
| 7    | 测评记录         | 4.6        |
| 8    | 成就             | 5.1        |
| 9    | 任务             | 5.2        |
| 10   | 徽章             | 5.3        |
| 11   | 称号             | 5.4        |
| 12   | 排行榜 + Rating  | 5.5        |
| 13   | 证书认证         | 5.6        |
| 14   | AI 辅助（后端）  | 6.3        |
| 15   | AI 出题造数据    | 6.4        |
| 16   | AI 辅助（前端）  | 6.5        |
| 17   | HydroOJ 题目导入 | 7          |

### P0：项目初始化提示词

```text
你是一名资深全栈工程师，请为 kapOJ（一个基于 Go (Gin) + Vue3 + PostgreSQL + Redis 的 Online Judge 平台）搭建 Monorepo 项目脚手架。

要求：
1. 使用 pnpm workspace 管理 Monorepo，结构为 apps/web（Vue3 + TypeScript + Vite + Pinia + Element Plus）、apps/server（Go + Gin + GORM）、packages/api（OpenAPI 规范，供前端生成类型）、docker。
2. 配置 PostgreSQL 与 Redis 的 docker-compose。
3. 后端：初始化 Go + Gin 项目、GORM 接入 PostgreSQL、全局配置/环境变量（Viper）、统一的错误处理与响应封装、JWT 鉴权中间件、健康检查接口 /health。
4. 前端：初始化 Vue3 + Vite 项目，接入 Element Plus、Pinia、Vue Router、Axios 封装、基础布局（顶部导航 + 侧边栏）。
5. 前端提供 eslint + prettier + tsconfig 配置，后端提供 golangci-lint + gofmt 配置，以及 README 启动说明。
6. 验收标准：docker compose 启动后，后端 /health 返回 ok，前端可访问并显示基础布局。

请输出完整项目脚手架代码。
```

---

## 9. 开发里程碑建议

| 里程碑 | 内容                                   | 交付物             | 状态 |
| ------ | -------------------------------------- | ------------------ | ---- |
| M0     | 项目初始化、认证、基础布局             | 可登录的前后端骨架 | ✅   |
| M1     | 题库/题目 + 沙盒评测 + 测评记录        | 可提交并判题的 MVP | ✅   |
| M2     | 比赛系统（三种赛制 + 榜单 + 公告）     | 可办比赛的平台     | ✅   |
| M3     | 游戏化模块（成就/任务/徽章/称号/金币） | 有趣味的成长体系   | ✅   |
| M4     | 排行榜 + Rating + 学习路线 + 社交互动  | 社区与成长闭环     | ✅   |
| M5     | AI 辅助 + 题目导入 + 证书认证          | 完整版 kapOJ       | 🚧   |

状态图例：✅ 已完成　🚧 进行中　⬜ 未开始。只有模块的验收标准全部满足后才标记 ✅。

M5 进度：M5-A（AI 后端基础：多 provider 配置、用户 API key 加密存储、补全/格式化/优化与题目提示缓存/限流）、M5-4（AI 出题造数据：标程 → AI 纯 Python 3 造数据脚本 → judge 沙盒生成输入/输出 → 预览确认入库，见 §6.4）、M5-5（AI 前端：提交页 AI 工具栏、题目详情 AI 提示、AI 配置页，见 §6.5）、M5-6（HydroOJ 题包导入 + 题目详情公开创建者摘要，见 §7）与 M5-7（证书认证：证书类型、用户提交/我的证书、鉴权图片、管理员审核、公开验证码核验，见 §3.7/§5.6）已完成；M5-8（头像上传）仍未完成，因此 M5 整体保持 🚧。

M3 进度：M3-A（金币、事件、成就、任务后端）、M3-B（徽章、称号、公开身份后端）与 M3-C（成就页、任务大厅、徽章商城、称号商店、装扮管理、游戏化管理、公开身份接入与真实浏览器验收）均已完成，M3 标记为 ✅。

M4 进度：M4-A（Rating 结算、8 级等级、Rating 历史与 Rating 榜后端）、M4-B（活跃度、签到金币、共享分、活跃度/共享榜与三榜缓存）、M4-C（排行榜页、每日签到入口、用户主页 Rating 历史卡片、导航与首页入口）、M4-2A（学习路线模型、权限、路线/节点 CRUD 与树结构）、M4-2B（服务端进度计算、AC 事件补算、完成锁存与题目下线守卫）、M4-2C（学习路线前端列表/树/编辑/官方管理页）、M4-3（轻量社交互动：题解点赞、关注、通知，接入活跃度与共享分，见 4.8）与 M4-4（Rating 衰减与赛季重置：`rating.settings.manage` 权限、管理设置 API 与 `/admin/rating` 设置页、月度高分段衰减、赛季软重置、`decay`/`season_reset` 历史与公开赛季接口）均已完成，M4 整体标记为 ✅。

平台管理后台/RBAC 进度：RBAC-A（权限模型、权限目录、首次 Seed/旧角色映射、实时 Access、权限中间件、`/users/me` 权限字段）与 RBAC-B（角色 CRUD/权限矩阵、用户角色分配、封禁/解封、审计日志、仪表盘统计 API）已完成；RBAC-C（现有管理接口迁移）与 RBAC-D（管理后台前端与 E2E）尚未开始。设计与拆分见 [`docs/superpowers/specs/2026-11-06-admin-rbac-design.md`](docs/superpowers/specs/2026-11-06-admin-rbac-design.md)。

---

## 附录：非功能性需求

- **性能**：榜单查询 P95 < 300ms（Redis 缓存）；评测任务吞吐量满足并发提交。
- **安全**：沙盒强隔离、API key 加密存储、防 SQL 注入/XSS/CSRF、接口限流。
- **可观测性**：结构化日志、监控指标、评测任务失败告警。
- **可扩展性**：评测服务可水平扩展；模块间解耦（事件驱动）。
- **接口限流**：登录、提交、AI 接口按用户限流；提交接口默认每用户每 3 秒最多 1 次（`KAPOJ_SUBMIT_COOLDOWN` 可配置）。
- **敏感信息**：日志不记录密码、API key 和 refresh token；测评输出与用户内容做 XSS 转义后再展示。
- **数据备份**：PostgreSQL 每日全量备份，保留 7 天；MinIO 中的题目数据与证书图片同步备份。
