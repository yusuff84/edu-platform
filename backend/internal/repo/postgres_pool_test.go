package repo

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestConnectPoolScansTimestamptzUTC — API отдаёт все time.Time в UTC
// независимо от часового пояса процесса/сервера Postgres. Регрессия:
// qa-p7 A11 на Postgres с Europe/Moscow получал +03:00 вместо Z.
// Без TEST_DATABASE_URL тест пропускается, как остальные PG-тесты.
func TestConnectPoolScansTimestamptzUTC(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан: интеграционный тест Postgres пропущен")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := ConnectPool(ctx, dsn)
	if err != nil {
		t.Fatalf("ConnectPool: %v", err)
	}
	defer pool.Close()

	var got time.Time
	if err := pool.QueryRow(ctx, `SELECT '2026-11-02T10:30:00Z'::timestamptz`).Scan(&got); err != nil {
		t.Fatalf("scan timestamptz: %v", err)
	}
	if got.Location() != time.UTC {
		t.Fatalf("timestamptz пришёл в %q, ожидали UTC", got.Location())
	}
	if formatted := got.Format(time.RFC3339); formatted != "2026-11-02T10:30:00Z" {
		t.Fatalf("timestamptz=%s, ожидали 2026-11-02T10:30:00Z", formatted)
	}
}
