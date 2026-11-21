# 国内网络部署加固设计（deploy.sh）

日期：2026-11-21
状态：待批准
关联问题：根目录 `question.txt`（10 项，服务器 42.193.50.191 / Ubuntu 26.04 LTS）

## 1. 目标

让 `deploy.sh` 在国内网络、Ubuntu 20.04–26.04 LTS 上可重复完成一次部署，并把失败点变成可诊断、可恢复的明确步骤。具体目标：

1. Docker/Compose 安装不再依赖 `get.docker.com` 单一路径；能识别并修复 Compose 插件多路径冲突、损坏二进制和 SIGSEGV。
2. Compose 始终读取仓库根 `.env`，不再因 `-f docker/...` 把项目目录切到 `docker/` 而丢变量。
3. 国内构建默认可用的 Go/npm 源，避免 `proxy.golang.org`、`registry.npmjs.org` 超时。
4. 镜像加速器清单更新、可探测、可覆盖；大镜像拉取有重试。
5. 支持 `--cert-crt/--cert-key` 自定义证书，避免 DNS 指向 CDN 时 Let's Encrypt 签发失败。
6. 提供 `./deploy.sh promote <用户名>`，一次写入 `users.role` 与 `user_roles`，解决 RBAC 未生效。
7. 明确支持 Ubuntu 26.04；失败时输出阶段、命令、日志路径和排查入口。

## 2. 非目标

- 不自动修改 DNS、ICP 备案、云安全组或防火墙；这些只能提示。
- 不默认替换宿主机的 Ubuntu apt 源；只在安装依赖时增加重试和超时。
- 不引入不可审计的第三方一键脚本；Docker 安装与加速配置由本脚本完成。
- 不改变应用权限模型，不绕过 `user_roles`。
- 不把密钥、`.env` 内容或证书私钥写入日志。

## 3. 约束

- 目标系统：Ubuntu 20.04/22.04/24.04/26.04 LTS，x86_64/arm64。
- 现有 Compose 语法依赖 Compose ≥ 2.24（`!override`）。
- 部署脚本以 root 运行；所有文件写入使用 `install`、`printf >` 或普通重定向，不依赖 `sudo tee`/heredoc 交互（对应问题 10）。
- 重复执行幂等：已有 `.env`、证书、加速器配置不被静默覆盖。
- 继续使用仓库根 `.env` 作为唯一应用配置源。

## 4. 方案选择

### A. 只在 deploy.sh 运行时打补丁

部署时用 `sed` 修改 Dockerfile、生成 `docker/.env` 软链、修改 Caddyfile、修 Compose 插件。

- 优点：只动一个文件。
- 缺点：每次部署污染工作区，`git pull` 冲突；手工 `docker compose build` 仍失败；补丁逻辑难以测试。放弃。

### B. 修源头 + deploy.sh 负责环境修复（采用）

把国内构建默认值写进 Dockerfile/Compose，把宿主环境修复、证书、RBAC 和诊断放进 `deploy.sh`。

- 优点：幂等、可复现；手工构建也能用；部署脚本只做编排和环境修复。
- 缺点：改动 `deploy.sh`、3 个 Dockerfile、Compose、Caddyfile、`.env.example` 和文档。

### C. 交给 linuxmirrors 等第三方脚本

Docker 安装和镜像加速交给第三方一键脚本。

- 优点：国内安装成功率高。
- 缺点：第三方脚本不可审计；Go/npm 构建、证书、RBAC 仍需自己改。放弃。

## 5. 详细设计

### 5.1 Docker 与 Compose 安装/修复

重写 `install_docker()` 为分阶段流程：

1. `ensure_base_packages()`：用带重试的 apt 安装 `ca-certificates curl gnupg openssl`；`apt-get update` 失败只警告，继续尝试安装。
2. 若 `docker compose version --short` 已可用且 ≥ 2.24，进入 `repair_compose_plugin()` 做健康检查后返回。
3. 若 Docker 已存在（例如 Docker CE），**绝不安装 `docker.io`**，避免 apt 替换/移除 Docker CE；只修复或安装 Compose 插件（优先 `docker-compose-v2`，其次 `docker-compose-plugin`）。
4. 仅当 Docker 不存在时，优先发行版包：`apt-get install -y docker.io docker-compose-v2`（Ubuntu 26.04 推荐路径）。
5. 发行版包失败时再执行 `get.docker.com`，然后补装 Compose 插件。
6. 仍无可用 Compose 时，才下载独立二进制：
   - `KAPOJ_COMPOSE_URL` 可覆盖；
   - 默认按 `uname -m` 选择 `docker-compose-linux-x86_64` 或 `docker-compose-linux-aarch64`；
   - 下载后先执行 `docker-compose version` 验证，验证失败不安装。

