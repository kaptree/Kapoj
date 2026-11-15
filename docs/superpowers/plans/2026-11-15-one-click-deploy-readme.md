# kapOJ 一键部署（Ubuntu 20.04）与 README 重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在全新 Ubuntu 20.04 服务器上以 `sudo bash deploy.sh` 一条命令完成 kapOJ 上线（Caddy 自动 HTTPS），并把 README 重构为标准结构、运行手册 §11 扩写为含详细反向代理的生产部署章节。

**Architecture:** 全 Docker 部署。新增 `apps/web/Dockerfile`（node:22 构建 → caddy:2-alpine 托管静态文件并反代 `/api`、`/health`）；新增生产 override `docker/docker-compose.prod.yml`（web 80/443 对外，pg/redis/minio/server 重绑 127.0.0.1，`!override` 端口合并）；`apps/server/Dockerfile` 基础镜像 gcr.io/distroless → debian:bookworm-slim（大陆可达）；根目录 `deploy.sh` 负责 Docker 安装、大陆镜像加速、`.env` 生成、构建启动、健康检查与引导输出。

**Tech Stack:** Docker + Docker Compose v2（≥2.24，`!override`）、Caddy 2、Node 22 + corepack pnpm 9.15、Go 1.26、Bash。

**Spec:** `docs/superpowers/specs/2026-11-15-deploy-readme-design.md`（实现时与计划一同阅读）

## Global Constraints

- 目标系统 Ubuntu 20.04（focal）；root/sudo；其他系统警告继续。
- Compose 版本下限 **v2.24**（`!override` 合并标签）。
- 镜像加速器默认清单（2026-11-15 websearch 核实，须注明核对日期）：`https://docker.1ms.run`、`https://docker.1panel.live`、`https://hub.rat.dev`；执行时须重新核实可用性。
- Docker 安装顺序：① `get.docker.com`；② 回退 `apt install docker.io` + GitHub releases compose v2 独立二进制（支持 `KAPOJ_COMPOSE_URL` 覆盖下载地址）。
- `.env` 已存在时**绝不覆盖、绝不重写任何键**（反目标）。
- 密钥生成：`POSTGRES_PASSWORD`/`MINIO_ROOT_PASSWORD` 用 `openssl rand -hex 16`；`KAPOJ_JWT_SECRET` 用 `openssl rand -base64 48`；`KAPOJ_AI_ENCRYPTION_KEY`/`KAPOJ_MAIL_ENCRYPTION_KEY` 用 `openssl rand -base64 32`。sed 替换用 `|` 分隔符（base64 字符集不含 `|` 与 `&`，安全）。
- `KAPOJ_SITE_ADDRESS` 写入 `.env`：`KAPOJ_DOMAIN` 非空时为域名，否则 `:80`；web 容器靠 `env_file: ../.env` 整体读取。
- 生产端口面：**仅 web 80/443 对公网**；server 127.0.0.1:8080、postgres 127.0.0.1:5433、redis 127.0.0.1:6380、minio 127.0.0.1:9000/9001。
- 生产 `KAPOJ_TRUSTED_PROXIES` 缺省写 `172.16.0.0/12`（仅当原值为空时）。
- 本地开发流程不变：`docker compose -f docker/docker-compose.yml up -d`（无 profile/override）行为与现状逐字节一致。
- 服务器规格底线 2核4GB/30GB；脚本低于 2GB 警告 + swap 引导、磁盘 <15GB 警告，均不自动改系统。
- 所有 compose 调用统一形如 `docker compose -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app …`。
- 冒烟测试用独立项目名 `-p kapoj-smoke`，避免污染本地开发卷；结束后 `down -v`。

## Review Focus

