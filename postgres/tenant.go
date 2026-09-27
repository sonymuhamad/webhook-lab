package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

type TenantRepository struct {
	pool *pgxpool.Pool
}

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{pool: pool}
}

func (r *TenantRepository) Create(ctx context.Context, name string) (webhook.Tenant, error) {
	row, err := queries(ctx, r.pool).CreateTenant(ctx, name)
	if err != nil {
		return webhook.Tenant{}, fmt.Errorf("insert tenant: %w", err)
	}
	return toTenant(row), nil
}

func (r *TenantRepository) CreateAPIKey(ctx context.Context, param webhook.CreateAPIKeyParam) error {
	err := queries(ctx, r.pool).CreateAPIKey(ctx, sqlcgen.CreateAPIKeyParams{
		TenantID: param.TenantID,
		KeyHash:  param.Hash,
		Prefix:   param.Prefix,
	})
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	return nil
}

func (r *TenantRepository) GetByAPIKeyHash(ctx context.Context, hash []byte) (webhook.Tenant, error) {
	row, err := queries(ctx, r.pool).GetTenantByAPIKeyHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return webhook.Tenant{}, webhook.ErrNotFound
	}
	if err != nil {
		return webhook.Tenant{}, fmt.Errorf("select tenant by api key: %w", err)
	}
	return toTenant(row), nil
}

func toTenant(row sqlcgen.Tenant) webhook.Tenant {
	return webhook.Tenant{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}
}
