package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

// Usage: go run ./cmd/migrate [up|down|status|redo|...] — defaults to up.
func main() {
	if err := run(); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	command := "up"
	var args []string
	if len(os.Args) > 1 {
		command, args = os.Args[1], os.Args[2:]
	}
	return postgres.Migrate(context.Background(), cfg.Postgres.URL, command, args...)
}
