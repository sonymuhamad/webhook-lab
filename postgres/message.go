package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

func (r *MessageRepository) Create(ctx context.Context, param webhook.CreateMessageParam) (webhook.Message, error) {
	row, err := queries(ctx, r.pool).CreateMessage(ctx, sqlcgen.CreateMessageParams{
		TenantID:       param.TenantID,
		EventType:      param.EventType,
		Payload:        param.Payload,
		IdempotencyKey: param.IdempotencyKey,
	})
	// ON CONFLICT DO NOTHING returns no row when the idempotency key is taken.
	if errors.Is(err, pgx.ErrNoRows) {
		return webhook.Message{}, webhook.ErrConflict
	}
	if err != nil {
		return webhook.Message{}, fmt.Errorf("insert message: %w", err)
	}
	return toMessage(row), nil
}

func (r *MessageRepository) GetByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (webhook.Message, error) {
	row, err := queries(ctx, r.pool).GetMessageByIdempotencyKey(ctx, sqlcgen.GetMessageByIdempotencyKeyParams{
		TenantID:       tenantID,
		IdempotencyKey: &key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return webhook.Message{}, webhook.ErrNotFound
	}
	if err != nil {
		return webhook.Message{}, fmt.Errorf("select message by idempotency key: %w", err)
	}
	return toMessage(row), nil
}

func (r *MessageRepository) Get(ctx context.Context, tenantID, messageID uuid.UUID) (webhook.Message, error) {
	row, err := queries(ctx, r.pool).GetMessage(ctx, sqlcgen.GetMessageParams{ID: messageID, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return webhook.Message{}, webhook.ErrNotFound
	}
	if err != nil {
		return webhook.Message{}, fmt.Errorf("select message: %w", err)
	}
	return toMessage(row), nil
}

func toMessage(row sqlcgen.Message) webhook.Message {
	return webhook.Message{
		ID:             row.ID,
		TenantID:       row.TenantID,
		EventType:      row.EventType,
		Payload:        row.Payload,
		IdempotencyKey: row.IdempotencyKey,
		CreatedAt:      row.CreatedAt,
	}
}
