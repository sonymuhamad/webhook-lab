//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

func createMessage(t *testing.T, tenantID uuid.UUID, idempotencyKey *string) webhook.Message {
	t.Helper()
	message, err := postgres.NewMessageRepository(testPool).Create(context.Background(), webhook.CreateMessageParam{
		TenantID:       tenantID,
		EventType:      "booking.created",
		Payload:        json.RawMessage(`{"booking_id":"b_1"}`),
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	return message
}

func TestMessageRepositoryCreateAndGet(t *testing.T) {
	resetDB(t)
	repo := postgres.NewMessageRepository(testPool)
	tenant, _ := createTenant(t, "acme")
	created := createMessage(t, tenant.ID, nil)

	got, err := repo.Get(context.Background(), tenant.ID, created.ID)

	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID || got.EventType != "booking.created" {
		t.Errorf("got %+v", got)
	}
	var payload map[string]string
	if err := json.Unmarshal(got.Payload, &payload); err != nil || payload["booking_id"] != "b_1" {
		t.Errorf("payload = %s, err = %v", got.Payload, err)
	}
}

func TestMessageRepositoryGetIsScopedToTenant(t *testing.T) {
	resetDB(t)
	repo := postgres.NewMessageRepository(testPool)
	owner, _ := createTenant(t, "owner")
	other, _ := createTenant(t, "other")
	message := createMessage(t, owner.ID, nil)

	_, err := repo.Get(context.Background(), other.ID, message.ID)

	if !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMessageRepositoryIdempotencyKey(t *testing.T) {
	resetDB(t)
	repo := postgres.NewMessageRepository(testPool)
	tenantA, _ := createTenant(t, "tenant-a")
	tenantB, _ := createTenant(t, "tenant-b")
	key := "order-42"
	original := createMessage(t, tenantA.ID, &key)

	t.Run("same tenant, same key conflicts", func(t *testing.T) {
		_, err := repo.Create(context.Background(), webhook.CreateMessageParam{
			TenantID: tenantA.ID, EventType: "x", Payload: json.RawMessage(`{}`), IdempotencyKey: &key,
		})
		if !errors.Is(err, webhook.ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
		got, err := repo.GetByIdempotencyKey(context.Background(), tenantA.ID, key)
		if err != nil || got.ID != original.ID {
			t.Fatalf("GetByIdempotencyKey = %+v, %v; want the original message", got, err)
		}
	})
	t.Run("keys are per tenant", func(t *testing.T) {
		createMessage(t, tenantB.ID, &key)
	})
	t.Run("messages without a key never conflict", func(t *testing.T) {
		createMessage(t, tenantA.ID, nil)
		createMessage(t, tenantA.ID, nil)
	})
}
