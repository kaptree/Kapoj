# kapOJ 一键部署（Ubuntu 20.04）与 README 重构 设计文档

日期：2026-11-15
状态：待批准

## 意图与背景

kapOJ 当前只覆盖本地开发流程：`docker/docker-compose.yml` 提供 PostgreSQL 17 / Redis 7 / MinIO，`server`/`judge` 在 `app` profile 下；**没有前端容器**，`运行手册.md` §11 明确列出生产部署缺口（前端静态托管、HTTPS、端口暴露、密钥生成）需要手工处理。

目标：让一台全新的 Ubuntu 20.04 服务器执行**一条命令**（`sudo bash deploy.sh`）后，kapOJ 完整上线并可访问；同时把 450 行功能日志式 README 重构为面向新读者的标准结构。

用户已确认的决策：
1. **部署方案 C**：全 Docker + Caddy 入口容器，域名自动 Let's Encrypt HTTPS（无域名时 HTTP 80 直接可用）。
2. **README 整体重构**：简介-功能索引-架构-快速开始-生产部署-配置-文档索引 结构，功能细节压缩为索引并链接到 DEVELOPMENT.md / 运行手册.md。
3. **服务器在中国大陆**：脚本需处理 Docker Hub 拉取不可达/限速问题（可选镜像加速），并把 `apps/server/Dockerfile` 的 `gcr.io` 基础镜像换为大陆可达源（用户已豁免该文件的「不改业务代码」限制）。
4. **服务器规格 4 核 4GB 内存 / 30GB 磁盘**：在服务器上现场构建镜像（不需要本地构建上传的分支）。

## 目标

- 服务器唯一依赖是 Docker；脚本自动安装（含 Ubuntu 20.04 focal 的兼容回退）。
- `.env` 不存在时自动生成：随机数据库/MinIO 密码、JWT（≥32 字节）、AI/邮件加密密钥（base64 32 字节）、chmod 600；已存在则原样保留（幂等）。
- 新增前端入口容器（Caddy）：托管 Vite 构建产物、SPA 路由回退、`/api` 与 `/health` 反代到 `server`；`KAPOJ_DOMAIN` 非空时自动签发/续期 HTTPS 证书，为空时仅 HTTP 80。
- 生产 override 把 PostgreSQL/Redis/MinIO/server 端口绑定到 `127.0.0.1`，不暴露公网；预置评测所需四个语言镜像。
- `deploy.sh` 幂等：重复执行等于升级；提供 `update/status/logs/stop/restart` 子命令。
- 部署完成打印：访问地址、MinIO Console（提示 SSH 隧道）、首个超管授权步骤（运行手册 §7.3 的 SQL）、防火墙建议。
- README 重构后：新读者 5 分钟理解项目并跑起来；功能索引不丢失入口（链接到既有文档对应章节）；本地开发、测试章节保留并指向运行手册。

## 非目标

- 不引入外部托管 MinIO/S3 的自动配置（`.env` 已支持 `KAPOJ_MINIO_*` 覆盖，文档提示即可）。
- 不做数据库备份自动化（平台已有内置备份功能 M-backup，文档链接）。
- 不支持集群/多机部署、灰度、回滚到旧版本（`update` 重建即最新）。
- 不改动 `apps/server`、`apps/judge`、`apps/web` 的任何**业务逻辑**代码。唯一例外：`apps/server/Dockerfile` 的运行镜像基础从 `gcr.io/distroless/static-debian12:nonroot` 换为大陆可达的等价方案（见交付物 1），行为保持 nonroot + 静态二进制不变。
- 不改变现有本地开发工作流：`docker compose -f docker/docker-compose.yml up -d` + `go run` + `vite dev` 保持原样。
- 不支持 1GB 内存机器的服务器现场构建（README 写明生产推荐 2核4G/30G 起步；低于此配置提示但不阻止，走 swap 引导）。

## 反目标（出现即失败）

- 任何一步把数据库/Redis/MinIO 凭证或密钥明文打印到控制台或写进 git。
- 破坏现有开发流程或让 `docker compose -f docker/docker-compose.yml up -d`（无 profile、无 override）行为变化。
- 脚本在 `.env` 已存在时覆盖已有密钥。

## 约束