1. **生产 override 未生效 → 5433/6380/9000 暴露公网**：期望渲染结果中这些端口全部绑 127.0.0.1 且无重复条目。测试：Task 3 的 `docker compose config` 渲染断言。
2. **`.env` 已存在时密钥被重置**：期望第二次 `install` 前后 `.env` 的 sha256 完全一致。测试：Task 6 的 idempotency 步骤。
3. **反代后客户端 IP 显示为容器网关**：期望 `KAPOJ_TRUSTED_PROXIES` 在四种拓扑下取值正确（Task 8 表格 + Task 4 步骤校验值写入）。
4. **SPA 子路由刷新 404**：期望 `/leaderboard` 等任意路径刷新返回 index.html。测试：Task 6 的 curl 断言。
5. **自备 nginx 大文件上传 413**：期望样例配置 `client_max_body_size` ≥ 2g（备份包上限）。测试：Task 8 样例文本断言（grep 2g）。

---

### Task 1: 前端镜像（root .dockerignore + apps/web/Dockerfile + Caddyfile）

**Files:**
- Create: `.dockerignore`（仓库根）
- Create: `apps/web/Dockerfile`
- Create: `docker/caddy/Caddyfile`

**Interfaces:**
- Produces: 镜像 `kapoj-web`（build context = 仓库根，`docker compose build web` 可构建）；Caddyfile 消费环境变量 `KAPOJ_SITE_ADDRESS`（默认 `:80`），反代上游 `server:8080`。

- [ ] **Step 1: 建分支并提交设计文档**

```bash
cd /Users/admin/codespace/kapoj
git checkout -b feat/one-click-deploy
git add docs/superpowers/specs/2026-11-15-deploy-readme-design.md
git add docs/superpowers/plans/2026-11-15-one-click-deploy-readme.md
git commit -m "docs: add one-click deploy and README redesign design"
```

- [ ] **Step 2: 创建根 `.dockerignore`**

```
.git
.env
**/node_modules
**/dist
**/coverage
**/.gocache
apps/web/playwright-report
apps/web/test-results
apps/server/bin
apps/server/data
minio-data
docs
*.md
Export.zip
.DS_Store
```

（`*.md` 排除文档以缩小上下文；前端构建不需要它们。）

- [ ] **Step 3: 创建 `apps/web/Dockerfile`**

```dockerfile
# 构建上下文必须是仓库根目录（pnpm workspace + lockfile + 已提交的 src/api/schema.d.ts）
FROM node:22-alpine AS build
WORKDIR /repo
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY apps/web/package.json apps/web/
RUN corepack enable && corepack pnpm install --frozen-lockfile --filter web...
COPY apps/web apps/web
RUN corepack pnpm --filter web build

FROM caddy:2-alpine
COPY docker/caddy/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /repo/apps/web/dist /srv
EXPOSE 80 443
```

- [ ] **Step 4: 创建 `docker/caddy/Caddyfile`**

```caddyfile
{$KAPOJ_SITE_ADDRESS::80} {
	encode zstd gzip

	@backend path /api/* /health
	handle @backend {
		reverse_proxy server:8080
	}

	handle {
		root * /srv
		try_files {path} /index.html
		file_server
	}

	/assets/* {
		header Cache-Control "public, max-age=31536000, immutable"
	}

	@index path /index.html
	header @index Cache-Control "no-cache"
}
```

（Caddy 反代自动附加 `X-Forwarded-For`/`X-Forwarded-Proto`；域名模式下自动 80→443 跳转与 Let's Encrypt 签发续期。）

- [ ] **Step 5: 构建镜像**

Run: `docker build -f apps/web/Dockerfile -t kapoj-web:dev .`
Expected: 构建成功（vue-tsc + vite build 通过），镜像存在。

- [ ] **Step 6: 容器行为验证**

```bash
docker run --rm -d --name kapoj-web-test -p 8099:80 -e KAPOJ_SITE_ADDRESS=:80 kapoj-web:dev
sleep 2
curl -sf http://127.0.0.1:8099/ | grep -q '<div id="app">' && echo INDEX_OK
curl -sf http://127.0.0.1:8099/leaderboard | grep -q '<div id="app">' && echo SPA_OK
ASSET=$(curl -s http://127.0.0.1:8099/ | grep -o '/assets/[^"]*\.js' | head -1)
curl -sI "http://127.0.0.1:8099$ASSET" | grep -qi 'immutable' && echo ASSET_CACHE_OK
docker rm -f kapoj-web-test
```

