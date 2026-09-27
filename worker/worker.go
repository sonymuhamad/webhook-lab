package worker

import (
	"context"
	"log/slog"
	"time"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
)

type Worker struct {
	deliveries webhook.DeliveryUsecase
	cfg        config.Worker
}

func New(deliveries webhook.DeliveryUsecase, cfg config.Worker) *Worker {
	return &Worker{deliveries: deliveries, cfg: cfg}
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
