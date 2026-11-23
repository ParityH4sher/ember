package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

// fakeRunner hands each run to the test, which decides when and how it ends.
type fakeRunner struct{ calls chan *runCall }

type runCall struct {
	agent string
	turn  agents.Turn
	ctx   context.Context
	reply chan runReply
}

type runReply struct {
	result agents.Result
	err    error
}

func (f *fakeRunner) Run(ctx context.Context, agent string, turn agents.Turn) (agents.Result, error) {
	call := &runCall{agent: agent, turn: turn, ctx: ctx, reply: make(chan runReply, 1)}
	f.calls <- call
	select {
	case r := <-call.reply:
		return r.result, r.err
	case <-ctx.Done():
		return agents.Result{}, &agents.RunError{Code: agents.CodeInterrupted}
	}
}

func (f *fakeRunner) next(t *testing.T) *runCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no agent run started")
		return nil
	}
}

type chatFixture struct {
	handler http.Handler // as a signed-in user
	noRole  http.Handler // as a request that did not pass through auth.Wrap
	vaults  *projects.Vaults
	runner  *fakeRunner
	runs    *ChatRuns
}

const testAgents = `{"routerSocket":"/run/router.sock","agents":[{"id":"chat-fixture","name":"Offline fixture"},{"id":"other","name":"Other"}]}`

func newChatFixture(t *testing.T, enabled bool) *chatFixture {
	t.Helper()
	vaults, err := projects.OpenVaults(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := vaults.Create(projects.Project{ID: "desk", Name: "Desk organizer"}); err != nil {
		t.Fatal(err)
	}
	f := &chatFixture{vaults: vaults, runner: &fakeRunner{calls: make(chan *runCall, 4)}}
	var opts []Option
	if enabled {
		config, err := agents.ParseConfig([]byte(testAgents))
		if err != nil {
			t.Fatal(err)
		}
		f.runs = NewChatRuns(config, f.runner, slog.New(slog.NewTextHandler(io.Discard, nil)))
		t.Cleanup(func() { f.runs.Close(2 * time.Second) })
		opts = append(opts, WithChatRuns(f.runs))
	}
	f.noRole = NewDeploymentHandler("test", vaults, nil, nil, opts...)
	f.handler = as(auth.RoleUser, f.noRole)
	return f
}

func (f *chatFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, httptest.NewRequest(method, path, reader))
	return rr
}

func decode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return v
}

func expect(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status %d, want %d: %s", rr.Code, status, rr.Body.String())
	}
	if code != "" {
		if got := decode[map[string]string](t, rr)["code"]; got != code {
			t.Fatalf("code %q, want %q", got, code)
		}
	}
}

func (f *chatFixture) createChat(t *testing.T) projects.Chat {
	t.Helper()
	rr := f.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture"}`)
	expect(t, rr, http.StatusCreated, "")
	return decode[projects.Chat](t, rr)
}

