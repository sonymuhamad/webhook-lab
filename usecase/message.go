package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
)

type Message struct {
	messages   webhook.MessageRepository
	deliveries webhook.DeliveryRepository
	tx         webhook.Transactor
}

func NewMessage(messages webhook.MessageRepository, deliveries webhook.DeliveryRepository, tx webhook.Transactor) *Message {
	return &Message{messages: messages, deliveries: deliveries, tx: tx}
}

// Create writes the message and one pending delivery per active endpoint in
// the same transaction. Once it returns, the worker is guaranteed to find the
// deliveries, even if this process dies straight after.
//
// A repeated idempotency key returns the original message and creates
// nothing; the new payload is ignored, not compared.
func (u *Message) Create(ctx context.Context, param webhook.CreateMessageParam) (webhook.Message, error) {
	var message webhook.Message
	err := u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		message, err = u.messages.Create(ctx, param)
		if errors.Is(err, webhook.ErrConflict) {
			message, err = u.messages.GetByIdempotencyKey(ctx, param.TenantID, *param.IdempotencyKey)
			return err
		}
		if err != nil {
			return err
		}

		_, err = u.deliveries.CreateForActiveEndpoints(ctx, message)
		return err
	})
	if err != nil {
		return webhook.Message{}, fmt.Errorf("create message: %w", err)
	}
	return message, nil
}

func (u *Message) Get(ctx context.Context, tenantID, messageID uuid.UUID) (webhook.MessageDetail, error) {
	message, err := u.messages.Get(ctx, tenantID, messageID)
	if err != nil {
		return webhook.MessageDetail{}, fmt.Errorf("get message: %w", err)
	}

	deliveries, err := u.deliveries.ListByMessage(ctx, message.ID)
	if err != nil {
		return webhook.MessageDetail{}, fmt.Errorf("get message deliveries: %w", err)
	}

	deliveryIDs := make([]uuid.UUID, len(deliveries))
	for i, d := range deliveries {
		deliveryIDs[i] = d.ID
	}
	attempts, err := u.deliveries.ListAttempts(ctx, deliveryIDs)
	if err != nil {
		return webhook.MessageDetail{}, fmt.Errorf("get message attempts: %w", err)
	}

	attemptsByDelivery := make(map[uuid.UUID][]webhook.Attempt, len(deliveries))
	for _, a := range attempts {
		attemptsByDelivery[a.DeliveryID] = append(attemptsByDelivery[a.DeliveryID], a)
	}

	detail := webhook.MessageDetail{Message: message, Deliveries: make([]webhook.DeliveryDetail, len(deliveries))}
	for i, d := range deliveries {
		detail.Deliveries[i] = webhook.DeliveryDetail{Delivery: d, Attempts: attemptsByDelivery[d.ID]}
	}
	return detail, nil
}
