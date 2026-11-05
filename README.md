# kapOJ

打造最有趣味的 Online Judge 平台。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md)，进度见 [`docs/任务清单.md`](docs/任务清单.md) 与 [`docs/开发日志.md`](docs/开发日志.md)。

当前进度：**M4-A 已完成**（M0–M3 已完成；M4 进行中。M4-A 交付 Rated 比赛 Rating 结算、8 级等级、Rating 历史与 Rating 榜后端；M4-B 的活跃度、签到金币、共享分与三榜，M4-C 的排行榜前端与验收，以及学习路线、社交、Rating 衰减/赛季重置尚未实现）。

## 目录结构

```
apps/server     Go 后端（Gin + GORM）
apps/judge      独立评测进程（Asynq + Docker 沙盒）
apps/web        Vue 3 前端（Element Plus + md-editor-v3 + Monaco）
packages/api    OpenAPI 规范，前端类型由此生成
docker          docker compose（PostgreSQL 17、Redis 7、后端/评测镜像）
docs            任务清单、开发日志、实施计划
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
- M3 不包含排行榜与 Rating；M4-A 的 Rating 榜/Rating 历史见下一节，活跃度/共享榜等仍未实现。

## Rating 与排行榜（M4-A）

- `GET /api/leaderboard/rating?page=&page_size=`：公开 Rating 榜，只含 `status=active` 用户，按 Rating 降序、用户 ID 升序；行内 `score` 为 Rating，并返回 `level`、`equippedBadges`、`equippedTitle`。
- `GET /api/users/:id/rating-history?page=&page_size=`：公开 Rating 历史，按时间倒序返回比赛名、名次、赛前/赛后 Rating、`delta` 与 `createdAt`；用户不存在返回 40400。
- Rated 比赛结束后由后台扫描器自动结算，只统计比赛窗口内至少有 1 次已判（非 `pending`/`judging`）提交的报名者；使用 PostgreSQL 全局 advisory lock 串行处理不同比赛，`rating_history` 部分唯一索引 `(user_id, contest_id) WHERE contest_id IS NOT NULL` 保证重复扫描不会重复写历史或重复更新 Rating。结算完成写入 `contests.rating_settled_at`。
- `KAPOJ_RATING_SCAN_INTERVAL`：扫描间隔，10s–10m，默认 `60s`；每轮最多处理 20 场。
- `KAPOJ_RATING_JUDGE_GRACE`：比赛结束后等待 `pending`/`judging` 提交的宽限，1m–60m，默认 `10m`；超过宽限按已判结果结算。
- 等级为灰/绿/青/蓝/紫/橙/红/黑红；黑红名 `color=#000000`、`accentColor=#FF0000`，其他等级 `accentColor` 为空字符串。
- Rating 榜基础行使用 Redis 缓存 60 秒（key 含版本号），结算后递增版本；Redis/缓存故障时回退数据库；缓存故障本身不会导致 5xx（身份装饰查询失败仍可能返回 5xx）。
- 本阶段**未包含**活跃度榜、共享榜、签到金币（M4-B）、排行榜前端与用户主页 Rating 历史卡片（M4-C），也未实现学习路线、社交互动、Rating 衰减/赛季重置。

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