// settled polls the chat as the browser does until no message is pending.
func (f *chatFixture) settled(t *testing.T, chatID string) projects.Chat {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rr := f.do(t, "GET", "/v1/projects/desk/chats/"+chatID, "")
		expect(t, rr, http.StatusOK, "")
		chat := decode[projects.Chat](t, rr)
		pending := false
		for _, m := range chat.Messages {
			pending = pending || m.Status == projects.StatusPending
		}
		if !pending {
			return chat
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("chat stayed pending")
	return projects.Chat{}
}

func TestAgentsListing(t *testing.T) {
	rr := newChatFixture(t, false).do(t, "GET", "/v1/agents", "")
	expect(t, rr, http.StatusOK, "")
	if body := strings.TrimSpace(rr.Body.String()); body != `{"agents":[],"enabled":false}` {
		t.Fatalf("disabled listing = %s", body)
	}
	rr = newChatFixture(t, true).do(t, "GET", "/v1/agents", "")
	list := decode[struct {
		Enabled bool
		Agents  []agents.Agent
	}](t, rr)
	if !list.Enabled || len(list.Agents) != 2 || list.Agents[0] != (agents.Agent{ID: "chat-fixture", Name: "Offline fixture"}) {
		t.Fatalf("enabled listing = %+v", list)
	}
}

func TestChatTurnRoundTrip(t *testing.T) {
	f := newChatFixture(t, true)
	chat := f.createChat(t)
	if chat.Schema != projects.ChatSchema || chat.Title != "" || chat.Agent != "chat-fixture" || len(chat.Messages) != 0 {
		t.Fatalf("created chat = %+v", chat)
	}

	rr := f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"  Which filament <PETG> & why?\nDetails follow.  "}`)
	expect(t, rr, http.StatusAccepted, "")
	accepted := decode[projects.Chat](t, rr)
	if len(accepted.Messages) != 2 || accepted.Messages[1].Status != projects.StatusPending ||
		accepted.Messages[0].Content != "Which filament <PETG> & why?\nDetails follow." || accepted.Title != "Which filament <PETG> & why?" {
		t.Fatalf("accepted = %+v", accepted)
	}
	summaries := decode[map[string][]chatSummary](t, f.do(t, "GET", "/v1/projects/desk/chats", ""))["chats"]
	if len(summaries) != 1 || !summaries[0].Pending || summaries[0].MessageCount != 2 {
		t.Fatalf("summaries while running = %+v", summaries)
	}

	call := f.runner.next(t)
	turn := call.turn
	if call.agent != "chat-fixture" || turn.Schema != agents.TurnSchema || turn.Project != (agents.ProjectRef{ID: "desk", Name: "Desk organizer"}) ||
		turn.Chat.ID != chat.ID || len(turn.Messages) != 1 || turn.Messages[0].Role != "user" {
		t.Fatalf("turn = %+v", turn)
	}
	call.reply <- runReply{result: agents.Result{RunID: "ember-run-1", Output: "PETG: tough and easy to print."}}

	done := f.settled(t, chat.ID)
	answer := done.Messages[1]
	if answer.Status != projects.StatusCompleted || answer.Content != "PETG: tough and easy to print." || answer.Run != "ember-run-1" || answer.Error != "" {
		t.Fatalf("answer = %+v", answer)
	}

	// The next turn carries the conversation so far.
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"And for outdoors?"}`), http.StatusAccepted, "")
	call = f.runner.next(t)
	got := call.turn.Messages
	if len(got) != 3 || got[1] != (agents.Message{Role: "agent", Content: "PETG: tough and easy to print."}) || got[2].Content != "And for outdoors?" {
		t.Fatalf("second turn = %+v", got)
	}
	call.reply <- runReply{result: agents.Result{Output: "ASA."}}
	if done := f.settled(t, chat.ID); len(done.Messages) != 4 || done.Messages[3].Content != "ASA." || done.Title != accepted.Title {
		t.Fatalf("after second turn = %+v", done)
	}
}

func TestChatBusy(t *testing.T) {
	f := newChatFixture(t, true)
	first, second := f.createChat(t), f.createChat(t)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+first.ID+"/messages", `{"content":"one"}`), http.StatusAccepted, "")
	call := f.runner.next(t)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+first.ID+"/messages", `{"content":"two"}`), http.StatusConflict, "chat_busy")
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+second.ID+"/messages", `{"content":"three"}`), http.StatusTooManyRequests, "agent_busy")
	if c, _ := f.vaults.Chat("desk", second.ID); len(c.Messages) != 0 {
		t.Fatal("a refused message was saved")
	}
	call.reply <- runReply{result: agents.Result{Output: "done"}}
	f.settled(t, first.ID)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+second.ID+"/messages", `{"content":"three"}`), http.StatusAccepted, "")
	f.runner.next(t).reply <- runReply{result: agents.Result{Output: "ok"}}
	f.settled(t, second.ID)
}

// A client that sees the answer may send the next message at once, in the
// same chat (the chat claim) or in another one (the run slot).
func TestChatSendRightAfterAnswer(t *testing.T) {
	f := newChatFixture(t, true)
	chats := []projects.Chat{f.createChat(t), f.createChat(t)}
	for i := range 40 {
		chat := chats[(i/2)%2]
		expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"next"}`), http.StatusAccepted, "")
		f.runner.next(t).reply <- runReply{result: agents.Result{Output: "ok"}}
		if got := f.settled(t, chat.ID).Messages; got[len(got)-1].Status != projects.StatusCompleted {
			t.Fatalf("turn %d ended as %+v", i, got[len(got)-1])
		}
	}
}

func TestChatRunFailuresAreSanitized(t *testing.T) {
	f := newChatFixture(t, true)
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&agents.RunError{Code: agents.CodeTimeout, RunID: "ember-run-2"}, agents.UserMessage(agents.CodeTimeout)},
		{errors.New("podman: exit status 125 at /var/lib/containers"), agents.UserMessage(agents.CodeFailed)},
	} {
		chat := f.createChat(t)
		expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"hello"}`), http.StatusAccepted, "")
		f.runner.next(t).reply <- runReply{err: tc.err}
		failed := f.settled(t, chat.ID).Messages[1]
		if failed.Status != projects.StatusFailed || failed.Error != tc.want || failed.Content != "" || strings.Contains(failed.Error, "/var") {
			t.Fatalf("failed message = %+v", failed)
		}
	}
	// A failed turn is not sent to the agent again.
	chat := f.createChat(t)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"first"}`), http.StatusAccepted, "")
	f.runner.next(t).reply <- runReply{err: &agents.RunError{Code: agents.CodeFailed}}
	f.settled(t, chat.ID)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"second"}`), http.StatusAccepted, "")
	call := f.runner.next(t)
	if msgs := call.turn.Messages; len(msgs) != 2 || msgs[0].Content != "first" || msgs[1].Content != "second" {
		t.Fatalf("turn after a failure = %+v", msgs)
	}
	call.reply <- runReply{result: agents.Result{Output: "ok"}}
	f.settled(t, chat.ID)
}

