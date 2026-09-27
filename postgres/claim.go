package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

// NaiveClaimer implements webhook.Claimer by reading due deliveries without
// taking or marking them. Concurrent callers receive the same deliveries.
type NaiveClaimer struct {
	pool *pgxpool.Pool
}

func NewNaiveClaimer(pool *pgxpool.Pool) *NaiveClaimer {
	return &NaiveClaimer{pool: pool}
}

func (c *NaiveClaimer) ClaimDue(ctx context.Context, limit int) ([]webhook.DueDelivery, error) {
	rows, err := queries(ctx, c.pool).ListDueDeliveries(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("select due deliveries: %w", err)
	}

	due := make([]webhook.DueDelivery, len(rows))
	for i, row := range rows {
		due[i] = toDueDelivery(row)
	}
	return due, nil
}

// SkipLockedClaimer implements webhook.Claimer with FOR UPDATE SKIP LOCKED
// and a lease: a claimed delivery is hidden from other callers until the
// lease runs out.
//
// If recording the outcome takes longer than the lease, another worker can
// claim and send the delivery again, and both workers then update it. That
// is the at-least-once trade-off; the lease must outlast a batch to keep it
// rare.
type SkipLockedClaimer struct {
	pool  *pgxpool.Pool
	lease time.Duration
}

func NewSkipLockedClaimer(pool *pgxpool.Pool, lease time.Duration) *SkipLockedClaimer {
	return &SkipLockedClaimer{pool: pool, lease: lease}
}

func (c *SkipLockedClaimer) ClaimDue(ctx context.Context, limit int) ([]webhook.DueDelivery, error) {
	rows, err := queries(ctx, c.pool).ClaimDueDeliveries(ctx, sqlcgen.ClaimDueDeliveriesParams{
		BatchSize:    int32(limit),
		LeaseSeconds: c.lease.Seconds(),
	})
	if err != nil {
		return nil, fmt.Errorf("claim due deliveries: %w", err)
	}

	due := make([]webhook.DueDelivery, len(rows))
	for i, row := range rows {
		// Both queries select the same columns; the conversion stops compiling
		// if they ever drift apart.
		due[i] = toDueDelivery(sqlcgen.ListDueDeliveriesRow(row))
	}
	return due, nil
}

func toDueDelivery(row sqlcgen.ListDueDeliveriesRow) webhook.DueDelivery {
	return webhook.DueDelivery{
		ID:               row.ID,
		MessageID:        row.MessageID,
		TenantID:         row.TenantID,
		EndpointURL:      row.EndpointURL,
		EventType:        row.EventType,
		Payload:          row.Payload,
		MessageCreatedAt: row.MessageCreatedAt,
		AttemptCount:     int(row.AttemptCount),
	}
}
