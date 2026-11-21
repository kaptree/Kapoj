# 国内网络部署加固（deploy.sh）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `deploy.sh` 在国内网络、Ubuntu 20.04–26.04 上完成可重复部署，覆盖 `question.txt` 的 10 个失败点。

**Architecture:** 把国内构建源写进 Dockerfile/Compose（源头可复现），把 Docker/Compose 安装修复、镜像加速、`.env` 加载、自定义证书、RBAC 提升和失败诊断放进 `deploy.sh`。部署脚本只做环境修复和编排，不在运行时改 Dockerfile。

**Tech Stack:** Bash、Docker Compose v2（≥2.24）、Caddy 2、PostgreSQL 17、Go 1.26、Node 22/pnpm 9。

**Spec:** `docs/superpowers/specs/2026-11-21-deploy-cn-hardening-design.md`

## Global Constraints

- 目标系统：Ubuntu 20.04/22.04/24.04/26.04 LTS，x86_64 或 aarch64；其他系统只警告，非 Ubuntu 不保证。
- Compose 必须 ≥ 2.24（现有 `!override` 语法依赖）。
- 已存在 Docker CE 时**绝不安装 `docker.io`**；只修/装 Compose 插件。
- 显式 `KAPOJ_DOCKER_MIRRORS` 优先于旧 `daemon.json`；未显式设置时不动旧 `registry-mirrors`。
- `daemon.json` 已存在但没有 `python3` 时终止并提示手工合并，绝不覆盖其他键。
- 仓库根 `.env` 是唯一应用配置源；`docker/.env -> ../.env` 只是手工 Compose 命令的兼容软链。
- `promote` 只支持内置 `postgres` 服务；检测到 `KAPOJ_DATABASE_URL` 非空时明确报错。
- 日志不得包含 `.env` 值、JWT、数据库密码、证书私钥。
- 重复执行幂等：已有 `.env`、证书、加速器配置不静默覆盖。
- CDN 模式已确认：HTTPS:443 回源 + 公网可信证书；`--insecure-health` 仅备用。
- 所有脚本修改保持 `set -euo pipefail`，不新增不可审计的第三方一键脚本。

## Review Focus

- **已装 Docker CE 时误装 `docker.io`**：apt 可能替换/移除 CE，导致 Docker 数据目录和服务异常。测试：`install_docker` 在 `command -v docker` 成功时不得出现 `apt-get install ... docker.io`。
- **`daemon.json` 有其他键但无 Python**：覆盖会丢 `data-root`/`log-driver`。测试：构造含 `data-root` 的 `daemon.json`，模拟无 `python3`，函数必须失败且文件不变。
- **`docker/.env` 已有普通文件**：不能覆盖用户配置。测试：放一个普通文件，`ensure_env_symlink` 只警告、内容不变。
- **空 `KAPOJ_TLS_DIRECTIVE`**：Caddyfile 必须仍然有效。测试：`caddy validate` 空值/非空值两种都通过。
- **导入用户名/外部数据库的 `promote`**：用户名含 `.`/`-`/非 ASCII 时必须能提升；`KAPOJ_DATABASE_URL` 非空时必须拒绝并给出手工 SQL，不能静默改本地库。

---

### Task 1: 国内构建源与 Compose 构建参数

**Files:**
- Modify: `apps/server/Dockerfile`
- Modify: `apps/judge/Dockerfile`
- Modify: `apps/web/Dockerfile`
- Modify: `docker/docker-compose.yml`
- Modify: `docker/docker-compose.prod.yml`
- Modify: `.env.example`

**Interfaces:**
- Produces: 构建参数 `GOPROXY`（server/judge）、`NPM_REGISTRY`（web）；Compose 变量 `KAPOJ_GOPROXY`、`KAPOJ_NPM_REGISTRY`。

- [ ] **Step 1: 给 Go 镜像加 GOPROXY**

在 `apps/server/Dockerfile` 和 `apps/judge/Dockerfile` 的 `FROM golang:1.26 AS build` 之后、`WORKDIR` 之前加入：

```dockerfile
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY
```

- [ ] **Step 2: 给 Web 镜像加 npm 源**

在 `apps/web/Dockerfile` 的 `FROM node:22-alpine AS build` 之后加入：

```dockerfile
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV npm_config_registry=$NPM_REGISTRY
ENV COREPACK_NPM_REGISTRY=$NPM_REGISTRY
```

