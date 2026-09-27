package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/httpapi"
)

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name    string
		pingErr error
		want    int
	}{
		{"database reachable", nil, http.StatusOK},
		{"database down", errors.New("connection refused"), http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := pingerFunc(func(context.Context) error { return tt.pingErr })
			router := httpapi.NewRouter(config.Auth{AdminToken: adminToken}, db, nil, nil, nil)

			rec := serve(router, http.MethodGet, "/healthz", "", "")

			assertStatus(t, rec, tt.want)
		})
	}
}
