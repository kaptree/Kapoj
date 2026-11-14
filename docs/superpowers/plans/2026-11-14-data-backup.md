# 数据备份与恢复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付全平台/账号 ZIP 备份、恢复、预设定时任务和后台管理页。

**Architecture:** Go 内逐表 `COPY ... CSV` 流式写入 ZIP；恢复走 schema 校验 + 事务 COPY + 文件暂存/替换。备份/恢复任务由 Asynq `backup` 队列执行，服务内 ticker 扫描预设计划。全平台恢复成功后 graceful shutdown + `syscall.Exec` 自动重启。

**Tech Stack:** Go + Gin + GORM + pgx + Asynq + PostgreSQL、Vue 3 + PrimeVue、Vitest。

**Spec:** [`docs/superpowers/specs/2026-11-14-data-backup-design.md`](../specs/2026-11-14-data-backup-design.md)

## Global Constraints

- 备份目录默认 `<StorageDir>/backups`，不进入全平台文件快照。
- 每类备份只保留最新成功包；失败不删除旧包。
- 全平台包包含数据库全部 public 业务表 + 业务文件 + `ai.key`/`mail.key`；账号包不包含平台内容。
- 账号恢复按原 `user_id`；用户名/邮箱冲突中止；完整快照覆盖密码哈希。
- 全平台恢复要求确认短语、自动安全备份、单全局恢复锁、schema 一致。
- ZIP 不加密，仅 `system.backup.manage` 可操作/下载；所有写操作写审计。
- 恢复/上传限制：默认 2 GiB 上传、20 GiB 解压、200k 条目。
- 所有新增接口更新 OpenAPI 并生成前端类型；先写失败测试。

## Review Focus

- Zip Slip/超限条目不能写出备份目录或撑爆磁盘。
- schema 不匹配、错误 kind、损坏 CSV 必须在中止前不修改现有数据。
- 恢复失败必须回滚数据库事务；文件替换失败要恢复旧文件。
- 账号恢复不能误删/覆盖其他用户数据。
- 新备份失败不能删除旧包；并发备份不能写同一个 latest 文件。

---

### Task 1: 备份模型、归档与数据库导出/恢复

**Files:**
- Create: `apps/server/internal/backup/model.go`
- Create: `apps/server/internal/backup/archive.go`
- Create: `apps/server/internal/backup/database.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/db/db.go`
- Tests: `apps/server/internal/backup/archive_test.go`、`database_test.go`

**Interfaces:**
- `Manifest{FormatVersion int, Kind Kind, CreatedAt time.Time, SchemaHash string, DatabaseVersion string, Tables []string, Users []int64, FileCount int, UncompressedBytes int64}`
- `Database.Dump(ctx, w io.Writer, skipTables []string) (Manifest, error)`
- `Database.Restore(ctx, r io.Reader, manifest Manifest) error`
- `SchemaHash(ctx) (string, error)` 基于 public 表/列/类型排序。

- [x] **Step 1: 写失败测试**：schema hash 稳定；CSV 往返；错误 schema 拒绝；manifest/zip 路径校验。
- [x] **Step 2: 实现模型/归档/数据库 COPY**。
- [x] **Step 3: 运行 `go test -p 1 ./internal/backup`。**
- [x] **Step 4: 提交。**

### Task 2: 全平台备份/恢复与文件替换

**Files:**
- Create: `apps/server/internal/backup/platform.go`
- Create: `apps/server/internal/backup/files.go`
- Tests: `platform_test.go`、`files_test.go`

**Interfaces:**
- `Service.CreatePlatformBackup(ctx, actorID int64) (*Package, error)`
- `Service.RestorePlatform(ctx, actorID int64, zipPath string) error`
- 文件遍历排除 `backups/` 与临时恢复目录；恢复前安全备份、文件暂存/回滚。

- [x] **Step 1: 测试文件快照、排除备份目录、恢复回滚。**
- [x] **Step 2: 实现 ZIP 写入（manifest + database + files）、临时文件原子替换、旧包删除。**
- [x] **Step 3: 实现全平台恢复与安全备份。**
- [x] **Step 4: 测试并提交。**

### Task 3: 账号备份/恢复