- [ ] **Step 3: Compose 传递参数**

`docker/docker-compose.yml` 的 `server.build` 和 `judge.build` 各加：

```yaml
      args:
        GOPROXY: ${KAPOJ_GOPROXY:-https://goproxy.cn,direct}
```

`docker/docker-compose.prod.yml` 的 `web.build` 加：

```yaml
      args:
        NPM_REGISTRY: ${KAPOJ_NPM_REGISTRY:-https://registry.npmmirror.com}
```

- [ ] **Step 4: `.env.example` 增加可覆盖项**

在 Judge 配置附近加入：

```bash
# ---------- 构建源（国内默认；海外可改回官方源） ----------
KAPOJ_GOPROXY=https://goproxy.cn,direct
KAPOJ_NPM_REGISTRY=https://registry.npmmirror.com
```

- [ ] **Step 5: 验证 Compose 配置**

Run:

```bash
docker compose --env-file .env.example \
  -f docker/docker-compose.yml \
  -f docker/docker-compose.prod.yml \
  --profile app config --quiet
```

Expected: 退出码 0，无 “required variable” 报错。

- [ ] **Step 6: 验证构建参数生效**

Run:

```bash
docker build --target build -t kapoj-server-buildtest apps/server
docker run --rm kapoj-server-buildtest go env GOPROXY
docker build --target build -t kapoj-web-buildtest -f apps/web/Dockerfile .
docker run --rm kapoj-web-buildtest printenv npm_config_registry
```

Expected: 分别输出 `https://goproxy.cn,direct` 和 `https://registry.npmmirror.com`。若本机网络无法构建，记录失败原因并在服务器验收时补跑。

- [ ] **Step 7: Commit**

```bash
git add apps/server/Dockerfile apps/judge/Dockerfile apps/web/Dockerfile \
  docker/docker-compose.yml docker/docker-compose.prod.yml .env.example
git commit -m "feat(deploy): default builds to China Go/npm mirrors"
```

---

### Task 2: deploy.sh 可测试骨架与 .env 辅助函数

**Files:**
- Modify: `deploy.sh`
- Create: `scripts/test-deploy-lib.sh`

**Interfaces:**
- Produces: `KAPOJ_DEPLOY_LIB_ONLY=1` 只加载函数不执行 `main`；`$ENV_FILE`、`$ROOT` 全局；`set_env_key`、`env_get`、`ensure_env_symlink`。
- Consumes: 无。

- [ ] **Step 1: 写失败测试**

创建 `scripts/test-deploy-lib.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok - $*"; }

export KAPOJ_DEPLOY_LIB_ONLY=1
# shellcheck disable=SC1091
. "$ROOT_DIR/deploy.sh"

ROOT="$TMP"
ENV_FILE="$TMP/.env"

# 1. update existing key
printf 'FOO=old\nBAR=keep\n' > "$ENV_FILE"
set_env_key FOO new
grep -qx 'FOO=new' "$ENV_FILE" || fail "set_env_key did not update FOO"
grep -qx 'BAR=keep' "$ENV_FILE" || fail "set_env_key dropped BAR"
pass "set_env_key updates existing key"

# 2. append missing key
set_env_key NEW_KEY value
grep -qx 'NEW_KEY=value' "$ENV_FILE" || fail "set_env_key did not append NEW_KEY"
pass "set_env_key appends missing key"

# 3. env_get
[ "$(env_get FOO)" = "new" ] || fail "env_get FOO"
[ "$(env_get MISSING)" = "" ] || fail "env_get missing should be empty"
pass "env_get reads values"

# 4. symlink creation
mkdir -p "$TMP/docker"
ensure_env_symlink
[ -L "$TMP/docker/.env" ] || fail "ensure_env_symlink did not create symlink"
[ "$(readlink "$TMP/docker/.env")" = "../.env" ] || fail "wrong symlink target"
pass "ensure_env_symlink creates ../.env"

# 5. never overwrite a regular file
rm "$TMP/docker/.env"
printf 'user-owned\n' > "$TMP/docker/.env"
ensure_env_symlink
[ ! -L "$TMP/docker/.env" ] || fail "ensure_env_symlink replaced regular file"
[ "$(cat "$TMP/docker/.env")" = "user-owned" ] || fail "regular file content changed"
pass "ensure_env_symlink preserves regular file"

echo "all deploy helper tests passed"
```

