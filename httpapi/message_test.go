package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/enum"
)

func TestCreateMessage(t *testing.T) {
	router, tenant := authenticatedRouter(t)
	message := webhook.Message{ID: uuid.New(), TenantID: tenant.ID, EventType: "booking.created", CreatedAt: time.Now().UTC()}
	router.messages.EXPECT().
		Create(gomock.Any(), webhook.CreateMessageParam{
			TenantID:  tenant.ID,
			EventType: "booking.created",
			Payload:   json.RawMessage(`{"booking_id":"b_1"}`),
		}).
		Return(message, nil)

	rec := serve(router, http.MethodPost, "/messages", tenantAPIKey,
		`{"event_type":"booking.created","payload":{"booking_id":"b_1"}}`)

	assertStatus(t, rec, http.StatusAccepted)
	var body struct {
		ID uuid.UUID `json:"id"`
	}
	decode(t, rec, &body)
	if body.ID != message.ID {
		t.Errorf("id = %s, want %s", body.ID, message.ID)
	}
}

func TestCreateMessagePassesIdempotencyKey(t *testing.T) {
	router, _ := authenticatedRouter(t)
	router.messages.EXPECT().
		Create(gomock.Any(), gomock.Cond(func(p webhook.CreateMessageParam) bool {
			return p.IdempotencyKey != nil && *p.IdempotencyKey == "order-42"
		})).
		Return(webhook.Message{ID: uuid.New()}, nil)

	req := `{"event_type":"booking.created","payload":{}}`
	rec := serveWithHeader(router, http.MethodPost, "/messages", tenantAPIKey, req, "Idempotency-Key", "order-42")

	assertStatus(t, rec, http.StatusAccepted)
}

func TestCreateMessageRejectsBadRequest(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		header string
	}{
		{"missing event_type", `{"payload":{}}`, ""},
		{"missing payload", `{"event_type":"booking.created"}`, ""},
		{"event_type too long", `{"event_type":"` + strings.Repeat("a", 101) + `","payload":{}}`, ""},
		{"idempotency key too long", `{"event_type":"booking.created","payload":{}}`, strings.Repeat("k", 256)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No Create EXPECT: a bad request must be rejected before the usecase.
			router, _ := authenticatedRouter(t)

			rec := serveWithHeader(router, http.MethodPost, "/messages", tenantAPIKey, tt.body, "Idempotency-Key", tt.header)

			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}

func TestGetMessage(t *testing.T) {
	router, tenant := authenticatedRouter(t)
	statusCode := 503
	detail := webhook.MessageDetail{
		Message: webhook.Message{ID: uuid.New(), EventType: "booking.created", Payload: json.RawMessage(`{"a":1}`)},
		Deliveries: []webhook.DeliveryDetail{{
			Delivery: webhook.Delivery{ID: uuid.New(), EndpointURL: "https://example.com/hook", Status: enum.DeliveryStatusPending, AttemptCount: 1},
			Attempts: []webhook.Attempt{{StatusCode: &statusCode, Duration: 120 * time.Millisecond}},
		}},
	}
	router.messages.EXPECT().Get(gomock.Any(), tenant.ID, detail.Message.ID).Return(detail, nil)

	rec := serve(router, http.MethodGet, "/messages/"+detail.Message.ID.String(), tenantAPIKey, "")

	assertStatus(t, rec, http.StatusOK)
	var body struct {
		Payload    json.RawMessage `json:"payload"`
		Deliveries []struct {
			Status   string `json:"status"`
			Attempts []struct {
				StatusCode *int  `json:"status_code"`
				DurationMs int64 `json:"duration_ms"`
			} `json:"attempts"`
		} `json:"deliveries"`
	}
	decode(t, rec, &body)
	if string(body.Payload) != `{"a":1}` {
		t.Errorf("payload = %s", body.Payload)
	}
	if len(body.Deliveries) != 1 || body.Deliveries[0].Status != "pending" {
		t.Fatalf("deliveries = %+v", body.Deliveries)
	}
	attempts := body.Deliveries[0].Attempts
	if len(attempts) != 1 || attempts[0].StatusCode == nil || *attempts[0].StatusCode != 503 || attempts[0].DurationMs != 120 {
		t.Errorf("attempts = %+v", attempts)
	}
}

func TestGetMessageNotFound(t *testing.T) {
	t.Run("malformed id", func(t *testing.T) {
		router, _ := authenticatedRouter(t)

		rec := serve(router, http.MethodGet, "/messages/not-a-uuid", tenantAPIKey, "")

		assertStatus(t, rec, http.StatusNotFound)
	})
	t.Run("unknown or other tenant's id", func(t *testing.T) {
		router, _ := authenticatedRouter(t)
		router.messages.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(webhook.MessageDetail{}, webhook.ErrNotFound)

		rec := serve(router, http.MethodGet, "/messages/"+uuid.NewString(), tenantAPIKey, "")

		assertStatus(t, rec, http.StatusNotFound)
	})
}
