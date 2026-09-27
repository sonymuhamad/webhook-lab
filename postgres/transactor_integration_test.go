//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sonymuhamad/webhook-lab/postgres"
)

func countTenants(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM tenants").Scan(&n); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	return n
}

func TestTransactorCommits(t *testing.T) {
	resetDB(t)
	repo := postgres.NewTenantRepository(testPool)

	err := postgres.NewTransactor(testPool).WithinTx(context.Background(), func(ctx context.Context) error {
		_, err := repo.Create(ctx, "acme")
		return err
	})

	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
	if n := countTenants(t); n != 1 {
		t.Errorf("tenants = %d, want 1", n)
	}
}

// Proves repositories actually join the transaction from the context: if one
// used the pool instead, its insert would survive the rollback.
func TestTransactorRollsBackOnError(t *testing.T) {
	resetDB(t)
	repo := postgres.NewTenantRepository(testPool)
	failure := errors.New("second step failed")

	err := postgres.NewTransactor(testPool).WithinTx(context.Background(), func(ctx context.Context) error {
		if _, err := repo.Create(ctx, "acme"); err != nil {
			return err
		}
		return failure
	})

	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want %v", err, failure)
	}
	if n := countTenants(t); n != 0 {
		t.Errorf("tenants = %d, want 0 after rollback", n)
	}
}

func TestTransactorNestedCallJoinsOuter(t *testing.T) {
	resetDB(t)
	repo := postgres.NewTenantRepository(testPool)
	tx := postgres.NewTransactor(testPool)
	failure := errors.New("outer failed")

	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		err := tx.WithinTx(ctx, func(ctx context.Context) error {
			_, err := repo.Create(ctx, "inner")
			return err
		})
		if err != nil {
			return err
		}
		return failure
	})

	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want %v", err, failure)
	}
	if n := countTenants(t); n != 0 {
		t.Errorf("tenants = %d, want 0: the inner call committed on its own", n)
	}
}
