# kapOJ

打造最有趣味的 Online Judge 平台。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md)，进度见 [`docs/任务清单.md`](docs/任务清单.md) 与 [`docs/开发日志.md`](docs/开发日志.md)。

当前进度：**M4-2A、M4-2B、M4-2C、M4-3、M4-4、RBAC-D、M5-A、M5-4、M5-5、M5-6、M5-7、M5-8、M6 与 M7 已完成**（M0–M3 已完成；M4 已完成；M5 已完成。M4-A 交付 Rated 比赛 Rating 结算、8 级等级、Rating 历史与 Rating 榜后端；M4-B 交付活跃度、每日签到金币、共享分与活跃度/共享榜后端；M4-C 交付排行榜页（Rating/活跃度/共享三 Tab、等级颜色、签到）、用户主页 Rating 历史卡片、导航与首页入口；M4-2A 交付学习路线后端模型、官方/自定义权限、路线/节点 CRUD 与树结构，路线 API 已可用；M4-2B 交付服务端进度计算（AC 事件补算 + 读取补算、完成锁存）与题目下线/删除守卫；M4-2C 交付学习路线前端列表/树详情/编辑/官方管理页；M4-3 交付轻量社交互动（题解点赞、关注、站内通知，回复/点赞/关注接入活跃度与共享分，前端点赞按钮/关注 Dialog/通知铃铛/通知页，见下）；M4-4 交付 Rating 衰减与赛季重置（`rating.settings.manage` 权限与 Rating 分组、管理设置 API、`/admin/rating` 设置页、月度高分段衰减、赛季软重置、`decay`/`season_reset` 历史与公开赛季接口）；RBAC-A 交付权限模型、权限目录、首次 Seed/旧角色映射、实时 Access、权限中间件与 `/users/me` 权限字段；RBAC-B 交付角色 CRUD/权限矩阵、用户角色分配、封禁/解封、审计日志与仪表盘统计 API；RBAC-C 交付题目/比赛/测评/游戏化/学习路线管理接口的权限码迁移、Viewer 能力拆分与内容管理审计；RBAC-D 交付 `auth.can`/路由 meta、AdminLayout 管理壳、仪表盘/用户管理/角色权限/审计日志页面与菜单/路由/按钮权限化；M5-A 交付 AI 后端基础：多 provider 配置、用户 API key 加密存储、补全/格式化/优化与题目提示缓存/限流；M5-5 交付 AI 前端：提交页 AI 补全/格式化/优化工具栏、题目详情 AI 提示、`/settings/ai` AI 配置页与用户菜单入口（见下）；另交付 AI 管理开关：`ai.settings.manage` 权限（第 36 个，分组「AI」）、`GET/PUT /api/admin/ai/settings` 与 `/admin/ai` 开关页，关闭后 `/api/ai/*` 统一 404（见下）；M5-6 交付 HydroOJ 题包导入（`POST /api/problems/import/hydro`、大小限制/防路径穿越、slug 冲突不覆盖、默认下线、部分成功与回滚）与题目详情公开创建者摘要（`author{id,username,nickname}`），前端新增 `/problems/import` 导入页与详情创建者链接（见下）；M5-4 交付 AI 出题造数据（标程 → AI 纯 Python 3 造数据脚本 → judge 沙盒生成输入/输出 → 预览确认入库，见下）；M5-7 交付证书认证（证书类型、用户提交/我的证书、鉴权图片、管理员审核、公开验证码核验，见下）；M5-8 交付头像上传（本地存储上传/移除、公开读取、上传优先与外链替换语义，见下）；另交付账号安全（修改密码 + 活跃会话：会话列表/退出其他/退出所有、sessionId/Redis 会话结构、改密限流 42904、`/settings/security` 账号安全页，见下）；M6 交付站点管理（后台配置品牌 Logo/favicon、首页欢迎区/快速开始/成长中心、页脚备案与运行/访问量，公开 `GET /api/site` 与访问上报，第 38 个权限码 `site.settings.manage` 与审计，前端 `site` store、公开页脚、数据化首页与 `/admin/site` 三 Tab 管理页）；M7 交付客观题（答案题：后台配置 1–20 个标准答案、无测试点发布/提交/评测、judge 跳过沙盒并按去首尾空白 + 忽略大小写判定、AC 固定 100 分；标准答案仅题目编辑者可见，提交答案仅本人或实时比赛管理员可见；前端题目编辑/详情/比赛列表/提交页/测评记录按题型切换））。前端并行进行 PrimeVue 4 迁移，**UI-00～UI-20（含 UI-19 题库编辑与 UI-20 异常页）、UI-30（M4-C 排行榜/历史）、UI-31（M4-7 学习路线）、UI-32（M4-3 社交互动）、UI-33（M5-7 证书认证）、UI-34（M5-5 AI 面板）、UI-35（M5-6 题目导入）、UI-39（M5-4 AI 造数据）与 RBAC-D 管理后台页面已完成并提交**，Element Plus 依赖与过渡样式已彻底移除，详见 [`docs/primevue-migration.md`](docs/primevue-migration.md)。题目详情资源区、官方题解与解题思路论坛已完成（见下）。

## 目录结构

```
apps/server     Go 后端（Gin + GORM）
apps/judge      独立评测进程（Asynq + Docker 沙盒）
apps/web        Vue 3 前端（PrimeVue 4 + md-editor-v3 + Monaco）
packages/api    OpenAPI 规范，前端类型由此生成
docker          docker compose（PostgreSQL 17、Redis 7、后端/评测镜像）
docs            任务清单、开发日志、PrimeVue 迁移进度、实施计划
```

## 环境要求

- Go 1.26（`go.mod` 要求；Go 工具链会自动下载）
- Node.js 22，pnpm 通过 `corepack pnpm` 调用，无需全局安装
- Docker（运行 PostgreSQL 与 Redis）

## 本地启动

1. 启动数据库与缓存：

   ```bash
   docker compose -f docker/docker-compose.yml up -d
   ```

   PostgreSQL 映射到宿主机 **5433** 端口，Redis 映射到 **6380** 端口，避免与本机已有服务冲突。`kapoj_test` 测试库会自动创建。

2. 配置后端环境变量（参考 `.env.example`）：

   ```bash
   export KAPOJ_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj?sslmode=disable'
   export KAPOJ_REDIS_ADDR=localhost:6380
   export KAPOJ_JWT_SECRET='替换为至少 32 字节的随机字符串'
   export KAPOJ_STORAGE_DIR="$(pwd)/apps/server/data"   # 必须使用绝对路径；judge 也从这里读
   ```

