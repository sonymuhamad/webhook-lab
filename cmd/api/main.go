package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/di"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
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

	srv, cleanup, err := di.InitAPI(ctx, cfg)
	if err != nil {
		return fmt.Errorf("init api: %w", err)
	}
	defer cleanup()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	// GOMAXPROCS is logged because Go 1.25+ derives it from the container's
	// CPU limit; lab numbers are only comparable when this value matches.
	slog.Info("api started", "addr", cfg.HTTP.Addr, "gomaxprocs", runtime.GOMAXPROCS(0))

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	slog.Info("api shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown http: %w", err)
	}
	return nil
}
