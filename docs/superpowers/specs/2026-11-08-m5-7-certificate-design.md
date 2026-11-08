# M5-7 证书认证设计

> 目标：用户提交比赛证书图片，管理员审核通过/驳回；通过后生成唯一验证码，任何人可用验证码核验证书类型、等级、持有者昵称与通过日期；证书图片仅本人与管理员可见。

## 1. 背景与现状

- `DEVELOPMENT.md` §3.7 已给出证书/证书类型字段；§5.6 给出功能需求与接口草案。
- 当前项目只有本地文件存储抽象（`storage.Storage`），MinIO 未实现；徽章图片上传使用本地 storage + 公开图片接口，证书图片需要更严格的访问控制。
- RBAC 目前 36 个权限码（`ai.settings.manage` 为第 36 个）；管理后台已有统一 `AdminNav`、`AppPageHeader`/`AppSurface`/`DataTable`/`Dialog`/`Toast`/`Confirm` 模式。
- 用户个人中心入口在 `UserMenu`（我的主页/装扮管理/编辑资料/AI 设置）；尚无证书页面。
- M4-3 有站内通知，但证书需求未要求审核通知；本次不做通知。

## 2. 目标与非目标

### 目标

- 证书类型：内置蓝桥杯、ACM-ICPC、CCCC 天梯赛、CCPC、NOI、NOIP、USACO；管理员可新增/编辑/启用停用/删除未被引用的类型。
- 用户：任意登录用户上传证书图片、选择类型、填写等级，提交认证；查看自己的证书列表与状态；查看被驳回原因；通过后查看验证码并复制验证链接。
- 允许同一用户对同一证书类型提交多张证书（例如不同年份/不同等级）；不做 `(user,type)` 唯一约束，重复/无效由管理员驳回。
- 管理员：审核列表（默认 pending，可按状态筛选、分页），查看证书图片与用户信息，通过/驳回；驳回必须填写原因。
- 通过：生成唯一验证码（`KAPOJ-XXXX-XXXX`，大小写不敏感），公开验证页/API 返回类型、等级、持有者昵称、通过日期，不返回图片、邮箱、用户 ID、审核人等敏感信息。
- 图片：仅本人与 `certificate.manage` 管理员可通过鉴权接口读取；本地 storage 保存，前端用鉴权 blob 加载预览；公开验证页不展示图片。
- 新增权限 `certificate.manage`（第 37 个，分组「证书」，默认仅 `super_admin`）；审核与类型 CRUD 写审计。
- OpenAPI/生成类型、用户页、公开验证页、管理员页、文档与全量检查。

### 非目标

- 不接入 MinIO/对象存储；复用本地 `storage.Storage`。不实现图片 CDN/签名 URL。
- 不做证书审核站内通知、邮件通知、证书撤销/重新审核、证书删除、用户撤回申请。
- 不把证书展示到公开用户主页；通过验证码页面核验即可。个人中心 `/settings/certificates` 展示自己的证书。
- 不做 OCR/自动识别证书、图片裁剪/压缩、证书 PDF、二维码生成图片（验证链接可复制，二维码后续）。
- 不改用户头像（M5-8）、不改题目/比赛/游戏化逻辑。

### 反目标

- 证书图片通过公开 URL、storage key 或公开验证接口泄露。
- 驳回原因/审核人/邮箱出现在公开验证结果。
- 未登录用户可读取他人证书图片。
- 证书类型删除导致历史证书丢失类型信息。
- 证书图片上传绕过大小/类型校验。

## 3. 方案与取舍

### 方案 A（采纳）：独立 `certificate` 领域 + 本地 storage + 鉴权图片接口 + 三个页面