3. 启动后端（启动时自动执行数据库迁移）：

   ```bash
   cd apps/server && go run ./cmd/server
   ```

   访问 `http://localhost:8080/health`，返回 `"status":"ok"` 即正常。后端同时运行游戏化事件 worker（Asynq 队列 `game`，并发可用 `KAPOJ_GAME_CONCURRENCY` 调整，1–16，默认 2）与 Rating 结算扫描器（默认每 60 秒一次，可用 `KAPOJ_RATING_SCAN_INTERVAL` 调整，10s–10m）。比赛结束后的评测等待时间用 `KAPOJ_RATING_JUDGE_GRACE` 配置（1m–60m，默认 10m）。

4. 启动评测服务（需要 Docker）：

   ```bash
   docker pull gcc:14
   docker pull python:3.12-slim
   docker pull eclipse-temurin:21-jdk
   docker pull golang:1.26

   # 与后端使用相同的数据库、Redis 和 KAPOJ_STORAGE_DIR
   cd apps/judge && go run ./cmd/judge
   ```

   评测服务消费 Redis 中的 `judge` 队列，在独立容器里编译运行 C/C++、Python3、Java、Go 代码。

5. 启动前端：

   ```bash
   corepack pnpm install
   corepack pnpm dev:web
   ```

   打开 `http://localhost:5173`。开发服务器会把 `/api` 与 `/health` 代理到 `http://localhost:8080`（可用 `KAPOJ_BACKEND_URL` 修改）。登录后可在「测评记录」查看实时结果，在题目页点「提交代码」使用 Monaco 编辑器提交。

也可以用容器运行后端与评测服务：

```bash
KAPOJ_JWT_SECRET='至少 32 字节的随机字符串' docker compose -f docker/docker-compose.yml --profile app up -d --build
```

## 游戏化（M3）

**前端页面**

- 侧边栏「成长」子菜单：成就墙 `/achievements`、任务大厅 `/tasks`、徽章商城 `/shop/badges`、称号商店 `/shop/titles`、装扮管理 `/appearance`；`super_admin` 额外显示游戏化管理 `/admin/gamification`（成就/任务/徽章/称号四个 Tab）。
- 用户菜单显示格式化后的金币余额，并提供「装扮管理」入口；购买/装备后余额以服务端返回为准刷新。
- 个人主页、测评记录（昵称下方与详情抽屉）和比赛榜单（选手列）展示装备的称号与徽章；空装备不显示占位。
- 装扮管理展示金币余额与流水，可装备/卸下徽章（显示 n/上限）和称号。

**接口与规则**

- `GET /api/coins/transactions`：登录用户查看金币余额与流水（分页）。
- `GET /api/achievements`、`GET /api/users/me/achievements`：成就列表与我的成就；登录读取会自动补算解锁并发奖，接口仍是幂等的。
- `GET /api/tasks`、`POST /api/tasks/:id/claim`：任务大厅与领取；领取后 AC 全部题目会自动完成并发金币。
- `GET /api/badges`、`POST /api/badges/:id/purchase|equip|unequip`、`GET /api/users/:id/badges`：徽章商城、购买与装备；装备上限由 `KAPOJ_MAX_EQUIPPED_BADGES` 配置（1–10，默认 3）。
- 徽章图片：管理员通过 `POST /api/badges/:id/image` 上传 PNG/JPEG/WebP/GIF（≤ 2 MiB），文件保存为 `KAPOJ_STORAGE_DIR` 下的 `badges/<id>`，公开接口 `GET /api/badges/:id/image` 读取；数据库中 `imageUrl` 记录为 `/api/badges/<id>/image`。
- `GET /api/titles`、`POST /api/titles/:id/purchase|equip|unequip`：称号商店、解锁、购买与装备；同一用户同时只装备 1 个称号。
- 个人主页、测评记录与比赛榜单返回 `equippedBadges`、`equippedTitle` 公开身份字段。
- 拥有 `gamification.*.manage` 权限的管理员可在 `/admin/gamification` 管理成就/任务/徽章/称号并上传徽章图片；也可用 `POST /api/admin/users/:id/coins`（`user.coins.adjust`）调整金币（需填写备注，事务内写审计）。
- M3 不包含排行榜与 Rating；M4-A～M4-C 的 Rating 榜、Rating 历史、活跃度榜、共享榜与签到见下一节。

## 排行榜、Rating 与活跃度（M4-A～M4-C）

**前端页面**

- 侧边栏「排行榜」`/leaderboard` 已启用：Rating（默认）、活跃度、共享三个 Tab，每个 Tab 独立分页（每页 20 条）并保留各自页码，切换 Tab 不串数据。Rating Tab 按服务端等级显示灰/绿/青/蓝/紫/橙/红/黑红颜色，黑红名首字符黑、其余红；用户列链接个人主页并展示已装备称号/徽章。
- 活跃度 Tab 展示今日/总活跃度与「签到 +5 金币」按钮：首次签到 +5 活跃度与 +5 金币，成功后刷新用户金币、签到摘要与活跃度榜；当日已签到按钮禁用；游客显示登录入口。重复签到不会重复发币。
- 用户主页 `/users/:id` 新增「Rating 历史」卡片：最近 20 条 Rating 变化表格与简单 SVG 折线；按类型显示标签（比赛 / Rating 衰减 / 赛季重置），比赛行保留链接、私密比赛显示“私密比赛”且不生成链接，衰减/重置行不生成比赛链接；无记录显示“暂无 Rating 记录”。
- 首页新增排行榜快捷入口与 M4-C 系统状态；侧栏版本为 M4-C。

**接口与规则**

