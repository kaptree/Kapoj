#!/bin/sh
set -eu

# Runs only on first initialization of the Postgres data volume.
# POSTGRES_USER / POSTGRES_DB come from the official image environment.
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set ON_ERROR_STOP=1 \
  --set=testdb=kapoj_test \
  --set=owner="$POSTGRES_USER" <<'EOSQL'
CREATE DATABASE :"testdb" OWNER :"owner";
EOSQL
