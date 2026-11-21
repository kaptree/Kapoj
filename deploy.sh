#!/usr/bin/env bash
# kapOJ 一键部署脚本（目标系统：Ubuntu 20.04+ LTS，含 26.04）
#
# 用法：
#   sudo bash deploy.sh [install] [--domain oj.example.com] [--yes] [--skip-images]
#   sudo bash deploy.sh install|update --domain <域名> --cert-crt <证书> --cert-key <私钥>
#   sudo bash deploy.sh update [--pull]
#   sudo bash deploy.sh promote <用户名>
#   sudo bash deploy.sh status | logs [service...] | stop | restart
#
# install：安装 Docker（含国内镜像加速）→ 生成 .env（随机密钥）→ 构建启动全部服务
#          → 拉取评测语言镜像 → 健康检查 → 打印访问地址与管理员引导。
# 重复执行 install 幂等：已存在的 .env 不会被覆盖或修改。
#
# 环境变量覆盖（sudo 会重置环境，赋值必须放在 sudo 之后，例如：
#   sudo KAPOJ_DOCKER_MIRRORS="https://你的加速地址" bash deploy.sh
#   sudo KAPOJ_DOCKER_MIRRORS=off bash deploy.sh）：
#   KAPOJ_DOCKER_MIRRORS  空格分隔的镜像加速地址；设为 off 跳过加速配置
#   KAPOJ_COMPOSE_URL     compose v2 独立二进制下载地址（回退安装分支）
#   KAPOJ_JUDGE_DOCKER_DIRECT=1  跳过 socket-proxy，judge 直连 Docker socket（仅故障回退）
#
# 详细说明见 运行手册.md §11（生产部署）。
set -Eeuo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(pwd)"
ENV_FILE="$ROOT/.env"

# ---------- 常量 ----------
BASE_COMPOSE="docker/docker-compose.yml"
PROD_COMPOSE="docker/docker-compose.prod.yml"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "$BASE_COMPOSE" -f "$PROD_COMPOSE" --profile app)
DIRECT_COMPOSE="docker/docker-compose.direct.yml"
SOCKET_PROXY_IMAGE="tecnativa/docker-socket-proxy:lts@sha256:be1ac64f33bd82043beb504822a17ba6b71668ea1ad7cf51a9a01d27bb7ab047"
JUDGE_IMAGES=(gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26)
# 镜像加速清单核对日期：2026-11-15（公共加速器可用性会变化，失效时用 KAPOJ_DOCKER_MIRRORS 覆盖，
# 或改用阿里云/腾讯云控制台分配的专属加速地址）
DEFAULT_MIRRORS=(https://docker.1ms.run https://docker.1panel.live https://hub.rat.dev)
MIN_COMPOSE_VERSION="2.24"
HEALTH_URL="http://127.0.0.1:8080/health"

DOMAIN=""
CERT_CRT=""
CERT_KEY=""
TLS_VERIFY=1
FLAG_YES=0
FLAG_SKIP_IMAGES=0
FLAG_PULL=0
FLAG_INSECURE_HEALTH=0

# ---------- 输出 ----------
if [ -t 1 ]; then
  C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_BLUE=$'\033[34m'; C_BOLD=$'\033[1m'; C_OFF=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_OFF=""
fi
STEP_NO=0
STEP_TOTAL=9

log()  { printf '%s\n' "${C_BLUE}    $*${C_OFF}"; }
ok()   { printf '%s\n' "${C_GREEN} ✔  $*${C_OFF}"; }
warn() { printf '%s\n' "${C_YELLOW} ⚠  $*${C_OFF}" >&2; }
die()  { printf '%s\n' "${C_RED} ✘  $*${C_OFF}" >&2; exit 1; }
step() { STEP_NO=$((STEP_NO + 1)); printf '%s\n' "${C_BOLD}[ STEP $STEP_NO/$STEP_TOTAL ] $*${C_OFF}"; }

LOG_FILE="${KAPOJ_DEPLOY_LOG:-}"
ERROR_REPORTED=0

init_log() {
  [ -n "$LOG_FILE" ] || LOG_FILE="$(mktemp /tmp/kapoj-deploy-XXXXXX.log)"
  exec > >(tee -a "$LOG_FILE") 2>&1
  trap 'on_error' ERR
}

on_error() {
  local code=$?
  [ "$ERROR_REPORTED" -eq 0 ] || return 0
  ERROR_REPORTED=1
  warn "部署失败（退出码 ${code}，命令：${BASH_COMMAND}）"
  warn "完整日志：$LOG_FILE"
  warn "排查：docker compose ps；docker compose logs --tail=100；docker info"
}

# ---------- 前置检查 ----------
need_root() {
  [ "$(id -u)" -eq 0 ] || die "请用 root 运行：sudo bash deploy.sh $*"
}

check_os() {
  local os_release="${KAPOJ_OS_RELEASE_FILE:-/etc/os-release}"
  if [ -r "$os_release" ]; then
    # shellcheck disable=SC1090
    . "$os_release"
    if [ "${ID:-}" = "ubuntu" ] && version_ge "${VERSION_ID:-0}" "20.04"; then
      ok "检测到 Ubuntu ${VERSION_ID} LTS（${PRETTY_NAME:-}）"
    else
      warn "当前系统为 ${PRETTY_NAME:-未知}，本脚本按 Ubuntu 20.04+ 设计；将尽力而为，遇到问题参见运行手册 §11 手工部署。"
    fi
  else
    warn "未找到 ${os_release}，无法识别系统版本，继续执行。"
  fi
}

check_arch() {
  case "$(uname -m)" in
    x86_64|aarch64) ;;
    *) die "不支持的 CPU 架构：$(uname -m)（仅支持 x86_64/aarch64）" ;;
  esac
}

