package mail

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/secret"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// Service reads and updates mail settings and derives readiness.
type Service struct {
	repo Repository
	keys *secret.KeyManager
	now  func() time.Time
}

// Option customizes a mail Service. Task 3 adds sender-related options.
type Option func(*Service)

// NewService constructs a mail settings service.
func NewService(repo Repository, keys *secret.KeyManager, opts ...Option) *Service {
	svc := &Service{repo: repo, keys: keys, now: time.Now}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Get returns the admin view of the current settings.
func (s *Service) Get(ctx context.Context) (*AdminView, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	return s.view(settings), nil
}

// Update validates and stores settings, encrypting a new SMTP password when
// one is supplied.
func (s *Service) Update(ctx context.Context, actorID int64, in SettingsInput) (*AdminView, error) {
	current, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	next := *current
	next.Provider = in.Provider
	next.SMTPHost = strings.TrimSpace(in.SMTPHost)
	next.SMTPPort = in.SMTPPort
	next.SMTPTLS = in.SMTPTLS
	next.SMTPUsername = strings.TrimSpace(in.SMTPUsername)
	next.FromEmail = strings.TrimSpace(in.FromEmail)
	next.FromName = strings.TrimSpace(in.FromName)
	next.ReplyTo = strings.TrimSpace(in.ReplyTo)
	next.PublicBaseURL = strings.TrimRight(strings.TrimSpace(in.PublicBaseURL), "/")
	next.SendingEnabled = in.SendingEnabled
	next.RequireEmailVerification = in.RequireEmailVerification
	next.UpdatedBy = actorID
	next.UpdatedAt = s.now().UTC()

	if in.SMTPPassword != nil {
		if *in.SMTPPassword == "" {
			next.SMTPPasswordEncrypted = ""
		} else {
			if s.keys == nil {
				return nil, httpx.ErrInternal
			}
			if err := s.keys.EnsureKey(); err != nil {
				return nil, err
			}
			ciphertext, err := s.keys.Encrypt(*in.SMTPPassword)
			if err != nil {
				return nil, err
			}
			next.SMTPPasswordEncrypted = ciphertext
		}
	}

	if err := validateInput(next); err != nil {
		return nil, err
	}
	if next.SendingEnabled || next.RequireEmailVerification {
		if !s.ready(&next) {
			return nil, httpx.ErrValidation("邮件服务未就绪")
		}
	}
	if err := s.repo.Update(ctx, &next); err != nil {
		return nil, err
	}
	return s.view(&next), nil
}

// Ready reports whether the saved settings can send mail.
func (s *Service) Ready(ctx context.Context) (bool, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return false, err
	}
	return s.ready(settings), nil
}

// VerificationRequired reports the global email verification switch.
func (s *Service) VerificationRequired(ctx context.Context) (bool, error) {
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return false, err
	}
	return settings.RequireEmailVerification, nil
}

func (s *Service) view(settings *Settings) *AdminView {
	state := s.passwordState(settings)
	return &AdminView{
		Provider:                 settings.Provider,
		SMTPHost:                 settings.SMTPHost,
		SMTPPort:                 settings.SMTPPort,
		SMTPTLS:                  settings.SMTPTLS,
		SMTPUsername:             settings.SMTPUsername,
		PasswordState:            state,
		FromEmail:                settings.FromEmail,
		FromName:                 settings.FromName,
		ReplyTo:                  settings.ReplyTo,
		PublicBaseURL:            settings.PublicBaseURL,
		SendingEnabled:           settings.SendingEnabled,
		RequireEmailVerification: settings.RequireEmailVerification,
		Ready:                    s.ready(settings),
		UpdatedBy:                settings.UpdatedBy,
		UpdatedAt:                settings.UpdatedAt.UTC(),
	}
}

func (s *Service) passwordState(settings *Settings) PasswordState {
	if settings.SMTPPasswordEncrypted == "" {
		return PasswordNone
	}
	if s.keys == nil || !s.keys.Configured() {
		return PasswordKeyError
	}
	if _, err := s.keys.Decrypt(settings.SMTPPasswordEncrypted); err != nil {
		return PasswordKeyError
	}
	return PasswordReady
}

func (s *Service) ready(settings *Settings) bool {
	if !settings.SendingEnabled {
		return false
	}
	if !validProvider(settings.Provider) || !validTLS(settings.SMTPTLS) {
		return false
	}
	if strings.TrimSpace(settings.SMTPHost) == "" || settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
		return false
	}
	if user.ValidateEmail(settings.FromEmail) != nil {
		return false
	}
	if !validBaseURL(settings.PublicBaseURL) {
		return false
	}
	state := s.passwordState(settings)
	if settings.Provider == ProviderCustom {
		return (settings.SMTPUsername == "" && state == PasswordNone) ||
			(settings.SMTPUsername != "" && state == PasswordReady)
	}
	return settings.SMTPUsername != "" && state == PasswordReady
}

func validateInput(settings Settings) error {
	if !validProvider(settings.Provider) {
		return httpx.ErrValidation("不支持的邮件服务商")
	}
	if !validTLS(settings.SMTPTLS) {
		return httpx.ErrValidation("不支持的 TLS 模式")
	}
	if settings.SMTPTLS == TLSNone && settings.Provider != ProviderCustom {
		return httpx.ErrValidation("仅自定义 SMTP 允许不使用 TLS")
	}
	if strings.TrimSpace(settings.SMTPHost) == "" || len(settings.SMTPHost) > 255 {
		return httpx.ErrValidation("SMTP 主机不能为空且不能超过 255 个字符")
	}
	if strings.ContainsAny(settings.SMTPHost+settings.SMTPUsername+settings.FromEmail+settings.FromName+settings.ReplyTo, "\r\n") {
		return httpx.ErrValidation("邮件配置不能包含换行符")
	}
	if settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
		return httpx.ErrValidation("SMTP 端口需在 1–65535 之间")
	}
	if len(settings.SMTPUsername) > 255 {
		return httpx.ErrValidation("SMTP 用户名不能超过 255 个字符")
	}
	if err := user.ValidateEmail(settings.FromEmail); err != nil {
		return err
	}
	if settings.ReplyTo != "" {
		if err := user.ValidateEmail(settings.ReplyTo); err != nil {
			return err
		}
	}
	if len([]rune(settings.FromName)) > 100 {
		return httpx.ErrValidation("发件人名称不能超过 100 个字符")
	}
	if settings.PublicBaseURL != "" && !validBaseURL(settings.PublicBaseURL) {
		return httpx.ErrValidation("站点公开地址需为 http(s) 根地址，不能包含路径、查询或片段")
	}
	if (settings.SendingEnabled || settings.RequireEmailVerification) && settings.PublicBaseURL == "" {
		return httpx.ErrValidation("启用邮件或强制验证前必须填写站点公开地址")
	}
	if settings.Provider != ProviderCustom && settings.SMTPUsername == "" {
		return httpx.ErrValidation("当前邮件服务商要求填写 SMTP 用户名")
	}
	return nil
}

func validProvider(provider Provider) bool {
	switch provider {
	case ProviderQQ, ProviderTencentSES, ProviderAliyunDM, ProviderCustom:
		return true
	default:
		return false
	}
}

func validTLS(mode TLSMode) bool {
	switch mode {
	case TLSSSL, TLSStartTLS, TLSNone:
		return true
	default:
		return false
	}
}

func validBaseURL(raw string) bool {
	if raw == "" || len(raw) > 500 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Path == "" || parsed.Path == "/"
}
