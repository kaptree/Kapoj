# kapOJ 开发文档

> 本文档基于 `plan.md` 细化，是 kapOJ（Kap Online Judge）平台的详细开发规格说明书。
> 目标：**打造出最有趣味的 Online Judge 平台**。
> 文档包含完整的架构设计、数据库设计、模块需求，并为每个模块提供了可直接交给 Codex 的**开发提示词（Prompt）**。
> 开发进度见 [`docs/任务清单.md`](docs/任务清单.md)，每次变更记录见 [`docs/开发日志.md`](docs/开发日志.md)。当前进度：**M1 已完成**。

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

---

## 2. 技术栈与总体架构

### 2.1 推荐技术栈

| 层次     | 技术选型                                                   | 说明                                                                             |
| -------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------- |
| 前端     | Vue 3 + TypeScript + Vite + Pinia + Element Plus + ECharts | SPA，响应式，榜单/统计用 ECharts                                                 |
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

**当前状态（M1-C）**：已建立 `apps/web`、`apps/server`、`apps/judge`、`packages/api`、`docker`、`docs`。后端按领域分包，位于 `apps/server/internal/`：`config`、`httpx`（统一响应与错误码）、`health`、`db`、`dbtypes`（jsonb 列表类型）、`storage`（文件存储抽象与本地实现）、`user`（模型、仓储、校验、资料服务）、`auth`（令牌与认证服务）、`middleware`、`userapi`（用户 HTTP 接口）、`problem`（题目模型、仓储、校验、服务）、`problemapi`（题目 HTTP 接口）、`submission`（提交模型、仓储、服务、限流）、`submissionapi`（提交 HTTP 接口）、`submissionevents`（Redis→SSE）、`judgequeue`（Asynq 队列契约）、`router`。`apps/judge` 是独立 Go 模块与进程，包含 `config`、`db`、`model`、`queue`、`events`、`reconcile`、`storage`、`verdict`、`sandbox`、`pipeline`、`worker`。本地开发时，docker compose 中的 PostgreSQL 映射到宿主机 5433 端口，Redis 映射到 6380 端口，启动方式见 `README.md`。

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
  role,            -- user / problem_admin / contest_admin / super_admin
  rating,          -- 当前比赛 Rating（由 rating_history 结算得出，此处为缓存）
  coins,           -- 金币余额，不允许为负（CHECK coins >= 0）
  bio, status,     -- active / banned
  created_at, updated_at
)
-- 唯一约束：username、email

-- 实现差异（M0）：username 与 email 统一存小写，因此唯一性不区分大小写；
-- 唯一索引名为 uniq_users_username、uniq_users_email，CHECK 约束名为 chk_users_coins_non_negative；
-- 新用户 rating 默认 1500。rating_history 表在 M4 创建（未实现）。

