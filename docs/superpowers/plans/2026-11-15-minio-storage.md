# 全平台 MinIO 存储 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Server、Judge、备份包和存量业务文件统一迁移到 MinIO，消除本地文件卷丢数据的问题。

**Architecture:** Server 与 Judge 各自保留薄存储接口；MinIO 实现 `Put/Open/Delete/List/DeletePrefix`，Local 实现继续供单元测试。备份服务改用两个 ObjectStore（业务桶 + 备份桶），ZIP 只作为本地临时文件。Judge 通过同一 MinIO 读写测试数据，沙箱仍用本地临时目录。

**Tech Stack:** Go 1.26、MinIO Go SDK、Docker Compose、Vue 不变。

**Spec:** `docs/superpowers/specs/2026-11-15-minio-storage-design.md`

## Global Constraints

- 不修改数据库表结构、公开 API 字段或现有 object key。
- 不把 AI/邮件密钥放入 MinIO；生产使用环境变量或本地密钥卷。
- Compose 默认 `KAPOJ_STORAGE_BACKEND=minio`；Local 仅测试/离线。
- MinIO 桶默认 `kapoj` / `kapoj-backups`，对象由后端代理读取，不暴露预签名 URL。
- 失败时不得清空现有业务对象；恢复失败必须回滚文件。
- `go test -p 1 ./...`（server）与 judge 全量测试必须通过。

---

### Task 1: Storage 抽象与 MinIO 后端

**Files:**
- Modify: `apps/server/internal/storage/storage.go`
- Modify: `apps/server/internal/storage/local.go`
- Create: `apps/server/internal/storage/minio.go`
- Create: `apps/server/internal/storage/local_object_test.go`
- Modify: `apps/server/go.mod` / `go.sum`

**Interfaces:**
- `type ObjectInfo struct { Key string; Size int64 }`
- `type ObjectStore interface { Storage; List(ctx,prefix) ([]ObjectInfo,error); DeletePrefix(ctx,prefix) error }`
- `type MinIOOptions struct { Endpoint, AccessKey, SecretKey, Bucket, Region string; UseSSL, AutoCreate bool }`
- `func NewMinIO(ctx context.Context, opts MinIOOptions) (*MinIO, error)`
- `func (m *MinIO) EnsureBucket(ctx context.Context) error`
- `func (m *MinIO) List(ctx context.Context, prefix string) ([]ObjectInfo, error)`
- `func (m *MinIO) DeletePrefix(ctx context.Context, prefix string) error`
- `func ValidateKey(key string) error`

- [ ] Add tests for Local List/DeletePrefix and key validation.
- [ ] Add MinIO implementation using a temp-file spool to enforce max bytes before upload; `Open` uses `GetObject` + `Stat` for not-found; `List` uses `ListObjects`; `DeletePrefix` uses `RemoveObjects`.
- [ ] Add `storage.NewMinIO` bucket ensure and retry.
- [ ] Run `go test ./internal/storage` and `go vet ./...`.

### Task 2: Server 配置、启动 Wiring、Compose 与迁移命令

