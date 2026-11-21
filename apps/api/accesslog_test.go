package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogCorrelatesWithoutRequestData(t *testing.T) {
	var logs bytes.Buffer
	handler := accessLog(slog.New(slog.NewJSONHandler(&logs, nil)), httpHandler())
	var previousID string
	for _, tc := range []struct {
		path  string
		route string
		want  int
	}{
		{"/healthz?token=fixture-private", "GET /healthz", http.StatusOK},
		{"/fixture-private", "unmatched", http.StatusNotFound},
	} {
		logs.Reset()
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.Header.Set("X-Request-ID", "fixture-private")
		r.Header.Set("Authorization", "Bearer fixture-private")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		id := w.Header().Get("X-Request-ID")
		if id == "" || id == previousID || id == "fixture-private" {
			t.Fatalf("server did not generate a fresh request ID: %q", id)
		}
		previousID = id
		if strings.Contains(logs.String(), "fixture-private") {
			t.Fatal("access log disclosed untrusted request data")
		}
		var event struct {
			ID     string `json:"request_id"`
			Route  string `json:"route"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event.ID != id || event.Route != tc.route || event.Status != tc.want || w.Code != tc.want {
			t.Fatalf("incorrect access record: %+v (HTTP %d)", event, w.Code)
		}
	}
}