- `GET /api/leaderboard/rating?page=&page_size=`：公开 Rating 榜，只含 `status=active` 用户，按 Rating 降序、用户 ID 升序；行内 `score` 为 Rating，并返回 `level`、`equippedBadges`、`equippedTitle`。
- `GET /api/users/:id/rating-history?page=&page_size=`：公开 Rating 历史，按时间倒序返回 `kind`（`contest`/`decay`/`season_reset`）、比赛名/标签、名次、`periodKey`、赛前/赛后 Rating、`delta` 与 `createdAt`；非比赛行 `contestId=0`、`rank=0`，私密比赛仍脱敏；用户不存在返回 40400。
- Rated 比赛结束后由后台扫描器自动结算，只统计比赛窗口内至少有 1 次已判（非 `pending`/`judging`）提交的报名者；使用 PostgreSQL 全局 advisory lock 串行处理不同比赛，`rating_history` 部分唯一索引 `(user_id, contest_id) WHERE contest_id IS NOT NULL` 保证重复扫描不会重复写历史或重复更新 Rating。结算完成写入 `contests.rating_settled_at`。
- `KAPOJ_RATING_SCAN_INTERVAL`：扫描间隔，10s–10m，默认 `60s`；每轮最多处理 20 场。
- `KAPOJ_RATING_JUDGE_GRACE`：比赛结束后等待 `pending`/`judging` 提交的宽限，1m–60m，默认 `10m`；超过宽限按已判结果结算。
- 等级为灰/绿/青/蓝/紫/橙/红/黑红；黑红名 `color=#000000`、`accentColor=#FF0000`，其他等级 `accentColor` 为空字符串。
- `GET /api/leaderboard/activity?page=&page_size=`：公开活跃度榜，只含 `status=active` 且总活跃度 > 0 的用户，按总分降序、用户 ID 升序；`score` 为活跃度。
- `GET /api/leaderboard/sharing?page=&page_size=`：公开共享榜，只含 `status=active` 且共享分 > 0 的用户，按共享分降序、用户 ID 升序；`score` 为共享分。
- 活跃度规则：提交 +2（每日上限 40）、比赛报名 +10（每日上限 50）、每日签到 +5（每天一次）；同一提交/比赛/日期不重复计分，达到每日上限后不再加分，均不影响主流程。M4-3 起论坛回复 +3、点赞 +1、关注 +2，各每日上限 20 分，同一对象去重；取消赞/关注不扣分。
- 签到金币：`POST /api/activity/signin` 每个本地自然日首次签到额外 +5 金币，签到活跃度、金币流水与余额在同一事务内完成；重复签到返回 `awarded=false`，不重复发币。`GET /api/activity/summary` 返回今日是否签到、今日活跃度与总活跃度。
- 共享积分：创建或克隆比赛 +20，服务启动时会把已有 `created_by>0` 的比赛幂等回填到 `shared_items`；题解帖子每获 1 个赞，作者共享分 +5；取消赞扣回；删除帖子清理共享项（见「社交互动（M4-3）」）。
- 三个榜单的基础行都使用 Redis 版本缓存 60 秒，key 形如 `kapoj:leaderboard:<kind>:v<version>:<page>:<size>`；Rating 结算、活跃度写入、共享项写入后递增对应 `kapoj:leaderboard:<kind>:version`。缓存只含基础行，装备在读取后批量装饰；Redis/缓存故障时回退数据库，缓存故障本身不会导致 5xx（身份装饰查询失败仍可能返回 5xx）。
- 社交互动（题解点赞/关注/通知）已随 M4-3 完成，见「社交互动（M4-3）」小节。

## Rating 衰减与赛季重置（M4-4）

**前端页面**

- 管理后台新增「Rating 设置」导航项 `/admin/rating`（需 `rating.settings.manage`）：配置衰减开关/阈值/未活跃天数/月比例/单次上限、赛季开关/月数/重置比例/锚点，并只读展示当前赛季与下次重置；保存校验范围与服务端一致，错误中文。
- 排行榜页顶部展示当前赛季名称与下次重置时间（来自公开赛季接口）；请求失败静默降级，不影响榜单。
- 用户主页 Rating 历史按 `kind` 显示「比赛 / Rating 衰减 / 赛季重置」标签。

**接口与规则**

- `GET /api/rating/season/current`：公开返回当前赛季 `{key,name,startsAt,endsAt,nextResetAt}`。
- `GET/PUT /api/admin/rating/settings`：需 `rating.settings.manage`（super 绕过）；GET 返回设置 + 当前赛季 + 下次重置，PUT 整体更新并写审计（action=`rating.settings.manage`、target=`rating_settings`）。
- 衰减默认：阈值 2400、未活跃 60 天、月比例 3%、单次上限 100；仅 `status=active` 且 `rating>阈值`、最近一次 Rated 比赛距今 ≥ 未活跃天数的用户按月扣分，公式 `decay=min(上限, max(1, round((rating-阈值)*比例/100)))`，`new=max(阈值, rating-decay)`；每月一次（运行键 `decay:YYYY-MM`，UTC），跨月停机只处理当前月。
- 赛季默认：周期 6 个月、重置比例 30%、锚点为首次读取时的当月 1 日（UTC，必须是某月 1 日）；赛季结束后首次扫描对所有 `rating!=1500` 用户 `new=max(0, round(1500+(rating-1500)*(100-重置比例)/100))`（默认 30%：2600→2270、1200→1290），计算值不变（含 1500）不写历史；跨多赛季只处理最近结束的上一赛季。
- 幂等：`rating_runs` 唯一键 + 衰减/重置历史部分唯一索引 `(user_id,kind,period_key)`；重复 tick、多实例、重启不会重复扣分/重置。首次创建默认设置或修改锚点/月数时，预标记新「上一赛季」为已跳过，不立即触发重置。
- 衰减/重置成功后递增榜单缓存版本 `kapoj:leaderboard:rating:version`（best-effort，Redis 故障只记日志）。复用现有 Rating 扫描器（`KAPOJ_RATING_SCAN_INTERVAL`，默认 60s），不新增 worker 进程或环境变量。

## 学习路线（M4-2A / M4-2B / M4-2C）

**前端页面**

- 侧边栏「学习路线」进入 `/learning-paths`：官方/公开自定义路线卡片，筛选「全部/官方/自定义/我创建的」（游客隐藏“我创建的”），登录用户可「新建路线」；卡片展示标题、作者、`已完成 n / n 节点` 与官方/公开/私有标签，作者或 `learning_path.official.manage` 可看到「编辑」入口。
- 路线树 `/learning-paths/:id`：`TreeTable` 展示节点层级（`completed` 绿色、`active` 主色、`locked` 灰色锁），节点列含 `solvedCount/requiredCount` 进度条与题目数；点击节点右侧展示描述与题目链接；游客根 `active`、其余 `locked`；私有/下架路线显示「学习路线不存在或无权查看」。
- 编辑页 `/learning-paths/new`、`/learning-paths/:id/edit`：路线表单（标题/描述/封面/排序/公开；官方开关仅创建时且 `learning_path.official.manage` 可见并强制公开）+ 节点树编辑器（添加根/子节点、编辑、上移/下移、删除叶子；题目 1–50 去重、`requiredCount` 1–题目数）。
- 官方管理 `/admin/learning-paths`：管理全部路线（含下架）；`learning_path.official.manage` 可新建官方、编辑、下架/恢复；`learning_path.admin.read` 只读。

