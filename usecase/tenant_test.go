package usecase_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/mock"
	"github.com/sonymuhamad/webhook-lab/usecase"
)

func TestTenantCreate(t *testing.T) {
	repo := mock.NewMockTenantRepository(gomock.NewController(t))
	tenant := webhook.Tenant{ID: uuid.New(), Name: "acme"}
	repo.EXPECT().Create(gomock.Any(), "acme").Return(tenant, nil)

	var stored webhook.CreateAPIKeyParam
	repo.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, param webhook.CreateAPIKeyParam) error {
			stored = param
			return nil
		})

	result, err := usecase.NewTenant(repo, passthroughTx(t)).Create(context.Background(), webhook.CreateTenantParam{Name: "  acme  "})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if result.Tenant != tenant {
		t.Errorf("tenant = %+v, want %+v", result.Tenant, tenant)
	}
	if stored.TenantID != tenant.ID {
		t.Errorf("api key stored for tenant %s, want %s", stored.TenantID, tenant.ID)
	}
	if !strings.HasPrefix(result.APIKey, "whl_") {
		t.Errorf("api key %q is missing the whl_ prefix", result.APIKey)
	}
	// The repository must receive the hash, never the plaintext key.
	wantHash := sha256.Sum256([]byte(result.APIKey))
	if string(stored.Hash) != string(wantHash[:]) {
		t.Error("stored hash is not the SHA-256 of the returned key")
	}
	if !strings.HasPrefix(result.APIKey, stored.Prefix) || len(stored.Prefix) >= len(result.APIKey) {
		t.Errorf("stored prefix %q must be a strict prefix of the key", stored.Prefix)
	}
}

func TestTenantCreateRejectsBlankName(t *testing.T) {
	// No EXPECT: the repository must not be called.
	repo := mock.NewMockTenantRepository(gomock.NewController(t))

	_, err := usecase.NewTenant(repo, passthroughTx(t)).Create(context.Background(), webhook.CreateTenantParam{Name: "   "})

	var validationErr webhook.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestTenantCreateStopsWhenTenantInsertFails(t *testing.T) {
	repo := mock.NewMockTenantRepository(gomock.NewController(t))
	dbErr := errors.New("connection reset")
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(webhook.Tenant{}, dbErr)
	// No CreateAPIKey EXPECT: a key must not be written for a tenant that was not created.

	_, err := usecase.NewTenant(repo, passthroughTx(t)).Create(context.Background(), webhook.CreateTenantParam{Name: "acme"})

	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, dbErr)
	}
}

func TestTenantAuthenticate(t *testing.T) {
	const apiKey = "whl_VALIDKEYVALIDKEYVALIDKEY"
	tenant := webhook.Tenant{ID: uuid.New(), Name: "acme"}
	hash := sha256.Sum256([]byte(apiKey))

	tests := []struct {
		name      string
		apiKey    string
		setupRepo func(repo *mock.MockTenantRepository)
		want      webhook.Tenant
		wantErr   error
	}{
		{
			name:   "valid key",
			apiKey: apiKey,
			setupRepo: func(repo *mock.MockTenantRepository) {
				repo.EXPECT().GetByAPIKeyHash(gomock.Any(), hash[:]).Return(tenant, nil)
			},
			want: tenant,
		},
		{
			name:      "key without prefix never reaches the database",
			apiKey:    "sk_live_something",
			setupRepo: func(*mock.MockTenantRepository) {},
			wantErr:   webhook.ErrUnauthorized,
		},
		{
			name:   "unknown key",
			apiKey: apiKey,
			setupRepo: func(repo *mock.MockTenantRepository) {
				repo.EXPECT().GetByAPIKeyHash(gomock.Any(), gomock.Any()).Return(webhook.Tenant{}, webhook.ErrNotFound)
			},
			wantErr: webhook.ErrUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mock.NewMockTenantRepository(gomock.NewController(t))
			tt.setupRepo(repo)

			got, err := usecase.NewTenant(repo, passthroughTx(t)).Authenticate(context.Background(), tt.apiKey)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("tenant = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A database failure must surface as an internal error, not as a 401 that
// would tell a valid tenant its key is wrong.
func TestTenantAuthenticateDoesNotMaskRepositoryError(t *testing.T) {
	repo := mock.NewMockTenantRepository(gomock.NewController(t))
	dbErr := errors.New("connection reset")
	repo.EXPECT().GetByAPIKeyHash(gomock.Any(), gomock.Any()).Return(webhook.Tenant{}, dbErr)

	_, err := usecase.NewTenant(repo, passthroughTx(t)).Authenticate(context.Background(), "whl_VALIDKEYVALIDKEYVALIDKEY")

	if !errors.Is(err, dbErr) || errors.Is(err, webhook.ErrUnauthorized) {
		t.Fatalf("err = %v, want it to wrap %v and not be ErrUnauthorized", err, dbErr)
	}
}
