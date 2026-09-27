package webhook

//go:generate go tool mockgen -source=message.go -destination=mock/message.go -package=mock

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	EventType      string
	Payload        json.RawMessage
	IdempotencyKey *string
	CreatedAt      time.Time
}

type CreateMessageParam struct {
	TenantID       uuid.UUID
	EventType      string
	Payload        json.RawMessage
	IdempotencyKey *string
}

type MessageDetail struct {
	Message    Message
	Deliveries []DeliveryDetail
}

type MessageUsecase interface {
	Create(ctx context.Context, param CreateMessageParam) (Message, error)
	Get(ctx context.Context, tenantID, messageID uuid.UUID) (MessageDetail, error)
}

type MessageRepository interface {
	// Create returns ErrConflict when the tenant already sent a message with
	// the same idempotency key.
	Create(ctx context.Context, param CreateMessageParam) (Message, error)
	GetByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (Message, error)
	Get(ctx context.Context, tenantID, messageID uuid.UUID) (Message, error)
}
