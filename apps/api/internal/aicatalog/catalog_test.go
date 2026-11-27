package aicatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func seeded(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, func(id string) bool { return id == "chat-fixture" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutProvider("anthropic", ProviderInput{Name: "Anthropic", Kind: KindAnthropic, APIKey: ptr("sk-ant-secret")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutProvider("lan-ollama", ProviderInput{Name: "LAN Ollama", Kind: KindOllama, BaseURL: "http://ollama.lan:11434/", Local: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutModel("sonnet", Model{Name: "Claude Sonnet", Provider: "anthropic", Upstream: "claude-sonnet-x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutModel("qwen", Model{Name: "Qwen 3 8B", Provider: "lan-ollama", Upstream: "qwen3:8b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutAgent("claude", Agent{Name: "Claude Code", Harness: "claude-code", RouterAgent: "claude-code", Models: []string{"sonnet", "qwen"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCatalogPersistsAndHidesKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s := seeded(t, path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("catalog file mode = %v; want 0600, it holds keys", info.Mode().Perm())
	}
	v := s.View()
	if !v.Persisted || len(v.Providers) != 2 || len(v.Models) != 2 || len(v.Agents) != 1 {
		t.Fatalf("view = %+v", v)
	}
	if !v.Providers[0].HasAPIKey || v.Providers[0].BaseURL != "https://api.anthropic.com/v1" {
		t.Fatalf("anthropic = %+v; want default URL and a key", v.Providers[0])
	}
	if v.Providers[1].BaseURL != "http://ollama.lan:11434" {
		t.Fatalf("trailing slash kept: %q", v.Providers[1].BaseURL)
	}
	if v.Agents[0].DefaultModel != "sonnet" {
		t.Fatalf("default model = %q; want the first listed", v.Agents[0].DefaultModel)
	}

	reopened, err := Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := reopened.Provider("anthropic")
	if p.APIKey != "sk-ant-secret" {
		t.Fatalf("key after reload = %q", p.APIKey)
	}
	r, ok := reopened.Agent("claude")
	if !ok || r.Model == nil || r.Model.Upstream != "claude-sonnet-x" || r.Provider == nil || r.Provider.Kind != KindAnthropic {
		t.Fatalf("resolved = %+v, %v", r, ok)
	}
}

func TestCatalogKeyHandling(t *testing.T) {
	s := seeded(t, "")
	// Omitting the key keeps it.
	if p, err := s.PutProvider("anthropic", ProviderInput{Name: "Anthropic (team)", Kind: KindAnthropic}); err != nil || !p.HasAPIKey {
		t.Fatalf("rename = %+v, %v; want key kept", p, err)
	}
	// Moving the provider without re-entering the key drops it.
	if p, err := s.PutProvider("anthropic", ProviderInput{Name: "Anthropic", Kind: KindAnthropic, BaseURL: "https://proxy.example/v1"}); err != nil || p.HasAPIKey {
		t.Fatalf("move = %+v, %v; want key dropped", p, err)
	}
	if p, err := s.PutProvider("anthropic", ProviderInput{Name: "Anthropic", Kind: KindAnthropic, BaseURL: "https://proxy.example/v1", APIKey: ptr("k2")}); err != nil || !p.HasAPIKey {
		t.Fatalf("new key = %+v, %v", p, err)
	}
	if p, err := s.PutProvider("anthropic", ProviderInput{Name: "Anthropic", Kind: KindAnthropic, BaseURL: "https://proxy.example/v1", ClearAPIKey: true}); err != nil || p.HasAPIKey {
		t.Fatalf("clear = %+v, %v", p, err)
	}
	// Hosted kinds are never local, so they need https.
	if p, _ := s.PutProvider("openai", ProviderInput{Name: "OpenAI", Kind: KindOpenAI, Local: true}); p.Local {
		t.Fatal("OpenAI marked local")
	}
}

func TestCatalogValidation(t *testing.T) {
	s := seeded(t, "")
	before := s.View()
	var invalidErr *ValidationError
	for name, err := range map[string]error{
		"bad provider ID": func() error {
			_, e := s.PutProvider("Bad_ID", ProviderInput{Name: "x", Kind: KindOllama, BaseURL: "http://x", Local: true})
			return e
		}(),
		"unknown kind": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: "gemini", BaseURL: "https://x"})
			return e
		}(),
		"no ollama URL": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: KindOllama, Local: true})
			return e
		}(),
		"URL credentials": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: KindOllama, BaseURL: "http://u:p@x", Local: true})
			return e
		}(),
		"hosted over http": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: KindOpenAICompatible, BaseURL: "http://x/v1"})
			return e
		}(),
		"file URL": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: KindOllama, BaseURL: "file:///etc", Local: true})
			return e
		}(),
		"key with newline": func() error {
			_, e := s.PutProvider("x", ProviderInput{Name: "x", Kind: KindOpenAI, APIKey: ptr("a\nb")})
			return e
		}(),
		"model no provider": func() error { _, e := s.PutModel("m", Model{Name: "M", Provider: "nope", Upstream: "m"}); return e }(),
		"upstream space": func() error {
			_, e := s.PutModel("m", Model{Name: "M", Provider: "anthropic", Upstream: "a b"})
			return e
		}(),
		"unknown harness": func() error { _, e := s.PutAgent("a", Agent{Name: "A", Harness: "gpt", RouterAgent: "r"}); return e }(),
		"bad router agent": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "sha256:abc"})
			return e
		}(),
		"unknown model": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "r", Models: []string{"nope"}})
			return e
		}(),
		"default unlisted": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "r", Models: []string{"qwen"}, DefaultModel: "sonnet"})
			return e
		}(),
		"reserved ID": func() error {
			_, e := s.PutAgent("chat-fixture", Agent{Name: "A", Harness: "custom", RouterAgent: "r"})
			return e
		}(),
	} {
		if !errors.As(err, &invalidErr) {
			t.Errorf("%s: err = %v; want a ValidationError", name, err)
		}
	}
	after := s.View()
	if len(after.Providers) != len(before.Providers) || len(after.Models) != len(before.Models) || len(after.Agents) != len(before.Agents) {
		t.Fatal("a rejected change altered the catalog")
	}
}

