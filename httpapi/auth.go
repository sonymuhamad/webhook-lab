package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
)

type tenantIDKey struct{}

func requireAdminToken(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			// Constant-time so response timing does not reveal how many
			// leading bytes of a guess were correct.
			if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
				writeError(w, r, webhook.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requireAPIKey(tenants webhook.TenantUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey, ok := bearerToken(r)
			if !ok {
				writeError(w, r, webhook.ErrUnauthorized)
				return
			}

			tenant, err := tenants.Authenticate(r.Context(), apiKey)
			if err != nil {
				writeError(w, r, err)
				return
			}

			ctx := context.WithValue(r.Context(), tenantIDKey{}, tenant.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// tenantIDFrom panics when called outside a route guarded by requireAPIKey;
// that is a routing bug, not a request error.
func tenantIDFrom(ctx context.Context) uuid.UUID {
	return ctx.Value(tenantIDKey{}).(uuid.UUID)
}

func bearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token, ok && token != ""
}
