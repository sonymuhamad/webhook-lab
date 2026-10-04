//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/enum"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

// seedDue creates n pending deliveries that are due now and returns their IDs.
func seedDue(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	return seedDueFor(t, "acme", n)
}

// seedDueFor is seedDue for a new tenant of the given name.
func seedDueFor(t *testing.T, tenantName string, n int) []uuid.UUID {
	t.Helper()
	ctx := context.Background()
	repo := postgres.NewDeliveryRepository(testPool)
	tenant, _ := createTenant(t, tenantName)
	createEndpoint(t, tenant.ID, "https://example.com/hook")

	ids := make([]uuid.UUID, 0, n)
	for range n {
		message := createMessage(t, tenant.ID, nil)
		if _, err := repo.CreateForActiveEndpoints(ctx, message); err != nil {
			t.Fatalf("CreateForActiveEndpoints: %v", err)
		}
		deliveries, err := repo.ListByMessage(ctx, message.ID)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("ListByMessage = %+v, %v", deliveries, err)
		}
		ids = append(ids, deliveries[0].ID)
	}
	return ids
}

func claimers() map[string]webhook.Claimer {
	return map[string]webhook.Claimer{
		"naive":      postgres.NewNaiveClaimer(testPool),
		"skiplocked": postgres.NewSkipLockedClaimer(testPool, time.Minute),
		"fair":       postgres.NewFairClaimer(testPool, time.Minute),
	}
}

