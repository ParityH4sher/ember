package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

// aiFixture is an API with chat runs on, an empty AI catalog, one worker and
// the given model gateway (nil for none).
func aiFixture(t *testing.T, gateway *aicatalog.Gateway) (admin, user http.Handler, runner *fakeRunner) {
	t.Helper()
	vaults, err := projects.OpenVaults(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create(projects.Project{ID: "desk", Name: "Desk organizer"}); err != nil {
		t.Fatal(err)
	}
	config, err := agents.ParseConfig([]byte(testAgents))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := aicatalog.Open("",
		func(id string) bool { _, ok := config.Agent(id); return ok },
		func(id string) bool { return id == "opencode" })
	if err != nil {
		t.Fatal(err)
	}
	runner = &fakeRunner{calls: make(chan *runCall, 4)}
	runs := NewChatRuns(config, runner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { runs.Close(2 * time.Second) })
	api := NewDeploymentHandler("test", vaults, nil, nil, WithChatRuns(runs), WithAICatalog(catalog),
		WithModelGateway(gateway), WithWorkers([]WorkerInfo{{ID: "opencode", Harness: "opencode"}}))
	return as(auth.RoleAdmin, api), as(auth.RoleUser, api), runner
}

func must(t *testing.T, rr *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status = %d, body %s; want %d", rr.Code, rr.Body, status)
	}
}

func TestAIAdminNeedsAdmin(t *testing.T) {
	_, user, _ := aiFixture(t, nil)
	for _, r := range [][3]string{
		{"GET", "/v1/admin/ai", ""},
		{"GET", "/v1/admin/ai/gateway/models", ""},
		{"PUT", "/v1/admin/ai/models/m", `{}`},
		{"DELETE", "/v1/admin/ai/models/m", ""},
		{"PUT", "/v1/admin/ai/agents/a", `{}`},
		{"DELETE", "/v1/admin/ai/agents/a", ""},
	} {
		rr := send(user, r[0], r[1], r[2])
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s %s as user = %d; want 403", r[0], r[1], rr.Code)
		}
	}
}

func TestAIAdminProviderRoutesRemoved(t *testing.T) {
	admin, _, _ := aiFixture(t, nil)
	for _, r := range [][3]string{
		{"PUT", "/v1/admin/ai/providers/p", `{"name":"P","kind":"ollama","baseURL":"http://x","local":true}`},
		{"DELETE", "/v1/admin/ai/providers/p", ""},
		{"POST", "/v1/admin/ai/providers/p/discover", ""},
	} {
		if rr := send(admin, r[0], r[1], r[2]); rr.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d; want 404", r[0], r[1], rr.Code)
		}
	}
	// A model can no longer carry provider details.
	expect(t, send(admin, "PUT", "/v1/admin/ai/models/m", `{"name":"M","provider":"p","upstream":"u"}`), 400, "invalid_request")
}

