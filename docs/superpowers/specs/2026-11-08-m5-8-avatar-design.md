# M5-8 头像上传设计

> 目标：用户可在个人资料页上传头像图片；上传头像优先于外链头像，填写外链会替换上传；支持移除头像；头像图片公开读取，旧文件在替换/移除时清理。

## 1. 背景与现状

- 用户模型只有 `avatar_url`（外链），个人资料页提示“头像上传将在后续版本提供”。
- 徽章已有本地 storage 图片上传/读取模式（`gamification` + `storage.Storage`），头像可复用。
- 项目未接入 MinIO；当前 storage 抽象为本地文件系统。M5-8 使用本地 storage，不新增对象存储依赖。
- 用户头像出现在 `UserMenu`、`UserProfileView`、`IdentityShowcase`、社交摘要、测评记录、比赛榜单等处，均读取 `avatarUrl`；上传后必须让这些位置自动使用新头像。

## 2. 目标与非目标

### 目标

- 新增本人头像上传 `POST /api/users/me/avatar`（multipart `file`）与移除 `DELETE /api/users/me/avatar`；返回最新 `Me`。
- 新增公开头像读取 `GET /api/users/:id/avatar`；无上传头像/文件缺失返回 404。
- 上传图片 ≤2 MiB，仅 PNG/JPEG/WebP/GIF（magic bytes 校验）；存储 key `avatars/<userID>/<random>`，DB 保存 key 与 contentType。
- 上传成功：`avatar_url` 指向内部头像接口并带版本参数；清除旧上传文件；`Me.avatarUploaded=true`。
- 外链头像保留：非空 `avatarUrl` 保存时替换上传头像并清除上传文件；空 `avatarUrl` 不清除已上传头像；移除头像清空外链与上传。
- 个人资料页增加上传/移除/预览；`MeDTO` 与 `ProfileDTO` 增加 `avatarUploaded`，个人页与管理员编辑页遇到上传头像时不把内部 URL 填入外链输入；上传后所有展示位置通过 `avatarUrl` 自动更新。
- 兼容现有用户：已有外链头像不受影响；无头像用户默认首字母。

### 非目标

- 不接 MinIO/对象存储；不实现裁剪/压缩/转码/审核/敏感图检测。
- 不提供管理员为他人上传头像；管理员仍可编辑外链（会替换上传）。
- 不新增权限码；本人上传/移除只需登录，头像读取公开。
- 不改变用户资料其他字段、社交/榜单/测评展示逻辑。
- 不做头像历史版本/回收站；旧文件替换后删除。

### 反目标

- 上传后 `UserMenu`/个人主页/榜单仍显示旧头像。
- 外链输入把内部头像 URL 当作外链保存，导致上传被清除。
- 上传失败/DB 失败后旧头像丢失或留下半套文件。
- 未登录用户可上传/移除他人头像。
- 头像图片无限大小/任意类型。

## 3. 方案与取舍

### 方案 A（采纳）：本地 storage + 上传优先 + 公开读取接口

- 在 `users` 表增加 `avatar_key`/`avatar_content_type`；`avatar_url` 仍是所有展示位置读取的有效 URL。
- 上传写随机 key，DB 更新 `avatar_url=/api/users/<id>/avatar?v=<unix>`、key/contentType；删除旧 key。
- 公开 `GET /api/users/:id/avatar` 按 key 读取并返回 contentType；URL 带 v 缓存破坏，响应可长缓存。
- 个人资料页增加上传/移除，保留外链输入；上传后外链输入清空，保存非空外链时替换上传。
- 优点：复用现有 storage/展示链路；改动集中在 user 域；无新依赖；缓存/旧文件清理明确。
- 代价：新增两个用户列与一个公开读取路由；外链/上传二选一的语义需要前端提示。

### 方案 B（未采纳）：用户只上传，不保留外链

- 优点：语义最简。
- 缺点：破坏现有外链头像用户；管理员编辑头像外链功能需移除或失去意义。

### 方案 C（未采纳）：头像 base64 存 DB

