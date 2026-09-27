package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/postgres/sqlcgen"
)

type EndpointRepository struct {
	pool *pgxpool.Pool
}

func NewEndpointRepository(pool *pgxpool.Pool) *EndpointRepository {
	return &EndpointRepository{pool: pool}
}

func (r *EndpointRepository) Create(ctx context.Context, param webhook.CreateEndpointParam) (webhook.Endpoint, error) {
	row, err := queries(ctx, r.pool).CreateEndpoint(ctx, sqlcgen.CreateEndpointParams{
		TenantID: param.TenantID,
		URL:      param.URL,
	})
	if err != nil {
		return webhook.Endpoint{}, fmt.Errorf("insert endpoint: %w", err)
	}
	return webhook.Endpoint{ID: row.ID, TenantID: row.TenantID, URL: row.URL, CreatedAt: row.CreatedAt}, nil
}

// ListByTenant runs the page and the count as two statements outside a
// transaction, so Total can be off by an endpoint created in between.
func (r *EndpointRepository) ListByTenant(ctx context.Context, param webhook.ListEndpointsParam) (webhook.ListEndpointsResult, error) {
	q := queries(ctx, r.pool)
	rows, err := q.ListEndpointsByTenant(ctx, sqlcgen.ListEndpointsByTenantParams{
		TenantID: param.TenantID,
		Limit:    int32(param.Pagination.Limit),
		Offset:   int32(param.Pagination.Offset),
	})
	if err != nil {
		return webhook.ListEndpointsResult{}, fmt.Errorf("select endpoints: %w", err)
	}
	total, err := q.CountEndpointsByTenant(ctx, param.TenantID)
	if err != nil {
		return webhook.ListEndpointsResult{}, fmt.Errorf("count endpoints: %w", err)
	}

	endpoints := make([]webhook.Endpoint, len(rows))
	for i, row := range rows {
		endpoints[i] = webhook.Endpoint{ID: row.ID, TenantID: row.TenantID, URL: row.URL, CreatedAt: row.CreatedAt}
	}
	return webhook.ListEndpointsResult{Endpoints: endpoints, Total: total}, nil
}
