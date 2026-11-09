# 修改密码与活跃会话管理设计

> 目标：用户可修改密码；查看当前账号的活跃会话（设备/IP/最近使用），单独退出某设备、退出其他设备或退出所有设备。修改密码后吊销全部会话并要求重新登录。

## 1. 背景与现状

- 密码使用 bcrypt；`user.ValidatePassword` 限制 8–72 字节。
- Refresh token 为 Redis 中的不透明随机串，只存 SHA-256；`RefreshStore` 已有 `Issue`/`Consume`/`Revoke`/`RevokeAll`，但未保存设备/IP/时间，也没有会话列表 API。
- 登录/注册/刷新返回 `TokenPair{accessToken,refreshToken}`；前端 `auth` store 持久化 token 与 user，没有 sessionId。
- 个人资料页没有修改密码入口；用户菜单只有退出登录。
- 认证路由组 `/api/auth/*` 无 `Authenticate`；用户自助路由在 `api` 认证组，使用 `RequireAuth`。

## 2. 目标与非目标

### 目标

- 修改密码 `PUT /api/users/me/password`：校验当前密码；新密码 8–72 字节且不能与当前相同；成功后 bcrypt 更新并吊销该用户全部会话；前端清空会话并跳转登录。复用 `LoginLimiter` 以 `password:<userID>` 为 key 限流（10 次/5 分钟），成功重置；超限返回新错误码 42904。
- 会话列表 `GET /api/users/me/sessions`：返回会话 ID、IP、User-Agent、创建时间、最近使用时间；不返回 token/hash。
- 单独退出 `DELETE /api/users/me/sessions/:id`：只能删除本人会话；删除当前会话等同退出登录。
- 退出其他设备 `POST /api/users/me/sessions/logout-others`：保留指定 sessionId（当前设备），吊销其他会话。
- 退出所有设备 `POST /api/users/me/sessions/logout-all`：吊销全部会话；前端清空并跳转登录。
- 登录/注册/刷新响应带 `sessionId`；刷新在同一会话内轮换 refresh token，不新增设备会话。
- 前端新增 `/settings/security` 页面（PrimeVue）与 UserMenu「账号安全」入口；显示当前设备标记、会话列表、退出操作、修改密码表单。

### 非目标

- 不做密码找回/邮箱验证/2FA/Passkey。
- 不做管理员查看/踢出他人会话；不做会话地理定位/设备指纹。
- 不做“记住我”、并发登录限制、会话数量上限。
- 不改变 access token 15 分钟/refresh token 7 天的配置语义；不做 token 黑名单。
- 不做审计（自助操作）；不改用户角色/资料。

### 反目标

- 修改密码后旧 refresh token 仍可刷新。
- 会话列表泄露 refresh token/hash。
- 用户能删除/退出他人会话。
- 刷新时把同一设备拆成多个会话，列表膨胀。
- 修改密码无需当前密码。

## 3. 方案与取舍

### 方案 A（采纳）：RefreshStore 升级为会话存储，会话 ID 稳定

- 每个 refresh token 关联一个 `sessionId`；刷新时轮换 token 但保持 sessionId。
- Redis：
  - `refresh:<hash>` -> JSON `{userId, sessionId}`（兼容旧值纯 userId，视为 legacy）。
  - `session:<id>` -> JSON `{userId, tokenHash, ip, userAgent, createdAt, lastUsedAt}`，TTL 与 refresh token 一致。
  - `user_refresh:<userId>` 集合（token hash）保留，用于快速 RevokeAll。
  - `user_sessions:<userId>` 集合（sessionId）。
- `Issue(userID, meta) -> token, sessionID`；`Consume(token) -> userID, sessionID`；`Rotate(userID, sessionID, meta) -> newToken`（更新 session 记录/token hash/lastUsedAt）；`List(userID)`；`RevokeSession`/`RevokeOthers`/`RevokeAll`。
- 优点：会话稳定、可单独踢出、兼容旧 token、Redis 泄露仍不可用 token；不需要 DB 迁移。
- 代价：RefreshStore 逻辑更复杂；需要处理旧值/过期/集合清理。

### 方案 B（未采纳）：每次刷新生成新会话

- 优点：实现简单。
- 缺点：同一设备每次刷新都会新增会话，列表膨胀；无法表达“设备会话”。

### 方案 C（未采纳）：会话存 PostgreSQL 表

- 优点：可审计/查询。
- 缺点：refresh token 高频轮换，DB 写入与清理成本高；当前 token 存 Redis，保持同层更自然。

## 4. 数据与 Redis 结构

