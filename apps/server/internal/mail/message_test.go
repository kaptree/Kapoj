package mail

import (
	"bytes"
	"io"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMessageEncodesChineseSubjectAndMultipart(t *testing.T) {
	raw, err := buildMessage(Config{
		FromEmail: "noreply@example.com",
		FromName:  "kapOJ",
	}, Message{
		To:      "alice@example.com",
		Subject: "验证你的 kapOJ 邮箱",
		Text:    "hello text",
		HTML:    "<b>hello html</b>",
	})
	require.NoError(t, err)

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	assert.Contains(t, msg.Header.Get("Subject"), "=?UTF-8?")
	assert.Contains(t, msg.Header.Get("Content-Type"), "multipart/alternative")
	from, err := mail.ParseAddress(msg.Header.Get("From"))
	require.NoError(t, err)
	assert.Equal(t, "kapOJ", from.Name)
	assert.Equal(t, "noreply@example.com", from.Address)

	body, err := io.ReadAll(msg.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "hello text")
	assert.Contains(t, string(body), "<b>hello html</b>")
}

func TestBuildMessageStripsHeaderInjection(t *testing.T) {
	raw, err := buildMessage(Config{FromEmail: "noreply@example.com"},
		Message{To: "alice@example.com", Subject: "hi\r\nBcc: evil@example.com", Text: "x"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "\r\nBcc: evil@example.com", "CRLF must not create a new header line")
	assert.Contains(t, strings.ToLower(string(raw)), "subject:")
}

func TestBuildMessageRejectsInvalidRecipient(t *testing.T) {
	_, err := buildMessage(Config{FromEmail: "noreply@example.com"}, Message{To: "not-an-email", Subject: "x"})
	assert.Error(t, err)
}
