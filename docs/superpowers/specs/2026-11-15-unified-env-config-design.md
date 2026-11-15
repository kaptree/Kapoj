# 统一配置文件（.env）设计

> 日期：2026-11-15
> 范围：Server、Judge、Docker Compose、Postgres、MinIO、AI/邮件加密密钥
> 决策：后台可编辑的站点/SMTP/AI Provider 设置继续保存在数据库；启动/部署配置、密钥和组件默认账号统一到根目录 `.env`。

## 1. 为什么是 `.env`

Docker Compose、PostgreSQL 官方镜像和 MinIO 都原生读取环境变量，而 Compose 会自动加载仓库根目录的 `.env`。因此 `.env` 是唯一能同时被：

- Compose（`${POSTGRES_*}` / `${MINIO_ROOT_*}` 插值）
- Server / Judge（`env_file` 或 `KAPOJ_CONFIG_FILE`）
- Postgres / MinIO 容器

使用的单文件格式。YAML 需要额外的 env 生成/自定义 entrypoint，反而会产生第二个配置源。

## 2. 文件

- 实际配置：仓库根目录 `.env`，git-ignored，权限 0600。
- 模板：`.env.example`，提交到仓库，只含占位符/开发默认值。
- Server/Judge 可选：`KAPOJ_CONFIG_FILE=/path/to/.env`，在没有 Compose 注入环境变量时直接读取同一文件。
- 唯一保留的启动环境变量是 `KAPOJ_CONFIG_FILE`；其他配置全部在 `.env`。

## 3. 内容分组

```dotenv
# PostgreSQL container
POSTGRES_USER=kapoj
POSTGRES_PASSWORD=...
POSTGRES_DB=kapoj
KAPOJ_DB_HOST=postgres
KAPOJ_DB_PORT=5432

# Redis
KAPOJ_REDIS_ADDR=redis:6379

# MinIO container + Server/Judge access
MINIO_ROOT_USER=kapoj
MINIO_ROOT_PASSWORD=...
KAPOJ_MINIO_ENDPOINT=minio:9000
KAPOJ_MINIO_BUCKET=kapoj
KAPOJ_MINIO_BACKUP_BUCKET=kapoj-backups
KAPOJ_STORAGE_BACKEND=minio

# Secrets
KAPOJ_JWT_SECRET=...
KAPOJ_AI_ENCRYPTION_KEY=...
KAPOJ_MAIL_ENCRYPTION_KEY=...

# Server, Judge, backup, timeouts, images...
```

Server/Judge 在 `KAPOJ_MINIO_ACCESS_KEY/SECRET_KEY` 为空时回退到 `MINIO_ROOT_USER/PASSWORD`，避免同一密钥写两遍。数据库 URL 为空时由 `POSTGRES_* + KAPOJ_DB_HOST/PORT` 构造，`KAPOJ_DATABASE_URL` 仍可作为显式覆盖。

## 4. Compose

- `postgres` / `minio`：从 `.env` 的 `POSTGRES_*` / `MINIO_ROOT_*` 插值；缺失则 Compose 直接失败。
- `server` / `judge`：`env_file: ../.env`，读取同一文件；不再逐个维护 environment。
- Server/Judge 不再挂载 `apps/server/data` 存放密钥；AI/邮件密钥在 `.env`，业务对象在 MinIO。
- `.env` 在仓库根目录，Compose 命令从仓库根目录执行。

## 5. 密钥迁移

- 现有 AI 密钥从 `apps/server/data/ai.key` 读取并写入 `.env` 的 `KAPOJ_AI_ENCRYPTION_KEY`，保持现有用户 AI Key 可解密。
- 没有现有邮件密钥时生成一个新的 base64 32 字节密钥写入 `.env`；当前 SMTP 未配置，因此不会破坏任何密文。
- 密钥文件逻辑保留为兼容回退，但配置文件中提供密钥后来源为 `env`，后台 AI 密钥板块只读，轮换需编辑 `.env` 并重启。
- `.env` 含敏感密钥，必须 0600、git-ignored、独立备份；平台备份不包含 `.env`。

## 6. 本地开发

Compose 是主路径。直接 `go run` 时：

```bash
KAPOJ_CONFIG_FILE="$PWD/.env" \
KAPOJ_DB_HOST=localhost KAPOJ_REDIS_ADDR=localhost:6380 \
KAPOJ_MINIO_ENDPOINT=localhost:9000 KAPOJ_STORAGE_DIR="$PWD/apps/server/data" \
go run ./cmd/server
```

非空环境变量优先于配置文件，便于本地覆盖 Compose 服务名。

## 7. 非目标

- 不迁移 `site_settings`、`mail_settings`、`ai_settings`、`ai_providers` 等数据库运行时可编辑配置。
- 不改变公开 API、权限或管理后台功能。
- 不把 `.env` 放入 Docker 镜像或 Git。
