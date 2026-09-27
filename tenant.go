package webhook

//go:generate go tool mockgen -source=tenant.go -destination=mock/tenant.go -package=mock

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

type CreateTenantParam struct {
	Name string
}

type CreateTenantResult struct {
	Tenant Tenant
	// APIKey is the plaintext key. Only its hash is stored, so this is the
	// one time it can be shown to the tenant.
	APIKey string
}

type CreateAPIKeyParam struct {
	TenantID uuid.UUID
	Hash     []byte
	Prefix   string
}

type TenantUsecase interface {
	Create(ctx context.Context, param CreateTenantParam) (CreateTenantResult, error)
	Authenticate(ctx context.Context, apiKey string) (Tenant, error)
}

type TenantRepository interface {
	Create(ctx context.Context, name string) (Tenant, error)
	CreateAPIKey(ctx context.Context, param CreateAPIKeyParam) error
	GetByAPIKeyHash(ctx context.Context, hash []byte) (Tenant, error)
}
