package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/enum"
)

const maxIdempotencyKeyLength = 255

type messageHandler struct {
	messages webhook.MessageUsecase
}

type createMessageRequest struct {
	EventType string          `json:"event_type" validate:"required,max=100"`
	Payload   json.RawMessage `json:"payload" validate:"required"`
}

type createMessageResponse struct {
	ID        uuid.UUID `json:"id"`
	EventType string    `json:"event_type"`
	CreatedAt time.Time `json:"created_at"`
}

type messageDetailResponse struct {
	ID         uuid.UUID          `json:"id"`
	EventType  string             `json:"event_type"`
	Payload    json.RawMessage    `json:"payload"`
	CreatedAt  time.Time          `json:"created_at"`
	Deliveries []deliveryResponse `json:"deliveries"`
}

type deliveryResponse struct {
	ID            uuid.UUID           `json:"id"`
	EndpointID    uuid.UUID           `json:"endpoint_id"`
	EndpointURL   string              `json:"endpoint_url"`
	Status        enum.DeliveryStatus `json:"status"`
	AttemptCount  int                 `json:"attempt_count"`
	NextAttemptAt time.Time           `json:"next_attempt_at"`
	Attempts      []attemptResponse   `json:"attempts"`
}

type attemptResponse struct {
	StatusCode *int      `json:"status_code"`
	Error      *string   `json:"error"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// create answers 202: the message is stored and queued, not yet delivered.
func (h *messageHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}

	var idempotencyKey *string
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		if len(key) > maxIdempotencyKeyLength {
			writeError(w, r, webhook.ValidationError{Message: "Idempotency-Key must be at most 255 characters"})
			return
		}
		idempotencyKey = &key
	}

	message, err := h.messages.Create(r.Context(), webhook.CreateMessageParam{
		TenantID:       tenantIDFrom(r.Context()),
		EventType:      req.EventType,
		Payload:        req.Payload,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusAccepted, createMessageResponse{
		ID:        message.ID,
		EventType: message.EventType,
		CreatedAt: message.CreatedAt,
	})
}

func (h *messageHandler) get(w http.ResponseWriter, r *http.Request) {
	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, webhook.ErrNotFound)
		return
	}

	detail, err := h.messages.Get(r.Context(), tenantIDFrom(r.Context()), messageID)
	if err != nil {
		writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toMessageDetailResponse(detail))
}

func toMessageDetailResponse(detail webhook.MessageDetail) messageDetailResponse {
	resp := messageDetailResponse{
		ID:         detail.Message.ID,
		EventType:  detail.Message.EventType,
		Payload:    detail.Message.Payload,
		CreatedAt:  detail.Message.CreatedAt,
		Deliveries: make([]deliveryResponse, len(detail.Deliveries)),
	}
	for i, dd := range detail.Deliveries {
		d := dd.Delivery
		resp.Deliveries[i] = deliveryResponse{
			ID:            d.ID,
			EndpointID:    d.EndpointID,
			EndpointURL:   d.EndpointURL,
			Status:        d.Status,
			AttemptCount:  d.AttemptCount,
			NextAttemptAt: d.NextAttemptAt,
			Attempts:      make([]attemptResponse, len(dd.Attempts)),
		}
		for j, a := range dd.Attempts {
			resp.Deliveries[i].Attempts[j] = attemptResponse{
				StatusCode: a.StatusCode,
				Error:      a.Error,
				DurationMs: a.Duration.Milliseconds(),
				CreatedAt:  a.CreatedAt,
			}
		}
	}
	return resp
}