check_resources() {
  if command -v free >/dev/null 2>&1; then
    avail_mb=$(free -m | awk '/^Mem:/ {print $7}')
    if [ "${avail_mb:-0}" -lt 2000 ]; then
      warn "可用内存 ${avail_mb}MB < 2000MB：现场构建前端可能被 OOM 杀掉。建议先添加 2G swap："
      log "  fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile"
      log "  echo '/swapfile none swap sw 0 0' >> /etc/fstab"
    else
      ok "可用内存 ${avail_mb}MB"
    fi
  fi
  if command -v df >/dev/null 2>&1; then
    avail_gb=$(df -BG / | awk 'NR==2 {gsub("G","",$4); print $4}')
    if [ "${avail_gb:-0}" -lt 15 ]; then
      warn "根分区剩余 ${avail_gb}GB < 15GB：镜像与数据可能占满磁盘。"
    else
      ok "磁盘剩余 ${avail_gb}GB"
    fi
  fi
}

version_ge() { # $1=实际版本 $2=最低版本
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

compose_version() {
  docker compose version --short 2>/dev/null | sed 's/^v//' || true
}

compose_ok() {
  local v
  v="$(compose_version)"
  [ -n "$v" ] && version_ge "$v" "$MIN_COMPOSE_VERSION"
}

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

repair_compose_plugin() {
  local dirs=() dir f v
  if [ -n "${KAPOJ_COMPOSE_PLUGIN_DIRS:-}" ]; then
    read -r -a dirs <<< "$KAPOJ_COMPOSE_PLUGIN_DIRS"
  else
    dirs=(
      /usr/local/lib/docker/cli-plugins
      /usr/local/libexec/docker/cli-plugins
      /usr/lib/docker/cli-plugins
      /usr/libexec/docker/cli-plugins
      "${DOCKER_CONFIG:-$HOME/.docker}/cli-plugins"
    )
  fi

  for dir in "${dirs[@]}"; do
    f="$dir/docker-compose"
    [ -x "$f" ] || continue

    local works=0
    if command -v timeout >/dev/null 2>&1; then
      timeout 5 "$f" version >/dev/null 2>&1 && works=1
    else
      "$f" version >/dev/null 2>&1 && works=1
    fi
    if [ "$works" -ne 1 ]; then
      mv "$f" "$f.broken.$(date +%s)"
      warn "已隔离损坏的 Compose 插件：${f}（备份为 .broken.*）"
      continue
    fi

    v="$("$f" version --short 2>/dev/null | sed 's/^v//' || true)"
    if [ -z "$v" ]; then
      v="$("$f" version 2>/dev/null | sed -n 's/.*v\([0-9][0-9.]*\).*/\1/p' | head -1 || true)"
    fi
    if [ -n "$v" ] && ! version_ge "$v" "$MIN_COMPOSE_VERSION"; then
      mv "$f" "$f.old.$(date +%s)"
      warn "已隔离过旧的 Compose 插件：${f}（版本 $v < ${MIN_COMPOSE_VERSION}，备份为 .old.*）"
    fi
  done

  if [ -x /usr/libexec/docker/cli-plugins/docker-compose ] \
    && [ ! -e /usr/lib/docker/cli-plugins/docker-compose ]; then
    mkdir -p /usr/lib/docker/cli-plugins
    ln -s /usr/libexec/docker/cli-plugins/docker-compose /usr/lib/docker/cli-plugins/docker-compose
  fi
}

install_compose_binary() {
  local arch url dir
  case "$(uname -m)" in
    x86_64) arch="x86_64" ;;
    aarch64) arch="aarch64" ;;
    *) die "不支持的 CPU 架构：$(uname -m)" ;;
  esac
  url="${KAPOJ_COMPOSE_URL:-https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$arch}"
  dir="/usr/local/lib/docker/cli-plugins"
  mkdir -p "$dir"
  log "下载 Compose 独立二进制：$url"
  if ! curl -fL --connect-timeout 10 --max-time 180 "$url" -o "$dir/docker-compose"; then
    die "Compose 二进制下载失败。请手动安装 ≥ v${MIN_COMPOSE_VERSION} 后重试，或用 KAPOJ_COMPOSE_URL 指定可达地址。"
  fi
  chmod +x "$dir/docker-compose"
  if ! "$dir/docker-compose" version >/dev/null 2>&1; then
    rm -f "$dir/docker-compose"
    die "Compose 二进制校验失败（可能下载到了错误页面）。请用 KAPOJ_COMPOSE_URL 指定正确地址。"
  fi
}