**Files:**
- Create: `apps/server/internal/backup/account.go`
- Tests: `account_test.go`

**Interfaces:**
- `Service.CreateAccountBackup(ctx, actorID int64, userID int64) (*Package, error)`（0 = 全部账号）
- `Service.RestoreAccount(ctx, actorID int64, zipPath string, userID int64) error`
- 账号表白名单 + 过滤条件集中定义；恢复替换关联数据并重算题目计数。

- [x] **Step 1: 测试单账号往返、全部账号包、密码/角色/金币恢复、用户名冲突中止、其他用户数据不变。**
- [x] **Step 2: 实现表过滤导出、文件收集、事务恢复。**
- [x] **Step 3: 测试并提交。**

### Task 4: 计划任务、任务队列与自动重启

**Files:**
- Create: `apps/server/internal/backup/queue.go`
- Create: `apps/server/internal/backup/scheduler.go`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/cmd/server/main.go`
- Tests: `scheduler_test.go`

**Interfaces:**
- `Schedule{Kind, Enabled, Frequency, RunHour, RunMinute, Weekday, DayOfMonth, NextRunAt, LastRunAt, LastStatus, LastError}`
- `NextRun(after time.Time) time.Time`
- `Queue` 使用 Asynq 队列 `backup`，任务 `backup.create`、`backup.restore`。
- 全平台恢复成功 3 秒后触发 `RestartRequested()`；main graceful shutdown 后 `syscall.Exec`。

- [x] **Step 1: 测试 daily/weekly/monthly next run、due claim、禁用计划。**
- [x] **Step 2: 实现队列、worker、ticker；接入 main。**
- [x] **Step 3: 测试并提交。**

### Task 5: 备份 API、RBAC 与 OpenAPI

**Files:**
- Create: `apps/server/internal/backupapi/handler.go`
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/rbac/catalog.go`、`seed.go`
- Modify: `packages/api/openapi.yaml`
- Tests: `backupapi/handler_test.go`、RBAC tests

**Interfaces:**
- 新权限 `system.backup.manage`，审计 target `backup`。
- `GET /api/admin/backups`
- `POST /api/admin/backups/platform`
- `POST /api/admin/backups/accounts` body `{userId?}`
- `GET /api/admin/backups/:id/download`
- `PUT /api/admin/backups/schedules/:kind`
- `POST /api/admin/backups/restore/platform`（上传或 `{packageId}`）
- `POST /api/admin/backups/restore/account`（上传或 `{packageId, userId}`）
- `GET /api/admin/backups/restores/:id`
- `POST /api/admin/backups/restart`

- [x] **Step 1: 测试权限、创建/下载、调度、恢复参数校验、审计。**
- [x] **Step 2: 实现 handler/路由/RBAC/OpenAPI。**
- [x] **Step 3: `pnpm gen:api`，测试并提交。**

### Task 6: 后台数据备份页面

**Files:**
- Create: `apps/web/src/views/AdminBackupView.vue`
- Create: `apps/web/src/api/backups.ts`
- Modify: `apps/web/src/api/types.ts`、`router/index.ts`、`components/layout/AdminNav.vue`、`permissions.ts`
- Tests: `AdminBackupView.spec.ts`、permissions/nav specs

**Interfaces:**
- 全平台/账号两个卡片、最新包、定时表单、上传/选择最新包恢复、确认对话框、状态轮询、自动重启提示。
- 账号备份可选择单个用户或全部账号；账号恢复选择目标用户。

- [x] **Step 1: 写失败组件测试。**
- [x] **Step 2: 实现 API、页面、导航和权限。**
- [x] **Step 3: 测试/lint/build，提交。**

### Task 7: 全量验证与部署

- [x] server/judge 顺序 `go test -p 1 ./...`
- [x] web 全量测试、lint、build
- [x] 本地真实创建/恢复测试（测试库/临时目录）
- [x] `git diff --check`，提交
- [x] 重建 server 容器，线上验证页面、定时配置、备份 ZIP 下载

## Self-Review

- 全平台/账号两条链路均覆盖创建、下载、恢复、保留策略。
- 恢复安全性、schedule、权限、前端页面各自有独立测试任务。
- 账号恢复明确排除平台内容并检查跨用户冲突。
