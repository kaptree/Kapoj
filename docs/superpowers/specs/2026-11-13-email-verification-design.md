# 邮箱验证与密码找回设计

> 目标：注册流程改为邮箱验证制；邮件发送通道支持 QQ 邮箱 SMTP、腾讯云 SES SMTP、阿里云邮件推送 SMTP 与自定义 SMTP；后台可配置邮件服务并一键切换“全站强制邮箱验证”；忘记密码通过邮件链接重置。存量用户迁移时视为已验证，避免上线即锁死老用户。

## 1. 背景与现状

- `POST /api/auth/register` 当前直接创建用户并返回 `{accessToken, refreshToken, sessionId, user}`，注册即登录。
- `users` 表没有邮箱验证状态字段；`user.MeDTO` 不返回邮箱验证信息。
- 项目没有邮件发送模块，也没有 SMTP / 云邮件 SDK 依赖。
- Redis 已用于 refresh token 会话与登录限流（`auth.RefreshStore`、`auth.LoginLimiter`），适合继续存放一次性邮箱 token 与频率限制。
- 后端已有 asynq 游戏事件队列，但邮件发送尚未接入队列。
- 前端只有登录/注册页；路由守卫对 `requiresAuth` 页面会在未登录时跳转登录并带 `redirect`。没有邮箱验证、忘记密码、重置密码页面。
- 后台已有站点设置、AI 设置等页面，均使用“独立权限 + 管理路由 + 审计”的模式；可复用该模式新增邮件设置。
- 现有权限目录包含 `site.settings.manage` 等 39 个权限码；新增邮件设置权限后目录继续扩展。
- OpenAPI 契约位于 `packages/api/openapi.yaml`，前端类型由 `pnpm gen:api` 生成。

## 2. 目标与非目标

### 目标

- 新增后台邮件设置页 `/admin/mail`，独立权限 `mail.settings.manage`，保存、测试发送、开关均写管理审计。
- 邮件发送支持：
  - QQ 邮箱 SMTP（`smtp.qq.com:465` SSL，密码为 QQ 邮箱授权码）。
  - 腾讯云 SES SMTP（`smtp.qcloudmail.com:465` SSL）。
  - 阿里云邮件推送 SMTP（`smtpdm.aliyun.com:465` SSL）。
  - 自定义 SMTP（主机、端口、TLS 模式、用户名、密码均可配置）。
- 后台可发送测试邮件，验证配置是否可用。
- 新增全站开关 `requireEmailVerification`（默认开启）：
  - 开启时：注册只创建账号并发送验证邮件，不直接登录；未验证用户登录/刷新返回 `40303`；已登录的未验证用户访问受保护接口由中间件拦截。
  - 关闭时：注册仍可发送验证邮件，但允许直接登录；后台随时可打开开关，打开后未验证用户被拦截。
- 新增邮箱验证流程：
  - 注册后发送验证链接，链接指向前端 `/verify-email?token=...`。
  - 验证成功后标记 `email_verified_at`，返回新的登录会话（自动登录）。
  - 支持重新发送验证邮件（带冷却与频率限制，防枚举）。
- 新增密码找回流程：
  - `POST /api/auth/password/forgot` 发送重置链接，始终返回通用成功信息，不暴露邮箱是否存在。
  - `POST /api/auth/password/reset` 校验一次性 token，更新密码并吊销该用户全部会话。
- 存量用户迁移时标记为已验证（方案 A），不打断老用户。
- 前端新增 `/verify-email`、`/forgot-password`、`/reset-password` 页面；登录页增加“忘记密码”；注册页增加“验证邮件已发送”状态。
- OpenAPI、生成类型、README、DEVELOPMENT、运行手册同步更新。
- 单元测试、集成测试、前端测试与真实 SMTP 冒烟。

### 非目标

- 不做修改邮箱时的二次验证；本轮只验证注册邮箱。
- 不做 2FA、Passkey、短信验证码。
- 不做营销邮件、批量发信、退订管理、投递统计。
- 不接入腾讯云/阿里云官方 HTTP API；本轮统一走厂商 SMTP 通道，`mail.Sender` 接口预留以后接入官方 API。
- 不做多邮件通道自动 fallback；同一时间只有一个生效通道。
- 不做异步邮件队列；本轮同步发送，失败由用户重发与管理员测试兜底。
- 不做邮件模板后台可视化编辑；模板内置在代码中。
- 不做管理后台查看/强制验证单个用户的操作；全站开关即可。
- 不改变现有账号密码强度、会话、限流语义（除新增的邮箱验证拦截）。

### 反目标

- 强制验证开启时，未验证用户仍能登录或调用受保护接口。
- 存量用户升级后被误判为未验证而全部锁死。
- 通过注册/重发/忘记密码接口枚举邮箱是否存在。
- SMTP 密码、加密密钥、邮箱 token 出现在 API 响应、日志或前端持久化中。
- 邮件链接使用请求 Host 拼接，导致 Host header 注入。
- 邮件头注入（Subject/From/To 中的 CRLF）。
- 后台保存了未就绪的强制验证配置，导致注册静默失败。
- 邮件发送失败时注册接口谎报“邮件已发送”。

## 3. 方案与取舍

