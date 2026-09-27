package sender_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/sender"
)

func TestSendPostsPayloadWithWebhookHeaders(t *testing.T) {
	var got *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	req := webhook.SendRequest{
		URL:       srv.URL + "/hook",
		MessageID: uuid.New(),
		EventType: "booking.created",
		Payload:   json.RawMessage(`{"booking_id":"b_1"}`),
	}
	result, err := sender.NewHTTP(config.Delivery{Timeout: time.Second}).Send(context.Background(), req)

	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", result.StatusCode)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/hook" {
		t.Errorf("request = %s %s", got.Method, got.URL.Path)
	}
	if string(gotBody) != `{"booking_id":"b_1"}` {
		t.Errorf("body = %s", gotBody)
	}
	if got.Header.Get("webhook-id") != req.MessageID.String() {
		t.Errorf("webhook-id = %q, want the message id", got.Header.Get("webhook-id"))
	}
	if got.Header.Get("webhook-timestamp") == "" || got.Header.Get("webhook-event-type") != "booking.created" {
		t.Errorf("headers = %v", got.Header)
	}
}

func TestSendReturnsErrorResponsesAsResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	result, err := sender.NewHTTP(config.Delivery{Timeout: time.Second}).Send(context.Background(), webhook.SendRequest{URL: srv.URL})

	if err != nil {
		t.Fatalf("a 503 is a response, not a send error: %v", err)
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", result.StatusCode)
	}
}

func TestSendDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	t.Cleanup(target.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)

	result, err := sender.NewHTTP(config.Delivery{Timeout: time.Second}).Send(context.Background(), webhook.SendRequest{URL: srv.URL})

	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.StatusCode != http.StatusTemporaryRedirect || redirected {
		t.Errorf("status = %d, redirected = %v; want 307 and no redirect", result.StatusCode, redirected)
	}
}

func TestSendTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })

	result, err := sender.NewHTTP(config.Delivery{Timeout: 50 * time.Millisecond}).Send(context.Background(), webhook.SendRequest{URL: srv.URL})

	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if result.Duration < 50*time.Millisecond {
		t.Errorf("duration = %s, want at least the timeout", result.Duration)
	}
}
