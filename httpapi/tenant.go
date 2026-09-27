package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
)

type tenantHandler struct {
	tenants webhook.TenantUsecase
}

type createTenantRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

type createTenantResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *tenantHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	result, err := h.tenants.Create(r.Context(), webhook.CreateTenantParam{Name: req.Name})
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, createTenantResponse{
		ID:        result.Tenant.ID,
		Name:      result.Tenant.Name,
		APIKey:    result.APIKey,
		CreatedAt: result.Tenant.CreatedAt,
	})
}
