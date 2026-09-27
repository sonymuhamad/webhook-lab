package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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
	r.Use(middleware.Recoverer)

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

	return r
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
	switch {
	case errors.As(err, &validationErr):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: validationErr.Message})
	case errors.Is(err, webhook.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
	case errors.Is(err, webhook.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	default:
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
