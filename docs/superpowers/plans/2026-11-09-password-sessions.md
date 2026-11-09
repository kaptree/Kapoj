# 修改密码与活跃会话管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户可修改密码、查看活跃会话（设备/IP/最近使用），单独退出设备、退出其他设备或退出所有设备；改密后吊销全部会话并重新登录。

**Architecture:** 将 `RefreshStore` 升级为会话存储（sessionId 稳定、refresh token 轮换、meta/IP/UA、List/Revoke/RevokeOthers/RevokeAll）；`auth.Service` 增加 sessionId、修改密码、会话列表/踢出；前端新增 `/settings/security` 与 auth store sessionId；不新增权限、不改 users 表。

**Tech Stack:** Go 1.26、Gin、Redis 7、bcrypt、Vue 3、PrimeVue 4、Vitest。

**Spec:** `docs/superpowers/specs/2026-11-09-password-sessions-design.md`

## Global Constraints

- 修改密码必须验证当前密码；新密码 8–72 字节且不得与当前相同；限流 `password:<userID>` 10 次/5 分钟，超限 42904；成功吊销全部会话；前端清空并重新登录。
- 会话列表只返回 id/IP/userAgent/createdAt/lastUsedAt；绝不返回 refresh token/hash；仅本人；单独踢出/退出其他/全部均校验归属，未知/他人 404。
- 刷新在同一 sessionId 内轮换 refresh token；滑动续期 7 天；legacy refresh token（纯 userID 值）兼容并迁移到新会话。
- SessionMeta IP/UA 仅展示；IP 用 `c.ClientIP()`，反向代理需 trusted proxies（文档说明）。
- 无新权限；自助操作不写审计；不改 users 表；不新增依赖。
- 不改变 access token 15m/refresh 7d 配置；改密后 access token 最迟到 15m 过期，refresh 全部失效。
- 中文文案；PrimeVue 4；不推送/合并；`Export.zip` 保持未跟踪。

## Review Focus

1. **会话语义**：Issue/Consume/Rotate 同 sessionId、滑动 TTL、legacy 迁移、List 清理、RevokeSession/Others/All 归属；列表不泄露 token/hash。
2. **修改密码**：当前密码校验、8–72、不得相同、42904 限流、RevokeAll、UpdatePassword 顺序；旧密码失败/新密码成功/旧 refresh 失效。
3. **刷新/登录**：登录/注册/刷新响应 sessionId；刷新保持 sessionId；legacy token 首次刷新迁移；现有登录/登出测试更新。
4. **权限/边界**：401 未登录、404 他人/未知会话、40001 当前密码/非法/相同、42904/503；不能操作他人会话。
5. **前端**：auth store sessionId 持久化/refresh 更新/clear；Security 页当前设备标记、列表、单踢/其他/全部确认、改密校验/清空跳登录、空/加载/错误。
6. **回归**：现有登录/注册/refresh/logout/UserMenu 行为；OpenAPI/生成类型；不改其他资料/权限。

---

### Task 1: RefreshStore 会话模型（SESS-A）

**Files:**
- Modify: `apps/server/internal/auth/tokens.go`
- Modify: `apps/server/internal/auth/tokens_test.go`

**Interfaces:**
- Produces:
  ```go
  type SessionMeta struct { IP string; UserAgent string }
  type SessionInfo struct {
      ID string; IP, UserAgent string
      CreatedAt, LastUsedAt time.Time
  }
  var ErrSessionNotFound = errors.New("session not found")

  // Keep existing Issue/Consume wrappers so the auth package compiles before Task 2.
  func (s *RefreshStore) Issue(ctx context.Context, userID int64) (string, error)
  func (s *RefreshStore) IssueWithMeta(ctx context.Context, userID int64, meta SessionMeta) (token, sessionID string, err error)
  func (s *RefreshStore) Consume(ctx context.Context, token string) (int64, error)
  func (s *RefreshStore) ConsumeSession(ctx context.Context, token string) (userID int64, sessionID string, err error)
  func (s *RefreshStore) Rotate(ctx context.Context, userID int64, sessionID string, meta SessionMeta) (token string, err error)
  func (s *RefreshStore) List(ctx context.Context, userID int64) ([]SessionInfo, error)
  func (s *RefreshStore) RevokeSession(ctx context.Context, userID int64, sessionID string) error
  func (s *RefreshStore) RevokeOthers(ctx context.Context, userID int64, keepSessionID string) error
  func (s *RefreshStore) RevokeAll(ctx context.Context, userID int64) error
  ```
