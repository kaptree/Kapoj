# 数据备份与恢复设计

**日期：** 2026-11-14
**状态：** 已实施（2026-11-15）
**范围：** 全平台备份/恢复、用户账号备份/恢复、定时任务、管理后台集成

## 1. 目标

为平台增加两类 ZIP 备份：

1. **全平台备份**：数据库全部业务数据 + `StorageDir` 下业务文件（包含 AI/邮件密钥文件），用于整站灾难恢复。
2. **用户账号备份**：单个用户的完整账号快照，用于账号级恢复；定时任务生成“全部账号”包。

两类备份都支持：

- 手动创建、下载；
- 定时任务（每天/每周/每月预设频率）；
- 从服务器保留的最新包恢复；
- 上传外部 ZIP 恢复；
- 新备份成功后立即删除上一份成功备份（失败不删除旧包）。

## 2. 已确认决策

- 账号备份为完整账号快照：用户资料（含密码哈希、邮箱、角色、金币、Rating）、头像、提交、金币流水、成就/任务/徽章/称号、学习进度、通知、关注、评价、证书、购买/推荐等；**不包含该用户创建的题目/比赛/学习路线等平台内容**。
- 账号恢复按原 `user_id`：存在则覆盖该账号资料并替换其关联数据；不存在则按原 ID 重建；用户名/邮箱被其他账号占用时中止并提示冲突。
- 定时账号备份生成一个包含全部账号的包，只保留最新一份；手动备份仍可只导出单个用户。
- 全平台备份包含 `data/ai.key`、`data/mail.key` 等密钥文件。备份包视为高敏感数据，只允许获得 `system.backup.manage` 的管理员下载。
- 定时频率使用预设：每天/每周/每月 + 时间，不支持自由 cron。
- 恢复来源同时支持服务器最新包和上传 ZIP。
- 全平台恢复前自动生成一份“恢复前安全备份”；恢复需要输入确认短语。
- ZIP 不加密，依赖 HTTPS、权限与服务器文件权限。
- 恢复仍存在的账号时覆盖其密码哈希（完整快照）。
- 全平台恢复成功后由后台按钮触发受控自动重启；账号恢复不重启服务。

## 3. 非目标

- 不备份 Redis 会话/缓存（登录会话不跨恢复保留）。
- 不做云存储、异地复制、对象存储生命周期管理。
- 不做备份包密码加密；依赖管理员权限、HTTPS 和服务器文件权限保护 ZIP。
- 不支持跨数据库大版本/schema 版本的恢复；schema 不匹配必须拒绝。
- 账号恢复不重启服务；仅全平台恢复触发受控自动重启。

## 4. 方案对比

### 方案 A：`pg_dump` + `tar`（未采用）

优点：数据库恢复最完整，工具成熟。
缺点：当前 server 镜像是 distroless，没有 `pg_dump/pg_restore`；需要把镜像改成带 PostgreSQL 客户端的发行版，或新增 sidecar，部署复杂度高。恢复还依赖数据库客户端版本。

### 方案 B：Go 内逐表 `COPY ... CSV` + ZIP（采用）

用 pgx 的 `COPY TO STDOUT` 把每张 public 表流式写入 ZIP 的 `database/<table>.csv`；恢复时先校验包，再在事务中 `COPY FROM STDIN`。业务文件按 `StorageDir` 遍历写入 `files/`。

优点：不依赖外部二进制；流式处理，内存占用稳定；账号备份可复用同一套表读取逻辑；可在恢复前完整校验 schema 与文件。
缺点：需要处理外键、序列和 schema 版本；恢复要求数据库角色能设置 `session_replication_role=replica`（当前 Docker 的 kapoj 用户是超级用户）。

### 方案 C：文件系统快照（未采用）

直接复制 PostgreSQL 数据目录和 `data/`。恢复需要停库/停机，跨机器和版本不可移植，不适合后台在线操作。

## 5. ZIP 格式

全平台包：

```text
manifest.json
database/<table>.csv
files/<storage-key>...
```

账号包（单用户）：

```text
manifest.json
account/user.json
account/<table>.csv
account/files/<storage-key>...
```

全部账号包：

```text
manifest.json
accounts/<userID>/user.json
accounts/<userID>/<table>.csv
accounts/<userID>/files/<storage-key>...
```

`manifest.json`：

```json
{
  "formatVersion": 1,
  "kind": "platform | account | accounts",
  "createdAt": "2026-11-14T12:00:00Z",
  "schemaHash": "sha256:...",
  "databaseVersion": "PostgreSQL 17.x",
  "tables": ["users", "..."],
  "users": [1, 2, 3],
  "fileCount": 123,
  "uncompressedBytes": 123456
}
```

