#!/usr/bin/env bash
# Isolated full-stack environment for the Playwright core-flow smoke test.
set -euo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV_FILE=".env.e2e"
PROJECT="kapoj-e2e"
API_URL="http://127.0.0.1:18080"
WEB_URL="http://127.0.0.1:15173"
COMPOSE=(docker compose -p "$PROJECT" --env-file "$ENV_FILE"
  -f docker/docker-compose.yml
  -f docker/docker-compose.prod.yml
  -f docker/docker-compose.e2e.yml
  --profile app)
JUDGE_IMAGES=(gcc:14 python:3.12-slim eclipse-temurin:21-jdk golang:1.26)

usage() {
  cat <<'EOF'
Usage: scripts/e2e-stack.sh <up|down|logs|status>

  up      create .env.e2e, pull judge images, start the stack, wait for health
  down    stop the stack and remove e2e PostgreSQL/MinIO volumes
  logs    show recent stack logs (optional tail count, default 300)
  status  show compose service status
EOF
}

need_docker() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker is required" >&2
    exit 1
  }
  docker compose version >/dev/null 2>&1 || {
    echo "docker compose v2 is required" >&2
    exit 1
  }
}

ensure_env() {
  if [ ! -f "$ENV_FILE" ]; then
    command -v python3 >/dev/null 2>&1 || {
      echo "python3 is required to generate $ENV_FILE" >&2
      exit 1
    }
    local tmp="${ENV_FILE}.tmp.$$"
    cp .env.example "$tmp"
    if ! python3 - "$tmp" <<'PY'
import base64
import re
import secrets
import sys

path = sys.argv[1]
text = open(path, encoding="utf-8").read()


def replace(key: str, value: str) -> None:
    global text
    pattern = rf"^{re.escape(key)}=.*$"
    text, count = re.subn(pattern, f"{key}={value}", text, flags=re.M)
    if count != 1:
        sys.exit(f"expected exactly one {key} entry in the template")


replace("POSTGRES_PASSWORD", secrets.token_hex(16))
replace("MINIO_ROOT_PASSWORD", secrets.token_hex(16))
replace("KAPOJ_JWT_SECRET", base64.b64encode(secrets.token_bytes(48)).decode())
replace("KAPOJ_AI_ENCRYPTION_KEY", base64.b64encode(secrets.token_bytes(32)).decode())
replace("KAPOJ_MAIL_ENCRYPTION_KEY", base64.b64encode(secrets.token_bytes(32)).decode())

open(path, "w", encoding="utf-8").write(text)
PY
    then
      rm -f "$tmp"
      echo "failed to generate $ENV_FILE" >&2
      exit 1
    fi
    chmod 600 "$tmp"
    mv "$tmp" "$ENV_FILE"
    echo "created $ENV_FILE"
  fi
  chmod 600 "$ENV_FILE"
}

# Load the e2e file into the shell environment so Compose interpolation cannot
# silently pick up POSTGRES_* / MINIO_* values exported for the main stack.
load_env() {
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
}

pull_images() {
  if [ "${KAPOJ_E2E_SKIP_PULL:-0}" = "1" ]; then
    echo "skipping judge image pull (KAPOJ_E2E_SKIP_PULL=1)"
    return 0
  fi
  local image attempt
  for image in "${JUDGE_IMAGES[@]}"; do
    if docker image inspect "$image" >/dev/null 2>&1; then
      echo "image cached: $image"
      continue
    fi
    for attempt in 1 2 3; do
      if docker pull "$image"; then
        break
      fi
      if [ "$attempt" -eq 3 ]; then
        echo "failed to pull $image" >&2
        return 1
      fi
      sleep 15
    done
  done
}

health_check() {
  curl -fsS --connect-timeout 3 --max-time 5 "$1" >/dev/null 2>&1
}

