# 邮箱验证与密码找回实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把注册改为可强制邮箱验证，新增 QQ/腾讯云/阿里云/自定义 SMTP 后台配置、验证邮件、重发、忘记密码与重置密码全流程。

**Architecture:** 新增独立 `mail` 域（SMTP sender、模板、单例设置、加密密码）与 `secret` 加密原语；认证域增加一次性 Redis token、验证/重置流程和 JWT `emailVerified` claim；后台新增 `mail.settings.manage` 权限与 `/admin/mail`；前端新增验证/找回密码页面并改造注册/登录。

**Tech Stack:** Go 1.26、Gin、GORM/PostgreSQL、Redis、net/smtp、Vue 3、PrimeVue、Pinia、OpenAPI。

**Spec:** `docs/superpowers/specs/2026-11-13-email-verification-design.md`

## Global Constraints

- 不引入腾讯云/阿里云 SDK，不新增邮件第三方 Go 依赖；只使用标准库 `net/smtp`、`crypto/tls`、`mime/*`。
- 邮件设置存 PostgreSQL 单例 `mail_settings`；SMTP 密码 AES-256-GCM 加密，API 只返回 `passwordState`。
- 新错误码固定为：40003、40303、42905、50201、50301；沿用 50300。
- 强制验证默认开启；除首个引导账号外，邮件未就绪时注册返回 50301。
- 存量用户首次迁移标记 `email_verified_at = created_at`；使用 `db_migrations` 标记 + advisory lock，且只回填一次。
- 所有邮件链接使用后台 `publicBaseUrl` 拼接；不信任请求 Host。
- 重发/忘记密码始终返回通用成功信息；发送失败只记日志。
- 后端响应统一 `{code,message,data}`；前端类型由 OpenAPI 生成。
- 现有未登录访问受保护页面的 `authGuard` 跳登录/注册行为不变。
- 提交按任务拆分；每个任务先写失败测试，再实现，测试通过后提交。

## Review Focus

- SMTP 输入：错误 TLS/端口、中文主题、CRLF header injection、空用户名匿名 SMTP；期望配置校验拒绝危险值，消息头安全编码。
- 未验证访问：登录、刷新、`api`/`problemJSON`/`uploads`/`imports` 四组受保护路由、首个引导账号；期望该拦的全拦、该放行的引导/关闭状态放行。
- Token 生命周期：过期、重复消费、重发轮换、重置后旧验证 token 失效；期望全部一次性且只认最新 token。
- 邮箱枚举与限流：重发/忘记密码对未知邮箱、已验证邮箱、超限、可信代理 IP；期望文案与状态码不泄露账号存在性，IP 解析不被 `X-Forwarded-For` 伪造。
- 密钥与 fail-closed：密钥丢失后 `passwordState=key_error`、`Ready=false`，后台不能开启强制验证；期望注册返回 50301 而不是创建收不到邮件的账号。

---

### Task 1: 抽取 secret 加密原语与邮件密钥管理

**Files:**
- Create: `apps/server/internal/secret/crypto.go`
- Create: `apps/server/internal/secret/key_manager.go`
- Create: `apps/server/internal/secret/crypto_test.go`
- Create: `apps/server/internal/secret/key_manager_test.go`
- Modify: `apps/server/internal/ai/crypto.go`

**Interfaces:**
- Produces:
  - `secret.NewEncryptor(base64Key string) (*secret.Encryptor, error)`
  - `(*secret.Encryptor).Encrypt(plaintext string) (string, error)`
  - `(*secret.Encryptor).Decrypt(ciphertext string) (string, error)`
  - `secret.NewKeyManager(envKey, filePath string) (*secret.KeyManager, error)`
  - `(*secret.KeyManager).Configured() bool`
  - `(*secret.KeyManager).Source() string`（`env` / `file` / `none`）
  - `(*secret.KeyManager).EnsureKey() error`（无密钥时生成 0600 文件）
  - `(*secret.KeyManager).Encrypt/Decrypt`
- AI 包保留 `ai.Encryptor`、`ai.NewEncryptor`、`ai.ErrInvalidKey` 兼容别名。

- [ ] **Step 1: 写失败测试**

`secret/crypto_test.go`：
```go
func TestEncryptorRoundTripAndWrongKey(t *testing.T) {
    enc, err := NewEncryptor(testKeyB64)
    require.NoError(t, err)
    ct, err := enc.Encrypt("smtp-password")
    require.NoError(t, err)
    assert.True(t, strings.HasPrefix(ct, "v1:"))
    got, err := enc.Decrypt(ct)
    require.NoError(t, err)
    assert.Equal(t, "smtp-password", got)
    other, _ := NewEncryptor(otherKeyB64)
    _, err = other.Decrypt(ct)
    assert.Error(t, err)
}
```

`secret/key_manager_test.go`：
```go
func TestEnsureKeyGeneratesFile(t *testing.T) {
    path := filepath.Join(t.TempDir(), "mail.key")
    km, err := NewKeyManager("", path)
    require.NoError(t, err)
    assert.False(t, km.Configured())
    require.NoError(t, km.EnsureKey())
    assert.True(t, km.Configured())
    assert.Equal(t, "file", km.Source())
    info, err := os.Stat(path)
    require.NoError(t, err)
    assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
    ct, err := km.Encrypt("x")
    require.NoError(t, err)
    got, err := km.Decrypt(ct)
    require.NoError(t, err)
    assert.Equal(t, "x", got)
}

func TestKeyManagerEnvIsReadOnly(t *testing.T) {
    km, err := NewKeyManager(testKeyB64, filepath.Join(t.TempDir(), "mail.key"))
    require.NoError(t, err)
    assert.Equal(t, "env", km.Source())
    assert.NoError(t, km.EnsureKey())
    _, err = os.Stat(filepath.Join(filepath.Dir(km.filePath), "unused"))
    assert.True(t, os.IsNotExist(err))
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/secret/...`
Expected: FAIL（包/函数不存在）

- [ ] **Step 3: 实现 secret 包**

把 `apps/server/internal/ai/crypto.go` 的 AES-GCM 实现迁到 `secret/crypto.go`，保留 `v1:` 前缀、base64 密钥、32 字节校验、12 字节 nonce、GCM 认证。`key_manager.go`：
- env key 非空：校验后 source=`env`，`EnsureKey` 直接返回 nil，不写文件。
- env 空：读 file；不存在/非法 → source=`none`，非法文件仅 warning。
- `EnsureKey`：已配置返回 nil；否则生成 32 字节随机、base64、原子写 0600，父目录 0700，source=`file`。
- `Encrypt/Decrypt` 未配置时返回 `errors.New("secret: key not configured")`。

- [ ] **Step 4: 运行 secret 测试**

Run: `cd apps/server && go test ./internal/secret/...`
Expected: PASS

- [ ] **Step 5: 让 AI 包复用 secret，保持兼容**