`repair_compose_plugin()`：

- 扫描 `/usr/local/lib/docker/cli-plugins`、`/usr/lib/docker/cli-plugins`、`/usr/libexec/docker/cli-plugins`、`${DOCKER_CONFIG:-$HOME/.docker}/cli-plugins`。
- 对每个 `docker-compose` 文件执行 `timeout 5 <file> version`：
  - 退出码非 0 或超时（损坏/SIGSEGV）→ 重命名为 `docker-compose.broken.<时间戳>`，打印原路径和备份路径。
  - 版本 < 2.24 → 重命名为 `docker-compose.old.<时间戳>`，避免它遮蔽新插件。
- 若 `/usr/libexec/docker/cli-plugins/docker-compose` 存在且 `/usr/lib/docker/cli-plugins/docker-compose` 不存在，创建软链接（对应问题 1）。
- 最后必须 `docker compose version --short` ≥ 2.24；失败时列出所有扫描路径和版本，提示安装 `docker-compose-v2` 后重试。

### 5.2 Compose 的 .env 加载

- `COMPOSE` 数组改为显式传入仓库根 `.env`：

  ```bash
  COMPOSE=(docker compose --env-file "$ROOT/.env" -f "$BASE_COMPOSE" -f "$PROD_COMPOSE" --profile app)
  ```

- `ensure_env_symlink()`：若 `docker/.env` 不存在，创建 `../.env` 软链；若已存在且不是软链，只警告不覆盖。该软链让运行手册里的手工 `docker compose -f docker/...` 也能找到变量，并解决 `required variable POSTGRES_USER is missing`（问题 4）。
- `install` 和 `update` 都在构建前调用 `ensure_env_symlink()`。

### 5.3 国内构建源

Dockerfile 增加可覆盖的构建参数：

- `apps/server/Dockerfile`、`apps/judge/Dockerfile`：

  ```dockerfile
  ARG GOPROXY=https://goproxy.cn,direct
  ENV GOPROXY=$GOPROXY
  ```

- `apps/web/Dockerfile`：

  ```dockerfile
  ARG NPM_REGISTRY=https://registry.npmmirror.com
  ENV npm_config_registry=$NPM_REGISTRY
  ENV COREPACK_NPM_REGISTRY=$NPM_REGISTRY
  ```

Compose 传递参数：

- `server.build.args.GOPROXY = ${KAPOJ_GOPROXY:-https://goproxy.cn,direct}`
- `judge.build.args.GOPROXY = ${KAPOJ_GOPROXY:-https://goproxy.cn,direct}`
- `web.build.args.NPM_REGISTRY = ${KAPOJ_NPM_REGISTRY:-https://registry.npmmirror.com}`

`.env.example` 增加 `KAPOJ_GOPROXY`、`KAPOJ_NPM_REGISTRY`，已有 `.env` 没有这些键时由 Compose 默认值兜底。手工 `docker compose build` 也自动走国内源（问题 5）。

### 5.4 Docker 镜像加速

更新 `DEFAULT_MIRRORS` 为 2026 年仍可用的候选，并在写入前探测：

1. 若 `KAPOJ_DOCKER_MIRRORS=off`，跳过。
2. 若 `/etc/docker/daemon.json` 已含 `registry-mirrors` 且未显式设置 `KAPOJ_DOCKER_MIRRORS`，只提示不改动。
3. 若显式设置了 `KAPOJ_DOCKER_MIRRORS`，先备份 `daemon.json`，再替换其中的 `registry-mirrors` 键；交互模式确认，`--yes` 直接执行。显式变量优先于旧配置。
4. 候选顺序：
   - 腾讯云元数据可达（`http://metadata.tencentyun.com/latest/meta-data/`）时，首位使用 `https://mirror.ccs.tencentyun.com`；
   - `https://docker.1ms.run`；
   - `https://docker.1panel.live`；
   - `https://hub.rat.dev`。
