package aicatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

const (
	discoverTimeout = 10 * time.Second
	maxDiscovery    = 4 << 20
	maxDiscovered   = 500
)

// DiscoverError is a failed model listing. Its text is safe to show an admin:
// it carries the upstream status, not the upstream body.
type DiscoverError struct{ Msg string }

func (e *DiscoverError) Error() string { return e.Msg }

// NewDiscoveryClient returns the HTTP client used to list a provider's
// models. It follows no redirects, so a key is only ever sent to the
// configured origin.
func NewDiscoveryClient() *http.Client {
	return &http.Client{
		Timeout:       discoverTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Discover lists the model IDs a provider offers, sorted. It is an admin
// convenience for filling in models, not a health check of chat.
func Discover(ctx context.Context, client *http.Client, p Provider) ([]string, error) {
	var endpoint string
	switch p.Kind {
	case KindOllama:
		endpoint = p.BaseURL + "/api/tags"
	case KindAnthropic, KindOpenAI, KindOpenAICompatible:
		endpoint = p.BaseURL + "/models"
	default:
		return nil, &DiscoverError{Msg: "this provider kind cannot list models"}
	}
	if (p.Kind == KindAnthropic || p.Kind == KindOpenAI) && p.APIKey == "" {
		return nil, &DiscoverError{Msg: "set the provider's API key first"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &DiscoverError{Msg: "the base URL is not usable"}
	}
	req.Header.Set("Accept", "application/json")
	if p.APIKey != "" {
		switch p.Kind {
		case KindAnthropic:
			req.Header.Set("x-api-key", p.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		default:
			req.Header.Set("Authorization", "Bearer "+p.APIKey)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &DiscoverError{Msg: "the provider did not answer in time"}
		}
		return nil, &DiscoverError{Msg: "the provider is not reachable from the Ember API"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, &DiscoverError{Msg: fmt.Sprintf("the provider refused the API key (HTTP %d)", resp.StatusCode)}
		}
		return nil, &DiscoverError{Msg: fmt.Sprintf("the provider answered HTTP %d", resp.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscovery+1))
	if err != nil || len(data) > maxDiscovery {
		return nil, &DiscoverError{Msg: "the provider's model list could not be read"}
	}
	var ids []string
	if p.Kind == KindOllama {
		var body struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if json.Unmarshal(data, &body) != nil {
			return nil, &DiscoverError{Msg: "the answer is not an Ollama model list; check the base URL"}
		}
		for _, m := range body.Models {
			ids = append(ids, m.Name)
		}
	} else {
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &body) != nil {
			return nil, &DiscoverError{Msg: "the answer is not a model list; check the base URL"}
		}
		for _, m := range body.Data {
			ids = append(ids, m.ID)
		}
	}
	// Keep only IDs that would pass PutModel's validation.
	ids = slices.DeleteFunc(ids, func(id string) bool { return !validUpstream(id) })
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > maxDiscovered {
		ids = ids[:maxDiscovered]
	}
	return ids, nil
}