### 3.1 邮件通道：统一 SMTP 引擎 + 厂商预设（采纳）

- 一个 `mail.Sender` 实现，底层使用 Go 标准库 `net/smtp` + `crypto/tls`。
- 后台选择 provider 预设，自动带出 host/port/TLS；所有厂商最终都走 SMTP。
- 优点：
  - 一套代码覆盖 QQ、腾讯云、阿里云和自定义 SMTP。
  - 不引入腾讯云/阿里云 Go SDK，不依赖模板审核。
  - 验证邮件和重置邮件共用同一套 HTML/纯文本模板。
  - 测试与运维简单，后台可发测试邮件。
- 代价：
  - 使用厂商 SMTP 通道，拿不到官方 API 的投递统计、模板 ID、MessageId 查询。
  - 需要管理员在云厂商控制台开启 SMTP 并获取 SMTP 密码/授权码。

### 3.2 未采纳方案

- **方案 B：SMTP + 腾讯云/阿里云官方 API 适配器**。官方 API 更规范，但需要引入两个较重的 Go SDK；腾讯云 SES 的 `Simple` 已标记废弃，通常要预先申请模板并配置模板 ID/变量；阿里云也要验证发信域名。配置项、测试量和上线门槛明显更高。
- **方案 C：只做 SMTP，不做厂商预设**。管理员需要自己查 host/port/TLS，后台体验差，容易配错。

### 3.3 发送方式：同步发送（采纳）

- 注册、重发、忘记密码在请求内同步调用 SMTP，超时约 10–20 秒。
- 理由：邮件量低；同步能立即知道是否发送成功；避免为低频邮件新增 asynq worker、队列和重试语义。
- 失败处理：
  - 强制验证开启时，账号已创建但邮件发送失败 → 注册仍返回 201，`verificationEmailSent=false`，前端提示重发；不删除账号，避免重试注册撞唯一键。
  - 重发/忘记密码发送失败 → 仍返回通用成功信息并记录服务端日志，避免枚举；管理员可通过测试邮件排查。
- 以后若需要重试/削峰，可在 `mail.Sender` 接口后替换为队列实现，不影响认证流程。

### 3.4 后台配置与密钥（采纳）

- 邮件配置存 PostgreSQL 单例表 `mail_settings`，不写环境变量（除加密密钥）。
- SMTP 密码使用 AES-256-GCM 加密后落库，API 只返回 `passwordState`，不返回明文或密文。
- 邮件加密密钥独立于 AI 加密密钥：
  - 环境变量 `KAPOJ_MAIL_ENCRYPTION_KEY`（base64 32 字节）优先。
  - 未配置时使用 `KAPOJ_MAIL_KEY_FILE`，默认 `data/mail.key`；首次保存密码时自动生成 0600 文件。
  - 密钥丢失时旧密文不可解，管理员重新填写 SMTP 密码即可恢复。
- 复用/抽取现有 `ai` 包的 AES-GCM 原语到 `internal/secret`，AI 包保留兼容别名；邮件与 AI 使用各自独立的 key manager，互不影响轮换。

### 3.5 强制策略（采纳）

- `requireEmailVerification` 默认 `true`。这是“注册必须验证”的默认安全姿态。
- 存量用户迁移时 `email_verified_at = created_at`，视为已验证。
- 如果强制验证开启但邮件服务未就绪，注册返回 `50301`，而不是创建无法使用的账号。后台保存配置时也拒绝“开启强制验证但服务未就绪”的组合。
- 管理员可先关闭强制验证完成上线/配置，配置并测试通过后再打开。
- **引导账号例外**：`users` 表为空时，第一个注册账号跳过邮箱验证并直接标记为已验证，用于首次部署。该账号仍只是普通用户，需按现有运行手册通过 SQL 授予 `super_admin`，因此不会给匿名访问者额外的管理权限；后续账号一律按开关执行。

## 4. 数据与 Redis 结构

### 4.1 users 表

新增字段：

```go
EmailVerifiedAt *time.Time `gorm:"type:timestamptz"`
```

- 新注册用户为 `NULL`（未验证）。
- 验证成功后写入当前 UTC 时间。
- `user.MeDTO` 增加 `emailVerified bool`（`EmailVerifiedAt != nil`）。
- 迁移策略（方案 A，要求崩溃安全、可恢复）：
  1. `db.Migrate` 使用 PostgreSQL advisory lock 串行化迁移；新增一次性迁移记录表：
     ```sql
     CREATE TABLE IF NOT EXISTS db_migrations (
       key text PRIMARY KEY,
       applied_at timestamptz NOT NULL
     );
     ```
  2. 执行原有 `AutoMigrate` 添加 `email_verified_at`。
  3. 在事务中检查 `db_migrations` 是否已有 `email_verified_at_backfill_v1`；没有则执行：
     ```sql
     UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL;
     ```
     并在同一事务插入 `email_verified_at_backfill_v1` 标记。
  4. 如果进程在 `AutoMigrate` 后、标记提交前崩溃，下次启动标记仍缺失，会继续回填；标记提交成功后不再回填，因此不会把新注册的未验证用户误标为已验证。
  5. 运行手册注明：升级时先停止旧版本实例（或确保没有旧版本在写 `users`），再启动新版本完成回填，避免滚动发布窗口内旧实例创建的新用户被误标记为已验证。
