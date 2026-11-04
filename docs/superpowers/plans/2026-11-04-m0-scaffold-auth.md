# M0：项目脚手架与用户认证 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 kapOJ Monorepo，交付可注册、登录、刷新令牌、编辑资料的前后端骨架（里程碑 M0）。

**Architecture:** `apps/server` 是 Go + Gin + GORM 单体服务，按领域分包（`config`、`httpx`、`db`、`user`、`auth`、`middleware`、`router`）。业务服务依赖仓储接口与 Redis 客户端，单元测试用内存假仓储和 miniredis，仓储层用真实 PostgreSQL 做集成测试。`apps/web` 是 Vue 3 SPA，通过 Vite 代理访问 `/api`。

**Tech Stack:** Go 1.22+、Gin、GORM（postgres 驱动）、Viper、golang-jwt/jwt v5、bcrypt、go-redis v9、miniredis、testify；Vue 3、TypeScript、Vite、Pinia、Vue Router、Element Plus、Axios、Vitest、openapi-typescript；PostgreSQL 16、Redis 7（docker compose）。

**Spec:** `DEVELOPMENT.md`（2.1–2.4、3.1、4.1、8 P0、附录）

## Global Constraints

- 后端技术栈固定为 Go + Gin + GORM + PostgreSQL 16 + Redis，不得替换。
- 主键 bigint 自增；数据库时间用 `timestamptz`，接口返回 RFC 3339。
- 响应格式 `{ "code": 0, "message": "ok", "data": ... }`，出错时 `code` 非 0，并同时设置对应 HTTP 状态码。
- 分页参数 `page`、`page_size`（默认 20，最大 100），返回 `{ total, items }`。
- 角色枚举：`user`、`problem_admin`、`contest_admin`、`super_admin`；用户状态：`active`、`banned`。
- 新用户初始 `rating` 为 1500，`coins` 为 0，且 `CHECK (coins >= 0)`。
- 密码 bcrypt 加密，长度至少 8 位。
- refresh token 存 Redis，登出后立即吊销。
- 登录失败 5 分钟内最多 10 次。
- 日志不得记录密码与 refresh token。
- 接口 JSON 字段用 camelCase，数据库列用 snake_case。
- 包管理：前端用 pnpm workspace（通过 `corepack pnpm` 调用）。

## 已定的实现决策（与规格不同处须在 DEVELOPMENT.md 标注「实现差异」）

- 业务接口统一挂在 `/api` 前缀下（如 `/api/auth/login`），`/health` 不加前缀。
- 注册成功后直接返回令牌（自动登录），与登录响应结构相同。
- 用户名只允许 `[a-zA-Z0-9_]`、3–32 位，统一存小写；邮箱存小写。唯一性因此不区分大小写。
- 登录字段为 `account`，可填用户名或邮箱。
- access token 为 HS256 JWT，有效期 15 分钟；refresh token 为 32 字节随机串（base64url），有效期 7 天，每次刷新轮换，Redis 中只存其 SHA-256。
- 新增超级管理员接口 `GET /api/admin/users`（用户分页列表），用于验收 403 规则。
- 头像上传依赖 MinIO，推迟到 M5-8；M0 只支持填写 `avatarUrl`。
- 被封禁用户无法登录或刷新令牌；已签发的 access token 在过期前（最长 15 分钟）仍可用。

## 错误码

| code | HTTP | 含义 |
|------|------|------|
| 0 | 200/201 | 成功 |
| 40000 | 400 | 请求体格式错误 |
| 40001 | 400 | 参数校验失败（message 说明字段） |
| 40100 | 401 | 未登录或令牌无效 |
| 40101 | 401 | 账号或密码错误 |
| 40300 | 403 | 权限不足 |
| 40301 | 403 | 账号已封禁 |
| 40400 | 404 | 资源不存在 |
| 40901 | 409 | 用户名已被占用 |
| 40902 | 409 | 邮箱已被注册 |
| 42901 | 429 | 登录失败次数过多 |
| 50000 | 500 | 服务器内部错误 |
| 50300 | 503 | 依赖服务不可用 |

## Review Focus

