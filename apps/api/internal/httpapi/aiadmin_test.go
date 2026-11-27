package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

// aiFixture is an API with chat runs on and an empty AI catalog.
func aiFixture(t *testing.T) (admin, user http.Handler, runner *fakeRunner) {
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
	catalog, err := aicatalog.Open("", func(id string) bool { _, ok := config.Agent(id); return ok })
	if err != nil {
		t.Fatal(err)
	}
	runner = &fakeRunner{calls: make(chan *runCall, 4)}
	runs := NewChatRuns(config, runner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { runs.Close(2 * time.Second) })
	api := NewDeploymentHandler("test", vaults, nil, nil, WithChatRuns(runs), WithAICatalog(catalog))
	return as(auth.RoleAdmin, api), as(auth.RoleUser, api), runner
}

func must(t *testing.T, rr *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status = %d, body %s; want %d", rr.Code, rr.Body, status)
	}
}

func TestAIAdminNeedsAdmin(t *testing.T) {
	_, user, _ := aiFixture(t)
	for _, r := range [][3]string{
		{"GET", "/v1/admin/ai", ""},
		{"PUT", "/v1/admin/ai/providers/p", `{"name":"P","kind":"ollama","baseURL":"http://x","local":true}`},
		{"DELETE", "/v1/admin/ai/providers/p", ""},
		{"POST", "/v1/admin/ai/providers/p/discover", ""},
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

func TestAIAdminConfiguresAgentForChat(t *testing.T) {
	admin, user, runner := aiFixture(t)

	must(t, send(admin, "PUT", "/v1/admin/ai/providers/anthropic", `{"name":"Anthropic","kind":"anthropic","apiKey":"sk-ant-very-secret"}`), 200)
	must(t, send(admin, "PUT", "/v1/admin/ai/models/sonnet", `{"name":"Claude Sonnet","provider":"anthropic","upstream":"claude-sonnet-x"}`), 200)
	// The operator's agents file already uses this ID.
	expect(t, send(admin, "PUT", "/v1/admin/ai/agents/chat-fixture", `{"name":"X","harness":"custom","routerAgent":"r","models":[],"enabled":true}`), 422, "invalid_configuration")
	must(t, send(admin, "PUT", "/v1/admin/ai/agents/claude", `{"name":"Claude Code","harness":"claude-code","routerAgent":"claude-code-image","models":["sonnet"],"enabled":true}`), 200)
	expect(t, send(admin, "DELETE", "/v1/admin/ai/models/sonnet", ""), 409, "in_use")

	view := send(admin, "GET", "/v1/admin/ai", "")
	must(t, view, 200)
	if strings.Contains(view.Body.String(), "sk-ant") {
		t.Fatal("GET /v1/admin/ai returned a provider key")
	}
	body := decode[struct {
		ChatEnabled    bool                       `json:"chatEnabled"`
		OperatorAgents []agentInfo                `json:"operatorAgents"`
		Providers      []aicatalog.PublicProvider `json:"providers"`
	}](t, view)
	if !body.ChatEnabled || len(body.OperatorAgents) != 2 || len(body.Providers) != 1 || !body.Providers[0].HasAPIKey {
		t.Fatalf("view = %+v", body)
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
	want := agents.ModelRef{ID: "sonnet", Name: "Claude Sonnet", ProviderKind: "anthropic", Upstream: "claude-sonnet-x"}
	if call.turn.Model == nil || *call.turn.Model != want {
		t.Fatalf("turn model = %+v; want %+v", call.turn.Model, want)
	}
	call.reply <- runReply{result: agents.Result{Output: "ok"}}

	// Disabling the agent hides it and stops new turns; the chat stays readable.
	must(t, send(admin, "PUT", "/v1/admin/ai/agents/claude", `{"name":"Claude Code","harness":"claude-code","routerAgent":"claude-code-image","models":["sonnet"],"enabled":false}`), 200)
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

func TestAIAdminDiscover(t *testing.T) {
	admin, _, _ := aiFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:8b"}]}`))
	}))
	defer upstream.Close()
	expect(t, send(admin, "POST", "/v1/admin/ai/providers/none/discover", ""), 404, "not_found")
	must(t, send(admin, "PUT", "/v1/admin/ai/providers/lan", `{"name":"LAN Ollama","kind":"ollama","baseURL":"`+upstream.URL+`","local":true}`), 200)
	rr := send(admin, "POST", "/v1/admin/ai/providers/lan/discover", "")
	must(t, rr, 200)
	if got := strings.TrimSpace(rr.Body.String()); got != `{"models":["qwen3:8b"]}` {
		t.Fatalf("discover = %s", got)
	}
	upstream.Close()
	expect(t, send(admin, "POST", "/v1/admin/ai/providers/lan/discover", ""), 502, "discovery_failed")
}
