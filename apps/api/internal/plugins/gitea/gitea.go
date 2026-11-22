// Package gitea exposes one operator-selected public repository, read-only.
package gitea

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
)

type adapter struct {
	endpoint string
	client   *http.Client
}

func Register(repoURL string) (plugins.Registration, error) {
	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return plugins.Registration{}, errors.New("EMBER_GITEA_REPOSITORY_URL must be a public repository URL without credentials, query or fragment")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	valid := regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$`)
	if len(parts) < 2 {
		return plugins.Registration{}, errors.New("Gitea URL must end in owner/repository")
	}
	for _, part := range parts {
		if !valid.MatchString(part) {
			return plugins.Registration{}, errors.New("invalid Gitea repository path")
		}
	}
	n := len(parts)
	prefix := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/"+parts[n-2]+"/"+parts[n-1])
	u.Path = prefix + "/api/v1/repos/" + parts[n-2] + "/" + parts[n-1]
	empty := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	input := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"pathParams": empty, "query": empty, "body": empty,
	}}
	return plugins.Registration{
		Definition: plugins.Definition{
			ID: "gitea", Name: "Gitea", Version: "1.0.0", APIVersion: "1",
			Description:  "Read public metadata for the repository selected by this deployment.",
			Capabilities: []string{"repository.read"}, ConfigurationSchema: empty,
			Operations: []plugins.Operation{{
				OperationID: "repository", Name: "Repository details", Method: "POST",
				Path: "/api/v1/plugins/gitea/api/v1/repository", InputSchema: input,
				OutputSchema: map[string]any{"type": "object", "properties": map[string]any{
					"repository":    map[string]any{"type": "string"},
					"defaultBranch": map[string]any{"type": "string"},
					"description":   map[string]any{"type": "string"},
				}},
			}},
		},
		Adapter: &adapter{u.String(), &http.Client{Timeout: 5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}},
	}, nil
}

func (a *adapter) Invoke(ctx context.Context, operation string, _, _ map[string]any) (any, error) {
	if operation != "repository" {
		return nil, errors.New("unknown operation")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("repository unavailable")
	}
	const limit = 128 << 10
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	var repo struct {
		Name          string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		Description   string `json:"description"`
	}
	if err != nil || len(body) > limit || json.Unmarshal(body, &repo) != nil || repo.Name == "" {
		return nil, errors.New("invalid repository response")
	}
	return map[string]string{"repository": repo.Name, "defaultBranch": repo.DefaultBranch, "description": repo.Description}, nil
}
