package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverPanicAnswers500AndLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	h := recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/messages/123", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error != "internal server error" {
		t.Errorf("body = %+v, err = %v", body, err)
	}

	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("log is not one JSON line: %q", logs.String())
	}
	if entry["msg"] != "panic in handler" || entry["panic"] != "boom" || entry["path"] != "/messages/123" {
		t.Errorf("log entry = %v", entry)
	}
	if stack, _ := entry["stack"].(string); !strings.Contains(stack, "recover_test.go") {
		t.Error("log entry is missing the stack trace")
	}
}

func TestRecoverPanicRepanicsAbortHandler(t *testing.T) {
	h := recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler to propagate", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
