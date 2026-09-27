package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// recoverPanic turns a handler panic into a 500 and logs it through slog, so
// it reaches the same log pipeline as every other error. http.ErrAbortHandler
// is re-panicked: it is net/http's signal to abort the response silently.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}

			slog.ErrorContext(r.Context(), "panic in handler",
				"method", r.Method,
				"path", r.URL.Path,
				"panic", rec,
				"stack", string(debug.Stack()),
			)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		}()

		next.ServeHTTP(w, r)
	})
}