1. **用户名或邮箱只有大小写不同**（`Alice` 与 `alice`）：应视为重复，返回 409。由 Task 4 的 `TestRegisterRejectsCaseVariant` 覆盖。
2. **密码超过 72 字节**：bcrypt 会报错，应返回 40001 而不是 500。由 Task 4 的 `TestRegisterRejectsPasswordOver72Bytes` 覆盖。
3. **refresh token 轮换后被重复使用**：第二次使用应返回 40100。前端并发多个 401 时只发一次刷新请求。由 Task 4 的 `TestRefreshTokenCannotBeReused` 和 Task 9 的 `concurrent 401s share one refresh` 覆盖。
4. **Authorization 头格式错误、令牌过期、令牌由其他密钥签发**：都返回 40100，不得 500。由 Task 5 的 `TestAuthenticateRejectsBadTokens` 覆盖。
5. **两次注册并发写入同一用户名**：应由数据库唯一索引兜底并映射为 40901。由 Task 3 的 `TestCreateDuplicateUsernameMapsToErr` 覆盖。

---

### Task 1: Monorepo 脚手架与本地依赖服务（M0-1）

**Files:**
- Create: `package.json`、`pnpm-workspace.yaml`、`.gitignore`、`.editorconfig`
- Create: `docker/docker-compose.yml`、`docker/postgres/init.sql`、`.env.example`

- [ ] **Step 1:** 根 `package.json` 设 `"private": true`、`"packageManager": "pnpm@9.x"`，脚本 `dev:web`、`test:web`、`lint:web`、`gen:api`。`pnpm-workspace.yaml` 包含 `apps/web`、`packages/*`。
- [ ] **Step 2:** `docker-compose.yml` 定义 `postgres`（`postgres:16`，用户/密码/库均为 `kapoj`，端口 5432，挂载 `init.sql`）和 `redis`（`redis:7`，端口 6379），都带 healthcheck。`init.sql` 额外创建 `kapoj_test` 库。
- [ ] **Step 3:** 验证：`docker compose -f docker/docker-compose.yml up -d` 后 `docker compose ... ps` 两个服务均为 healthy。
- [ ] **Step 4:** Commit：`chore: monorepo scaffold and local services`

### Task 2: 后端基础——配置、统一响应、健康检查（M0-2）

**Files:**
- Create: `apps/server/go.mod`（module `github.com/kaptree/kapoj/apps/server`）
- Create: `apps/server/internal/config/config.go`、`config_test.go`
- Create: `apps/server/internal/httpx/response.go`、`errors.go`、`response_test.go`
- Create: `apps/server/internal/health/handler.go`、`handler_test.go`

**Interfaces:**
- Produces: `config.Load() (*config.Config, error)`，字段 `HTTPAddr`、`DatabaseURL`、`RedisAddr`、`JWTSecret`、`AccessTokenTTL`、`RefreshTokenTTL`。环境变量前缀 `KAPOJ_`，默认值 `:8080`、`localhost:6379`、`15m`、`168h`。
- Produces: `httpx.AppError{Status int; Code int; Message string}`，实现 `error`；预定义构造函数 `httpx.ErrBadRequest`、`ErrValidation(msg string)`、`ErrUnauthorized`、`ErrForbidden`、`ErrNotFound`，以及上方错误码表中的其他值。
- Produces: `httpx.OK(c *gin.Context, data any)`、`httpx.Created(c, data)`、`httpx.Fail(c *gin.Context, err error)`（非 `AppError` 一律记录日志并返回 50000）、`httpx.Page[T]{Total int64; Items []T}`。
- Produces: `health.Pinger interface{ Ping(ctx context.Context) error }`、`health.Handler(db, redis Pinger) gin.HandlerFunc`。

- [ ] **Step 1:** 写失败测试：`TestLoadRequiresJWTSecret`（未设置或短于 32 字节时返回错误）、`TestLoadDefaults`。
- [ ] **Step 2:** 写失败测试：`TestFailWritesAppError`（`ErrForbidden` 写出 HTTP 403 与 `{"code":40300}`）、`TestFailHidesInternalError`（普通 error 写出 500、`code` 50000，响应中不包含原始错误文本）。
- [ ] **Step 3:** 写失败测试：`TestHealthOK`（两个 Pinger 都成功时 200，`data` 为 `{"status":"ok","db":"ok","redis":"ok"}`）、`TestHealthDegraded`（db Ping 失败时 503、`code` 50300、`data.db` 为 `"down"`）。
- [ ] **Step 4:** 运行 `go test ./...`，确认失败。
- [ ] **Step 5:** 实现上述 Interfaces。
- [ ] **Step 6:** 运行 `go test ./...`，全部 PASS。
- [ ] **Step 7:** Commit：`feat(server): config, response envelope, health check`