**Files:**
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/cmd/server/main.go`
- Create: `apps/server/internal/storage/migrate.go`
- Create: `apps/server/internal/storage/migrate_test.go`
- Modify: `docker/docker-compose.yml`
- Modify: `.env.example`
- Create: `minio-data/.gitkeep` maybe via `.gitignore`

**Interfaces:**
- Config fields: `StorageBackend`, `MinIOEndpoint`, `MinIOAccessKey`, `MinIOSecretKey`, `MinIOBucket`, `MinIOBackupBucket`, `MinIOUseSSL`, `MinIORegion`, `MinIOAutoCreateBucket`.
- `func MigrateLocalToStore(ctx context.Context, sourceDir string, dst storage.ObjectStore) (MigrateResult, error)`.
- `MigrateResult{Copied, Skipped, Failed int64}`.
- Server main selects backend; `migrate-storage` subcommand runs migration and exits.

- [ ] Add MinIO compose service with healthcheck, persistent host bind mount, server/judge env.
- [ ] Add config defaults/validation and server backend selection.
- [ ] Add migration command for business objects (skip keys/`.gitkeep`) and legacy backup ZIPs, rewriting legacy backup DB paths to object keys.
- [ ] Run server tests and compose config validation.

### Task 3: Judge ObjectStore + MinIO

**Files:**
- Modify: `apps/judge/internal/storage/reader.go`, `writer.go`
- Create: `apps/judge/internal/storage/store.go`, `local.go`, `minio.go`
- Modify: `apps/judge/internal/config/config.go`
- Modify: `apps/judge/internal/pipeline/pipeline.go`
- Modify: `apps/judge/internal/generation/pipeline.go`
- Modify: `apps/judge/cmd/judge/main.go`
- Modify: `apps/judge/go.mod` / `go.sum`
- Tests: pipeline/generation/worker tests use Local store.

**Interfaces:**
- `type Store interface { Open(ctx,key)(io.ReadCloser,error); Put(ctx,key,io.Reader,int64)(int64,error); RemovePrefix(ctx,key) error }`
- `func NewLocal(root string) (*Local,error)`
- `func NewMinIO(ctx context.Context, opts MinIOOptions) (*MinIO,error)`
- `pipeline.New(repo, store, sandbox, publisher, ...)`
- `generation.New(repo, store, sandbox)`

- [ ] Add Store interface and Local implementation.
- [ ] Add MinIO implementation (same bucket layout and env as server).
- [ ] Switch pipeline/generation to Store; download testcases to temp, generated files to store.
- [ ] Update tests to seed Local store; run judge tests.

### Task 4: Backup Service ObjectStore 化

**Files:**
- Modify: `apps/server/internal/backup/service.go`
- Modify: `apps/server/internal/backup/admin.go`
- Modify: `apps/server/internal/backup/platform.go`
- Modify: `apps/server/internal/backup/account.go`
- Modify: `apps/server/internal/backup/files.go`
- Modify: `apps/server/internal/backupapi/handler.go`
- Modify tests: `apps/server/internal/backup/*_test.go`, `apps/server/internal/backupapi/handler_test.go`

**Interfaces:**
- `backup.Options{Files storage.ObjectStore; Packages storage.ObjectStore; WorkDir string; ...}`
- `backup_packages.filename` stores package object key.
- `SaveUpload` returns an object key under `uploads/`.
- `DownloadPackage(ctx,id) (io.ReadCloser,string,error)` for API streaming.
- Helpers: `addStoreObjects`, `copyStoreToDir`, `copyDirToStore`, `addStoreFile`.

- [ ] Refactor file snapshot to list/read business store; exclude `backups/`.
- [ ] Refactor package create/download/delete/safety backup to package store.
- [ ] Refactor platform/account restore to download ZIP, use temp dirs, clear/upload business store, rollback on failure.
- [ ] Adapt backup API download to stream object.
- [ ] Update tests to Local ObjectStores; run backup/backupapi tests and server full suite.

### Task 5: Compose/MinIO 集成与文档

**Files:**
- `docker/docker-compose.yml`
- `.env.example`
- `README.md`, `运行手册.md`, `docs/数据备份与恢复.md`
- `.gitignore`

- [ ] Add MinIO service, console port, bucket env, server/judge env, minio-data bind mount.
- [ ] Document production TLS/credentials/bucket precreation, migration command, backup now in MinIO.
- [ ] Run `docker compose config`, rebuild server/judge, start MinIO, run migration, verify user profile images and backup create/download.

### Task 6: Full Verification

- [ ] `go test -p 1 ./...` (server)
- [ ] `go test ./...` (judge)
- [ ] `go vet ./...` both
- [ ] `docker compose ... up -d --build minio server judge`
- [ ] `migrate-storage`
- [ ] Verify `/api/users/33/profile`, badge/avatar/certificate image endpoints, backup create/download/restore.
- [ ] Ensure no 404s and no page errors.