**接口与规则**

- `GET /api/learning-paths`：游客可访问；返回官方路线、公开自定义路线与自己的私有路线，`mine=true` 只看自己创建的路线，`includeInactive=true` 仅管理员可查看已下架路线；排序为官方优先、`sortOrder`、`id`。
- `GET /api/learning-paths/{id}`、`GET /api/learning-paths/{id}/tree`：返回路线元信息/树结构；私有路线仅作者与管理员可见，已下架路线仅管理员可见，否则 40400。
- `POST /api/learning-paths`：登录用户创建自定义路线，默认私有；管理员可创建强制公开的官方路线。`PUT`/`DELETE /api/learning-paths/{id}` 仅作者或管理员，删除为软删除（下架），管理员可恢复。
- 节点接口 `POST/PUT/DELETE /api/learning-paths/{id}/nodes...`：父节点必须同路线、不能成环、最大深度 8；题目 1–50 道且必须存在/公开；`requiredCount` 为 1–题目数；仅叶子节点可删除。
- **服务端进度（M4-2B）**：登录用户读取列表/树前服务端重算并写回进度。节点 `solvedCount` = 该节点题目中去重 AC 的题数，`solvedCount >= requiredCount` 时 `status=completed`；根节点默认 `active`，子节点仅父节点完成后 `active`、否则 `locked`。已完成节点锁存：之后改题或提高 `requiredCount` 不回退已完成用户（`solvedCount` 保留完成时的值），未完成/进行中节点按新配置重算。游客根 `active`、其余 `locked`、`solvedCount=0`，不写进度。
- **AC 事件（M4-2B）**：`submission.accepted` 事件触发该用户进度重算（与 gamification 在同一 Asynq handler 组合，幂等可重试）；读取补算兜底。
- **题目下线守卫（M4-2B）**：题目被任一活跃学习路线引用时，`PUT /problems/{id}` 将 `isPublic` 改为 false（下线）与 `DELETE /problems/{id}` 均返回 40001「题目被学习路线引用，不能下线/删除」；移除引用节点后允许下线/删除。

## 社交互动（M4-3）

**前端页面**

- 题解列表 `/problems/:id/solutions` 与详情 `/problems/:id/solutions/:postId` 的帖子卡片显示点赞数与点赞按钮：登录用户可点赞/取消赞（已赞高亮，乐观更新失败回滚并 Toast），游客点击跳登录。
- 用户主页 `/users/:id` 资料头部显示粉丝数/关注数，点击数字打开粉丝/关注 Dialog（两个 Tab、分页）；登录且非本人显示「关注/已关注」按钮，游客点击跳登录。
- 顶部栏用户菜单旁新增通知铃铛（仅登录显示），未读数 badge，60 秒轮询、卸载清理，点击进入 `/notifications`。
- `/notifications` 通知页：分页列表（类型标题、actor、时间、已读/未读样式）、点击标记已读并跳转站内 `link`、「全部已读」按钮、空/错/载入态；游客跳登录。

**接口与规则**

- 点赞：`POST/DELETE /api/problems/:id/solutions/:postId/like`，登录；返回 `{liked, likeCount}`；重复点赞/取消幂等；自赞 40001「不能点赞自己的帖子」，隐藏题/不存在帖子 404。帖子列表/详情返回 `likeCount`/`likedByMe`（游客 false）。
- 关注：`POST/DELETE /api/users/:id/follow`，登录；返回 `{following, followersCount}`；自己/不存在/非 active 分别 40001/404/40001。公开 `GET /api/users/:id/social`（`followersCount`/`followingCount`/`isFollowing`，游客 `isFollowing=false`）、`GET /api/users/:id/followers|following`（分页，摘要不含 email）。封禁/非 active 用户仍可读计数/列表，只有写关注要求 active。
- 通知：`GET /api/notifications`（分页）、`GET /api/notifications/unread-count`、`POST /api/notifications/:id/read`（非本人 404）、`POST /api/notifications/read-all`，均需登录；类型 `solution_reply`/`solution_like`/`follow`，`dedupe_key` 去重（重复赞/关注/同一回复不重复通知），自己触发自己不生成通知。
- 活跃度：论坛回复 +3、点赞 +1、关注 +2，每类每日上限 20；唯一 ref 去重，取消赞/关注不扣分。
- 共享分：题解帖子每获 1 赞作者 +5（`shared_items(type=solution)` 的 `like_count`），取消赞扣回，删除帖子清理点赞与共享项；共享榜公式第二项自此生效。

## 平台管理后台与 RBAC（RBAC-D）

**权限模型**

- RBAC 权限来源为 `users → user_roles → roles → role_permissions → permissions` 的实时并集；`users.role` 与 JWT `role` 是 legacy 兼容字段，**自 RBAC-C 起不再参与任何管理授权，仅用于展示/兼容**。
- 权限码格式为 `domain.action`，共 37 个，按管理后台、用户与角色、题库、测评记录、比赛、游戏化、学习路线、Rating、AI、证书分组；完整列表见 [`DEVELOPMENT.md`](DEVELOPMENT.md) 4.7。常用码：`admin.dashboard.view`、`user.read`、`role.read`、`problem.create`、`problem.view_hidden`、`contest.create`、`gamification.achievement.manage`、`learning_path.official.manage`、`learning_path.admin.read`、`rating.settings.manage`、`ai.settings.manage`、`certificate.manage`。
- 内置角色 7 个：`super_admin`（绕过所有权限检查）、`content_admin`（全部 `problem.*` + `learning_path.official.manage`/`learning_path.admin.read`）、`contest_admin`（全部 `contest.*` + `submission.view_live_contest`）、`gamification_admin`（全部 `gamification.*`）、`user_admin`、`security_admin`、`auditor`。系统角色不可删除、code/name 不可改；`super_admin` 权限不可修改；除 `super_admin` 外，内置角色权限可由 `role.permissions.update` 调整。
- `GET /api/users/me` 返回 `roles`、`permissions`、`isSuperAdmin`；超级管理员的 `permissions` 可能为空，前端应以 `isSuperAdmin=true` 表示拥有全部权限。登录/注册响应中的这三个字段默认为空数组/false，登录后请再调用 `/users/me`。

**管理后台（已上线）**

