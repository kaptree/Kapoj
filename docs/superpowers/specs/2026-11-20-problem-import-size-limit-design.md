# 题包导入大小上限设计

> 日期：2026-11-20
> 状态：待评审
> 关联：题包导入 `/problems/import/package`、`/problems/import/hydro`、`/admin/site`

## 1. 目标

在管理后台配置题包压缩包大小上限，并让 kapOJ/HydroOJ 题包导入支持到 **2 GiB**。

- 后台 `/admin/site` 新增「题库导入」Tab。
- 默认上限 **200 MiB**，可配置范围 **1–2048 MiB**。
- 两个导入接口都使用同一个上限：`POST /api/problems/import/package`、`POST /api/problems/import/hydro`。
- 解压上限按包上限自动放大：`max(1 GiB, 2 × 包上限)`；2 GiB 包对应 4 GiB 解压上限，仍保留 zip bomb 防护。
- 前端导入页从服务端读取当前上限，提示与客户端校验不再硬编码 200 MB。

## 2. 现状

- 后端 `problem.MaxHydroZipBytes = 200 MiB`，`ImportHydro`/`ImportPackage` 服务硬编码该值。
- 路由 `imports` 组的请求体上限是 **210 MiB**。
- 解压后总量默认上限 **1 GiB**，且声明大小的预扫描使用固定 `maxHydroUncompressed`。
- 前端 `ProblemImportView` 硬编码 `MAX_FILE_MB = 200`。
- 生产 Caddy 未限制请求体；自建 Nginx 需要配置 `client_max_body_size`。

## 3. 非目标

- 不做分片上传、断点续传、上传进度条。
- 不做异步导入队列；2 GiB 上传仍是一次同步请求，受 Nginx/Caddy/浏览器超时影响。
- 不改变题包内部的单文件/单题限制（测试点 16 MiB、单题测试数据 200 MiB、最多 100 题等）。
- 不新增第二个可配置的“解压上限”；解压上限由包上限推导。
- 不修改数据库已有题包、测试数据或导入结果。

## 4. 数据模型

新增迁移 `0004_problem_import_settings`：

```sql
CREATE TABLE public.problem_import_settings (
    id bigint PRIMARY KEY,
    max_package_mb integer NOT NULL DEFAULT 200,
    updated_by bigint NOT NULL DEFAULT 0,
    updated_at timestamp with time zone NOT NULL
);
INSERT INTO public.problem_import_settings (id, max_package_mb, updated_by, updated_at)
VALUES (1, 200, 0, now())
ON CONFLICT (id) DO NOTHING;
```

- 单例行 `id=1`。
- `max_package_mb` 校验 **1–2048**。
- 读取时若行不存在则懒创建默认行。
- 不新增权限码；沿用 `site.settings.manage`（与 `/admin/site` 页面一致）。

## 5. 后端接口

### 5.1 设置服务

新增 `internal/problemimport`：

```go
type SettingsView struct {
    MaxPackageMB         int       `json:"maxPackageMb"`
    MaxZipBytes          int64     `json:"maxZipBytes"`
    MaxUncompressedBytes int64     `json:"maxUncompressedBytes"`
    UpdatedAt            time.Time `json:"updatedAt"`
}

type SettingsInput struct {
    MaxPackageMB int `json:"maxPackageMb"`
}

type SettingsService struct{ db *gorm.DB }
func NewSettingsService(db *gorm.DB) *SettingsService
func (s *SettingsService) Get(ctx context.Context) (*SettingsView, error)
func (s *SettingsService) Update(ctx context.Context, actorID int64, in SettingsInput, audit func(*gorm.DB) error) (*SettingsView, error)
```

计算：

```text
MaxZipBytes = MaxPackageMB * 1 MiB
MaxUncompressedBytes = max(1 GiB, 2 * MaxZipBytes)
```

### 5.2 管理接口

```text
GET /api/admin/problems/import/settings     site.settings.manage
PUT /api/admin/problems/import/settings     site.settings.manage
```

`PUT` 在事务内更新并写审计：

```text
action: problem.import.settings
targetType: problem_import_settings
summary: 题包导入大小上限设为 2048 MB
```

### 5.3 导入页读取接口

```text
GET /api/problems/import/limits
需要 problem.create
```

返回：

```json
{
  "maxPackageMb": 200,
  "maxUncompressedMb": 1024
}
```

- 导入页在挂载时读取；失败时前端回退到 200 MB，不阻塞页面。
- 这是只读值，不包含敏感信息。

