package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/mock"
	"github.com/sonymuhamad/webhook-lab/usecase"
)

func newMessageUsecase(t *testing.T) (*usecase.Message, *mock.MockMessageRepository, *mock.MockDeliveryRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	messages := mock.NewMockMessageRepository(ctrl)
	deliveries := mock.NewMockDeliveryRepository(ctrl)
	return usecase.NewMessage(messages, deliveries, passthroughTx(t)), messages, deliveries
}

func TestMessageCreateFansOutToEndpoints(t *testing.T) {
	u, messages, deliveries := newMessageUsecase(t)
	param := webhook.CreateMessageParam{TenantID: uuid.New(), EventType: "booking.created", Payload: json.RawMessage(`{}`)}
	message := webhook.Message{ID: uuid.New(), TenantID: param.TenantID, EventType: param.EventType}
	messages.EXPECT().Create(gomock.Any(), param).Return(message, nil)
	deliveries.EXPECT().CreateForActiveEndpoints(gomock.Any(), message).Return(int64(2), nil)

	got, err := u.Create(context.Background(), param)

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != message.ID {
		t.Errorf("message id = %s, want %s", got.ID, message.ID)
	}
}

// A retried request with the same idempotency key must return the original
// message and must not fan out a second time.
func TestMessageCreateWithRepeatedIdempotencyKey(t *testing.T) {
	u, messages, _ := newMessageUsecase(t)
	key := "order-42"
	param := webhook.CreateMessageParam{TenantID: uuid.New(), EventType: "booking.created", IdempotencyKey: &key}
	original := webhook.Message{ID: uuid.New(), TenantID: param.TenantID}
	messages.EXPECT().Create(gomock.Any(), param).Return(webhook.Message{}, webhook.ErrConflict)
	messages.EXPECT().GetByIdempotencyKey(gomock.Any(), param.TenantID, key).Return(original, nil)
	// No CreateForActiveEndpoints EXPECT.

	got, err := u.Create(context.Background(), param)

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != original.ID {
		t.Errorf("message id = %s, want the original %s", got.ID, original.ID)
	}
}

func TestMessageCreateFailsWhenFanOutFails(t *testing.T) {
	u, messages, deliveries := newMessageUsecase(t)
	dbErr := errors.New("connection reset")
	messages.EXPECT().Create(gomock.Any(), gomock.Any()).Return(webhook.Message{ID: uuid.New()}, nil)
	deliveries.EXPECT().CreateForActiveEndpoints(gomock.Any(), gomock.Any()).Return(int64(0), dbErr)

	_, err := u.Create(context.Background(), webhook.CreateMessageParam{TenantID: uuid.New()})

	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, dbErr)
	}
}

func TestMessageGetGroupsAttemptsByDelivery(t *testing.T) {
	u, messages, deliveries := newMessageUsecase(t)
	tenantID := uuid.New()
	message := webhook.Message{ID: uuid.New(), TenantID: tenantID}
	first := webhook.Delivery{ID: uuid.New(), MessageID: message.ID}
	second := webhook.Delivery{ID: uuid.New(), MessageID: message.ID}
	messages.EXPECT().Get(gomock.Any(), tenantID, message.ID).Return(message, nil)
	deliveries.EXPECT().ListByMessage(gomock.Any(), message.ID).Return([]webhook.Delivery{first, second}, nil)
	deliveries.EXPECT().ListAttempts(gomock.Any(), []uuid.UUID{first.ID, second.ID}).Return([]webhook.Attempt{
		{ID: uuid.New(), DeliveryID: first.ID},
		{ID: uuid.New(), DeliveryID: first.ID},
		{ID: uuid.New(), DeliveryID: second.ID},
	}, nil)

	got, err := u.Get(context.Background(), tenantID, message.ID)

	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Deliveries) != 2 {
		t.Fatalf("deliveries = %d, want 2", len(got.Deliveries))
	}
	if n := len(got.Deliveries[0].Attempts); n != 2 {
		t.Errorf("first delivery attempts = %d, want 2", n)
	}
	if n := len(got.Deliveries[1].Attempts); n != 1 {
		t.Errorf("second delivery attempts = %d, want 1", n)
	}
}

func TestMessageGetNotFound(t *testing.T) {
	u, messages, _ := newMessageUsecase(t)
	messages.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(webhook.Message{}, webhook.ErrNotFound)

	_, err := u.Get(context.Background(), uuid.New(), uuid.New())

	if !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