func TestAIAdminConfiguresAgentForChat(t *testing.T) {
	admin, user, runner := aiFixture(t, nil)

	// A gateway model name may contain a slash; it travels percent-encoded.
	saved := send(admin, "PUT", "/v1/admin/ai/models/anthropic%2Fclaude-sonnet", `{"name":"Claude Sonnet"}`)
	must(t, saved, 200)
	if got := decode[aicatalog.Model](t, saved); got != (aicatalog.Model{ID: "anthropic/claude-sonnet", Name: "Claude Sonnet"}) {
		t.Fatalf("saved model = %+v", got)
	}
	expect(t, send(admin, "PUT", "/v1/admin/ai/models/has%20space", `{"name":"X"}`), 422, "invalid_configuration")
	// The operator's agents file already uses this ID.
	expect(t, send(admin, "PUT", "/v1/admin/ai/agents/chat-fixture", `{"name":"X","harness":"custom","routerAgent":"r","models":[],"enabled":true}`), 422, "invalid_configuration")
	must(t, send(admin, "PUT", "/v1/admin/ai/agents/claude", `{"name":"Claude Code","harness":"claude-code","routerAgent":"claude-code-image","models":["anthropic/claude-sonnet"],"enabled":true}`), 200)
	expect(t, send(admin, "DELETE", "/v1/admin/ai/models/anthropic%2Fclaude-sonnet", ""), 409, "in_use")

	// Session agents name a configured worker instead of a router agent.
	expect(t, send(admin, "PUT", "/v1/admin/ai/agents/coder", `{"name":"Coder","harness":"opencode","worker":"elsewhere","enabled":false}`), 422, "invalid_configuration")
	expect(t, send(admin, "PUT", "/v1/admin/ai/agents/coder", `{"name":"Coder","harness":"opencode","routerAgent":"r","worker":"opencode","enabled":false}`), 422, "invalid_configuration")
	must(t, send(admin, "PUT", "/v1/admin/ai/agents/coder", `{"name":"Coder","harness":"opencode","worker":"opencode","enabled":false}`), 200)

	view := send(admin, "GET", "/v1/admin/ai", "")
	must(t, view, 200)
	if view.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("catalog view is cacheable")
	}
	body := decode[struct {
		Persisted      bool              `json:"persisted"`
		ChatEnabled    bool              `json:"chatEnabled"`
		Harnesses      []string          `json:"harnesses"`
		Workers        []WorkerInfo      `json:"workers"`
		Gateway        map[string]bool   `json:"gateway"`
		OperatorAgents []agentInfo       `json:"operatorAgents"`
		Models         []aicatalog.Model `json:"models"`
		Agents         []aicatalog.Agent `json:"agents"`
	}](t, view)
	if body.Persisted || !body.ChatEnabled || len(body.Harnesses) != 4 || len(body.OperatorAgents) != 2 ||
		len(body.Workers) != 1 || body.Workers[0] != (WorkerInfo{ID: "opencode", Harness: "opencode"}) ||
		body.Gateway["configured"] || len(body.Models) != 1 || len(body.Agents) != 2 || body.Agents[1].Worker != "opencode" {
		t.Fatalf("view = %+v", body)
	}
	if strings.Contains(view.Body.String(), "providers") {
		t.Fatal("GET /v1/admin/ai still reports providers")
	}

	listed := decode[struct {
		Agents []agentInfo `json:"agents"`
	}](t, send(user, "GET", "/v1/agents", ""))
	if len(listed.Agents) != 3 || listed.Agents[2] != (agentInfo{ID: "claude", Name: "Claude Code", Harness: "claude-code", Model: "Claude Sonnet"}) {
		t.Fatalf("agents = %+v", listed.Agents)
	}

	created := send(user, "POST", "/v1/projects/desk/chats", `{"agent":"claude"}`)
	must(t, created, 201)
	chat := decode[projects.Chat](t, created)
	must(t, send(user, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"Plan a desk organizer"}`), 202)
	call := runner.next(t)
	if call.agent != "claude-code-image" {
		t.Fatalf("router agent = %q; want the catalog agent's routerAgent", call.agent)
	}
	want := agents.ModelRef{ID: "anthropic/claude-sonnet", Name: "Claude Sonnet"}
	if call.turn.Model == nil || *call.turn.Model != want {
		t.Fatalf("turn model = %+v; want %+v", call.turn.Model, want)
	}
	call.reply <- runReply{result: agents.Result{Output: "ok"}}

	// Disabling the agent hides it and stops new turns; the chat stays readable.
	must(t, send(admin, "PUT", "/v1/admin/ai/agents/claude", `{"name":"Claude Code","harness":"claude-code","routerAgent":"claude-code-image","models":["anthropic/claude-sonnet"],"enabled":false}`), 200)
	deadline := time.Now().Add(5 * time.Second)
	for {
		rr := send(user, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"again"}`)
		if rr.Code == http.StatusServiceUnavailable {
			break
		}
		if rr.Code != http.StatusConflict || time.Now().After(deadline) {
			t.Fatalf("send to disabled agent = %d %s", rr.Code, rr.Body)
		}
		time.Sleep(20 * time.Millisecond)
	}
	must(t, send(user, "GET", "/v1/projects/desk/chats/"+chat.ID, ""), 200)
}

func TestAIAdminGatewayModels(t *testing.T) {
	admin, _, _ := aiFixture(t, nil)
	expect(t, send(admin, "GET", "/v1/admin/ai/gateway/models", ""), 503, "gateway_unavailable")

	var status atomic.Int32 // the race detector does not see the HTTP round trip
	status.Store(http.StatusOK)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer sk-ember-gateway" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen3:8b"},{"id":"anthropic/claude-sonnet"}],"detail":"upstream secret"}`))
	}))
	defer upstream.Close()
	gateway, err := aicatalog.NewGateway(upstream.URL, "sk-ember-gateway")
	if err != nil {
		t.Fatal(err)
	}
	admin, _, _ = aiFixture(t, gateway)
	view := decode[struct {
		Gateway map[string]bool `json:"gateway"`
	}](t, send(admin, "GET", "/v1/admin/ai", ""))
	if !view.Gateway["configured"] {
		t.Fatal("gateway.configured = false with a gateway")
	}
	rr := send(admin, "GET", "/v1/admin/ai/gateway/models", "")
	must(t, rr, 200)
	if got := strings.TrimSpace(rr.Body.String()); got != `{"models":[{"id":"anthropic/claude-sonnet"},{"id":"qwen3:8b"}]}` {
		t.Fatalf("gateway models = %s", got)
	}
	status.Store(http.StatusBadGateway)
	rr = send(admin, "GET", "/v1/admin/ai/gateway/models", "")
	expect(t, rr, 503, "gateway_unavailable")
	if strings.Contains(rr.Body.String(), "secret") {
		t.Fatalf("gateway error passed on the upstream body: %s", rr.Body)
	}
	upstream.Close()
	expect(t, send(admin, "GET", "/v1/admin/ai/gateway/models", ""), 503, "gateway_unavailable")
}
