package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// Message is one transactional email.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender sends one message. Implementations must be safe for sequential use.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// SenderFactory builds a sender from the current settings. Tests inject a fake.
type SenderFactory func(cfg Config) (Sender, error)

// Config is the resolved SMTP configuration for one send.
type Config struct {
	Provider  Provider
	Host      string
	Port      int
	TLS       TLSMode
	Username  string
	Password  string
	FromEmail string
	FromName  string
	ReplyTo   string
	Timeout   time.Duration

	// tlsConfig is only set by tests that run a self-signed STARTTLS server.
	tlsConfig *tls.Config
}

type smtpSender struct {
	cfg Config
}

// NewSMTPSender validates cfg and returns a standard-library SMTP sender.
func NewSMTPSender(cfg Config) (Sender, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" {
		return nil, fmt.Errorf("mail: smtp host is required")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("mail: smtp port must be between 1 and 65535")
	}
	if !validTLS(cfg.TLS) {
		return nil, fmt.Errorf("mail: unsupported tls mode %q", cfg.TLS)
	}
	if err := user.ValidateEmail(cfg.FromEmail); err != nil {
		return nil, fmt.Errorf("mail: invalid from address: %w", err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	return &smtpSender{cfg: cfg}, nil
}

func (s *smtpSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	recipient, err := parseRecipient(msg.To)
	if err != nil {
		return err
	}
	raw, err := buildMessage(s.cfg, msg)
	if err != nil {
		return err
	}

	conn, err := s.dial(ctx)
	if err != nil {
		return fmt.Errorf("mail: dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("mail: smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if s.cfg.TLS == TLSStartTLS {
		if err := client.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := client.Mail(s.cfg.FromEmail); err != nil {
		return fmt.Errorf("mail: mail from: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("mail: rcpt to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("mail: close data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mail: quit: %w", err)
	}
	return nil
}

func (s *smtpSender) dial(ctx context.Context) (net.Conn, error) {
	address := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: s.cfg.Timeout}

	var (
		conn net.Conn
		err  error
	)
	if s.cfg.TLS == TLSSSL {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, s.tlsConfig())
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(s.cfg.Timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *smtpSender) tlsConfig() *tls.Config {
	if s.cfg.tlsConfig != nil {
		return s.cfg.tlsConfig.Clone()
	}
	return &tls.Config{
		ServerName: s.cfg.Host,
		MinVersion: tls.VersionTLS12,
	}
}

func parseRecipient(raw string) (string, error) {
	address, err := netmail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("mail: invalid recipient: %w", err)
	}
	return address.Address, nil
}