- 目标系统：Ubuntu 20.04 LTS（focal），root 或 sudo 用户；其他系统给出警告并尽力而为。
- **中国大陆网络**：Docker Hub 直连拉取经常不可达或严重限速。`install` 流程在安装 Docker 后、首次拉取前探测连通性（对 registry-1.docker.io 的短超时探测），不可达时提供**可选**镜像加速配置（写入/合并 `/etc/docker/daemon.json` 的 `registry-mirrors`，用户已有自定义配置时不覆盖、只提示）；可用 `KAPOJ_DOCKER_MIRRORS` 环境变量预置或跳过交互。加速器地址清单在实现时经 websearch 核实（此类清单时效性强），并提示阿里云/腾讯云用户优先使用厂商专属加速地址。
- focal 上 Docker 官方源可能在 2025 年后不再提供新版本：安装顺序为 ① `get.docker.com` 官方脚本；② 失败回退 `apt install docker.io` + 从 GitHub releases 安装 compose v2 独立二进制；安装后必须验证 `docker compose version` 可用（实现时用 websearch 核实 focal 支持现状并写入脚本注释）。
- **Compose 版本下限 v2.24**：生产 override 的端口重绑依赖 `!override` YAML 合并标签（compose 默认对 `ports` 是追加合并，直接覆盖会 EADDRINUSE 且仍暴露公网）。
- **构建在服务器现场进行**：目标规格 2核4GB/30GB 起步（用户实际为 4核4G/30G，充分满足）。脚本安装前检查可用内存（<2GB 警告并给 swap 引导，不自动改系统）与磁盘剩余（<15GB 警告）。
- **代码上服务器不依赖 git**：推荐 git clone，也支持 scp/zip 上传；`update` 子命令在非 git 目录就地重建，`--pull` 标志仅 git 仓库内可用。
- 前端构建：`schema.d.ts` 已提交 git，构建无需 `gen:api`；构建上下文必须是**仓库根目录**（pnpm workspace + lockfile）。
- judge 容器挂载 `/var/run/docker.sock`（base 已有），宿主机必须是 rootful Docker。
- `KAPOJ_TRUSTED_PROXIES` 需指向 compose 网段（如 `172.16.0.0/12`），否则会话列表 IP 显示异常。
- 服务器时区/防火墙不做强制修改；ufw 存在时提示放行 80/443，不自动开启 ufw。

## 方案（已选定）

**方案 C：全 Docker + Caddy 自动 HTTPS**（用户选定）。其余两个方案：B（宿主机 nginx + Node 构建前端，依赖多、失败点多）被否；纯 A（nginx 托管无自动 HTTPS）被 C 取代。

### 交付物 0：`apps/server/Dockerfile` 基础镜像替换（小改，唯一豁免的业务目录文件）

`gcr.io/distroless/static-debian12:nonroot` 在大陆不可达。改为 `debian:bookworm-slim` + 显式非 root 用户（`useradd` 系统用户）：

```dockerfile
FROM debian:bookworm-slim
RUN groupadd -r -g 65532 kapoj && useradd -r -g 65532 -u 65532 kapoj \
    && mkdir -p /data && chown kapoj:kapoj /data
COPY --from=build /out/server /server
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
```

行为等价：保持 nonroot、`/data` 可写；**注意静态二进制仍依赖运行时 CA 信任库**（出站 TLS：SMTP/AI/S3），运行阶段必须包含 `/etc/ssl/certs/ca-certificates.crt`（从构建阶段 COPY，golang 镜像自带）；代价是镜像约大 30MB + 200KB CA 包。`apps/judge/Dockerfile` 的基础 `docker:cli` 来自 Docker Hub，无需改动。

### 交付物 1：`apps/web/Dockerfile`（新增）

多阶段构建，构建上下文为仓库根目录：

- stage 1 `build`：`node:22-alpine`，`corepack enable` → `pnpm install --frozen-lockfile --filter web...`（含 workspace 依赖）→ `pnpm --filter web build`，产物 `apps/web/dist`。
- stage 2 运行：`caddy:2-alpine`，拷贝 `dist` → `/srv`，拷贝 `docker/caddy/Caddyfile` → `/etc/caddy/Caddyfile`；EXPOSE 80/443。

`docker/caddy/Caddyfile`（新增）：

```caddyfile
{$KAPOJ_SITE_ADDRESS::80} {
	encode zstd gzip
	handle /api/* {
		reverse_proxy server:8080
	}
	handle /health {
		reverse_proxy server:8080
	}
	handle {
		root * /srv
		try_files {path} /index.html
		file_server
	}
	header /assets/* Cache-Control "public, max-age=31536000, immutable"
}
```

`KAPOJ_SITE_ADDRESS` 由 `.env` 的 `KAPOJ_DOMAIN` 派生：空 → `:80`（仅 HTTP）；`oj.example.com` → 域名（Caddy 自动 HTTPS + 80 跳转）。

### 交付物 2：`docker/docker-compose.prod.yml`（新增，生产 override）

仅由 `deploy.sh` 以 `-f docker-compose.yml -f docker-compose.prod.yml` 方式叠加：