- 新建库没有用户，回填为空操作；老库首次升级时所有存量用户变为已验证。

### 4.2 mail_settings 表（单例）

```go
type Settings struct {
    ID                       int16     `gorm:"primaryKey"`
    Provider                 string    `gorm:"size:32;not null;default:'qq'"`
    SMTPHost                 string    `gorm:"size:255;not null;default:'smtp.qq.com'"`
    SMTPPort                 int       `gorm:"not null;default:465"`
    SMTPTLS                  string    `gorm:"size:16;not null;default:'ssl'"`
    SMTPUsername             string    `gorm:"size:255;not null;default:''"`
    SMTPPasswordEncrypted    string    `gorm:"type:text;not null;default:''"`
    FromEmail                string    `gorm:"size:254;not null;default:''"`
    FromName                 string    `gorm:"size:100;not null;default:'kapOJ'"`
    ReplyTo                  string    `gorm:"size:254;not null;default:''"`
    PublicBaseURL            string    `gorm:"size:500;not null;default:''"`
    SendingEnabled           bool      `gorm:"not null;default:false"`
    RequireEmailVerification bool      `gorm:"not null;default:true"`
    UpdatedBy                int64     `gorm:"not null;default:0"`
    UpdatedAt                time.Time `gorm:"type:timestamptz;not null"`
}
```

- `Provider`：`qq` / `tencent_ses` / `aliyun_dm` / `custom`。
- `SMTPTLS`：`ssl`（隐式 TLS，常见 465）/ `starttls`（常见 587）/ `none`（仅自定义 provider 使用，后台显示风险提示）。
- `Ready` 为派生状态：
  - `SendingEnabled == true`
  - host 非空、port 合法
  - `FromEmail` 合法
  - `PublicBaseURL` 为合法 http(s) origin
  - 预设 provider 要求 `SMTPUsername` 非空且密码状态为 `ready`；`custom` 允许匿名 SMTP
- 密码状态 `passwordState` 必须实际尝试解密，不能只看密文是否非空：
  - `none`：未配置密码；
  - `ready`：密文可用当前邮件密钥解密；
  - `key_error`：有密文但当前密钥无法解密（密钥丢失/轮换）。
  - `Ready` 要求预设 provider 的 `passwordState == ready`；`key_error` 时后台显示“密钥不匹配，请重新填写 SMTP 密码”。
- `RequireEmailVerification` 只有在 `Ready == true` 时才能保存为 `true`。
- 保存时若 `passwordState == key_error` 且未提供新密码，拒绝开启 `sendingEnabled` 或 `requireEmailVerification`。

### 4.3 Redis 结构

```text
email_verify:<sha256(token)>          -> userId    TTL 24h
email_verify_user:<userId>            -> tokenHash TTL 24h
email_verify_cooldown:<userId>        -> 1         TTL 60s
email_verify_ip:<ip>                  -> count     TTL 1h
password_reset:<sha256(token)>        -> userId    TTL 30m
password_reset_user:<userId>          -> tokenHash TTL 30m
password_reset_email:<sha256(email)>  -> 1         TTL 60s
password_reset_ip:<ip>                -> count     TTL 1h
email_attempt_ip:<ip>                 -> count     TTL 5m
```

- token：32 字节 `crypto/rand`，`base64.RawURLEncoding`；只存 SHA-256 hex。
- 重新签发验证/重置 token 时，先删除该用户旧 token，保证同一时间只有最新链接有效。
- 消费 token 使用 Redis Lua 脚本或 `GETDEL` + 索引清理，确保一次性。
- 频率限制：
  - 重发验证：同一用户 60 秒冷却；同一 IP 5 次/小时。
  - 忘记密码：同一邮箱 60 秒冷却；同一 IP 5 次/小时。
  - 验证/重置尝试：同一 IP 20 次/5 分钟，防 token 暴力尝试。
- 客户端 IP：新增 `KAPOJ_TRUSTED_PROXIES`（逗号分隔 CIDR，默认空 = 不信任任何代理头）。`router.New` 调用 `SetTrustedProxies` 后再使用 `c.ClientIP()`；默认直连场景使用 `RemoteAddr`。反向代理部署必须在运行手册中配置，否则所有用户共享代理 IP，IP 限流会误伤。
- 所有 token 不得写日志；日志只记录 userID、邮箱脱敏值、错误类型。

### 4.4 厂商预设

| provider | 名称 | host | port | tls | 备注 |
|---|---|---|---|---|---|
| `qq` | QQ 邮箱 | `smtp.qq.com` | 465 | ssl | 密码填 QQ 邮箱“授权码”，不是登录密码 |
| `tencent_ses` | 腾讯云 SES | `smtp.qcloudmail.com` | 465 | ssl | 控制台开启 SMTP，使用 SMTP 密码 |
| `aliyun_dm` | 阿里云邮件推送 | `smtpdm.aliyun.com` | 465 | ssl | 控制台开启 SMTP，使用 SMTP 密码 |
| `custom` | 自定义 SMTP | 空 | 587 | starttls | 可配置匿名 SMTP，用于内网/测试 |

