package usecase

import (
	"context"
	"fmt"

	webhook "github.com/sonymuhamad/webhook-lab"
)

type Endpoint struct {
	repo webhook.EndpointRepository
}

func NewEndpoint(repo webhook.EndpointRepository) *Endpoint {
	return &Endpoint{repo: repo}
}

// Create does not block localhost or private addresses, because the lab's
// fake receiver runs locally. A public deployment would need to, to prevent
// SSRF.
func (u *Endpoint) Create(ctx context.Context, param webhook.CreateEndpointParam) (webhook.Endpoint, error) {
	endpoint, err := u.repo.Create(ctx, param)
	if err != nil {
		return webhook.Endpoint{}, fmt.Errorf("create endpoint: %w", err)
	}
	return endpoint, nil
}

func (u *Endpoint) List(ctx context.Context, param webhook.ListEndpointsParam) (webhook.ListEndpointsResult, error) {
	result, err := u.repo.ListByTenant(ctx, param)
	if err != nil {
		return webhook.ListEndpointsResult{}, fmt.Errorf("list endpoints: %w", err)
	}
	return result, nil
}