- 拥有任一管理权限（`ADMIN_ACCESS_PERMISSIONS`，或 `super_admin`）的账号登录后，侧栏与用户菜单显示「管理后台」入口 `/admin`。
- 管理后台包含：概览仪表盘 `/admin`（`admin.dashboard.view` 显示统计，`admin.audit.view` 显示最近审计）、用户管理 `/admin/users`（`user.read`）、角色权限 `/admin/roles`（`role.read`）、审计日志 `/admin/audit`（`admin.audit.view`）、游戏化管理 `/admin/gamification`（任一 `gamification.*.manage`）。菜单与页面动作按权限显隐；无权限直接访问返回 403（前端跳转 `/forbidden`，后端 40300）。
- 内置 `auditor`（审计员）为只读角色：`admin.dashboard.view` + `admin.audit.view` + `user.read` + `role.read`，不含任何写权限。审计员登录后可看仪表盘/审计日志/用户/角色页面，但看不到编辑、封禁、调币、分配角色、新建/删除角色等变更按钮，后端对这些写操作同样返回 403。
- 角色/权限变更后，受影响账号刷新页面或重新拉取 `/users/me` 即生效；后端始终实时校验，旧 token 不影响新权限。
- **legacy role 不再授权**：`users.role`/JWT role 自 RBAC-C 起仅作展示/兼容，所有管理授权只认 `user_roles`/`role_permissions`。

**管理 API**

- 角色与权限：`GET /api/admin/permissions`、`GET/POST /api/admin/roles`、`PUT/DELETE /api/admin/roles/{id}`、`PUT /api/admin/roles/{id}/permissions`。
- 用户管理：`GET /api/admin/users`（keyword/status/roleId 筛选与分页）、`GET/PUT /api/admin/users/{id}`、`PUT /api/admin/users/{id}/roles`、`POST /api/admin/users/{id}/ban|unban`、`POST /api/admin/users/{id}/coins`（`user.coins.adjust`，事务内审计）。
- 审计与统计：`GET /api/admin/audit`、`GET /api/admin/stats`。
- 所有 `/api/admin/*` 接口使用 `RequirePermission`，`super_admin` 绕过但仍写审计；敏感操作（角色/权限、用户角色分配、封禁/解封、管理员编辑用户、金币调整）在同一事务内写审计，失败回滚。
- **保护规则**：系统角色不可删除；`super_admin` 权限不可修改；只有 super 能授予/移除/封禁/解封 super；禁止封禁自己；禁止移除/封禁最后一个 active super（返回 40001）。

**内容管理接口（RBAC-C 已迁移）**

- 题目：`POST /problems` → `problem.create`；`PUT /problems/:id` → `problem.update`（修改 `isPublic` 另需 `problem.publish`）；`DELETE` → `problem.delete`；测试点/排序 → `problem.testcase.manage`；checker → `problem.checker.manage`；隐藏题读取 → `problem.view_hidden`。
- 比赛：创建/克隆 → `contest.create`；更新 → `contest.update`；删除 → `contest.delete`；报名用户 → `contest.participant.read`；公告写 → `contest.announcement.manage`；非公开/开赛前/榜单读取 → `contest.view_hidden`。
- 测评：隐藏题提交 → `problem.view_hidden`；赛时保护绕过 → `submission.view_live_contest`；免报名比赛测试提交 → `contest.update`。
- 游戏化：成就/任务/徽章/称号各用 `gamification.*.manage`（含 `includeInactive`）；`gamification.config` 公开只读。
- 学习路线：`learning_path.official.manage`（写/管理官方与任意路线）、`learning_path.admin.read`（只读查看全部/下架/完整题目 ID）；作者自定义路线规则保留。
- 内容写操作（题目/比赛/游戏化/学习路线）写 best-effort 审计，action 为权限码。
- 这些接口均有对应管理后台页面（RBAC-D）；现有题库/比赛/游戏化管理入口已迁移到 `/admin` 管理壳，游戏化管理页 `/admin/gamification` 按四个 `gamification.*.manage` 权限生成 tab。

**Seed 与超管账号**

- 服务启动时在数据库迁移后执行 `rbac.Seed`。只有首次部署（`rbac_meta` 中没有 `seed_version`）才写入内置角色权限并把旧 `users.role` 映射为新角色：`super_admin → super_admin`、`problem_admin → content_admin`、`contest_admin → contest_admin`、`user → 不分配`。
- 首次 Seed 后，修改 `users.role` 不会再自动获得 RBAC 角色，也不会重新授权；请用 RBAC-B 的管理 API 维护 `user_roles`（`PUT /api/admin/users/{id}/roles`）。重启不会重复插入角色/权限/关联，也不会恢复已删除的角色权限。
- 自 RBAC-C 起，管理授权只认 `user_roles`/`role_permissions`；`users.role`/JWT role 仅作展示/兼容，不再用于任何管理接口授权。

## AI 辅助（M5-A）

**启用方式**

- AI 接口始终挂载；加密密钥来自环境变量 `KAPOJ_AI_ENCRYPTION_KEY`（优先）或本地密钥文件 `KAPOJ_AI_KEY_FILE`（默认 `data/ai.key`）。未配置密钥时后端正常启动，`GET /api/ai/configs` 返回 `keyConfigured=false`，保存配置/AI 调用返回 40001「AI 加密密钥未配置」。生成密钥：`openssl rand -base64 32`。
- 可在管理后台 `/admin/ai` 的「加密密钥」板块自动生成或手动填写密钥（写入密钥文件，0600），运行时立即生效，无需重启（见下方「AI 加密密钥管理」）。
- 可选参数：`KAPOJ_AI_REQUEST_TIMEOUT`（AI 上游请求超时，默认 `45s`）、`KAPOJ_AI_COOLDOWN`（同一用户两次 AI 调用最小间隔，默认 `2s`）。

**用户如何使用**

- 用户登录后通过 `POST /api/ai/configs` 保存自己的 AI provider、模型与 API key（key 使用 AES-256-GCM 加密存储，`GET /api/ai/configs` 只返回脱敏值）。内置 provider：`deepseek`、`openai`、`qwen`、`glm`。
- 保存配置后即可调用 `POST /api/ai/complete`（代码补全）、`POST /api/ai/format`（格式化）、`POST /api/ai/optimize`（优化），以及 `POST /api/ai/hint`（题目 AI 提示，首个用户生成后全局缓存，后续用户复用）。

**接口与规则**

