package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/httpapi"
	"github.com/sonymuhamad/webhook-lab/mock"
)

const adminToken = "test-admin-token-at-least-32-characters"

type pingerFunc func(ctx context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

type testRouter struct {
	http.Handler
	tenants   *mock.MockTenantUsecase
	endpoints *mock.MockEndpointUsecase
	messages  *mock.MockMessageUsecase
}

func newTestRouter(t *testing.T) testRouter {
	t.Helper()
	ctrl := gomock.NewController(t)
	tenants := mock.NewMockTenantUsecase(ctrl)
	endpoints := mock.NewMockEndpointUsecase(ctrl)
	messages := mock.NewMockMessageUsecase(ctrl)
	healthy := pingerFunc(func(context.Context) error { return nil })

	return testRouter{
		Handler:   httpapi.NewRouter(config.Auth{AdminToken: adminToken}, healthy, tenants, endpoints, messages),
		tenants:   tenants,
		endpoints: endpoints,
		messages:  messages,
	}
}

func serve(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	return serveWithHeader(h, method, path, token, body, "", "")
}

// serveWithHeader sets one extra header; an empty value sets none.
func serveWithHeader(h http.Handler, method, path, token, body, header, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if value != "" {
		req.Header.Set(header, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body)
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}