install_docker() {
  ensure_base_packages

  if command -v docker >/dev/null 2>&1; then
    repair_compose_plugin
    if ! compose_ok; then
      warn "Docker 已安装，但 Compose $(compose_version) 不满足 ≥ ${MIN_COMPOSE_VERSION}；只修复 Compose，不安装 docker.io。"
      if apt_get install docker-compose-plugin; then
        repair_compose_plugin
      else
        warn "docker-compose-plugin 包不可用（可能缺少 Docker apt 源），将下载独立二进制。"
      fi
    fi
  else
    step "安装 Docker Engine 与 Compose 插件"
    if apt_get install docker.io docker-compose-v2; then
      log "已通过 Ubuntu 软件源安装 docker.io + docker-compose-v2"
    else
      warn "Ubuntu 软件源安装失败，尝试 get.docker.com"
      if curl -fsSL https://get.docker.com | sh >/tmp/kapoj-getdocker.log 2>&1; then
        log "get.docker.com 安装完成"
        apt_get install docker-compose-plugin || warn "docker-compose-plugin 安装失败，将下载独立二进制。"
      else
        warn "get.docker.com 也失败（见 /tmp/kapoj-getdocker.log）"
      fi
    fi
    repair_compose_plugin
  fi

  if ! compose_ok; then
    install_compose_binary
  fi

  systemctl enable docker >/dev/null 2>&1 || service docker start >/dev/null 2>&1 || true
  systemctl restart docker 2>/dev/null || service docker restart >/dev/null 2>&1 || true
  sleep 2
  command -v docker >/dev/null 2>&1 || die "Docker 安装后仍不可用，请查看上方日志手动安装。"
  compose_ok || die "Compose 版本 $(compose_version) < ${MIN_COMPOSE_VERSION}。请检查插件路径后重试。"
  ok "Docker 就绪（$(docker --version | awk '{print $3}' | tr -d ,)，compose $(docker compose version --short)）"
}

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

