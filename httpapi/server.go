package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
)

func NewRouter(
	auth config.Auth,
	db Pinger,
	tenants webhook.TenantUsecase,
	endpoints webhook.EndpointUsecase,
	messages webhook.MessageUsecase,
) http.Handler {
	health := &healthHandler{db: db}
	tenant := &tenantHandler{tenants: tenants}
	endpoint := &endpointHandler{endpoints: endpoints}
	message := &messageHandler{messages: messages}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer, labelRoute)

	r.Get("/healthz", health.check)

	r.Route("/admin", func(r chi.Router) {
		r.Use(requireAdminToken(auth.AdminToken))
		r.Post("/tenants", tenant.create)
	})

	r.Group(func(r chi.Router) {
		r.Use(requireAPIKey(tenants))
		r.Post("/endpoints", endpoint.create)
		r.Get("/endpoints", endpoint.list)
		r.Post("/messages", message.create)
		r.Get("/messages/{id}", message.get)
	})

	return otelhttp.NewHandler(r, "webhook-api", otelhttp.WithFilter(func(r *http.Request) bool {
		return r.URL.Path != "/healthz"
	}))
}

// labelRoute adds the matched chi pattern, such as /messages/{id}, to the
// request metrics. Without it every message ID would become its own series.
func labelRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(semconv.HTTPRoute(chi.RouteContext(r.Context()).RoutePattern()))
		}
	})
}

func NewServer(cfg config.HTTP, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write json response", "error", err)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

// writeError maps domain errors to HTTP statuses. Anything unrecognised is
// logged and returned as a generic 500 so internal details do not leak.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr webhook.ValidationError
	var conflictErr webhook.ConflictError
	switch {
	case errors.As(err, &validationErr):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: validationErr.Message})
	case errors.As(err, &conflictErr):
		writeJSON(w, http.StatusConflict, errorResponse{Error: conflictErr.Message})
	case errors.Is(err, webhook.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
	case errors.Is(err, webhook.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	default:
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