- [ ] **Step 2: 运行测试确认失败**

Run: `bash scripts/test-deploy-lib.sh`

Expected: FAIL，因为 `KAPOJ_DEPLOY_LIB_ONLY` 未处理，脚本会执行 `main`，或函数不存在。

- [ ] **Step 3: 加只加载函数的守卫**

在 `deploy.sh` 底部 `main "$@"` 之前加入：

```bash
if [ "${KAPOJ_DEPLOY_LIB_ONLY:-0}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi
```

- [ ] **Step 4: 引入 `$ROOT`/`$ENV_FILE` 并替换 `.env` 字面量**

在 `cd` 之后加：

```bash
ROOT="$(pwd)"
ENV_FILE="$ROOT/.env"
```

把 `load_entry_settings`、`gen_env`、`set_env_key` 等函数中的 `.env` 改为 `"$ENV_FILE"`。`set_env_key` 改为：

```bash
set_env_key() { # $1=键 $2=值
  if grep -q "^$1=" "$ENV_FILE" 2>/dev/null; then
    sed -i.bak "s|^$1=.*|$1=$2|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  else
    printf '%s=%s\n' "$1" "$2" >> "$ENV_FILE"
  fi
}
```

- [ ] **Step 5: 实现 `env_get` 与 `ensure_env_symlink`**

```bash
env_get() { # $1=键
  grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- || true
}

ensure_env_symlink() {
  local link="$ROOT/docker/.env"
  if [ -L "$link" ]; then
    return 0
  fi
  if [ -e "$link" ]; then
    warn "$link 已存在且不是软链，保留用户文件不动"
    return 0
  fi
  mkdir -p "$ROOT/docker"
  ln -s ../.env "$link"
  ok "已创建 $link -> ../.env"
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `bash scripts/test-deploy-lib.sh`

Expected: `all deploy helper tests passed`。

- [ ] **Step 7: Commit**

```bash
git add deploy.sh scripts/test-deploy-lib.sh
git commit -m "refactor(deploy): make helpers testable and fix .env access"
```

---

### Task 3: Ubuntu 26.04、Docker/Compose 安装与插件修复

**Files:**
- Modify: `deploy.sh`
- Modify: `scripts/test-deploy-lib.sh`

**Interfaces:**
- Produces: `check_os`、`check_arch`、`ensure_base_packages`、`compose_ok`、`repair_compose_plugin`、`install_docker`、`init_log`、`on_error`。
- Consumes: Task 2 的 `$ROOT`、`$ENV_FILE`、`warn/ok/die`。

- [ ] **Step 1: 写失败测试**

在 `scripts/test-deploy-lib.sh` 末尾加：

```bash
# 6. OS release parsing
printf 'ID=ubuntu\nVERSION_ID="26.04"\nPRETTY_NAME="Ubuntu 26.04 LTS"\n' > "$TMP/os-release"
KAPOJ_OS_RELEASE_FILE="$TMP/os-release" check_os
pass "check_os accepts Ubuntu 26.04"

# 7. plugin repair: broken and old binaries moved aside
PLUGINS="$TMP/plugins"
mkdir -p "$PLUGINS"
printf '#!/bin/sh\nexit 139\n' > "$PLUGINS/docker-compose"
chmod +x "$PLUGINS/docker-compose"
KAPOJ_COMPOSE_PLUGIN_DIRS="$PLUGINS" repair_compose_plugin
[ ! -e "$PLUGINS/docker-compose" ] || fail "broken plugin not moved"
ls "$PLUGINS"/docker-compose.broken.* >/dev/null 2>&1 || fail "broken backup missing"
pass "repair_compose_plugin moves broken binary"

printf '#!/bin/sh\n[ "$1" = version ] && echo v2.20.0\n' > "$PLUGINS/docker-compose"
chmod +x "$PLUGINS/docker-compose"
KAPOJ_COMPOSE_PLUGIN_DIRS="$PLUGINS" repair_compose_plugin
[ ! -e "$PLUGINS/docker-compose" ] || fail "old plugin not moved"
ls "$PLUGINS"/docker-compose.old.* >/dev/null 2>&1 || fail "old backup missing"
pass "repair_compose_plugin moves old binary"

