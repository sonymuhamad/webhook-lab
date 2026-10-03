// Command receiver is a fake webhook endpoint for local runs and labs. It can
// be told to answer slowly or to fail a share of requests, and it counts how
// many deliveries it accepted and how many of those were the same delivery
// accepted again.
//
// One receiver stands in for many tenants' services: requests under /slow/
// wait -slow-latency instead of -latency, and /stats also breaks the counts
// down by the first path segment.
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
	"strings"
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
	paths      map[string]*pathStats
}

type pathStats struct {
	Received int `json:"received"`
	Failed   int `json:"failed"`
}

type statsResponse struct {
	Received   int                  `json:"received"`
	Failed     int                  `json:"failed"`
	Unique     int                  `json:"unique"`
	Duplicates int                  `json:"duplicates"`
	Messages   int                  `json:"messages"`
	Paths      map[string]pathStats `json:"paths"`
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	latency := flag.Duration("latency", 0, "delay before answering each delivery")
	failRate := flag.Float64("fail-rate", 0, "share of deliveries answered with -fail-status, 0 to 1")
	failStatus := flag.Int("fail-status", http.StatusInternalServerError, "status code for failed deliveries")
	slowLatency := flag.Duration("slow-latency", 5*time.Second, "delay before answering deliveries under /slow/")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := &stats{deliveries: make(map[string]int), messages: make(map[string]struct{}), paths: make(map[string]*pathStats)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.snapshot())
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		group := pathGroup(r.URL.Path)
		if group == "slow" {
			time.Sleep(*slowLatency)
		} else {
			time.Sleep(*latency)
		}
		messageID := r.Header.Get("webhook-id")
		deliveryID := r.Header.Get("webhook-delivery-id")

		if rand.Float64() < *failRate {
			s.recordFailure(group)
			logger.Info("delivery rejected", "webhook_id", messageID, "delivery_id", deliveryID, "status", *failStatus)
			w.WriteHeader(*failStatus)
			return
		}

		// Only accepted deliveries count towards unique and duplicates: a
		// rejected one is expected to come back.
		s.recordSuccess(group, messageID, deliveryID)
		logger.Info("delivery accepted", "webhook_id", messageID, "delivery_id", deliveryID, "path", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	logger.Info("receiver listening", "addr", *addr, "latency", latency.String(), "fail_rate", *failRate)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		logger.Error("receiver stopped", "error", err)
		os.Exit(1)
	}
}

// pathGroup returns the first segment of path: "slow" for /slow/s1.
func pathGroup(path string) string {
	group, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return group
}

func (s *stats) path(group string) *pathStats {
	p, ok := s.paths[group]
	if !ok {
		p = &pathStats{}
		s.paths[group] = p
	}
	return p
}

func (s *stats) recordSuccess(group, messageID, deliveryID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
	s.path(group).Received++
	s.deliveries[deliveryID]++
	s.messages[messageID] = struct{}{}
}

func (s *stats) recordFailure(group string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed++
	s.path(group).Failed++
}

func (s *stats) snapshot() statsResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	paths := make(map[string]pathStats, len(s.paths))
	for group, p := range s.paths {
		paths[group] = *p
	}
	return statsResponse{
		Received:   s.received,
		Failed:     s.failed,
		Unique:     len(s.deliveries),
		Duplicates: s.received - len(s.deliveries),
		Messages:   len(s.messages),
		Paths:      paths,
	}
}