Expected: 三行 OK 依次输出（`/api/*` 无后端时 502 属预期，不校验）。

- [ ] **Step 7: Commit**

```bash
git add .dockerignore apps/web/Dockerfile docker/caddy/Caddyfile
git commit -m "feat: add web image with caddy static hosting and api proxy"
```

### Task 2: server 基础镜像替换（大陆可达）

**Files:**
- Modify: `apps/server/Dockerfile:9-14`（仅运行阶段）

**Interfaces:**
- Produces: 运行阶段基于 `debian:bookworm-slim`，非 root uid/gid `65532:65532`，`/data` 目录属主 65532，其余（构建阶段、ENTRYPOINT、EXPOSE）不变。

- [ ] **Step 1: 替换运行阶段为：**

```dockerfile
FROM debian:bookworm-slim
RUN groupadd -r -g 65532 kapoj && useradd -r -g 65532 -u 65532 kapoj \
    && mkdir -p /data && chown kapoj:kapoj /data
COPY --from=build /out/server /server
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
```

- [ ] **Step 2: 构建并验证非 root**

```bash
docker build -t kapoj-server:dev apps/server
docker run --rm --entrypoint id kapoj-server:dev -u
```

Expected: 输出 `65532`。

- [ ] **Step 3: Commit**

```bash
git add apps/server/Dockerfile
git commit -m "chore: switch server runtime base to debian bookworm for CN registry reachability"
```

### Task 3: 生产 override `docker/docker-compose.prod.yml`

**Files:**
- Create: `docker/docker-compose.prod.yml`

**Interfaces:**
- Consumes: Task 1 的 `web` 镜像构建（context `..`，dockerfile `apps/web/Dockerfile`）；Task 2 后的 server/judge 构建；base compose 的服务名（`postgres`/`redis`/`minio`/`server`/`judge`）。
- Produces: `deploy.sh`（Task 5）依赖的服务集合与端口绑定；卷 `caddy_data`、`caddy_config`。

- [ ] **Step 1: 创建文件，内容为：**

```yaml
# 生产部署 override：仅由 deploy.sh 以
#   docker compose -f docker-compose.yml -f docker-compose.prod.yml --profile app …
# 方式叠加。本地开发不要加载本文件。
services:
  postgres:
    ports: !override
      - "127.0.0.1:5433:5432"
    restart: unless-stopped

  redis:
    ports: !override
      - "127.0.0.1:6380:6379"
    restart: unless-stopped

  minio:
    ports: !override
      - "127.0.0.1:9000:9000"
      - "127.0.0.1:9001:9001"
    restart: unless-stopped

  server:
    ports: !override
      - "127.0.0.1:8080:8080"
    restart: unless-stopped

  judge:
    restart: unless-stopped

  web:
    build:
      context: ..
      dockerfile: apps/web/Dockerfile
    env_file:
      - ../.env
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - caddy_data:/data
      - caddy_config:/config
    restart: unless-stopped
    depends_on:
      server:
        condition: service_started
    # 无 healthcheck：deploy.sh 从宿主机 curl 真实端点判定就绪

volumes:
  caddy_data:
  caddy_config:
```

- [ ] **Step 2: 渲染断言（Review Focus #1）**

```bash
cd /Users/admin/codespace/kapoj
docker compose -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app config \
  | grep -E 'published|host_ip' 
```

Expected: postgres/redis/minio/server 的 `host_ip` 均为 `127.0.0.1`；web 为 80/443 且无 host_ip 限制（全公网）；每个服务无重复端口条目（`!override` 生效）。

```bash
docker compose -f docker/docker-compose.yml config | grep -A1 'published: "5433"'
```

Expected: 无 `host_ip: 127.0.0.1` —— base 行为不变（开发流程不受影响）。

- [ ] **Step 3: Commit**

