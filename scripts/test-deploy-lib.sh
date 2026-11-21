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
