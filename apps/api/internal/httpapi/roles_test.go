package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/weather"
)

// as stands in for auth.Wrap, which sets the caller's role in production.
func as(role auth.Role, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(auth.WithRole(r.Context(), role)))
	})
}

func send(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

// Admins change plugins; users only read them and call their operations.
func TestPluginRoles(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"timezone":"UTC","current":{"time":"2026-11-23T12:00","temperature_2m":4}}`))
	}))
	defer upstream.Close()
	api := NewHandlerWithPlugins("test", []plugins.Registration{{
		Definition: weather.Definition(),
		Adapter:    weather.NewWithClient(upstream.URL, upstream.Client()),
	}})
	admin, user, anonymous := as(auth.RoleAdmin, api), as(auth.RoleUser, api), api
	const invoke = "/v1/plugins/weather/api/v1/current"
	const input = `{"body":{"latitude":1,"longitude":2}}`

	for _, path := range []string{"/v1/plugins/weather/configuration", "/v1/plugins/weather/activation"} {
		body := map[bool]string{true: `{"values":{}}`, false: `{"active":true}`}[strings.HasSuffix(path, "configuration")]
		rec := send(user, http.MethodPut, path, body)
		var e struct{ Code, Message string }
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusForbidden || e.Code != "insufficient_role" || !strings.Contains(e.Message, "admin role") {
			t.Fatalf("user PUT %s: status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
	if rec := send(user, http.MethodGet, "/v1/plugins/weather", ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"active":false`) {
		t.Fatalf("user's refused activation changed the plugin: %s", rec.Body.String())
	}

	if rec := send(admin, http.MethodPut, "/v1/plugins/weather/activation", `{"active":true}`); rec.Code != http.StatusOK {
		t.Fatalf("admin activation status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/v1/plugins", "/v1/plugins/weather", "/v1/plugins/weather/configuration", "/v1/plugins/weather/api"} {
		if rec := send(user, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("user GET %s status=%d", path, rec.Code)
		}
	}
	if rec := send(user, http.MethodPost, invoke, input); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"temperatureC":4`) {
		t.Fatalf("user invoke status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := send(admin, http.MethodPost, invoke, input); rec.Code != http.StatusOK {
		t.Fatalf("admin invoke status=%d body=%s", rec.Code, rec.Body.String())
	}

	// A request that did not pass through auth.Wrap has no role and gets nothing.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/plugins", ""},
		{http.MethodGet, "/v1/projects", ""},
		{http.MethodPost, invoke, input},
		{http.MethodPut, "/v1/plugins/weather/activation", `{"active":false}`},
	} {
		if rec := send(anonymous, tc.method, tc.path, tc.body); rec.Code != http.StatusForbidden {
			t.Errorf("no role %s %s status=%d", tc.method, tc.path, rec.Code)
		}
	}
	if rec := send(anonymous, http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("health must not need a role, status=%d", rec.Code)
	}
}
