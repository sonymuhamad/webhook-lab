package webhook

//go:generate go tool mockgen -source=delivery.go -destination=mock/delivery.go -package=mock

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/sonymuhamad/webhook-lab/enum"
)

type Delivery struct {
	ID            uuid.UUID
	MessageID     uuid.UUID
	EndpointID    uuid.UUID
	EndpointURL   string
	Status        enum.DeliveryStatus
	AttemptCount  int
	NextAttemptAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Attempt struct {
	ID         uuid.UUID
	DeliveryID uuid.UUID
	// StatusCode is nil when no response arrived; Error then says why.
	StatusCode *int
	Error      *string
	Duration   time.Duration
	CreatedAt  time.Time
}

type DeliveryDetail struct {
	Delivery Delivery
	Attempts []Attempt
}

// DueDelivery is a pending delivery with everything the sender needs.
type DueDelivery struct {
	ID               uuid.UUID
	MessageID        uuid.UUID
	TenantID         uuid.UUID
	EndpointURL      string
	EventType        string
	Payload          json.RawMessage
	MessageCreatedAt time.Time
	AttemptCount     int
}

// PendingCount splits pending deliveries into those the worker can send now
// and those waiting for a retry delay to pass.
type PendingCount struct {
	Due       int64
	Scheduled int64
}

type CreateAttemptParam struct {
	DeliveryID uuid.UUID
	TenantID   uuid.UUID
	StatusCode *int
	Error      *string
	Duration   time.Duration
}

type UpdateDeliveryParam struct {
	ID            uuid.UUID
	Status        enum.DeliveryStatus
	NextAttemptAt time.Time
}

type SendRequest struct {
	URL        string
	MessageID  uuid.UUID
	DeliveryID uuid.UUID
	EventType  string
	Payload    json.RawMessage
}

type SendResult struct {
	StatusCode int
	Duration   time.Duration
}

type DeliveryUsecase interface {
	ProcessDue(ctx context.Context, limit int) (int, error)
	CountPending(ctx context.Context) (PendingCount, error)
}

type Claimer interface {
	ClaimDue(ctx context.Context, limit int) ([]DueDelivery, error)
}

type DeliveryRepository interface {
	CreateForActiveEndpoints(ctx context.Context, message Message) (int64, error)
	ListByMessage(ctx context.Context, messageID uuid.UUID) ([]Delivery, error)
	Update(ctx context.Context, param UpdateDeliveryParam) error
	CreateAttempt(ctx context.Context, param CreateAttemptParam) error
	ListAttempts(ctx context.Context, deliveryIDs []uuid.UUID) ([]Attempt, error)
	CountPending(ctx context.Context) (PendingCount, error)
}

type Sender interface {
	// Send returns an error only when no HTTP response arrived. Any response,
	// including a 5xx, is a SendResult. Duration is set in both cases.
	Send(ctx context.Context, req SendRequest) (SendResult, error)
}
