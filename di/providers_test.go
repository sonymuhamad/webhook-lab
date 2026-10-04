package di

import (
	"testing"
	"time"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/enum"
)

func TestProvideClaimerRejectsBadConfig(t *testing.T) {
	delivery := config.Delivery{Timeout: 10 * time.Second}
	tests := map[string]config.Worker{
		"unknown mode":             {ClaimMode: enum.ClaimMode("fifo"), BatchSize: 10, ClaimLease: time.Hour},
		"lease shorter than batch": {ClaimMode: enum.ClaimModeSkiplocked, BatchSize: 10, ClaimLease: time.Minute},
		"lease equal to batch":     {ClaimMode: enum.ClaimModeSkiplocked, BatchSize: 10, ClaimLease: 100 * time.Second},
		"fair, lease too short":    {ClaimMode: enum.ClaimModeFair, BatchSize: 10, ClaimLease: time.Minute},
	}
	for name, worker := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := provideClaimer(worker, delivery, nil); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestProvideClaimerAcceptsLeaseLongerThanBatch(t *testing.T) {
	worker := config.Worker{ClaimMode: enum.ClaimModeSkiplocked, BatchSize: 10, ClaimLease: 101 * time.Second}
	if _, err := provideClaimer(worker, config.Delivery{Timeout: 10 * time.Second}, nil); err != nil {
		t.Fatalf("provideClaimer: %v", err)
	}
}