- 新增 `apps/server/internal/certificate`（模型/仓储/服务）与 `certificateapi`；`storage.Storage` 保存 `certificates/<id>`；`GET /api/certificates/:id/image` 仅 owner/admin；前端用 `http.getBlob` 加载并 `URL.createObjectURL` 预览。
- 前端新增 `/settings/certificates`（我的证书/提交）、`/verify`（公开核验）、`/admin/certificates`（审核 + 类型管理）。
- 优点：领域清晰、权限/审计/图片访问边界明确；复用现有 UI/存储/分页模式；不引入新依赖。
- 代价：新增一个后端领域与三处前端页面；图片预览需要 blob 加载，略多于普通 `<img>`。

### 方案 B（未采纳）：把证书放进 `user` 域并复用 `userapi`

- 优点：文件少。
- 缺点：用户资料/认证/审核/图片混杂；权限与审计边界不清；后续扩展证书类型困难。

### 方案 C（未采纳）：用户只填写外链图片 URL

- 优点：实现最简。
- 缺点：不满足“上传证书图片”，外链可能失效/泄露，无法保证图片仅本人和管理员可见。

### 图片访问方式

- 采纳鉴权接口 + blob：图片 key 不暴露，公开验证无图；前端通过带 Bearer 的请求读取二进制。
- 未采纳公开图片 URL：违反“证书图片仅本人与管理员可见”。
- 未采纳签名 URL：需新增签名/过期机制，当前无此基础设施，超出 M5-7。

## 4. 数据模型

### 4.1 certificate_types

```text
certificate_types (
  id bigserial primary key,
  name varchar(50) not null unique,
  description varchar(200) not null default '',
  is_active boolean not null default true,
  sort_order int not null default 0,
  created_at timestamptz not null,
  updated_at timestamptz not null
)
```

- Seed（首次启动且表为空时插入；已有数据不覆盖管理员修改）：蓝桥杯、ACM-ICPC、CCCC 天梯赛、CCPC、NOI、NOIP、USACO；sort_order 1..7。
- 删除规则：有任意证书引用时禁止删除（40001「已有证书使用该类型，请先停用」）；无引用可删除。停用后公开类型列表不返回，历史证书仍显示提交时快照的类型名。允许改名，历史证书/公开核验仍显示快照名。

### 4.2 certificates

```text
certificates (
  id bigserial primary key,
  user_id bigint not null,
  certificate_type_id bigint not null,
  certificate_type_name varchar(50) not null, -- 提交时快照，类型改名不影响历史
  level varchar(50) not null,
  image_key varchar(255) not null,
  image_content_type varchar(100) not null,
  status varchar(20) not null default 'pending', -- pending/approved/rejected
  reject_reason varchar(500) not null default '',
  verify_code varchar(32) null unique,            -- approved 后生成
  submitted_at timestamptz not null,
  reviewed_at timestamptz null,
  reviewed_by bigint null,
  created_at timestamptz not null,
  updated_at timestamptz not null
)
-- index(user_id, submitted_at desc)
-- index(status, submitted_at)
-- unique(verify_code)
```

- `image_key` 形如 `certificates/<id>`，只在服务端使用；不通过 DTO 返回 storage key。
- `certificate_type_name` 在提交时从当前类型名称快照；历史证书/公开核验使用快照，类型改名不回溯。类型停用/删除规则仍按 `certificate_type_id` 引用检查。
- `verify_code` 为 `*string`，pending/rejected 为 NULL；Postgres 唯一索引允许多个 NULL。
- `reviewed_by` 为管理员用户 ID；驳回原因只对本人/管理员可见。
- 不建证书删除/软删除；历史保留。

## 5. 后端设计

### 5.1 领域包

- `certificate.Model`：`CertificateType`、`Certificate`、`Status`（pending/approved/rejected）。
- `certificate.Repository`（GORM）：
  - 类型：`ListTypes(ctx, includeInactive)`, `FindType`, `CreateType`, `UpdateType`, `DeleteType`（引用检查）。
  - 证书：`Create`, `Find`, `ListMine(userID, page, size)`, `ListAdmin(filter, page, size)`, `Review(id, reviewerID, status, reason, verifyCode, now)`（仅 pending），`FindByVerifyCode(code)`, `FindImage(id)`.