- 预设只用于后台自动填值；host/port/tls 仍可手动修改。
- 发信地址必须与厂商要求的发信域名/发信地址匹配；后台测试邮件用于验证。

### 4.5 加密密钥

- 新增配置：
  - `KAPOJ_MAIL_ENCRYPTION_KEY`：可选，base64 32 字节；非法时启动失败。
  - `KAPOJ_MAIL_KEY_FILE`：默认 `data/mail.key`。
  - `KAPOJ_MAIL_TIMEOUT`：SMTP 连接/读写超时，默认 `20s`。
  - `KAPOJ_TRUSTED_PROXIES`：逗号分隔 CIDR，默认空（不信任代理头）。
- 首次保存 SMTP 密码且未配置密钥时，自动生成 32 字节随机密钥并写入 key file（0600，原子写）。
- 环境变量密钥只读；文件密钥可由后台保存动作生成。
- 邮件密钥与 AI 密钥独立；AI 密钥轮换不影响邮件设置。

## 5. 后端设计

### 5.1 mail 包

新增 `apps/server/internal/mail`：

- `Message{To, Subject, Text, HTML}`。
- `Sender` 接口：
  ```go
  type Sender interface {
      Send(ctx context.Context, msg Message) error
  }
  ```
- `Config`：provider、host、port、tls、username、password、fromEmail、fromName、replyTo。
- `NewSMTPSender(cfg Config) (Sender, error)`：校验配置，返回 SMTP 发送器。
- SMTP 实现：
  - `ssl`：`tls.DialWithDialer` + `smtp.NewClient`。
  - `starttls`：`smtp.Dial` + `StartTLS`。
  - `none`：仅 `custom` provider 允许；后台显示风险提示。
  - 有用户名时使用 `smtp.PlainAuth`；无用户名时跳过 AUTH。
  - 连接/读写 deadline 由 `KAPOJ_MAIL_TIMEOUT`（默认 20s）控制。
- 消息构造：
  - `multipart/alternative`，同时包含 `text/plain` 与 `text/html`。
  - Subject 使用 `mime.QEncoding.Encode("UTF-8", subject)`。
  - From 使用 `mail.Address{Name, Address}`。
  - 生成 `Date`、`Message-ID`、`MIME-Version`。
  - 所有 header 输入拒绝/剔除 CR/LF，防注入。
- 模板：
  - `SendVerification(ctx, to, nickname, token)`：主题“验证你的 kapOJ 邮箱”，24 小时有效。
  - `SendPasswordReset(ctx, to, nickname, token)`：主题“重置你的 kapOJ 密码”，30 分钟有效。
  - `SendTest(ctx, to)`：主题“kapOJ 邮件配置测试”。
  - 链接：`{PublicBaseURL}/verify-email?token=...`、`{PublicBaseURL}/reset-password?token=...`。
  - HTML 使用 `html/template` 自动转义；不引用外部图片/字体。
- `Service`：
  - `Get/Update` 设置（加密密码、校验、持久化）。
  - `Ready(ctx)`、`VerificationRequired(ctx)`。
  - `SendVerification`/`SendPasswordReset`/`SendTest`：每次发送读取当前设置，配置变更立即生效。
  - 未就绪时返回 `httpx.ErrMailNotConfigured`；SMTP 失败返回 `httpx.ErrMailSendFailed`。
  - 可注入 `SenderFactory`，测试使用假发送器。
- 错误：
  - `ErrMailNotConfigured` → 50301。
  - `ErrMailSendFailed` → 50201。
  - `ErrInvalidEmailToken` → 40003。
  - `ErrEmailNotVerified` → 40303。
  - `ErrTooManyEmailRequests` → 42905。

### 5.2 后台 API

新增 `apps/server/internal/mailapi`，并新增权限与审计：

- 权限：`mail.settings.manage`，分组「邮件」，默认仅 super（Seed 只补目录，不自动授权已有角色）。
- 审计：
  - `AuditActionMailSettingsManage = "mail.settings.manage"`
  - `AuditTargetMailSettings = "mail_settings"`
- 路由（均要求登录 + `mail.settings.manage`）：
  - `GET /api/admin/mail` → `AdminMailView`。
  - `PUT /api/admin/mail` → 保存并返回 `AdminMailView`。
  - `POST /api/admin/mail/test` → `{to}`，发送测试邮件，成功返回空数据。
- `AdminMailView`：
  - `provider`、`smtpHost`、`smtpPort`、`smtpTls`、`smtpUsername`
  - `passwordState`：`none` / `ready` / `key_error`（不返回密码/密文）
  - `fromEmail`、`fromName`、`replyTo`、`publicBaseUrl`
  - `sendingEnabled`、`requireEmailVerification`
  - `ready`、`updatedAt`
- `MailSettingsInput`：
  - 字段同上；`smtpPassword *string`：
    - 省略/null：保持现有密码；
    - 非空：加密替换；
    - 空字符串：清除密码。
  - 保存校验：
    - provider 枚举；tls 枚举；port 1–65535。
    - host/fromEmail/publicBaseUrl 格式；publicBaseUrl 只允许 http(s) origin，不允许 query/fragment。
    - `sendingEnabled=true` 时必须 `ready=true`。
    - `requireEmailVerification=true` 时必须 `sendingEnabled=true` 且 `ready=true`。
    - 预设 provider 必须有 username 和密码（新密码或已有密码）。
  - 写操作 best-effort 审计，失败仅 warning。