- 优点：无需 storage。
- 缺点：用户表膨胀、查询变慢、备份变大；与现有图片存储模式不一致。

### 方案 D（未采纳）：MinIO/对象存储

- 优点：生产可扩展。
- 缺点：当前未接入，超出 M5-8；本地 storage 已满足。

## 4. 数据模型

```text
users (
  ... existing ...
  avatar_url varchar(512) not null default '', -- 有效 URL（外链或内部头像接口）
  avatar_key varchar(255) not null default '', -- storage key，仅内部上传
  avatar_content_type varchar(100) not null default ''
)
```

- `avatarUploaded` 为计算字段（`avatar_key != ''`），不单独存列。
- 内部 URL：`/api/users/<id>/avatar?v=<unix>`；v 在每次上传/移除后变化，用于缓存破坏。
- 现有 `avatar_url` 外链数据不变；`avatar_key` 为空表示无上传。

## 5. 后端设计

### 5.1 用户服务

- `user.Service` 增加可选 `storage.Storage`（`NewService(repo, opts...)` 或 `WithStorage`，不破坏现有构造）。
- `UploadAvatar(ctx,id,io.Reader) (*User,error)`：
  1. 读 ≤2 MiB+1；空/超限/`http.DetectContentType` 非 PNG/JPEG/WebP/GIF → 40001。
  2. 随机 key `avatars/<id>/<hex>`；`store.Put`；失败返回。
  3. DB 更新 `avatar_key`、`avatar_content_type`、`avatar_url`（带 v）。
  4. DB 失败删除新文件；成功删除旧 key（若不同，best-effort）。
- `DeleteAvatar(ctx,id)`：清空 `avatar_key/content_type/avatar_url`；删除旧文件（best-effort）。
- `AvatarImage(ctx,id) (*AvatarImage,error)`：读取 `avatar_key`；空/缺失 → 404；返回 bytes/contentType。
- `UpdateMe`/admin update 语义：
  - `avatarUrl` 非空：校验 http(s)，设置 `avatar_url`，清空 `avatar_key/content_type`；本人路径 best-effort 删除旧文件（admin 路径 rbac 服务不持有 storage，旧文件留待后续清理，记录 minor）。
  - `avatarUrl` 为空字符串：只清空外链，保留上传头像（避免表单把内部 URL 当外链提交时误删）。
- `MeDTO` 与 `ProfileDTO` 都增加 `avatarUploaded bool`；公开资料只多一个布尔值，不含 storage key。管理员编辑页用 `ProfileDTO.avatarUploaded` 判断是否把外链输入留空。
- 所有展示查询继续读 `avatar_url`，无需改社交/榜单/测评。

### 5.2 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/users/me/avatar` | 登录 | multipart `file`；返回最新 `Me` |
| DELETE | `/api/users/me/avatar` | 登录 | 移除上传/外链头像；返回 `Me` |
| GET | `/api/users/:id/avatar` | 公开 | 上传头像原始图片；无上传/文件缺失 404 |

- 上传走 `uploads` 路由组（大 body），服务层限制 2 MiB；multipart 错误中文。
- `GET` 响应 `Content-Type` 来自 DB；可设 `Cache-Control: public, max-age=31536000, immutable`（URL 带 v）。
- 不新增权限码；本人路由 `RequireAuth`；公开路由在无认证 `public` 组。
- 错误：401/404/40001；无新错误码。

### 5.3 图片安全

- magic bytes 校验；允许 PNG/JPEG/WebP/GIF；2 MiB。
- 随机 key，避免覆盖旧图；上传成功删除旧 key；失败清理新 key。
- 不把 storage key 返回给客户端；仅返回内部 URL。
- 公开读取只返回图片字节；不做目录遍历（storage 已校验 key）。

## 6. 前端设计

### 6.1 个人资料页 `/settings/profile`

