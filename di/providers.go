package di

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/enum"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

// provideClaimer is the only place that knows which claim strategy runs.
func provideClaimer(worker config.Worker, delivery config.Delivery, pool *pgxpool.Pool) (webhook.Claimer, error) {
	switch worker.ClaimMode {
	case enum.ClaimModeNaive:
		return postgres.NewNaiveClaimer(pool), nil
	case enum.ClaimModeSkiplocked, enum.ClaimModeFair:
		longestBatch := time.Duration(worker.BatchSize) * delivery.Timeout
		if worker.ClaimLease <= longestBatch {
			return nil, fmt.Errorf("WORKER_CLAIM_LEASE %s must exceed WORKER_BATCH_SIZE × DELIVERY_TIMEOUT (%s)",
				worker.ClaimLease, longestBatch)
		}
		if worker.ClaimMode == enum.ClaimModeFair {
			return postgres.NewFairClaimer(pool, worker.ClaimLease), nil
		}
		return postgres.NewSkipLockedClaimer(pool, worker.ClaimLease), nil
	default:
		return nil, fmt.Errorf("unknown WORKER_CLAIM_MODE %q", worker.ClaimMode)
	}
}
