package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setup(t *testing.T, worker http.HandlerFunc) http.Handler {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pw, _ := r.BasicAuth()
		if user != "opencode" || pw != "private-worker" {
			t.Error("wrong worker identity")
			w.WriteHeader(401)
			return
		}
		worker(w, r)
	}))
	t.Cleanup(s.Close)
	return (&panel{upstream: s.URL, password: "private-worker", operator: "test-operator", client: s.Client()}).handler()
}

func visit(h http.Handler, method, path, body string, auth, action bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, prefix+path, strings.NewReader(body))
	if auth {
		r.SetBasicAuth("operator", "test-operator")
	}
	if action {
		r.Header.Set("X-Ember-Action", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPanelBoundary(t *testing.T) {
	h := setup(t, func(http.ResponseWriter, *http.Request) { t.Error("request reached worker") })
	for _, tc := range []struct {
		method, path, body string
		auth, action       bool
		want               int
	}{
		{"GET", "events", "", false, false, 401},
		{"POST", "interrupt", "", true, false, 403},
		{"POST", "steer", `{"text":"hi","sessionID":"ses_other"}`, true, true, 400},
		{"POST", "steer", `{"text":"` + strings.Repeat("z", 33<<10) + `"}`, true, true, 400},
		{"POST", "permissions/per_1/always", "", true, true, 400},
		{"GET", "api/session", "", true, true, 404},
		{"GET", "", "", true, false, 200},
	} {
		if w := visit(h, tc.method, tc.path, tc.body, tc.auth, tc.action); w.Code != tc.want {
			t.Errorf("%s: got %d want %d", tc.path, w.Code, tc.want)
		}
	}
}

func TestReviewLifecycle(t *testing.T) {
	created := false
	h := setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.RequestURI() {
		case "GET " + session:
			if !created {
				w.WriteHeader(404)
				return
			}
			fmt.Fprintf(w, `{"data":{"id":%q}}`, sessionID)
		case "POST /api/session":
			var v struct {
				ID       string
				Location struct{ Directory string }
			}
			if created || json.NewDecoder(r.Body).Decode(&v) != nil || v.ID != sessionID || v.Location.Directory != "/workspace" {
				t.Error("wrong session creation")
			}
			created = true
			fmt.Fprintf(w, `{"data":{"id":%q}}`, sessionID)
		case "POST " + session + "/prompt":
			var v struct{ Text, Delivery string }
			if json.NewDecoder(r.Body).Decode(&v) != nil || v.Delivery != "steer" || v.Text != "focus on errors" {
				t.Error("wrong prompt delivery")
			}
			fmt.Fprint(w, `{"data":{"delivery":"steer"}}`)
		case "GET " + session + "/message?limit=50&order=desc":
			fmt.Fprint(w, `{"data":[{"type":"assistant","content":[{"type":"reasoning","text":"private"},{"type":"text","text":"A result"}]},{"type":"user","text":"review"}]}`)
		case "GET /api/event":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"session.text.delta\",\"data\":{\"sessionID\":\"ses_other\",\"delta\":\"private\"}}\n\n")
			fmt.Fprintf(w, "data: {\"type\":\"session.text.delta\",\"data\":{\"sessionID\":%q,\"delta\":\"progress\"}}\n\n", sessionID)
		case "POST " + session + "/permission/per_1/reply":
			var d struct{ Decision string }
			if json.NewDecoder(r.Body).Decode(&d) != nil || d.Decision != "once" {
				t.Error("wrong permission decision")
			}
			w.WriteHeader(204)
		case "POST " + session + "/interrupt?resume=false":
			fmt.Fprint(w, `{"interrupted":false}`)
		default:
			t.Errorf("unexpected upstream %s", r.URL)
			w.WriteHeader(404)
		}
	})
	for _, tc := range []struct {
		method, path, body string
		code               int
		contains           string
	}{
		{"POST", "connect", "", 204, ""}, {"POST", "steer", `{"text":"focus on errors"}`, 202, ""},
		{"POST", "connect", "", 204, ""}, {"GET", "messages", "", 200, `[{"role":"You","text":"review"},{"role":"Agent","text":"A result"}]`},
		{"GET", "events", "", 200, `"delta":"progress"`}, {"POST", "permissions/per_1/once", "", 204, ""},
		{"POST", "interrupt", "", 200, `"interrupted":false`},
	} {
		w := visit(h, tc.method, tc.path, tc.body, true, true)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.contains) || strings.Contains(w.Body.String(), "private") {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