- `schemaHash` 由当前 public 表名、列名、类型排序后计算；恢复包必须与当前库一致。
- `users` 仅账号包使用；全平台包不含用户列表。
- 文件路径做 Zip Slip 校验；限制条目数、单文件大小和解压总大小（默认 2 GiB 上传、20 GiB 解压，配置可覆盖）。

## 6. 数据库备份/恢复

### 6.1 备份

- 读取 `information_schema.tables` 获取全部 public 基表。
- 每张表执行：
  ```sql
  COPY (SELECT * FROM "table") TO STDOUT WITH (FORMAT csv, HEADER true)
  ```
  直接写入 ZIP 条目，避免大表全量进内存。
- 包含 `db_migrations`，保证恢复后的迁移状态与包一致。

### 6.2 恢复

1. 读取并校验 manifest、schemaHash、ZIP 路径、文件 checksum。
2. 获取全局恢复锁，暂停备份定时调度。
3. 自动创建恢复前安全全平台备份（保留最新一份）。
4. 数据库事务内：
   - `SET LOCAL session_replication_role = replica` 跳过外键触发器；
   - `TRUNCATE <所有表> RESTART IDENTITY CASCADE`；
   - 逐表 `COPY FROM STDIN`；
   - 用 `pg_get_serial_sequence` 重置每张表的序列；
   - 提交。
5. 存储文件恢复：
   - 先将 ZIP 文件解压到 `StorageDir/.restore-<id>/` 并校验；
   - 将当前业务文件移动到临时目录（保留回滚），再移入新文件；
   - 任一步失败则回滚文件与数据库事务。
6. 写审计、更新备份状态，然后触发受控自动重启。

**自动重启：** 恢复成功并返回 HTTP 响应后，主进程进入 graceful shutdown，关闭 HTTP server/数据库连接，然后通过 `syscall.Exec` 以同一可执行文件、同一参数和环境变量重新启动自身。这样 AI/邮件密钥文件、配置与数据库连接全部重新加载，不依赖容器 restart policy。若平台不支持 `syscall.Exec` 或 re-exec 失败，记录错误并由部署层（systemd/Docker `restart: unless-stopped`）兜底。账号恢复不重启。

若数据库角色不允许 `session_replication_role=replica`，恢复返回明确错误，要求使用具备足够权限的数据库账号。

## 7. 账号备份/恢复

### 7.1 账号数据范围

包含 `users` 中该用户行，以及以下按用户归属的表（实现时集中配置白名单）：

- `submissions`、`coin_transactions`、`user_achievements`、`user_task_records`、
  `user_login_days`、`user_badges`、`user_titles`、`user_roles`、
  `user_learning_progress`、`learning_path_purchases`、`learning_path_recommendations`、
  `user_ai_configs`、`activity_logs`、`shared_items`、`rating_history`、
  `contest_participants`、`certificates`、`notifications`、
  `problem_solution_posts`、`problem_solution_replies`、`problem_solution_post_likes`、
  `problem_solution_unlocks`、`testcase_generations`、`testcase_generation_cases`、
  `user_evaluations`、`user_follows`。
- 文件：头像、证书图片、测试数据生成文件。
- **不包含** `problems` / `contests` / `learning_paths` / `announcements` / `tasks` / `ai_hints` 等由用户创建的平台内容。

账号包的每张表只包含该用户相关行；`user_evaluations` 包含 target/author 任一方为该用户的行；`user_follows` 包含 follower/followee 任一方为该用户的行。

### 7.2 恢复

1. 校验包与目标用户 ID；若包内没有该用户，报错。
2. 检查用户名、邮箱是否被其他账号占用；冲突则中止，不自动改名。
3. 事务内：
   - 若用户存在：更新 `users` 行（含密码哈希、角色、金币、Rating、状态）；
   - 若不存在：按原 ID 插入；
   - 删除该用户在账号白名单表中的现有行（关注/评价双向）；
   - 按原 ID 插入备份行；若主键与现存其他用户行冲突，整体回滚并提示；
   - 恢复 `user_roles`。
4. 重算该用户提交涉及题目的 `submit_count` / `accept_count`，重算学习路线参与人数。
5. 恢复该用户的头像、证书、测试生成文件；其他用户文件不动。
6. 写审计；提示管理员该用户需重新登录（旧会话可能仍有效，但资料已更新）。

## 8. 存储与保留策略

- 备份目录：`KAPOJ_BACKUP_DIR`，默认 `<StorageDir>/backups`。
- 全平台包：`platform/latest.zip`，只保留最新成功包。
- 全部账号包：`accounts/latest.zip`，只保留最新成功包。
- 单用户手动包：`accounts/user-<id>/latest.zip`，每个用户保留最新一份。
- 恢复前安全包：`safety/latest.zip`，保留最新一份。
- 新包先写临时文件，校验成功后再原子重命名；成功后删除旧包和旧元数据。备份失败绝不删除旧包。
- 备份文件不出现在全平台 `files/` 快照中（排除备份目录）。

