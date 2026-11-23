package main

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/httpapi"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

func httpHandler(t *testing.T) http.Handler {
	t.Helper()
	vaults, err := projects.OpenVaults(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// As in run: sign-in off, so auth.Wrap gives every caller the admin role.
	return auth.New(nil, auth.Options{}).Wrap(httpapi.NewHandler(version, vaults))
}

func TestHealthz(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	httpHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(body, map[string]string{"status": "ok", "version": version}) {
		t.Errorf("unexpected health response: %v", body)
	}
}

func TestRouteBoundaries(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/healthz", http.StatusNotFound}, // The proxy owns /api/.
		{http.MethodGet, "/v1/plugins", http.StatusOK},
		{http.MethodGet, "/unknown", http.StatusNotFound},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httpHandler(t).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestPluginConfigurationActivationAndSecretRedaction(t *testing.T) {
	handler := httpHandler(t)
	request := httptest.NewRequest(http.MethodPut, "/v1/plugins/immich/configuration",
		strings.NewReader(`{"values":{"baseUrl":"http://immich.local","apiKey":"top-secret"}}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("configure status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "top-secret") {
		t.Fatalf("response disclosed secret: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/v1/plugins/immich/activation", strings.NewReader(`{"active":true}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("activate status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"active":true`) {
		t.Fatalf("activation missing from response: %s", recorder.Body.String())
	}

	// Omitting the write-only credential during a later configuration update
	// preserves it server-side without returning it to the client.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/v1/plugins/immich/configuration",
		strings.NewReader(`{"values":{"baseUrl":"http://immich-new.local"}}`)))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "top-secret") {
		t.Fatalf("secret preservation response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPluginConfigurationRejectsInvalidAndUnavailablePlugins(t *testing.T) {
	handler := httpHandler(t)
	for _, tc := range []struct {
		name   string
		path   string
		body   string
		status int
	}{
		{"unknown plugin", "/v1/plugins/not-included/configuration", `{"values":{}}`, http.StatusNotFound},
		{"missing required", "/v1/plugins/immich/configuration", `{"values":{"baseUrl":"http://immich.local"}}`, http.StatusUnprocessableEntity},
		{"invalid url", "/v1/plugins/immich/configuration", `{"values":{"baseUrl":"file:///etc/passwd","apiKey":"x"}}`, http.StatusUnprocessableEntity},
		{"unknown field", "/v1/plugins/immich/configuration", `{"values":{"baseUrl":"http://immich.local","apiKey":"x","extra":true}}`, http.StatusUnprocessableEntity},
		{"wrong type", "/v1/plugins/immich/configuration", `{"values":{"baseUrl":42,"apiKey":"x"}}`, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body)))
			if recorder.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestProjectDeleteLifecycle(t *testing.T) {
	handler := httpHandler(t)
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(`{"id":"demo","name":"Demo"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/v1/projects/demo", nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodDelete, "/v1/projects/demo", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d want 404", missing.Code)
	}
}