ensure_mirrors() {
  if [ "${KAPOJ_DOCKER_MIRRORS:-}" = "off" ]; then
    warn "KAPOJ_DOCKER_MIRRORS=off，跳过镜像加速配置（后续拉取可能失败）"
    return
  fi

  local daemon_json="${KAPOJ_DAEMON_JSON:-/etc/docker/daemon.json}"
  local explicit=0
  [ -n "${KAPOJ_DOCKER_MIRRORS:-}" ] && explicit=1

  if [ "$explicit" -eq 0 ] && [ -f "$daemon_json" ] \
    && grep -q 'registry-mirrors' "$daemon_json"; then
    warn "${daemon_json} 已含 registry-mirrors 配置，不改动。若拉取仍慢，可显式设置 KAPOJ_DOCKER_MIRRORS 覆盖。"
    return
  fi

  local candidates=()
  if [ "$explicit" -eq 1 ]; then
    parse_mirrors "$KAPOJ_DOCKER_MIRRORS"
    candidates=("${MIRRORS[@]}")
  else
    if curl -fsS --connect-timeout 2 --max-time 3 \
      http://metadata.tencentyun.com/latest/meta-data/ >/dev/null 2>&1; then
      candidates+=(https://mirror.ccs.tencentyun.com)
    fi
    candidates+=("${DEFAULT_MIRRORS[@]}")
  fi

  local mirrors=() m
  for m in "${candidates[@]}"; do
    [ -n "$m" ] || continue
    if mirror_reachable "$m"; then
      mirrors+=("$m")
    else
      warn "镜像加速地址不可达，已跳过：$m"
    fi
  done
  if [ "${#mirrors[@]}" -eq 0 ]; then
    warn "没有可用的镜像加速地址，继续使用 Docker Hub 直连（拉取可能失败）"
    return
  fi

  step "配置 Docker 镜像加速"
  if [ "$FLAG_YES" -eq 0 ]; then
    log "将写入加速地址：${mirrors[*]}"
    if [ -f "$daemon_json" ]; then
      log "已存在的 ${daemon_json} 将备份为 daemon.json.bak 后合并 registry-mirrors 键"
    fi
    log "期间会重启 Docker（本机正在运行的其他容器会短暂中断）"
    local reply=""
    printf '%s' "继续？(回车=是 / n=否): "
    if ! read -r reply; then reply="n"; fi
    case "$reply" in
      ""|y|Y|yes|Yes) ;;
      *) warn "已跳过镜像加速配置（后续拉取可能失败；可设 KAPOJ_DOCKER_MIRRORS 或用 --yes 跳过询问）"; return ;;
    esac
  fi

  local json="["
  local first=1
  for m in "${mirrors[@]}"; do
    [ $first -eq 1 ] || json+=", "
    json+="\"$m\""
    first=0
  done
  json+="]"
  mkdir -p "$(dirname "$daemon_json")"
  local py="${KAPOJ_PYTHON3:-python3}"
  if [ -f "$daemon_json" ]; then
    cp "$daemon_json" "$daemon_json.bak"
    command -v "$py" >/dev/null 2>&1 \
      || die "${daemon_json} 已存在但无 python3 可安全合并。请手工加入 registry-mirrors 后重试（原文件未改动，备份路径 daemon.json.bak）。"
    if ! MIRRORS_JSON="$json" "$py" - "$daemon_json" <<'PYEOF'
import json, os, sys
path = sys.argv[1]
with open(path) as f:
    cfg = json.load(f)
cfg["registry-mirrors"] = json.loads(os.environ["MIRRORS_JSON"])
with open(path, "w") as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
PYEOF
    then
      die "${daemon_json} 解析/写入失败，原文件未改动（备份 daemon.json.bak）。请手工合并 registry-mirrors。"
    fi
    log "已合并 registry-mirrors 到 ${daemon_json}（原文件备份为 daemon.json.bak）"
  else
    printf '{ "registry-mirrors": %s }\n' "$json" > "$daemon_json"
    log "已写入 ${daemon_json}"
  fi
  systemctl restart docker 2>/dev/null || service docker restart >/dev/null 2>&1 || true
  sleep 2
  docker info --format '{{json .RegistryConfig.Mirrors}}' >/dev/null 2>&1 \
    || warn "Docker 重启后暂不可用，请稍后运行 docker info 检查"
  ok "镜像加速已启用：${mirrors[*]}"
}

