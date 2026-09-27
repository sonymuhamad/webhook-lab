//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

// testPool is shared by every test in the package. Tests must call resetDB
// first and must not run in parallel.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		log.Fatal(err)
	}
	os.Exit(code)
}

// run starts one Postgres container for the whole package, since starting one
// per test would dominate the runtime.
func run(m *testing.M) (int, error) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("webhook_lab"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			log.Printf("terminate postgres container: %v", err)
		}
	}()
	if err != nil {
		return 0, fmt.Errorf("start postgres container: %w", err)
	}

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("postgres connection string: %w", err)
	}
	if err := postgres.Migrate(ctx, url, "up"); err != nil {
		return 0, fmt.Errorf("migrate: %w", err)
	}

	pool, closePool, err := postgres.NewPool(ctx, config.Postgres{URL: url})
	if err != nil {
		return 0, fmt.Errorf("connect postgres: %w", err)
	}
	defer closePool()
	testPool = pool

	return m.Run(), nil
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"TRUNCATE attempts, deliveries, messages, endpoints, api_keys, tenants")
	if err != nil {
		t.Fatalf("reset database: %v", err)
	}
}