```bash
git add docker/docker-compose.prod.yml
git commit -m "feat: add production compose override with loopback-only infra ports and caddy entrypoint"
```

### Task 4: `.env.example` 增补

**Files:**
- Modify: `.env.example`（Server 段附近插入；`KAPOJ_TRUSTED_PROXIES` 行注释替换）

**Interfaces:**
- Produces: 两个新键 `KAPOJ_DOMAIN`、`KAPOJ_SITE_ADDRESS`（Task 5 的 deploy.sh 直接 sed 维护它们）。

- [ ] **Step 1: 在 `# ---------- Server ----------` 段的 `KAPOJ_HTTP_ADDR` 之前插入：**

```bash
# ---------- 部署入口（web/Caddy 容器读取） ----------
# 对外域名。为空：仅 HTTP，访问 http://<服务器IP>；填写（如 oj.example.com）：
# Caddy 自动签发/续期 Let's Encrypt 证书并 80→443 跳转。
# 前提：域名 A 记录指向本服务器、安全组/防火墙放行 80/443、大陆服务器需完成 ICP 备案。
KAPOJ_DOMAIN=
# 站点地址，由 deploy.sh 根据 KAPOJ_DOMAIN 自动维护（域名或 :80）；手动部署时自行设置。
KAPOJ_SITE_ADDRESS=:80
```

- [ ] **Step 2: 把 `KAPOJ_TRUSTED_PROXIES=` 行及其上方注释替换为：**

```bash
# 反向代理部署时 Gin 信任的代理网段（逗号分隔 CIDR），用于还原客户端真实 IP。
# 内置 Caddy/compose 部署建议 172.16.0.0/12；自备 nginx 在容器网段之外时按 运行手册 §11.3 取值。
# 留空表示未使用反向代理（本机直连）。
KAPOJ_TRUSTED_PROXIES=
```

- [ ] **Step 3: 校验**

Run: `grep -n 'KAPOJ_DOMAIN\|KAPOJ_SITE_ADDRESS' .env.example`
Expected: 两行均存在；文件其余内容未变（`git diff` 仅两处）。

- [ ] **Step 4: Commit**

```bash
git add .env.example
git commit -m "feat: add domain and site-address keys to env template"
```

### Task 5: `deploy.sh` 一键部署脚本

**Files:**
- Create: `deploy.sh`（仓库根，`chmod +x`）

**Interfaces:**
- Consumes: Task 3 的 override 文件与服务名；Task 4 的 `KAPOJ_DOMAIN`/`KAPOJ_SITE_ADDRESS` 键。
- Produces: 子命令 `install`（默认）/`update`/`status`/`logs`/`stop`/`restart`；标志 `--domain <d>`、`--skip-images`、`--pull`（仅 update）、`--yes`（跳过确认）；环境变量覆盖 `KAPOJ_DOCKER_MIRRORS`（空格分隔）、`KAPOJ_COMPOSE_URL`。

脚本约定（全文遵守）：`#!/usr/bin/env bash`、`set -euo pipefail`、`COMPOSE=(docker compose -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app)`、必须以 root 运行（EUID≠0 时报错提示 sudo，不自动提权）、彩色输出（非 TTY 自动去色）、每步 `[ STEP ]` 前缀日志。

函数清单（签名：`函数名` — 职责）：

