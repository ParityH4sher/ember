package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
)

const forecastURL = "https://api.open-meteo.com/v1/forecast"

type adapter struct {
	endpoint string
	client   *http.Client
}

func New() plugins.Adapter {
	return NewWithClient(forecastURL, &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

// NewWithClient allows deterministic adapter tests to use a local HTTP fixture.
func NewWithClient(endpoint string, client *http.Client) plugins.Adapter {
	return &adapter{endpoint: endpoint, client: client}
}

func Definition() plugins.Definition {
	inputSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"pathParams", "query", "body"},
		"properties": map[string]any{
			"pathParams": map[string]any{"type": "object", "additionalProperties": false},
			"query":      map[string]any{"type": "object", "additionalProperties": false},
			"body": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"latitude", "longitude"},
				"properties": map[string]any{
					"latitude":  map[string]any{"type": "number", "minimum": -90, "maximum": 90},
					"longitude": map[string]any{"type": "number", "minimum": -180, "maximum": 180},
				},
			},
		},
	}
	outputSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"latitude", "longitude", "timezone", "observedAt", "temperatureC", "weatherCode", "source", "attributionUrl"},
		"properties": map[string]any{
			"latitude":                map[string]any{"type": "number"},
			"longitude":               map[string]any{"type": "number"},
			"timezone":                map[string]any{"type": "string"},
			"observedAt":              map[string]any{"type": "string"},
			"temperatureC":            map[string]any{"type": "number"},
			"apparentTemperatureC":    map[string]any{"type": "number"},
			"relativeHumidityPercent": map[string]any{"type": "integer"},
			"precipitationMM":          map[string]any{"type": "number"},
			"windSpeedKPH":             map[string]any{"type": "number"},
			"isDay":                    map[string]any{"type": "boolean"},
			"weatherCode":              map[string]any{"type": "integer"},
			"source":                   map[string]any{"type": "string"},
			"attributionUrl":           map[string]any{"type": "string"},
		},
	}
	return plugins.Definition{
		ID:           "weather",
		Name:         "Weather",
		Version:      "1.0.0",
		APIVersion:   "1",
		Description:  "Current weather from Open-Meteo. The free public API is for non-commercial use and requires CC BY 4.0 attribution.",
		Capabilities: []string{"weather.read"},
		ConfigurationSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
		Operations: []plugins.Operation{{
			OperationID:  "current",
			Name:         "Get current weather",
			Description:  "Get current conditions by geographic coordinates.",
			Method:       "POST",
			Path:         "/api/v1/plugins/weather/api/v1/current",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		}},
	}
}

func (a *adapter) Invoke(ctx context.Context, operationID string, _ map[string]any, input map[string]any) (any, error) {
	if operationID != "current" {
		return nil, fmt.Errorf("unsupported weather operation")
	}
	body, _ := input["body"].(map[string]any)
	latitude, latOK := body["latitude"].(float64)
	longitude, lonOK := body["longitude"].(float64)
	if !latOK || !lonOK || math.IsNaN(latitude) || math.IsInf(latitude, 0) ||
		math.IsNaN(longitude) || math.IsInf(longitude, 0) ||
		latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return nil, fmt.Errorf("coordinates are out of range")
	}

	params := url.Values{
		"latitude":  {strconv.FormatFloat(latitude, 'f', -1, 64)},
		"longitude": {strconv.FormatFloat(longitude, 'f', -1, 64)},
		"current":   {"temperature_2m,relative_humidity_2m,apparent_temperature,is_day,precipitation,weather_code,wind_speed_10m"},
		"timezone":  {"auto"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Ember-Weather-Plugin/1.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weather provider returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("weather provider response too large")
	}
	var upstream struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
		Current   struct {
			Time                string  `json:"time"`
			Temperature         float64 `json:"temperature_2m"`
			RelativeHumidity    int     `json:"relative_humidity_2m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
			IsDay               int     `json:"is_day"`
			Precipitation       float64 `json:"precipitation"`
			WeatherCode         int     `json:"weather_code"`
			WindSpeed           float64 `json:"wind_speed_10m"`
		} `json:"current"`
	}
	if err := json.Unmarshal(data, &upstream); err != nil {
		return nil, err
	}
	if strings.TrimSpace(upstream.Timezone) == "" || strings.TrimSpace(upstream.Current.Time) == "" {
		return nil, fmt.Errorf("weather provider returned incomplete data")
	}
	return map[string]any{
		"latitude":                upstream.Latitude,
		"longitude":               upstream.Longitude,
		"timezone":                upstream.Timezone,
		"observedAt":              upstream.Current.Time,
		"temperatureC":            upstream.Current.Temperature,
		"apparentTemperatureC":    upstream.Current.ApparentTemperature,
		"relativeHumidityPercent": upstream.Current.RelativeHumidity,
		"precipitationMM":          upstream.Current.Precipitation,
		"windSpeedKPH":             upstream.Current.WindSpeed,
		"isDay":                    upstream.Current.IsDay == 1,
		"weatherCode":              upstream.Current.WeatherCode,
		"source":                   "Open-Meteo",
		"attributionUrl":           "https://open-meteo.com/",
	}, nil
}