# ---------- .env ----------
# 从 .env 读取部署入口设置，供 summary/wait_entry/status 使用；
# --domain 与 .env 不一致时更新这两个键（脚本唯一受支持的 .env 写路径，用户显式意图）。
load_entry_settings() {
  [ -f "$ENV_FILE" ] || return 0
  local env_domain env_site env_tls
  env_domain=$(grep -E '^KAPOJ_DOMAIN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
  env_site=$(grep -E '^KAPOJ_SITE_ADDRESS=' "$ENV_FILE" | head -1 | cut -d= -f2-)
  env_tls=$(grep -E '^KAPOJ_TLS_VERIFY=' "$ENV_FILE" | head -1 | cut -d= -f2-)
  if [ "$FLAG_INSECURE_HEALTH" -eq 1 ]; then
    TLS_VERIFY=0
    set_env_key KAPOJ_TLS_VERIFY 0
  else
    TLS_VERIFY="${env_tls:-1}"
  fi
  if [ -n "$DOMAIN" ] && [ "$DOMAIN" != "$env_domain" ]; then
    validate_domain "$DOMAIN"
    set_env_key KAPOJ_DOMAIN "$DOMAIN"
    SITE_ADDRESS="$DOMAIN"
    set_env_key KAPOJ_SITE_ADDRESS "$SITE_ADDRESS"
    ok "已更新 .env：KAPOJ_DOMAIN / KAPOJ_SITE_ADDRESS → $DOMAIN"
  else
    DOMAIN="$env_domain"
    SITE_ADDRESS="${env_site:-:80}"
    if [ -n "$DOMAIN" ] && [ "$SITE_ADDRESS" != "$DOMAIN" ]; then
      SITE_ADDRESS="$DOMAIN"
      set_env_key KAPOJ_SITE_ADDRESS "$SITE_ADDRESS"
      ok "已同步 .env：KAPOJ_SITE_ADDRESS=$SITE_ADDRESS（与 KAPOJ_DOMAIN 对齐）"
    fi
  fi
}

gen_env() {
  if [ -f "$ENV_FILE" ]; then
    ok "检测到根目录 .env，保留现有配置（不覆盖任何密钥）"
    return
  fi
  # 交互输入先于文件创建：避免 cp 之后因非交互/EOF 半途退出、留下占位符 .env
  if [ -n "$DOMAIN" ]; then
    validate_domain "$DOMAIN"
  elif [ "$FLAG_YES" -eq 1 ]; then
    DOMAIN=""
  elif [ -t 0 ]; then
    printf '%s' "请输入对外域名（如 oj.example.com；留空则仅用 HTTP 80 访问）: "
    read -r DOMAIN || die "读取输入失败（EOF）。非交互环境请用 --domain <域名> 或 --yes（无域名 HTTP）。"
    [ -n "$DOMAIN" ] && validate_domain "$DOMAIN"
  else
    die "检测到非交互终端且未提供 --domain/--yes。请使用：sudo bash deploy.sh --yes（仅 HTTP）或 sudo bash deploy.sh --domain <域名>（自动 HTTPS）。"
  fi
  step "生成统一配置文件 .env"
  ( umask 077 && cp .env.example "$ENV_FILE" && chmod 600 "$ENV_FILE" )
  if [ -n "$DOMAIN" ]; then
    validate_domain "$DOMAIN"
  elif [ "$FLAG_YES" -eq 0 ]; then
    printf '%s' "请输入对外域名（如 oj.example.com；留空则仅用 HTTP 80 访问）: "
    read -r DOMAIN
    [ -n "$DOMAIN" ] && validate_domain "$DOMAIN"
  fi
  if [ -n "$DOMAIN" ]; then
    SITE_ADDRESS="$DOMAIN"
  else
    SITE_ADDRESS=":80"
    [ "$FLAG_YES" -eq 0 ] && warn "未设置域名：将仅以 http://<服务器IP> 提供服务，可在 .env 填 KAPOJ_DOMAIN 后执行 ./deploy.sh update 启用 HTTPS"
  fi
  set_env_key KAPOJ_DOMAIN "$DOMAIN"
  set_env_key KAPOJ_SITE_ADDRESS "$SITE_ADDRESS"
  set_env_key POSTGRES_PASSWORD   "$(openssl rand -hex 16)"
  set_env_key MINIO_ROOT_PASSWORD "$(openssl rand -hex 16)"
  set_env_key KAPOJ_JWT_SECRET         "$(openssl rand -base64 48)"
  set_env_key KAPOJ_AI_ENCRYPTION_KEY  "$(openssl rand -base64 32)"
  set_env_key KAPOJ_MAIL_ENCRYPTION_KEY "$(openssl rand -base64 32)"
  cur_trusted=$(grep -E '^KAPOJ_TRUSTED_PROXIES=' "$ENV_FILE" | head -1 | cut -d= -f2-)
  if [ -z "$cur_trusted" ]; then
    set_env_key KAPOJ_TRUSTED_PROXIES "172.16.0.0/12"
  fi
  chmod 600 "$ENV_FILE"
  # 占位符守卫：任何密钥仍为模板默认值/空值时大声警告（不该出现，出现即说明生成中断）
  local bad=""
  if grep -q '^POSTGRES_PASSWORD=change-me' "$ENV_FILE"; then bad+="POSTGRES_PASSWORD "; fi
  if grep -q '^MINIO_ROOT_PASSWORD=change-me' "$ENV_FILE"; then bad+="MINIO_ROOT_PASSWORD "; fi
  if grep -q '^KAPOJ_JWT_SECRET=change-me' "$ENV_FILE"; then bad+="KAPOJ_JWT_SECRET "; fi
  if [ -z "$(grep -E '^KAPOJ_AI_ENCRYPTION_KEY=' "$ENV_FILE" | head -1 | cut -d= -f2-)" ]; then bad+="KAPOJ_AI_ENCRYPTION_KEY "; fi
  if [ -z "$(grep -E '^KAPOJ_MAIL_ENCRYPTION_KEY=' "$ENV_FILE" | head -1 | cut -d= -f2-)" ]; then bad+="KAPOJ_MAIL_ENCRYPTION_KEY "; fi
  if [ -n "$bad" ]; then
    warn "警告：.env 中以下密钥仍为占位/空值：$bad—— 请立即修改或删除 .env 后重跑 install！"
  fi
  ok "已生成 .env（随机数据库/MinIO 密码、JWT、AI/邮件加密密钥），请妥善备份"
}

validate_domain() {
  printf '%s' "$1" | grep -qE '^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$' \
    || die "域名格式不合法：$1（应为小写域名，如 oj.example.com）"
}

set_env_key() { # $1=键 $2=值
  if grep -q "^$1=" "$ENV_FILE" 2>/dev/null; then
    sed -i.bak "s|^$1=.*|$1=$2|" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  else
    printf '%s=%s\n' "$1" "$2" >> "$ENV_FILE"
  fi
}

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

# ---------- 构建/启动/镜像 ----------
ensure_socket_proxy() {
  if [ "${KAPOJ_JUDGE_DOCKER_DIRECT:-0}" = "1" ]; then
    ok "评测服务使用直连 Docker socket（回退模式，KAPOJ_JUDGE_DOCKER_DIRECT=1）"
    return
  fi
  if docker image inspect "$SOCKET_PROXY_IMAGE" >/dev/null 2>&1; then
    ok "Docker socket 白名单代理镜像已就绪"
    return
  fi
  log "拉取 Docker socket 白名单代理（$SOCKET_PROXY_IMAGE）"
  if ! retry 3 docker pull "$SOCKET_PROXY_IMAGE"; then
    die "socket-proxy 镜像拉取失败；可检查网络，或使用 KAPOJ_JUDGE_DOCKER_DIRECT=1 临时回退直连（不推荐长期使用）"
  fi
  ok "Docker socket 白名单代理镜像已就绪"
}

build_up() {
  step "构建全部镜像（前端构建约 1-2 分钟，Go 构建约 1 分钟）"
  "${COMPOSE[@]}" config --quiet
  "${COMPOSE[@]}" build --progress=plain
  step "启动全部服务"
  "${COMPOSE[@]}" up -d
  ok "容器已启动：$("${COMPOSE[@]}" ps --format '{{.Service}}' | sort | tr '\n' ' ')"
}

pull_judge_images() {
  if [ "$FLAG_SKIP_IMAGES" -eq 1 ]; then
    warn "跳过评测语言镜像拉取（--skip-images）；judge 首次评测对应语言时会按需拉取"
    return
  fi
  step "预拉取评测语言镜像（后台并行，失败不阻塞部署）"
  local images=() key value
  for key in KAPOJ_JUDGE_CPP_IMAGE KAPOJ_JUDGE_PYTHON_IMAGE \
    KAPOJ_JUDGE_JAVA_IMAGE KAPOJ_JUDGE_GO_IMAGE; do
    value="$(env_get "$key")"
    [ -n "$value" ] && images+=("$value")
  done
  if [ "${#images[@]}" -eq 0 ]; then
    images=("${JUDGE_IMAGES[@]}")
  fi

  local pids=() failed=() img
  for img in "${images[@]}"; do
    retry 3 docker pull "$img" >/dev/null 2>&1 &
    pids+=($!)
  done
  local i=0
  for img in "${images[@]}"; do
    if ! wait "${pids[$i]}"; then
      failed+=("$img")
    fi
    i=$((i + 1))
  done
  if [ "${#failed[@]}" -gt 0 ]; then
    warn "以下镜像拉取失败（不影响平台运行，评测对应语言时需重新拉取）：${failed[*]}"
  else
    ok "评测语言镜像已就绪：${images[*]}"
  fi
}

# ---------- 健康检查 ----------
wait_health() {
  step "等待后端健康（最长 120 秒）"
  local i=0
  while [ $i -lt 60 ]; do
    if curl -m 3 -fsS "$HEALTH_URL" 2>/dev/null | grep -q '"status"[[:space:]]*:[[:space:]]*"ok"'; then
      ok "后端健康：$HEALTH_URL"
      wait_entry
      return
    fi
    sleep 2
    i=$((i + 1))
  done
  "${COMPOSE[@]}" ps || true
  "${COMPOSE[@]}" logs --no-color --tail=100 || true
  die "后端未在 120 秒内就绪。完整日志：${LOG_FILE}；请按上方日志排查，常见原因见 运行手册 §10。"
}

wait_entry() {
  if [ -n "$DOMAIN" ]; then
    log "等待 HTTPS 证书签发（最长 60 秒，需要域名已解析到本机且 80/443 可达）"
    local insecure=0
    if [ "${TLS_VERIFY:-1}" = "0" ]; then
      insecure=1
      warn "KAPOJ_TLS_VERIFY=0：健康检查跳过证书校验（仅用于私有源站证书）"
    fi
    local i=0
    while [ $i -lt 30 ]; do
      if [ "$insecure" -eq 1 ]; then
        curl -k -m 5 -fsS --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/health" >/dev/null 2>&1 && {
          ok "HTTPS 入口就绪：https://$DOMAIN"
          return
        }
      elif curl -m 5 -fsS --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/health" >/dev/null 2>&1; then
        ok "HTTPS 入口就绪：https://$DOMAIN"
        return
      fi
      sleep 2
      i=$((i + 1))
    done
    die "HTTPS 入口 60 秒内未就绪。服务容器已启动，常见原因：域名 A 记录未生效、云安全组未放行 80/443、域名未 ICP 备案。
    排查：ss -ltnp | grep ':80\\|:443' 与 docker compose -f $BASE_COMPOSE -f $PROD_COMPOSE logs web
    稍后可手动验证 https://$DOMAIN 或重跑 ./deploy.sh update"
  else
    local i=0
    while [ $i -lt 15 ]; do
      if curl -m 3 -fsS http://127.0.0.1/ >/dev/null 2>&1; then
        ok "HTTP 入口就绪：http://<服务器IP>"
        return
      fi
      sleep 2
      i=$((i + 1))
    done
    "${COMPOSE[@]}" logs --tail=30 web || true
    die "HTTP 入口 30 秒内未就绪。请按上方 web 日志排查（常见：80 端口被宿主机其他进程占用）。"
  fi
}