- `log/ok/warn/die` — 输出。
- `need_root` — EUID 检查。
- `check_os` — 读 `/etc/os-release`；非 `Ubuntu 20.04` 时 warn 继续。
- `check_resources` — `free -m` 可用内存 <2000MB → warn + 打印 swap 引导命令（`fallocate -l 2G /swapfile` 三行，不执行）；`df -BG /` 剩余 <15GB → warn。
- `install_docker` — `command -v docker && docker compose version` 已满足则跳过（compose <2.24 时仍走重装提示）；否则① `curl -fsSL https://get.docker.com | sh`（失败或安装后 `docker compose version` 不可用时）② `apt-get update && apt-get install -y docker.io` + 下载 compose 二进制：`${KAPOJ_COMPOSE_URL:-https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64}` → `/usr/local/lib/docker/cli-plugins/docker-compose` + `chmod +x`（下载失败时 die 并打印手动安装指引）。成功标准：`docker compose version` 输出 ≥ v2.24。
- `ensure_mirrors` — 探测 `curl -fsS --connect-timeout 5 https://registry-1.docker.io/v2/`（任意 HTTP 响应=可达）；可达或 `KAPOJ_DOCKER_MIRRORS=off` 时跳过；不可达时：`/etc/docker/daemon.json` 已存在且含 `registry-mirrors` → 只提示不改动；否则写入 `{"registry-mirrors":[<默认三个，或 KAPOJ_DOCKER_MIRRORS 拆分>]}`（写入前备份为 daemon.json.bak）→ `systemctl restart docker`；无 systemd 环境时 `service docker restart`。注释注明「镜像清单核对日期 2026-11-15，可用性会变化」。
- `gen_env` — `.env` 存在 → `ok "保留现有 .env（不覆盖任何密钥）"` 返回；否则从 `.env.example` 复制 + `chmod 600`，`DOMAIN` 取 `--domain` 参数或 `read -r` 交互（`--yes` 时为空），校验正则 `^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$` 或空；写 `KAPOJ_DOMAIN` 与 `KAPOJ_SITE_ADDRESS`（域名非空=域名，空=`:80`）；五个密钥按 Global Constraints 生成并 `sed -i "s|^KEY=.*|KEY=VALUE|"`；`KAPOJ_TRUSTED_PROXIES` 值为空时写 `172.16.0.0/12`；随后打印「已生成 .env，请妥善备份」并再次 `chmod 600`。
- `build_up` — `$COMPOSE build` → `$COMPOSE up -d`。
- `pull_judge_images` — `--skip-images` 时跳过；四个镜像 `gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26` 并行后台 `docker pull`，`wait` 收集失败，仅 warn 不 die。
- `wait_health` — 60×2s 轮询 `curl -fsS http://127.0.0.1:8080/health` 含 `"status":"ok"`；通过后按地址二次确认：无域 `curl -fsS http://127.0.0.1/`；有域 `curl -fsS --resolve "$KAPOJ_DOMAIN:443:127.0.0.1" "https://$KAPOJ_DOMAIN/health"`（60s 内每 2s 重试，等待证书签发）；超时 die 并输出 `docker compose logs` 排查指引。
- `print_summary` — 打印：前端地址（`https://域名` 或 `http://IP`）、API `…/health`、MinIO Console `ssh -L 9001:127.0.0.1:9001 user@server` 后开 `http://127.0.0.1:9001`、超管授权（引用运行手册 §11.4/§7.3 SQL 两行）、`ufw allow 80,443/tcp` 提示（检测 ufw active 时打印，不执行）、`./deploy.sh update` 提示。
- `cmd_update` — `--pull` 时：`.git` 存在则 `git pull --ff-only`，否则 die「非 git 目录无法 --pull，请重新上传代码后直接 update」；`build_up` + `docker image prune -f` + `wait_health`。
- `cmd_status` — `$COMPOSE ps` + 两个 curl 健康行。
- `cmd_logs` — `$COMPOSE logs -f --tail=100 "$@"`。
- `cmd_stop`/`cmd_restart` — `$COMPOSE stop` / `restart`。
- `main` — case 分发 `$1`（默认 install）；install 顺序：`need_root → check_os → check_resources → install_docker → ensure_mirrors → gen_env → build_up → pull_judge_images → wait_health → print_summary`；`--yes`/`--domain`/`--skip-images`/`--pull` 参数解析移出 `$1`。

- [ ] **Step 1: 按上述约定实现 `deploy.sh` 并 `chmod +x deploy.sh`**

- [ ] **Step 2: 语法与静态检查**