- `certificate.Service`：
  - `ListTypes(includeInactive bool, canManage bool)`。
  - `CreateType/UpdateType/DeleteType`（权限在 handler；服务校验字段）。
  - `Submit(ctx,userID,typeID,level,image io.Reader)`：类型 active；level trim 1–50；读取 ≤5 MiB；`http.DetectContentType` 必须 PNG/JPEG/WebP；插入 pending → image key `certificates/<id>` → storage.Put；失败删除行；返回个人视图。
  - `ListMine`, `ListAdmin`.
  - `GetImage(ctx, id, viewer UserID, canManage bool)`：仅 owner/admin；否则 404；返回 bytes/contentType。
  - `Review(ctx, actorID, id, action, reason)`：仅 pending；approve 生成唯一 code，reject 要求 reason 1–500；事务更新 + 审计；返回视图。
  - `Verify(ctx, code)`：规范化 code；approved 才返回 valid；返回类型名/等级/昵称/approvedAt；未知/pending/rejected 返回 `{valid:false}` 200。
- 验证码：`crypto/rand` + 无歧义大写字母/数字（去掉 I/L/O/U/0/1），格式 `KAPOJ-XXXX-XXXX`；唯一冲突重试；查找大小写不敏感。

### 5.2 权限与审计

- 新增 `rbac.PermCertificateManage = "certificate.manage"`，分组 `PermissionGroupCertificate = "证书"`，默认仅 super_admin；权限总数 36 → 37。
- 用户提交/我的证书/图片：登录即可；图片仅本人或 `certificate.manage`。
- 管理员类型 CRUD/审核列表/审核：`certificate.manage`。
- 审计：
  - 类型创建/更新/删除：action `certificate.manage`，target `certificate_type`，summary 中文。
  - 审核：action `certificate.manage`，target `certificate`，targetID 证书 ID，summary「通过/驳回证书」；驳回原因不写审计（避免敏感文本），只写结果。
- 公开验证不需要权限/登录。

### 5.3 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/certificate-types` | 公开 | 启用的证书类型（id/name/description/sortOrder） |
| POST | `/api/certificates` | 登录 | multipart：`typeId`、`level`、`file`（≤5 MiB，PNG/JPEG/WebP） |
| GET | `/api/certificates/my` | 登录 | 我的证书分页（含 imageUrl/status/rejectReason/verifyCode） |
| GET | `/api/certificates/:id/image` | 登录 + owner/admin | 原始图片；非 owner/admin 404 |
| GET | `/api/certificates/verify?code=...` | 公开 | 公开核验；返回 valid + 类型/等级/昵称/approvedAt |
| GET | `/api/admin/certificates?status=&page=&page_size=` | `certificate.manage` | 审核列表，默认 pending |
| POST | `/api/admin/certificates/:id/review` | `certificate.manage` | `{action:"approve"|"reject", reason?}`；仅 pending |
| GET | `/api/admin/certificate-types?includeInactive=true` | `certificate.manage` | 全部类型 |
| POST | `/api/admin/certificate-types` | `certificate.manage` | `{name,description,isActive,sortOrder}` |
| PUT | `/api/admin/certificate-types/:id` | `certificate.manage` | 同上 |
| DELETE | `/api/admin/certificate-types/:id` | `certificate.manage` | 仅未被引用 |

- 上传使用 `uploads` 路由组（40 MiB body）；服务层限制文件 5 MiB，超出 40001「证书图片不能超过 5 MiB」；multipart 解析错误映射中文。
- 错误码：401/403/404/40001；无新错误码。
- DTO：
  - `CertificateTypeView{id,name,description,isActive,sortOrder}`。
  - `CertificateView{id,type,level,status,imageUrl,rejectReason,verifyCode,submittedAt,reviewedAt,user?}`；`type` 使用提交时快照名；`user` 仅管理员列表返回 `{id,username,nickname}`。
  - `PublicCertificateView{valid,type?,level?,nickname?,approvedAt?}`。
  - `CertificatePage{total,items}`。
