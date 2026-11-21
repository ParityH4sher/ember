package main

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"
)

// Keep request data out of access logs: the matched pattern is server-defined;
// the raw URL, headers and request body can contain user data.
func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		out := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(out, r)

		pattern := r.Pattern
		if pattern == "" {
			pattern = "unmatched"
		}
		code := out.status
		if code == 0 {
			code = http.StatusOK
		}
		logger.Info("http request", "request_id", id, "route", pattern,
			"status", code, "duration_us", time.Since(start).Microseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