5. 对每个候选请求 `https://<mirror>/v2/`，`--connect-timeout 3 --max-time 5`，HTTP 状态为 `000` 的丢弃；全部不可用时警告并继续直连。
6. 默认在国内环境主动建议配置（不再只在 Docker Hub 探测失败时才提示）；`--yes` 直接应用，交互模式保留确认。
7. 用 Python 合并现有 `daemon.json`（保留其他键），备份为 `daemon.json.bak`。`python3` 加入 `ensure_base_packages()`；若仍不可用且 `daemon.json` 已存在，**终止并提示手工合并**，绝不用只含 `registry-mirrors` 的文件覆盖其他 Docker 配置。重启 Docker 后验证。

### 5.5 自定义证书

新增参数：

```bash
sudo bash deploy.sh install --domain oj.example.com --cert-crt /path/fullchain.pem --cert-key /path/privkey.pem
sudo bash deploy.sh update  --domain oj.example.com --cert-crt ... --cert-key ...
```

`deploy_custom_cert()`：

- 两个参数必须同时提供，且必须提供 `--domain`。
- 校验文件可读；用 `install -m 644` 复制证书到 `docker/caddy/certs/fullchain.pem`，`install -m 600` 复制私钥到 `docker/caddy/certs/privkey.pem`。
- 写入 `.env`：`KAPOJ_TLS_DIRECTIVE=tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem`。
- 不使用 `sudo tee`，避免问题 10 的终端策略限制。
- 打印“自有证书不会自动续期”的提醒。

`docker/caddy/Caddyfile` 站点块增加一行 `{$KAPOJ_TLS_DIRECTIVE}`；变量为空时展开为空行，不启用自定义证书。

`docker/docker-compose.prod.yml` 的 `web`：

- `environment` 增加 `KAPOJ_TLS_DIRECTIVE: "${KAPOJ_TLS_DIRECTIVE:-}"`。
- `volumes` 增加 `./caddy/certs:/etc/caddy/certs:ro`。

`wait_entry()` 默认校验证书链；失败时提示检查证书域名、链、DNS 和安全组。若源站使用 CDN 厂商的私有源站证书（如 Cloudflare Origin CA），可用 `--insecure-health` 或 `KAPOJ_TLS_VERIFY=0` 只跳过部署脚本的健康检查证书校验，不影响浏览器/CDN 的 TLS。

**已确认（CDN 回源模式）**：CDN 通过 HTTPS:443 回源，源站证书由公网 CA 签发。`wait_entry()` 默认校验证书链；`--insecure-health` 仅作为私有源站证书的备用开关，不用于当前 CDN 模式。

### 5.6 RBAC 管理员提升

新增子命令：

```bash
sudo bash deploy.sh promote <用户名>
```

`cmd_promote()`：

- 仅支持 Compose 内置的 `postgres` 服务。若 `.env` 中 `KAPOJ_DATABASE_URL` 非空（外部数据库），直接报错并给出手工 SQL，不静默更新本地未使用的容器。
- 不限制用户名格式（Hydro 导入的账号可能含点、连字符或非 ASCII）；用 psql 变量绑定并先检查用户存在，避免 SQL 注入。
- 通过 `docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'` 执行。
- 同一事务内：

  ```sql
  UPDATE users SET role = 'super_admin' WHERE username = :'username';
  INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
  SELECT u.id, r.id, NULL, now()
  FROM users u JOIN roles r ON r.code = 'super_admin'
  WHERE u.username = :'username'
  ON CONFLICT (user_id, role_id) DO NOTHING;
  ```

- 输出“请退出后重新登录”，因为权限来自 `user_roles`（问题 7）。
- `print_summary()` 改为推荐 `./deploy.sh promote <用户名>`，不再让用户手抄两条 SQL。

### 5.7 Ubuntu 26.04 兼容

- `check_os()`：`ID=ubuntu` 且 `VERSION_ID >= 20.04` 时通过，显示实际版本；低于 20.04 或非 Ubuntu 才警告。
- 新增 `check_arch()`：支持 `x86_64`/`aarch64`，其他架构直接报错。
- 脚本头部、用法和运行手册统一改为“Ubuntu 20.04+（含 26.04）”。
- Compose 插件路径差异由 5.1 处理（问题 8）。

### 5.8 失败日志与诊断

- 启动时设置 `KAPOJ_DEPLOY_LOG`（默认 `/tmp/kapoj-deploy-<时间戳>.log`），用 `tee` 同时写终端和日志。
- `trap ERR`：打印失败命令、退出码、日志路径、最近阶段提示：
  - `docker compose ps`
  - `docker compose logs --tail=100`
  - `docker info`
  - Compose 插件路径检查