- `GET/POST /api/ai/configs`：查询/保存当前用户的 AI 配置；未配置时 `configured=false`；`apiKeyMasked` 只返回脱敏值，不返回明文或密文。provider 必须为启用状态，model 为空时使用 provider 默认模型，首次保存必须提供 key，已有配置且 key 留空则保留原密钥。
- `POST /api/ai/complete`：返回补全片段 `{completion}`；请求体 `{code, language, cursor}`（cursor 必填）。
- `POST /api/ai/format`：返回格式化后的完整代码 `{code}`。
- `POST /api/ai/optimize`：返回 `{suggestions, code}`。
- `POST /api/ai/hint`：请求体 `{problemId}`，返回 `{hint, cached}`；缓存命中不要求用户配置、不消耗限流；非公开题对无 `problem.view_hidden` 的用户返回 40400。
- 所有 AI 接口仅登录用户可用（未登录 40100）；无 AI 配置调用生成接口返回 40001；限流返回 42903；AI 上游失败返回 50200；Redis 不可用返回 50300。
- **安全**：平台 provider 的密钥不写入数据库；用户 key 只加密存储、接口只返回脱敏值，不写日志/审计。不要把真实 API key 写进文档或提交到仓库。

## AI 辅助前端（M5-5）

前端已接入 M5-A AI 后端，入口如下（需后端已配置 AI 加密密钥，否则页面显示「管理员尚未配置 AI 密钥」；AI 开关关闭时显示「AI 功能未启用」）：

- **提交页 AI 工具栏** `/problems/:id/submit`：编辑器工具栏（语言选择与提交按钮之间）新增「AI 补全 / AI 格式化 / AI 优化」按钮。补全按当前光标（UTF-8 字节偏移）插入返回片段；格式化/优化先弹预览 Dialog，确认后才替换代码。未配置 AI 时显示提示与 `/settings/ai` 链接并禁用按钮；代码超过 64 KiB、限流、上游错误均给出中文提示。
- **题目详情 AI 提示** `/problems/:id`：提示卡片右上角「AI 提示」按钮；游客点击跳登录，登录用户调用生成/读取提示，缓存命中显示「来自缓存」。首个用户生成后全局缓存，其他用户复用。
- **AI 设置页** `/settings/ai`（登录可见，用户菜单「AI 设置」入口）：选择 provider（deepseek/openai/qwen/glm）、模型名与 API key；首次保存需填 key，已配置时留空保留现有密钥；只显示脱敏 key。密钥加密存储、不会回显明文。

前端行为与错误映射（40001/42903/50200/50300/404/64 KiB/空补全）见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §6.5；后端接口见上节「AI 辅助（M5-A）」。

## AI 管理开关（`/admin/ai`）

- 管理后台新增「AI 设置」`/admin/ai`（需 `ai.settings.manage` 权限，默认仅 `super_admin`）：显示当前状态（已启用/已关闭/不可用）与是否已配置加密密钥，用开关启用/关闭 AI 并保存；下方「加密密钥」板块展示密钥状态并可生成/手动填写/轮换（见下节）。
- 接口 `GET/PUT /api/admin/ai/settings`：返回/更新 `{enabled, keyConfigured, updatedAt}`；关闭后所有 `/api/ai/*` 立即返回 404，用户侧显示「AI 功能未启用」，重新开启立即恢复，无需重启。
- 密钥要求：`keyConfigured` 动态取自当前密钥（env 或密钥文件）；未配置时管理页显示警告且不能启用（启用请求返回 40001），关闭仍可保存。关闭不删除用户 AI 配置与提示缓存。
- 写操作写审计（action=`ai.settings.manage`、target=`ai_settings`）；无权限账号访问管理接口返回 403。完整语义见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §6.6。

## AI 加密密钥管理

- 密钥文件 `KAPOJ_AI_KEY_FILE`（默认 `data/ai.key`，0600）；环境变量 `KAPOJ_AI_ENCRYPTION_KEY` 优先且后台只读（此时 `/admin/ai` 密钥板块只读显示「由环境变量管理」）。
- 管理 API（均需 `ai.settings.manage`）：`GET /api/admin/ai/key` 返回 `{configured,source,masked,fingerprint}`（不含完整密钥）；`POST /api/admin/ai/key/generate` 自动生成 32 字节密钥并一次性返回完整密钥（请立即备份）；`PUT /api/admin/ai/key` 手动填写 base64/32 字节密钥。
- 轮换：已有密钥时生成/替换会自动用旧密钥解密、新密钥重新加密所有 `user_ai_configs`；事务失败回滚，旧文件备份为 `ai.key.bak` 并在成功后删除。无旧密钥但存在用户配置时返回 40001，防止破坏性覆盖。
- 无密钥错误：未配置密钥时用户保存配置、补全/格式化/优化、无缓存提示返回 40001「AI 加密密钥未配置」；`GET /api/ai/configs` 返回 `keyConfigured=false`；提示缓存命中仍可返回。
- 完整语义见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §6.7。

## AI 出题造数据（M5-4）

管理员在题目编辑中提交标程后，AI 生成纯 Python 3 标准库造数据脚本，judge 在 Docker 沙盒生成输入/输出，管理员预览确认后写入题目测试数据。

**使用步骤**

1. 确保评测服务已启动并拉取了 Python 镜像（造数据脚本用 `python:3.12-slim` 运行，标程按语言用对应镜像）：
   ```bash
   docker pull python:3.12-slim     # 生成器必需
   docker pull gcc:14               # 标程为 C/C++ 时
   docker pull eclipse-temurin:21-jdk   # 标程为 Java 时
   docker pull golang:1.26          # 标程为 Go 时
   ```
2. 登录拥有 `problem.testcase.manage` 权限的管理员账号，进入题目编辑页，在测试点区域点击「AI 造数据」进入 `/problems/:id/testcase-generator`。
3. 选择标程语言（c/cpp/python3/java/go）、粘贴/填写标程源码（≤64 KiB）、选择生成组数（1–10），点击「生成数据」。仅传统题支持（special/interactive 不支持自动生成输出）。
4. 页面轮询生成状态（约 2 秒一次，最长 10 分钟）；生成完成后预览各组输入/输出（最多显示前 64 KiB），点击「确认入库」把测试点写入题目（score=10、排序接在已有测试点后），或「丢弃」删除生成结果与文件。
5. 确认后的测试数据即可正常发布题目并参与评测；发布流程不变。

**规则**

