package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectPool — точка входа для будущей pgx-интеграции.
//
// TODO (инфра/БД): заменить in-memory Store на SQL-репозитории поверх этого пула,
// добавить миграции golang-migrate. Сигнатуры методов Store повторяют границы
// будущих репозиториев, поэтому handler/service слои менять не придётся.
func ConnectPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MaxConnIdleTime = 5 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}