Run: `bash -n deploy.sh && echo SYNTAX_OK; command -v shellcheck >/dev/null && shellcheck -S warning deploy.sh || echo SHELLCHECK_SKIPPED`
Expected: `SYNTAX_OK`；shellcheck 无 error 级告警（或环境无 shellcheck 时跳过）。

- [ ] **Step 3: 子命令冒烟（栈未启动状态）**

Run: `./deploy.sh status; echo EXIT=$?`
Expected: 打印 compose ps 表（可为空）与健康检查失败提示，`EXIT=0`（status 不因未启动而失败）。

Run（非 git 目录的 `update --pull` 应明确报错，Review Focus 之外的补充断言）:

```bash
rsync -a --exclude .git --exclude node_modules --exclude minio-data ./ /tmp/kapoj-nogit/
cd /tmp/kapoj-nogit && ./deploy.sh update --pull; echo EXIT=$?
```

Expected: 报错「非 git 目录无法 --pull…」，`EXIT=1`，且不触发构建。验证后 `rm -rf /tmp/kapoj-nogit`。

- [ ] **Step 4: Commit**

```bash
git add deploy.sh
git commit -m "feat: add one-click deploy script for ubuntu 20.04"
```

### Task 6: 本地全栈冒烟验证

**Files:** 无新文件（验证任务；只允许向上修复前序任务的缺陷）

**Interfaces:**
- Consumes: Task 1–5 全部产物；本机 Docker（已确认 29.4.2 可用）。

- [ ] **Step 1: 用独立项目名启动全栈（不污染开发卷）**

```bash
cd /Users/admin/codespace/kapoj
docker compose -p kapoj-smoke -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app up -d --build
```

Expected: postgres/redis/minio/server/judge/web 全部启动；`.env` 已存在故 gen_env 路径在本步不触发（Task 5 Step 3 已单独验证）。

- [ ] **Step 2: curl 矩阵（Review Focus #4）**

```bash
curl -fsS http://127.0.0.1:8080/health                     # {"status":"ok"...}
curl -fsS http://127.0.0.1/health                          # 经 Caddy 反代
curl -fsS http://127.0.0.1/api/site                        # 经 Caddy → server JSON
curl -fsS http://127.0.0.1/leaderboard | grep -q 'id="app"' # SPA 回退
curl -fsS http://127.0.0.1/health -H 'X-Forwarded-Proto: https'  # 不 5xx
```

Expected: 全部通过。

- [ ] **Step 3: idempotency（Review Focus #2）**

```bash
SHA_BEFORE=$(sha256sum .env | cut -d' ' -f1)
./deploy.sh install --yes --skip-images
SHA_AFTER=$(sha256sum .env | cut -d' ' -f1)
[ "$SHA_BEFORE" = "$SHA_AFTER" ] && echo ENV_UNTOUCHED
```

Expected: `ENV_UNTOUCHED`，install 走到健康检查并打印汇总。

- [ ] **Step 4: 清理**

```bash
docker compose -p kapoj-smoke -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app down -v
docker rmi kapoj-web:dev kapoj-server:dev 2>/dev/null || true
```

- [ ] **Step 5: 记录结果**：把矩阵输出贴回会话；发现缺陷回到对应 Task 修复后重跑本任务。

- [ ] **Step 6: Commit（如有修复）**

```bash
git add -A ':!*.env'
git commit -m "fix: address smoke test findings"
```

### Task 7: README 整体重构

**Files:**
- Rewrite: `README.md`（453 行 → 目标 ≤200 行）

**Interfaces:**
- Consumes: 既有 `DEVELOPMENT.md`、`运行手册.md`（§11 新结构由 Task 8 先行或同批完成——若执行顺序相反，链接以 Task 8 定稿的小节编号为准）；`.env.example` 键名；Task 3/5 的端口与命令事实。

- [ ] **Step 1: 按以下骨架重写（章节顺序固定，行数为指引）：**