- Existing `Revoke(ctx, token)` retained; use Consume + RevokeSession.
- Redis keys per design: `refresh:<hash>` JSON `{userId,sessionId}`, `session:<id>` JSON, `user_refresh:<uid>` set, `user_sessions:<uid>` set; TTL refreshTTL.
- Consumes: existing miniredis tests.

- [ ] **Step 1: 写失败测试**

  `tokens_test.go`：
  - `TestIssueConsumeSession`：IssueWithMeta 返回 token/sessionId；ConsumeSession 返回同 user/session；二次 Consume invalid；session 记录 meta/IP/UA/created/lastUsed；旧 Issue/Consume 包装仍可用。
  - `TestRotateKeepsSession`：Rotate 后 token 变、sessionId 不变、lastUsed 更新、旧 token invalid、TTL 重置（miniredis TTL）。
  - `TestListSessions`：两个 Issue → List 2 条按 lastUsed/created；不返回 token/hash；过期/缺失清理。
  - `TestRevokeSession`：删除指定；其他保留；未知/他人 ErrSessionNotFound。
  - `TestRevokeOthersAndAll`：保留当前/全部清空。
  - `TestLegacyRefreshValue`：手工写 `refresh:<hash>` 为纯 userID；Consume 兼容 sessionId 空；Rotate/新建会话。
  - `TestRevokeAllLegacy`：旧 key/set 清理。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  go test -count=1 ./internal/auth/ -run 'TestIssueConsumeSession|TestRotateKeepsSession|TestListSessions|TestRevokeSession|TestRevokeOthers|TestLegacyRefreshValue' -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - sessionId：16 字节 crypto/rand hex（32 字符）；token 保持 32 字节 base64url。
  - IssueWithMeta：生成 token/hash/sessionId；写 refresh JSON、session JSON、两个 set，TTL；旧 `Issue` 调 IssueWithMeta 并丢弃 sessionId。
  - ConsumeSession：GetDel refresh；删除 user_refresh 成员；解析 JSON，兼容纯 int（sessionId 空）；不删 session；旧 `Consume` 调 ConsumeSession 并返回 userID。
  - Rotate：生成新 token/hash；更新 session JSON tokenHash/lastUsedAt/ip/ua；写 refresh JSON、set；TTL 滑动；旧 token 已消费。
  - List：SMembers user_sessions；批量 Get session；跳过/清理无效；按 lastUsedAt desc。
  - RevokeSession：Get session，校验 userID；删 refresh token key、session key、sets。
  - RevokeOthers：List/集合遍历，保留 keep；RevokeSession each。
  - RevokeAll：遍历 user_refresh（legacy）与 user_sessions，删 keys/sets。
  - `Revoke(token)`：Consume → if sessionId != "" RevokeSession else remove legacy hash; 幂等。
  - 所有 Redis 错误原样返回。

- [ ] **Step 4: 运行测试、gofmt**

  ```bash
  cd apps/server
  gofmt -w internal/auth
  go test -count=1 ./internal/auth/ -run 'TestIssueConsumeSession|TestRotateKeepsSession|TestListSessions|TestRevokeSession|TestRevokeOthers|TestLegacyRefreshValue|TestRefresh'
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/auth/tokens.go apps/server/internal/auth/tokens_test.go
  git commit -m "feat(auth): add session tracking"
  ```

---

### Task 2: AuthService/仓储/Handler 修改密码与会话 API（SESS-B）

**Files:**
- Modify: `apps/server/internal/user/repository.go`
- Modify: `apps/server/internal/user/memory.go`
- Modify: `apps/server/internal/user/repository_test.go`
- Modify: `apps/server/internal/auth/service.go`
- Modify: `apps/server/internal/auth/service_test.go`
- Modify: `apps/server/internal/auth/handler.go`
- Modify: `apps/server/internal/auth/handler_test.go`
- Modify: `apps/server/internal/httpx/errors.go`
- Modify: `apps/server/internal/router/router.go`

