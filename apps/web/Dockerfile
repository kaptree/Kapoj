# 构建上下文必须是仓库根目录（pnpm workspace + lockfile + 已提交的 src/api/schema.d.ts）
FROM node:22-alpine AS build
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV npm_config_registry=$NPM_REGISTRY
ENV COREPACK_NPM_REGISTRY=$NPM_REGISTRY
WORKDIR /repo
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY apps/web/package.json apps/web/
RUN corepack enable && corepack pnpm install --frozen-lockfile --filter web...
COPY apps/web apps/web
RUN corepack pnpm --filter web build

FROM caddy:2-alpine
COPY docker/caddy/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /repo/apps/web/dist /srv
EXPOSE 80 443