```text
refresh:<sha256(token)> = {"userId":1,"sessionId":"<id>"}   TTL=refreshTTL
session:<sessionId>      = {"userId":1,"tokenHash":"<hash>","ip":"...","userAgent":"...",
                            "createdAt":"RFC3339","lastUsedAt":"RFC3339"} TTL=refreshTTL
user_refresh:<userId>    = SET<hash>       TTL=refreshTTL
user_sessions:<userId>   = SET<sessionId>  TTL=refreshTTL
```

- sessionId：16 字节 crypto/rand，hex/base64url；公开给本人，不含 token。
- IP/UA 截断（IP 45、UA 512），仅本人可见；空值允许。
- 兼容：`refresh:<hash>` 若解析为纯整数 userId，视为 legacy（sessionId 空）；刷新时创建新 sessionId 并写新结构。
- `Consume` 删除 token key、user_refresh 成员；不删除 session 记录（刷新轮换需要）。
- `Rotate` 更新 session 记录 tokenHash/lastUsedAt/IP/UA，写新 refresh key/set，并重置 session/user_refresh/user_sessions 的 TTL（滑动 7 天窗口）；旧 token 已消费。`List` 不延长 TTL。
- `List` 读取 `session:*`，跳过缺失/过期，清理 set 中无效项；返回 createdAt/lastUsedAt/ip/ua。
- `RevokeSession`：读 session 记录（校验 userID），删 refresh token key、session key、两个 set 成员。
- `RevokeOthers`/`RevokeAll`：按 session 集合逐个删除。
- 旧 `RevokeAll` 仍可用（清 user_refresh 集合 + 对应 token key；同时清 sessions）。

## 5. 后端设计

### 5.1 用户仓储

- `user.Repository.UpdatePassword(ctx,id,hash) error`（GORM + memory）。
- 不改 users 表结构。

### 5.2 认证服务

- `SessionMeta{IP, UserAgent string}`。
- `RegisterWithMeta`/`LoginWithMeta`/`RefreshWithMeta`（保留旧 `Register/Login/Refresh` 包装，meta 为空）：
  - `Issue` 生成 token+sessionId；`Session`/`TokenPair` 增加 `SessionID string json:"sessionId"`。
  - 登录 hook 不变。
- `ChangePassword(ctx,userID,currentPassword,newPassword) error`：
  1. 读取用户；bcrypt 校验当前密码（错误 40001「当前密码错误」）。
  2. `ValidatePassword(new)`；`new == current` → 40001「新密码不能与当前密码相同」。
  3. 生成新 hash；`LoginLimiter.Hit("password:<userID>")`（超限 42904；Redis 错误返回 503）；`RevokeAll(userID)`（失败则中止，不改密码）；`UpdatePassword`。
  4. 返回 nil；前端清空并重新登录。
  - 接受边界：`RevokeAll` 成功后 `UpdatePassword` 失败时，所有会话已失效但密码未变；用户可用旧密码重新登录，不做跨存储补偿（无 DB/Redis 事务）。
- `ListSessions(ctx,userID) ([]SessionView,error)`；`RevokeSession(ctx,userID,sessionID)`；`RevokeOthers(ctx,userID,keepSessionID)`；`LogoutAll(ctx,userID)`。
- `Logout(ctx,refreshToken)`：`Consume` → `RevokeSession`（当前会话）；幂等。
- `RefreshWithMeta`：`Consume` 得 userID/sessionId；若 legacy/空则新 sessionId；校验用户/封禁；`Rotate` 得新 refresh；签发 access；返回 `TokenPair`（含 sessionId）。
- 密码修改后 `RevokeAll` 清空所有会话。

### 5.3 API

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| PUT | `/api/users/me/password` | 登录 | `{currentPassword,newPassword}` → 204/200 |
| GET | `/api/users/me/sessions` | 登录 | `SessionView[]` |
| DELETE | `/api/users/me/sessions/:id` | 登录 | 删除本人会话 |
| POST | `/api/users/me/sessions/logout-others` | 登录 | `{sessionId}` 保留当前 |
| POST | `/api/users/me/sessions/logout-all` | 登录 | 吊销全部 |

- 认证路由放在 `api` 认证组：`auth.RegisterAccountRoutes(api, authSvc)`；`RequireAuth`。
- `SessionView{id,ip,userAgent,createdAt,lastUsedAt}`；不返回 token/hash/userId。
- 错误：401 未登录；40001 当前密码错误/新密码非法/相同；404 会话不存在/不属于本人；42904 修改密码尝试过多；503 Redis 不可用。
- `Session`/登录/注册/刷新响应加 `sessionId`；OpenAPI 更新。
- 无新权限；无审计。