### Task 3: 数据库接入、User 模型与仓储（M0-3）

**Files:**
- Create: `apps/server/internal/db/db.go`
- Create: `apps/server/internal/user/model.go`、`repository.go`、`repository_test.go`
- Create: `apps/server/internal/testutil/pg.go`

**Interfaces:**
- Produces: `db.Open(dsn string) (*gorm.DB, error)`、`db.Migrate(g *gorm.DB) error`（AutoMigrate 加 `coins >= 0` CHECK 约束）。
- Produces: `user.Role`（常量 `RoleUser`、`RoleProblemAdmin`、`RoleContestAdmin`、`RoleSuperAdmin`）、`user.Status`（`StatusActive`、`StatusBanned`）。
- Produces: `user.User` GORM 模型，字段见 DEVELOPMENT.md 3.1；`username`、`email` 唯一索引分别命名为 `uniq_users_username`、`uniq_users_email`；`Rating` 默认 1500。
- Produces: `user.Repository` 接口：`Create(ctx, *User) error`、`FindByID(ctx, id int64) (*User, error)`、`FindByUsername(ctx, string)`、`FindByEmail(ctx, string)`、`Update(ctx, *User) error`、`List(ctx, offset, limit int) ([]User, int64, error)`。
- Produces: 哨兵错误 `user.ErrNotFound`、`user.ErrUsernameTaken`、`user.ErrEmailTaken`；`user.NewGormRepository(*gorm.DB) Repository`。
- Produces: `testutil.PG(t *testing.T) *gorm.DB`：读取 `KAPOJ_TEST_DATABASE_URL`，未设置则 `t.Skip`；迁移后清空 `users` 表。

- [ ] **Step 1:** 写失败的集成测试：`TestCreateAndFind`（新用户 Rating 为 1500、Coins 为 0）、`TestCreateDuplicateUsernameMapsToErr`、`TestCreateDuplicateEmailMapsToErr`、`TestFindMissingReturnsErrNotFound`、`TestCoinsCannotBeNegative`（Update 把 Coins 设为 -1 时返回错误）、`TestListPaginates`。
- [ ] **Step 2:** 运行 `KAPOJ_TEST_DATABASE_URL=postgres://kapoj:kapoj@localhost:5432/kapoj_test?sslmode=disable go test ./internal/user/...`，确认失败。
- [ ] **Step 3:** 实现。唯一冲突通过 `*pgconn.PgError` 的 `Code == "23505"` 加 `ConstraintName` 映射；`gorm.ErrRecordNotFound` 映射为 `ErrNotFound`。
- [ ] **Step 4:** 再次运行，全部 PASS。
- [ ] **Step 5:** Commit：`feat(server): user model and repository`

### Task 4: 认证服务（M0-4）

**Files:**
- Create: `apps/server/internal/auth/tokens.go`、`tokens_test.go`
- Create: `apps/server/internal/auth/limiter.go`、`limiter_test.go`
- Create: `apps/server/internal/auth/service.go`、`service_test.go`
- Create: `apps/server/internal/user/memory.go`（内存版 Repository，供测试用，模拟唯一约束）
- Create: `apps/server/internal/user/validate.go`、`validate_test.go`

**Interfaces:**
- Consumes: `user.Repository`、`user.User`、哨兵错误（Task 3）；`httpx.AppError`（Task 2）。
- Produces: `auth.Claims{UserID int64; Role user.Role}`；`auth.TokenIssuer`，方法 `IssueAccess(u *user.User) (string, error)`、`ParseAccess(token string) (*Claims, error)`。
- Produces: `auth.RefreshStore`，基于 `*redis.Client`：`Issue(ctx, userID int64) (string, error)`、`Consume(ctx, token string) (int64, error)`（用 GETDEL，令牌不存在时返回 `auth.ErrInvalidRefresh`）、`Revoke(ctx, token string) error`、`RevokeAll(ctx, userID int64) error`。Redis 键：`refresh:<sha256>`、`user_refresh:<uid>`（Set）。
- Produces: `auth.LoginLimiter`：`Allow(ctx, account string) (bool, error)`、`Fail(ctx, account)`、`Reset(ctx, account)`，键 `login_fail:<account>`，上限 10 次，窗口 5 分钟。
- Produces: `auth.Service`：`Register(ctx, RegisterInput) (*Session, error)`、`Login(ctx, account, password string) (*Session, error)`、`Refresh(ctx, refreshToken string) (*TokenPair, error)`、`Logout(ctx, refreshToken string) error`。`RegisterInput{Username, Email, Password, Nickname string}`，`TokenPair{AccessToken, RefreshToken string}`，`Session{TokenPair; User *user.User}`。
- Produces: `user.ValidateUsername`、`ValidateEmail`、`ValidatePassword`、`ValidateNickname`、`ValidateBio`、`ValidateAvatarURL`（各返回 `error`，失败时为 `httpx.ErrValidation`）；`user.Normalize(s string) string`（trim 后转小写）。

