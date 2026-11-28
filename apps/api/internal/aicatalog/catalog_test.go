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
	"sync"
	"testing"
	"time"
)

func isFixture(id string) bool { return id == "chat-fixture" }
func isWorker(id string) bool  { return id == "opencode" }

func seeded(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, isFixture, isWorker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutModel("claude-sonnet", Model{Name: "Claude Sonnet"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutModel("ollama/qwen3:8b", Model{Name: " Qwen 3 8B "}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutAgent("claude", Agent{Name: "Claude Code", Harness: "claude-code", RouterAgent: "claude-code", Models: []string{"claude-sonnet", "ollama/qwen3:8b"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutAgent("coder", Agent{Name: "Coder", Harness: HarnessOpenCode, Worker: "opencode", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCatalogPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s := seeded(t, path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("catalog file mode = %v; want 0600", info.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"schema": "ember.ai-catalog/v2"`) {
		t.Fatalf("file = %s; want schema v2", data)
	}
	v := s.View()
	if !v.Persisted || len(v.Models) != 2 || len(v.Agents) != 2 {
		t.Fatalf("view = %+v", v)
	}
	if v.Models[1].Name != "Qwen 3 8B" {
		t.Fatalf("name not trimmed: %q", v.Models[1].Name)
	}
	if v.Agents[0].DefaultModel != "claude-sonnet" {
		t.Fatalf("default model = %q; want the first listed", v.Agents[0].DefaultModel)
	}

	reopened, err := Open(path, isFixture, isWorker)
	if err != nil {
		t.Fatal(err)
	}
	if m := reopened.Migration(); m.From != "" {
		t.Fatalf("v2 file reported a migration: %+v", m)
	}
	r, ok := reopened.Agent("claude")
	if !ok || r.Model == nil || *r.Model != (Model{ID: "claude-sonnet", Name: "Claude Sonnet"}) {
		t.Fatalf("resolved = %+v, %v", r, ok)
	}
	r, ok = reopened.Agent("coder")
	if !ok || r.Model != nil || r.Agent.Worker != "opencode" || r.Agent.RouterAgent != "" {
		t.Fatalf("session agent = %+v, %v", r, ok)
	}
	// A worker removed from the agents file makes the catalog invalid, as a
	// reserved ID does: the operator must fix one of the two files.
	if _, err := Open(path, isFixture, nil); err == nil || !strings.Contains(err.Error(), "worker") {
		t.Fatalf("open without the worker = %v", err)
	}
}

func TestCatalogValidation(t *testing.T) {
	s := seeded(t, "")
	before := s.View()
	var invalidErr *ValidationError
	for name, err := range map[string]error{
		"model ID with space":   func() error { _, e := s.PutModel("a b", Model{Name: "M"}); return e }(),
		"model ID leading dash": func() error { _, e := s.PutModel("-m", Model{Name: "M"}); return e }(),
		"model ID too long":     func() error { _, e := s.PutModel(strings.Repeat("m", 201), Model{Name: "M"}); return e }(),
		"model no name":         func() error { _, e := s.PutModel("m", Model{Name: " "}); return e }(),
		"unknown harness":       func() error { _, e := s.PutAgent("a", Agent{Name: "A", Harness: "gpt", RouterAgent: "r"}); return e }(),
		"bad router agent": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "sha256:abc"})
			return e
		}(),
		"router agent with worker": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "r", Worker: "opencode"})
			return e
		}(),
		"opencode without worker": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: HarnessOpenCode})
			return e
		}(),
		"opencode unknown worker": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: HarnessOpenCode, Worker: "elsewhere"})
			return e
		}(),
		"opencode with router agent": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: HarnessOpenCode, Worker: "opencode", RouterAgent: "r"})
			return e
		}(),
		"unknown model": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "r", Models: []string{"nope"}})
			return e
		}(),
		"default unlisted": func() error {
			_, e := s.PutAgent("a", Agent{Name: "A", Harness: "codex", RouterAgent: "r", Models: []string{"ollama/qwen3:8b"}, DefaultModel: "claude-sonnet"})
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
	if len(after.Models) != len(before.Models) || len(after.Agents) != len(before.Agents) {
		t.Fatal("a rejected change altered the catalog")
	}
}

func TestCatalogDeleteChecksReferences(t *testing.T) {
	s := seeded(t, "")
	if err := s.DeleteModel("claude-sonnet"); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete used model = %v", err)
	}
	if _, err := s.PutAgent("claude", Agent{Name: "Claude Code", Harness: "claude-code", RouterAgent: "claude-code", Models: []string{"ollama/qwen3:8b"}, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Agent("claude"); ok {
		t.Fatal("a disabled agent resolved for chat")
	}
	if err := s.DeleteModel("claude-sonnet"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent("claude"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent("claude"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}

func TestOpenMigratesV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	v1 := `{
  "schema": "ember.ai-catalog/v1",
  "providers": [
    {"id":"anthropic","name":"Anthropic","kind":"anthropic","baseURL":"https://api.anthropic.com/v1","local":false,"apiKey":"sk-ant-very-secret"},
    {"id":"lan","name":"LAN","kind":"ollama","baseURL":"http://ollama.lan:11434","local":true}
  ],
  "models": [
    {"id":"sonnet","name":"Claude Sonnet","provider":"anthropic","upstream":"claude-sonnet-x"},
    {"id":"qwen","name":"Qwen","provider":"lan","upstream":"qwen3:8b"}
  ],
  "agents": [
    {"id":"claude","name":"Claude Code","harness":"claude-code","routerAgent":"claude-code","models":["sonnet","qwen"],"defaultModel":"sonnet","enabled":true},
    {"id":"oc","name":"OpenCode image","harness":"opencode","routerAgent":"opencode-image","models":[],"enabled":false}
  ]
}`
	if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, isFixture, isWorker)
	if err != nil {
		t.Fatal(err)
	}
	m := s.Migration()
	if m.From != "ember.ai-catalog/v1" || m.DroppedProviders != 2 || m.DroppedKeys != 1 || !slices.Equal(m.Relabelled, []string{"oc"}) {
		t.Fatalf("migration = %+v", m)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"sk-ant", "providers", "upstream", "ember.ai-catalog/v1"} {
		if strings.Contains(string(data), gone) {
			t.Errorf("rewritten file still contains %q:\n%s", gone, data)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("rewritten file mode = %v; want 0600", info.Mode().Perm())
	}
	v := s.View()
	if !slices.Equal(v.Models, []Model{{ID: "sonnet", Name: "Claude Sonnet"}, {ID: "qwen", Name: "Qwen"}}) {
		t.Fatalf("models = %+v", v.Models)
	}
	if len(v.Agents) != 2 || v.Agents[0].DefaultModel != "sonnet" || v.Agents[1].Harness != "custom" || v.Agents[1].RouterAgent != "opencode-image" {
		t.Fatalf("agents = %+v", v.Agents)
	}
	// The rewritten file opens as v2 with nothing left to migrate.
	again, err := Open(path, isFixture, isWorker)
	if err != nil || again.Migration().From != "" {
		t.Fatalf("reopen = %+v, %v", again.Migration(), err)
	}
}

func TestOpenRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open("relative.json", nil, nil); err == nil {
		t.Fatal("accepted a relative path")
	}
	if s, err := Open(filepath.Join(dir, "missing.json"), nil, nil); err != nil || len(s.View().Models) != 0 {
		t.Fatalf("missing file = %v; want an empty catalog", err)
	}
	for name, doc := range map[string]string{
		"other schema":   `{"schema":"x","models":[],"agents":[]}`,
		"dangling":       `{"schema":"ember.ai-catalog/v2","models":[],"agents":[{"id":"a","name":"A","harness":"codex","routerAgent":"r","worker":"","models":["m"],"defaultModel":"m","enabled":true}]}`,
		"v2 with keys":   `{"schema":"ember.ai-catalog/v2","providers":[{"apiKey":"k"}],"models":[],"agents":[]}`,
		"v1 bad agent":   `{"schema":"ember.ai-catalog/v1","providers":[],"models":[],"agents":[{"id":"Bad","name":"A","harness":"codex","routerAgent":"r","models":[],"enabled":true}]}`,
		"not json":       `{`,
		"reserved agent": `{"schema":"ember.ai-catalog/v2","models":[],"agents":[{"id":"chat-fixture","name":"A","harness":"codex","routerAgent":"r","worker":"","models":[],"enabled":true}]}`,
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path, isFixture, isWorker); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A rejected v1 file is left as it was, keys included, for the operator.
	data, _ := os.ReadFile(filepath.Join(dir, "v1-bad-agent.json"))
	if !strings.Contains(string(data), "ember.ai-catalog/v1") {
		t.Fatal("a v1 file that failed validation was rewritten")
	}
}

func TestNewGateway(t *testing.T) {
	for _, ok := range []string{"http://litellm:4000", "https://gateway.example", "http://litellm:4000/"} {
		g, err := NewGateway(ok, "sk-ember")
		if err != nil {
			t.Errorf("%s: %v", ok, err)
			continue
		}
		if strings.HasSuffix(g.Origin(), "/") {
			t.Errorf("%s: origin %q keeps a slash", ok, g.Origin())
		}
	}
	for _, bad := range []string{"", "litellm:4000", "ftp://x", "http://u:p@x", "http://x/v1", "http://x?a=b", "http://x#f", "http://"} {
		if _, err := NewGateway(bad, "k"); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
	for _, key := range []string{"", "a b", "a\nb", strings.Repeat("k", 1025)} {
		if _, err := NewGateway("http://x", key); err == nil {
			t.Errorf("key %q: accepted", key)
		}
	}
}

func TestLoadGateway(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte("sk-ember\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if g, err := LoadGateway("", ""); g != nil || err != nil {
		t.Fatalf("unset = %v, %v; want no gateway", g, err)
	}
	if _, err := LoadGateway("http://litellm:4000", ""); err == nil {
		t.Fatal("accepted a URL without a key file")
	}
	if _, err := LoadGateway("", keyFile); err == nil {
		t.Fatal("accepted a key file without a URL")
	}
	if _, err := LoadGateway("http://litellm:4000", filepath.Join(dir, "missing")); err == nil {
		t.Fatal("accepted a missing key file")
	}
	g, err := LoadGateway("http://litellm:4000", keyFile)
	if err != nil || g.key != "sk-ember" {
		t.Fatalf("load = %+v, %v; want the trimmed key", g, err)
	}
}

func TestGatewayModels(t *testing.T) {
	// The race detector does not see the HTTP round trip as a sync point.
	var mu sync.Mutex
	var gotPath, gotAuth string
	var handler http.HandlerFunc
	answer := func(h http.HandlerFunc) { mu.Lock(); handler = h; mu.Unlock() }
	got := func() (string, string) { mu.Lock(); defer mu.Unlock(); return gotPath, gotAuth }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		h := handler
		mu.Unlock()
		h(w, r)
	}))
	defer upstream.Close()
	g, err := NewGateway(upstream.URL, "sk-ember")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	answer(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"b-model"},{"id":"openai/gpt-4o"},{"id":"b-model"},{"id":"has space"},{"id":""}]}`))
	})
	ids, err := g.Models(ctx)
	path, auth := got()
	if err != nil || !slices.Equal(ids, []string{"b-model", "openai/gpt-4o"}) || path != "/v1/models" || auth != "Bearer sk-ember" {
		t.Fatalf("models = %v, %v at %s, auth %q", ids, err, path, auth)
	}

	var gwErr *GatewayError
	answer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"secret upstream detail"}`))
	})
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) || strings.Contains(gwErr.Msg, "secret") || !strings.Contains(gwErr.Msg, "401") {
		t.Fatalf("denied = %v", err)
	}
	answer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`provider key sk-upstream leaked in a stack trace`))
	})
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) || strings.Contains(gwErr.Msg, "sk-upstream") || !strings.Contains(gwErr.Msg, "500") {
		t.Fatalf("server error = %v", err)
	}
	answer(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/v1/models", http.StatusFound)
	})
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) {
		t.Fatalf("redirect = %v; want refused, not followed", err)
	}
	answer(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<html>login</html>`)) })
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) {
		t.Fatalf("not JSON = %v", err)
	}
	answer(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[` + strings.Repeat(`{"id":"xxxxxxxxxxxxxxxx"},`, maxModelList/26+1) + `{"id":"y"}]}`))
	})
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) || !strings.Contains(gwErr.Msg, "could not be read") {
		t.Fatalf("oversized = %v", err)
	}

	answer(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) })
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err = g.Models(short); !errors.As(err, &gwErr) || !strings.Contains(gwErr.Msg, "in time") {
		t.Fatalf("timeout = %v", err)
	}

	upstream.Close()
	if _, err = g.Models(ctx); !errors.As(err, &gwErr) || !strings.Contains(gwErr.Msg, "not reachable") {
		t.Fatalf("unreachable = %v", err)
	}
}
