//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres"
)

func TestEndpointRepositoryListIsScopedToTenant(t *testing.T) {
	resetDB(t)
	repo := postgres.NewEndpointRepository(testPool)
	tenantA, _ := createTenant(t, "tenant-a")
	tenantB, _ := createTenant(t, "tenant-b")

	_, err := repo.Create(context.Background(), webhook.CreateEndpointParam{TenantID: tenantA.ID, URL: "https://a.example.com"})
	if err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	got, err := repo.ListByTenant(context.Background(), webhook.ListEndpointsParam{
		TenantID:   tenantB.ID,
		Pagination: webhook.Pagination{Limit: 20},
	})
	if err != nil {
		t.Fatalf("ListByTenant: %v", err)
	}
	if got.Total != 0 || len(got.Endpoints) != 0 {
		t.Errorf("tenant B sees %+v, want nothing", got)
	}
}

func TestEndpointRepositoryListPagination(t *testing.T) {
	resetDB(t)
	repo := postgres.NewEndpointRepository(testPool)
	tenant, _ := createTenant(t, "acme")
	for i := range 3 {
		url := fmt.Sprintf("https://example.com/%d", i)
		if _, err := repo.Create(context.Background(), webhook.CreateEndpointParam{TenantID: tenant.ID, URL: url}); err != nil {
			t.Fatalf("create endpoint: %v", err)
		}
	}
	_, err := testPool.Exec(context.Background(), "UPDATE endpoints SET disabled_at = now() WHERE url = 'https://example.com/1'")
	if err != nil {
		t.Fatalf("disable endpoint: %v", err)
	}

	tests := []struct {
		name     string
		page     webhook.Pagination
		wantURLs []string
	}{
		{"all, disabled excluded", webhook.Pagination{Limit: 20}, []string{"https://example.com/0", "https://example.com/2"}},
		{"first page", webhook.Pagination{Limit: 1}, []string{"https://example.com/0"}},
		{"second page", webhook.Pagination{Limit: 1, Offset: 1}, []string{"https://example.com/2"}},
		{"past the end", webhook.Pagination{Limit: 20, Offset: 5}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListByTenant(context.Background(), webhook.ListEndpointsParam{TenantID: tenant.ID, Pagination: tt.page})
			if err != nil {
				t.Fatalf("ListByTenant: %v", err)
			}
			if got.Total != 2 {
				t.Errorf("total = %d, want 2", got.Total)
			}
			if len(got.Endpoints) != len(tt.wantURLs) {
				t.Fatalf("got %d endpoints, want %d", len(got.Endpoints), len(tt.wantURLs))
			}
			for i, want := range tt.wantURLs {
				if got.Endpoints[i].URL != want {
					t.Errorf("endpoints[%d].url = %q, want %q", i, got.Endpoints[i].URL, want)
				}
			}
		})
	}
}
