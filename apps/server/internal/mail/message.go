package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/kaptree/kapoj/apps/server/internal/user"
)

// buildMessage renders a complete RFC 5322 MIME message with a plain-text and
// an HTML alternative. Header inputs are stripped of CR/LF to prevent header
// injection.
func buildMessage(cfg Config, msg Message) ([]byte, error) {
	if err := user.ValidateEmail(cfg.FromEmail); err != nil {
		return nil, fmt.Errorf("mail: invalid from address: %w", err)
	}
	to, err := netmail.ParseAddress(strings.TrimSpace(msg.To))
	if err != nil {
		return nil, fmt.Errorf("mail: invalid recipient: %w", err)
	}
	from := netmail.Address{Name: sanitizeHeader(cfg.FromName), Address: sanitizeHeader(cfg.FromEmail)}
	subject := sanitizeHeader(msg.Subject)
	if subject == "" {
		subject = "kapOJ 通知"
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from.String())
	fmt.Fprintf(&buf, "To: %s\r\n", to.String())
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Message-ID: <%s@%s>\r\n", randomMessageID(), domainOf(cfg.FromEmail))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	if cfg.ReplyTo != "" {
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", sanitizeHeader(cfg.ReplyTo))
	}

	writer := multipart.NewWriter(&buf)
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", writer.Boundary())

	text := msg.Text
	if strings.TrimSpace(text) == "" {
		text = "请使用支持 HTML 的邮件客户端查看这封邮件。"
	}
	htmlBody := msg.HTML
	if strings.TrimSpace(htmlBody) == "" {
		htmlBody = "<pre>" + html.EscapeString(text) + "</pre>"
	}

	if err := writePart(writer, "text/plain; charset=UTF-8", text); err != nil {
		return nil, err
	}
	if err := writePart(writer, "text/html; charset=UTF-8", htmlBody); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writePart(writer *multipart.Writer, contentType, body string) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	quoted := quotedprintable.NewWriter(part)
	if _, err := quoted.Write([]byte(body)); err != nil {
		_ = quoted.Close()
		return err
	}
	return quoted.Close()
}

func sanitizeHeader(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func randomMessageID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func domainOf(address string) string {
	if _, domain, ok := strings.Cut(address, "@"); ok {
		return sanitizeHeader(domain)
	}
	return "localhost"
}