- 测试邮件：
  - 使用已保存配置，不接受前端传入的密码/主机，避免测试接口变成任意 SMTP 探测器。
  - 收件人邮箱校验；成功/失败都写日志，成功写审计。
- 默认行：
  - `provider=qq`、`smtpHost=smtp.qq.com`、`smtpPort=465`、`smtpTls=ssl`、`fromName=kapOJ`、`sendingEnabled=false`、`requireEmailVerification=true`。
  - 首次部署在未配置邮件前，除首个引导账号外，后续注册会返回 50301；这是有意的 fail-closed 行为，文档必须写明。

### 5.3 认证流程

Auth Service 增加可选依赖：

- `Mailer` 接口：
  ```go
  type Mailer interface {
      VerificationRequired(ctx context.Context) (bool, error)
      Ready(ctx context.Context) (bool, error)
      SendVerification(ctx context.Context, to, nickname, token string) error
      SendPasswordReset(ctx context.Context, to, nickname, token string) error
  }
  ```
- `EmailTokenStore`（Redis）：签发/消费/重发频率限制。
- `NewService` 使用 variadic options，保持现有调用兼容：
  - `WithMailer(m Mailer)`
  - `WithEmailTokens(s *EmailTokenStore)`
- `mailer == nil` 时视为未开启强制验证（兼容现有测试/旧装配）。

注册：

1. 校验用户名/邮箱/密码/昵称。
2. **引导账号例外**：若 `users` 表为空，创建第一个用户并直接写入 `EmailVerifiedAt=now`，跳过邮箱验证、创建会话并返回；用于首次部署。仓储增加 `Count` 或复用 `List(ctx,0,1)` 判断。
3. 若 mailer 存在：
   - `required, err := mailer.VerificationRequired(ctx)`。
   - 若 `required && !mailer.Ready(ctx)` → `50301`，不创建用户。
4. 创建用户，`EmailVerifiedAt=nil`。
5. 若 `required`：
   - 签发验证 token。
   - 发送验证邮件；失败只记录日志，`VerificationEmailSent=false`。
   - 返回 `RegisterResult{VerificationRequired:true, Email, VerificationEmailSent}`，不创建会话，不触发登录钩子。
6. 若 `!required`：
   - 若邮件就绪，签发 token 并 best-effort 发送验证邮件。
   - 创建会话，触发登录钩子。
   - 返回 `RegisterResult{Session, VerificationRequired:false, ...}`。
7. 重复用户名/邮箱仍返回 40901/40902。

`RegisterResult`：

```go
type RegisterResult struct {
    Session               *Session
    VerificationRequired  bool
    Email                 string
    VerificationEmailSent bool
}
```

- API 层映射为 `{verificationRequired, email, verificationEmailSent, session?}`；`session` 仅在自动登录时出现。
- 空库并发注册的极端窗口由运行手册规避（部署时先完成首个账号注册再开放访问）；引导账号仍需 SQL 授予 `super_admin`。

登录：

1. 现有密码校验、封禁校验。
2. 若 mailer 存在且 `VerificationRequired` 为 true，且 `EmailVerifiedAt == nil` → `40303`。
3. 成功后重置登录失败计数并创建会话。

刷新：

1. 现有 refresh 校验、用户/封禁校验。
2. 若强制验证开启且用户未验证 → `40303`。
3. 与现有实现一致，refresh token 在用户校验前已被消费，因此返回 `40303` 后旧 refresh token 已失效；用户需重新登录触发 `40303`，或直接使用验证链接。验证成功接口会返回新会话，前端用它覆盖旧会话。

邮箱验证：

- `POST /api/auth/email/verify {token}`：
  1. 消费 token；无效/过期 → 40003。
  2. 查用户；不存在/封禁 → 40003/40301。
  3. 若未验证，写入 `email_verified_at=now`；已验证则幂等。
  4. 删除该用户其余验证 token。
  5. 创建会话并触发登录钩子；返回 Session。
- `POST /api/auth/email/resend {email}`：
  1. 规范化邮箱；邮件未就绪 → 50301。
  2. IP 频率限制；超限 → 42905。
  3. 无论邮箱是否存在都返回 200 通用信息。
  4. 用户存在、未封禁、未验证时：检查 60 秒冷却；签发新 token 并发送；发送失败只记录日志。
- `POST /api/auth/password/forgot {email}`：
  1. 规范化邮箱；邮件未就绪 → 50301。
  2. IP 频率限制；超限 → 42905。
  3. 无论邮箱是否存在都返回 200 通用信息。
  4. 用户存在且未封禁时：检查邮箱冷却；签发重置 token 并发送；失败只记录日志。
