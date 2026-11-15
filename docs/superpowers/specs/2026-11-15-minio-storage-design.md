# 全平台 MinIO 存储改造设计

> 日期：2026-11-15
> 范围：Server、Judge、备份/恢复、存量数据迁移
> 目标：把全部业务对象存储统一到 MinIO，消除本地目录与数据库生命周期不一致导致的文件丢失。

## 1. 背景

当前所有上传文件都落在 `KAPOJ_STORAGE_DIR`：

- 徽章图片、头像、证书图片、站点 Logo/Favicon
- 题目测试数据、checker、题面附件
- AI 生成测试数据
- 平台/账号备份 ZIP
- AI/邮件加密密钥文件（安全材料）

数据库只保存 key 或内部 URL。Postgres 与文件目录使用不同的 Docker 卷；存储卷被删除/重建后，数据库引用仍然存在，最终表现为大量 404，且没有可靠的跨主机/跨容器共享能力。

## 2. 目标与非目标

### 目标

- MinIO 成为 Server 与 Judge 共用的业务对象存储。
- 保持现有 object key 与数据库字段不变：`badges/<id>`、`avatars/...`、`certificates/...`、`problems/...`、`site/...`。
- 备份 ZIP 也进入 MinIO；创建/恢复使用本地临时文件，不把临时文件当持久化存储。
- 提供一次性迁移命令，把现有 `apps/server/data` 中的业务文件复制到 MinIO，并校验 key。
- 保留 `local` 后端用于单元测试/离线开发；Compose 与生产默认 `minio`。
- 启动时继续执行存储引用审计：缺失对象输出明确警告。
- 不改变公开 API 结构：图片仍由后端代理输出，不暴露 MinIO 凭证或预签名 URL。

### 非目标

- 不修改数据库表结构。
- 不改变用户上传大小限制、图片类型校验、权限和业务逻辑。
- 不把沙箱运行目录、编译产物、临时测试文件放进 MinIO；它们仍是本地临时文件。
- 不把 AI/邮件加密密钥放进 MinIO：MinIO 凭证与加密密钥不应形成循环依赖。生产使用环境变量或本地密钥卷。

## 3. 方案选择

### 3.1 存储后端

选用 **统一 ObjectStore 接口 + MinIO 后端 + Local 后端**。

- Server 的 `storage.Storage` 继续服务现有业务；新增 `storage.ObjectStore`（追加 `List` / `DeletePrefix`）供备份使用。
- MinIO 使用 `github.com/minio/minio-go/v7`。
- Local 仅用于测试/离线开发，并保留现有 key 校验语义。

### 3.2 MinIO 桶布局

- 业务桶：`kapoj`（可配置），key 与现有路径一致。
- 备份桶：`kapoj-backups`（可配置），key 使用 `platform/*.zip`、`accounts/*.zip`、`safety/*.zip`、`uploads/*.zip`。
- 两个桶都由后端启动时幂等创建（可关闭自动创建，生产建议预先创建并限制权限）。

### 3.3 Judge

Judge 新增自己的 `storage.Store` 接口与 Local/MinIO 实现：

- 读取 checker/测试输入：`Open` → 本地临时文件 → 沙箱。
- 生成测试数据：`Put` 到 MinIO，`RemovePrefix` 清理失败重试残留。
- 沙箱、编译、执行目录仍使用本地临时目录，任务结束后清理。

### 3.4 备份/恢复

- 备份业务文件：通过 `ObjectStore.List` 枚举业务桶，流式写入 ZIP。
- 备份 ZIP：本地临时文件写完后 `Put` 到备份桶；数据库 `backup_packages.filename` 保存对象 key。
- 恢复：从备份桶下载到本地临时 ZIP → 校验 → 解压文件到临时目录 → 清空业务桶 → 上传恢复文件 → 恢复数据库；失败时用恢复前快照回滚。
- 上传恢复包：直接存到备份桶 `uploads/`，恢复任务保存对象 key；恢复完成后删除。
- 安全备份：写入备份桶 `safety/`，仍只保留最新一份。

