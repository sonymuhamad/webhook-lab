package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
)

const tenantAPIKey = "whl_TENANTKEYTENANTKEYTENANT"

func authenticatedRouter(t *testing.T) (testRouter, webhook.Tenant) {
	t.Helper()
	router := newTestRouter(t)
	tenant := webhook.Tenant{ID: uuid.New(), Name: "acme"}
	router.tenants.EXPECT().Authenticate(gomock.Any(), tenantAPIKey).Return(tenant, nil).AnyTimes()
	return router, tenant
}

// The tenant ID must come from the API key, never from the request body.
func TestCreateEndpointUsesAuthenticatedTenant(t *testing.T) {
	router, tenant := authenticatedRouter(t)
	created := webhook.Endpoint{ID: uuid.New(), TenantID: tenant.ID, URL: "https://example.com/hook"}
	router.endpoints.EXPECT().
		Create(gomock.Any(), webhook.CreateEndpointParam{TenantID: tenant.ID, URL: "https://example.com/hook"}).
		Return(created, nil)

	rec := serve(router, http.MethodPost, "/endpoints", tenantAPIKey, `{"url":"https://example.com/hook"}`)

	assertStatus(t, rec, http.StatusCreated)
	var body struct {
		ID  uuid.UUID `json:"id"`
		URL string    `json:"url"`
	}
	decode(t, rec, &body)
	if body.ID != created.ID || body.URL != created.URL {
		t.Errorf("body = %+v", body)
	}
}

func TestCreateEndpointRejectsTenantIDInBody(t *testing.T) {
	router, _ := authenticatedRouter(t)

	rec := serve(router, http.MethodPost, "/endpoints", tenantAPIKey,
		`{"url":"https://example.com/hook","tenant_id":"`+uuid.NewString()+`"}`)

	assertStatus(t, rec, http.StatusBadRequest)
}

func TestListEndpoints(t *testing.T) {
	tests := []struct {
		query    string
		wantPage webhook.Pagination
	}{
		{"", webhook.Pagination{Limit: 20, Offset: 0}},
		{"?limit=5&offset=10", webhook.Pagination{Limit: 5, Offset: 10}},
		{"?limit=100", webhook.Pagination{Limit: 100, Offset: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			router, tenant := authenticatedRouter(t)
			endpoint := webhook.Endpoint{ID: uuid.New(), TenantID: tenant.ID, URL: "https://example.com/hook"}
			router.endpoints.EXPECT().
				List(gomock.Any(), webhook.ListEndpointsParam{TenantID: tenant.ID, Pagination: tt.wantPage}).
				Return(webhook.ListEndpointsResult{Endpoints: []webhook.Endpoint{endpoint}, Total: 42}, nil)

			rec := serve(router, http.MethodGet, "/endpoints"+tt.query, tenantAPIKey, "")

			assertStatus(t, rec, http.StatusOK)
			var body struct {
				Data []struct {
					ID uuid.UUID `json:"id"`
				} `json:"data"`
				Pagination struct {
					Limit  int   `json:"limit"`
					Offset int   `json:"offset"`
					Total  int64 `json:"total"`
				} `json:"pagination"`
			}
			decode(t, rec, &body)
			if len(body.Data) != 1 || body.Data[0].ID != endpoint.ID {
				t.Errorf("data = %+v", body.Data)
			}
			if body.Pagination.Limit != tt.wantPage.Limit || body.Pagination.Offset != tt.wantPage.Offset || body.Pagination.Total != 42 {
				t.Errorf("pagination = %+v", body.Pagination)
			}
		})
	}
}

func TestListEndpointsRejectsInvalidPagination(t *testing.T) {
	for _, query := range []string{"?limit=0", "?limit=101", "?limit=abc", "?offset=-1", "?offset=1.5"} {
		t.Run(query, func(t *testing.T) {
			// No List EXPECT: an invalid page must be rejected before the usecase.
			router, _ := authenticatedRouter(t)

			rec := serve(router, http.MethodGet, "/endpoints"+query, tenantAPIKey, "")

			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}

func TestCreateEndpointRejectsInvalidURL(t *testing.T) {
	for _, url := range []string{"", "not-a-url", "ftp://example.com", "/relative/path", "https://"} {
		t.Run(url, func(t *testing.T) {
			// No Create EXPECT: an invalid URL must be rejected before the usecase.
			router, _ := authenticatedRouter(t)
			body, _ := json.Marshal(map[string]string{"url": url})

			rec := serve(router, http.MethodPost, "/endpoints", tenantAPIKey, string(body))

			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}