**Interfaces:**
- Produces:
  ```go
  // user.Repository
  UpdatePassword(ctx context.Context, id int64, hash string) error

  // auth.Service
  type SessionView struct { ID, IP, UserAgent string; CreatedAt, LastUsedAt time.Time }
  type ChangePasswordInput struct { CurrentPassword, NewPassword string }
  func (s *Service) ChangePassword(ctx context.Context, userID int64, in ChangePasswordInput) error
  func (s *Service) ListSessions(ctx context.Context, userID int64) ([]SessionView, error)
  func (s *Service) RevokeSession(ctx context.Context, userID int64, sessionID string) error
  func (s *Service) LogoutOthers(ctx context.Context, userID int64, keepSessionID string) error
  func (s *Service) LogoutAll(ctx context.Context, userID int64) error
  // TokenPair.SessionID string `json:"sessionId"`
  // RegisterWithMeta/LoginWithMeta/RefreshWithMeta; old wrappers
  ```
- HTTP:
  - `PUT /api/users/me/password` `{currentPassword,newPassword}` → 200 null.
  - `GET /api/users/me/sessions` → SessionView[].
  - `DELETE /api/users/me/sessions/:id` → 200 null.
  - `POST /api/users/me/sessions/logout-others` `{sessionId}` → 200 null.
  - `POST /api/users/me/sessions/logout-all` → 200 null.
  - `auth.RegisterAccountRoutes(api, svc)`; all `RequireAuth`.
- `httpx.ErrTooManyPasswordAttempts` (429, 42904, 中文).
- SessionMeta from handler: IP `c.ClientIP()`, UA `c.Request.UserAgent()`.

- [ ] **Step 1: 写失败测试**

  `user/repository_test.go`：UpdatePassword 更新 hash；不存在 ErrNotFound；memory repo。
  `auth/service_test.go`：
  - `TestLoginReturnsSessionID`/register；Refresh 保持 sessionId；legacy refresh 生成新 sessionId。
  - `TestChangePassword`：当前错误 40001；新密码短/相同 40001；成功 hash 更新、旧 refresh 失效、旧密码登录失败、新密码成功、RevokeAll。
  - `TestChangePasswordRateLimit`：第 11 次 42904；成功后重置（可注入/使用 limiter）。
  - `TestListRevokeSessions`：两个会话、List、RevokeSession、LogoutOthers、LogoutAll。
  - `TestLogoutRevokesCurrentSession`.
  `auth/handler_test.go`：
  - `TestPasswordEndpoint`：401/40001/42904/成功。
  - `TestSessionEndpoints`：列表无 token/hash；DELETE 当前/他人 404；logout-others 保留当前；logout-all；未登录 401。
  - 登录/注册/刷新响应 `sessionId`；旧测试 fixture 更新。

- [ ] **Step 2: 运行测试确认失败**

  ```bash
  cd apps/server
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/auth/ ./internal/user/ ./internal/router/ \
    -run 'TestChangePassword|TestListRevoke|TestSession|TestPassword|TestLoginReturns|TestRefresh' -v
  ```
  Expected: FAIL。

- [ ] **Step 3: 实现**

  - user repo UpdatePassword（GORM `Update("password_hash", hash)`；rows 0 → ErrNotFound）；memory.
  - auth.Service 持有 limiter；`newSession(ctx,u,meta)`；`RegisterWithMeta/LoginWithMeta/RefreshWithMeta`；old wrappers meta empty.
  - `TokenPair.SessionID`；`issuePair` → `RefreshStore.IssueWithMeta/Rotate`; session DTO includes.
  - `RefreshWithMeta`: `ConsumeSession`; if sessionID empty → `IssueWithMeta` new; else `Rotate`; validate user/banned; access token; return pair with sessionId.
  - `ChangePassword`: read user; bcrypt current; ValidatePassword(new); new==current 40001; limiter.Hit("password:<id>") false→42904, err→503; RevokeAll (fail abort); UpdatePassword; limiter.Reset. Accept revoke-first/update-fail edge.
  - List/Revoke/LogoutOthers/LogoutAll; Logout uses `ConsumeSession` → `RevokeSession`; empty sessionId falls back to legacy `Revoke`.
  - handler: SessionMeta; account routes; bind inputs; map errors; sessionId on login/register/refresh.
  - `httpx` 42904; router `auth.RegisterAccountRoutes(api, authSvc)` after auth routes.
  - OpenAPI not yet (Task 3).
  - Update existing tests for new SessionID/TTL/signatures.