### 5.4 前端

- `auth` store：`Persisted`/state 增加 `sessionId`；`setSession`/`setTokens` 保存（refresh 返回的 sessionId 保持）；`clear` 清空；`logout` 调用 `/auth/logout`。
- `api/auth.ts`：`changePassword`、`listSessions`、`revokeSession`、`logoutOthers`、`logoutAll`。
- 新页面 `/settings/security`（requiresAuth，UserMenu「账号安全」）：
  - 修改密码表单：当前密码/新密码/确认（PrimeVue Password），校验 8–72 字节、两次一致、不能与当前相同；成功后 Toast「密码已修改，请重新登录」，`auth.clear()` + 跳登录。
  - 会话列表：DataTable/卡片，显示设备摘要（User-Agent 截断/简单识别）、IP、创建/最近使用；当前 sessionId 显示「当前设备」Tag；行操作「退出此设备」；工具栏「退出其他设备」「退出所有设备」（ConfirmDialog）。
  - 退出其他设备：POST `logout-others`，保留 `auth.sessionId`，Toast + 刷新列表。
  - 退出所有设备/当前设备：调用 API 后 `auth.clear()` + 登录。
  - 空/加载/错误状态；移动端表格滚动；暗/亮主题。
- UserMenu 增加「账号安全」。
- OpenAPI/生成类型；不新增依赖。

## 6. 安全与测试

### 安全

- 修改密码必须验证当前密码；新密码 8–72 字节；成功后全部 refresh token 失效。
- 会话列表不返回 refresh token/hash；会话 ID 随机、仅本人可见。
- 删除/退出其他设备校验 sessionId 属于当前用户；未知/他人 404。
- IP/UA 仅本人可见；不写日志中的 token。IP 使用 `c.ClientIP()`，仅作展示（best-effort）；反向代理部署需配置 Gin trusted proxies，否则可能显示代理 IP。
- 修改密码限流 10 次/5 分钟（`password:<userID>`），成功重置；超限 42904，Redis 错误 503。
- 旧 refresh token 兼容（legacy）并在首次刷新迁移到新会话结构。
- 无新权限；自助操作。

### 后端测试

- RefreshStore：Issue/Consume/Rotate 同 session；List；RevokeSession/RevokeOthers/RevokeAll；legacy 纯 userId 兼容；过期/缺失清理；meta 保存。
- AuthService：注册/登录返回 sessionId；刷新保持 sessionId/滑动 TTL；修改密码校验/哈希/RevokeAll/旧 refresh 失效/限流 42904/RevokeAll 成功但 UpdatePassword 失败时密码未变且会话已吊销；logout 当前会话；list/revoke/others/all 权限校验。
- Handler：401/40001/404；session 列表不返回 token/hash；logout-others 保留当前；logout-all；修改密码成功/当前错误/新密码相同/过短。
- 现有登录/刷新/登出测试更新；MemoryRepository UpdatePassword。

### 前端测试

- auth store 持久化 sessionId；登录/refresh 更新；clear。
- SecurityView：加载会话/当前标记；退出其他/所有/单设备确认；修改密码校验/成功清空跳登录/错误；空/加载/错误；UserMenu 入口。
- OpenAPI/类型/lint/build。

### 真实冒烟

- 登录 A（设备 1）、再登录 A（设备 2）→ 列表 2 条；当前标记正确。
- 单独退出设备 2 → 列表 1；设备 2 refresh 失败；设备 1 正常。
- 退出其他设备（当前设备 1）→ 列表 1；其他全部失效。
- 退出所有设备 → 列表空；当前 refresh 失效。
- 修改密码：当前密码错误 40001；新密码过短/相同 40001；成功 → 旧密码登录失败、新密码成功、旧 refresh 全部失效；清理临时用户/会话。

## 7. 交付拆分

- SESS-A：RefreshStore 会话模型/legacy/meta/List/Revoke 操作 + tests。
- SESS-B：AuthService 修改密码/sessionId/refresh 同会话/logout/列表/踢出 API + tests。
- SESS-C：OpenAPI/生成类型 + auth store sessionId + Security 页面/UserMenu + tests。
- SESS-D：文档、全量检查、真实多会话冒烟。

## 8. 留给实施计划的决定

- `RefreshStore` 方法签名与 JSON 结构细节；legacy 迁移。
- `SessionMeta` 从 handler 传入的字段/截断。
- 会话列表 UI 的设备摘要格式。
- 修改密码后是否保留当前 access token（前端立即清空，后端 revoke 全部）。
- 手工浏览器验收可用性。
