//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/enum"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

func createEndpoint(t *testing.T, tenantID uuid.UUID, url string) webhook.Endpoint {
	t.Helper()
	endpoint, err := postgres.NewEndpointRepository(testPool).Create(context.Background(), webhook.CreateEndpointParam{TenantID: tenantID, URL: url})
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	return endpoint
}

func TestDeliveryRepositoryFanOutSkipsDisabledAndOtherTenants(t *testing.T) {
	resetDB(t)
	repo := postgres.NewDeliveryRepository(testPool)
	tenant, _ := createTenant(t, "acme")
	other, _ := createTenant(t, "other")
	active := createEndpoint(t, tenant.ID, "https://a.example.com")
	disabled := createEndpoint(t, tenant.ID, "https://b.example.com")
	createEndpoint(t, other.ID, "https://other.example.com")
	if _, err := testPool.Exec(context.Background(), "UPDATE endpoints SET disabled_at = now() WHERE id = $1", disabled.ID); err != nil {
		t.Fatalf("disable endpoint: %v", err)
	}
	message := createMessage(t, tenant.ID, nil)

	n, err := repo.CreateForActiveEndpoints(context.Background(), message)
	if err != nil {
		t.Fatalf("CreateForActiveEndpoints: %v", err)
	}

	deliveries, err := repo.ListByMessage(context.Background(), message.ID)
	if err != nil {
		t.Fatalf("ListByMessage: %v", err)
	}
	if n != 1 || len(deliveries) != 1 || deliveries[0].EndpointID != active.ID {
		t.Fatalf("created %d, listed %+v; want exactly one delivery to %s", n, deliveries, active.ID)
	}
	if deliveries[0].Status != enum.DeliveryStatusPending || deliveries[0].EndpointURL != active.URL {
		t.Errorf("delivery = %+v, want pending to %s", deliveries[0], active.URL)
	}
}

func TestDeliveryRepositoryListDueOnlyReturnsPendingAndDue(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewDeliveryRepository(testPool)
	tenant, _ := createTenant(t, "acme")
	endpoint := createEndpoint(t, tenant.ID, "https://example.com/hook")

	newDelivery := func() uuid.UUID {
		message := createMessage(t, tenant.ID, nil)
		if _, err := repo.CreateForActiveEndpoints(ctx, message); err != nil {
			t.Fatalf("CreateForActiveEndpoints: %v", err)
		}
		deliveries, err := repo.ListByMessage(ctx, message.ID)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("ListByMessage = %+v, %v", deliveries, err)
		}
		return deliveries[0].ID
	}
	due := newDelivery()
	later := newDelivery()
	done := newDelivery()

	if err := repo.Update(ctx, webhook.UpdateDeliveryParam{ID: later, Status: enum.DeliveryStatusPending, NextAttemptAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := repo.Update(ctx, webhook.UpdateDeliveryParam{ID: done, Status: enum.DeliveryStatusSucceeded, NextAttemptAt: time.Now()}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.ListDue(ctx, 10)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(got) != 1 || got[0].ID != due {
		t.Fatalf("due = %+v, want only %s", got, due)
	}
	if got[0].EndpointURL != endpoint.URL || got[0].EventType != "booking.created" || len(got[0].Payload) == 0 {
		t.Errorf("due delivery is missing what the sender needs: %+v", got[0])
	}
}

func TestDeliveryRepositoryAttemptsAndUpdate(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewDeliveryRepository(testPool)
	tenant, _ := createTenant(t, "acme")
	createEndpoint(t, tenant.ID, "https://example.com/hook")
	message := createMessage(t, tenant.ID, nil)
	if _, err := repo.CreateForActiveEndpoints(ctx, message); err != nil {
		t.Fatalf("CreateForActiveEndpoints: %v", err)
	}
	deliveries, _ := repo.ListByMessage(ctx, message.ID)
	deliveryID := deliveries[0].ID

	statusCode := 503
	errMsg := "connection refused"
	for _, attempt := range []webhook.CreateAttemptParam{
		{DeliveryID: deliveryID, TenantID: tenant.ID, StatusCode: &statusCode, Duration: 120 * time.Millisecond},
		{DeliveryID: deliveryID, TenantID: tenant.ID, Error: &errMsg, Duration: 10 * time.Second},
	} {
		if err := repo.CreateAttempt(ctx, attempt); err != nil {
			t.Fatalf("CreateAttempt: %v", err)
		}
		if err := repo.Update(ctx, webhook.UpdateDeliveryParam{ID: deliveryID, Status: enum.DeliveryStatusPending, NextAttemptAt: time.Now()}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}

	attempts, err := repo.ListAttempts(ctx, []uuid.UUID{deliveryID})
	if err != nil {
		t.Fatalf("ListAttempts: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0].StatusCode == nil || *attempts[0].StatusCode != 503 || attempts[0].Error != nil || attempts[0].Duration != 120*time.Millisecond {
		t.Errorf("first attempt = %+v", attempts[0])
	}
	if attempts[1].StatusCode != nil || attempts[1].Error == nil || *attempts[1].Error != errMsg {
		t.Errorf("second attempt = %+v", attempts[1])
	}

	deliveries, _ = repo.ListByMessage(ctx, message.ID)
	if deliveries[0].AttemptCount != 2 {
		t.Errorf("attempt_count = %d, want 2", deliveries[0].AttemptCount)
	}
}
