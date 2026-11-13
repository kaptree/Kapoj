package mail

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSMTPServer struct {
	addr        string
	requireAuth bool
	data        string
	done        chan struct{}
	serveErr    error
}

func startFakeSMTP(t *testing.T, requireAuth bool) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &fakeSMTPServer{addr: ln.Addr().String(), requireAuth: requireAuth, done: make(chan struct{})}

	go func() {
		defer close(server.done)
		conn, err := ln.Accept()
		if err != nil {
			server.serveErr = err
			return
		}
		defer func() { _ = conn.Close() }()
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		write := func(line string) {
			_, _ = rw.WriteString(line + "\r\n")
			_ = rw.Flush()
		}
		write("220 fake ESMTP")

		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				server.serveErr = err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"):
				_, _ = rw.WriteString("250-fake\r\n")
				if requireAuth {
					_, _ = rw.WriteString("250-AUTH PLAIN\r\n")
				}
				_, _ = rw.WriteString("250 OK\r\n")
				_ = rw.Flush()
			case strings.HasPrefix(upper, "HELO"):
				write("250 fake")
			case strings.HasPrefix(upper, "AUTH PLAIN"):
				write("235 2.7.0 Authentication successful")
			case strings.HasPrefix(upper, "MAIL FROM"):
				write("250 OK")
			case strings.HasPrefix(upper, "RCPT TO"):
				write("250 OK")
			case upper == "DATA":
				write("354 End data with <CR><LF>.<CR><LF>")
				var body strings.Builder
				for {
					dataLine, err := rw.ReadString('\n')
					if err != nil {
						server.serveErr = err
						return
					}
					if dataLine == ".\r\n" || dataLine == ".\n" {
						break
					}
					body.WriteString(dataLine)
				}
				server.data = body.String()
				write("250 OK")
			case upper == "QUIT":
				write("221 Bye")
				return
			default:
				write("250 OK")
			}
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case <-server.done:
		case <-time.After(2 * time.Second):
			t.Log("fake SMTP server did not stop")
		}
	})
	return server
}

func (s *fakeSMTPServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(s.addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return host, port
}

func (s *fakeSMTPServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("fake SMTP server did not finish")
	}
	if s.serveErr != nil {
		t.Fatalf("fake SMTP server error: %v", s.serveErr)
	}
}

func TestSMTPSenderSendsMessage(t *testing.T) {
	server := startFakeSMTP(t, true)
	host, port := server.hostPort(t)

	sender, err := NewSMTPSender(Config{
		Provider:  ProviderCustom,
		Host:      host,
		Port:      port,
		TLS:       TLSNone,
		Username:  "bot",
		Password:  "secret",
		FromEmail: "noreply@example.com",
		FromName:  "kapOJ",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)
	err = sender.Send(context.Background(), Message{
		To:      "alice@example.com",
		Subject: "验证你的邮箱",
		Text:    "hello text",
		HTML:    "<b>hello html</b>",
	})
	require.NoError(t, err)
	server.wait(t)

	assert.Contains(t, server.data, "Subject:")
	assert.Contains(t, server.data, "hello text")
	assert.Contains(t, server.data, "<b>hello html</b>")
	assert.Contains(t, server.data, "alice@example.com")
}

func TestSMTPSenderAllowsAnonymousCustomSMTP(t *testing.T) {
	server := startFakeSMTP(t, false)
	host, port := server.hostPort(t)

	sender, err := NewSMTPSender(Config{
		Provider:  ProviderCustom,
		Host:      host,
		Port:      port,
		TLS:       TLSNone,
		FromEmail: "noreply@example.com",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, sender.Send(context.Background(), Message{To: "alice@example.com", Subject: "x", Text: "x"}))
	server.wait(t)
}

func TestSMTPSenderStartTLSRequiresServerSupport(t *testing.T) {
	server := startFakeSMTP(t, false)
	host, port := server.hostPort(t)

	sender, err := NewSMTPSender(Config{
		Provider:  ProviderCustom,
		Host:      host,
		Port:      port,
		TLS:       TLSStartTLS,
		FromEmail: "noreply@example.com",
		Timeout:   5 * time.Second,
	})
	require.NoError(t, err)
	err = sender.Send(context.Background(), Message{To: "alice@example.com", Subject: "x", Text: "x"})
	assert.Error(t, err)
}

func TestNewSMTPSenderValidatesConfig(t *testing.T) {
	_, err := NewSMTPSender(Config{Host: "127.0.0.1", Port: 25, TLS: TLSNone})
	assert.Error(t, err, "missing from email must be rejected")

	_, err = NewSMTPSender(Config{Host: "", Port: 25, TLS: TLSNone, FromEmail: "noreply@example.com"})
	assert.Error(t, err, "missing host must be rejected")

	_, err = NewSMTPSender(Config{Host: "127.0.0.1", Port: 0, TLS: TLSNone, FromEmail: "noreply@example.com"})
	assert.Error(t, err, "invalid port must be rejected")

	_, err = NewSMTPSender(Config{Host: "127.0.0.1", Port: 25, TLS: "bogus", FromEmail: "noreply@example.com"})
	assert.Error(t, err, "invalid tls mode must be rejected")
}

func TestSMTPSenderTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portText)

	sender, err := NewSMTPSender(Config{
		Provider:  ProviderCustom,
		Host:      host,
		Port:      port,
		TLS:       TLSNone,
		FromEmail: "noreply@example.com",
		Timeout:   50 * time.Millisecond,
	})
	require.NoError(t, err)
	// The listener accepts but never speaks, so the SMTP greeting times out.
	start := time.Now()
	err = sender.Send(context.Background(), Message{To: "alice@example.com", Subject: "x", Text: "x"})
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second, fmt.Sprintf("send must respect timeout, took %s", time.Since(start)))
}
