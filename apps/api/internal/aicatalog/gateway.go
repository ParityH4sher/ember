package aicatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	gatewayTimeout = 10 * time.Second
	maxGatewayURL  = 300
	maxGatewayKey  = 1024
	maxModelList   = 4 << 20
	maxListed      = 500
)

// GatewayError is a failed model listing. Its text is safe to show an admin:
// it carries the gateway's status, never its body.
type GatewayError struct{ Msg string }

func (e *GatewayError) Error() string { return e.Msg }

// Gateway lists the models the model gateway (LiteLLM) offers. Ember only
// reads the list; provider endpoints and keys stay in the gateway.
type Gateway struct {
	origin string
	key    string
	client *http.Client
}

// LoadGateway reads EMBER_MODEL_GATEWAY_URL and EMBER_MODEL_GATEWAY_KEY_FILE
// style settings. Both empty means no gateway (nil, nil); one without the
// other is a configuration error.
func LoadGateway(rawURL, keyFile string) (*Gateway, error) {
	if rawURL == "" && keyFile == "" {
		return nil, nil
	}
	if rawURL == "" || keyFile == "" {
		return nil, errors.New("model gateway: set both EMBER_MODEL_GATEWAY_URL and EMBER_MODEL_GATEWAY_KEY_FILE, or neither")
	}
	f, err := os.Open(keyFile)
	if err != nil {
		return nil, fmt.Errorf("model gateway: reading the key file: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxGatewayKey+2))
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("model gateway: reading the key file: %w", err)
	}
	return NewGateway(rawURL, strings.TrimSpace(string(data)))
}

// NewGateway checks the gateway's origin and key. The origin is an http or
// https URL with no path, query, fragment or user info: Ember appends
// /v1/models itself, so a path could only be a mistake or a way to send the
// key somewhere else.
func NewGateway(rawURL, key string) (*Gateway, error) {
	u, err := url.Parse(rawURL)
	if err != nil || len(rawURL) > maxGatewayURL || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Host == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("model gateway: EMBER_MODEL_GATEWAY_URL must be an http or https origin such as http://litellm:4000, without path, query or credentials")
	}
	if key == "" || len(key) > maxGatewayKey {
		return nil, fmt.Errorf("model gateway: the key file must hold a key of 1 to %d bytes", maxGatewayKey)
	}
	for _, r := range key {
		if r <= ' ' || r > '~' {
			return nil, errors.New("model gateway: the key may contain only printable ASCII without spaces")
		}
	}
	return &Gateway{
		origin: u.Scheme + "://" + u.Host,
		key:    key,
		client: &http.Client{
			Timeout: gatewayTimeout,
			// The key goes only to the configured origin, never to a
			// redirect target.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Origin returns the gateway's origin, for logs. It holds no secret.
func (g *Gateway) Origin() string { return g.origin }

// Models lists the gateway's model names, sorted, keeping only names the
// catalog accepts. It is an admin convenience for offering models, not a
// health check of chat.
func (g *Gateway) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.origin+"/v1/models", nil)
	if err != nil {
		return nil, &GatewayError{Msg: "the gateway URL is not usable"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.key)
	resp, err := g.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &GatewayError{Msg: "the model gateway did not answer in time"}
		}
		return nil, &GatewayError{Msg: "the model gateway is not reachable from the Ember API"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Drain a little so the connection can be reused; never show it.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, &GatewayError{Msg: fmt.Sprintf("the model gateway refused Ember's key (HTTP %d)", resp.StatusCode)}
		}
		return nil, &GatewayError{Msg: fmt.Sprintf("the model gateway answered HTTP %d", resp.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxModelList+1))
	if err != nil || len(data) > maxModelList {
		return nil, &GatewayError{Msg: "the model gateway's model list could not be read"}
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &body) != nil || body.Data == nil {
		return nil, &GatewayError{Msg: "the answer is not an OpenAI-style model list; check EMBER_MODEL_GATEWAY_URL"}
	}
	ids := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		if ValidModelID(m.ID) {
			ids = append(ids, m.ID)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > maxListed {
		ids = ids[:maxListed]
	}
	return ids, nil
}