### 3.5 迁移

新增 server 子命令：

```bash
/server migrate-storage
```

- 读取 `KAPOJ_STORAGE_DIR` 下的本地文件。
- 复制业务文件到 MinIO 业务桶，保留 key；跳过 `backups/`、`ai.key`、`mail.key`、`.gitkeep`。
- 单独扫描旧备份目录（`KAPOJ_BACKUP_DIR` 或 `<StorageDir>/backups`），把 ZIP 上传到备份桶，并把 `backup_packages.filename` / `backup_restore_jobs.uploaded_file` 中指向本地路径的记录改写为 object key。
- 输出 copied/skipped/legacy backup copied/updated 数量；失败非零退出。
- 迁移不删除本地文件，便于回滚。

## 4. 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `KAPOJ_STORAGE_BACKEND` | `local` | `local` 或 `minio`；Compose 默认 `minio` |
| `KAPOJ_MINIO_ENDPOINT` | `localhost:9000` | MinIO API 地址，不含协议 |
| `KAPOJ_MINIO_ACCESS_KEY` | 空 | 访问密钥 |
| `KAPOJ_MINIO_SECRET_KEY` | 空 | 秘密密钥 |
| `KAPOJ_MINIO_BUCKET` | `kapoj` | 业务桶 |
| `KAPOJ_MINIO_BACKUP_BUCKET` | `kapoj-backups` | 备份桶 |
| `KAPOJ_MINIO_USE_SSL` | `false` | 生产建议开启 |
| `KAPOJ_MINIO_REGION` | 空 | 可选 |
| `KAPOJ_MINIO_AUTO_CREATE_BUCKET` | `true` | 开发默认；生产可关闭 |
| `KAPOJ_STORAGE_DIR` | `data` | local 后端、密钥文件、迁移源 |
| `KAPOJ_REQUIRE_PERSISTENT_STORAGE` | `false` | 仅 local 后端生效 |

## 5. 失败与降级

- MinIO 不可达：启动失败（业务对象存储是强依赖）。
- 桶不存在且禁止自动创建：启动失败，提示桶名。
- 对象缺失：启动审计告警；徽章图片读取路径返回空 `imageUrl`，前端使用字母回退。
- 备份对象缺失：下载/恢复返回 404，不修改当前数据。
- 迁移中断：可重复执行，已存在的 key 覆盖写；本地文件不删除。

## 6. 测试

- Storage：
  - Local/MinIO 的 key 校验一致。
  - MinIO Put/Open/Delete/List/DeletePrefix（MinIO 集成测试，环境变量 gated）。
  - 启动迁移跳过密钥/备份目录。
- Server：
  - 现有业务测试全部用 Local 后端通过。
  - 备份创建/下载/恢复使用两个 Local ObjectStore 通过。
  - 备份文件名变为对象 key 后，下载与删除行为正确。
- Judge：
  - 现有 pipeline/generation 测试用 Local Store 通过。
  - MinIO gated 集成测试覆盖读写。
- Compose：
  - MinIO 服务启动、桶自动创建、Server/Judge 健康运行。
  - 用户主页徽章/头像/证书图片可读，无 404。
  - 创建平台备份、恢复、下载验证对象流转。

## 7. 风险

- 备份 refactor 涉及文件系统到对象存储的语义变化，必须保持 ZIP 格式与安全校验不变。
- Judge 与 Server 的 MinIO 配置必须一致（endpoint、bucket、凭证、region）。
- MinIO 本身也需要持久化卷；Compose 使用宿主机绑定目录 `minio-data`，不把对象再放回容器可写层。
- 生产环境应使用 TLS、非 root 凭证、预创建桶和生命周期策略；Compose 默认值仅用于开发。
