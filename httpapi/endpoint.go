package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
)

type endpointHandler struct {
	endpoints webhook.EndpointUsecase
}

type createEndpointRequest struct {
	URL string `json:"url" validate:"required,http_url,max=2048"`
}

type endpointResponse struct {
	ID        uuid.UUID `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type listEndpointsResponse struct {
	Data       []endpointResponse `json:"data"`
	Pagination paginationResponse `json:"pagination"`
}

func (h *endpointHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createEndpointRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	endpoint, err := h.endpoints.Create(r.Context(), webhook.CreateEndpointParam{
		TenantID: tenantIDFrom(r.Context()),
		URL:      req.URL,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toEndpointResponse(endpoint))
}

func (h *endpointHandler) list(w http.ResponseWriter, r *http.Request) {
	page, err := parsePagination(r)
	if err != nil {
		writeError(w, r, err)
		return
	}

	result, err := h.endpoints.List(r.Context(), webhook.ListEndpointsParam{
		TenantID:   tenantIDFrom(r.Context()),
		Pagination: page,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	resp := listEndpointsResponse{
		Data:       make([]endpointResponse, len(result.Endpoints)),
		Pagination: paginationResponse{Limit: page.Limit, Offset: page.Offset, Total: result.Total},
	}
	for i, endpoint := range result.Endpoints {
		resp.Data[i] = toEndpointResponse(endpoint)
	}
	writeJSON(w, http.StatusOK, resp)
}

func toEndpointResponse(e webhook.Endpoint) endpointResponse {
	return endpointResponse{ID: e.ID, URL: e.URL, CreatedAt: e.CreatedAt}
}
