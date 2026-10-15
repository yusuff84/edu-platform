package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultOAuthTimeout — таймаут одного запроса к OAuth-провайдеру.
const DefaultOAuthTimeout = 8 * time.Second

// maxOAuthBody — ограничение на размер ответа провайдера (защита от мусора).
const maxOAuthBody = 1 << 20 // 1 МБ

// OAuthError — ошибка внешнего OAuth-провайдера с понятным русским текстом.
// Наружу не выходят внутренности: HTTP-статус провайдера и обобщённая причина,
// без сырых тел ответов и сетевых ошибок.
type OAuthError struct {
	Op         string // операция: «обмен кода на токен», «получение профиля» и т.п.
	Msg        string // человекочитаемая причина по-русски
	StatusCode int    // HTTP-статус провайдера, 0 если не применимо
}

func (e *OAuthError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("OAuth %s: %s (HTTP %d)", e.Op, e.Msg, e.StatusCode)
	}
	return fmt.Sprintf("OAuth %s: %s", e.Op, e.Msg)
}

// oauthErrorf — конструктор OAuthError.
func oauthErrorf(op string, status int, format string, args ...any) *OAuthError {
	return &OAuthError{Op: op, StatusCode: status, Msg: fmt.Sprintf(format, args...)}
}

// readLimited читает тело HTTP-ответа с ограничением maxOAuthBody.
func readLimited(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxOAuthBody))
}

// NewState генерирует криптостойкий state для OAuth-редиректа.
// T13 хранит его в короткоживущем cookie и сверяет при callback
// (защита от подмены code в query).
func NewState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("не удалось сгенерировать OAuth state")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// httpClient возвращает переданный клиент или дефолтный с таймаутом.
func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: DefaultOAuthTimeout}
}