func TestCatalogDeleteChecksReferences(t *testing.T) {
	s := seeded(t, "")
	if err := s.DeleteProvider("anthropic"); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete used provider = %v", err)
	}
	if err := s.DeleteModel("sonnet"); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete used model = %v", err)
	}
	if _, err := s.PutAgent("claude", Agent{Name: "Claude Code", Harness: "claude-code", RouterAgent: "claude-code", Models: []string{"qwen"}, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Agent("claude"); ok {
		t.Fatal("a disabled agent resolved for chat")
	}
	if err := s.DeleteModel("sonnet"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProvider("anthropic"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent("claude"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent("claude"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}

func TestOpenRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open("relative.json", nil); err == nil {
		t.Fatal("accepted a relative path")
	}
	if s, err := Open(filepath.Join(dir, "missing.json"), nil); err != nil || len(s.View().Providers) != 0 {
		t.Fatalf("missing file = %v; want an empty catalog", err)
	}
	for name, doc := range map[string]string{
		"other schema": `{"schema":"x","providers":[],"models":[],"agents":[]}`,
		"dangling":     `{"schema":"ember.ai-catalog/v1","providers":[],"models":[{"id":"m","name":"M","provider":"p","upstream":"u"}],"agents":[]}`,
		"not json":     `{`,
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDiscover(t *testing.T) {
	var gotPath, gotAuth, gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotKey = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-api-key")
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:8b"},{"name":"llama3.2:3b"},{"name":"has space"}]}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"b-model"},{"id":"a-model"},{"id":"a-model"}]}`))
		case "/denied/models":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"secret upstream detail"}`))
		case "/redirect/models":
			http.Redirect(w, r, "https://elsewhere.example/models", http.StatusFound)
		}
	}))
	defer upstream.Close()
	client := NewDiscoveryClient()
	ctx := context.Background()

	ids, err := Discover(ctx, client, Provider{Kind: KindOllama, BaseURL: upstream.URL})
	if err != nil || !slices.Equal(ids, []string{"llama3.2:3b", "qwen3:8b"}) || gotPath != "/api/tags" {
		t.Fatalf("ollama = %v, %v at %s", ids, err, gotPath)
	}
	ids, err = Discover(ctx, client, Provider{Kind: KindOpenAICompatible, BaseURL: upstream.URL + "/v1", APIKey: "k"})
	if err != nil || !slices.Equal(ids, []string{"a-model", "b-model"}) || gotAuth != "Bearer k" {
		t.Fatalf("openai-compatible = %v, %v, auth %q", ids, err, gotAuth)
	}
	if _, err = Discover(ctx, client, Provider{Kind: KindAnthropic, BaseURL: upstream.URL + "/v1", APIKey: "ak"}); err != nil || gotKey != "ak" || gotAuth != "" {
		t.Fatalf("anthropic = %v, x-api-key %q, auth %q", err, gotKey, gotAuth)
	}
	var d *DiscoverError
	if _, err = Discover(ctx, client, Provider{Kind: KindOpenAI, BaseURL: upstream.URL + "/v1"}); !errors.As(err, &d) {
		t.Fatalf("OpenAI without key = %v", err)
	}
	_, err = Discover(ctx, client, Provider{Kind: KindOpenAICompatible, BaseURL: upstream.URL + "/denied", APIKey: "k"})
	if !errors.As(err, &d) || strings.Contains(d.Msg, "secret") || !strings.Contains(d.Msg, "401") {
		t.Fatalf("denied = %v", err)
	}
	if _, err = Discover(ctx, client, Provider{Kind: KindOpenAICompatible, BaseURL: upstream.URL + "/redirect", APIKey: "k"}); !errors.As(err, &d) {
		t.Fatalf("redirect = %v; want refused, not followed", err)
	}
}