`ai/crypto.go` 改为：
```go
package ai

import "github.com/kaptree/kapoj/apps/server/internal/secret"

type Encryptor = secret.Encryptor
var NewEncryptor = secret.NewEncryptor
var ErrInvalidKey = secret.ErrInvalidKey
```
Run: `cd apps/server && go test ./internal/ai/...`
Expected: PASS（现有 AI 加密测试不变）

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/secret apps/server/internal/ai/crypto.go
git commit -m "feat(server): add secret encryption and mail key manager"
```

---

### Task 2: 邮件设置模型、仓储与就绪状态

**Files:**
- Create: `apps/server/internal/mail/model.go`
- Create: `apps/server/internal/mail/dto.go`
- Create: `apps/server/internal/mail/repository.go`
- Create: `apps/server/internal/mail/settings.go`
- Create: `apps/server/internal/mail/settings_test.go`

**Interfaces:**
- Produces:
  - `mail.Provider` 常量 `ProviderQQ/ProviderTencentSES/ProviderAliyunDM/ProviderCustom`
  - `mail.TLSMode` 常量 `TLSSSL/TLSStartTLS/TLSNone`
  - `mail.PasswordState` 常量 `PasswordNone/PasswordReady/PasswordKeyError`
  - `mail.Settings` GORM 模型，表名 `mail_settings`
  - `mail.Repository`、`mail.NewGormRepository(*gorm.DB)`、`mail.NewMemoryRepository()`
  - `mail.Service`、`mail.NewService(repo, keys, opts...)`
  - `(*mail.Service).Get(ctx) (*mail.AdminView, error)`
  - `(*mail.Service).Update(ctx, actorID int64, in mail.SettingsInput) (*mail.AdminView, error)`
  - `(*mail.Service).Ready(ctx) (bool, error)`
  - `(*mail.Service).VerificationRequired(ctx) (bool, error)`
- `WithTimeout`/`WithSenderFactory` 由 Task 3 在 `settings.go` 中追加，Task 2 只保留 `NewService` 的可变参数入口。

- [ ] **Step 1: 写失败测试**

`settings_test.go` 至少包含：
```go
func TestDefaultSettingsAreNotReady(t *testing.T) {
    svc := newTestService(t)
    v, err := svc.Get(context.Background())
    require.NoError(t, err)
    assert.Equal(t, ProviderQQ, v.Provider)
    assert.Equal(t, 465, v.SMTPPort)
    assert.True(t, v.RequireEmailVerification)
    assert.False(t, v.SendingEnabled)
    assert.False(t, v.Ready)
    assert.Equal(t, PasswordNone, v.PasswordState)
}

func TestCannotEnableEnforcementWhenNotReady(t *testing.T) {
    svc := newTestService(t)
    _, err := svc.Update(context.Background(), 1, SettingsInput{
        Provider: ProviderQQ, SMTPHost: "smtp.qq.com", SMTPPort: 465, SMTPTLS: TLSSSL,
        FromEmail: "noreply@example.com", PublicBaseURL: "https://oj.example.com",
        SendingEnabled: true, RequireEmailVerification: true,
    })
    assertCode(t, err, 40001) // 缺用户名/密码，不能 ready
}

