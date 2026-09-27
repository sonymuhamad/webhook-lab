// Command receiver is a fake webhook endpoint for local runs and labs. It can
// be told to answer slowly or to fail a share of requests, and it counts how
// many deliveries it accepted and how many of those were the same delivery
// accepted again.
//
//	go run ./cmd/receiver -addr :9000 -latency 200ms -fail-rate 0.2
//	curl localhost:9000/stats
package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"sync"
	"time"
)

// Duplicates are counted per webhook-delivery-id, not per webhook-id: one
// message fans out to several endpoints under the same webhook-id, and those
// sends are distinct deliveries even when they all reach this one receiver.
type stats struct {
	mu         sync.Mutex
	received   int
	failed     int
	deliveries map[string]int
	messages   map[string]struct{}
}

type statsResponse struct {
	Received   int `json:"received"`
	Failed     int `json:"failed"`
	Unique     int `json:"unique"`
	Duplicates int `json:"duplicates"`
	Messages   int `json:"messages"`
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	latency := flag.Duration("latency", 0, "delay before answering each delivery")
	failRate := flag.Float64("fail-rate", 0, "share of deliveries answered with -fail-status, 0 to 1")
	failStatus := flag.Int("fail-status", http.StatusInternalServerError, "status code for failed deliveries")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := &stats{deliveries: make(map[string]int), messages: make(map[string]struct{})}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.snapshot())
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(*latency)
		messageID := r.Header.Get("webhook-id")
		deliveryID := r.Header.Get("webhook-delivery-id")

		if rand.Float64() < *failRate {
			s.recordFailure()
			logger.Info("delivery rejected", "webhook_id", messageID, "delivery_id", deliveryID, "status", *failStatus)
			w.WriteHeader(*failStatus)
			return
		}

		// Only accepted deliveries count towards unique and duplicates: a
		// rejected one is expected to come back.
		s.recordSuccess(messageID, deliveryID)
		logger.Info("delivery accepted", "webhook_id", messageID, "delivery_id", deliveryID, "path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	logger.Info("receiver listening", "addr", *addr, "latency", latency.String(), "fail_rate", *failRate)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		logger.Error("receiver stopped", "error", err)
		os.Exit(1)
	}
}

func (s *stats) recordSuccess(messageID, deliveryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
	s.deliveries[deliveryID]++
	s.messages[messageID] = struct{}{}
}

func (s *stats) recordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed++
}

func (s *stats) snapshot() statsResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return statsResponse{
		Received:   s.received,
		Failed:     s.failed,
		Unique:     len(s.deliveries),
		Duplicates: s.received - len(s.deliveries),
		Messages:   len(s.messages),
	}
}
