package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sonymuhamad/webhook-lab/config"
)

//go:embed migrations/*.sql
var Migrations embed.FS

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
	return pool, pool.Close, nil
}
