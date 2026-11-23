package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/immich"
)

// The key saved through the configuration route reaches the adapter, is sent
// only as the x-api-key header, and never appears in an API response.
func TestImmichPluginUsesConfiguredKey(t *testing.T) {
	const key = "saved-secret-key"
	authorized := true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorized || r.Header.Get("x-api-key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/server/version":
			fmt.Fprint(w, `{"major":3,"minor":4,"patch":2}`)
		case "/api/users/me":
			fmt.Fprint(w, `{"id":"u1","name":"ember-poc","isAdmin":false}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	handler := NewHandlerWithPlugins("test", []plugins.Registration{{
		Definition: immich.Definition(), Adapter: immich.New(),
	}})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
		if strings.Contains(recorder.Body.String(), key) {
			t.Fatalf("%s %s exposed the API key: %s", method, path, recorder.Body.String())
		}
		return recorder
	}

	catalog := do(http.MethodGet, "/v1/plugins/immich/api", "")
	for _, expected := range []string{`"operationId":"server"`, `"operationId":"albums"`, `"pluginVersion":"1.1.0"`} {
		if !strings.Contains(catalog.Body.String(), expected) {
			t.Fatalf("catalog missing %s: %s", expected, catalog.Body.String())
		}
	}
	if r := do(http.MethodPut, "/v1/plugins/immich/configuration",
		`{"values":{"baseUrl":"`+upstream.URL+`","apiKey":"`+key+`"}}`); r.Code != http.StatusOK {
		t.Fatalf("configuration status=%d body=%s", r.Code, r.Body.String())
	}
	if r := do(http.MethodPut, "/v1/plugins/immich/activation", `{"active":true}`); r.Code != http.StatusOK {
		t.Fatalf("activation status=%d body=%s", r.Code, r.Body.String())
	}

	r := do(http.MethodPost, "/v1/plugins/immich/api/v1/server", `{}`)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"user":"ember-poc"`) ||
		!strings.Contains(r.Body.String(), `"version":"3.4.2"`) {
		t.Fatalf("server status=%d body=%s", r.Code, r.Body.String())
	}

	authorized = false
	if r := do(http.MethodPost, "/v1/plugins/immich/api/v1/server", `{}`); r.Code != http.StatusBadGateway ||
		!strings.Contains(r.Body.String(), `"upstream_error"`) {
		t.Fatalf("rejected key: status=%d body=%s", r.Code, r.Body.String())
	}
}
