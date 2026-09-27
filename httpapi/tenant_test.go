package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
)

func TestCreateTenant(t *testing.T) {
	router := newTestRouter(t)
	tenant := webhook.Tenant{ID: uuid.New(), Name: "acme", CreatedAt: time.Now().UTC()}
	router.tenants.EXPECT().
		Create(gomock.Any(), webhook.CreateTenantParam{Name: "acme"}).
		Return(webhook.CreateTenantResult{Tenant: tenant, APIKey: "whl_NEWKEY"}, nil)

	rec := serve(router, http.MethodPost, "/admin/tenants", adminToken, `{"name":"acme"}`)

	assertStatus(t, rec, http.StatusCreated)
	var body struct {
		ID     uuid.UUID `json:"id"`
		Name   string    `json:"name"`
		APIKey string    `json:"api_key"`
	}
	decode(t, rec, &body)
	if body.ID != tenant.ID || body.Name != "acme" || body.APIKey != "whl_NEWKEY" {
		t.Errorf("body = %+v", body)
	}
}

func TestCreateTenantRejectsBadRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{"name":`},
		{"unknown field", `{"name":"acme","plan":"pro"}`},
		{"missing name", `{}`},
		{"name too long", `{"name":"` + strings.Repeat("a", 101) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No Create EXPECT: a bad body must be rejected before the usecase.
			router := newTestRouter(t)

			rec := serve(router, http.MethodPost, "/admin/tenants", adminToken, tt.body)

			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}

func TestCreateTenantMapsValidationError(t *testing.T) {
	router := newTestRouter(t)
	router.tenants.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(webhook.CreateTenantResult{}, webhook.ValidationError{Message: "name must not be blank"})

	rec := serve(router, http.MethodPost, "/admin/tenants", adminToken, `{"name":"   "}`)

	assertStatus(t, rec, http.StatusBadRequest)
	var body struct {
		Error string `json:"error"`
	}
	decode(t, rec, &body)
	if body.Error != "name must not be blank" {
		t.Errorf("error = %q, want the validation message", body.Error)
	}
}
