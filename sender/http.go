package sender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
)

// Read and discard at most this much of a response body, so the connection
// can be reused without letting a receiver stream an unbounded reply.
const maxDrainBytes = 64 << 10

// HTTP implements webhook.Sender with a plain POST.
type HTTP struct {
	client *http.Client
}

func NewHTTP(cfg config.Delivery) *HTTP {
	return &HTTP{client: &http.Client{
		Timeout: cfg.Timeout,
		// A redirect is treated as the endpoint's final answer and fails the
		// attempt: following it would send the payload to a URL the tenant
		// never registered.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// Send posts the payload with the Standard Webhooks id and timestamp headers.
// webhook-id is the message ID: the same for every endpoint and every retry,
// so receivers dedupe events on it. webhook-delivery-id is not part of the
// standard; it names one message-to-endpoint delivery, stays the same across
// that delivery's retries, and exists for tracing a single send.
func (s *HTTP) Send(ctx context.Context, req webhook.SendRequest) (webhook.SendResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, bytes.NewReader(req.Payload))
	if err != nil {
		return webhook.SendResult{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("webhook-id", req.MessageID.String())
	httpReq.Header.Set("webhook-delivery-id", req.DeliveryID.String())
	httpReq.Header.Set("webhook-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	httpReq.Header.Set("webhook-event-type", req.EventType)

	start := time.Now()
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return webhook.SendResult{Duration: time.Since(start)}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes)) // the status code is the whole answer

	return webhook.SendResult{StatusCode: resp.StatusCode, Duration: time.Since(start)}, nil
}