- `POST /api/auth/password/reset {token,newPassword}`：
  1. 校验新密码 8–72 字节；不合法 → 40001（此时不消费 token）。
  2. 消费 token；无效/过期 → 40003。
  3. 查用户/封禁；不存在 → 40003；封禁 → 40301。
  4. `RevokeAll` 吊销全部会话。
  5. 更新密码 hash。
  6. 若 `email_verified_at` 为空，写入当前时间：能点击邮件重置链接即证明邮箱控制权。同时删除该用户剩余的验证 token，避免旧验证链接在重置后再签发会话。
  7. 返回 200，不自动登录；前端跳登录。
- 现有 `PUT /api/users/me/password` 修改密码逻辑不变。

### 5.4 全站未验证拦截

- Access token claims 增加 `emailVerified bool`：
  - `TokenIssuer.IssueAccess` 根据 `u.EmailVerifiedAt != nil` 写入。
  - 旧 token 没有该 claim，解析为 `false`，由中间件回查数据库兼容。
- 新增 `middleware.RequireVerifiedEmail(policy, users)`：
  1. 无 claims → 交给后续 `RequireAuth`（或直接放行，由后续处理）。
  2. `claims.EmailVerified == true` → 放行。
  3. 查询当前 `VerificationRequired`；关闭 → 放行。
  4. 查询用户；`EmailVerifiedAt != nil` → 放行（兼容旧 token / 多设备验证）。
  5. 否则 → `40303`。
  6. 查询失败 → `50300`。
- 应用到所有认证路由组：`api`、`problemJSON`、`uploads`、`imports`。
- `auth.RegisterRoutes` 的公开接口不受影响；`/auth/logout` 仍是公开路由，未验证用户可退出。
- 前端处理：
  - 非 `/auth/*` 请求收到 `40303` → 统一跳转 `/verify-email`，不清空 token；验证成功后用新会话覆盖。
  - `/auth/login` 的 `40303` 由 LoginView 自行处理，保留内联“邮箱尚未验证”提示与带入的邮箱；全局跳转不拦截登录接口。
  - 自动 refresh 收到 `40303` 时调用同一个“去验证邮箱”钩子，不静默清空会话。