- 现有 `avatarUrl` 外链输入保留。
- 新增头像预览（当前有效 `avatarUrl` 或上传预览）、文件选择（2 MiB/PNG/JPEG/WebP/GIF）、`上传头像` 按钮、`移除头像` 按钮（仅 `avatarUploaded` 或存在外链时显示）。
- 上传成功：更新 auth store 的 `Me`，清空外链输入，Toast；上传中禁用。
- 移除成功：更新 store，清空外链输入，Toast。
- 保存资料：非空外链在保存后替换上传（后端清理 key）；空外链保留上传。页面提示「填写外链会替换已上传头像；留空则保留上传头像」。
- `Me.avatarUploaded` 用于加载时不要把内部 URL 填进外链输入；`avatarPreview` 优先上传预览，其次外链，最后当前 `auth.user.avatarUrl`。
- 上传/移除按钮与保存按钮互斥 loading；错误显示 Message/Toast。
- `api/users.ts` 增加 `uploadAvatar(file): Promise<Me>`、`deleteAvatar(): Promise<Me>`；`http.postForm`/`http.delete`。
- 管理员用户编辑（`AdminUsersView`）：`getProfile` 返回 `avatarUploaded`；为 true 时外链输入留空并提示「该用户已上传头像；留空保留，填写外链会替换上传」；保存空外链保留上传，填写非空外链由 `AdminUpdateUser` 清空 `avatar_key/content_type`。
- 其他页面（UserMenu/UserProfile/榜单/测评）无需改动，`avatarUrl` 自动更新。

### 6.2 OpenAPI/类型

- `Me` 与 `Profile` 增加 `avatarUploaded`。
- paths：POST/DELETE `/users/me/avatar`、GET `/users/{id}/avatar`（binary）。
- `corepack pnpm gen:api`；types 别名无需新类型（Me 已有）。
- 不新增依赖。

## 7. 测试与验收

### 后端

- Upload：合法 PNG/JPEG/WebP/GIF；空/超 2 MiB/文本 40001；成功 key/contentType/avatarUrl/avatarUploaded；旧文件删除。
- Delete：清空字段/删除文件；无上传时幂等；返回 Me。
- AvatarImage：上传后 200 + contentType；无上传/缺失 404；公开无 auth。
- 外链语义：非空外链清空 key/contentType 并删除旧文件；空外链保留上传；admin update 非空外链清空 key（文件清理 minor）。
- `ProfileDTO.avatarUploaded` 正确；管理员编辑页遇到上传头像时外链留空、保存空外链不改上传、填写外链替换。
- `Me`/`Profile` 的所有现有调用不回归；`avatarUploaded` 正确。
- 权限：上传/移除未登录 401；不能操作他人（路由只有 me）；公开读取无 auth。

### 前端

- ProfileEditView：加载时上传头像不填入外链输入；上传校验/成功/失败；移除；外链替换上传提示；预览优先级；保存空外链保留上传；Toast/加载态。
- `api/users` 方法；UserMenu/UserProfile 头像 URL 更新（可复用现有 spec）。
- lint/build。

### 真实冒烟

- 临时用户上传 1x1 PNG → Me.avatarUploaded=true、avatarUrl 内部 URL；`GET /users/:id/avatar` 200；UserMenu/Profile 显示；再上传新图 → 旧 URL v 变化、旧文件删除；保存非空外链 → 上传清除、内部接口 404；移除 → 清空并 404；清理临时用户/文件。浏览器可用则检查个人资料页；不可用记录。

## 8. 交付拆分

- AV-A：用户模型/迁移、service 上传/移除/读取、外链语义、Me.avatarUploaded、API、测试。
- AV-B：OpenAPI/类型、ProfileEditView 上传/移除/预览、api/users、测试。
- AV-C：文档、全量检查、真实冒烟。

## 9. 留给实施计划的决定

- `user.NewService` 的可选 storage 注入签名（Option）。
- 旧文件删除的 best-effort 顺序与 admin 外链清理 minor。
- 前端上传预览的 object URL 生命周期。
- 头像图片格式是否包含 GIF（本设计包含）。
- 手工浏览器验收是否可用。