echo "all deploy helper tests passed"
```

- [ ] **Step 2: 运行测试确认失败**

Run: `bash scripts/test-deploy-lib.sh`

Expected: FAIL，`check_os`/`repair_compose_plugin` 未实现或行为不符。

- [ ] **Step 3: 更新 `check_os` 与新增 `check_arch`**

`check_os` 读取 `${KAPOJ_OS_RELEASE_FILE:-/etc/os-release}`；`ID=ubuntu` 且 `version_ge "$VERSION_ID" "20.04"` 时 `ok`，否则 `warn`。`check_arch` 只接受 `x86_64`/`aarch64`，其他 `die`。

- [ ] **Step 4: 新增基础包与 Compose 辅助函数**

```bash
apt_get() {
  DEBIAN_FRONTEND=noninteractive apt-get \
    -o Acquire::Retries=3 \
    -o Acquire::http::Timeout=30 \
    -o Acquire::https::Timeout=30 \
    -y "$@"
}

ensure_base_packages() {
  command -v apt-get >/dev/null 2>&1 || die "本脚本需要 apt-get（Ubuntu/Debian）"
  apt_get update || warn "apt-get update 失败，继续尝试使用现有索引"
  apt_get install ca-certificates curl gnupg openssl python3
}

compose_version() { docker compose version --short 2>/dev/null | sed 's/^v//' || true; }
compose_ok() {
  local v
  v="$(compose_version)"
  [ -n "$v" ] && version_ge "$v" "$MIN_COMPOSE_VERSION"
}
```

- [ ] **Step 5: 实现 `repair_compose_plugin`**

扫描目录时优先使用 `KAPOJ_COMPOSE_PLUGIN_DIRS`（测试用），否则使用 `/usr/local/lib/docker/cli-plugins`、`/usr/local/libexec/docker/cli-plugins`、`/usr/lib/docker/cli-plugins`、`/usr/libexec/docker/cli-plugins`、`${DOCKER_CONFIG:-$HOME/.docker}/cli-plugins`。对每个 `docker-compose`：

- `timeout 5 "$f" version` 失败 → `mv "$f" "$f.broken.$(date +%s)"`。macOS 本地测试没有 `timeout` 时直接执行 `"$f" version`（测试用的损坏脚本会立即退出）。
- 版本 < 2.24 → `mv "$f" "$f.old.$(date +%s)"`。
- `/usr/libexec/docker/cli-plugins/docker-compose` 存在且 `/usr/lib/docker/cli-plugins/docker-compose` 不存在时创建软链。
- 不在此函数内调用 `docker compose`；版本检查留给 `install_docker`。

- [ ] **Step 6: 重写 `install_docker`**

顺序：

1. `ensure_base_packages`。
2. 若 `command -v docker`：`repair_compose_plugin`；`compose_ok` 则返回；否则只装 Docker 官方仓库插件 `apt_get install docker-compose-plugin`（**不要装 `docker-compose-v2`**，它可能拉入 `docker.io` 替换 Docker CE）；不可用时走第 4 步独立二进制；再 `repair_compose_plugin`。
3. 若 Docker 不存在：`apt_get install docker.io docker-compose-v2`；失败再 `curl -fsSL https://get.docker.com | sh`；然后补 Compose 包。
4. 仍不可用：按 `uname -m` 下载 `${KAPOJ_COMPOSE_URL:-https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$ARCH}` 到 `/usr/local/lib/docker/cli-plugins/docker-compose`，`chmod +x`，执行 `docker-compose version` 验证。
5. `systemctl enable/restart docker` 或 `service` 回退；最后 `compose_ok || die`，列出扫描路径和版本。

- [ ] **Step 7: 加日志与 ERR trap**

```bash
LOG_FILE="${KAPOJ_DEPLOY_LOG:-$(mktemp /tmp/kapoj-deploy-XXXXXX.log)}"
ERROR_REPORTED=0
on_error() {
  local code=$?
  [ "$ERROR_REPORTED" -eq 0 ] || return 0
  ERROR_REPORTED=1
  warn "部署失败（退出码 $code，命令：$BASH_COMMAND）"
  warn "完整日志：$LOG_FILE"
  warn "排查：docker compose ps；docker compose logs --tail=100；docker info"
}
```

`set -Eeuo pipefail`。在 `main` 解析参数后、执行命令前：

```bash
exec > >(tee -a "$LOG_FILE") 2>&1
trap 'on_error' ERR
```