- 公开验证不返回 `imageUrl`/`userId`/`email`/`reviewedBy`/`rejectReason`。

### 5.4 图片安全

- `http.DetectContentType` 校验 magic bytes；允许 `image/png`、`image/jpeg`、`image/webp`。
- 文件读取 `io.LimitReader(max+1)`；超限返回 40001；空文件 40001。
- 存储 key 在插入后使用 `certificates/<id>`；DB 只存 key/contentType；DTO 只给鉴权图片 URL。
- 非 owner/admin 的图片请求返回 404（不暴露存在性）。
- 公开验证不会读取/返回图片。

## 6. 前端设计

### 6.1 我的证书 `/settings/certificates`

- 路由 `certificate-settings`，`requiresAuth`；UserMenu 增加「我的证书」入口。
- 页面：`AppPageHeader`（标题「我的证书」+「申请认证」按钮）、`AppSurface`、`DataTable`/列表、空态、加载/错误。
- 列表字段：类型、等级、状态 Tag（待审核/已通过/已驳回）、提交时间、审核时间、驳回原因（仅 rejected）、验证码 + 复制验证链接（approved）、图片预览。
- 提交 Dialog：类型 `Select`（active types）、等级 `InputText`（1–50）、图片文件选择（≤5 MiB，PNG/JPEG/WebP，本地 object URL 预览）；提交后 Toast + 刷新。
- 图片预览：`getCertificateImage(id): Promise<Blob>`（带 Authorization），`URL.createObjectURL`；卸载/刷新时 revoke。列表缩略图按需加载；失败显示占位。
- 复制验证链接：`${location.origin}/verify?code=${verifyCode}`；复制按钮 Toast。
- 驳回原因在列表/Dialog 中显示；approved 显示 verifyCode。

### 6.2 公开核验 `/verify`

- 路由 `certificate-verify`（public，无需登录）；支持 `?code=...` 自动核验。
- 页面：AppPageHeader + AppSurface，验证码 `InputText` +「核验」Button；结果卡片：
  - valid：类型、等级、持有者昵称、通过日期；绿色 Tag「验证有效」。
  - invalid：中性提示「未找到有效证书，请检查验证码」。
- 不显示证书图片、邮箱、用户 ID、审核信息。
- 验证码格式提示 `KAPOJ-XXXX-XXXX`；大小写/空格容错由后端 normalize。

### 6.3 管理后台 `/admin/certificates`

- 路由 `admin-certificates`，`permission: certificate.manage`；AdminNav「内容」组增加「证书审核」。
- Tabs：
  - **证书审核**：状态 `Select`（待审核/全部/已通过/已驳回）、`DataTable`（用户/类型/等级/提交时间/状态/操作）、分页；默认 pending。审核 Dialog 显示图片（blob）、用户、类型、等级、提交时间、驳回原因（若已审）；通过/驳回按钮；驳回需填原因（Textarea，1–500）。
  - **证书类型**：类型 `DataTable`（名称/描述/排序/启用/操作）；新建/编辑 Dialog（name 1–50、description ≤200、isActive、sortOrder）；删除确认；被引用时显示 40001 中文错误。
- 空态/加载/错误/Toast/Confirm 遵循现有模式；不新增依赖。

### 6.4 API 客户端与类型

- `api/certificates.ts`：类型列表、提交（multipart）、我的证书、图片 Blob、公开核验；`api/admin.ts`：管理列表、审核、类型 CRUD。
- `http` 增加 `getBlob(url)`（带 Bearer/refresh 的二进制 GET，不解析 Envelope）；证书图片使用它。
- `types.ts` 别名 + OpenAPI 生成；`permissions.ts` 增加 `certificate.manage` 与 `ADMIN_ACCESS_PERMISSIONS`。
- UserMenu/AdminNav/路由 meta 更新；OpenAPI 新增 paths/schemas。

