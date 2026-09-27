package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate runs a goose command ("up", "down", "status", ...) against the
// embedded migrations.
func Migrate(ctx context.Context, url, command string, args ...string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.RunContext(ctx, command, db, "migrations", args...)
}
