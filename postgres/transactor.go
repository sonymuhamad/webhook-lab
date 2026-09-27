package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

type txKey struct{}

type Transactor struct {
	pool *pgxpool.Pool
}

func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool}
}

// WithinTx runs fn in a transaction carried on the context it passes to fn;
// repositories called with that context join the transaction. fn's error
// rolls back and is returned as-is. A nested call joins the outer
// transaction instead of opening a second one.
func (t *Transactor) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful commit

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// queries returns sqlc queries bound to the transaction in ctx, or to the
// pool when there is none. Every repository method must go through it, or
// its statement silently runs outside the caller's transaction.
func queries(ctx context.Context, pool *pgxpool.Pool) *sqlcgen.Queries {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return sqlcgen.New(tx)
	}
	return sqlcgen.New(pool)
}
