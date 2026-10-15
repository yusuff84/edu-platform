package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPool создаёт pgx-пул для SQL-репозитория (T13: устаревший TODO
// про «заменить in-memory Store на SQL-репозитории и golang-migrate» удалён —
// SQL-репозиторий уже реализован в postgres_store.go, а миграции применяет
// собственный раннер internal/migrate при старте (см. cmd/api/main.go)).
func ConnectPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}