wait_health() {
  local deadline=$((SECONDS + 180))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if health_check "$API_URL/health" && health_check "$WEB_URL/health"; then
      return 0
    fi
    sleep 2
  done
  echo "e2e stack did not become healthy in 180s" >&2
  "${COMPOSE[@]}" ps >&2 || true
  "${COMPOSE[@]}" logs --no-color --tail=200 >&2 || true
  echo "hint: run 'bash scripts/e2e-stack.sh down -v' before retrying" >&2
  return 1
}

psql_exec() {
  "${COMPOSE[@]}" exec -T postgres \
    psql -U "${POSTGRES_USER:-kapoj}" -d "${POSTGRES_DB:-kapoj}" -v ON_ERROR_STOP=1 "$@"
}

seed() {
  ensure_env
  load_env

  psql_exec >/dev/null <<'SQL'
INSERT INTO mail_settings (id, updated_at, require_email_verification, sending_enabled)
VALUES (1, now(), false, false)
ON CONFLICT (id) DO UPDATE
  SET require_email_verification = false,
      sending_enabled = false,
      updated_at = now();
SQL

  local admin_exists
  admin_exists=$(psql_exec -tAc "SELECT 1 FROM users WHERE username = 'm2badmin'" | tr -d '[:space:]')
  if [ "$admin_exists" != "1" ]; then
    curl -fsS --connect-timeout 3 --max-time 30 -X POST "$API_URL/api/auth/register" \
      -H 'Content-Type: application/json' \
      --data '{"username":"m2badmin","email":"m2badmin@example.com","password":"correct-horse"}' \
      >/dev/null
  fi

  psql_exec >/dev/null <<'SQL'
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, now()), status = 'active'
WHERE username = 'm2badmin';

INSERT INTO user_roles (user_id, role_id, assigned_at)
SELECT u.id, r.id, now()
FROM users u CROSS JOIN roles r
WHERE u.username = 'm2badmin' AND r.code = 'super_admin'
ON CONFLICT (user_id, role_id) DO NOTHING;

INSERT INTO users (username, nickname, email, email_verified_at, password_hash, salt, role, rating, coins, bio, status, created_at, updated_at)
SELECT 'm2bsolver1', 'm2bsolver1', 'm2bsolver1@example.com', now(), password_hash, salt, 'user', 1500, 0, '', 'active', now(), now()
FROM users WHERE username = 'm2badmin'
ON CONFLICT (username) DO NOTHING;

INSERT INTO users (username, nickname, email, email_verified_at, password_hash, salt, role, rating, coins, bio, status, created_at, updated_at)
SELECT 'm2bsolver2', 'm2bsolver2', 'm2bsolver2@example.com', now(), password_hash, salt, 'user', 1500, 0, '', 'active', now(), now()
FROM users WHERE username = 'm2badmin'
ON CONFLICT (username) DO NOTHING;
SQL

  echo "seeded e2e accounts"
}

up() {
  local start=$SECONDS
  need_docker
  ensure_env
  load_env
  pull_images
  "${COMPOSE[@]}" up -d --build
  wait_health
  seed
  local elapsed=$((SECONDS - start))
  echo "e2e stack ready in ${elapsed}s"
  if [ "$elapsed" -gt 600 ]; then
    echo "warning: startup exceeded the 600s target" >&2
  fi
  echo "API: $API_URL"
  echo "Web: $WEB_URL"
  echo "Run: E2E_API_URL=$API_URL E2E_BASE_URL=$WEB_URL corepack pnpm test:e2e"
  echo "Or:  corepack pnpm test:e2e:core"
}

down() {
  need_docker
  ensure_env
  load_env
  "${COMPOSE[@]}" down -v --remove-orphans
}

logs() {
  need_docker
  ensure_env
  load_env
  "${COMPOSE[@]}" logs --no-color --tail="${2:-300}"
}

status() {
  need_docker
  ensure_env
  load_env
  "${COMPOSE[@]}" ps
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  logs) logs "$@" ;;
  status) status ;;
  *)
    usage
    exit 2
    ;;
esac