- AI 生成的脚本必须是纯 Python 3 标准库脚本（stdin 读 1-based 整数 seed、stdout 输出一组输入，不写文件/不联网）；若模型返回 cyaron 等第三方依赖，沙盒会因缺少依赖而失败，请修改标程重新生成。
- 生成器/标程只在 judge Docker 沙盒执行（无网络、只读、CPU/内存/输出限制）；生成器 5s/256MB/16 MiB 输出，标程按题目 time/memory 限制与 16 MiB 输出上限。服务器进程绝不执行用户/AI 代码。
- 权限复用 `problem.testcase.manage`，不新增权限码；确认/丢弃/创建写审计。AI 管理开关只关闭 `/api/ai/*`，不影响本题管理接口。
- 接口：`POST /api/problems/:id/testcase-generations`、`GET .../testcase-generations/:genId`、`GET .../cases/:caseId/preview?kind=input|output`、`POST .../confirm`、`DELETE .../testcase-generations/:genId`。

**失败排查**

- 平台密钥缺失：创建任务返回 40002「AI 加密密钥未配置」。请用 `openssl rand -base64 32` 配置 `KAPOJ_AI_ENCRYPTION_KEY`（或 `/admin/ai` 的「加密密钥」板块生成）。
- 用户未配置 AI 模型：返回 40001「尚未配置 AI 模型」。请在 `/settings/ai` 保存自己的 provider/模型/API key。
- 生成脚本语法错误/标程编译失败：生成状态 `failed`，页面显示中文/编译错误摘要；修改标程后重新生成。
- Docker/镜像不可用：judge 报错找不到镜像或 Docker API 错误；先确认 `docker info` 正常、已 `docker pull python:3.12-slim` 及对应标程镜像。
- storage 目录不可读：judge 启动即失败（`KAPOJ_STORAGE_DIR ... is not readable`）；确认 server 与 judge 使用同一个绝对路径。
- 队列任务未消费（状态一直 `pending`）：确认 `apps/judge` 已启动、与后端连接同一个 Redis（`KAPOJ_REDIS_ADDR`），且状态不是 `pending` 卡住（可重新发起一次任务）。

## 题目详情资源、官方题解与解题思路论坛

题目详情页右侧的 Tabs 已改为四个可点击资源卡片（官方题解、解题思路、提交统计、关联比赛），提示单独放在题面下方；五个子页面路由为 `/problems/:id/official-solution`、`/problems/:id/solutions`、`/problems/:id/solutions/:postId`、`/problems/:id/statistics`、`/problems/:id/contests`。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md) 4.3，设计见 [`docs/superpowers/specs/2026-11-07-problem-detail-resources-design.md`](docs/superpowers/specs/2026-11-07-problem-detail-resources-design.md)。

- **官方题解**：由题目创建者（或超级管理员）编辑说明/代码与价格；创建者、超级管理员、已通过该题的用户免费查看，价格 0 时所有人免费，其他用户支付金币永久解锁（改价不影响已解锁用户）。`GET/PUT /api/problems/:id/official-solution`、`POST /api/problems/:id/official-solution/purchase`；`ProblemDetail.solutionPriceCoins` 用于卡片显示「免费 / 金币 N」。
- **解题思路**：每个登录用户可发布帖子（标题 + Markdown 正文），其他用户可回复、点赞；作者可编辑/删除自己的内容，题目创建者或 `problem.update` 管理员可删除他人内容。`GET/POST /api/problems/:id/solutions`、`GET/PUT/DELETE /api/problems/:id/solutions/:postId`、`POST/DELETE /api/problems/:id/solutions/:postId/like`、`POST/PUT/DELETE /api/problems/:id/solutions/:postId/replies...`。
- **关联比赛**：`GET /api/contests?problemId=<id>` 返回包含该题且当前用户可见的比赛（公开 OR 本人已报名 OR 拥有 `contest.view_hidden`）。
- 隐藏题的官方题解与讨论对无 `problem.view_hidden` 的用户返回 404；详情页对无权限用户隐藏官方题解/解题思路卡片。

## 题目导入（M5-6）

- 拥有 `problem.create` 权限的账号可在题目列表「导入题包」进入 `/problems/import`，上传 HydroOJ 导出的 zip 题包（单文件 ≤ 200 MB）；也可用 `POST /api/problems/import/hydro`（multipart，字段 `file`）通过 API 导入。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §7。
- 每个含 `problem.yaml` 的目录视为一道题：`problem.yaml` 的 `title`/`tag` 映射为题目标题/标签，`problem_zh.md`（缺失回退 `problem.md`）作为 Markdown 题面，`testdata/` 下同名 `.in`/`.out` 成对映射为测试点（score=10，按数字再字典序排序）。
- 导入题目默认下线（`is_public=false`），创建者为上传者，`source=HydroOJ`，难度 1、1000ms/256MB；slug 为 `hydro-<净化后的文件夹名>`，与已有题目冲突时不覆盖，单题失败进入 `failed`，不影响其他题。
- 限制与安全：题包 ≤200 MiB（body 210 MiB）、解压 ≤1 GiB、条目 ≤5000、题数 ≤100、单题测试点 ≤500、单文件 ≤16 MiB；拒绝绝对路径/`..`/符号链接/加密条目/非常规文件，忽略 `__MACOSX` 与隐藏文件，不执行题包内脚本。
- 题目详情页对所有人展示「创建者」并链接 `/users/:id`（`author{id,username,nickname}`，无创建者时不显示）；发布后即可正常提交评测。

## 证书认证（M5-7）

用户上传比赛证书图片申请认证，管理员审核通过/驳回；通过后生成唯一验证码，任何人可用验证码公开核验证书类型、等级、持有者昵称与通过日期；证书图片仅本人与管理员可见。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §5.6。

**前端页面**

- 用户菜单「我的证书」进入 `/settings/certificates`（登录）：查看自己的证书列表（状态待审核/已通过/已驳回、驳回原因、验证码与「复制验证链接」、图片预览），「申请认证」提交证书图片（选择类型、填写等级、上传 ≤5 MiB PNG/JPEG/WebP）。
- `/verify`（公开）：输入验证码核验，有效时展示证书类型、等级、持有者昵称与通过日期；无效时中性提示。支持 `?code=` 自动核验；不展示证书图片、邮箱、用户 ID 等敏感信息。
- 管理后台「证书审核」`/admin/certificates`（需 `certificate.manage`）：证书审核 Tab（默认待审核、状态筛选/分页、图片预览、通过/驳回，驳回必填原因）+ 证书类型 Tab（新增/编辑/停用/删除，被引用删除报错）。

**接口与规则**

