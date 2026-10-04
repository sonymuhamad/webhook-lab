package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/enum"
)

const (
	outcomeSucceeded  = "succeeded"
	outcomeHTTPError  = "http_error"
	outcomeNoResponse = "no_response"
)

// busyEndpointDelay is how long a delivery to an endpoint at its cap waits
// before it is due again.
const busyEndpointDelay = time.Second

// The metric API returns a usable no-op instrument alongside any error, and
// errors only come from an invalid name or unit, which these constants rule
// out.
var (
	deliveryMeter = otel.Meter("github.com/sonymuhamad/webhook-lab/usecase")

	attemptCounter, _ = deliveryMeter.Int64Counter("delivery.attempts",
		metric.WithDescription("Delivery attempts by outcome."))
	sendDuration, _ = deliveryMeter.Float64Histogram("delivery.send.duration",
		metric.WithDescription("Time spent waiting for the endpoint to answer."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10))
	claimDuration, _ = deliveryMeter.Float64Histogram("delivery.claim.duration",
		metric.WithDescription("Time to take one batch of due deliveries off the queue."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1))
	postponedCounter, _ = deliveryMeter.Int64Counter("delivery.postponed",
		metric.WithDescription("Deliveries handed back unsent because their endpoint was at its concurrency cap."))
	deliveryLag, _ = deliveryMeter.Float64Histogram("delivery.lag",
		metric.WithDescription("Time from message accepted to successful delivery, retries included."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1800, 3600))
)

type Delivery struct {
	claimer   webhook.Claimer
	repo      webhook.DeliveryRepository
	sender    webhook.Sender
	tx        webhook.Transactor
	cfg       config.Delivery
	endpoints *endpointSlots
}

func NewDelivery(
	claimer webhook.Claimer,
	repo webhook.DeliveryRepository,
	sender webhook.Sender,
	tx webhook.Transactor,
	cfg config.Delivery,
	worker config.Worker,
) *Delivery {
	return &Delivery{
		claimer:   claimer,
		repo:      repo,
		sender:    sender,
		tx:        tx,
		cfg:       cfg,
		endpoints: newEndpointSlots(worker.EndpointConcurrency),
	}
}

// ProcessDue sends up to limit due deliveries one after another and returns
// how many it processed. A delivery whose endpoint is at its concurrency cap
// is postponed instead of sent, and still counts as processed. ProcessDue
// stops at the first delivery whose outcome could not be saved; that delivery
// stays pending and is sent again later.
func (u *Delivery) ProcessDue(ctx context.Context, limit int) (int, error) {
	start := time.Now()
	due, err := u.claimer.ClaimDue(ctx, limit)
	claimDuration.Record(ctx, time.Since(start).Seconds())
	if err != nil {
		return 0, fmt.Errorf("claim due deliveries: %w", err)
	}

	for i, d := range due {
		if !u.endpoints.acquire(d.EndpointID) {
			if err := u.repo.Postpone(ctx, d.ID, time.Now().Add(busyEndpointDelay)); err != nil {
				return i, fmt.Errorf("postpone %s: %w", d.ID, err)
			}
			postponedCounter.Add(ctx, 1)
			continue
		}
		err := u.deliver(ctx, d)
		u.endpoints.release(d.EndpointID)
		if err != nil {
			return i, fmt.Errorf("deliver %s: %w", d.ID, err)
		}
	}
	return len(due), nil
}

func (u *Delivery) CountPending(ctx context.Context) (webhook.PendingCount, error) {
	count, err := u.repo.CountPending(ctx)
	if err != nil {
		return webhook.PendingCount{}, fmt.Errorf("count pending deliveries: %w", err)
	}
	return count, nil
}

func (u *Delivery) deliver(ctx context.Context, d webhook.DueDelivery) error {
	result, sendErr := u.sender.Send(ctx, webhook.SendRequest{
		URL:        d.EndpointURL,
		MessageID:  d.MessageID,
		DeliveryID: d.ID,
		EventType:  d.EventType,
		Payload:    d.Payload,
	})

	attempt := webhook.CreateAttemptParam{DeliveryID: d.ID, TenantID: d.TenantID, Duration: result.Duration}
	outcome := outcomeSucceeded
	switch {
	case sendErr != nil:
		msg := sendErr.Error()
		attempt.Error = &msg
		outcome = outcomeNoResponse
	case result.StatusCode < 200 || result.StatusCode >= 300:
		attempt.StatusCode = &result.StatusCode
		outcome = outcomeHTTPError
	default:
		attempt.StatusCode = &result.StatusCode
	}
	recordAttempt(ctx, d, outcome, result.Duration)

	return u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.repo.CreateAttempt(ctx, attempt); err != nil {
			return err
		}
		return u.repo.Update(ctx, u.nextState(d, outcome == outcomeSucceeded))
	})
}

func (u *Delivery) nextState(d webhook.DueDelivery, succeeded bool) webhook.UpdateDeliveryParam {
	now := time.Now()
	switch {
	case succeeded:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusSucceeded, NextAttemptAt: now}
	case d.AttemptCount+1 >= u.cfg.MaxAttempts:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusFailed, NextAttemptAt: now}
	default:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusPending, NextAttemptAt: now.Add(u.cfg.RetryDelay)}
	}
}

// recordAttempt measures lag against the message's created_at, which comes
// from the database clock. It is only as accurate as the skew between that
// clock and this host's.
func recordAttempt(ctx context.Context, d webhook.DueDelivery, outcome string, duration time.Duration) {
	withOutcome := metric.WithAttributes(attribute.String("outcome", outcome))
	attemptCounter.Add(ctx, 1, withOutcome)
	sendDuration.Record(ctx, duration.Seconds(), withOutcome)
	if outcome == outcomeSucceeded {
		deliveryLag.Record(ctx, time.Since(d.MessageCreatedAt).Seconds())
	}
}

// endpointSlots counts the sends in progress per endpoint across all loops
// of this process. The cap is per process: two worker processes may each
// send to an endpoint up to the cap.
type endpointSlots struct {
	limit int
	mu    sync.Mutex
	busy  map[uuid.UUID]int
}

// newEndpointSlots returns nil for a limit of 0 or less, which caps nothing.
func newEndpointSlots(limit int) *endpointSlots {
	if limit <= 0 {
		return nil
	}
	return &endpointSlots{limit: limit, busy: make(map[uuid.UUID]int)}
}

func (s *endpointSlots) acquire(endpoint uuid.UUID) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[endpoint] >= s.limit {
		return false
	}
	s.busy[endpoint]++
	return true
}

func (s *endpointSlots) release(endpoint uuid.UUID) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[endpoint]--; s.busy[endpoint] == 0 {
		delete(s.busy, endpoint)
	}
}
