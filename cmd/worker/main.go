package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/di"
	"github.com/sonymuhamad/webhook-lab/telemetry"
)

const telemetryFlushTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logExport, shutdownTelemetry, err := telemetry.Setup(ctx, cfg.Telemetry, "webhook-worker")
	if err != nil {
		return fmt.Errorf("setup telemetry: %w", err)
	}
	slog.SetDefault(telemetry.NewLogger(cfg.LogLevel, logExport))

	w, cleanup, err := di.InitWorker(ctx, cfg)
	if err != nil {
		flushTelemetry(shutdownTelemetry)
		return fmt.Errorf("init worker: %w", err)
	}
	defer cleanup()
	// Deferred after cleanup so it runs before it: the final export reads
	// metrics that query the pool, which must still be open.
	defer flushTelemetry(shutdownTelemetry)

	slog.Info("worker started",
		"batch_size", cfg.Worker.BatchSize,
		"poll_interval", cfg.Worker.PollInterval.String(),
		"gomaxprocs", runtime.GOMAXPROCS(0),
	)
	w.Run(ctx)
	slog.Info("worker stopped")
	return nil
}

func flushTelemetry(shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), telemetryFlushTimeout)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		slog.Error("flush telemetry", "error", err)
	}
}