- [ ] **Step 8: 更新 install/update 调用**

`cmd_install` 开头调用 `check_os`、`check_arch`、`install_docker`、`ensure_env_symlink`（`install_docker` 内部负责 `ensure_base_packages`）。`cmd_update` 同样调用（`ensure_env_symlink`、`install_docker` 已在则快速返回）。

- [ ] **Step 9: 运行测试**

Run:

```bash
bash -n deploy.sh
bash scripts/test-deploy-lib.sh
command -v shellcheck >/dev/null && shellcheck deploy.sh || true
```

Expected: 语法通过，helper 测试全过。

- [ ] **Step 10: Commit**

```bash
git add deploy.sh scripts/test-deploy-lib.sh
git commit -m "fix(deploy): support Ubuntu 26.04 and repair Docker Compose plugins"
```

---

### Task 4: 镜像加速、重试与诊断

**Files:**
- Modify: `deploy.sh`
- Modify: `scripts/test-deploy-lib.sh`

**Interfaces:**
- Produces: `parse_mirrors`、`mirror_reachable`、`retry`、重写 `ensure_mirrors`、`ensure_socket_proxy`、`pull_judge_images`。
- Consumes: Task 3 的 `apt_get`、`compose_ok`、日志。

- [ ] **Step 1: 写失败测试**

在 `scripts/test-deploy-lib.sh` 末尾加：

```bash
# 8. mirror parsing accepts commas and spaces
parse_mirrors 'https://a.example,https://b.example https://c.example'
[ "${#MIRRORS[@]}" -eq 3 ] || fail "parse_mirrors count"
[ "${MIRRORS[0]}" = "https://a.example" ] || fail "parse_mirrors order"
pass "parse_mirrors handles commas and spaces"
```

- [ ] **Step 2: 运行测试确认失败**

Run: `bash scripts/test-deploy-lib.sh`

Expected: FAIL，`parse_mirrors` 未定义。

- [ ] **Step 3: 实现镜像函数**

```bash
parse_mirrors() {
  local raw="${1//,/ }"
  # shellcheck disable=SC2206
  MIRRORS=($raw)
}

mirror_reachable() {
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' \
    --connect-timeout 3 --max-time 5 "${1%/}/v2/" || true)"
  case "$code" in
    2*|3*|401|403) return 0 ;;
    *) return 1 ;;
  esac
}

retry() { # $1=次数，其余=命令
  local attempts="$1" n=1
  shift
  until "$@"; do
    if [ "$n" -ge "$attempts" ]; then return 1; fi
    warn "命令失败，5 秒后重试（$n/$attempts）：$*"
    sleep 5
    n=$((n + 1))
  done
}
```

- [ ] **Step 4: 更新默认镜像与 `ensure_mirrors`**

`DEFAULT_MIRRORS=(https://docker.1ms.run https://docker.1panel.live https://hub.rat.dev)`。腾讯云元数据可达时把 `https://mirror.ccs.tencentyun.com` 放首位。

`ensure_mirrors`：

1. `KAPOJ_DOCKER_MIRRORS=off` → 跳过。
2. 无显式变量且 `daemon.json` 已有 `registry-mirrors` → 提示后返回。
3. 解析候选；逐个 `mirror_reachable` 过滤。
4. 全部不可达 → 警告并返回。
5. 显式变量或确认后：备份 `daemon.json`；用 Python 合并/替换 `registry-mirrors`。Python 解析失败同样 `die`，文件不动。
6. `daemon.json` 存在但无 `python3` → `die`，文件不动。
7. `systemctl restart docker`，`sleep 2`，`docker info` 验证。

- [ ] **Step 5: 重试与评测镜像覆盖**

`ensure_socket_proxy` 用 `retry 3 docker pull "$SOCKET_PROXY_IMAGE"`。`pull_judge_images` 从 `.env` 读取 `KAPOJ_JUDGE_CPP_IMAGE`、`KAPOJ_JUDGE_PYTHON_IMAGE`、`KAPOJ_JUDGE_JAVA_IMAGE`、`KAPOJ_JUDGE_GO_IMAGE`，缺失时用默认四个；并行拉取时每个 `retry 3 docker pull`。

- [ ] **Step 6: 健康检查诊断**

`wait_health` 失败时打印 `"${COMPOSE[@]}" ps`、`logs --tail=100`，以及日志路径。`build_up` 先 `"${COMPOSE[@]}" config --quiet`，再 `build --progress=plain`、`up -d`。

