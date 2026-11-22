package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/weather"
)

func TestWeatherPluginDiscoveryAndInvocation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			t.Errorf("upstream path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("latitude"); got != "47.6" {
			t.Errorf("latitude = %q", got)
		}
		if got := r.URL.Query().Get("longitude"); got != "-122.3" {
			t.Errorf("longitude = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"latitude":47.6,"longitude":-122.3,"timezone":"America/Los_Angeles",
			"current":{"time":"2026-11-21T12:00","temperature_2m":12.5,
			"relative_humidity_2m":70,"apparent_temperature":11.2,"is_day":1,
			"precipitation":0,"weather_code":2,"wind_speed_10m":8.4}
		}`))
	}))
	defer upstream.Close()

	handler := NewHandlerWithPlugins("test", []plugins.Registration{{
		Definition: weather.Definition(),
		Adapter:    weather.NewWithClient(upstream.URL+"/v1/forecast", upstream.Client()),
	}})

	catalog := httptest.NewRecorder()
	handler.ServeHTTP(catalog, httptest.NewRequest(http.MethodGet, "/v1/plugins/weather/api", nil))
	if catalog.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	for _, expected := range []string{
		`"pluginVersion":"1.0.0"`,
		`"apiVersion":"1"`,
		`"operationId":"current"`,
		`"/api/v1/plugins/weather/api/v1/current"`,
	} {
		if !strings.Contains(catalog.Body.String(), expected) {
			t.Fatalf("catalog missing %s: %s", expected, catalog.Body.String())
		}
	}

	activation := httptest.NewRecorder()
	handler.ServeHTTP(activation, httptest.NewRequest(http.MethodPut, "/v1/plugins/weather/activation", strings.NewReader(`{"active":true}`)))
	if activation.Code != http.StatusOK {
		t.Fatalf("activation status=%d body=%s", activation.Code, activation.Body.String())
	}

	result := httptest.NewRecorder()
	handler.ServeHTTP(result, httptest.NewRequest(http.MethodPost,
		"/v1/plugins/weather/api/v1/current",
		strings.NewReader(`{"body":{"latitude":47.6,"longitude":-122.3}}`)))
	if result.Code != http.StatusOK {
		t.Fatalf("invoke status=%d body=%s", result.Code, result.Body.String())
	}
	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data["temperatureC"] != 12.5 ||
		response.Data["weatherCode"] != float64(2) ||
		response.Data["source"] != "Open-Meteo" ||
		response.Data["attributionUrl"] != "https://open-meteo.com/" {
		t.Fatalf("unexpected weather result: %#v", response.Data)
	}
}

func TestWeatherPluginRejectsInvalidCoordinatesBeforeUpstream(t *testing.T) {
	handler := NewHandlerWithPlugins("test", []plugins.Registration{{
		Definition: weather.Definition(),
		Adapter: weather.NewWithClient(
			"http://127.0.0.1:1/should-not-be-called",
			http.DefaultClient,
		),
	}})
	activation := httptest.NewRecorder()
	handler.ServeHTTP(activation, httptest.NewRequest(http.MethodPut, "/v1/plugins/weather/activation", strings.NewReader(`{"active":true}`)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost,
		"/v1/plugins/weather/api/v1/current",
		strings.NewReader(`{"body":{"latitude":91,"longitude":0}}`)))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want=422 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestWeatherPluginRejectsUnknownAPIVersion(t *testing.T) {
	handler := NewHandlerWithPlugins("test", []plugins.Registration{{
		Definition: weather.Definition(),
		Adapter:    weather.New(),
	}})
	activation := httptest.NewRecorder()
	handler.ServeHTTP(activation, httptest.NewRequest(http.MethodPut, "/v1/plugins/weather/activation", strings.NewReader(`{"active":true}`)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost,
		"/v1/plugins/weather/api/v2/current",
		strings.NewReader(`{"body":{"latitude":47.6,"longitude":-122.3}}`)))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=404 body=%s", recorder.Code, recorder.Body.String())
	}
}