## 7. 安全与隐私

- 公开验证只返回类型/等级/昵称/通过日期；不返回图片/邮箱/userId/审核人/驳回原因。
- 证书图片仅 owner 或 `certificate.manage`；非授权 404；不暴露 storage key。
- 上传 magic bytes 校验 + 5 MiB 限制；不信任扩展名/MIME header。
- 管理员审核/类型 CRUD 使用 `certificate.manage` + 审计；用户提交仅登录即可。
- 验证码 crypto 随机、唯一、不可枚举；大小写不敏感。
- 公开核验不在 M5-7 增加额外限流：8 位高熵验证码 + 未知/未通过统一返回 `{valid:false}`，不提供存在性 oracle；若未来需要可复用 Redis 限流，不阻塞本次。
- 不新增外部服务；不写日志中的图片内容/验证码全量；审计不写验证码。

## 8. 测试与验收

### 后端

- 类型：seed 幂等；公开列表只 active；管理员 includeInactive；CRUD 校验；被引用删除 40001；停用后公开不返回、历史证书仍可读。
- 提交：未登录 401；类型不存在/停用 40001；level 空/超长 40001；空图/超 5 MiB/非图片 40001；成功 pending、image key/contentType、我的列表可见。
- 图片：owner 200 正确 Content-Type；其他用户 404；管理员 200；不存在 404；不带 token 401。
- 审核：非管理员 403；pending approve 生成唯一 code、状态/审核人/时间；重复审核 40001；reject 无原因 40001、有原因成功；驳回原因仅 owner/admin；审计 action/target/summary。
- 公开验证：approved 返回类型/等级/昵称/approvedAt；pending/rejected/未知返回 `{valid:false}` 200；大小写/空格容错；不返回图片/userId/email/rejectReason。
- 分页/权限/DB 迁移/索引/权限目录 37。

### 前端

- 我的证书：加载/空/错误；提交校验/成功/失败；状态 Tag/驳回原因/验证码复制；图片 blob 预览与 object URL 清理。
- 公开核验：输入/自动 query/有效/无效/加载错误。
- 管理页：审核列表默认 pending、筛选/分页；通过/驳回（原因必填）；图片预览；类型 CRUD/停用/删除被引用错误；权限/导航。
- OpenAPI/生成类型/lint/build。

### 真实冒烟

- 临时用户提交（临时小 PNG）→ pending；管理员审核通过 → verifyCode；公开验证接口/页面返回类型/等级/昵称/日期且无图片；owner 图片 200、其他用户 404、管理员 200；驳回流程（原因必填、公开验证 invalid）；类型 CRUD（被引用删除 40001）；清理临时证书/图片/类型/审计/用户，不删除既有数据。
- 浏览器可用时检查三页面的移动端/暗亮主题；不可用则记录并用组件测试覆盖。

## 9. 交付拆分

- CERT-A：证书类型/证书模型、迁移、seed、权限 `certificate.manage`、类型 CRUD API。
- CERT-B：用户提交/我的证书/图片鉴权读取/公开验证 API。
- CERT-C：管理员审核 API + 审计 + 状态机/验证码。
- CERT-D：OpenAPI/生成类型 + `http.getBlob` + 我的证书/公开核验前端。
- CERT-E：管理员证书审核/类型管理前端 + 导航/权限。
- CERT-F：文档、全量检查、真实冒烟。

## 10. 留给实施计划的决定

- 包/文件拆分（`certificate`/`certificateapi`）与构造函数签名。
- 图片 blob 缓存/object URL 生命周期细节。
- 管理员列表筛选参数与分页默认值。
- 验证码生成/规范化/冲突重试的具体实现。
- 证书类型排序/删除校验的具体错误文案。
- 手工浏览器验收是否可用。
