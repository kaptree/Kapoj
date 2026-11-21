# kapOJ

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI](https://github.com/kaptree/Kapoj/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kaptree/Kapoj/actions/workflows/ci.yml)

打造最有趣味的 Online Judge 平台：题目、比赛、评测之外，还有金币、徽章、称号、Rating 天梯、学习路线、AI 辅助与证书认证。规格见 [`DEVELOPMENT.md`](DEVELOPMENT.md)，操作指南见 [`运行手册.md`](运行手册.md)，进度见 [`docs/任务清单.md`](docs/任务清单.md) 与 [`docs/开发日志.md`](docs/开发日志.md)。

当前进度：**M0–M9 全部里程碑已完成**（含 RBAC 权限体系、AI 辅助全套、用户 AI 多模型与 Anthropic 协议、Agent 擂台 V1、HydroOJ 题包导入、数据备份/恢复、邮箱验证与付费学习路线）。

## 功能特性

| 分组 | 能力概览 | 详见 |
|------|---------|------|
| 核心评测 | 题库（传统题/客观题/special judge，难度 1–100 五档着色）、CodeMirror 提交页、独立评测进程（Docker 沙盒，C/C++/Python3/Java/Go）、比赛（Rated/练习、公告、榜单）、测评记录与统计 | [`DEVELOPMENT.md` §4.3–4.4、§4.6](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| 账号与安全 | 注册/登录、邮箱验证与密码找回（支持 QQ/腾讯云/阿里云 SMTP）、修改密码、活跃会话管理（单设备/其他/全部退出） | [`DEVELOPMENT.md` §4.1](DEVELOPMENT.md)、[运行手册 §5.3](运行手册.md#53-邮件服务配置qq--腾讯云--阿里云) |
| 权限与管理后台 | 45 个权限码、7 个内置角色、实时权限校验、审计日志；仪表盘/用户/角色/审计/内容管理页面，legacy role 仅展示不再授权 | [`DEVELOPMENT.md` §4.7](DEVELOPMENT.md)、[运行手册 §7](运行手册.md#7-创建管理员账号与-rbac-权限) |
| 游戏化与排行 | 金币流水、成就、任务、徽章/称号商店与装备、每日签到；排行榜四榜（Rating 天梯含 8 级颜色、月度衰减、赛季重置）、活跃度榜、共享榜、富豪榜与自动签到 | [`DEVELOPMENT.md` §5](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| 学习路线 | 官方/自定义路线树（算法之树可视化）、服务端进度计算与完成锁存、付费路线（金币购买、参与统计）、题目下线守卫 | [`DEVELOPMENT.md` §4.5](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| 社交互动 | 题解论坛（发帖/回复）、点赞、关注、站内通知铃铛；互动接入活跃度与共享分 | [`DEVELOPMENT.md` §4.8](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| AI 辅助 | 用户多模型（每套独立名称/协议/接口地址/模型名/token，AES-256-GCM 加密，切换当前模型）、OpenAI 兼容与 Anthropic 协议、代码补全/格式化/优化、题目提示全局缓存、AI 出题造数据（judge 沙盒生成）、管理开关与密钥轮换；平台级 AI 模型（自定义地址/token/模型、协议选择、连通性测试、默认选择）用于题目难度初评与每日按通过率重算（通用定时任务 `system.jobs.manage`） | [`DEVELOPMENT.md` §6、§6.9](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| Agent 擂台 | 每用户 1 个外部 Agent；一次性明文 token（只存 SHA-256，可轮换/吊销）、随机公开题领取、顺序提交/轮询/放弃、每题 20 次与 30 分钟时限、独立难度加权 Elo Rating/历史/榜单、超时清算与结算 worker；Agent 提交与人类统计/题目计数/难度完全隔离；`agent.manage` 管理停用、恢复与重置 | [`DEVELOPMENT.md` §6.10](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作) |
| 内容运营 | 题目导入（HydroOJ 题包，防路径穿越、部分成功回滚）、题目详情资源（官方题解付费解锁/讨论区/统计/关联比赛）、证书认证（图片审核+公开验证码核验）、头像上传、账号安全设置页、站点管理（Logo/首页/页脚/访问统计）、平台与账号级备份/恢复 | [`DEVELOPMENT.md` §5.6、§7](DEVELOPMENT.md)、[运行手册 §8](运行手册.md#8-日常操作)、[docs/数据备份与恢复.md](docs/数据备份与恢复.md) |

> 各功能的完整接口清单与规则以 DEVELOPMENT.md 对应章节为准；本表仅作索引入口。

## 系统架构

```
用户 ──HTTP(S)──▶ web（Caddy 入口容器：静态资源 + SPA 回退）
                     │  /api、/health 反代
                     ▼
                  server（Go + Gin + GORM：REST API、游戏化 worker、Rating 结算器）
                     │            │              │
                     ▼            ▼              ▼
              PostgreSQL 17    Redis 7        MinIO（对象存储：头像/徽章/证书/备份）
                     ▲
                     │ Redis 队列 judge
                  judge（Go + Asynq）──Docker API──▶ socket-proxy（白名单）
                                                         │ /var/run/docker.sock:ro
                                                         ▼
                                                      dockerd
```

- 前端构建产物由 Caddy 容器直接托管（生产）；本地开发用 Vite（5173 端口）。
- server 启动时先执行 `apps/server/migrations/` 中的版本化迁移，再运行 RBAC Seed；生产默认不运行 GORM AutoMigrate（开发可用 `KAPOJ_DEV_AUTOMIGRATE=1` 显式开启）。
- judge 默认通过 `socket-proxy` 白名单代理访问 Docker（仅容器生命周期、exec 与只读诊断 API），不直接挂载 `/var/run/docker.sock`；直连回退 `KAPOJ_JUDGE_DOCKER_DIRECT=1` 仅用于故障排查。平台进程绝不直接执行用户代码。

## 目录结构

```
apps/server                  Go 后端（Gin + GORM + Asynq worker；migrations/ 为版本化 SQL）
apps/judge                   独立评测进程（Docker 沙盒）
apps/web                     Vue 3 前端（PrimeVue 4 + md-editor-v3 + CodeMirror 6）
packages/api                 OpenAPI 规范，前端类型由此生成
docker/docker-compose.yml    开发环境：PostgreSQL 17、Redis 7、MinIO、后端/评测镜像
docker/docker-compose.prod.yml  生产 override：端口收紧 + Caddy 入口（web 服务）
docker/caddy/Caddyfile       入口容器配置（静态托管 + /api 反代）
deploy.sh                    Ubuntu 20.04 一键部署脚本（install/update/status/logs/stop/restart）
docs                         任务清单、开发日志、规格设计、实施计划
```

## 环境要求

- **本地开发**：Go 1.26（工具链会自动下载）、Node.js 22（pnpm 经 `corepack pnpm` 调用，无需全局安装）、Docker。
- **生产部署**：Ubuntu 20.04 服务器，推荐 2 核 / 4GB 内存 / 30GB 磁盘起步；唯一依赖 Docker（`deploy.sh` 可自动安装并配置国内镜像加速）。

## 快速开始（本地开发）

在仓库根目录按顺序执行（详细说明与终端分工见 [运行手册 §3](运行手册.md#3-首次启动开发模式)）：

1. 启动数据库、缓存与对象存储（PostgreSQL 映射 **5433**，Redis **6380**，MinIO **9000/9001**，自动创建 `kapoj_test` 测试库）：

   ```bash
   docker compose -f docker/docker-compose.yml up -d
   ```

2. 创建统一配置文件（仓库根目录 `.env` 是唯一配置源）：

   ```bash
   cp .env.example .env
   chmod 600 .env
   ```

3. 启动后端（二选一；启动时自动执行版本化数据库迁移）。容器方式（推荐，自动读取根目录 `.env`）：

   ```bash
   docker compose -f docker/docker-compose.yml --profile app up -d --build
   ```

   或本机 `go run`：

   ```bash
   set -a; source .env; set +a
   export KAPOJ_DB_HOST=localhost KAPOJ_DB_PORT=5433
   export KAPOJ_REDIS_ADDR=localhost:6380 KAPOJ_MINIO_ENDPOINT=localhost:9000
   export KAPOJ_STORAGE_DIR="$(pwd)/apps/server/data"
   cd apps/server && go run ./cmd/server
   ```

   访问 <http://localhost:8080/health> 返回 ok 即正常。

4. 启动评测服务（需 Docker，消费 Redis `judge` 队列）：

   ```bash
   docker pull gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26
   cd apps/judge && go run ./cmd/judge      # 复用上一步导出的 .env 变量
   ```

5. 启动前端并打开 <http://localhost:5173>（`/api` 与 `/health` 自动代理到 8080）：

   ```bash
   corepack pnpm install
   corepack pnpm dev:web
   ```

## 生产部署（Ubuntu 20.04–26.04 LTS）

一条命令完成部署（自动安装 Docker/Compose、配置国内镜像加速与 Go/npm 构建源、生成随机密钥 `.env`、构建启动全部服务、拉取评测镜像、健康检查）：

```bash
git clone <本仓库> kapoj && cd kapoj
sudo bash deploy.sh                       # 仅 HTTP，访问 http://<服务器IP>
sudo bash deploy.sh --domain oj.example.com   # 或填域名，自动签发 Let's Encrypt 证书
sudo bash deploy.sh --domain oj.example.com \
  --cert-crt /path/fullchain.pem --cert-key /path/privkey.pem   # 使用自有证书
```

- **升级**：`git pull && sudo bash deploy.sh update`；重复执行 `install` 幂等，不会覆盖已有 `.env`。
- **日常运维**：`./deploy.sh status | logs | stop | restart`。
- **对外端口**：仅 `80/443`（Caddy 入口）；PostgreSQL `5433`、Redis `6380`、MinIO `9000/9001`、后端 `8080` 全部只绑定 `127.0.0.1`，不暴露公网。
- **反向代理与 HTTPS 详解**（自备 nginx/宝塔/CDN、自有证书、真实 IP 配置、排查表）：见 [运行手册 §11.3](运行手册.md#113-反向代理与-https)。
- **创建第一个管理员**：前端注册后执行 `sudo bash deploy.sh promote <用户名>`（同时写 `users.role` 与 `user_roles`）；外部数据库的手工 SQL 见 [运行手册 §7.3](运行手册.md#73-给账号授予管理员)。
- **备份**：平台内置备份/恢复（管理员后台），建议每日全量并异地保存，见 [docs/数据备份与恢复.md](docs/数据备份与恢复.md)。
- **评测安全**：默认自动启动 `socket-proxy`，judge 不直接挂载 Docker socket；独立评测节点、rootless Docker 与直连回退见 [运行手册 §12](运行手册.md#12-评测节点安全加固) 与 [docs/安全运维.md](docs/安全运维.md)。

## 配置

- 仓库根目录 `.env` 是唯一配置源（模板 [`.env.example`](.env.example)）；首次部署由 `deploy.sh` 自动生成，随机化五个必改项：`POSTGRES_PASSWORD`、`MINIO_ROOT_PASSWORD`、`KAPOJ_JWT_SECRET`、`KAPOJ_AI_ENCRYPTION_KEY`、`KAPOJ_MAIL_ENCRYPTION_KEY`。
- 对外域名 `KAPOJ_DOMAIN` 与入口地址 `KAPOJ_SITE_ADDRESS` 由脚本维护；反向代理网段 `KAPOJ_TRUSTED_PROXIES` 取值见 [运行手册 §11.3](运行手册.md#113-反向代理与-https)。
- SMTP、AI provider、站点外观等运行期设置在管理后台配置，保存于数据库，无需改 `.env`。

## 文档索引

| 文档 | 内容 |
|------|------|
| [`DEVELOPMENT.md`](DEVELOPMENT.md) | 开发规格：架构、数据库设计、全部模块接口与规则、AI 提示词汇总 |
| [`运行手册.md`](运行手册.md) | 启动/停止/验证/日常操作/常见问题/生产部署（含反向代理详解） |
| [`docs/任务清单.md`](docs/任务清单.md) · [`docs/开发日志.md`](docs/开发日志.md) | 进度与变更记录 |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) · [`docs/superpowers/plans/`](docs/superpowers/plans/) | 各功能设计文档与实施计划 |
| [`docs/性能测试报告.md`](docs/性能测试报告.md) · [`docs/数据备份与恢复.md`](docs/数据备份与恢复.md) | 性能基线、备份恢复操作 |
| [`docs/安全运维.md`](docs/安全运维.md) | 评测节点威胁模型、Docker socket 加固、残余风险与事件处置 |

## 测试与检查

CI（[`.github/workflows/ci.yml`](.github/workflows/ci.yml)）在 push 与 PR 到 `main` 时并行运行 `backend`、`judge`、`web` 三个基础 Job，任一失败即整体失败；PR 与手动触发时额外运行 `e2e`（Docker Compose 全栈 + Playwright，含 `core-flow.spec.ts` 核心链路，失败报告与 trace/视频保留 7 天）。手动触发可通过 `skip_sandbox_tests` 跳过 Docker 沙盒集成测试。

后端（数据库相关测试需要 `-p 1`，因为各包共用同一个测试库）：

```bash
cd apps/server
set -a; source ../../.env; set +a
export KAPOJ_TEST_DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5433/kapoj_test?sslmode=disable"
go test -p 1 ./...
go vet ./...
gofmt -l .
golangci-lint run   # 配置见 apps/server/.golangci.yml
```

未设置 `KAPOJ_TEST_DATABASE_URL` 时，数据库测试会被跳过，其余测试照常运行。

评测服务（真实 Docker 沙盒测试需要 `KAPOJ_DOCKER_TESTS=1`，约 30 秒）：

```bash
cd apps/judge
set -a; source ../../.env; set +a
export KAPOJ_TEST_DATABASE_URL="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5433/kapoj_test?sslmode=disable"
go test -p 1 ./...
KAPOJ_DOCKER_TESTS=1 go test -p 1 ./internal/sandbox/
```

前端：

```bash
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web

# 独立 e2e 全栈（Playwright + 本机 Chrome）
corepack pnpm e2e:stack:up       # 首次构建 + 预拉评测镜像；重复运行走缓存
corepack pnpm test:e2e:core      # 只跑注册/建题/AC/WA/金币/计数核心链路
E2E_API_URL=http://127.0.0.1:18080 E2E_BASE_URL=http://127.0.0.1:15173 corepack pnpm test:e2e
corepack pnpm e2e:stack:down     # 清理 e2e PostgreSQL 与 MinIO 卷
```

e2e 栈使用独立 Compose 项目 `kapoj-e2e`、仅绑定 `127.0.0.1` 的固定端口（API `18080`、Web `15173`、PG `15433`、Redis `16380`、MinIO `19000/19001`）和独立命名卷，不修改主 `.env` 或 `minio-data`。首次启动目标 ≤ 10 分钟；本地重复运行复用 Docker、pnpm 和评测镜像缓存。PR 的 `e2e` job 会运行全部 spec；失败时上传 HTML 报告和 `test-results`（trace/视频/截图），保留 7 天。视频需要 Playwright ffmpeg，`test:e2e:core` 会自动安装，单独运行完整 e2e 前可执行一次：

```bash
corepack pnpm --filter web exec playwright install ffmpeg
```

修改 `packages/api/openapi.yaml` 后，重新生成前端类型：

```bash
corepack pnpm gen:api
```

本项目基于 [MIT 许可证](LICENSE) 开源。Copyright © 2026 kaptree。