func TestChatCloseInterruptsRuns(t *testing.T) {
	f := newChatFixture(t, true)
	chat := f.createChat(t)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"hello"}`), http.StatusAccepted, "")
	call := f.runner.next(t)
	f.runs.Close(2 * time.Second)
	if call.ctx.Err() == nil {
		t.Fatal("Close did not cancel the run")
	}
	stored, err := f.vaults.Chat("desk", chat.ID)
	if err != nil || stored.Messages[1].Status != projects.StatusFailed || stored.Messages[1].Error != agents.UserMessage(agents.CodeInterrupted) {
		t.Fatalf("stored after close = %+v, %v", stored.Messages, err)
	}
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"again"}`), http.StatusServiceUnavailable, "agents_unavailable")
}

func TestChatStalePendingIsInterrupted(t *testing.T) {
	f := newChatFixture(t, true)
	chat := f.createChat(t)
	// As if the API had stopped while an agent was answering.
	if _, err := f.vaults.UpdateChat("desk", chat.ID, 0, func(c *projects.Chat) error {
		c.Messages = append(c.Messages,
			projects.ChatMessage{ID: projects.NewID(), Role: "user", Content: "lost"},
			projects.ChatMessage{ID: projects.NewID(), Role: "agent", Status: projects.StatusPending})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	shown := decode[projects.Chat](t, f.do(t, "GET", "/v1/projects/desk/chats/"+chat.ID, ""))
	if shown.Messages[1].Status != projects.StatusFailed || shown.Messages[1].Error != agents.UserMessage(agents.CodeInterrupted) {
		t.Fatalf("shown = %+v", shown.Messages[1])
	}
	if summaries := decode[map[string][]chatSummary](t, f.do(t, "GET", "/v1/projects/desk/chats", ""))["chats"]; summaries[0].Pending {
		t.Fatal("listed a stale run as pending")
	}
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+chat.ID+"/messages", `{"content":"retry"}`), http.StatusAccepted, "")
	call := f.runner.next(t)
	if stored, _ := f.vaults.Chat("desk", chat.ID); stored.Messages[1].Status != projects.StatusFailed {
		t.Fatal("the stale pending message was not corrected on the next write")
	}
	call.reply <- runReply{result: agents.Result{Output: "ok"}}
	f.settled(t, chat.ID)
}

