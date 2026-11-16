#!/usr/bin/env bash
# kapOJ 一键部署脚本（目标系统：Ubuntu 20.04 LTS）
#
# 用法：
#   sudo bash deploy.sh [install] [--domain oj.example.com] [--yes] [--skip-images]
#   sudo bash deploy.sh update [--pull]
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
#
# 详细说明见 运行手册.md §11（生产部署）。
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------- 常量 ----------
BASE_COMPOSE="docker/docker-compose.yml"
PROD_COMPOSE="docker/docker-compose.prod.yml"
COMPOSE=(docker compose -f "$BASE_COMPOSE" -f "$PROD_COMPOSE" --profile app)
JUDGE_IMAGES=(gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26)
# 镜像加速清单核对日期：2026-11-15（公共加速器可用性会变化，失效时用 KAPOJ_DOCKER_MIRRORS 覆盖，
# 或改用阿里云/腾讯云控制台分配的专属加速地址）
DEFAULT_MIRRORS=(https://docker.1ms.run https://docker.1panel.live https://hub.rat.dev)
MIN_COMPOSE_VERSION="2.24"
HEALTH_URL="http://127.0.0.1:8080/health"

DOMAIN=""
FLAG_YES=0
FLAG_SKIP_IMAGES=0
FLAG_PULL=0

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

# ---------- 前置检查 ----------
need_root() {
  [ "$(id -u)" -eq 0 ] || die "请用 root 运行：sudo bash deploy.sh $*"
}

check_os() {
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [ "${ID:-}" = "ubuntu" ] && [ "${VERSION_ID:-}" = "20.04" ]; then
      ok "检测到 Ubuntu 20.04 LTS（${PRETTY_NAME:-}）"
    else
      warn "当前系统为 ${PRETTY_NAME:-未知}，本脚本按 Ubuntu 20.04 设计；将尽力而为，遇到问题参见运行手册 §11 手工部署。"
    fi
  else
    warn "未找到 /etc/os-release，无法识别系统版本，继续执行。"
  fi
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

compose_version() {
  docker compose version --short 2>/dev/null | tr -d 'v' || echo "0"
}

version_ge() { # $1=实际版本 $2=最低版本
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

install_docker() {
  if command -v docker >/dev/null 2>&1 && version_ge "$(compose_version)" "$MIN_COMPOSE_VERSION"; then
    ok "Docker 已安装（$(docker --version | awk '{print $3}' | tr -d ,)），compose $(docker compose version --short)"
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    warn "检测到 compose $(compose_version) < ${MIN_COMPOSE_VERSION}（需要 !override 语法），将重装/升级。"
  fi
  step "安装 Docker Engine 与 compose 插件"
  if curl -fsSL https://get.docker.com | sh >/tmp/kapoj-getdocker.log 2>&1; then
    log "get.docker.com 安装完成"
  else
    warn "get.docker.com 失败（见 /tmp/kapoj-getdocker.log），回退 Ubuntu 源 docker.io + compose 独立二进制"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y docker.io
    mkdir -p /usr/local/lib/docker/cli-plugins
    if ! curl -fsSL "${KAPOJ_COMPOSE_URL:-https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64}" \
        -o /usr/local/lib/docker/cli-plugins/docker-compose; then
      die "compose v2 二进制下载失败。请手动安装 ≥ v${MIN_COMPOSE_VERSION} 后重试：参见 https://docs.docker.com/compose/install/ ；或在大陆网络下用 KAPOJ_COMPOSE_URL 指定可达的下载地址。"
    fi
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  fi
  systemctl enable docker >/dev/null 2>&1 || service docker start >/dev/null 2>&1 || true
  systemctl restart docker 2>/dev/null || service docker restart >/dev/null 2>&1 || true
  sleep 2
  command -v docker >/dev/null 2>&1 || die "Docker 安装后仍不可用，请查看上方日志手动安装。"
  version_ge "$(compose_version)" "$MIN_COMPOSE_VERSION" \
    || die "compose 版本 $(compose_version) < ${MIN_COMPOSE_VERSION}。请手动升级 docker compose 插件后重试。"
  ok "Docker 就绪（compose $(docker compose version --short)）"
}

ensure_mirrors() {
  # 大陆网络探测：registry-1.docker.io 返回任意 HTTP 状态码即视为可达（未授权时是 401）
  code=$(curl -s -o /dev/null -w '%{http_code}' --connect-timeout 5 https://registry-1.docker.io/v2/ || true)
  if [ "${code:-000}" != "000" ]; then
    ok "Docker Hub 直连可达（HTTP ${code}），跳过镜像加速配置"
    return
  fi
  if [ "${KAPOJ_DOCKER_MIRRORS:-}" = "off" ]; then
    warn "Docker Hub 不可达，且 KAPOJ_DOCKER_MIRRORS=off，跳过加速配置（后续拉取可能失败）"
    return
  fi
  step "Docker Hub 不可达，配置镜像加速"
  local daemon_json=/etc/docker/daemon.json
  if [ -f "$daemon_json" ] && grep -q 'registry-mirrors' "$daemon_json"; then
    warn "${daemon_json} 已含 registry-mirrors 配置，不改动。若拉取仍慢请自行核对加速地址有效性。"
    return
  fi
  local mirrors
  if [ -n "${KAPOJ_DOCKER_MIRRORS:-}" ]; then
    # shellcheck disable=SC2206
    mirrors=($KAPOJ_DOCKER_MIRRORS)
  else
    mirrors=("${DEFAULT_MIRRORS[@]}")
  fi
  # 明确征得同意：此操作会修改宿主机配置并重启 Docker（影响本机其他容器）
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
  mkdir -p /etc/docker
  if [ -f "$daemon_json" ]; then
    cp "$daemon_json" "$daemon_json.bak"
    if command -v python3 >/dev/null 2>&1; then
      MIRRORS_JSON="$json" python3 - "$daemon_json" <<'PYEOF'
import json, os, sys
path = sys.argv[1]
with open(path) as f:
    cfg = json.load(f)
cfg["registry-mirrors"] = json.loads(os.environ["MIRRORS_JSON"])
with open(path, "w") as f:
    json.dump(cfg, f, indent=2, ensure_ascii=False)
PYEOF
      log "已合并 registry-mirrors 到 ${daemon_json}（原文件备份为 daemon.json.bak）"
    else
      die "${daemon_json} 已存在但未含 registry-mirrors，且无 python3 可合并。请手动编辑加入 \"registry-mirrors\": ${json} 后重试。"
    fi
  else
    printf '{ "registry-mirrors": %s }\n' "$json" > "$daemon_json"
    log "已写入 ${daemon_json}"
  fi
  systemctl restart docker 2>/dev/null || service docker restart >/dev/null 2>&1 || true
  sleep 2
  ok "镜像加速已启用：${mirrors[*]}"
}

# ---------- .env ----------
# 从 .env 读取部署入口设置，供 summary/wait_entry/status 使用；
# --domain 与 .env 不一致时更新这两个键（脚本唯一受支持的 .env 写路径，用户显式意图）。
load_entry_settings() {
  [ -f .env ] || return 0
  local env_domain env_site
  env_domain=$(grep -E '^KAPOJ_DOMAIN=' .env | head -1 | cut -d= -f2-)
  env_site=$(grep -E '^KAPOJ_SITE_ADDRESS=' .env | head -1 | cut -d= -f2-)
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
  if [ -f .env ]; then
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
  ( umask 077 && cp .env.example .env && chmod 600 .env )
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
  cur_trusted=$(grep -E '^KAPOJ_TRUSTED_PROXIES=' .env | head -1 | cut -d= -f2-)
  if [ -z "$cur_trusted" ]; then
    set_env_key KAPOJ_TRUSTED_PROXIES "172.16.0.0/12"
  fi
  chmod 600 .env
  # 占位符守卫：任何密钥仍为模板默认值/空值时大声警告（不该出现，出现即说明生成中断）
  local bad=""
  if grep -q '^POSTGRES_PASSWORD=change-me' .env; then bad+="POSTGRES_PASSWORD "; fi
  if grep -q '^MINIO_ROOT_PASSWORD=change-me' .env; then bad+="MINIO_ROOT_PASSWORD "; fi
  if grep -q '^KAPOJ_JWT_SECRET=change-me' .env; then bad+="KAPOJ_JWT_SECRET "; fi
  if [ -z "$(grep -E '^KAPOJ_AI_ENCRYPTION_KEY=' .env | head -1 | cut -d= -f2-)" ]; then bad+="KAPOJ_AI_ENCRYPTION_KEY "; fi
  if [ -z "$(grep -E '^KAPOJ_MAIL_ENCRYPTION_KEY=' .env | head -1 | cut -d= -f2-)" ]; then bad+="KAPOJ_MAIL_ENCRYPTION_KEY "; fi
  if [ -n "$bad" ]; then
    warn "警告：.env 中以下密钥仍为占位/空值：$bad—— 请立即修改或删除 .env 后重跑 install！"
  fi
  ok "已生成 .env（随机数据库/MinIO 密码、JWT、AI/邮件加密密钥），请妥善备份"
}

validate_domain() {
  printf '%s' "$1" | grep -qE '^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$' \
    || die "域名格式不合法：$1（应为小写域名，如 oj.example.com）"
}

set_env_key() { # $1=键 $2=值 —— 首次生成 .env 时使用
  sed -i.bak "s|^$1=.*|$1=$2|" .env && rm -f .env.bak
}

# ---------- 构建/启动/镜像 ----------
build_up() {
  step "构建全部镜像（前端构建约 1-2 分钟，Go 构建约 1 分钟）"
  "${COMPOSE[@]}" build
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
  local pids=() failed=() img
  for img in "${JUDGE_IMAGES[@]}"; do
    docker pull "$img" >/dev/null 2>&1 &
    pids+=($!)
  done
  local i=0
  for img in "${JUDGE_IMAGES[@]}"; do
    if ! wait "${pids[$i]}"; then
      failed+=("$img")
    fi
    i=$((i + 1))
  done
  if [ "${#failed[@]}" -gt 0 ]; then
    warn "以下镜像拉取失败（不影响平台运行，评测对应语言时需重新拉取）：${failed[*]}"
  else
    ok "评测语言镜像已就绪：${JUDGE_IMAGES[*]}"
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
  "${COMPOSE[@]}" logs --tail=50 || true
  die "后端未在 120 秒内就绪。请按上方日志排查，常见原因见 运行手册 §10。"
}

wait_entry() {
  if [ -n "$DOMAIN" ]; then
    log "等待 HTTPS 证书签发（最长 60 秒，需要域名已解析到本机且 80/443 可达）"
    local i=0
    while [ $i -lt 30 ]; do
      if curl -m 5 -fsS --resolve "$DOMAIN:443:127.0.0.1" "https://$DOMAIN/health" >/dev/null 2>&1; then
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
  printf '%s\n' " 创建第一个管理员：先在前端注册账号，然后授予 super_admin（详见 运行手册 §11.4/§7.3）："
  printf '%s\n' "   docker compose -f $BASE_COMPOSE exec -T postgres psql -U kapoj -d kapoj -c \"UPDATE users SET role='super_admin' WHERE username='<你的用户名>';\""
  printf '%s\n' "   docker compose -f $BASE_COMPOSE exec -T postgres psql -U kapoj -d kapoj -c \"INSERT INTO user_roles (user_id, role_id, assigned_by, assigned_at) SELECT u.id, r.id, NULL, NOW() FROM users u JOIN roles r ON r.code='super_admin' WHERE u.username='<你的用户名>' ON CONFLICT DO NOTHING;\""
  printf '%s\n' ""
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    printf '%s\n' " ⚠ 检测到 ufw 已启用：请放行 80/443 —— sudo ufw allow 80,443/tcp"
  fi
  printf '%s\n' " 日常运维      : ./deploy.sh status | logs | stop | restart | update"
  printf '%s\n' "${C_BOLD}========================================================${C_OFF}\n"
}

# ---------- 子命令 ----------
cmd_install() {
  step "环境检查（root/系统/内存/磁盘）"
  check_os
  check_resources
  load_entry_settings
  install_docker
  ensure_mirrors
  gen_env
  [ -f "$PROD_COMPOSE" ] || die "缺少 $PROD_COMPOSE，请在仓库根目录运行本脚本。"
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
  load_entry_settings
  build_up
  docker image prune -f >/dev/null 2>&1 || true
  wait_health
  print_summary
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
  sed -n '2,19p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

main() {
  local cmd="install"
  local args=()
  while [ $# -gt 0 ]; do
    case "$1" in
      install|update|status|logs|stop|restart) cmd="$1"; shift ;;
      --domain)
        if [ $# -lt 2 ]; then die "--domain 需要一个参数，如 --domain oj.example.com"; fi
        DOMAIN="$2"; shift 2 ;;
      --yes)   FLAG_YES=1; shift ;;
      --skip-images) FLAG_SKIP_IMAGES=1; shift ;;
      --pull)  FLAG_PULL=1; shift ;;
      --help|-h) usage; exit 0 ;;
      *) args+=("$1"); shift ;;
    esac
  done
  [ "${#args[@]}" -eq 0 ] || die "未知参数：${args[*]}（--help 查看用法）"
  case "$cmd" in
    install) need_root install; cmd_install ;;
    update)  cmd_update ;;
    status)  cmd_status ;;
    logs)    cmd_logs "${args[@]+"${args[@]}"}" ;;
    stop)    need_root stop;   cmd_stop ;;
    restart) need_root restart; cmd_restart ;;
  esac
}

main "$@"