# ---------- 汇总 ----------
print_summary() {
  local addr="http://<服务器IP>"
  [ -n "$DOMAIN" ] && addr="https://$DOMAIN"
  printf '\n%s\n' "${C_BOLD}==================== kapOJ 部署完成 ====================${C_OFF}"
  printf '%s\n' " 前端入口      : $addr"
  printf '%s\n' " API 健康      : $addr/health"
  printf '%s\n' " MinIO Console : http://127.0.0.1:9001 （仅本机可达，SSH 隧道访问："
  printf '%s\n' "                 ssh -L 9001:127.0.0.1:9001 user@<服务器IP>，账号见 .env 的 MINIO_ROOT_USER）"
  printf '%s\n' ""
  printf '%s\n' " 创建第一个管理员：先在前端注册账号，然后执行（详见 运行手册 §11.4/§7.3）："
  printf '%s\n' "   sudo bash deploy.sh promote <你的用户名>"
  printf '%s\n' " 外部数据库（KAPOJ_DATABASE_URL）需按运行手册手工执行 users.role + user_roles 两条 SQL。"
  printf '%s\n' ""
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    printf '%s\n' " ⚠ 检测到 ufw 已启用：请放行 80/443 —— sudo ufw allow 80,443/tcp"
  fi
  if [ "${KAPOJ_JUDGE_DOCKER_DIRECT:-0}" = "1" ]; then
    printf '%s\n' " Docker 访问   : 直连 socket（KAPOJ_JUDGE_DOCKER_DIRECT=1 回退模式，建议尽快切回 socket-proxy）"
  else
    printf '%s\n' " Docker 访问   : socket-proxy 白名单代理（judge 独立网络）"
  fi
  printf '%s\n' " 日常运维      : ./deploy.sh status | logs | stop | restart | update"
  printf '%s\n' "${C_BOLD}========================================================${C_OFF}\n"
}

