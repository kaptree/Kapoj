# kapOJ

打造最有趣味的 Online Judge 平台。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md)，进度见 [`docs/任务清单.md`](docs/任务清单.md) 与 [`docs/开发日志.md`](docs/开发日志.md)。

当前进度：**M4-C 已完成**（M0–M3 已完成；M4 进行中。M4-A 交付 Rated 比赛 Rating 结算、8 级等级、Rating 历史与 Rating 榜后端；M4-B 交付活跃度、每日签到金币、共享分与活跃度/共享榜后端；M4-C 交付排行榜页（Rating/活跃度/共享三 Tab、等级颜色、签到）、用户主页 Rating 历史卡片、导航与首页入口，三个榜单 API 与页面均已可用；学习路线、社交、Rating 衰减/赛季重置尚未实现）。前端并行进行 PrimeVue 4 迁移，**UI-00～UI-18 与 UI-30（M4-C 排行榜/历史）已完成并提交**，剩余题库编辑（UI-19）与异常页（UI-20），详见 [`docs/primevue-migration.md`](docs/primevue-migration.md)。

## 目录结构

```
apps/server     Go 后端（Gin + GORM）
apps/judge      独立评测进程（Asynq + Docker 沙盒）
apps/web        Vue 3 前端（PrimeVue 4 + md-editor-v3 + Monaco；Element Plus 过渡期仅剩未迁移页面）
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
- 超级管理员可在 `/admin/gamification` 管理成就/任务/徽章/称号并上传徽章图片，也可用 `POST /api/admin/users/:id/coins` 调整金币（需填写备注）。
- M3 不包含排行榜与 Rating；M4-A/M4-B 的 Rating 榜、Rating 历史、活跃度榜、共享榜与签到见下一节。

## 排行榜、Rating 与活跃度（M4-A～M4-C）

**前端页面**

- 侧边栏「排行榜」`/leaderboard` 已启用：Rating（默认）、活跃度、共享三个 Tab，每个 Tab 独立分页（每页 20 条）并保留各自页码，切换 Tab 不串数据。Rating Tab 按服务端等级显示灰/绿/青/蓝/紫/橙/红/黑红颜色，黑红名首字符黑、其余红；用户列链接个人主页并展示已装备称号/徽章。
- 活跃度 Tab 展示今日/总活跃度与「签到 +5 金币」按钮：首次签到 +5 活跃度与 +5 金币，成功后刷新用户金币、签到摘要与活跃度榜；当日已签到按钮禁用；游客显示登录入口。重复签到不会重复发币。
- 用户主页 `/users/:id` 新增「Rating 历史」卡片：最近 20 场比赛表格与简单 SVG 折线；私密比赛显示“私密比赛”且不生成链接；无记录显示“暂无 Rating 记录”。
- 首页新增排行榜快捷入口与 M4-C 系统状态；侧栏版本为 M4-C。

**接口与规则**

- `GET /api/leaderboard/rating?page=&page_size=`：公开 Rating 榜，只含 `status=active` 用户，按 Rating 降序、用户 ID 升序；行内 `score` 为 Rating，并返回 `level`、`equippedBadges`、`equippedTitle`。
- `GET /api/users/:id/rating-history?page=&page_size=`：公开 Rating 历史，按时间倒序返回比赛名、名次、赛前/赛后 Rating、`delta` 与 `createdAt`；用户不存在返回 40400。
- Rated 比赛结束后由后台扫描器自动结算，只统计比赛窗口内至少有 1 次已判（非 `pending`/`judging`）提交的报名者；使用 PostgreSQL 全局 advisory lock 串行处理不同比赛，`rating_history` 部分唯一索引 `(user_id, contest_id) WHERE contest_id IS NOT NULL` 保证重复扫描不会重复写历史或重复更新 Rating。结算完成写入 `contests.rating_settled_at`。
- `KAPOJ_RATING_SCAN_INTERVAL`：扫描间隔，10s–10m，默认 `60s`；每轮最多处理 20 场。
- `KAPOJ_RATING_JUDGE_GRACE`：比赛结束后等待 `pending`/`judging` 提交的宽限，1m–60m，默认 `10m`；超过宽限按已判结果结算。
- 等级为灰/绿/青/蓝/紫/橙/红/黑红；黑红名 `color=#000000`、`accentColor=#FF0000`，其他等级 `accentColor` 为空字符串。
- `GET /api/leaderboard/activity?page=&page_size=`：公开活跃度榜，只含 `status=active` 且总活跃度 > 0 的用户，按总分降序、用户 ID 升序；`score` 为活跃度。
- `GET /api/leaderboard/sharing?page=&page_size=`：公开共享榜，只含 `status=active` 且共享分 > 0 的用户，按共享分降序、用户 ID 升序；`score` 为共享分。
- 活跃度规则：提交 +2（每日上限 40）、比赛报名 +10（每日上限 50）、每日签到 +5（每天一次）；同一提交/比赛/日期不重复计分，达到每日上限后不再加分，均不影响主流程。
- 签到金币：`POST /api/activity/signin` 每个本地自然日首次签到额外 +5 金币，签到活跃度、金币流水与余额在同一事务内完成；重复签到返回 `awarded=false`，不重复发币。`GET /api/activity/summary` 返回今日是否签到、今日活跃度与总活跃度。
- 共享积分：创建或克隆比赛 +20，服务启动时会把已有 `created_by>0` 的比赛幂等回填到 `shared_items`；共享分公式中的“题解被点赞 5/赞”尚未实现，本阶段恒为 0。
- 三个榜单的基础行都使用 Redis 版本缓存 60 秒，key 形如 `kapoj:leaderboard:<kind>:v<version>:<page>:<size>`；Rating 结算、活跃度写入、共享项写入后递增对应 `kapoj:leaderboard:<kind>:version`。缓存只含基础行，装备在读取后批量装饰；Redis/缓存故障时回退数据库，缓存故障本身不会导致 5xx（身份装饰查询失败仍可能返回 5xx）。
- 学习路线、社交互动（评论/点赞/关注/题解点赞/通知）以及 Rating 衰减/赛季重置尚未实现，属于后续 M4 阶段。

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
```

修改 `packages/api/openapi.yaml` 后，重新生成前端类型：

```bash
corepack pnpm gen:api
```
