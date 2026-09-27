package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/di"
)

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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	w, cleanup, err := di.InitWorker(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init worker: %w", err)
	}
	defer cleanup()

	slog.Info("worker started",
		"batch_size", cfg.Worker.BatchSize,
		"poll_interval", cfg.Worker.PollInterval.String(),
		"gomaxprocs", runtime.GOMAXPROCS(0),
	)
	w.Run(ctx)
	slog.Info("worker stopped")
	return nil
}