- [ ] **Step 7: 运行测试**

Run:

```bash
bash -n deploy.sh
bash scripts/test-deploy-lib.sh
```

Expected: 全过；`parse_mirrors` 测试通过。

- [ ] **Step 8: Commit**

```bash
git add deploy.sh scripts/test-deploy-lib.sh
git commit -m "fix(deploy): refresh mirrors, add pull retries and diagnostics"
```

---

### Task 5: 自定义证书

**Files:**
- Modify: `deploy.sh`
- Modify: `docker/caddy/Caddyfile`
- Modify: `docker/docker-compose.prod.yml`
- Modify: `.env.example`
- Modify: `scripts/test-deploy-lib.sh`

**Interfaces:**
- Produces: `deploy_custom_cert`、flags `--cert-crt`/`--cert-key`/`--insecure-health`、`.env` 键 `KAPOJ_TLS_DIRECTIVE`/`KAPOJ_TLS_VERIFY`。
- Consumes: Task 2 `set_env_key`/`env_get`，Task 3 `die`。

- [ ] **Step 1: 写失败测试**

在 `scripts/test-deploy-lib.sh` 末尾加：

```bash
# 9. custom cert copies files and writes directive
mkdir -p "$TMP/docker/caddy/certs"
printf 'cert\n' > "$TMP/fullchain.pem"
printf 'key\n' > "$TMP/privkey.pem"
printf 'KAPOJ_DOMAIN=\nKAPOJ_TLS_DIRECTIVE=\nKAPOJ_TLS_VERIFY=1\n' > "$ENV_FILE"
DOMAIN=oj.example.com
CERT_CRT="$TMP/fullchain.pem"
CERT_KEY="$TMP/privkey.pem"
deploy_custom_cert
[ "$(cat "$TMP/docker/caddy/certs/fullchain.pem")" = "cert" ] || fail "cert not copied"
[ "$(cat "$TMP/docker/caddy/certs/privkey.pem")" = "key" ] || fail "key not copied"
grep -q '^KAPOJ_TLS_DIRECTIVE=tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem$' "$ENV_FILE" \
  || fail "TLS directive not written"
pass "deploy_custom_cert copies files and writes directive"
```

- [ ] **Step 2: 运行测试确认失败**

Run: `bash scripts/test-deploy-lib.sh`

Expected: FAIL，`deploy_custom_cert` 未定义。

- [ ] **Step 3: Caddyfile 与 Compose**

`docker/caddy/Caddyfile` 站点块内加一行 `{$KAPOJ_TLS_DIRECTIVE}`（在 `encode` 之前）。

`docker/docker-compose.prod.yml` 的 `web.environment` 加：

```yaml
      KAPOJ_TLS_DIRECTIVE: "${KAPOJ_TLS_DIRECTIVE:-}"
```

`web.volumes` 加：

```yaml
      - ./caddy/certs:/etc/caddy/certs:ro
```

`.env.example` 的 HTTPS/站点配置附近加：

```bash
# 自定义证书：由 deploy.sh --cert-crt/--cert-key 写入；留空则 Caddy 自动 HTTPS
KAPOJ_TLS_DIRECTIVE=
# 1=部署健康检查校验证书；0=跳过（仅私有源站证书）
KAPOJ_TLS_VERIFY=1
```

- [ ] **Step 4: 实现 `deploy_custom_cert`**

```bash
deploy_custom_cert() {
  if [ "${TLS_VERIFY:-1}" = "0" ]; then
    set_env_key KAPOJ_TLS_VERIFY 0
  fi
  if [ -z "${CERT_CRT:-}" ] && [ -z "${CERT_KEY:-}" ]; then
    return 0
  fi
  [ -n "${DOMAIN:-}" ] || die "--cert-crt/--cert-key 需要同时提供 --domain"
  [ -n "${CERT_CRT:-}" ] && [ -n "${CERT_KEY:-}" ] \
    || die "--cert-crt 与 --cert-key 必须同时提供"
  [ -r "$CERT_CRT" ] || die "证书不可读：$CERT_CRT"
  [ -r "$CERT_KEY" ] || die "私钥不可读：$CERT_KEY"

  local dir="$ROOT/docker/caddy/certs"
  mkdir -p "$dir"
  cmp -s "$CERT_CRT" "$dir/fullchain.pem" || install -m 644 "$CERT_CRT" "$dir/fullchain.pem"
  cmp -s "$CERT_KEY" "$dir/privkey.pem" || install -m 600 "$CERT_KEY" "$dir/privkey.pem"
  set_env_key KAPOJ_TLS_DIRECTIVE \
    "tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem"
  ok "已部署自定义证书；证书不会自动续期"
}
```