校验规则：用户名 `^[a-zA-Z0-9_]{3,32}$`；邮箱用 `net/mail` 解析且不超过 254 字节；密码 8–72 字节；昵称 trim 后 1–32 个字符（为空时默认等于用户名）；简介不超过 200 个字符；头像为空或 `http(s)://` 开头且不超过 512 字节。

- [ ] **Step 1:** 写失败测试 `validate_test.go`：覆盖上述每条规则的边界值（2/3/32/33 位用户名，7/8/72/73 字节密码）。
- [ ] **Step 2:** 写失败测试 `tokens_test.go`：`TestAccessTokenRoundTrip`、`TestParseRejectsExpired`、`TestParseRejectsOtherSecret`；`TestRefreshConsumeOnce`、`TestRevokeAll`（用 miniredis，并用 `FastForward` 验证 TTL 过期）。
- [ ] **Step 3:** 写失败测试 `limiter_test.go`：第 10 次失败后 `Allow` 返回 false；`FastForward(5*time.Minute)` 后恢复；`Reset` 后清零。
- [ ] **Step 4:** 写失败测试 `service_test.go`：`TestRegisterReturnsSession`（用户名存为小写，Rating 1500）、`TestRegisterRejectsCaseVariant`（已有 `alice` 时注册 `Alice` 返回 40901；邮箱大小写不同时返回 40902）、`TestRegisterRejectsPasswordOver72Bytes`（40001）、`TestLoginByUsernameOrEmail`、`TestLoginWrongPasswordSameErrorAsUnknownUser`（均为 40101）、`TestLoginBannedUser`（40301）、`TestLoginRateLimited`（10 次失败后，即使密码正确也返回 42901）、`TestRefreshRotates`、`TestRefreshTokenCannotBeReused`（40100）、`TestLogoutRevokes`（登出后刷新返回 40100）。
- [ ] **Step 5:** 运行 `go test ./internal/auth/... ./internal/user/...`，确认失败。
- [ ] **Step 6:** 实现上述 Interfaces。
- [ ] **Step 7:** 运行测试，全部 PASS。
- [ ] **Step 8:** Commit：`feat(server): auth service with jwt, refresh rotation and login limit`

### Task 5: 鉴权中间件与角色校验（M0-5）

**Files:**
- Create: `apps/server/internal/middleware/auth.go`、`auth_test.go`

**Interfaces:**
- Consumes: `auth.TokenIssuer.ParseAccess`、`auth.Claims`（Task 4）。
- Produces: `middleware.Authenticate(t *auth.TokenIssuer) gin.HandlerFunc`（解析 `Authorization: Bearer <token>`，成功时写入上下文；无令牌时放行，供游客访问）、`middleware.RequireAuth() gin.HandlerFunc`（未认证返回 40100）、`middleware.RequireRole(roles ...user.Role) gin.HandlerFunc`（`super_admin` 总是放行，否则不在列表中返回 40300）、`middleware.CurrentClaims(c *gin.Context) (*auth.Claims, bool)`。

- [ ] **Step 1:** 写失败测试：`TestAuthenticateRejectsBadTokens`（表驱动：`Basic xxx`、`Bearer`、`Bearer garbage`、过期令牌、其他密钥签发的令牌，均为 401、`code` 40100）、`TestRequireAuthAllowsValidToken`、`TestRequireRoleForbidsUser`（403、`code` 40300）、`TestRequireRoleAllowsSuperAdmin`。
- [ ] **Step 2:** 运行测试，确认失败。
- [ ] **Step 3:** 实现。带有 Authorization 头但令牌无效时直接返回 40100，不降级为游客。
- [ ] **Step 4:** 运行测试，全部 PASS。
- [ ] **Step 5:** Commit：`feat(server): auth and role middleware`

