// Package mailer отправляет транзакционные письма через SMTP.
//
// Пакет не зависит от HTTP-слоя и конфигурируется только при наличии
// SMTP_ADDR/SMTP_FROM. Пустая конфигурация означает «письма выключены»:
// приглашение всё равно создаётся, а админ получает ссылку в API/UI.
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Message — простое текстовое письмо.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Sender отправляет письмо. Реализация должна быть безопасна для вызова из
// нескольких HTTP-запросов одновременно (SMTP-клиент создаётся на вызов).
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Config — настройки SMTP.
type Config struct {
	Addr     string // host:port, например smtp.example.com:587
	From     string // адрес отправителя (можно с display name)
	Username string
	Password string
	// TLSMode: starttls (по умолчанию), implicit (порт 465) или none
	// (локальный MailHog/тестовый SMTP без TLS).
	TLSMode string
	Timeout time.Duration
}

// SMTP — SMTP-реализация Sender.
type SMTP struct {
	cfg Config
}

// NewSMTP создаёт отправитель. Если Addr или From пусты, возвращает (nil, nil):
// email-канал считается отключённым, а не ошибочным.
func NewSMTP(cfg Config) (*SMTP, error) {
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.From = strings.TrimSpace(cfg.From)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if cfg.Addr == "" && cfg.From == "" {
		return nil, nil
	}
	if cfg.Addr == "" || cfg.From == "" {
		return nil, errors.New("mailer: для SMTP нужны оба параметра — SMTP_ADDR и SMTP_FROM")
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	switch cfg.TLSMode {
	case "starttls", "implicit", "none":
	default:
		return nil, fmt.Errorf("mailer: неизвестный TLS_MODE %q (допустимы starttls, implicit, none)", cfg.TLSMode)
	}
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return nil, fmt.Errorf("mailer: некорректный SMTP_ADDR %q: %w", cfg.Addr, err)
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("mailer: некорректный SMTP_FROM %q: %w", cfg.From, err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &SMTP{cfg: cfg}, nil
}

// Send отправляет текстовое письмо. Заголовки защищены от CRLF-инъекций,
// соединение ограничено таймаутом, а отмена ctx прерывает установку
// соединения (SMTP-команды stdlib не принимают context).
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	if s == nil {
		return errors.New("mailer: отправитель не настроен")
	}
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("mailer: некорректный отправитель: %w", err)
	}
	to, err := mail.ParseAddress(strings.TrimSpace(msg.To))
	if err != nil {
		return fmt.Errorf("mailer: некорректный получатель: %w", err)
	}
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return errors.New("mailer: перевод строки в теме письма")
	}
	if strings.ContainsAny(msg.Text, "\x00") {
		return errors.New("mailer: NUL-байт в теле письма")
	}

	host, _, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("mailer: некорректный SMTP_ADDR: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return errors.New("mailer: пустой host в SMTP_ADDR")
	}

	conn, err := (&net.Dialer{Timeout: s.cfg.Timeout}).DialContext(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("mailer: подключение к SMTP: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(s.cfg.Timeout))
	}

	if s.cfg.TLSMode == "implicit" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("mailer: TLS-рукопожатие: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("mailer: SMTP-клиент: %w", err)
	}
	defer client.Close()

	if s.cfg.TLSMode == "starttls" {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return errors.New("mailer: SMTP-сервер не поддерживает STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("mailer: STARTTLS: %w", err)
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mailer: SMTP-аутентификация: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("mailer: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mailer: RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(from.String(), to.String(), msg.Subject, msg.Text)); err != nil {
		_ = w.Close()
		return fmt.Errorf("mailer: запись письма: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: завершение письма: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("mailer: QUIT: %w", err)
	}
	return nil
}

// buildMessage собирает MIME-письмо с UTF-8-темой и текстовым телом.
func buildMessage(from, to, subject, text string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\nTo: ")
	b.WriteString(to)
	b.WriteString("\r\nSubject: ")
	b.WriteString(mime.QEncoding.Encode("UTF-8", subject))
	b.WriteString("\r\nDate: ")
	b.WriteString(time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("\r\nMIME-Version: 1.0")
	b.WriteString("\r\nContent-Type: text/plain; charset=UTF-8")
	b.WriteString("\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(normalizeCRLF(text))
	b.WriteString("\r\n")
	return []byte(b.String())
}

func normalizeCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