rating_history (
  id, user_id, contest_id, rank, rating_before, rating_after, created_at
)
-- 唯一约束：(user_id, contest_id)，保证同一场比赛的 Rating 只结算一次
```

### 3.2 题库与题目

```sql
problems (
  id, title, slug, statement_md,        -- 题目描述 Markdown
  input_format_md, output_format_md,    -- 输入输出格式
  samples_json,                          -- 结构化样例（用于展示与导入校验），须与 statement_md 中的样例一致
  hint_md, solution_md, solution_code,   -- 提示/思路/解题代码
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

### 3.6 排行榜与交互

```sql
-- 活跃度
activity_logs (
  id, user_id, action, points, ref_type, ref_id, created_at
)
-- 每类行为的每日积分上限与去重规则见 5.5
-- 共享内容（用户创建比赛/题解）
shared_items (
  id, user_id, type,              -- contest / solution
  ref_id, like_count, created_at
)
comments ( id, user_id, target_type, target_id, content, created_at )
likes    ( id, user_id, target_type, target_id, created_at )
follows  ( id, follower_id, followee_id, created_at )
```

### 3.7 证书认证

```sql
certificates (
  id, user_id, contest_name,      -- 蓝桥杯 / ACM-ICPC / CCPC ...
  level, certificate_image_url,   -- 证书图片
  status,                          -- pending / approved / rejected
  reject_reason, verify_code,            -- verify_code：通过后生成，全局唯一，用于公开核验
  submitted_at, reviewed_at, reviewed_by
)
certificate_types (
  id, name, description, is_active, sort_order
)
```

### 3.8 学习路线

```sql
learning_paths (
  id, title, description_md, cover_url, is_official, sort_order
)
learning_path_nodes (
  id, path_id, parent_id, title, description_md, problem_ids_json, sort_order,
  required_count                         -- 完成本节点所需通过题数（默认为节点内题目总数）
)
user_learning_progress (
  id, user_id, path_id, node_id, status, solved_count, completed_at
)
-- status: locked / active / completed；唯一约束：(user_id, node_id)；进度由服务端根据 AC 事件计算
```

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

---

## 4. 基础功能模块

### 4.1 用户与认证

**功能需求**

- 注册、登录（用户名/邮箱 + 密码）、JWT 鉴权、刷新令牌。refresh token 保存在 Redis 中，登出或修改密码后立即吊销。
- 登录失败次数限制（如 5 分钟内最多 10 次），防止暴力破解。
- 个人中心：查看/编辑资料、头像上传、我的成就/徽章/称号/金币、装扮管理。
- 角色与权限（RBAC）：游客、注册用户、题目管理员、比赛管理员、超级管理员。

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
| GET  | /admin/users       | 用户分页列表（仅超级管理员）    |

**实现状态（M0 ✅）**：上表接口均已实现，完整定义见 `packages/api/openapi.yaml`。

**实现差异**：

- 注册成功后直接返回 `{ accessToken, refreshToken, user }`，即自动登录，与登录响应相同。
- 登录请求字段为 `account`（用户名或邮箱）和 `password`。
- 校验规则：用户名 3–32 位字母、数字或下划线；邮箱不超过 254 字节；密码 8–72 字节（72 是 bcrypt 的上限）；昵称 1–32 个字符，为空时默认等于用户名；简介不超过 200 个字符；头像链接为 http(s) 且不超过 512 字符。
- access token 为 HS256 JWT，有效期 15 分钟；refresh token 为随机串，有效期 7 天，每次刷新都会轮换，旧令牌立即失效。Redis 中只保存其 SHA-256。
- 登录尝试限制：每个用户 5 分钟内最多 10 次。每次尝试在校验密码前原子计数，并发请求无法绕过；用户名和邮箱登录共用同一个计数。超过后即使密码正确也返回 42901，登录成功后清零。账号不存在与密码错误返回同一个错误。
- `/auth/*` 接口不校验 Authorization 头，由请求体中的 refresh token 识别用户，因此 access token 过期后仍可登出和刷新。
- 前端在多个标签页之间同步登录状态；某个标签页轮换令牌后，其他标签页会改用新令牌，而不是被登出。
- 修改资料只写入被修改的字段，不会覆盖同时发生的封禁、角色或金币变动。
- 被封禁用户无法登录或刷新令牌；已签发的 access token 在过期前（最长 15 分钟）仍然有效。
- `GET /users/:id/profile` 不返回邮箱和角色；`equippedBadges`、`equippedTitle` 在 M3 前固定为空数组和 null。
- `GET /admin/users` 为新增接口，用于超级管理员的用户管理，也用于验收「管理员接口对普通用户返回 403」。
- 头像上传依赖 MinIO，**未实现**，安排在 M5-8；目前只能填写头像链接。修改密码接口**未实现**。

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

**实现状态（M1-A）**

- 后端题目 CRUD、测试数据管理、checker 上传与前端题目列表、详情、编辑页、测评记录页、Monaco 提交页均已完成；提交、评测与 SSE 实时刷新已在 M1-B/M1-C 交付。
- 权限：写接口需要 `problem_admin` 或 `super_admin`（`super_admin` 总是放行）；`contest_admin` 不能管理题目。未公开题目对非管理员返回 40400，与不存在一致。
- 创建时忽略 `isPublic`，新题一律未公开；发布需至少 1 个测试点，特判题还需已上传 checker；公开题目不能删除最后一个测试点。发布校验与删除保护都在题目行锁事务内完成，并发删除测试点不会让公开题目变成 0 个测试点。
- **更新是整体替换**：`PUT /problems/:id` 必须提交全部可编辑字段；未提交的字符串/数组会清空，`isPublic` 缺省为 `false`（即下线）。前端编辑页会先加载完整题目再提交。题目创建/更新的 JSON 请求体上限为 4 MiB。
- 难度为整数 1–5（入门、普及、提高、省选、NOI）。
- 列表筛选：`keyword` 同时匹配标题与 slug（不区分大小写、按字面量匹配，`%`/`_` 不作通配符）；`tag` 精确匹配整个标签；`difficulty` 精确匹配；`includeHidden` 仅对题目管理员有效。
- 题解可见性：`solutionMd`、`solutionCode`、`checkerUploaded`、`createdBy` 返回给题目管理员，或本人已在该题有 `accepted` 提交的普通用户；`hintMd` 对所有人生效。
- 交互题允许创建与存储，M1 不参与评测；提交时的拒绝逻辑在 M1-B。
- 前端使用 md-editor-v3 编辑与渲染 Markdown，渲染结果统一经 `sanitize-html` 净化；KaTeX 与 highlight.js 使用本地实例，不加载 CDN 资源。
- 关联比赛：M2 之前详情页显示「比赛功能将在 M2 上线」。

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

- 学习路线 CRUD（官方可创建，后续可扩展用户自定义）。
- 路线内包含按算法组织的题目，呈树状结构（算法模块 → 阶段 → 题目）。
- 用户按阶段挑战，完成部分算法后可解锁下一个算法模块。
- 点亮分支结构，点进每个算法模块可查看题目清单。

**关键设计**

- 节点模型：`learning_path_nodes` 支持 `parent_id` 自关联，形成树。
- 解锁规则：节点只有在父节点完成后才可挑战；节点完成 = 通过其 `required_count` 道题（默认为节点内全部题目）。根节点默认解锁。
- 前端用 Element Plus Tree 或自定义树形组件可视化。结构是树而非 DAG，每个节点只有一个父节点。

**Codex 提示词（学习路线）**

```text
你是一名全栈工程师，请为 kapOJ 实现「学习路线」模块。

技术栈：Go + Gin + GORM + PostgreSQL 后端，Vue3 + Element Plus（Tree/自定义树形可视化）前端。

请完成：
1. 数据模型：LearningPath、LearningPathNode（含 parent_id 自关联）、UserLearningProgress。
2. 后端接口：
   - 学习路线 CRUD：GET/POST/PUT/DELETE /learning-paths（题目管理员及以上）
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
- 隐藏题目：公开列表只显示 `problems.is_public=true` 的提交（登录用户额外可看到自己的提交）；详情仅本人、题目管理员或超级管理员可见，其他用户返回 40400；`contest_admin` 不因比赛管理权限看到隐藏题。
- 昵称为不区分大小写的模糊匹配，按字面量转义 `%`/`_`；非法 status/language/负 id 返回 40001。
- Web 在创建提交（pending）与入队失败（system_error）、judge 在领取任务（judging）与写入终态后，都会向 Redis 频道 `kapoj:submissions` 发布 submission id；Web 端 SSE 订阅该频道，连接建立时立即发送注释行、每 15 秒 keep-alive，浏览器断线由 EventSource 自动重连。前端在 `onopen` 补拉一次并每 30 秒兜底刷新，避免 Redis/网络短暂中断造成列表长期停留在旧状态。
- 前端：`/submissions` 记录页（筛选、分页、状态彩色标签、详情抽屉、SSE 自动刷新）；`/problems/:id/submit` 提交页（Monaco、语言切换、代码改动 500ms 防抖写入本地草稿，存储异常不影响提交）；题目详情页在本人 AC 后显示「解题思路」「解题代码」。
- Monaco 使用本地打包的 editor worker，不访问 CDN；提交页路由懒加载编辑器，避免影响首屏体积。

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
- 等级阈值（示例，可配置）：

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

**共享积分（示例）**

| 行为             | 积分 |
| ---------------- | ---- |
| 创建比赛         | 20   |
| 提供的题解被点赞 | 5/赞 |

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

---

## 6. AI 辅助功能模块

### 6.1 功能清单

1. **代码自动补全**：编辑器内基于上下文补全代码。
2. **代码自动格式化**：一键格式化代码。
3. **代码自动优化**：AI 分析并给出优化建议/优化后代码。
4. **出题造数据**：创建题目时读取标程，用 `cyaron` 自动编写造数据脚本，生成 input/output 样例并上传到题目测试数据。
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

### 6.4 Codex 提示词（AI 辅助 - 出题造数据）

```text
你是一名 Python + 出题系统专家，请为 kapOJ 实现「AI 出题造数据」功能。

背景：创建题目时，管理员上传/粘贴标程（标准答案代码），系统读取标程后使用 Python 的 cyaron 库自动生成造数据脚本，运行脚本生成多组 input/output 测试样例，并上传到题目的测试数据中。

请完成：
1. 后端接口 POST /problems/:id/generate-testcases：
   - 接收标程（source_code、language）
   - 调用大模型生成 cyaron 造数据脚本（Python）
   - 在隔离环境中运行脚本生成 input 文件，再用标程运行 input 得到 output 文件（output 不由模型直接生成，避免错误数据）
   - 将生成的文件写入 problem_testcases（标记 is_sample 与 score）
2. AI 提示词设计：让模型根据标程的输入约束（数据范围、格式）生成合适的随机数据生成脚本，覆盖边界与随机情况。
3. 安全：生成脚本在 Docker 沙盒中运行，限制时间与资源。生成的测试数据须由管理员预览并确认后才入库。
4. 验收标准：给定标程，可自动生成多组合法测试数据并入库，数据能在评测中被使用。

请输出完整代码、AI 提示词模板与类型定义。
```

### 6.5 Codex 提示词（AI 辅助 - 前端）

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
| M2     | 比赛系统（三种赛制 + 榜单 + 公告）     | 可办比赛的平台     | ⬜   |
| M3     | 游戏化模块（成就/任务/徽章/称号/金币） | 有趣味的成长体系   | ⬜   |
| M4     | 排行榜 + Rating + 学习路线             | 社区与成长闭环     | ⬜   |
| M5     | AI 辅助 + 题目导入 + 证书认证          | 完整版 kapOJ       | ⬜   |

状态图例：✅ 已完成　🚧 进行中　⬜ 未开始。只有模块的验收标准全部满足后才标记 ✅。

---

## 附录：非功能性需求

- **性能**：榜单查询 P95 < 300ms（Redis 缓存）；评测任务吞吐量满足并发提交。
- **安全**：沙盒强隔离、API key 加密存储、防 SQL 注入/XSS/CSRF、接口限流。
- **可观测性**：结构化日志、监控指标、评测任务失败告警。
- **可扩展性**：评测服务可水平扩展；模块间解耦（事件驱动）。
- **接口限流**：登录、提交、AI 接口按用户限流；提交接口默认每用户每 3 秒最多 1 次（`KAPOJ_SUBMIT_COOLDOWN` 可配置）。
- **敏感信息**：日志不记录密码、API key 和 refresh token；测评输出与用户内容做 XSS 转义后再展示。
- **数据备份**：PostgreSQL 每日全量备份，保留 7 天；MinIO 中的题目数据与证书图片同步备份。
