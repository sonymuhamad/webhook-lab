package webhook

//go:generate go tool mockgen -source=endpoint.go -destination=mock/endpoint.go -package=mock

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Endpoint struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	URL       string
	CreatedAt time.Time
}

type CreateEndpointParam struct {
	TenantID uuid.UUID
	URL      string
}

type ListEndpointsParam struct {
	TenantID   uuid.UUID
	Pagination Pagination
}

type ListEndpointsResult struct {
	Endpoints []Endpoint
	Total     int64
}

type EndpointUsecase interface {
	Create(ctx context.Context, param CreateEndpointParam) (Endpoint, error)
	List(ctx context.Context, param ListEndpointsParam) (ListEndpointsResult, error)
}

type EndpointRepository interface {
	Create(ctx context.Context, param CreateEndpointParam) (Endpoint, error)
	ListByTenant(ctx context.Context, param ListEndpointsParam) (ListEndpointsResult, error)
}
