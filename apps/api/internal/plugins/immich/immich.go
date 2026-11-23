// Package immich reads from an operator-configured Immich server: its version,
// the user that the API key belongs to, and the albums that user owns or that
// are shared with it. It never writes to Immich.
//
// Only selected fields are returned, never a raw upstream body, so a
// misdirected base URL cannot be used to read arbitrary content back.
package immich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
)

const (
	responseLimit = 1 << 20
	// MaxAlbums bounds the album list returned to the browser.
	MaxAlbums = 200
)

type adapter struct {
	client *http.Client
}

// New returns the adapter with a bounded client that never follows redirects,
// so the API key is sent only to the configured server.
func New() plugins.Adapter {
	return NewWithClient(&http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

// NewWithClient allows deterministic adapter tests to use a local HTTP fixture.
func NewWithClient(client *http.Client) plugins.Adapter {
	return &adapter{client: client}
}

func Definition() plugins.Definition {
	empty := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	input := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"pathParams": empty, "query": empty, "body": empty,
	}}
	return plugins.Definition{
		ID:          "immich",
		Name:        "Immich",
		Version:     "1.1.0",
		APIVersion:  "1",
		Description: "Read-only access to an independently operated Immich server, as the user that owns the API key.",
		// Keep the original capability for clients that matched on it.
		Capabilities: []string{"photo-library", "photo-library.read"},
		ConfigurationSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"baseUrl", "apiKey"},
			"properties": map[string]any{
				"baseUrl": map[string]any{
					"type":        "string",
					"format":      "uri",
					"description": "Operator-configured Immich server URL.",
				},
				"apiKey": map[string]any{
					"type":        "string",
					"writeOnly":   true,
					"format":      "password",
					"description": "Immich API key. Never returned by Ember.",
				},
			},
		},
		Operations: []plugins.Operation{
			{
				OperationID: "server",
				Name:        "Server info",
				Description: "Check the connection: the Immich version and the user that the API key belongs to.",
				Method:      "POST",
				Path:        "/api/v1/plugins/immich/api/v1/server",
				InputSchema: input,
				OutputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"version": map[string]any{"type": "string"},
					"user":    map[string]any{"type": "string"},
					"isAdmin": map[string]any{"type": "boolean"},
				}},
			},
			{
				OperationID: "albums",
				Name:        "List albums",
				Description: fmt.Sprintf("Albums the key's user owns or that are shared with it, at most %d.", MaxAlbums),
				Method:      "POST",
				Path:        "/api/v1/plugins/immich/api/v1/albums",
				InputSchema: input,
				OutputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"albums": map[string]any{"type": "array", "items": map[string]any{
						"type": "object", "properties": map[string]any{
							"id":         map[string]any{"type": "string"},
							"name":       map[string]any{"type": "string"},
							"assetCount": map[string]any{"type": "integer"},
							"shared":     map[string]any{"type": "boolean"},
							"owner":      map[string]any{"type": "string"},
							"updatedAt":  map[string]any{"type": "string"},
						},
					}},
					"truncated": map[string]any{"type": "boolean"},
				}},
			},
		},
	}
}

type album struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AssetCount int    `json:"assetCount"`
	Shared     bool   `json:"shared"`
	Owner      string `json:"owner"`
	UpdatedAt  string `json:"updatedAt"`
}

func (a *adapter) Invoke(ctx context.Context, operationID string, configuration, _ map[string]any) (any, error) {
	base, key, err := settings(configuration)
	if err != nil {
		return nil, err
	}
	switch operationID {
	case "server":
		var version struct {
			Major int `json:"major"`
			Minor int `json:"minor"`
			Patch int `json:"patch"`
		}
		if err := a.get(ctx, base, "/api/server/version", key, &version); err != nil {
			return nil, err
		}
		if version.Major == 0 && version.Minor == 0 && version.Patch == 0 {
			return nil, errors.New("not an Immich version response")
		}
		// Unlike the version, this needs a valid key, so it proves the key works.
		var me struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			IsAdmin bool   `json:"isAdmin"`
		}
		if err := a.get(ctx, base, "/api/users/me", key, &me); err != nil {
			return nil, err
		}
		if me.ID == "" {
			return nil, errors.New("not an Immich user response")
		}
		return map[string]any{
			"version": fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch),
			"user":    me.Name,
			"isAdmin": me.IsAdmin,
		}, nil
	case "albums":
		var upstream []struct {
			ID         string `json:"id"`
			AlbumName  string `json:"albumName"`
			AssetCount int    `json:"assetCount"`
			Shared     bool   `json:"shared"`
			UpdatedAt  string `json:"updatedAt"`
			Owner      struct {
				Name string `json:"name"`
			} `json:"owner"`
		}
		if err := a.get(ctx, base, "/api/albums", key, &upstream); err != nil {
			return nil, err
		}
		albums := make([]album, 0, min(len(upstream), MaxAlbums))
		for _, u := range upstream {
			if len(albums) == MaxAlbums {
				break
			}
			if u.ID == "" {
				return nil, errors.New("not an Immich album response")
			}
			albums = append(albums, album{u.ID, u.AlbumName, u.AssetCount, u.Shared, u.Owner.Name, u.UpdatedAt})
		}
		return map[string]any{"albums": albums, "truncated": len(upstream) > MaxAlbums}, nil
	default:
		return nil, errors.New("unsupported Immich operation")
	}
}

// settings re-checks the stored configuration: the API validates it on save,
// but the adapter must not depend on that for where it sends the key.
func settings(configuration map[string]any) (string, string, error) {
	rawBase, _ := configuration["baseUrl"].(string)
	key, _ := configuration["apiKey"].(string)
	u, err := url.Parse(rawBase)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("immich base URL must be an http(s) URL without credentials, query or fragment")
	}
	if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n") {
		return "", "", errors.New("immich API key is missing or invalid")
	}
	return strings.TrimRight(u.String(), "/"), key, nil
}

func (a *adapter) get(ctx context.Context, base, path, key string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Ember-Immich-Plugin/1.1")
	req.Header.Set("x-api-key", key)
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("immich returned status %d for %s", res.StatusCode, path)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil {
		return err
	}
	if len(body) > responseLimit {
		return fmt.Errorf("immich response for %s is too large", path)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("invalid immich response for %s", path)
	}
	return nil
}