- `build_up()` 先执行 `docker compose config --quiet` 校验变量，再 `build --progress=plain`、`up -d`。
- 所有 `docker pull`（socket-proxy、评测镜像）增加最多 3 次重试和明确失败镜像名。
- 健康检查失败时打印 `ps`、后端/前端日志和日志文件路径。

### 5.9 评测镜像预拉取

- 保留并行预拉取，但每个镜像最多重试 3 次。
- 失败只警告，不阻塞平台启动；提示对应语言首次评测会重新拉取。
- 支持用 `.env` 中的 `KAPOJ_JUDGE_*_IMAGE` 覆盖预拉取清单，避免预拉镜像与实际评测镜像不一致（问题 9）。

### 5.10 命令与文件变更

`deploy.sh`：

- 新增 `promote <用户名>`。
- 新增 `--cert-crt`、`--cert-key`、`--insecure-health`。
- `install`/`update` 调用：`check_os`、`check_arch`、`ensure_base_packages`、`install_docker`、`repair_compose_plugin`、`ensure_mirrors`、`ensure_env_symlink`、`deploy_custom_cert`。
- `COMPOSE` 显式 `--env-file`。

其他文件：

- `apps/server/Dockerfile`、`apps/judge/Dockerfile`：GOPROXY。
- `apps/web/Dockerfile`：npm registry。
- `docker/docker-compose.yml`、`docker/docker-compose.prod.yml`：构建参数、TLS 指令、证书挂载。
- `docker/caddy/Caddyfile`：TLS 指令占位。
- `.env.example`：`KAPOJ_GOPROXY`、`KAPOJ_NPM_REGISTRY`、`KAPOJ_TLS_DIRECTIVE`、`KAPOJ_TLS_VERIFY`。
- `运行手册.md`、`README.md`：Ubuntu 26.04、国内源、自定义证书、`promote`、故障排查。

`New_deploy.sh` 与 `deploy.sh` 当前完全一致；本设计只把 `deploy.sh` 作为部署入口，不把 `New_deploy.sh` 纳入版本控制。若你希望保留一份同步副本，可在实现后复制。

## 6. 测试与验收

本地可验证：

1. `bash -n deploy.sh`、`shellcheck deploy.sh`（若安装）。
2. `docker compose --env-file .env -f docker/docker-compose.yml -f docker/docker-compose.prod.yml config --quiet`。
3. 用 `caddy validate` 验证空/非空 `KAPOJ_TLS_DIRECTIVE` 两种 Caddyfile。
4. 构建 server/judge/web，确认 GOPROXY、npm registry 参数生效（看构建日志）。
5. 在 e2e PostgreSQL 上验证 `promote` SQL 的 `users.role` + `user_roles` 双写。
6. 验证 `docker/.env` 软链不会覆盖已有普通文件。
7. `git diff --check`、CI 的 lint/unit 不受影响。

服务器验收（Ubuntu 26.04，国内网络）：

1. 全新服务器执行 `sudo bash deploy.sh --yes`，Docker/Compose 安装成功，构建阶段无 Go/npm 超时，健康检查通过。
2. 存在损坏 `/usr/local/lib/docker/cli-plugins/docker-compose` 时，脚本能备份并回退到 `/usr/libexec` 插件。
3. 无 `.env` 时生成随机密钥；重复执行 install 不覆盖。
4. `sudo bash deploy.sh promote <用户名>` 后重新登录，管理员菜单和权限生效。
5. `--domain` + `--cert-crt/--cert-key` 后 HTTPS 可访问，Caddy 日志无 ACME 错误。
6. 失败场景（如断网拉镜像）能看到日志路径和下一步提示。

## 7. 风险与回滚

- 移动损坏/过旧 Compose 插件前保留 `.broken`/`.old` 备份，可手工恢复。
- `daemon.json` 修改前备份为 `daemon.json.bak`。
- 自定义证书复制到 gitignore 的 `docker/caddy/certs/`，不提交私钥。
- Dockerfile 的国内源默认值可被 `KAPOJ_GOPROXY`、`KAPOJ_NPM_REGISTRY` 覆盖；海外环境可改成官方源。
- 所有改动不涉及数据库迁移，回滚只需恢复旧脚本/镜像并重新 `deploy.sh update`。