- `web` 服务：build 仓库根目录，`env_file: ../.env`，ports `80:80`、`443:443`，卷 `caddy_data:/data`、`caddy_config:/config`（证书持久化），`restart: unless-stopped`，依赖 server/postgres/redis/minio healthy。
- 端口重绑：postgres `127.0.0.1:5433`、redis `127.0.0.1:6380`、minio `127.0.0.1:9000`/`127.0.0.1:9001`、server `127.0.0.1:8080`。
- 为 postgres/redis/minio/server/judge 补 `restart: unless-stopped`。
- 新增顶层卷 `caddy_data`、`caddy_config`。

### 交付物 3：`deploy.sh`（新增，仓库根目录）

`bash deploy.sh [install|update|status|logs|stop|restart]`，默认 `install`。通用约定：`set -euo pipefail`、彩色输出、每步失败即停并给出排查提示；所有 compose 调用统一带 `-f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app`。

`install` 流程：
1. 权限检查（root 或 sudo）；OS 检查（非 Ubuntu 20.04 警告继续）；内存/磁盘检查（低于下限警告 + swap 引导）。
2. Docker 缺失则按「get.docker.com → docker.io + compose v2 独立二进制」顺序安装并验证（compose ≥ v2.24）。
3. **大陆连通性探测与可选镜像加速**：短超时探测 Docker Hub；不可达且用户同意时合并写入 `registry-mirrors`（已有 daemon.json 自定义配置时只提示不覆盖；`KAPOJ_DOCKER_MIRRORS` 可预置/跳过），重启 docker 生效。
4. `.env` 缺失：从 `.env.example` 复制，随机化 `POSTGRES_PASSWORD`、`MINIO_ROOT_PASSWORD`、`KAPOJ_JWT_SECRET`（`openssl rand -base64 48`）、`KAPOJ_AI_ENCRYPTION_KEY`、`KAPOJ_MAIL_ENCRYPTION_KEY`（`openssl rand -base64 32`），交互询问 `KAPOJ_DOMAIN`（可空），`chmod 600`。已存在：仅提示保留，不改动。
5. `KAPOJ_TRUSTED_PROXIES` 为空且非本地部署时写入 `172.16.0.0/12`。
6. `docker compose build` → `up -d`。
7. 拉取评测镜像 `gcc:14`、`python:3.12-slim`、`eclipse-temurin:21-jdk`、`golang:1.26`（后台并行，失败不阻塞部署，仅警告）。
8. 轮询健康：`http://127.0.0.1:8080/health` 返回 ok；再按 domain 检查 `http://127.0.0.1/`（无域）或 `https://$KAPOJ_DOMAIN`（有域，允许 60s 内证书就绪重试；大陆服务器使用 Let's Encrypt 需域名可从公网 80/443 访问，脚本给出提示）。
9. 打印汇总：前端地址、API 健康、MinIO Console `127.0.0.1:9001`（SSH 隧道示例）、首个超管授权 SQL（引用运行手册 §7.3）、`deploy.sh update` 用法、ufw 提示。

`update`：可选 `--pull` 先 `git pull`，随后 build + `up -d` + `docker image prune -f`。
`status/logs/stop/restart`：包装对应 compose 命令，logs 支持服务名参数。

### 交付物 4：README.md 重构（覆盖重写）

新结构（中文）：

```
# kapOJ
一句话简介 + 架构图（文本）
## 功能特性        ← 20+ 功能小节压缩为分组索引表，每行 1 句 + 链接 DEVELOPMENT.md/运行手册.md 对应章节
## 系统架构        ← 组件说明（web/caddy、server、judge、postgres、redis、minio）与调用关系
## 目录结构        ← 更新（含 docker/、deploy.sh、docs/）
## 快速开始（本地开发）← 精简自现有「本地启动」，细节链接运行手册
## 生产部署（Ubuntu 20.04 一键部署）← deploy.sh 用法、域名/HTTPS 一句话说明、端口表、升级、管理员引导、备份提示；反向代理详解链接运行手册 §11.3（README 不重复长配置样例）
## 配置            ← .env 关键变量分组说明，链接 .env.example；后台可改项说明
## 文档索引        ← DEVELOPMENT.md、运行手册.md、docs/任务清单、开发日志、specs/plans
## 测试与检查      ← 保留现有内容
## 当前进度        ← 2-3 句 + 链接 docs/任务清单.md
```

功能细节去向：现有各功能小节的「接口与规则」在 DEVELOPMENT.md（规格）与运行手册 §8（操作）中已有对应内容；索引行链接到这些章节，不新增长文。**信息保全规则（建造者检查裁决）**：实现时逐节核对链接目标，凡在 DEVELOPMENT.md / 运行手册中找不到对应内容、且会因压缩而丢失的信息，就在该功能索引行内保留 1-2 句摘要——绝不静默删除内容；索引因此略长于预期是可接受的。

### 交付物 5：小改动

