// Command receiver is a fake webhook endpoint for local runs and labs. It can
// be told to answer slowly or to fail a share of requests, and it counts how
// many deliveries arrived and how many distinct messages they carried.
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

type stats struct {
	mu       sync.Mutex
	received int
	failed   int
	seen     map[string]int
}

type statsResponse struct {
	Received   int `json:"received"`
	Failed     int `json:"failed"`
	Unique     int `json:"unique"`
	Duplicates int `json:"duplicates"`
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	latency := flag.Duration("latency", 0, "delay before answering each delivery")
	failRate := flag.Float64("fail-rate", 0, "share of deliveries answered with -fail-status, 0 to 1")
	failStatus := flag.Int("fail-status", http.StatusInternalServerError, "status code for failed deliveries")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := &stats{seen: make(map[string]int)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.snapshot())
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(*latency)
		messageID := r.Header.Get("webhook-id")

		if rand.Float64() < *failRate {
			s.recordFailure()
			logger.Info("delivery rejected", "webhook_id", messageID, "status", *failStatus)
			w.WriteHeader(*failStatus)
			return
		}

		// Only accepted deliveries count towards unique and duplicates: a
		// rejected one is expected to come back.
		s.recordSuccess(messageID)
		logger.Info("delivery accepted", "webhook_id", messageID, "path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	logger.Info("receiver listening", "addr", *addr, "latency", latency.String(), "fail_rate", *failRate)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		logger.Error("receiver stopped", "error", err)
		os.Exit(1)
	}
}

func (s *stats) recordSuccess(messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
	s.seen[messageID]++
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
		Unique:     len(s.seen),
		Duplicates: s.received - len(s.seen),
	}
}