### 5.4 导入处理

- `problemapi.Options` 增加 `ImportSettings`（接口或 `*problemimport.SettingsService`）。
- 两个导入 handler 在 `parseMultipart` 前读取当前设置：
  - `maxZip = settings.MaxZipBytes`；无设置时回退 `MaxImportFileBytes` 或 200 MiB。
  - `maxUncompressed = settings.MaxUncompressedBytes`。
  - `maxBody = maxZip + 16 MiB`（multipart 开销）。
  - 若 `Content-Length > maxBody`，直接 413。
  - 用 `http.MaxBytesReader` 包裹请求体，按当前设置拒绝超限。
- `problem.HydroImportOptions` 和 `PackageImportOptions` 增加：
  ```go
  MaxZipBytes          int64
  MaxUncompressedBytes int64
  ```
- `ImportHydro`/`ImportPackage` 使用请求级上限；未提供时保持现有默认 200 MiB / 1 GiB。
- 声明大小预扫描和实际读取都使用 `MaxUncompressedBytes`，不再固定 1 GiB。
- `ImportPackage` 把两个上限继续传给内部 `ImportHydro`。
- 路由 `imports` 组请求体上限改为 **2064 MiB**（2048 + 16）；JSON/普通上传组不变。

## 6. 前端

### 6.1 后台设置

`AdminSiteView` 新增 Tab「题库导入」：

- 数字输入 `maxPackageMb`，范围 1–2048，单位 MB。
- 显示推导值：「解压上限约 X GB」。
- 保存按钮调用 `PUT /admin/problems/import/settings`，成功后 toast。
- 重新打开页面读取当前值。
- 组件 `AdminProblemImportSettings.vue` 独立管理加载/保存/错误。

### 6.2 导入页

`ProblemImportView`：

- `maxFileMb` 从 `GET /problems/import/limits` 读取，默认 200。
- 客户端校验、错误提示、页面说明都使用 `maxFileMb`。
- 请求失败时回退 200 MB，并在页面不显示额外错误（保持可导入）。
- 文件大小展示与现有 `formatSize` 保持一致。

## 7. 部署与运维

- 生产 Caddy 默认无请求体限制，不需要改。
- 自建 Nginx 需将 `client_max_body_size` 至少设为 **2g**（建议 3g，含 multipart 开销）；同时确认 `proxy_read_timeout` 足够上传 2 GiB。
- 服务器临时目录需要至少 **2 GiB** 空闲空间（multipart 会 spool 到临时文件）。
- 导入仍同步执行；超大包可能耗时较长，建议在低峰期操作。
- 运行手册补充上述 Nginx/磁盘/超时说明。

## 8. 测试

后端：

- 设置服务：默认 200、范围 1–2048、读取懒创建、更新审计失败回滚。
- 管理 API：权限 403、GET/PUT、审计摘要、非法值 40001。
- 导入服务：`MaxZipBytes` 小值拒绝；`MaxUncompressedBytes` 小值拒绝声明/实际超限；默认行为不变。
- 导入 handler：当前设置限制文件大小；`Content-Length` 预检；两个导入端点都生效；limits 接口。
- 迁移：0004 up/down 可执行。

前端：

- 后台设置组件：加载、输入范围、保存、错误。
- `AdminSiteView` 显示新 Tab。
- 导入页：读取 2048 后显示 2048 MB、200 MB+ 文件可用；读取失败回退 200 MB。
- 现有导入测试保持通过。

## 9. 风险

| 风险 | 对策 |
| --- | --- |
| 2 GiB 上传占满临时盘 | 文档提示；后续可加磁盘预检 |
| 超大包超时 | 保持同步；文档建议低峰与反代超时配置 |
| 解压上限放大到 4 GiB 带来资源压力 | 仍有 4 GiB 硬上限、单题/单文件限制和条目数限制 |
| 旧客户端仍按 200 MB 提示 | 前端发布后读取新上限；旧页面仅影响提示，服务端仍按新值校验 |
| 自建 Nginx 未放宽 | 运行手册明确 `client_max_body_size`，否则 413 |

## 10. 待确认

1. 默认 200 MiB、范围 1–2048 MiB、设置放在 `/admin/site` 新 Tab，是否同意？
2. 解压上限按 `max(1 GiB, 2 × 包上限)` 自动推导，是否同意？
3. 2 GiB 仅指上传压缩包大小；内部 16 MiB 单测试点、200 MiB 单题测试数据等限制保持不变，是否同意？