# ---------- 子命令 ----------
cmd_install() {
  step "环境检查（root/系统/内存/磁盘）"
  check_os
  check_arch
  check_resources
  load_entry_settings
  install_docker
  ensure_mirrors
  gen_env
  ensure_env_symlink
  deploy_custom_cert
  [ -f "$PROD_COMPOSE" ] || die "缺少 ${PROD_COMPOSE}，请在仓库根目录运行本脚本。"
  ensure_socket_proxy
  build_up
  pull_judge_images
  wait_health
  print_summary
}

cmd_update() {
  need_root update
  if [ "$FLAG_PULL" -eq 1 ]; then
    if [ -d .git ]; then
      git pull --ff-only
    else
      die "当前目录不是 git 仓库，无法 --pull。请重新上传最新代码后直接运行 ./deploy.sh update。"
    fi
  fi
  check_os
  check_arch
  load_entry_settings
  install_docker
  ensure_mirrors
  ensure_env_symlink
  deploy_custom_cert
  ensure_socket_proxy
  build_up
  docker image prune -f >/dev/null 2>&1 || true
  wait_health
  print_summary
}

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
  ok "已提升 ${user} 为 super_admin"
}

cmd_status() {
  if ! command -v docker >/dev/null 2>&1; then
    warn "Docker 未安装（先运行 sudo bash deploy.sh install）"
    return 0
  fi
  load_entry_settings
  "${COMPOSE[@]}" ps
  if curl -m 3 -fsS "$HEALTH_URL" 2>/dev/null | grep -q '"status"[[:space:]]*:[[:space:]]*"ok"'; then
    ok "后端健康：$HEALTH_URL"
  else
    warn "后端不可达：$HEALTH_URL"
  fi
  if curl -m 3 -fsS http://127.0.0.1/ >/dev/null 2>&1 || curl -m 3 -kfsS "https://127.0.0.1/" >/dev/null 2>&1; then
    ok "web 入口可达：80/443"
  else
    warn "web 入口不可达（80/443）"
  fi
  return 0
}

