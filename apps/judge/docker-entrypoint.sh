#!/bin/sh
set -eu

# 默认使用 docker-compose 注入的 socket-proxy 地址；直连回退模式由
# docker-compose.direct.yml 设置 KAPOJ_JUDGE_DOCKER_DIRECT=1。
if [ "${KAPOJ_JUDGE_DOCKER_DIRECT:-0}" = "1" ]; then
  export DOCKER_HOST=unix:///var/run/docker.sock
fi

exec /judge "$@"