```
# kapOJ                                  ← 一句话简介 + 进度一句 + 文档链接（~5 行）
## 功能特性                               ← 分组索引表，每组一行：组名 | 一句话 | 链接（~40 行）
   分组：核心评测（题目/提交/比赛/判题）、账号与 RBAC、游戏化与排行、
        学习路线、社交互动、AI 辅助、站点与管理、导入与备份
   链接目标：DEVELOPMENT.md 对应章节锚点、运行手册 §7/§8
   【信息保全规则】某细节在两文档无对应且会丢失 → 在该行内保留 1-2 句摘要
## 系统架构                               ← 文本拓扑图 + 组件职责表（web/server/judge/pg/redis/minio）（~25 行）
## 目录结构                               ← 现有树 + deploy.sh、docker/caddy、docker-compose.prod.yml（~15 行）
## 快速开始（本地开发）                    ← 5 步：compose 起 pg/redis/minio → cp .env → 后端 → 评测 → 前端；
                                            命令从现 README §本地启动 精简，细节链接运行手册 §3（~45 行）
## 生产部署（Ubuntu 20.04）                ← 快速路径：git clone → sudo bash deploy.sh → 完成；
                                            域名/HTTPS 一句话；端口表（80/443 公网，其余 127.0.0.1）；
                                            升级/回退一句话；反向代理详解链接 运行手册 §11.3（~30 行）
## 配置                                   ← .env 分组要点（必改五密钥、域名、trusted proxies）
                                            + 后台可改项说明 + 链接 .env.example（~20 行）
## 文档索引                               ← DEVELOPMENT.md / 运行手册.md / docs/ 各一行（~10 行）
## 测试与检查                             ← 原「测试与检查」章节原样保留（~40 行）
```

- [ ] **Step 2: 自检**：原 453 行中每个 `##` 小节都能在新 README 的功能索引或文档索引中找到归属；被压缩行含链接或内联摘要（Review Focus 保全规则）。

Run: `grep -c '^## ' README.md`
Expected: 9 个二级标题。

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: restructure readme around quick start and deployment index"
```

### Task 8: 运行手册 §11 扩写（生产部署 + 详细反向代理）

**Files:**
- Modify: `运行手册.md` §11（`## 11. 生产部署注意事项` 起至文件尾整体替换）；同步更新 §目录 第 11 项名称。
- Modify: `.gitignore`（追加 `docker/caddy/certs/`，配合 §11.3 自有证书模式）

**Interfaces:**
- Consumes: Task 3 端口事实、Task 5 脚本行为、Task 4 的 `KAPOJ_DOMAIN` 语义。README（Task 7）链接本文 §11.3。

- [ ] **Step 1: 按以下小节骨架重写（§11.3 为重点）：**

```
## 11. 生产部署（Ubuntu 20.04）
### 11.1 一键部署          ← deploy.sh 六个子命令表 + install 十步流程说明 + 镜像加速说明
### 11.2 端口与拓扑        ← 文本拓扑图（用户→Caddy 80/443→{静态,/api→server}；pg/redis/minio/server 仅 127.0.0.1）
### 11.3 反向代理与 HTTPS   ← 四个模式 + 真实 IP 链路 + CDN + 排查表（内容见 Step 2/3）
### 11.4 管理员引导与日常运维 ← 超管授权 SQL（沿用 §7.3 两段）、update/日志、备份提示（链接 docs/数据备份与恢复.md）
   原 §11 条目归属：密钥→11.1、外部 MinIO/S3→11.4、会话 IP 限制→11.3、备份→11.4（不丢条目）
```

- [ ] **Step 2: §11.3 写入四个模式**（每个含配置/命令样例）：

  1. **内置 Caddy·HTTP**：`KAPOJ_DOMAIN` 留空，`http://IP` 直访；验证 `curl -I http://IP`。
  2. **内置 Caddy·自动 HTTPS**：`.env` 填 `KAPOJ_DOMAIN=oj.example.com` + `KAPOJ_SITE_ADDRESS=oj.example.com` → `./deploy.sh update`；前提三条（A 记录、放行 80/443、ICP 备案）；签发失败三原因（80 被宿主机其他服务占用→`ss -ltnp | grep :80`、安全组未放行、域名未备案）。
  3. **内置 Caddy·自有证书**：证书放 `docker/caddy/certs/`（加入 .gitignore），Caddyfile tls 段样例：

