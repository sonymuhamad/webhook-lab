//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

func createTenant(t *testing.T, name string) (webhook.Tenant, []byte) {
	t.Helper()
	ctx := context.Background()
	repo := postgres.NewTenantRepository(testPool)

	tenant, err := repo.Create(ctx, name)
	if err != nil {
		t.Fatalf("create tenant %q: %v", name, err)
	}
	hash := make([]byte, 32)
	rand.Read(hash)
	if err := repo.CreateAPIKey(ctx, webhook.CreateAPIKeyParam{TenantID: tenant.ID, Hash: hash, Prefix: "whl_TESTTEST"}); err != nil {
		t.Fatalf("create api key for %q: %v", name, err)
	}
	return tenant, hash
}

func TestTenantRepositoryCreateAndLookup(t *testing.T) {
	resetDB(t)
	repo := postgres.NewTenantRepository(testPool)
	created, hash := createTenant(t, "acme")

	if created.ID == uuid.Nil || created.Name != "acme" || created.CreatedAt.IsZero() {
		t.Fatalf("created tenant = %+v", created)
	}

	got, err := repo.GetByAPIKeyHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetByAPIKeyHash: %v", err)
	}
	if got.ID != created.ID || got.Name != created.Name {
		t.Errorf("got %+v, want %+v", got, created)
	}
}

func TestTenantRepositoryLookupMisses(t *testing.T) {
	resetDB(t)
	repo := postgres.NewTenantRepository(testPool)
	_, revokedHash := createTenant(t, "acme")
	_, err := testPool.Exec(context.Background(), "UPDATE api_keys SET revoked_at = now() WHERE key_hash = $1", revokedHash)
	if err != nil {
		t.Fatalf("revoke key: %v", err)
	}

	for name, hash := range map[string][]byte{
		"unknown hash": make([]byte, 32),
		"revoked key":  revokedHash,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := repo.GetByAPIKeyHash(context.Background(), hash)
			if !errors.Is(err, webhook.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}
}