## 9. 数据模型与接口

### 9.1 配置

- `KAPOJ_BACKUP_DIR`
- `KAPOJ_BACKUP_MAX_UPLOAD_BYTES`（默认 2 GiB）
- `KAPOJ_BACKUP_MAX_UNCOMPRESSED_BYTES`（默认 20 GiB）
- `KAPOJ_BACKUP_MAX_ENTRIES`（默认 200000）

### 9.2 表

```text
backup_packages
  id, kind(platform|accounts|account), user_id nullable,
  filename, size_bytes, status(running|ready|failed),
  error_message, schema_hash, created_by, created_at, completed_at

backup_schedules
  id, kind(platform|accounts) unique,
  enabled, frequency(daily|weekly|monthly),
  run_hour, run_minute, weekday, day_of_month,
  next_run_at, last_run_at, last_status, last_error,
  updated_by, updated_at

backup_restore_jobs
  id, kind(platform|account), package_id nullable, uploaded_filename,
  target_user_id nullable, status(running|success|failed),
  error_message, created_by, created_at, completed_at
```

### 9.3 API

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/admin/backups` | `system.backup.manage` | 最新包、定时配置、恢复任务状态 |
| POST | `/api/admin/backups/platform` | 同上 | 手动创建全平台备份 |
| POST | `/api/admin/backups/accounts` | 同上 | 手动创建账号备份；body `{userId?}`，空为全部账号 |
| GET | `/api/admin/backups/:id/download` | 同上 | 下载 ZIP |
| PUT | `/api/admin/backups/schedules/:kind` | 同上 | 更新预设定时任务 |
| POST | `/api/admin/backups/restore/platform` | 同上 | `multipart` 上传 ZIP，或 `{packageId}` |
| POST | `/api/admin/backups/restore/account` | 同上 | `{packageId, userId}` 或上传 ZIP + userId |
| GET | `/api/admin/backups/restores/:id` | 同上 | 恢复任务状态 |

所有写操作记录审计，target type `backup`。备份创建/恢复均为后台任务，API 返回 202 + job/package ID，前端轮询状态。

### 9.4 权限

新增权限码 `system.backup.manage`，归入“系统”权限组；内置角色默认仅超级管理员拥有（超级管理员天然绕过）。管理员可在角色权限页授予其他自定义角色。

## 10. 定时任务

- 服务启动后运行 `BackupScheduler`，每 30 秒扫描 `backup_schedules`。
- 使用 `FOR UPDATE SKIP LOCKED` 原子领取到期任务，计算下一次执行时间，写入 `next_run_at`。
- 任务投递到 Asynq 独立 `backup` 队列；worker 与 server 同进程启动，并发 1。
- 手动备份也走同一队列，避免并发写同一个 latest 文件。
- 恢复任务独占全局锁；恢复期间排队中的备份任务等待，恢复完成后继续。
- 后端保存 `last_status/last_error`，前端展示上次结果与下次执行时间。

## 11. 前端

新增 `/admin/backups` 页面，挂在后台“系统”分组：

- 全平台备份卡片：最新包状态/大小/时间、立即备份、下载、恢复、定时开关与频率；
- 账号备份卡片：选择用户或全部账号、立即备份、下载、恢复；定时全部账号；
- 恢复对话框：选择服务器最新包或上传 ZIP；全平台恢复要求输入 `RESTORE`；账号恢复选择目标用户；
- 显示进度/失败原因；恢复完成后提示重启；
- 明确提示：备份包含密码哈希与密钥，属于高敏感数据。

## 12. 验收标准

- [ ] 全平台 ZIP 包含全部数据库表与业务文件（含密钥），可在同 schema 环境完整恢复。
- [ ] 账号 ZIP 包含完整账号快照，不包含用户创建的平台内容。
- [ ] 账号恢复按原 user_id 覆盖/重建，用户名/邮箱冲突时中止并提示。
- [ ] 新备份成功后删除旧包；失败保留旧包。
- [ ] 每天/每周/每月定时任务可保存、可执行、可查看上次结果和下次时间。
- [ ] 恢复支持服务器最新包和上传 ZIP；全平台恢复前自动生成安全备份，需确认短语。
- [ ] 所有备份/恢复接口要求 `system.backup.manage`，下载不公开，写操作有审计。
- [ ] 非法 ZIP、schema 不匹配、Zip Slip、超限文件、错误类型恢复均被拒绝且不破坏现有数据。
- [ ] 管理后台页面、OpenAPI、前端类型、后端/前端测试与构建全部通过。

## 13. 已确认

- 全平台恢复完成后允许后台按钮触发受控自动重启；实现为 graceful shutdown + `syscall.Exec`，失败由部署层兜底。
- 备份 ZIP 不加密，依赖 HTTPS、`system.backup.manage` 权限和服务器文件权限。
- 账号恢复按完整账号快照覆盖密码哈希。
