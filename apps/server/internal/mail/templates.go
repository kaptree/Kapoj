package mail

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"strings"

	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/user"
)

type templateData struct {
	Nickname string
	Link     string
	Expires  string
}

var verificationTemplate = template.Must(template.New("verification").Parse(`<!doctype html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1f2937">
  <div style="max-width:560px;margin:0 auto;padding:28px;background:#ffffff;border-radius:12px">
    <h2 style="margin:0 0 16px;font-size:20px">验证你的 kapOJ 邮箱</h2>
    <p style="margin:0 0 12px">你好，{{.Nickname}}：</p>
    <p style="margin:0 0 20px">请在 {{.Expires}}内点击下面的按钮完成邮箱验证。</p>
    <p style="margin:0 0 20px">
      <a href="{{.Link}}" style="display:inline-block;padding:10px 18px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:8px">验证邮箱</a>
    </p>
    <p style="margin:0 0 8px;color:#6b7280;font-size:13px">如果按钮无法点击，请复制以下链接到浏览器打开：</p>
    <p style="margin:0 0 20px;word-break:break-all;font-size:13px"><a href="{{.Link}}">{{.Link}}</a></p>
    <p style="margin:0;color:#6b7280;font-size:13px">如果不是你本人操作，请忽略这封邮件。</p>
  </div>
</body>
</html>`))

var passwordResetTemplate = template.Must(template.New("reset").Parse(`<!doctype html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1f2937">
  <div style="max-width:560px;margin:0 auto;padding:28px;background:#ffffff;border-radius:12px">
    <h2 style="margin:0 0 16px;font-size:20px">重置你的 kapOJ 密码</h2>
    <p style="margin:0 0 12px">你好，{{.Nickname}}：</p>
    <p style="margin:0 0 20px">请在 {{.Expires}}内点击下面的按钮设置新密码。</p>
    <p style="margin:0 0 20px">
      <a href="{{.Link}}" style="display:inline-block;padding:10px 18px;background:#2563eb;color:#ffffff;text-decoration:none;border-radius:8px">重置密码</a>
    </p>
    <p style="margin:0 0 8px;color:#6b7280;font-size:13px">如果按钮无法点击，请复制以下链接到浏览器打开：</p>
    <p style="margin:0 0 20px;word-break:break-all;font-size:13px"><a href="{{.Link}}">{{.Link}}</a></p>
    <p style="margin:0;color:#6b7280;font-size:13px">如果不是你本人操作，请忽略这封邮件；你的密码不会被修改。</p>
  </div>
</body>
</html>`))

var testTemplate = template.Must(template.New("test").Parse(`<!doctype html>
<html lang="zh-CN">
<body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1f2937">
  <div style="max-width:560px;margin:0 auto;padding:28px;background:#ffffff;border-radius:12px">
    <h2 style="margin:0 0 16px;font-size:20px">kapOJ 邮件配置测试</h2>
    <p style="margin:0">这是一封测试邮件。收到它说明当前 SMTP 配置可以正常发送邮件。</p>
  </div>
</body>
</html>`))

func renderVerification(nickname, link string) (string, string, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		nickname = "同学"
	}
	data := templateData{Nickname: nickname, Link: link, Expires: "24 小时"}
	html, err := executeTemplate(verificationTemplate, data)
	if err != nil {
		return "", "", err
	}
	text := fmt.Sprintf("你好，%s：\n\n请在 24 小时内打开以下链接验证 kapOJ 邮箱：\n%s\n\n如果不是你本人操作，请忽略这封邮件。\n", nickname, link)
	return text, html, nil
}

func renderPasswordReset(nickname, link string) (string, string, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		nickname = "同学"
	}
	data := templateData{Nickname: nickname, Link: link, Expires: "30 分钟"}
	html, err := executeTemplate(passwordResetTemplate, data)
	if err != nil {
		return "", "", err
	}
	text := fmt.Sprintf("你好，%s：\n\n请在 30 分钟内打开以下链接重置 kapOJ 密码：\n%s\n\n如果不是你本人操作，请忽略这封邮件。\n", nickname, link)
	return text, html, nil
}

func renderTest() (string, string, error) {
	html, err := executeTemplate(testTemplate, templateData{})
	if err != nil {
		return "", "", err
	}
	return "这是一封 kapOJ 邮件配置测试邮件。收到它说明当前 SMTP 配置可以正常发送邮件。\n", html, nil
}

func executeTemplate(tmpl *template.Template, data templateData) (string, error) {
	var builder strings.Builder
	if err := tmpl.Execute(&builder, data); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// SendVerification renders and sends the email verification message.
func (s *Service) SendVerification(ctx context.Context, to, nickname, token string) error {
	if err := user.ValidateEmail(to); err != nil {
		return err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	if !s.ready(settings) {
		return httpx.ErrMailNotConfigured
	}
	text, html, err := renderVerification(nickname, s.link(settings, "/verify-email", token))
	if err != nil {
		return err
	}
	return s.deliver(ctx, settings, Message{
		To:      to,
		Subject: "验证你的 kapOJ 邮箱",
		Text:    text,
		HTML:    html,
	})
}

// SendPasswordReset renders and sends the password reset message.
func (s *Service) SendPasswordReset(ctx context.Context, to, nickname, token string) error {
	if err := user.ValidateEmail(to); err != nil {
		return err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	if !s.ready(settings) {
		return httpx.ErrMailNotConfigured
	}
	text, html, err := renderPasswordReset(nickname, s.link(settings, "/reset-password", token))
	if err != nil {
		return err
	}
	return s.deliver(ctx, settings, Message{
		To:      to,
		Subject: "重置你的 kapOJ 密码",
		Text:    text,
		HTML:    html,
	})
}

// SendTest sends a fixed test message to the given address.
func (s *Service) SendTest(ctx context.Context, to string) error {
	if err := user.ValidateEmail(to); err != nil {
		return err
	}
	settings, err := s.repo.Get(ctx)
	if err != nil {
		return err
	}
	if !s.ready(settings) {
		return httpx.ErrMailNotConfigured
	}
	text, html, err := renderTest()
	if err != nil {
		return err
	}
	return s.deliver(ctx, settings, Message{
		To:      to,
		Subject: "kapOJ 邮件配置测试",
		Text:    text,
		HTML:    html,
	})
}

func (s *Service) link(settings *Settings, path, token string) string {
	return strings.TrimRight(settings.PublicBaseURL, "/") + path + "?token=" + url.QueryEscape(token)
}

func (s *Service) deliver(ctx context.Context, settings *Settings, msg Message) error {
	cfg := Config{
		Provider:  settings.Provider,
		Host:      settings.SMTPHost,
		Port:      settings.SMTPPort,
		TLS:       settings.SMTPTLS,
		Username:  settings.SMTPUsername,
		FromEmail: settings.FromEmail,
		FromName:  settings.FromName,
		ReplyTo:   settings.ReplyTo,
		Timeout:   s.timeout,
	}
	if settings.SMTPPasswordEncrypted != "" && s.keys != nil {
		password, err := s.keys.Decrypt(settings.SMTPPasswordEncrypted)
		if err != nil {
			return httpx.ErrMailNotConfigured
		}
		cfg.Password = password
	}
	sender, err := s.newSender(cfg)
	if err != nil {
		return httpx.ErrMailNotConfigured
	}
	if err := sender.Send(ctx, msg); err != nil {
		slog.Warn("mail: send failed", "provider", settings.Provider, "err", err)
		return httpx.ErrMailSendFailed
	}
	return nil
}