cmd_logs() {
  "${COMPOSE[@]}" logs -f --tail=100 "${@+"$@"}"
}

cmd_stop()   { "${COMPOSE[@]}" stop; }
cmd_restart(){ "${COMPOSE[@]}" restart; }

# ---------- 入口 ----------
usage() {
  sed -n '2,21p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

main() {
  local cmd="install"
  local args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      install|update|status|logs|stop|restart|promote) cmd="$1"; shift ;;
      --domain)
        if [ $# -lt 2 ]; then die "--domain 需要一个参数，如 --domain oj.example.com"; fi
        DOMAIN="$2"; shift 2 ;;
      --cert-crt)
        if [ $# -lt 2 ]; then die "--cert-crt 需要一个证书文件路径"; fi
        CERT_CRT="$2"; shift 2 ;;
      --cert-key)
        if [ $# -lt 2 ]; then die "--cert-key 需要一个私钥文件路径"; fi
        CERT_KEY="$2"; shift 2 ;;
      --insecure-health) FLAG_INSECURE_HEALTH=1; TLS_VERIFY=0; shift ;;
      --yes)   FLAG_YES=1; shift ;;
      --skip-images) FLAG_SKIP_IMAGES=1; shift ;;
      --pull)  FLAG_PULL=1; shift ;;
      --help|-h) usage; exit 0 ;;
      *) args+=("$1"); shift ;;
    esac
  done
  if [ "$cmd" = "promote" ]; then
    [ "${#args[@]}" -le 1 ] || die "promote 只接受一个用户名"
  else
    [ "${#args[@]}" -eq 0 ] || die "未知参数：${args[*]}（--help 查看用法）"
  fi
  init_log
  if [ "${KAPOJ_JUDGE_DOCKER_DIRECT:-0}" = "1" ]; then
    [ -f "$DIRECT_COMPOSE" ] || die "缺少 ${DIRECT_COMPOSE}；无法启用直连回退模式。"
    COMPOSE+=(-f "$DIRECT_COMPOSE")
  fi
  case "$cmd" in
    install) need_root install; cmd_install ;;
    update)  cmd_update ;;
    promote) need_root promote; cmd_promote "${args[0]:-}" ;;
    status)  cmd_status ;;
    logs)    cmd_logs "${args[@]+"${args[@]}"}" ;;
    stop)    need_root stop;   cmd_stop ;;
    restart) need_root restart; cmd_restart ;;
  esac
}

if [ "${KAPOJ_DEPLOY_LIB_ONLY:-0}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

main "$@"