func TestChatValidation(t *testing.T) {
	off := newChatFixture(t, false)
	expect(t, off.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture"}`), http.StatusServiceUnavailable, "agents_unavailable")
	expect(t, off.do(t, "GET", "/v1/projects/desk/chats", ""), http.StatusOK, "")
	existing := projects.Chat{Schema: projects.ChatSchema, ID: projects.NewID(), Agent: "chat-fixture", Messages: []projects.ChatMessage{}}
	if err := off.vaults.CreateChat("desk", existing); err != nil {
		t.Fatal(err)
	}
	expect(t, off.do(t, "GET", "/v1/projects/desk/chats/"+existing.ID, ""), http.StatusOK, "")
	expect(t, off.do(t, "POST", "/v1/projects/desk/chats/"+existing.ID+"/messages", `{"content":"hi"}`), http.StatusServiceUnavailable, "agents_unavailable")

	f := newChatFixture(t, true)
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"missing"}`), http.StatusBadRequest, "unknown_agent")
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture","title":"two\nlines"}`), http.StatusBadRequest, "invalid_chat")
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture","profile":"trusted"}`), http.StatusBadRequest, "invalid_chat")
	expect(t, f.do(t, "POST", "/v1/projects/nope/chats", `{"agent":"chat-fixture"}`), http.StatusNotFound, "project_not_found")
	expect(t, f.do(t, "GET", "/v1/projects/nope/chats", ""), http.StatusNotFound, "project_not_found")
	expect(t, f.do(t, "GET", "/v1/projects/desk/chats/0123456789abcdef", ""), http.StatusNotFound, "chat_not_found")
	expect(t, f.do(t, "GET", "/v1/projects/desk/chats/..%2Frecord", ""), http.StatusNotFound, "chat_not_found")

	titled := decode[projects.Chat](t, f.do(t, "POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture","title":"  Bracket  "}`))
	if titled.Title != "Bracket" {
		t.Fatalf("title = %q", titled.Title)
	}
	chat := f.createChat(t)
	path := "/v1/projects/desk/chats/" + chat.ID + "/messages"
	expect(t, f.do(t, "POST", path, `{"content":"   "}`), http.StatusBadRequest, "invalid_message")
	expect(t, f.do(t, "POST", path, `{}`), http.StatusBadRequest, "invalid_message")
	expect(t, f.do(t, "POST", path, `{"content":"hi","role":"agent"}`), http.StatusBadRequest, "invalid_message")
	expect(t, f.do(t, "POST", path, `{"content":"a\u0000b"}`), http.StatusBadRequest, "invalid_message")
	expect(t, f.do(t, "POST", path, `{"content":"`+strings.Repeat("x", maxChatMessage+1)+`"}`), http.StatusRequestEntityTooLarge, "message_too_large")
	// Within 16 KiB, but too large for the router once escaped twice.
	expect(t, f.do(t, "POST", path, `{"content":"`+strings.Repeat(`\"`, 12<<10)+`"}`), http.StatusRequestEntityTooLarge, "message_too_large")
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/0123456789abcdef/messages", `{"content":"hi"}`), http.StatusNotFound, "chat_not_found")
	if stored, _ := f.vaults.Chat("desk", chat.ID); len(stored.Messages) != 0 {
		t.Fatalf("refused messages were saved: %+v", stored.Messages)
	}

	// A chat whose agent the operator removed stays readable but cannot run.
	orphan := projects.Chat{Schema: projects.ChatSchema, ID: projects.NewID(), Agent: "retired", Messages: []projects.ChatMessage{}}
	if err := f.vaults.CreateChat("desk", orphan); err != nil {
		t.Fatal(err)
	}
	expect(t, f.do(t, "POST", "/v1/projects/desk/chats/"+orphan.ID+"/messages", `{"content":"hi"}`), http.StatusServiceUnavailable, "agents_unavailable")
}

// Every chat route needs the user role; a request without one starts nothing.
func TestChatRoutesNeedUserRole(t *testing.T) {
	f := newChatFixture(t, true)
	chat := f.createChat(t)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/v1/agents", ""},
		{"GET", "/v1/projects/desk/chats", ""},
		{"POST", "/v1/projects/desk/chats", `{"agent":"chat-fixture"}`},
		{"GET", "/v1/projects/desk/chats/" + chat.ID, ""},
		{"POST", "/v1/projects/desk/chats/" + chat.ID + "/messages", `{"content":"hi"}`},
	} {
		rr := httptest.NewRecorder()
		f.noRole.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		expect(t, rr, http.StatusForbidden, "insufficient_role")
	}
	select {
	case <-f.runner.calls:
		t.Fatal("a request without a role started an agent run")
	default:
	}
	if chats, _ := f.vaults.Chats("desk"); len(chats) != 1 || len(chats[0].Messages) != 0 {
		t.Fatalf("a request without a role changed the chats: %+v", chats)
	}
}

func TestChatRoutesNeedFileStore(t *testing.T) {
	h := as(auth.RoleUser, NewHandlerWithPlugins("test", nil))
	for _, path := range []string{"/v1/projects/desk/chats", "/v1/projects/desk/chats/0123456789abcdef"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s on the memory store: %d", path, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/agents", nil))
	expect(t, rr, http.StatusOK, "")
}

func TestTitleFrom(t *testing.T) {
	for in, want := range map[string]string{
		"Short":                   "Short",
		"First line\nsecond":      "First line",
		"tab\tinside":             "tab inside",
		strings.Repeat("é", 61):   strings.Repeat("é", 60) + "…",
		strings.Repeat("ab ", 30): strings.TrimSpace(strings.Repeat("ab ", 20)) + "…",
	} {
		if got := titleFrom(in); got != want {
			t.Errorf("titleFrom(%q) = %q, want %q", in, got, want)
		}
	}
}