`DOMAIN`、`CERT_CRT`、`CERT_KEY`、`TLS_VERIFY` 在参数解析中赋值。`--insecure-health` 设置 `TLS_VERIFY=0` 并 `set_env_key KAPOJ_TLS_VERIFY 0`。

- [ ] **Step 5: `wait_entry` 支持跳过证书校验**

`load_entry_settings` 读取 `KAPOJ_TLS_VERIFY`（默认 1）到 `TLS_VERIFY`。为 0 时 HTTPS curl 加 `-k`，并打印警告。

- [ ] **Step 6: install/update 调用**

`cmd_install`、`cmd_update` 在 `gen_env`/`ensure_env_symlink` 之后调用 `deploy_custom_cert`。

- [ ] **Step 7: 验证 Caddyfile 与 Compose**

Run:

```bash
docker run --rm -e KAPOJ_SITE_ADDRESS=':80' -e KAPOJ_TLS_DIRECTIVE='' \
  -v "$PWD/docker/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
docker compose --env-file .env.e2e -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app config --quiet
bash scripts/test-deploy-lib.sh
```

Expected: `Valid configuration`；测试全过。

- [ ] **Step 8: Commit**

```bash
git add deploy.sh docker/caddy/Caddyfile docker/docker-compose.prod.yml .env.example scripts/test-deploy-lib.sh
git commit -m "feat(deploy): support custom TLS certificates"
```

---

### Task 6: promote 管理员命令

**Files:**
- Modify: `deploy.sh`
- Modify: `scripts/test-deploy-lib.sh`

**Interfaces:**
- Produces: `cmd_promote`、子命令 `promote <用户名>`；复用 `COMPOSE`。
- Consumes: Task 3 `COMPOSE`；Task 2 `env_get`。

- [ ] **Step 1: 写失败测试**

在 `scripts/test-deploy-lib.sh` 末尾加：

```bash
# 10. external database is refused
printf 'KAPOJ_DATABASE_URL=postgres://external/db\n' > "$ENV_FILE"
if ( cmd_promote someone >/dev/null 2>&1 ); then
  fail "promote should refuse KAPOJ_DATABASE_URL"
fi
pass "promote refuses external database"
```

- [ ] **Step 2: 运行测试确认失败**

Run: `bash scripts/test-deploy-lib.sh`

Expected: FAIL，`cmd_promote` 未定义或未拒绝。

- [ ] **Step 3: 实现 `cmd_promote`**

```bash
cmd_promote() {
  local user="${1:-}"
  [ -n "$user" ] || die "用法：sudo bash deploy.sh promote <用户名>"
  [ -f "$ENV_FILE" ] || die "缺少 .env，请先运行 install"
  if [ -n "$(env_get KAPOJ_DATABASE_URL)" ]; then
    die "当前使用外部数据库（KAPOJ_DATABASE_URL），promote 只支持内置 postgres。请手工执行文档中的 SQL。"
  fi

  "${COMPOSE[@]}" exec -T postgres sh -c '
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -v username="$1"
  ' sh "$user" <<'SQL'
SELECT id FROM users WHERE username = :'username' \gset
\if :{?id}
\else
  \echo '用户不存在或用户名为空'
  \quit 1
\endif
UPDATE users SET role = 'super_admin' WHERE id = :id;
INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
VALUES (:id, (SELECT id FROM roles WHERE code = 'super_admin'), NULL, now())
ON CONFLICT (user_id, role_id) DO NOTHING;
\echo '已授予 super_admin，请退出后重新登录'
SQL
}
```

- [ ] **Step 4: 接入命令解析与 usage**

`main` 的 case 增加 `promote`，并把第一个位置参数传给 `cmd_promote`。usage 增加示例。`print_summary` 改为推荐 `./deploy.sh promote <用户名>`，保留手工 SQL 作为外部数据库备注。

- [ ] **Step 5: 在 e2e 数据库验证 SQL**

Run:

