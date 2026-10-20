package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// startTestSMTP поднимает минимальный SMTP-сервер без TLS и возвращает адрес
// и канал с телом принятого письма.
func startTestSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan string, 1)
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) bool {
			if _, err := w.WriteString(s); err != nil {
				return false
			}
			return w.Flush() == nil
		}
		if !write("220 test-smtp ready\r\n") {
			return
		}
		inData := false
		var body strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					done <- body.String()
					if !write("250 2.0.0 queued\r\n") {
						return
					}
					continue
				}
				body.WriteString(line)
				continue
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				if !write("250-test-smtp\r\n250 HELP\r\n") {
					return
				}
			case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
				if !write("250 2.1.0 ok\r\n") {
					return
				}
			case cmd == "DATA":
				inData = true
				if !write("354 end with <CRLF>.<CRLF>\r\n") {
					return
				}
			case cmd == "QUIT":
				_ = write("221 2.0.0 bye\r\n")
				return
			default:
				if !write("250 2.0.0 ok\r\n") {
					return
				}
			}
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), done
}

func TestSMTPSendTextMessage(t *testing.T) {
	addr, received := startTestSMTP(t)
	s, err := NewSMTP(Config{
		Addr:    addr,
		From:    "Edu Platform <no-reply@example.test>",
		TLSMode: "none",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Send(ctx, Message{
		To:      "teacher@example.test",
		Subject: "Приглашение преподавателя",
		Text:    "Здравствуйте!\nСсылка: https://edu.example/register?inviteToken=abc",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case body := <-received:
		for _, want := range []string{
			"To: <teacher@example.test>",
			"Subject: =?UTF-8?",
			"https://edu.example/register?inviteToken=abc",
			"Content-Type: text/plain; charset=UTF-8",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("в письме нет %q:\n%s", want, body)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP-сервер не получил письмо")
	}
}

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	s, err := NewSMTP(Config{
		Addr:    "127.0.0.1:1",
		From:    "no-reply@example.test",
		TLSMode: "none",
	})
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	if err := s.Send(context.Background(), Message{
		To:      "teacher@example.test",
		Subject: "Тема\r\nBcc: evil@example.test",
	}); err == nil || !strings.Contains(err.Error(), "теме") {
		t.Fatalf("ожидали ошибку про тему, got %v", err)
	}
}

func TestNewSMTPDisabledWithoutConfig(t *testing.T) {
	s, err := NewSMTP(Config{})
	if err != nil || s != nil {
		t.Fatalf("пустая конфигурация: s=%v err=%v, ожидали nil/nil", s, err)
	}
}

func TestNewSMTPRejectsPartialConfig(t *testing.T) {
	if _, err := NewSMTP(Config{Addr: "smtp.example.test:587"}); err == nil {
		t.Fatal("ожидали ошибку: SMTP_ADDR без SMTP_FROM")
	}
	if _, err := NewSMTP(Config{From: "no-reply@example.test"}); err == nil {
		t.Fatal("ожидали ошибку: SMTP_FROM без SMTP_ADDR")
	}
}