- [ ] **Step 4: 运行测试、gofmt、lint**

  ```bash
  cd apps/server
  gofmt -w internal/auth internal/user internal/httpx internal/router
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' \
    go test -count=1 -p 1 ./internal/auth/ ./internal/user/ ./internal/httpx/ ./internal/router/
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run
  ```
  Expected: PASS。

- [ ] **Step 5: 提交**

  ```bash
  git add apps/server/internal/auth apps/server/internal/user apps/server/internal/httpx apps/server/internal/router
  git commit -m "feat(auth): add password change and session APIs"
  ```

---

### Task 3: OpenAPI、auth store sessionId、账号安全页（SESS-C）

**Files:**
- Modify: `packages/api/openapi.yaml`
- Regenerate: `apps/web/src/api/schema.d.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/api/auth.ts`
- Modify: `apps/web/src/stores/auth.ts`
- Modify: `apps/web/src/stores/auth.spec.ts`
- Create: `apps/web/src/views/SecuritySettingsView.vue`
- Create: `apps/web/src/views/SecuritySettingsView.spec.ts`
- Modify: `apps/web/src/components/UserMenu.vue`
- Modify: `apps/web/src/components/UserMenu.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Produces:
  ```ts
  // auth.ts
  changePassword(input: {currentPassword:string;newPassword:string}): Promise<null>
  listSessions(): Promise<SessionView[]>
  revokeSession(id: string): Promise<null>
  logoutOthers(sessionId: string): Promise<null>
  logoutAll(): Promise<null>
  // auth store: sessionId persisted and updated on setTokens/setSession; clear resets.
  ```
- Routes: `/settings/security` name `security-settings` requiresAuth; UserMenu「账号安全」.
- UI: password form; sessions DataTable/list; current session tag; per-row logout; logout others/all confirm; success/error states; change password success clear + login; mobile/dark/light.
- Consumes: Task 1-2 API; PrimeVue; existing `validatePassword`/Password/DataTable/Dialog/Confirm/Toast.

- [ ] **Step 1: OpenAPI + 类型**

  - `TokenPair`/`Session` add `sessionId` (required).
  - Paths: password/sessions; schemas `ChangePasswordInput`, `SessionView`, `LogoutOthersInput`; 401/40001/404/42904.
  - `gen:api`; types aliases.
- [ ] **Step 2: 写失败测试**

  `auth.spec.ts`：sessionId persisted; setTokens/refresh updates; clear; changePassword/logout endpoints? (store methods maybe not).
  `SecuritySettingsView.spec.ts`：
  - load sessions; current tag by `auth.sessionId`; device/IP/time render; empty/loading/error.
  - change password validation (short/mismatch/same? frontend same check optional), success calls API, clear, router login; wrong current 40001 message; 42904.
  - revoke session confirm accept; current session clears/login; other reloads.
  - logout others calls with sessionId, confirm, toast, reload; logout all clears/login.
  - object URL none.
  `UserMenu.spec.ts`: 账号安全 entry.
- [ ] **Step 3: 运行测试确认失败**

  ```bash
  corepack pnpm --filter web test -- src/stores/auth.spec.ts src/views/SecuritySettingsView.spec.ts src/components/UserMenu.spec.ts
  ```
  Expected: FAIL。
- [ ] **Step 4: 实现**

  - auth store `Persisted.sessionId`; `setTokens`/`setSession` update if provided; `clear` null; expose `sessionId`.
  - api/auth methods.
  - Security view: `AppPageHeader`/`AppSurface`; change password form (Password current/new/confirm; `validatePassword`; 8–72; match; optional same); submit; success `auth.clear()` + `router.push(login)` + toast? Toast before clear; use `useToast`.
  - sessions: `DataTable` or list; current tag; `deviceLabel(userAgent)` simple; `new Date(...).toLocaleString('zh-CN')`; actions.
  - `revokeSession`: confirm; if id===auth.sessionId → clear+login; else reload.
  - `logoutOthers`: confirm; `logoutOthers(auth.sessionId)`; reload.
  - `logoutAll`: confirm; API; clear+login.
  - errors: 40001/42904 message; 401 handled by client.
  - UserMenu entry; route.
  - PrimeVue only; no new deps.
- [ ] **Step 5: 运行测试、lint、build**

  ```bash
  corepack pnpm gen:api
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  ```
- [ ] **Step 6: 提交**

  ```bash
  git add packages/api/openapi.yaml apps/web/src/api apps/web/src/stores apps/web/src/views/SecuritySettingsView.vue \
    apps/web/src/views/SecuritySettingsView.spec.ts apps/web/src/components/UserMenu.vue \
    apps/web/src/components/UserMenu.spec.ts apps/web/src/router/index.ts
  git commit -m "feat(web): add account security page"
  ```

---

### Task 4: 文档、全量检查与多会话冒烟（SESS-D）

**Files:**
- Modify: `DEVELOPMENT.md`
- Modify: `docs/任务清单.md`
- Modify: `docs/开发日志.md`
- Modify: `README.md`
- Modify: `运行手册.md`
- Test: 全量后端/评测/前端/真实多会话

- [ ] **Step 1: 文档**

  - `DEVELOPMENT.md` §4.1 认证/资料：修改密码、会话列表、退出其他/所有、sessionId/Redis 结构、限流 42904、IP best-effort；新增“账号安全”完成状态。
  - `docs/任务清单.md`：新增“跨里程碑：账号安全（修改密码 + 活跃会话）✅”条目；测试数。
  - `docs/开发日志.md`：2026-11-09 条目（变更/文件/实现差异/验证）。
  - `README.md`：账号安全/会话管理功能。
  - `运行手册.md`：修改密码/会话管理、42904、代理 trusted proxies、Redis key/TTL、排障。
- [ ] **Step 2: 全量检查**

  ```bash
  cd apps/server
  gofmt -l .
  go vet ./...
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...
  /private/var/folders/kw/9jmh6w590612jph0yl4l04000000gn/T/opencode/bin/golangci-lint run

  cd ../judge
  gofmt -l .
  go vet ./...
  KAPOJ_TEST_DATABASE_URL='postgres://kapoj:kapoj@localhost:5433/kapoj_test?sslmode=disable' go test -count=1 -p 1 ./...

  cd /Users/admin/codespace/kapoj
  corepack pnpm test:web
  corepack pnpm lint:web
  corepack pnpm build:web
  git diff --check
  ```
- [ ] **Step 3: 真实冒烟**

  - 临时 server `:18081`/测试库/临时 Redis（或 6380 独立 key）：
  - 登录 A 两次（不同 UA/IP 模拟）→ 列表 2 条、当前标记；sessionId 返回。
  - 单独退出设备 2 → 列表 1；设备 2 refresh 失败；设备 1 正常。
  - 退出其他设备（当前 1）→ 列表 1；其他 refresh 失效。
  - 退出所有设备 → 列表空/当前 refresh 失效。
  - 修改密码：当前错误 40001；新密码短/相同 40001；10 次错误后 42904；成功 → 旧密码登录失败/新密码成功/旧 refresh 全部失效。
  - 清理临时用户/Redis key；不删除既有数据。
- [ ] **Step 4: 提交**

  ```bash
  git add DEVELOPMENT.md docs/任务清单.md docs/开发日志.md README.md 运行手册.md
  git commit -m "docs: record password and session management"
  ```

---

## Self-Review

**Spec coverage:** RefreshStore → Task 1; auth service/repo/handler/API → Task 2; OpenAPI/auth store/UI → Task 3; docs/checks/smoke → Task 4.

**Type consistency:** `SessionMeta`/`SessionInfo`/ErrSessionNotFound Task 1; `TokenPair.SessionID`/SessionView/ChangePasswordInput Task 2; Security page uses `auth.sessionId`; `SessionView` OpenAPI. `UpdatePassword` repo. 42904 error.

**Review Focus mapping:**
1. 会话语义 → Task 1 tests.
2. 修改密码 → Task 2 tests.
3. 刷新/登录 → Task 2 tests.
4. 权限/边界 → Task 2 handler tests.
5. 前端 → Task 3 specs + smoke.
6. 回归 → full checks + existing auth tests.