- `.env.example`：新增 `KAPOJ_DOMAIN=`（注释说明：空=HTTP 80；填写=自动 HTTPS）；`KAPOJ_TRUSTED_PROXIES` 注释补充生产建议值。
- **`运行手册.md` §11 扩写为完整「生产部署」章节**（用户明确要求反向代理部分写详细），小节结构：
  - 11.1 一键部署（deploy.sh 全流程与各步骤说明）
  - 11.2 端口与拓扑总览（谁暴露公网、谁只绑 127.0.0.1）
  - **11.3 反向代理与 HTTPS（重点，详细）**：
    - 内置 Caddy 入口：无域名 HTTP 80 直访；`KAPOJ_DOMAIN` 自动 Let's Encrypt 的前提（A 记录、安全组 80/443、ICP 备案提示）、签发/续期与常见失败原因；使用自有证书时的 Caddyfile `tls` 指令与证书卷挂载示例。
    - 自备反代在前（宿主机 nginx/宝塔/云 WAF）：compose 将 web 端口改绑 `127.0.0.1:<port>`；完整 nginx server 配置样例（`proxy_set_header Host/X-Real-IP/X-Forwarded-For/X-Forwarded-Proto`、`client_max_body_size` 需覆盖题包导入 200MB 与备份包 2GB、`proxy_read_timeout` 等超时）；宝塔面板操作要点。
    - 客户端真实 IP 链路：`KAPOJ_TRUSTED_PROXIES` 在「无代理 / 内置 Caddy / 自备 nginx 两层 / CDN 回源」四种拓扑下的推荐取值，以及填错的典型现象（会话列表 IP 显示为网关地址）。
    - CDN/边缘加速前置（阿里云/腾讯云）：回源协议与 Host、`/assets/*` 长缓存与 HTML 不缓存、上传类接口绕过缓存。
    - 排查表：502、413 上传受限、重定向循环（X-Forwarded-Proto 未透传）、证书签发失败（80 被占用/安全组未放行/域名未备案）。
  - 11.4 管理员引导与日常运维（超管授权 SQL、update/日志/备份）
  - 保留原 §11 既有条目（密钥、外部 MinIO/S3、备份、会话 IP 限制），并入对应小节，不丢内容。
- `.gitignore`：无需改动（`.env` 已忽略；`dist/` 已忽略）。

### 交付物 6：README 生产部署章节与手册的分工

README「生产部署」给**快速路径**：三行命令 + 域名/HTTPS 一句话说明 + 链接运行手册 §11 看反向代理详解与自备反代样例；README 不重复 11.3 的长配置样例。

## 验证计划

- 本机（macOS，Docker 可用时）：`docker compose -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app build` 全部镜像构建成功；`KAPOJ_SITE_ADDRESS=:80` 启动后 `curl 127.0.0.1/` 返回 HTML、`curl 127.0.0.1/health` 返回 ok、`curl 127.0.0.1/api/site` 返回 JSON、SPA 子路由（如 `/leaderboard`）刷新返回 index.html、`/assets/*` 带 immutable 缓存头。
- **生产 override 生效验证**：`docker compose ... config` 渲染结果中 postgres/redis/minio/server 的宿主机绑定均为 `127.0.0.1`（`!override` 生效，无重复端口条目）。
- `bash -n deploy.sh` 通过；有 shellcheck 则跑 shellcheck。
- 幂等性：连续执行两次 `deploy.sh install`，第二次不改动 `.env` 且直接到健康检查。
- 无 git 目录时 `update` 就地重建不报错；git 仓库外 `update --pull` 给出明确错误。
- Docker 不可用时至少验证脚本走到正确的报错分支。
- **只能在用户服务器上首跑验证的项**（脚本需对此输出明确的手工指引）：focal 的 Docker 安装分支、大陆镜像加速配置、评测语言镜像拉取、Let's Encrypt 签发。
- README 重构后核对：所有指向 DEVELOPMENT.md / 运行手册.md / docs/ 的链接锚点真实存在（grep 标题核对）。

## 留给实现者决定

- Caddyfile 中 gzip/zstd 与缓存头的精确写法、`try_files` 语法按 Caddy 2 当前版本文档为准。
- compose override 中各服务 `depends_on`/healthcheck 的最小充分集合。
- README 功能索引的分组成员数量与措辞；各链接锚点的最终核对。
- 脚本内工具函数拆分（如 `need_cmd`、`gen_secret`）与输出文案。
- focal 上 compose v2 独立二进制的具体版本选择与下载 URL 校验（≥ v2.24）。
- **大陆镜像加速器清单**：实现时用 websearch 核实当前（2026-11）仍可用的公共加速器，写入脚本常量并注明核对日期； 清单时效性强，脚本需支持用户自带加速地址。
- swap 引导的具体输出文案与检测阈值。