```bash
bash scripts/e2e-stack.sh up
docker compose -p kapoj-e2e --env-file .env.e2e \
  -f docker/docker-compose.yml -f docker/docker-compose.prod.yml -f docker/docker-compose.e2e.yml \
  exec -T postgres psql -U kapoj -d kapoj -v ON_ERROR_STOP=1 -v username=m2bsolver1 <<'SQL'
SELECT id FROM users WHERE username = :'username' \gset
UPDATE users SET role = 'super_admin' WHERE id = :id;
INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at)
VALUES (:id, (SELECT id FROM roles WHERE code = 'super_admin'), NULL, now())
ON CONFLICT (user_id, role_id) DO NOTHING;
SQL
docker compose -p kapoj-e2e --env-file .env.e2e \
  -f docker/docker-compose.yml -f docker/docker-compose.prod.yml -f docker/docker-compose.e2e.yml \
  exec -T postgres psql -U kapoj -d kapoj -tAc \
  "SELECT u.username, u.role, r.code FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE u.username='m2bsolver1'"
```

Expected: 输出 `m2bsolver1|super_admin|super_admin`。测试结束后用 `down -v` 清理 e2e 栈。

- [ ] **Step 6: 运行 helper 测试**

Run: `bash scripts/test-deploy-lib.sh`

Expected: 全过，包括外部数据库拒绝。

- [ ] **Step 7: Commit**

```bash
git add deploy.sh scripts/test-deploy-lib.sh
git commit -m "feat(deploy): add promote command for RBAC super admin"
```

---

### Task 7: 文档与全量验证

**Files:**
- Modify: `运行手册.md`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-11-21-deploy-cn-hardening-design.md`（如实现中发现偏差）

**Interfaces:**
- Consumes: Tasks 1–6。

- [ ] **Step 1: 更新运行手册 §11**

- 标题改为“生产部署（Ubuntu 20.04+，含 26.04）”。
- 增加 `promote` 命令和“重新登录”说明。
- 增加 `--cert-crt/--cert-key/--insecure-health` 示例。
- 增加国内构建源、镜像加速、Compose 插件路径、`.env` 软链的排查表。
- 把“手工 SQL 授管理员”改为推荐 `./deploy.sh promote`；外部数据库保留手工 SQL。

- [ ] **Step 2: 更新 README 部署段落**

- 说明支持 Ubuntu 20.04–26.04。
- 增加 `sudo bash deploy.sh promote <用户名>`。
- 增加自定义证书一行示例。
- 指向运行手册 §11。

- [ ] **Step 3: 全量验证**

Run:

```bash
bash -n deploy.sh
bash scripts/test-deploy-lib.sh
docker compose --env-file .env.example -f docker/docker-compose.yml -f docker/docker-compose.prod.yml --profile app config --quiet
docker run --rm -e KAPOJ_SITE_ADDRESS=':80' -e KAPOJ_TLS_DIRECTIVE='' \
  -v "$PWD/docker/caddy/Caddyfile:/etc/caddy/Caddyfile:ro" \
  caddy:2-alpine caddy validate --config /etc/caddy/Caddyfile
corepack pnpm lint:web
corepack pnpm build:web
git diff --check
```

Expected: 全部通过。若修改了 Dockerfile，再跑一次 server/judge/web 构建（Task 1 Step 6）。

- [ ] **Step 4: Commit**

```bash
git add 运行手册.md README.md docs/superpowers/specs/2026-11-21-deploy-cn-hardening-design.md
git commit -m "docs(deploy): document China-network deployment hardening"
```

---

## 自检记录

- 规格覆盖：10 个问题分别落在 Task 1（Go/npm）、Task 2（.env）、Task 3（Docker/Compose/Ubuntu）、Task 4（镜像/重试）、Task 5（证书）、Task 6（RBAC）、Task 7（文档/验收）。
- 类型/名称：`ENV_FILE`、`ROOT`、`set_env_key`、`env_get`、`ensure_env_symlink`、`compose_ok`、`repair_compose_plugin`、`deploy_custom_cert`、`cmd_promote` 在各任务中保持一致。
- 测试钩子：`KAPOJ_DEPLOY_LIB_ONLY`、`KAPOJ_OS_RELEASE_FILE`、`KAPOJ_COMPOSE_PLUGIN_DIRS` 仅用于本地/CI 测试，不改变生产行为。