- `GET /api/certificate-types`：公开启用的证书类型；`POST /api/certificates`（multipart `typeId`/`level`/`file`）、`GET /api/certificates/my`、`GET /api/certificates/:id/image`（本人或管理员）、`GET /api/certificates/verify?code=`（公开）。
- `GET /api/admin/certificates?status=&page=&page_size=`、`POST /api/admin/certificates/:id/review`、`GET/POST/PUT/DELETE /api/admin/certificate-types`（均需 `certificate.manage`）。
- 证书图片 ≤5 MiB、magic bytes 校验仅 PNG/JPEG/WebP，本地 `storage.Storage` 保存 `certificates/<id>`，仅本人或 `certificate.manage` 可读（他人/不存在 404，未登录 401）。
- 验证码格式 `KAPOJ-XXXX-XXXX`，大小写/空格/连字符不敏感；通过后生成，公开核验只返回类型/等级/昵称/通过日期，不返回图片/邮箱/userId/驳回原因/审核人。
- 证书类型：内置蓝桥杯、ACM-ICPC、CCCC 天梯赛、CCPC、NOI、NOIP、USACO；管理员可新增/编辑/启用停用/删除未被引用的类型；改名后历史证书与公开核验仍使用提交时快照的类型名。
- 审核与类型 CRUD 写审计（action=`certificate.manage`，target=`certificate`/`certificate_type`），驳回原因不写审计。

## 头像上传（M5-8）

用户可在个人资料页上传/移除头像，上传头像优先于外链头像，填写外链会替换上传；头像图片公开读取，旧文件在替换/移除时清理。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §4.1。

**前端页面**

- `/settings/profile` 编辑资料页：新增头像预览（当前有效头像或上传预览）、文件选择（≤2 MiB、PNG/JPEG/WebP/GIF）、「上传头像」与「移除头像」按钮；上传成功清空外链输入、更新头像与 Toast。提示「填写外链会替换已上传头像；留空则保留上传头像」。
- `/admin/users` 管理员用户编辑：该用户已有上传头像时，外链输入留空并提示「该用户已上传头像；留空保留，填写外链会替换上传」。
- 其他页面（UserMenu、用户主页、榜单、测评、社交）无需改动，`avatarUrl` 自动更新为新头像。

**接口与规则**

- `POST /api/users/me/avatar`（登录；multipart `file`）：仅 PNG/JPEG/WebP/GIF（magic bytes 校验）且 ≤2 MiB；保存为本地 storage key `avatars/<userID>/<random>`，`avatar_url` 写为 `/api/users/<id>/avatar?v=<unix>`，`Me.avatarUploaded=true`；替换/移除成功删除旧文件。
- `DELETE /api/users/me/avatar`（登录）：清空上传与外链头像；无头像时幂等。
- `GET /api/users/:id/avatar`（公开）：返回上传头像原始图片（Content-Type 来自数据库），响应 `Cache-Control: public, max-age=31536000, immutable`；外链头像或文件缺失返回 404。
- 来源语义：上传优先；`PUT /api/users/me` 保存非空外链会替换上传头像并删除旧文件；保存空外链保留已有上传（避免把内部 URL 当外链误删）；管理员编辑用户沿用同一语义。
- `Me` 与 `Profile` 增加 `avatarUploaded`（是否有上传头像），前端据此不把内部 URL 填入外链输入。不新增权限码、不接 MinIO、不提供裁剪/压缩/审核。

## 账号安全（修改密码 + 活跃会话）

用户可修改密码，并查看/管理当前账号的活跃会话（设备/IP/最近使用），单独退出某设备、退出其他设备或退出所有设备；修改密码后吊销全部会话并要求重新登录。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md) §4.1。

**前端页面**

- 用户菜单「账号安全」进入 `/settings/security`（登录）：上方修改密码表单（当前密码/新密码/确认，8–72 字节、两次一致、不能与当前相同）；下方活跃会话表格（设备摘要、`当前设备` Tag、IP、创建/最近使用时间），行内「退出此设备」、工具栏「退出其他设备」「退出所有设备」，均有二次确认。
- 当前设备由 `auth.sessionId` 与会话行 `id` 比对标记；改密成功、退出当前设备、退出所有设备后清空登录态并跳转登录，退出其他设备后刷新列表。

**接口与规则**

- `PUT /api/users/me/password`（登录）：校验当前密码；新密码 8–72 字节且不得与当前相同；成功后吊销全部会话；按用户限流 10 次/5 分钟，超限返回 42904「修改密码尝试次数过多，请 5 分钟后再试」，成功重置计数，Redis 不可用返回 50300。
- `GET /api/users/me/sessions`：返回 `SessionView{id,ip,userAgent,createdAt,lastUsedAt}`（最近使用优先），不返回 token/hash/用户 ID。
- `DELETE /api/users/me/sessions/:id`：退出本人单个会话（退出当前会话等同退出登录；他人/未知 404）。
- `POST /api/users/me/sessions/logout-others`（body `{sessionId}`）：保留当前设备、吊销其他会话；`POST /api/users/me/sessions/logout-all`：吊销全部会话。
- 登录/注册/刷新响应带 `sessionId`；刷新在同一会话内轮换 refresh token、sessionId 不变。Redis 会话结构：`refresh:<sha256>`、`session:<id>`、`user_refresh:<uid>`、`user_sessions:<uid>`，TTL 与 refresh token 一致（7 天）。IP 仅作展示（`c.ClientIP()`，best-effort），反向代理部署需配置 Gin trusted proxies。
- 改密只吊销 refresh token，已签发 access token 在过期前（最长 15 分钟）仍有效；前端改密后立即清空并重新登录。

## 测试与检查

后端（数据库相关测试需要 `-p 1`，因为各包共用同一个测试库）：

```bash
cd apps/server
export KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable'
go test -p 1 ./...
go vet ./...
gofmt -l .
golangci-lint run   # 配置见 apps/server/.golangci.yml
```

未设置 `KAPOJ_TEST_DATABASE_URL` 时，数据库测试会被跳过，其余测试照常运行。

评测服务（真实 Docker 沙盒测试需要 `KAPOJ_DOCKER_TESTS=1`，约 30 秒）：

```bash
cd apps/judge
export KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable'
go test -p 1 ./...
KAPOJ_DOCKER_TESTS=1 go test -p 1 ./internal/sandbox/
```

前端：

```bash
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
corepack pnpm test:e2e     # 管理后台真实浏览器验收（Playwright + 本机 Chrome）
```

修改 `packages/api/openapi.yaml` 后，重新生成前端类型：

```bash
corepack pnpm gen:api
```