func TestPasswordStateKeyErrorWhenKeyLost(t *testing.T) {
    svc := newTestServiceWithKey(t)
    saved, err := svc.Update(context.Background(), 1, completeInput("secret"))
    require.NoError(t, err)
    assert.Equal(t, PasswordReady, saved.PasswordState)
    svc.keys = nil // 模拟密钥丢失
    v, err := svc.Get(context.Background())
    require.NoError(t, err)
    assert.Equal(t, PasswordKeyError, v.PasswordState)
    assert.False(t, v.Ready)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/mail/...`
Expected: FAIL（类型/方法不存在）

- [ ] **Step 3: 实现模型与仓储**

按设计文档 4.2 的字段实现 `Settings`，`TableName() = "mail_settings"`。GORM 仓储：
- `Get` 惰性创建默认单例（`provider=qq`、`smtp_host=smtp.qq.com`、`smtp_port=465`、`smtp_tls=ssl`、`from_name=kapOJ`、`sending_enabled=false`、`require_email_verification=true`）。
- `Update` 写全部字段并设置 `updated_at`。
- 内存仓储用于单元测试。

- [ ] **Step 4: 实现设置服务**

- `Get` 读取设置，计算 `passwordState`、`ready`。
- `passwordState`：无密文 → none；`keys` 未配置或解密失败 → key_error；解密成功 → ready。
- `ready`：`SendingEnabled`、host、port、TLS、`FromEmail`、`PublicBaseURL` 合法；预设 provider 要求 username 非空且 passwordState=ready；custom 允许 username/password 都为空。
- `Update` 校验 provider/tls/port/host/fromEmail/replyTo/publicBaseUrl；`publicBaseUrl` 只允许 http(s) origin；`smtpPassword` 指针为 nil 保持、非空替换、空串清除；替换/清除时调用 `keys.EnsureKey()`/`Encrypt`。`keys == nil` 且需要加密密码时返回 50000（服务未正确装配），不要把空密钥当成可用。
- 若 `SendingEnabled` 或 `RequireEmailVerification` 为 true 而候选配置不 ready → `httpx.ErrValidation("邮件服务未就绪")`。
- `VerificationRequired` 返回当前 `RequireEmailVerification`。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/mail/...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/mail
git commit -m "feat(server): add mail settings model and readiness checks"
```

---

### Task 3: SMTP 发送器、MIME 消息与邮件模板

**Files:**
- Create: `apps/server/internal/mail/sender.go`
- Create: `apps/server/internal/mail/message.go`
- Create: `apps/server/internal/mail/templates.go`
- Create: `apps/server/internal/mail/sender_test.go`
- Create: `apps/server/internal/mail/message_test.go`
- Create: `apps/server/internal/mail/templates_test.go`

**Interfaces:**
- Produces:
  - `mail.Message{To, Subject, Text, HTML string}`
  - `mail.Sender`、`mail.SenderFactory`、`mail.NewSMTPSender(cfg mail.Config) (mail.Sender, error)`
  - `mail.Config{Provider, Host, Port, TLS, Username, Password, FromEmail, FromName, ReplyTo, Timeout}`
  - `mail.WithTimeout(d time.Duration) Option`
  - `mail.WithSenderFactory(f mail.SenderFactory) Option`
  - `(*mail.Service).SendVerification(ctx, to, nickname, token string) error`
  - `(*mail.Service).SendPasswordReset(ctx, to, nickname, token string) error`
  - `(*mail.Service).SendTest(ctx, to string) error`

- [ ] **Step 1: 写失败测试**

`message_test.go`：
```go
func TestBuildMessageEncodesChineseSubjectAndMultipart(t *testing.T) {
    raw, err := buildMessage(Config{
        FromEmail: "noreply@example.com", FromName: "kapOJ",
    }, Message{To: "alice@example.com", Subject: "验证你的 kapOJ 邮箱", Text: "hello", HTML: "<b>hello</b>"})
    require.NoError(t, err)
    msg, err := mail.ReadMessage(bytes.NewReader(raw))
    require.NoError(t, err)
    assert.Contains(t, msg.Header.Get("Subject"), "=?UTF-8?")
    assert.Contains(t, msg.Header.Get("Content-Type"), "multipart/alternative")
    body, _ := io.ReadAll(msg.Body)
    assert.Contains(t, string(body), "hello")
    assert.Contains(t, string(body), "<b>hello</b>")
}

func TestBuildMessageStripsHeaderInjection(t *testing.T) {
    raw, err := buildMessage(Config{FromEmail: "noreply@example.com"},
        Message{To: "alice@example.com", Subject: "hi\r\nBcc: evil@example.com", Text: "x"})
    require.NoError(t, err)
    assert.NotContains(t, string(raw), "Bcc: evil@example.com")
}
```

`sender_test.go` 启动进程内假 SMTP 服务器（`net.Listen("tcp", "127.0.0.1:0")`），实现 `EHLO/HELO`、`MAIL FROM`、`RCPT TO`、`DATA`、`QUIT`，断言：
- `TestSMTPSenderSendsMessage` 收到完整 MIME。
- `TestSMTPSenderRejectsMissingFrom` 返回错误。
- `TestSMTPSenderStartTLS` 覆盖 `starttls` 分支（测试服务器不要求真实 TLS 时只测 `none`；`starttls` 用 `smtp.NewClient` 的假服务覆盖 greeting/EHLO）。

`templates_test.go`：
```go
func TestVerificationTemplateEscapesNickname(t *testing.T) {
    _, html, err := renderVerification("<script>alert(1)</script>", "https://oj.example.com/verify-email?token=abc")
    require.NoError(t, err)
    assert.NotContains(t, html, "<script>alert(1)</script>")
    assert.Contains(t, html, "https://oj.example.com/verify-email?token=abc")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/mail/...`
Expected: FAIL（sender/message/template 不存在）

- [ ] **Step 3: 实现 MIME 消息构造**

`message.go`：
- `Config` 校验：FromEmail 非空；host/port 由调用方保证。
- `buildMessage`：清洗所有 header 的 CR/LF；`net/mail.ParseAddress` 解析 To；Subject 用 `mime.QEncoding.Encode("UTF-8", subject)`；From 用 `mail.Address`；Reply-To 可选；生成 `Date`、`Message-ID`（随机 hex + `@` + from domain）、`MIME-Version`；`multipart/alternative` 写 text/plain 与 text/html，两个 part 都用 quoted-printable。
- 不把密码、token 写日志。

- [ ] **Step 4: 实现 SMTP sender**

`sender.go`：
- `Config.Timeout` 默认 20s。
- `ssl`：`tls.DialWithDialer` + `smtp.NewClient`；`starttls`/`none`：自定义 `net.Dialer` + `smtp.NewClient`，starttls 再 `StartTLS`。
- 设置连接 deadline；context 取消时尽快失败。
- 有 username 时 `smtp.PlainAuth("", username, password, host)`；没有则跳过。
- `Mail`/`Rcpt`/`Data`/`Quit`；错误统一返回 `fmt.Errorf("mail: send: %w", err)`。

- [ ] **Step 5: 实现模板与 Service 发送方法**

`templates.go`：
- `renderVerification`、`renderPasswordReset`、`renderTest`，返回 text/html。
- 链接由 `settings.PublicBaseURL + "/verify-email?token=" + url.QueryEscape(token)` / `/reset-password?...`。
- HTML 用 `html/template`；文本用 `fmt.Sprintf`。
- `Service.SendVerification`/`SendPasswordReset`：读取设置、`Ready` 校验、构造链接、渲染、`senderFactory` 发送；未就绪 → `httpx.ErrMailNotConfigured`；发送失败 → `httpx.ErrMailSendFailed`。
- `SendTest` 使用固定内容，收件人校验。

- [ ] **Step 6: 运行测试**

Run: `cd apps/server && go test ./internal/mail/...`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/mail
git commit -m "feat(server): add SMTP sender and mail templates"
```

---

### Task 4: users.email_verified_at、仓储方法与一次性迁移

**Files:**
- Modify: `apps/server/internal/user/model.go`
- Modify: `apps/server/internal/user/dto.go`
- Modify: `apps/server/internal/user/repository.go`
- Modify: `apps/server/internal/user/memory.go`
- Modify: `apps/server/internal/user/repository_test.go`
- Modify: `apps/server/internal/db/db.go`
- Create: `apps/server/internal/db/email_migration_test.go`（`package db_test`，避免与 testutil 形成 import cycle）

**Interfaces:**
- Produces:
  - `user.User.EmailVerifiedAt *time.Time`
  - `user.MeDTO.EmailVerified bool`（JSON `emailVerified`）
  - `user.Repository.Count(ctx) (int64, error)`
  - `user.Repository.MarkEmailVerified(ctx, id int64, at time.Time) error`
  - `db.Migrate` 幂等执行 `email_verified_at_backfill_v1`

- [ ] **Step 1: 写失败测试**

`user/repository_test.go`：
```go
func TestMarkEmailVerified(t *testing.T) {
    repo := NewMemoryRepository()
    u := &User{Username: "alice", Email: "alice@example.com", Nickname: "alice", PasswordHash: "x", Role: RoleUser, Status: StatusActive}
    require.NoError(t, repo.Create(context.Background(), u))
    assert.Nil(t, u.EmailVerifiedAt)
    now := time.Now().UTC()
    require.NoError(t, repo.MarkEmailVerified(context.Background(), u.ID, now))
    got, err := repo.FindByID(context.Background(), u.ID)
    require.NoError(t, err)
    require.NotNil(t, got.EmailVerifiedAt)
    assert.WithinDuration(t, now, *got.EmailVerifiedAt, time.Second)
    assert.True(t, ToMe(got).EmailVerified)
}

func TestCount(t *testing.T) {
    repo := NewMemoryRepository()
    n, err := repo.Count(context.Background())
    require.NoError(t, err)
    assert.Zero(t, n)
    require.NoError(t, repo.Create(context.Background(), &User{...}))
    n, _ = repo.Count(context.Background())
    assert.Equal(t, int64(1), n)
}
```

`db/email_migration_test.go`（外部测试包，避免 `db` ↔ `testutil` import cycle）：
```go
package db_test

func TestMigrateBackfillsEmailVerifiedOnce(t *testing.T) {
    g := testutil.PG(t) // 已 Migrate 并清空所有表（包括 db_migrations）
    legacy := &user.User{Username: "legacy", Email: "legacy@example.com", Nickname: "legacy", PasswordHash: "x", Role: user.RoleUser, Status: user.StatusActive}
    require.NoError(t, g.Create(legacy).Error)

    // 模拟首次升级：标记不存在，Migrate 应回填。
    require.NoError(t, db.Migrate(g))
    var got user.User
    require.NoError(t, g.First(&got, legacy.ID).Error)
    require.NotNil(t, got.EmailVerifiedAt)

    // 第二个新用户不能被再次回填。
    fresh := &user.User{Username: "fresh", Email: "fresh@example.com", Nickname: "fresh", PasswordHash: "x", Role: user.RoleUser, Status: user.StatusActive}
    require.NoError(t, g.Create(fresh).Error)
    require.NoError(t, db.Migrate(g))
    require.NoError(t, g.First(&got, fresh.ID).Error)
    assert.Nil(t, got.EmailVerifiedAt)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/user/... ./internal/db/...`
Expected: FAIL（字段/方法不存在）

- [ ] **Step 3: 实现用户字段与仓储**

- `User` 增加 `EmailVerifiedAt *time.Time`，GORM 列 `email_verified_at`，不设默认值。
- `ToMe` 填 `EmailVerified: u.EmailVerifiedAt != nil`。
- GORM 仓储 `Count`、`MarkEmailVerified`（`Model(&User{}).Where("id = ?", id).Update("email_verified_at", at)`，RowsAffected=0 → ErrNotFound）。
- 内存仓储同步实现；`Create` 保留调用方传入的 `EmailVerifiedAt`。

- [ ] **Step 4: 实现迁移标记与 advisory lock**

- `db` 新增 `Migration{Key string; AppliedAt time.Time}`，表名 `db_migrations`。
- `Migrate` 用单事务：
  ```go
  return g.Transaction(func(tx *gorm.DB) error {
      if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x6b61706f6a)).Error; err != nil { return err }
      if err := tx.AutoMigrate(... &mail.Settings{}, &Migration{}); err != nil { return err }
      var count int64
      if err := tx.Model(&Migration{}).Where("key = ?", emailBackfillKey).Count(&count).Error; err != nil { return err }
      if count > 0 { return nil }
      if err := tx.Exec("UPDATE users SET email_verified_at = created_at WHERE email_verified_at IS NULL").Error; err != nil { return err }
      return tx.Create(&Migration{Key: emailBackfillKey, AppliedAt: time.Now().UTC()}).Error
  })
  ```
- `migrate` 列表增加 `&mail.Settings{}` 和 `&Migration{}`。
- 注意：Task 2 已创建 mail.Settings；本任务把它接入 `db.Migrate`。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/user/... ./internal/db/...`
Expected: PASS（无 `KAPOJ_TEST_DATABASE_URL` 时 db 测试 skip）

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/user apps/server/internal/db
git commit -m "feat(server): add email verification state and migration backfill"
```

---

### Task 5: Redis 一次性邮箱 token 与频率限制

**Files:**
- Create: `apps/server/internal/auth/email_tokens.go`
- Create: `apps/server/internal/auth/email_tokens_test.go`

**Interfaces:**
- Produces:
  - `auth.ErrInvalidEmailToken`
  - `auth.NewEmailTokenStore(rdb *redis.Client) *auth.EmailTokenStore`
  - `(*EmailTokenStore).IssueVerification(ctx, userID int64) (string, error)`
  - `(*EmailTokenStore).ConsumeVerification(ctx, token string) (int64, error)`
  - `(*EmailTokenStore).IssuePasswordReset(ctx, userID int64) (string, error)`
  - `(*EmailTokenStore).ConsumePasswordReset(ctx, token string) (int64, error)`
  - `(*EmailTokenStore).AllowResendIP(ctx, ip string) (bool, error)`
  - `(*EmailTokenStore).AllowResendUser(ctx, userID int64) (bool, error)`
  - `(*EmailTokenStore).AllowForgotIP(ctx, ip string) (bool, error)`
  - `(*EmailTokenStore).AllowForgotEmail(ctx, email string) (bool, error)`
  - `(*EmailTokenStore).AllowAttempt(ctx, ip string) (bool, error)`
  - `(*EmailTokenStore).InvalidateVerification(ctx, userID int64) error`

**Redis 常量：** 验证 24h、重置 30m、重发 60s、IP 5/hour、尝试 20/5min；key 按设计文档 4.3。

- [ ] **Step 1: 写失败测试**

用 `miniredis.RunT(t)` 构造 store，覆盖：
```go
func TestVerificationTokenIsOneTime(t *testing.T) {
    store, _ := newEmailStore(t)
    token, err := store.IssueVerification(ctx, 7)
    require.NoError(t, err)
    got, err := store.ConsumeVerification(ctx, token)
    require.NoError(t, err)
    assert.Equal(t, int64(7), got)
    _, err = store.ConsumeVerification(ctx, token)
    assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestReissueInvalidatesOldVerificationToken(t *testing.T) {
    store, _ := newEmailStore(t)
    old, _ := store.IssueVerification(ctx, 7)
    _, _ = store.IssueVerification(ctx, 7)
    _, err := store.ConsumeVerification(ctx, old)
    assert.ErrorIs(t, err, ErrInvalidEmailToken)
}

func TestResendLimits(t *testing.T) {
    store, _ := newEmailStore(t)
    ok, _ := store.AllowResendUser(ctx, 7)
    assert.True(t, ok)
    ok, _ = store.AllowResendUser(ctx, 7)
    assert.False(t, ok)
    for i := 0; i < 5; i++ {
        ok, _ = store.AllowResendIP(ctx, "1.2.3.4")
        assert.True(t, ok)
    }
    ok, _ = store.AllowResendIP(ctx, "1.2.3.4")
    assert.False(t, ok)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/auth/... -run 'Email|Token'`
Expected: FAIL（store 不存在）

- [ ] **Step 3: 实现 token 签发/消费**

- 32 字节 `crypto/rand`，`base64.RawURLEncoding`；SHA-256 hex 作为 key。
- Lua 脚本保证签发时删除旧 token、写入新 token 与 user 索引；消费脚本原子删除 token 并按 hash 清理 user 索引。
- 返回 `ErrInvalidEmailToken` 表示无效/过期/已消费。
- 日志不记录 token。

- [ ] **Step 4: 实现频率限制**

- `AllowResendUser`：`SET NX EX 60`。
- `AllowResendIP`/`AllowForgotIP`：`INCR` + `EXPIRE NX 1h`，上限 5。
- `AllowForgotEmail`：`SET NX EX 60` on `sha256(email)`。
- `AllowAttempt`：`INCR` + `EXPIRE NX 5m`，上限 20。
- Redis 错误返回 error，不吞掉。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/auth/... -run 'Email|Token'`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/auth/email_tokens.go apps/server/internal/auth/email_tokens_test.go
git commit -m "feat(server): add one-time email tokens and rate limits"
```

---

### Task 6: JWT emailVerified claim 与全站拦截中间件

**Files:**
- Modify: `apps/server/internal/auth/tokens.go`
- Modify: `apps/server/internal/middleware/auth.go`
- Modify: `apps/server/internal/auth/tokens_test.go`
- Modify: `apps/server/internal/middleware/auth_test.go`

**Interfaces:**
- Produces:
  - `auth.Claims.EmailVerified bool`
  - JWT payload `emailVerified`（旧 token 缺省 false）
  - `middleware.EmailVerificationPolicy` 接口 `VerificationRequired(ctx) (bool, error)`
  - `middleware.RequireVerifiedEmail(policy, users) gin.HandlerFunc`

- [ ] **Step 1: 写失败测试**

`auth/tokens_test.go`：
```go
func TestIssueAccessCarriesEmailVerified(t *testing.T) {
    issuer := NewTokenIssuer(testSecret, time.Hour)
    verified := time.Now()
    tok, err := issuer.IssueAccess(&user.User{ID: 1, Role: user.RoleUser, EmailVerifiedAt: &verified})
    require.NoError(t, err)
    claims, err := issuer.ParseAccess(tok)
    require.NoError(t, err)
    assert.True(t, claims.EmailVerified)
}
```

`middleware/auth_test.go`：
```go
func TestRequireVerifiedEmail(t *testing.T) {
    unverified := &user.User{ID: 1, Role: user.RoleUser}
    verifiedAt := time.Now()
    verified := &user.User{ID: 2, Role: user.RoleUser, EmailVerifiedAt: &verifiedAt}
    repo := user.NewMemoryRepository()
    require.NoError(t, repo.Create(context.Background(), unverified))
    require.NoError(t, repo.Create(context.Background(), verified))
    policy := fakePolicy{required: true}
    r := gin.New()
    r.GET("/x", func(c *gin.Context) {
        c.Set(auth.ClaimsContextKey, &auth.Claims{UserID: 1})
        c.Next()
    }, RequireVerifiedEmail(policy, repo), func(c *gin.Context) { c.Status(200) })
    // 断言未验证 -> 40303；已验证 -> 200；policy off -> 200。
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/auth/... ./internal/middleware/... -run 'EmailVerified|RequireVerified'`
Expected: FAIL

- [ ] **Step 3: 实现 claim**

- `Claims` 增加 `EmailVerified bool`。
- `jwtClaims` 增加 `EmailVerified bool json:"emailVerified"`。
- `IssueAccess` 设置 `EmailVerified: u.EmailVerifiedAt != nil`。
- `ParseAccess` 回填。
- 旧 token 缺省 false。

- [ ] **Step 4: 实现中间件**

- `EmailVerificationPolicy` 接口。
- `RequireVerifiedEmail`：
  - 无 claims 放行（后续 RequireAuth 处理）。
  - claim true 放行。
  - policy nil 放行。
  - `VerificationRequired` 错误 → `httpx.ErrUnavailable`。
  - 关闭 → 放行。
  - `users.FindByID`；不存在 → 401；错误 → 50300。
  - `EmailVerifiedAt != nil` 放行；否则 `httpx.ErrEmailNotVerified`。
- 新增错误码 `httpx.ErrEmailNotVerified`（Task 7 一并加，但本任务可先加）。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/auth/... ./internal/middleware/... -run 'EmailVerified|RequireVerified'`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/auth/tokens.go apps/server/internal/auth/tokens_test.go apps/server/internal/middleware apps/server/internal/httpx
git commit -m "feat(server): add emailVerified claim and verification middleware"
```

---

### Task 7: Auth Service 注册、验证、重发、忘记与重置流程

**Files:**
- Modify: `apps/server/internal/auth/service.go`
- Modify: `apps/server/internal/auth/service_test.go`
- Modify: `apps/server/internal/userapi/handler_test.go`（`Register` 返回 `RegisterResult`，取 `res.Session`）
- Modify: `apps/server/internal/httpx/errors.go`

**Interfaces:**
- Produces:
  - `auth.Mailer` 接口（`VerificationRequired`、`Ready`、`SendVerification`、`SendPasswordReset`）
  - `auth.RegisterResult{Session, VerificationRequired, Email, VerificationEmailSent}`
  - `auth.WithMailer(Mailer)`、`auth.WithEmailTokens(*EmailTokenStore)`
  - `(*Service).Register/RegisterWithMeta` 返回 `*RegisterResult`
  - `(*Service).VerifyEmail(ctx, token, meta) (*Session, error)`
  - `(*Service).ResendVerification(ctx, email, ip) error`
  - `(*Service).ForgotPassword(ctx, email, ip) error`
  - `(*Service).ResetPassword(ctx, token, newPassword) error`
- 新错误：`httpx.ErrInvalidEmailToken(40003)`、`ErrEmailNotVerified(40303)`、`ErrTooManyEmailRequests(42905)`、`ErrMailSendFailed(50201)`、`ErrMailNotConfigured(50301)`。

- [ ] **Step 1: 写失败测试**

`service_test.go` 增加 fake mailer：
```go
type fakeMailer struct {
    required bool
    ready bool
    verification []struct{ to, nickname, token string }
    reset []struct{ to, nickname, token string }
}
func (m *fakeMailer) VerificationRequired(context.Context) (bool, error) { return m.required, nil }
func (m *fakeMailer) Ready(context.Context) (bool, error) { return m.ready, nil }
func (m *fakeMailer) SendVerification(_ context.Context, to, nickname, token string) error { ... }
func (m *fakeMailer) SendPasswordReset(...) error { ... }
```

测试：
```go
func TestRegisterFirstUserBypassesVerification(t *testing.T) {
    svc, repo, _ := newServiceWithMail(t, true, false) // required=true, ready=false
    res, err := svc.Register(ctx, RegisterInput{Username: "admin", Email: "admin@example.com", Password: password})
    require.NoError(t, err)
    require.NotNil(t, res.Session)
    assert.False(t, res.VerificationRequired)
    u, _ := repo.FindByEmail(ctx, "admin@example.com")
    assert.NotNil(t, u.EmailVerifiedAt)
}

func TestRegisterRequiresMailWhenEnforced(t *testing.T) {
    svc, _, _ := newServiceWithMail(t, true, false)
    // 先创建一个引导用户，确保不是空库。
    _, err := svc.Register(ctx, RegisterInput{Username: "bootstrap", Email: "bootstrap@example.com", Password: password})
    require.NoError(t, err)
    _, err = svc.Register(ctx, RegisterInput{Username: "alice", Email: "alice@example.com", Password: password})
    assertCode(t, err, 50301)
}

func TestRegisterSendsVerificationWithoutSession(t *testing.T) {
    svc, _, mailer := newServiceWithMail(t, true, true)
    _, _ = svc.Register(ctx, RegisterInput{Username: "bootstrap", ...})
    res, err := svc.Register(ctx, RegisterInput{Username: "alice", ...})
    require.NoError(t, err)
    assert.True(t, res.VerificationRequired)
    assert.Nil(t, res.Session)
    assert.Len(t, mailer.verification, 1)
    assert.Equal(t, "alice@example.com", mailer.verification[0].to)
}

func TestLoginBlocksUnverifiedWhenEnforced(t *testing.T) { ... assertCode(t, err, 40303) }
func TestVerifyEmailMarksAndReturnsSession(t *testing.T) { ... }
func TestResendVerificationIsGenericForUnknownEmail(t *testing.T) { ... require.NoError ... }
func TestForgotPasswordIsGenericForUnknownEmail(t *testing.T) { ... }
func TestResetPasswordVerifiesEmailAndRevokesSessions(t *testing.T) { ... }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/auth/... -run 'Register|Login|Verify|Resend|Forgot|Reset'`
Expected: FAIL

- [ ] **Step 3: 增加错误码**

`httpx/errors.go`：
```go
ErrInvalidEmailToken = newErr(http.StatusBadRequest, 40003, "链接无效或已过期")
ErrEmailNotVerified = newErr(http.StatusForbidden, 40303, "邮箱尚未验证，请先完成邮箱验证")
ErrTooManyEmailRequests = newErr(http.StatusTooManyRequests, 42905, "邮件请求过于频繁，请稍后再试")
ErrMailSendFailed = newErr(http.StatusBadGateway, 50201, "邮件发送失败，请稍后重试")
ErrMailNotConfigured = newErr(http.StatusServiceUnavailable, 50301, "邮件服务未配置或未就绪")
```

- [ ] **Step 4: 实现 Service**

按设计文档 5.3 实现：
- `NewService` variadic options；`mailer`/`emailTokens` 可选。
- 注册：空库引导 → 创建已验证用户 + 会话；否则按 `required`/`ready` 分支；`required` 时签发 token、发送、返回无会话；非 required 时 best-effort 发送并创建会话。
- 登录/刷新：封禁后检查强制验证；未验证 → 40303。
- `VerifyEmail`：消费 token、标记 `MarkEmailVerified`、清理旧验证 token、创建会话。
- `ResendVerification`：`Ready` 检查 → IP 限制 → 查用户 → 用户冷却 → 签发/发送；未知/已验证/发送失败均返回 nil。
- `ForgotPassword`：`Ready` 检查 → IP 限制 → 查用户 → 邮箱冷却 → 签发/发送；未知/封禁/发送失败均返回 nil。
- `ResetPassword`：先 `ValidatePassword`；消费 token；查用户/封禁；`RevokeAll`；`UpdatePassword`；若未验证 `MarkEmailVerified`；`InvalidateVerification`。
- 日志只记录 userID 和 error，不记录 token/完整邮箱。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/auth/...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/auth apps/server/internal/httpx/errors.go
git commit -m "feat(server): add email verification and password reset flows"
```

---

### Task 8: 认证 HTTP 接口

**Files:**
- Modify: `apps/server/internal/auth/handler.go`
- Modify: `apps/server/internal/auth/handler_test.go`

**Interfaces:**
- Produces:
  - `POST /api/auth/register` → `registerResultDTO`
  - `POST /api/auth/email/verify` → `sessionDTO`
  - `POST /api/auth/email/resend` → `{message}`
  - `POST /api/auth/password/forgot` → `{message}`
  - `POST /api/auth/password/reset` → `null`

- [ ] **Step 1: 写失败测试**

`handler_test.go`：
```go
func TestRegisterEndpointVerificationRequired(t *testing.T) {
    r := newAuthEngineWithMail(t, true, true)
    w, resp := postJSON(r, "/api/auth/register", `{"username":"bootstrap","email":"bootstrap@example.com","password":"correct-horse"}`)
    require.Equal(t, 201, w.Code)
    // 第一个引导账号仍有 session
    w, resp = postJSON(r, "/api/auth/register", `{"username":"alice","email":"alice@example.com","password":"correct-horse"}`)
    require.Equal(t, 201, w.Code)
    data := resp["data"].(map[string]any)
    assert.True(t, data["verificationRequired"].(bool))
    assert.Nil(t, data["session"])
    assert.NotContains(t, w.Body.String(), "accessToken")
}

func TestVerifyEmailEndpoint(t *testing.T) { ... }
func TestResendEmailEndpointIsGeneric(t *testing.T) { ... }
func TestForgotPasswordEndpointIsGeneric(t *testing.T) { ... }
func TestResetPasswordEndpoint(t *testing.T) { ... }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/auth/... -run 'Endpoint'`
Expected: FAIL

- [ ] **Step 3: 实现 DTO 与路由**

- `registerResultDTO{VerificationRequired, Email, VerificationEmailSent, Session *sessionDTO}`；有 session 时填并 `notifyLogin`。
- `RegisterRoutes` 增加四个公开路由。
- `verifyEmail` 调用 service 并 `notifyLogin`。
- `resendEmail`/`forgotPassword` 调用 service 后用 `MessageView{Message: "如果该邮箱已注册..."}` 返回 200。
- `resetPassword` 返回 200 空数据。
- `sessionMeta` 复用。

- [ ] **Step 4: 运行测试**

Run: `cd apps/server && go test ./internal/auth/...`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/auth/handler.go apps/server/internal/auth/handler_test.go
git commit -m "feat(server): expose email verification and password reset endpoints"
```

---

### Task 9: 邮件管理 API、权限与审计

**Files:**
- Create: `apps/server/internal/mailapi/handler.go`
- Create: `apps/server/internal/mailapi/handler_test.go`
- Modify: `apps/server/internal/rbac/catalog.go`
- Modify: `apps/server/internal/rbac/audit.go`

**Interfaces:**
- Produces:
  - `rbac.PermMailSettingsManage = "mail.settings.manage"`
  - `rbac.AuditActionMailSettingsManage`、`rbac.AuditTargetMailSettings`
  - `mailapi.RegisterRoutes(api, svc, rbacSvc)`

- [ ] **Step 1: 写失败测试**

`mailapi/handler_test.go`：
```go
func TestAdminMailRequiresPermission(t *testing.T) {
    f := newMailFixture(t)
    // guest 401；normal user 403；mail.settings.manage 200。
}
func TestAdminMailUpdateAndAudit(t *testing.T) { ... }
func TestAdminMailTestSend(t *testing.T) { ... }
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/mailapi/...`
Expected: FAIL

- [ ] **Step 3: 实现权限与审计**

- `rbac/catalog.go` 增加 `PermMailSettingsManage`，分组「邮件」，描述“配置 SMTP 邮件服务与强制邮箱验证”。
- `rbac/audit.go` 增加 action/target 常量。
- 前端权限目录在 Task 14 同步。

- [ ] **Step 4: 实现 mailapi**

- `RegisterRoutes` 用 `RequireAuth` + `RequirePermission(mail.settings.manage)`。
- `GET/PUT /admin/mail`、`POST /admin/mail/test`。
- 保存/测试成功 best-effort 审计；失败只 warning。
- 测试邮件失败返回 50201，未就绪 50301。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/rbac/... ./internal/mailapi/...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/mailapi apps/server/internal/rbac
git commit -m "feat(server): add mail admin API and permission"
```

---

### Task 10: 配置、环境变量、OpenAPI 与前端类型

**Files:**
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Modify: `.env.example`
- Modify: `packages/api/openapi.yaml`
- Modify: `apps/web/src/api/schema.d.ts`（生成）
- Modify: `apps/web/src/api/types.ts`

**Interfaces:**
- Adds config:
  - `MailEncryptionKey string`
  - `MailKeyFile string`（默认 `data/mail.key`）
  - `MailTimeout time.Duration`（默认 20s）
  - `TrustedProxies []string`（`KAPOJ_TRUSTED_PROXIES`，逗号分隔）
- OpenAPI:
  - `RegisterResult`（`verificationRequired`、`email`、`verificationEmailSent`、可选 `session`）
  - 新 auth endpoints、`AdminMailView`、`MailSettingsInput`、`MailTestInput`
  - `Me.emailVerified`

- [ ] **Step 1: 写失败配置测试**

```go
func TestMailConfig(t *testing.T) {
    t.Setenv("KAPOJ_JWT_SECRET", strings.Repeat("a", 32))
    t.Setenv("KAPOJ_MAIL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
    t.Setenv("KAPOJ_MAIL_KEY_FILE", "/tmp/mail.key")
    t.Setenv("KAPOJ_MAIL_TIMEOUT", "10s")
    t.Setenv("KAPOJ_TRUSTED_PROXIES", "127.0.0.1/32,10.0.0.0/8")
    cfg, err := Load()
    require.NoError(t, err)
    assert.Equal(t, "/tmp/mail.key", cfg.MailKeyFile)
    assert.Equal(t, 10*time.Second, cfg.MailTimeout)
    assert.Equal(t, []string{"127.0.0.1/32", "10.0.0.0/8"}, cfg.TrustedProxies)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/config/... -run Mail`
Expected: FAIL

- [ ] **Step 3: 实现配置**

- `Load` 增加默认值与字段；`MailEncryptionKey` 非空时复用 base64/32 字节校验；`MailTimeout` 必须 >0；`TrustedProxies` 按逗号拆分、trim、逐个 `net.ParseCIDR` 或 `net.ParseIP` 校验，非法启动失败。
- `.env.example` 增加注释：`KAPOJ_MAIL_ENCRYPTION_KEY`、`KAPOJ_MAIL_KEY_FILE`、`KAPOJ_MAIL_TIMEOUT`、`KAPOJ_TRUSTED_PROXIES`；说明 QQ 授权码/腾讯云/阿里云 SMTP。

- [ ] **Step 4: 更新 OpenAPI**

- `/auth/register` 改为 `RegisterResultResponse`。
- 新增 `/auth/email/verify`、`/auth/email/resend`、`/auth/password/forgot`、`/auth/password/reset`。
- 新增 `/admin/mail` GET/PUT、`/admin/mail/test` POST。
- 新 schema：`RegisterResult`、`EmailVerificationInput`、`EmailRequest`、`ResetPasswordInput`、`MessageView`、`AdminMailView`、`MailSettingsInput`、`MailTestInput`、`PasswordState`。
- `Me` 增加 `emailVerified: boolean`。
- `session` 字段用 `nullable: true` + `allOf` 引用 `Session`。

- [ ] **Step 5: 生成类型并更新别名**

Run: `corepack pnpm gen:api`
Then add to `apps/web/src/api/types.ts`:
```ts
export type RegisterResult = Schemas['RegisterResult']
export type EmailVerificationInput = Schemas['EmailVerificationInput']
export type EmailRequest = Schemas['EmailRequest']
export type ResetPasswordInput = Schemas['ResetPasswordInput']
export type AdminMailView = Schemas['AdminMailView']
export type MailSettingsInput = Schemas['MailSettingsInput']
export type MailTestInput = Schemas['MailTestInput']
```
`Me` already includes `emailVerified` via generated schema.

- [ ] **Step 6: 运行检查**

Run:
```bash
cd apps/server && go test ./internal/config/...
corepack pnpm gen:api
```
Expected: PASS + 生成文件更新

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/config .env.example packages/api/openapi.yaml apps/web/src/api/schema.d.ts apps/web/src/api/types.ts
git commit -m "feat: add mail config and API contract"
```

---

### Task 11: Router/Main 装配与集成测试

**Files:**
- Modify: `apps/server/internal/router/router.go`
- Modify: `apps/server/internal/router/router_test.go`
- Modify: `apps/server/cmd/server/main.go`

**Interfaces:**
- Consumes:
  - `auth.WithMailer`、`auth.WithEmailTokens`
  - `middleware.RequireVerifiedEmail`
  - `mailapi.RegisterRoutes`
  - `mail.NewService`、`mail.WithTimeout`
  - `secret.NewKeyManager`
- Produces:
  - `router.Deps.Mail *mail.Service`
  - `main` 构造邮件服务并传入 `Deps.Mail`

- [ ] **Step 1: 写失败测试**

`router_test.go`：
- 注册响应改为 `data.session.accessToken`（无 Mail 时仍自动登录）。
- 传 `Deps.Mail` 时，第二个注册返回 `verificationRequired=true`。
- 未验证 token 访问 `/api/users/me` 返回 40303。
- 普通用户访问 `/api/admin/mail` 403，super 200。
- 默认 `TrustedProxies` 为空时，伪造 `X-Forwarded-For` 不影响 `ClientIP`（可用一个记录 IP 的测试路由或审计/session 列表验证）。

- [ ] **Step 2: 运行测试确认失败**

Run: `cd apps/server && go test ./internal/router/...`
Expected: FAIL

- [ ] **Step 3: 装配 router**

- `router.Deps` 增加 `Mail *mail.Service`。
- `auth.NewService` 传 `WithMailer(d.Mail)`、`WithEmailTokens(NewEmailTokenStore(d.Redis))`（仅 d.Mail 非 nil）。
- 认证路由组在 `Authenticate` 后加 `RequireVerifiedEmail(d.Mail, users)`（仅 d.Mail 非 nil），覆盖 `api`、`problemJSON`、`uploads`、`imports`。
- `mailapi.RegisterRoutes(api, d.Mail, d.RBAC)`（仅 d.Mail 非 nil）。
- `router.New` 调用 `r.SetTrustedProxies(d.Config.TrustedProxies)`；空切片表示不信任代理头。

- [ ] **Step 4: 装配 main**

- `main` 用 `secret.NewKeyManager(cfg.MailEncryptionKey, cfg.MailKeyFile)` 构造邮件密钥。
- 用 `mail.NewService(mail.NewGormRepository(g), mailKeys, mail.WithTimeout(cfg.MailTimeout))` 构造邮件服务并传入 `Deps.Mail`。
- 密钥配置非法时启动失败；未配置时服务仍可读取默认设置但 `Ready=false`。

- [ ] **Step 5: 运行测试**

Run: `cd apps/server && go test ./internal/router/...`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/router apps/server/cmd/server
git commit -m "feat(server): wire mail service and verification middleware"
```

---

### Task 12: 前端 API/Store/HTTP 与注册/登录改造

**Files:**
- Modify: `apps/web/src/api/auth.ts`
- Modify: `apps/web/src/stores/auth.ts`
- Modify: `apps/web/src/api/http.ts`
- Modify: `apps/web/src/api/http.spec.ts`
- Modify: `apps/web/src/api/client.ts`
- Modify: `apps/web/src/views/RegisterView.vue`
- Modify: `apps/web/src/views/LoginView.vue`
- Modify: `apps/web/src/views/RegisterView.spec.ts`
- Modify: `apps/web/src/views/LoginView.spec.ts`
- Create: `apps/web/src/stores/auth.spec.ts`（若无）

**Interfaces:**
- `authApi.verifyEmail(token)`, `resendVerification(email)`, `forgotPassword(email)`, `resetPassword(input)`
- `useAuthStore().register(input): Promise<RegisterResult>`
- `AuthHooks.onEmailNotVerified(): void`
- RegisterView 新状态：`verificationRequired`、`sentEmail`、`emailSent`、`resendCooldown`
- LoginView 40303 内联状态

- [ ] **Step 1: 写失败测试**

`RegisterView.spec.ts`：
```ts
it('shows verification panel without logging in', async () => {
  vi.mocked(authApi.register).mockResolvedValue({
    verificationRequired: true,
    email: 'alice@example.com',
    verificationEmailSent: true,
  })
  // 填表提交
  expect(wrapper.text()).toContain('验证邮件已发送')
  expect(auth.setSession).not.toHaveBeenCalled()
})

it('shows resend warning when send failed', async () => { ... })
it('auto logs in when verification is not required', async () => { ... })
```

`LoginView.spec.ts`：
```ts
it('shows unverified message and verification link on 40303', async () => {
  vi.mocked(authApi.login).mockRejectedValue(new ApiError(403, 40303, '邮箱尚未验证'))
  // submit
  expect(wrapper.text()).toContain('邮箱尚未验证')
  expect(wrapper.find('a[href*="/verify-email"]').exists()).toBe(true)
})
```

`http` tests：非 `/auth/*` 40303 调用 `onEmailNotVerified`；`/auth/login` 不调用。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- RegisterView.spec.ts LoginView.spec.ts http.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现 API 与 store**

- `authApi` 新函数按 OpenAPI 类型。
- `auth store.register`：
  ```ts
  async function register(input: RegisterRequest): Promise<RegisterResult> {
    const result = await authApi.register(input)
    if (result.session) {
      setSession(result.session)
      await fetchMe().catch(() => undefined)
    }
    return result
  }
  ```
- 不持久化任何邮箱 token。

- [ ] **Step 4: 实现 HTTP 40303 钩子**

- `AuthHooks` 增加 `onEmailNotVerified`。
- `http.ts` 在 `toApiError` 后判断 `err.code === 40303 && !isAuthEndpoint` → `hooks.onEmailNotVerified()`。
- refresh 收到 40303 时同样调用，不调用 `onAuthFailure` 清空。
- `client.ts` 跳转 `/verify-email`，若当前已在则跳过；可从 `useAuthStore().user?.email` 带 query。

- [ ] **Step 5: 改造注册/登录页**

- RegisterView：
  - 调 `auth.register` 后判断 `verificationRequired`。
  - 显示发送成功/失败面板、邮箱、重发按钮 + 60s 倒计时、返回登录。
  - 非强制时保持原 Toast + 首页跳转。
- LoginView：
  - 增加“忘记密码？”链接。
  - 捕获 `ApiError.code === 40303`，显示内联提示和带 email query 的 `/verify-email` 链接。

- [ ] **Step 6: 运行测试与类型检查**

Run:
```bash
corepack pnpm --filter web test -- RegisterView.spec.ts LoginView.spec.ts http.spec.ts
corepack pnpm --filter web build
```
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add apps/web/src/api/auth.ts apps/web/src/stores/auth.ts apps/web/src/api/http.ts apps/web/src/api/http.spec.ts apps/web/src/api/client.ts apps/web/src/views/RegisterView.vue apps/web/src/views/RegisterView.spec.ts apps/web/src/views/LoginView.vue apps/web/src/views/LoginView.spec.ts apps/web/src/stores/auth.spec.ts
git commit -m "feat(web): handle email verification in register and login"
```

---

### Task 13: 验证邮箱、忘记密码、重置密码页面与路由

**Files:**
- Create: `apps/web/src/views/VerifyEmailView.vue`
- Create: `apps/web/src/views/VerifyEmailView.spec.ts`
- Create: `apps/web/src/views/ForgotPasswordView.vue`
- Create: `apps/web/src/views/ForgotPasswordView.spec.ts`
- Create: `apps/web/src/views/ResetPasswordView.vue`
- Create: `apps/web/src/views/ResetPasswordView.spec.ts`
- Modify: `apps/web/src/router/index.ts`

**Interfaces:**
- Routes:
  - `/verify-email` name `verify-email`
  - `/forgot-password` name `forgot-password` meta `guestOnly`
  - `/reset-password` name `reset-password` meta `guestOnly`
- VerifyEmailView uses `authApi.verifyEmail`, `authApi.resendVerification`, `auth.setSession`.
- ResetPasswordView uses `authApi.resetPassword`.

- [ ] **Step 1: 写失败测试**

```ts
// VerifyEmailView.spec.ts
it('verifies token and logs in', async () => {
  route.query.token = 'tok'
  vi.mocked(authApi.verifyEmail).mockResolvedValue(session())
  mountView()
  await flushPromises()
  expect(authApi.verifyEmail).toHaveBeenCalledWith('tok')
  expect(auth.setSession).toHaveBeenCalled()
  expect(router.replace).toHaveBeenCalledWith({ name: 'home' })
})

it('shows expired state and resend form', async () => {
  vi.mocked(authApi.verifyEmail).mockRejectedValue(new ApiError(400, 40003, '链接无效或已过期'))
  ...
  expect(wrapper.text()).toContain('链接无效或已过期')
  expect(wrapper.find('input[type="email"]').exists()).toBe(true)
})
```

Forgot/Reset tests cover generic success, token missing, password mismatch, success redirect.

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- VerifyEmailView.spec.ts ForgotPasswordView.spec.ts ResetPasswordView.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现页面**

- 复用 `AuthLayout`、`AppFormField`、PrimeVue `Button/InputText/Password/Message`。
- VerifyEmailView：
  - onMounted：有 token 自动调用；成功 `auth.setSession` + Toast + 首页。
  - 失败/无 token：邮箱输入 + 重发；通用成功提示。
  - 从 `route.query.email` 预填。
- ForgotPasswordView：邮箱输入 → 提交 → 通用成功面板；60s 倒计时。
- ResetPasswordView：token 缺失/无效状态；新密码 + 确认；成功提示 + 去登录；调用 `authApi.resetPassword({token,newPassword})`。
- 路由注册；verify-email 不加 `guestOnly`，forgot/reset 加 `guestOnly`。

- [ ] **Step 4: 运行测试**

Run: `corepack pnpm --filter web test -- VerifyEmailView.spec.ts ForgotPasswordView.spec.ts ResetPasswordView.spec.ts`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/views/VerifyEmailView.vue apps/web/src/views/VerifyEmailView.spec.ts apps/web/src/views/ForgotPasswordView.vue apps/web/src/views/ForgotPasswordView.spec.ts apps/web/src/views/ResetPasswordView.vue apps/web/src/views/ResetPasswordView.spec.ts apps/web/src/router/index.ts
git commit -m "feat(web): add email verification and password recovery pages"
```

---

### Task 14: 后台邮件设置页与权限导航

**Files:**
- Create: `apps/web/src/api/mail.ts`
- Create: `apps/web/src/views/AdminMailView.vue`
- Create: `apps/web/src/views/AdminMailView.spec.ts`
- Modify: `apps/web/src/api/types.ts`
- Modify: `apps/web/src/permissions.ts`
- Modify: `apps/web/src/components/layout/AdminNav.vue`
- Modify: `apps/web/src/router/index.ts`
- Modify: `apps/web/src/permissions.spec.ts`

**Interfaces:**
- `getAdminMail()`, `updateAdminMail(input)`, `testAdminMail(to)`
- `PERM.mail.settings.manage`
- AdminNav 分组「系统」/「邮件设置」
- Route `/admin/mail` meta `permission: PERM.mail.settings.manage`

- [ ] **Step 1: 写失败测试**

`AdminMailView.spec.ts`：
```ts
it('loads and saves mail settings', async () => {
  vi.mocked(getAdminMail).mockResolvedValue(mailView({ ready: true }))
  // mount, fill fields, save
  expect(updateAdminMail).toHaveBeenCalled()
})

it('does not render or send password back', async () => {
  // mailView({ passwordState: 'ready' })
  expect(wrapper.find('input[type="password"]').element.value).toBe('')
  expect(wrapper.text()).toContain('已就绪')
})

it('blocks enabling enforcement when not ready', async () => {
  // ready=false, toggle require, save -> client validation message
})
```

`permissions.spec.ts` 增加 `PERM.mail.settings.manage`；AdminNav 入口仅在权限存在时显示。

- [ ] **Step 2: 运行测试确认失败**

Run: `corepack pnpm --filter web test -- AdminMailView.spec.ts permissions.spec.ts`
Expected: FAIL

- [ ] **Step 3: 实现 API 与权限**

- `api/mail.ts` 按 OpenAPI。
- `permissions.ts` 增加 `mail: { settings: { manage: 'mail.settings.manage' } }`，加入 `ADMIN_ACCESS_PERMISSIONS`。
- `AdminNav` 增加 `mailItem` 与「系统」分组，`auth.can(PERM.mail.settings.manage)` 控制。
- 路由 `/admin/mail`。

- [ ] **Step 4: 实现 AdminMailView**

- 加载视图、表单字段、provider 预设（QQ/腾讯云/阿里云/自定义）。
- 密码输入 placeholder“已配置，留空不修改”；显示 `passwordState`。
- `sendingEnabled`、`requireEmailVerification` 开关；未就绪时禁止打开强制验证并提示。
- 保存、测试发送（收件人 + 按钮，使用已保存配置）、Toast。
- `publicBaseUrl` 校验 http(s) origin。
- 不显示密码/密文。

- [ ] **Step 5: 运行测试与构建**

Run:
```bash
corepack pnpm --filter web test -- AdminMailView.spec.ts permissions.spec.ts
corepack pnpm --filter web build
```
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add apps/web/src/api/mail.ts apps/web/src/views/AdminMailView.vue apps/web/src/views/AdminMailView.spec.ts apps/web/src/api/types.ts apps/web/src/permissions.ts apps/web/src/permissions.spec.ts apps/web/src/components/layout/AdminNav.vue apps/web/src/router/index.ts
git commit -m "feat(web): add admin mail settings page"
```

---

### Task 15: 文档、全量检查与真实冒烟

**Files:**
- Modify: `README.md`
- Modify: `DEVELOPMENT.md`
- Modify: `运行手册.md`
- Modify: `docs/开发日志.md`
- Maybe modify: `.env.example`（Task 10 已加）

**Steps:**

- [ ] **Step 1: 文档更新**

- README：当前进度、注册/邮箱验证/密码找回、后台邮件设置、错误码。
- DEVELOPMENT：认证接口表、错误码表、users 字段、mail_settings、Redis key、迁移说明、环境变量。
- 运行手册：
  - 首次部署引导账号 + SQL 授予 super_admin。
  - QQ 授权码配置、腾讯云/阿里云 SMTP 配置、测试邮件。
  - 升级先停旧实例完成回填。
  - `KAPOJ_MAIL_ENCRYPTION_KEY`/`KAPOJ_MAIL_KEY_FILE` 备份。
  - `KAPOJ_TRUSTED_PROXIES` 反向代理配置。
  - 强制验证开关与 50301 行为。
- 开发日志：记录交付内容与冒烟结果。

- [ ] **Step 2: 后端全量测试**

Run: `cd apps/server && go test ./...`
Expected: PASS（数据库测试在未设置 `KAPOJ_TEST_DATABASE_URL` 时 skip；设置后必须通过）

- [ ] **Step 3: 前端测试、Lint、构建**

Run:
```bash
corepack pnpm test:web
corepack pnpm lint:web
corepack pnpm build:web
```
Expected: PASS

- [ ] **Step 4: OpenAPI 生成检查**

Run: `corepack pnpm gen:api`
Expected: `schema.d.ts` 无未提交差异（或提交生成结果）

- [ ] **Step 5: 真实冒烟**

启动临时 server、临时 PostgreSQL/Redis（复用本机 5433/6380 或 docker compose），用一个进程内假 SMTP 服务器或真实 QQ SMTP：
1. 首次注册引导账号 → 自动登录。
2. 配置邮件设置（假 SMTP 或 QQ）→ 测试邮件。
3. 第二个账号注册 → 收到验证链接 → 验证 → 自动登录 → `emailVerified=true`。
4. 未验证登录 → 40303；重发 → 新链接生效、旧链接 40003。
5. 忘记密码 → 重置 → 旧会话失效、旧密码失败、新密码成功。
6. 关闭/打开强制验证的行为。
7. 权限 401/403/200 与审计。
8. 清理临时库、Redis key、storage。

- [ ] **Step 6: 提交**

```bash
git add README.md DEVELOPMENT.md 运行手册.md docs/开发日志.md apps/web/src/api/schema.d.ts
git commit -m "docs: document email verification and mail settings"
```

---

## Self-Review Notes

- Spec coverage：后台配置、SMTP 预设、强制开关、存量回填、验证/重发、忘记/重置、中间件、权限审计、OpenAPI、前端页面、测试与冒烟均有任务。
- 类型一致性：`RegisterResult`、`AdminMailView`、`passwordState`、`EmailTokenStore` 方法名在各任务中保持一致。
- Review Focus 对应测试：SMTP 输入 → Task 3；未验证访问 → Task 6/7/8/11；token 生命周期 → Task 5/7；枚举与 IP → Task 5/7；密钥 fail-closed → Task 2/3/11。
