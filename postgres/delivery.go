package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

type DeliveryRepository struct {
	pool *pgxpool.Pool
}

func NewDeliveryRepository(pool *pgxpool.Pool) *DeliveryRepository {
	return &DeliveryRepository{pool: pool}
}

// CreateForActiveEndpoints fans the message out to every endpoint the tenant
// has enabled at this moment and returns how many deliveries it created.
func (r *DeliveryRepository) CreateForActiveEndpoints(ctx context.Context, message webhook.Message) (int64, error) {
	n, err := queries(ctx, r.pool).CreateDeliveriesForMessage(ctx, sqlcgen.CreateDeliveriesForMessageParams{
		MessageID: message.ID,
		TenantID:  message.TenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("insert deliveries: %w", err)
	}
	return n, nil
}

func (r *DeliveryRepository) CountPending(ctx context.Context) (webhook.PendingCount, error) {
	row, err := queries(ctx, r.pool).CountPendingDeliveries(ctx)
	if err != nil {
		return webhook.PendingCount{}, fmt.Errorf("count pending deliveries: %w", err)
	}
	return webhook.PendingCount{Due: row.Due, Scheduled: row.Scheduled}, nil
}

func (r *DeliveryRepository) ListByMessage(ctx context.Context, messageID uuid.UUID) ([]webhook.Delivery, error) {
	rows, err := queries(ctx, r.pool).ListDeliveriesByMessage(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("select deliveries: %w", err)
	}

	deliveries := make([]webhook.Delivery, len(rows))
	for i, row := range rows {
		deliveries[i] = webhook.Delivery{
			ID:            row.ID,
			MessageID:     row.MessageID,
			EndpointID:    row.EndpointID,
			EndpointURL:   row.EndpointURL,
			Status:        row.Status,
			AttemptCount:  int(row.AttemptCount),
			NextAttemptAt: row.NextAttemptAt,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		}
	}
	return deliveries, nil
}

func (r *DeliveryRepository) Update(ctx context.Context, param webhook.UpdateDeliveryParam) error {
	err := queries(ctx, r.pool).UpdateDeliveryAfterAttempt(ctx, sqlcgen.UpdateDeliveryAfterAttemptParams{
		ID:            param.ID,
		Status:        param.Status,
		NextAttemptAt: param.NextAttemptAt,
	})
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}
	return nil
}

func (r *DeliveryRepository) CreateAttempt(ctx context.Context, param webhook.CreateAttemptParam) error {
	var statusCode *int32
	if param.StatusCode != nil {
		code := int32(*param.StatusCode)
		statusCode = &code
	}

	err := queries(ctx, r.pool).CreateAttempt(ctx, sqlcgen.CreateAttemptParams{
		DeliveryID: param.DeliveryID,
		TenantID:   param.TenantID,
		StatusCode: statusCode,
		Error:      param.Error,
		DurationMs: int32(param.Duration.Milliseconds()),
	})
	if err != nil {
		return fmt.Errorf("insert attempt: %w", err)
	}
	return nil
}

func (r *DeliveryRepository) ListAttempts(ctx context.Context, deliveryIDs []uuid.UUID) ([]webhook.Attempt, error) {
	rows, err := queries(ctx, r.pool).ListAttemptsByDeliveries(ctx, deliveryIDs)
	if err != nil {
		return nil, fmt.Errorf("select attempts: %w", err)
	}

	attempts := make([]webhook.Attempt, len(rows))
	for i, row := range rows {
		var statusCode *int
		if row.StatusCode != nil {
			code := int(*row.StatusCode)
			statusCode = &code
		}
		attempts[i] = webhook.Attempt{
			ID:         row.ID,
			DeliveryID: row.DeliveryID,
			StatusCode: statusCode,
			Error:      row.Error,
			Duration:   time.Duration(row.DurationMs) * time.Millisecond,
			CreatedAt:  row.CreatedAt,
		}
	}
	return attempts, nil
}
