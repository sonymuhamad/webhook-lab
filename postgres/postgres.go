package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sonymuhamad/webhook-lab/config"
)

// NewPool connects to Postgres and verifies the connection before returning,
// so a wrong URL fails at startup instead of on the first request.
func NewPool(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, func(), error) {
	pool, err := pgxpool.New(ctx, cfg.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}

	metrics, err := registerPoolMetrics(pool)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("register pool metrics: %w", err)
	}

	cleanup := func() {
		metrics.Unregister() //nolint:errcheck // only fails for an already removed callback
		pool.Close()
	}
	return pool, cleanup, nil
}
