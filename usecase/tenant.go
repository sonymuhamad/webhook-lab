package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	webhook "github.com/sonymuhamad/webhook-lab"
)

const (
	// A fixed prefix lets secret scanners recognise a leaked key, the same way
	// GitHub (ghp_) and Stripe (sk_live_) do.
	apiKeyPrefix = "whl_"
	// Stored and logged in place of the key so one can be identified without
	// revealing it.
	apiKeyDisplayLength = len(apiKeyPrefix) + 8
)

type Tenant struct {
	repo webhook.TenantRepository
	tx   webhook.Transactor
}

func NewTenant(repo webhook.TenantRepository, tx webhook.Transactor) *Tenant {
	return &Tenant{repo: repo, tx: tx}
}

// Create stores the tenant and its first API key in one transaction, so a
// tenant never exists without a way to authenticate.
//
// Names are unique case-insensitively, checked here rather than by a database
// constraint. Two concurrent requests with the same name can both pass the
// check; that is accepted for an admin-only endpoint.
func (u *Tenant) Create(ctx context.Context, param webhook.CreateTenantParam) (webhook.CreateTenantResult, error) {
	name := strings.TrimSpace(param.Name)
	if name == "" {
		return webhook.CreateTenantResult{}, webhook.ValidationError{Message: "name must not be blank"}
	}

	taken, err := u.repo.NameExists(ctx, name)
	if err != nil {
		return webhook.CreateTenantResult{}, fmt.Errorf("create tenant: %w", err)
	}
	if taken {
		return webhook.CreateTenantResult{}, webhook.ConflictError{Message: fmt.Sprintf("tenant name %q is already taken", name)}
	}

	// rand.Text returns 26 base32 characters, i.e. 128 bits of entropy.
	apiKey := apiKeyPrefix + rand.Text()

	var tenant webhook.Tenant
	err = u.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		tenant, err = u.repo.Create(ctx, name)
		if err != nil {
			return err
		}
		return u.repo.CreateAPIKey(ctx, webhook.CreateAPIKeyParam{
			TenantID: tenant.ID,
			Hash:     hashAPIKey(apiKey),
			Prefix:   apiKey[:apiKeyDisplayLength],
		})
	})
	if err != nil {
		return webhook.CreateTenantResult{}, fmt.Errorf("create tenant: %w", err)
	}
	return webhook.CreateTenantResult{Tenant: tenant, APIKey: apiKey}, nil
}

// Authenticate returns webhook.ErrUnauthorized for a malformed, unknown, or
// revoked key, without saying which.
func (u *Tenant) Authenticate(ctx context.Context, apiKey string) (webhook.Tenant, error) {
	if !strings.HasPrefix(apiKey, apiKeyPrefix) {
		return webhook.Tenant{}, webhook.ErrUnauthorized
	}

	tenant, err := u.repo.GetByAPIKeyHash(ctx, hashAPIKey(apiKey))
	if errors.Is(err, webhook.ErrNotFound) {
		return webhook.Tenant{}, webhook.ErrUnauthorized
	}
	if err != nil {
		return webhook.Tenant{}, fmt.Errorf("authenticate tenant: %w", err)
	}
	return tenant, nil
}

// hashAPIKey uses a plain SHA-256 rather than bcrypt: the key is random with
// 128 bits of entropy, so a slow hash adds no protection, and a deterministic
// hash can be looked up by index on every request.
func hashAPIKey(apiKey string) []byte {
	sum := sha256.Sum256([]byte(apiKey))
	return sum[:]
}
