package httpapi_test

import (
	"errors"
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
)

func TestAdminRoutesRejectBadToken(t *testing.T) {
	for name, token := range map[string]string{
		"missing":        "",
		"wrong":          "wrong-token",
		"tenant api key": "whl_VALIDKEYVALIDKEYVALIDKEY",
	} {
		t.Run(name, func(t *testing.T) {
			router := newTestRouter(t)

			rec := serve(router, http.MethodPost, "/admin/tenants", token, `{"name":"acme"}`)

			assertStatus(t, rec, http.StatusUnauthorized)
		})
	}
}

func TestTenantRoutesRejectMissingKey(t *testing.T) {
	router := newTestRouter(t)

	rec := serve(router, http.MethodGet, "/endpoints", "", "")

	assertStatus(t, rec, http.StatusUnauthorized)
}

func TestTenantRoutesMapAuthenticateErrors(t *testing.T) {
	tests := []struct {
		name    string
		authErr error
		want    int
	}{
		{"rejected key", webhook.ErrUnauthorized, http.StatusUnauthorized},
		{"database failure", errors.New("connection reset"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouter(t)
			router.tenants.EXPECT().Authenticate(gomock.Any(), "whl_SOMEKEY").Return(webhook.Tenant{}, tt.authErr)

			rec := serve(router, http.MethodGet, "/endpoints", "whl_SOMEKEY", "")

			assertStatus(t, rec, tt.want)
		})
	}
}