### Task 6: 认证与用户 HTTP 接口（M0-6）

**Files:**
- Create: `apps/server/internal/auth/handler.go`、`handler_test.go`
- Create: `apps/server/internal/user/service.go`、`handler.go`、`handler_test.go`
- Create: `apps/server/internal/user/dto.go`

**Interfaces:**
- Consumes: `auth.Service`（Task 4）、中间件（Task 5）、`user.Repository`（Task 3）。
- Produces: `auth.RegisterRoutes(r gin.IRouter, s *auth.Service)`，挂载 `POST /auth/register`（201）、`/auth/login`、`/auth/refresh`、`/auth/logout`。
- Produces: `user.RegisterRoutes(r gin.IRouter, s *user.Service, t *auth.TokenIssuer)`，挂载 `GET /users/me`、`PUT /users/me`（需登录）、`GET /users/:id/profile`（公开）、`GET /admin/users`（仅 `super_admin`）。
- Produces: DTO `user.MeDTO`（`id, username, nickname, email, avatarUrl, role, rating, coins, bio, status, createdAt`）、`user.ProfileDTO`（`id, username, nickname, avatarUrl, bio, rating, coins, equippedBadges: [], equippedTitle: null, createdAt`）。`ProfileDTO` 不含 email。
- Produces: `user.Service.UpdateMe(ctx, id int64, in UpdateMeInput) (*User, error)`，`UpdateMeInput` 各字段为指针，只更新提供的字段。

- [ ] **Step 1:** 写失败的 httptest 测试（内存仓储 + miniredis）：注册 201 且返回 `accessToken`；请求体 JSON 格式错误返回 40000；登录、刷新、登出完整流程；`GET /users/me` 无令牌时 401；`PUT /users/me` 只修改 `bio` 时昵称不变；`GET /users/:id/profile` 不包含 `email` 字段；不存在的 id 或非数字 id 返回 40400；普通用户访问 `GET /admin/users` 返回 403；超级管理员访问返回 `{total, items}`，`page_size=500` 时按 100 截断。
- [ ] **Step 2:** 运行测试，确认失败。
- [ ] **Step 3:** 实现。
- [ ] **Step 4:** 运行 `go test ./...`，全部 PASS。
- [ ] **Step 5:** Commit：`feat(server): auth and user http endpoints`

### Task 7: 服务装配、Dockerfile 与 OpenAPI（M0-7）

**Files:**
- Create: `apps/server/internal/router/router.go`、`router_test.go`
- Create: `apps/server/cmd/server/main.go`、`apps/server/Dockerfile`
- Create: `packages/api/openapi.yaml`、`packages/api/package.json`
- Modify: `docker/docker-compose.yml`（新增 `server` 服务，profile 设为 `app`）

**Interfaces:**
- Produces: `router.New(deps router.Deps) *gin.Engine`，`Deps{DB *gorm.DB; Redis *redis.Client; Config *config.Config}`。挂载 `/health` 与 `/api` 分组，使用 Gin 的 Recovery 和自定义请求日志（只记录方法、路径、状态码、耗时，不记录请求体）。

- [ ] **Step 1:** 写失败测试 `TestRouterServesHealthAndAPI`（基于 `testutil.PG` 与 miniredis：`/health` 返回 200，`/api/users/me` 返回 401）。
- [ ] **Step 2:** 实现 router 与 `main.go`：启动时执行迁移，收到 SIGINT/SIGTERM 时优雅退出（超时 10 秒）。
- [ ] **Step 3:** `Dockerfile` 采用多阶段构建（`golang:1.24` 构建，`gcr.io/distroless/static` 运行）。
- [ ] **Step 4:** `openapi.yaml`（OpenAPI 3.0）描述 Task 6 的全部接口与 DTO。
- [ ] **Step 5:** 验证：`go build ./...` 成功；启动服务后 `curl localhost:8080/health` 返回 ok。
- [ ] **Step 6:** Commit：`feat(server): wire router, entrypoint, dockerfile and openapi spec`

### Task 8: 前端脚手架与基础布局（M0-8）

**Files:**
- Create: `apps/web/`（Vite + Vue 3 + TS 项目）：`vite.config.ts`、`tsconfig*.json`、`index.html`、`src/main.ts`、`src/App.vue`
- Create: `src/layouts/DefaultLayout.vue`、`src/views/HomeView.vue`、`src/views/NotFoundView.vue`、`src/router/index.ts`
- Create: `src/api/schema.d.ts`（由 `pnpm gen:api` 生成）

