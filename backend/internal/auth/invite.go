// Package auth — сессии, пароль и приглашения учителей.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// InviteTTL — время жизни приглашения учителя (7 дней).
const InviteTTL = 7 * 24 * time.Hour

// GenerateToken генерирует одноразовый токен приглашения.
// Возвращает открытый токен (показать один раз), его sha256-хеш (хранить)
// и короткий префикс для отображения в админке.
func GenerateToken() (token, tokenHash, prefix string, err error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", "", errors.New("не удалось сгенерировать токен")
	}
	token = base64.RawURLEncoding.EncodeToString(raw[:])
	tokenHash = HashToken(token)
	if len(token) > 8 {
		prefix = token[:8]
	} else {
		prefix = token
	}
	return token, tokenHash, prefix, nil
}

// HashToken считает sha256-хеш токена для хранения вместо открытого значения.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
