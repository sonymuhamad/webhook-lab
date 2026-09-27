package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
)

type Worker struct {
	deliveries webhook.DeliveryUsecase
	cfg        config.Worker
}

// New also registers the deliveries.pending gauge. Every worker reports the
// same table-wide count, so dashboards should take the max across instances,
// not the sum.
func New(deliveries webhook.DeliveryUsecase, cfg config.Worker) (*Worker, error) {
	meter := otel.Meter("github.com/sonymuhamad/webhook-lab/worker")
	due := metric.WithAttributes(attribute.String("state", "due"))
	scheduled := metric.WithAttributes(attribute.String("state", "scheduled"))

	_, err := meter.Int64ObservableGauge("deliveries.pending",
		metric.WithDescription("Pending deliveries: due now, or scheduled for a later retry."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			count, err := deliveries.CountPending(ctx)
			if err != nil {
				return err
			}
			o.Observe(count.Due, due)
			o.Observe(count.Scheduled, scheduled)
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("register pending gauge: %w", err)
	}

	return &Worker{deliveries: deliveries, cfg: cfg}, nil
}

// Run polls for due deliveries until ctx is cancelled. A full batch means more
// work is probably waiting, so it polls again at once; otherwise it sleeps for
// PollInterval.
//
// A batch already in progress finishes after cancellation, so an in-flight
// request is not cut off and recorded as a failed attempt. Shutdown can
// therefore take up to BatchSize × delivery timeout.
func (w *Worker) Run(ctx context.Context) {
	for {
		n, err := w.deliveries.ProcessDue(context.WithoutCancel(ctx), w.cfg.BatchSize)
		if err != nil {
			slog.ErrorContext(ctx, "process due deliveries", "error", err)
		}
		if n > 0 {
			slog.DebugContext(ctx, "processed deliveries", "count", n)
		}

		if err == nil && n == w.cfg.BatchSize && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(w.cfg.PollInterval):
		}
	}
}