- [ ] **Step 1:** 安装依赖：`vue`、`vue-router`、`pinia`、`element-plus`、`axios`；开发依赖 `vitest`、`@vue/test-utils`、`jsdom`、`openapi-typescript`、`vue-tsc`。
- [ ] **Step 2:** Vite 将 `/api` 和 `/health` 代理到 `http://localhost:8080`。
- [ ] **Step 3:** 布局包含顶部导航（站点名、登录/注册或用户菜单）和侧边栏（首页、题库、比赛、排行榜）。尚未实现的模块菜单项设为禁用，并显示「M1 上线」等说明。
- [ ] **Step 4:** 验证：`corepack pnpm --filter web build` 成功。
- [ ] **Step 5:** Commit：`feat(web): vue3 scaffold and base layout`

### Task 9: 前端认证流程（M0-9）

**Files:**
- Create: `src/api/http.ts`、`src/api/http.spec.ts`、`src/api/auth.ts`、`src/api/users.ts`
- Create: `src/stores/auth.ts`、`src/stores/auth.spec.ts`
- Create: `src/router/guard.ts`、`src/router/guard.spec.ts`
- Create: `src/views/LoginView.vue`、`RegisterView.vue`、`ProfileEditView.vue`、`UserProfileView.vue`

**Interfaces:**
- Produces: `createHttp(options?: { adapter?: AxiosAdapter }) : AxiosInstance`。成功时解包 `data`；失败时抛出 `ApiError{status, code, message}`；遇到 401 时（`/auth/*` 请求除外）发起一次共享的刷新请求并重试原请求，刷新失败则清除登录状态并跳转到 `/login`。
- Produces: Pinia store `useAuthStore`，状态 `accessToken`、`refreshToken`、`user`，持久化到 `localStorage` 的 `kapoj.auth` 键；action `login`、`register`、`logout`、`fetchMe`、`setTokens`、`clear`。
- Produces: `authGuard(to, auth) : true | RouteLocationRaw`。带 `meta.requiresAuth` 的路由在未登录时跳转到 `/login?redirect=<原路径>`；带 `meta.guestOnly` 的路由在已登录时跳转到 `/`。

- [ ] **Step 1:** 写失败测试 `http.spec.ts`（使用假 adapter）：`unwraps data`、`throws ApiError with code`、`refreshes once and retries on 401`、`concurrent 401s share one refresh`、`refresh failure clears auth`。
- [ ] **Step 2:** 写失败测试 `auth.spec.ts`（持久化与 `clear`）、`guard.spec.ts`（三种跳转情况）。
- [ ] **Step 3:** 运行 `corepack pnpm --filter web test`，确认失败。
- [ ] **Step 4:** 实现；页面使用 Element Plus 表单，前端校验规则与后端一致（用户名、邮箱、密码 8–72 位）。
- [ ] **Step 5:** 运行测试与构建，全部 PASS。
- [ ] **Step 6:** Commit：`feat(web): auth flow, token refresh and profile pages`

### Task 10: 代码规范、启动说明与端到端验收（M0-10）

**Files:**
- Create: `apps/web/eslint.config.js`、`.prettierrc`、`apps/server/.golangci.yml`
- Modify: `README.md`
- Modify: `DEVELOPMENT.md`（第 9 节 M0 状态、相关章节加「实现差异」、链接任务清单）
- Modify: `docs/任务清单.md`、`docs/开发日志.md`

- [ ] **Step 1:** 配置 eslint（`eslint-plugin-vue` + `typescript-eslint`）与 prettier；`golangci.yml` 启用 `govet`、`errcheck`、`staticcheck`、`gofmt`。
- [ ] **Step 2:** 验证：`corepack pnpm --filter web lint` 无错误；`gofmt -l .` 无输出；`go vet ./...` 通过。
- [ ] **Step 3:** README 写明启动步骤：docker compose、后端环境变量、`go run ./cmd/server`、`pnpm dev:web`、测试命令。
- [ ] **Step 4:** 端到端验收（对照 4.1 验收标准）：用 curl 依次注册、登录、刷新、访问 `/api/users/me`、以普通用户访问 `/api/admin/users`（403）、无令牌访问（401）。
- [ ] **Step 5:** 按 `skills/dev-docs-sync` 更新文档。
- [ ] **Step 6:** Commit：`docs: m0 progress and implementation notes`