### 5.5 API 汇总

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/auth/register` | 公开 | 注册；强制验证时返回 `RegisterResult`，否则含 Session |
| POST | `/api/auth/login` | 公开 | 未验证且强制开启 → 40303 |
| POST | `/api/auth/refresh` | 公开 | 未验证且强制开启 → 40303 |
| POST | `/api/auth/logout` | 公开 | 不变 |
| POST | `/api/auth/email/verify` | 公开 | 消费 token，标记已验证，返回 Session |
| POST | `/api/auth/email/resend` | 公开 | 通用成功信息；防枚举 |
| POST | `/api/auth/password/forgot` | 公开 | 通用成功信息；防枚举 |
| POST | `/api/auth/password/reset` | 公开 | 重置密码并吊销全部会话 |
| GET | `/api/admin/mail` | `mail.settings.manage` | 邮件设置视图 |
| PUT | `/api/admin/mail` | `mail.settings.manage` | 保存邮件设置 |
| POST | `/api/admin/mail/test` | `mail.settings.manage` | 发送测试邮件 |

### 5.6 错误码

| code | HTTP | 消息 | 场景 |
|---|---|---|---|
| 40003 | 400 | 链接无效或已过期 | 验证/重置 token 无效、过期、已消费 |
| 40303 | 403 | 邮箱尚未验证，请先完成邮箱验证 | 强制验证开启且用户未验证 |
| 42905 | 429 | 邮件请求过于频繁，请稍后再试 | 重发/忘记密码/验证尝试超限 |
| 50201 | 502 | 邮件发送失败，请稍后重试 | SMTP/云 SMTP 发送失败 |
| 50301 | 503 | 邮件服务未配置或未就绪 | 未配置、未启用、强制验证但服务未就绪 |
| 50300 | 503 | 依赖服务不可用 | Redis/数据库不可用（沿用） |

## 6. 前端设计

### 6.1 路由与页面

- `/verify-email`，name `verify-email`，公开，title「验证邮箱」。
  - 有 `token` query：进入即调用验证接口；显示验证中/成功/失败。
  - 成功：`auth.setSession(session)`，Toast，跳首页。
  - 失败/无 token：显示邮箱输入 + 重发按钮；重发成功显示通用提示。
  - 支持从登录页带入 `email` query 预填。
- `/forgot-password`，name `forgot-password`，`guestOnly`，title「找回密码」。
  - 邮箱输入 → 提交 → 通用成功面板；60 秒倒计时后可重发。
- `/reset-password`，name `reset-password`，`guestOnly`，title「重置密码」。
  - 从 query 读取 token；新密码 + 确认密码；提交成功 → 提示 + 去登录。
  - token 缺失/无效 → 过期提示 + 去「找回密码」。
- 现有 `/login`、`/register` 保持 `guestOnly`。
- 受保护页面未登录仍由现有 `authGuard` 带 `redirect` 跳登录/注册，不改语义。

### 6.2 注册页

- 提交后根据 `RegisterResult`：
  - `verificationRequired=true`：不跳首页，显示“验证邮件已发送至 xxx”面板。
    - `verificationEmailSent=false`：显示警告“邮件发送失败，请稍后重试”。
    - 重发按钮 60 秒倒计时；调用 `/auth/email/resend`。
    - 提供“返回登录”。
  - `verificationRequired=false`：保持原行为，自动登录并跳首页。
- 注册表单校验不变。

### 6.3 登录页

- 增加“忘记密码？”链接到 `/forgot-password`。
- 捕获 `40303`：
  - 显示“邮箱尚未验证”。
  - 提供“去验证邮箱”链接，带 `email` query（若输入的是邮箱）。
  - 不自动重发，避免误触。

### 6.4 后台邮件设置页

- 路由 `/admin/mail`，meta `permission: PERM.mail.settings.manage`。
- AdminNav 新增分组「系统」/「邮件设置」，仅该权限可见；加入 `ADMIN_ACCESS_PERMISSIONS`。
- 页面结构：
  - 状态卡片：邮件服务是否就绪、强制验证是否生效、当前 provider。
  - 发件配置：
    - provider 下拉（QQ / 腾讯云 SES / 阿里云邮件推送 / 自定义 SMTP）。
    - 切换 provider 时按预设填充 host/port/tls；用户已手改的值不强制覆盖（或提供“应用预设”按钮）。
    - host、port、TLS 模式、SMTP 用户名、SMTP 密码（Password，占位“已配置，留空不修改”）。
    - 发件邮箱、发件人名称、Reply-To。
    - 站点公开地址（`publicBaseUrl`，用于生成邮件链接）。
  - 开关：
    - `sendingEnabled` 启用邮件发送。
    - `requireEmailVerification` 强制邮箱验证；未就绪时禁止打开并提示。
  - 操作：
    - 保存（校验 + Toast）。
    - 测试发送：收件人输入 + 按钮；使用已保存配置；成功/失败 Toast。
  - 安全：
    - 永不回显密码/密文；只显示 `passwordState`：未配置 / 已就绪 / 密钥不匹配。
    - `none` TLS 仅自定义 provider 可选，并显示风险提示。

### 6.5 API/Store/类型

- `api/auth.ts` 新增：
  - `verifyEmail(token)`
  - `resendVerification(email)`
  - `forgotPassword(email)`
  - `resetPassword(token, newPassword)`
- `api/admin.ts` 或 `api/mail.ts` 新增：
  - `getAdminMail()`
  - `updateAdminMail(input)`
  - `testAdminMail(to)`
- `api/types.ts` 新增：
  - `RegisterResult`、`EmailVerificationInput`、`ResendVerificationInput`、`ForgotPasswordInput`、`ResetPasswordInput`
  - `AdminMailView`、`MailSettingsInput`
  - `Me` 增加 `emailVerified`
- `auth` store：
  - `register` 返回 `RegisterResult`；仅当 `session` 存在时 `setSession` 并 `fetchMe`。
  - 不持久化邮箱 token。
- `http` 客户端：
  - `AuthHooks` 增加 `onEmailNotVerified()`。
  - 非 `/auth/*` 请求收到 `40303` 时调用；`client.ts` 跳转 `/verify-email`，避免重复跳转。
  - 登录接口的 `40303` 不触发全局跳转，由 LoginView 内联展示；自动 refresh 的 `40303` 触发同一钩子，不清空本地会话。
- OpenAPI 更新后执行 `pnpm gen:api`，同步 `schema.d.ts`。

## 7. 安全与测试

### 安全

- 强制验证开启时，注册、登录、刷新、受保护接口四条路径都必须拦住未验证用户。
- 存量用户迁移只回填一次；新注册未验证用户不得被回填。
- 邮箱验证/重置 token：32 字节随机、只存哈希、单次消费、短 TTL、重发轮换、频率限制。
- 忘记密码/重发接口始终返回通用成功信息，避免邮箱枚举；发送失败只写日志。
- 邮件链接使用后台配置的 `publicBaseUrl`，不信任请求 Host。
- SMTP 密码 AES-256-GCM 加密；API 不返回明文/密文；日志不记录密码、token、完整邮箱。
- 邮件头 CR/LF 清洗，防 header injection。
- 后台邮件设置独立权限 `mail.settings.manage`，保存/测试写审计；未登录 401、无权限 403。
- `none` TLS 仅自定义 provider 可用，并在 UI 明示风险。
- 密码重置成功后吊销全部会话；旧密码立即失效。
- 管理后台默认 fail-closed：强制验证开启但邮件未就绪时，除首个引导账号外，注册返回 50301，而不是创建不可用账号。

### 后端测试

- `mail`：
  - 配置校验、provider 预设、`Ready` 派生状态。
  - MIME 消息构造：中文主题编码、multipart、From/To、Date/Message-ID、CRLF 清洗。
  - SMTP 发送：用进程内假 SMTP 服务器覆盖 `starttls`/`none`；`ssl` 至少覆盖配置校验与手工冒烟。
  - 加密：密码加密/解密、留空保持、清除密码、密钥缺失/错误；密钥丢失时 `passwordState=key_error` 且 `Ready=false`，不能开启强制验证。
  - 模板：验证/重置链接、昵称转义、过期时间。
- 设置/后台：
  - 单例默认值；保存校验；未就绪不能开启强制验证；密码不回显。
  - handler 401/403/200；保存/测试审计；非法 provider/port/tls/URL。
- 认证：
  - Token store：签发/消费/过期/重发轮换/一次性/频率限制。
  - 注册：强制开/关；首个引导账号例外；邮件未就绪 50301；发送失败 `verificationEmailSent=false`；关闭时自动登录。
  - 登录/刷新：未验证 40303；关闭时允许；封禁优先。
  - 验证：成功标记并返回 Session；已消费/过期 40003；幂等已验证。
  - 重发/忘记：通用 200、防枚举、冷却、IP 限流、发送失败仍 200。
  - 重置：密码校验先于消费；成功改密并吊销全部会话；无效 token 40003；未验证用户重置成功后变为已验证，旧验证 token 失效。
  - 中间件：claim true 放行；claim false + 强制开 + 未验证 40303；关闭/已验证放行；旧 token 回查兼容；依赖故障 50300。
  - 代理 IP：默认不信任 `X-Forwarded-For`；配置 `KAPOJ_TRUSTED_PROXIES` 后按可信代理解析。
- 迁移：
  - 旧库无 `email_verified_at` 时回填 `created_at` 并写入 `db_migrations` 标记；标记存在时不回填新用户；模拟“加列后崩溃”再启动可继续回填。
  - 首个注册账号（`users` 为空）跳过验证并标记已验证；第二个账号按开关执行。
- 路由集成：
  - 公开验证/重置接口可访问；受保护接口对未验证用户 40303；logout 可用。
  - `Me` 返回 `emailVerified`。
- OpenAPI 校验/生成类型。

### 前端测试

- RegisterView：强制验证成功面板、发送失败警告、重发倒计时、关闭时自动登录。
- VerifyEmailView：有 token 自动验证成功/失败、无 token 重发、错误提示。
- ForgotPasswordView：通用成功、防枚举文案、冷却。
- ResetPasswordView：token 缺失/无效、密码校验、成功跳登录。
- LoginView：40303 提示与“去验证邮箱”、忘记密码入口。
- AdminMailView：加载/保存/测试、密码不回显、未就绪禁止开启强制验证、权限/错误态。
- auth store：RegisterResult、session 可选、`emailVerified`。
- http 客户端：非登录接口 40303 跳转、不重复跳转；登录接口 40303 不跳转；refresh 40303 触发钩子且不静默清空会话。
- 路由/权限：`/admin/mail` 守卫、AdminNav 入口。

### 真实冒烟

- 用 QQ 邮箱授权码配置 SMTP；后台测试邮件收到。
- 新用户注册 → 收到验证邮件 → 链接验证 → 自动登录 → `/users/me.emailVerified=true`。
- 未验证用户登录 → 40303；重发 → 新链接可用，旧链接失效。
- 忘记密码 → 收到链接 → 重置 → 旧会话 refresh 失败、旧密码登录失败、新密码成功。
- 存量用户升级后直接可登录（迁移回填）。
- 关闭强制验证：新用户注册直接登录；打开后未验证用户被 40303 拦截，验证后恢复。
- 普通用户访问 `/api/admin/mail` 403，游客 401，super 200；保存/测试产生审计。
- 频率限制与错误码 40003/40303/42905/50201/50301。
- 腾讯云/阿里云 SMTP 有凭据时各做一次测试邮件；没有凭据时至少验证预设与配置保存。

## 8. 交付拆分

- **MAIL-A**：`internal/secret` 加密原语 + `mail` 包（SMTP sender、模板、配置、设置服务、假发送器）+ 单元测试。
- **MAIL-B**：`users.email_verified_at` 迁移 + `user.MeDTO` + `EmailTokenStore` + Auth Service 注册/登录/刷新/验证/重发/忘记/重置 + 中间件 + 测试。
- **MAIL-C**：`mail_settings` 模型/仓储 + `mailapi` 管理接口 + `mail.settings.manage` 权限/审计 + OpenAPI + 测试。
- **MAIL-D**：前端注册/登录改造 + `/verify-email`、`/forgot-password`、`/reset-password` + `/admin/mail` + 权限/导航/API/类型 + 测试。
- **MAIL-E**：`.env.example`、README、DEVELOPMENT、运行手册、OpenAPI 生成、全量检查、真实 SMTP 冒烟。运行手册必须补充：首次部署引导账号、升级时先停旧实例完成回填、`KAPOJ_TRUSTED_PROXIES` 配置、邮件密钥备份。

## 9. 留给实施计划的决定

- SMTP `ssl` 测试是否使用自签名 TLS 假服务器，或仅用配置校验 + 手工冒烟。
- 邮件 HTML 的具体视觉样式（保持简洁、无外部资源即可）。
- `publicBaseUrl` 是否允许多级路径前缀；当前设计为 origin。
- 加密原语抽取到 `internal/secret` 的具体文件拆分；若不抽取，需在 `mail` 内复制 AES-GCM 实现，但不得与 AI 共用密钥。
- 频率限制的具体实现（扩展现有 `LoginLimiter` 或新增通用 limiter）。
- 验证成功后自动登录是否触发 `user.login` 游戏化钩子；当前设计为触发。
- 后台 provider 切换时是否覆盖已手改的 host/port/tls；当前设计为“预设填充 + 可手改”，具体交互留给前端。
- 是否在账号安全页展示邮箱验证状态；当前设计不做，只改 `Me` 字段。
