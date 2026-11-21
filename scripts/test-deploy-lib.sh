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

# 6. OS release parsing
printf 'ID=ubuntu\nVERSION_ID="26.04"\nPRETTY_NAME="Ubuntu 26.04 LTS"\n' > "$TMP/os-release"
out=$(KAPOJ_OS_RELEASE_FILE="$TMP/os-release" check_os 2>&1)
printf '%s' "$out" | grep -q "检测到 Ubuntu" || fail "check_os did not accept Ubuntu 26.04: $out"
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

# 8. compose always gets the repository .env explicitly
grep -q -- '--env-file "$ENV_FILE"' "$ROOT_DIR/deploy.sh" \
  || fail "COMPOSE does not pass --env-file"
pass "compose passes --env-file"

# 9. mirror parsing accepts commas and spaces
parse_mirrors 'https://a.example,https://b.example https://c.example'
[ "${#MIRRORS[@]}" -eq 3 ] || fail "parse_mirrors count"
[ "${MIRRORS[0]}" = "https://a.example" ] || fail "parse_mirrors order"
pass "parse_mirrors handles commas and spaces"

# 10. explicit mirrors replace registry-mirrors but preserve other keys
printf '{"data-root":"/var/lib/docker-test","registry-mirrors":["https://old.example"]}\n' \
  > "$TMP/daemon.json"
FLAG_YES=1
KAPOJ_DAEMON_JSON="$TMP/daemon.json"
KAPOJ_DOCKER_MIRRORS="https://new.example"
mirror_reachable() { return 0; }
ensure_mirrors >/dev/null
python3 - "$TMP/daemon.json" <<'PY'
import json, sys
cfg = json.load(open(sys.argv[1]))
assert cfg["data-root"] == "/var/lib/docker-test", cfg
assert cfg["registry-mirrors"] == ["https://new.example"], cfg
PY
pass "ensure_mirrors preserves other keys and replaces mirrors"

# 11. existing daemon.json + no python3 -> fail, file unchanged
cp "$TMP/daemon.json" "$TMP/daemon-before.json"
if (KAPOJ_DAEMON_JSON="$TMP/daemon.json" KAPOJ_PYTHON3=python3-missing \
  ensure_mirrors >/dev/null 2>&1); then
  fail "ensure_mirrors should fail without python3"
fi
cmp -s "$TMP/daemon.json" "$TMP/daemon-before.json" || fail "daemon.json changed without python3"
pass "ensure_mirrors refuses to overwrite without python3"

# 12. custom cert copies files and writes directive
mkdir -p "$TMP/docker/caddy/certs"
printf 'cert\n' > "$TMP/fullchain.pem"
printf 'key\n' > "$TMP/privkey.pem"
printf 'KAPOJ_DOMAIN=\nKAPOJ_TLS_DIRECTIVE=\nKAPOJ_TLS_VERIFY=1\n' > "$ENV_FILE"
DOMAIN=oj.example.com
CERT_CRT="$TMP/fullchain.pem"
CERT_KEY="$TMP/privkey.pem"
TLS_VERIFY=1
deploy_custom_cert
[ "$(cat "$TMP/docker/caddy/certs/fullchain.pem")" = "cert" ] || fail "cert not copied"
[ "$(cat "$TMP/docker/caddy/certs/privkey.pem")" = "key" ] || fail "key not copied"
grep -q '^KAPOJ_TLS_DIRECTIVE=tls /etc/caddy/certs/fullchain.pem /etc/caddy/certs/privkey.pem$' "$ENV_FILE" \
  || fail "TLS directive not written"
pass "deploy_custom_cert copies files and writes directive"

# 13. external database is refused
declare -F cmd_promote >/dev/null || fail "cmd_promote is not defined"
printf 'KAPOJ_DATABASE_URL=postgres://external/db\n' > "$ENV_FILE"
if (cmd_promote someone >/dev/null 2>&1); then
  fail "promote should refuse KAPOJ_DATABASE_URL"
fi
pass "promote refuses external database"

echo "all deploy helper tests passed"
