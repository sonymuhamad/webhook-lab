package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/worker"
)

// blockingDeliveries holds every ProcessDue call open until release is
// closed, so a test can count how many loops are polling at the same time.
type blockingDeliveries struct {
	mu      sync.Mutex
	active  int
	arrived chan struct{}
	release chan struct{}
}

func (d *blockingDeliveries) ProcessDue(context.Context, int) (int, error) {
	d.mu.Lock()
	d.active++
	d.mu.Unlock()
	d.arrived <- struct{}{}
	<-d.release
	return 0, nil
}

func (d *blockingDeliveries) CountPending(context.Context) (webhook.PendingCount, error) {
	return webhook.PendingCount{}, nil
}

func TestRunStartsCountLoopsConcurrently(t *testing.T) {
	deliveries := &blockingDeliveries{arrived: make(chan struct{}, 10), release: make(chan struct{})}
	w, err := worker.New(deliveries, config.Worker{Count: 3, BatchSize: 10, PollInterval: time.Hour})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	for range 3 {
		select {
		case <-deliveries.arrived:
		case <-time.After(time.Second):
			t.Fatal("fewer than 3 loops polled concurrently")
		}
	}

	cancel()
	close(deliveries.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	if deliveries.active != 3 {
		t.Errorf("ProcessDue calls = %d, want 3", deliveries.active)
	}
}

func TestNewRejectsZeroCount(t *testing.T) {
	_, err := worker.New(&blockingDeliveries{}, config.Worker{Count: 0})
	if err == nil {
		t.Fatal("expected an error for WORKER_COUNT=0")
	}
}
