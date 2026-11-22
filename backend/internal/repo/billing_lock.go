package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AcquireBillingLock takes the billing lock shared by all purchase, refund and
// webhook operations. PostgreSQL advisory locks coordinate replicas as well as
// concurrent requests. A session connection is retained until unlock; failed
// unlocks discard the connection.
func (s *Store) AcquireBillingLock(ctx context.Context) (func(), error) {
	select {
	case s.billingToken <- struct{}{}:
		return func() { <-s.billingToken }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *PostgresStore) AcquireBillingLock(ctx context.Context) (func(), error) {
	// A separate connection prevents lock waiters from exhausting the pool
	// needed by the request holding the advisory lock.
	conn, err := pgx.ConnectConfig(ctx, s.pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, err
	}
	const key int64 = 753251624772
	for {
		var acquired bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired); err != nil {
			_ = conn.Close(context.Background())
			return nil, err
		}
		if acquired {
			return func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var unlocked bool
				if err := conn.QueryRow(cleanup, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil || !unlocked {
					_ = conn.Close(cleanup)
					return
				}
				_ = conn.Close(cleanup)
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = conn.Close(context.Background())
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