func TestClaimersReturnOnlyPendingAndDue(t *testing.T) {
	for name, claimer := range claimers() {
		t.Run(name, func(t *testing.T) {
			resetDB(t)
			ctx := context.Background()
			repo := postgres.NewDeliveryRepository(testPool)
			ids := seedDue(t, 3)
			due, later, done := ids[0], ids[1], ids[2]
			if err := repo.Update(ctx, webhook.UpdateDeliveryParam{ID: later, Status: enum.DeliveryStatusPending, NextAttemptAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatalf("update: %v", err)
			}
			if err := repo.Update(ctx, webhook.UpdateDeliveryParam{ID: done, Status: enum.DeliveryStatusSucceeded, NextAttemptAt: time.Now()}); err != nil {
				t.Fatalf("update: %v", err)
			}

			got, err := claimer.ClaimDue(ctx, 10)
			if err != nil {
				t.Fatalf("ClaimDue: %v", err)
			}
			if len(got) != 1 || got[0].ID != due {
				t.Fatalf("claimed %+v, want only %s", got, due)
			}
			if got[0].EndpointURL != "https://example.com/hook" || got[0].EndpointID == uuid.Nil ||
				got[0].EventType != "booking.created" || len(got[0].Payload) == 0 {
				t.Errorf("claimed delivery is missing what the sender needs: %+v", got[0])
			}
		})
	}
}

func TestSkipLockedClaimerHidesClaimedDeliveries(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	claimer := postgres.NewSkipLockedClaimer(testPool, time.Minute)
	seedDue(t, 3)

	first, err := claimer.ClaimDue(ctx, 10)
	if err != nil || len(first) != 3 {
		t.Fatalf("first claim = %d deliveries, %v; want 3", len(first), err)
	}
	second, err := claimer.ClaimDue(ctx, 10)
	if err != nil || len(second) != 0 {
		t.Fatalf("second claim = %d deliveries, %v; want 0 while the lease holds", len(second), err)
	}
}

// Stands in for a worker that died after claiming: once the lease has run
// out, the delivery is claimable again.
func TestSkipLockedClaimerReleasesAfterLease(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	claimer := postgres.NewSkipLockedClaimer(testPool, time.Minute)
	ids := seedDue(t, 1)
	if got, err := claimer.ClaimDue(ctx, 10); err != nil || len(got) != 1 {
		t.Fatalf("claim = %d, %v; want 1", len(got), err)
	}

	if _, err := testPool.Exec(ctx, "UPDATE deliveries SET next_attempt_at = now() - interval '1 second' WHERE id = $1", ids[0]); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	got, err := claimer.ClaimDue(ctx, 10)
	if err != nil || len(got) != 1 || got[0].ID != ids[0] {
		t.Fatalf("claim after lease = %+v, %v; want %s again", got, err, ids[0])
	}
}

// The property lab 01 measures, as a test: concurrent claims never hand the
// same delivery to two callers.
func TestSkipLockedClaimerConcurrentClaimsAreDisjoint(t *testing.T) {
	resetDB(t)
	claimer := postgres.NewSkipLockedClaimer(testPool, time.Minute)
	seedDue(t, 60)

	const callers = 6
	var (
		mu      sync.Mutex
		claimed = make(map[uuid.UUID]int)
		wg      sync.WaitGroup
	)
	for range callers {
		wg.Go(func() {
			got, err := claimer.ClaimDue(context.Background(), 10)
			if err != nil {
				t.Errorf("ClaimDue: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, d := range got {
				claimed[d.ID]++
			}
		})
	}
	wg.Wait()

	for id, n := range claimed {
		if n > 1 {
			t.Errorf("delivery %s claimed %d times", id, n)
		}
	}
	if len(claimed) != 60 {
		t.Errorf("claimed %d distinct deliveries, want all 60", len(claimed))
	}
}

// The property lab 03 measures, as a test: a tenant with a large backlog does
// not keep a newer tenant's deliveries out of the batch.
func TestFairClaimerTakesEveryTenantInTurn(t *testing.T) {
	resetDB(t)
	seedDueFor(t, "busy", 30)
	quiet := seedDueFor(t, "quiet", 2)

	got, err := postgres.NewFairClaimer(testPool, time.Minute).ClaimDue(context.Background(), 10)
	if err != nil || len(got) != 10 {
		t.Fatalf("claim = %d deliveries, %v; want 10", len(got), err)
	}
	claimed := make(map[uuid.UUID]bool)
	for _, d := range got {
		claimed[d.ID] = true
	}
	for _, id := range quiet {
		if !claimed[id] {
			t.Errorf("quiet tenant's delivery %s was left behind the busy tenant's backlog", id)
		}
	}
}

// Concurrent fair claims on one busy tenant may come back short, but they
// must never hand the same delivery to two callers.
func TestFairClaimerConcurrentClaimsAreDisjoint(t *testing.T) {
	resetDB(t)
	claimer := postgres.NewFairClaimer(testPool, time.Minute)
	seedDue(t, 60)

	var (
		mu      sync.Mutex
		claimed = make(map[uuid.UUID]int)
		wg      sync.WaitGroup
	)
	for range 6 {
		wg.Go(func() {
			got, err := claimer.ClaimDue(context.Background(), 10)
			if err != nil {
				t.Errorf("ClaimDue: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, d := range got {
				claimed[d.ID]++
			}
		})
	}
	wg.Wait()

	for id, n := range claimed {
		if n > 1 {
			t.Errorf("delivery %s claimed %d times", id, n)
		}
	}
	if len(claimed) == 0 {
		t.Error("no delivery claimed")
	}
}

func TestPostponeHidesDeliveryWithoutAnAttempt(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := postgres.NewDeliveryRepository(testPool)
	ids := seedDue(t, 1)

	if err := repo.Postpone(ctx, ids[0], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Postpone: %v", err)
	}

	if got, err := postgres.NewSkipLockedClaimer(testPool, time.Minute).ClaimDue(ctx, 10); err != nil || len(got) != 0 {
		t.Fatalf("claim after postpone = %d, %v; want 0 until the delivery is due again", len(got), err)
	}
	var attempts int
	if err := testPool.QueryRow(ctx, "SELECT attempt_count FROM deliveries WHERE id = $1", ids[0]).Scan(&attempts); err != nil {
		t.Fatalf("read attempt count: %v", err)
	}
	if attempts != 0 {
		t.Errorf("attempt_count = %d after postpone, want 0", attempts)
	}
}
