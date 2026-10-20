package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	// Все временные метки API — UTC (как в in-memory Store). Без этого pgx
	// отдаёт timestamptz в локальном поясе процесса: на сервере с
	// Europe/Moscow ответы получали смещение +03:00 вместо Z, и клиент/QA
	// видели разное представление одного и того же момента. ScanLocation
	// задаёт пояс при декодировании, timezone — единый пояс SQL-сессии.
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name: "timestamptz",
			OID:  pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{
				ScanLocation: time.UTC,
			},
		})
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