```caddyfile
# 在 Caddyfile 站点块内替换自动 HTTPS：
tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem
# 并在 docker-compose.prod.yml 的 web.volumes 追加：
#   - ./caddy/certs:/etc/caddy/certs:ro
```

  4. **自备 nginx 在前**（宝塔/云主机已有 443 站点）：compose 把 web 端口改绑 `127.0.0.1:8081:80`（说明：编辑 docker-compose.prod.yml 或加本地 override）；nginx 样例（**逐字使用，含 2g 上传**）：

```nginx
server {
    listen 443 ssl http2;
    server_name oj.example.com;
    ssl_certificate     /etc/nginx/ssl/oj.pem;
    ssl_certificate_key /etc/nginx/ssl/oj.key;

    client_max_body_size 2g;              # 备份包上传上限 2GiB；题包导入 200MB
    proxy_read_timeout   300s;
    proxy_send_timeout   300s;

    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

  宝塔要点：站点→反向代理→关闭「缓存」，目标 `http://127.0.0.1:8081`，并在配置文件手工补 `client_max_body_size 2g;`。

- [ ] **Step 3: §11.3 写入真实 IP 链路表与排查表：**

  `KAPOJ_TRUSTED_PROXIES` 取值表（4 行）：

| 拓扑 | 取值 | 说明 |
|------|------|------|
| 无代理（HTTP 直连） | 留空 | Gin 直接取 RemoteAddr |
| 内置 Caddy（默认） | `172.16.0.0/12` | deploy.sh 自动写入 |
| 自备 nginx 在前 | `172.16.0.0/12`（nginx 仅转发到容器） | nginx 补 XFF，Caddy 透传，server 信任容器网段 |
| CDN/WAF 回源 | CDN 回源网段 + `172.16.0.0/12`，逗号分隔 | 否则会话 IP 显示为 CDN 节点 |

  排查表（6 行）：502（容器未起/反代目标错→`docker compose ps`、`curl 127.0.0.1:8080/health`）、413（`client_max_body_size`）、重定向循环（nginx 未传 `X-Forwarded-Proto`）、证书签发失败（三原因）、IP 显示网关（表 above）、上传慢断连（`proxy_read_timeout`）。

- [ ] **Step 4: 锚点核对与 .gitignore**

```bash
grep -n '^### 11\.' 运行手册.md          # Expected: 11.1–11.4 四行
grep -n 'docker/caddy/certs' .gitignore  # Expected: 一行
```

README 链接文本与锚点一致。

- [ ] **Step 5: Commit**

```bash
git add 运行手册.md .gitignore
git commit -m "docs: expand ops manual production chapter with detailed reverse proxy guide"
```

### Task 9: 终验（链接、清单、回归）

**Files:** 无新文件（只允许修文档/脚本笔误）

- [ ] **Step 1: 文档锚点全检**

Run: 对 README 与 运行手册 中出现的每个 `DEVELOPMENT.md#…` / `运行手册.md#…` / `docs/…` 相对链接，用 `grep '^#' 对应文件` 核对锚点存在。
Expected: 无死链。

- [ ] **Step 2: 回归——本地开发流程未变**

Run: `docker compose -f docker/docker-compose.yml config >/dev/null && echo BASE_OK`
Run: `corepack pnpm test:web && corepack pnpm build:web`
Expected: `BASE_OK`；前端测试与构建通过（server/judge 未改业务代码，`go vet` 抽查 `apps/server` 亦可）。

- [ ] **Step 3: Global Constraints 逐条核对**：在会话中列出每条约束与满足证据（一段一行）。

- [ ] **Step 4: Commit（如有零星修复）并汇总**

```bash
git add -A ':!.env' ':!Export.zip'
git commit -m "docs: final verification pass"
```
