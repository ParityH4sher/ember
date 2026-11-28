package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/harness"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

// fakeHarness is an in-memory worker. Tests write a session's history and
// emit its events.
type fakeHarness struct {
	mu          sync.Mutex
	opened      map[string]bool
	prompts     []string // "session: text"
	history     map[string][]harness.Message
	perms       map[string][]harness.Permission
	replies     []string
	failPrompt  bool
	interrupts  int
	emitOnStop  bool // Interrupt records the end and emits run.interrupted
	stream      chan harness.Event
	subscribed  chan struct{}
	subscribeOK bool
}

func newFakeHarness() *fakeHarness {
	return &fakeHarness{
		opened: map[string]bool{}, history: map[string][]harness.Message{}, perms: map[string][]harness.Permission{},
		subscribed: make(chan struct{}, 16), subscribeOK: true, emitOnStop: true,
	}
}

func (f *fakeHarness) Open(_ context.Context, s harness.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.Directory != "/workspace" {
		return harness.ErrUnavailable
	}
	f.opened[s.ID] = true
	return nil
}

func (f *fakeHarness) Prompt(_ context.Context, id, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPrompt {
		return fmt.Errorf("%w: provider said sk-secret", harness.ErrUnavailable)
	}
	f.prompts = append(f.prompts, id+": "+text)
	f.history[id] = append(f.history[id], harness.Message{Role: harness.RoleUser, Text: text})
	return nil
}

func (f *fakeHarness) Interrupt(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	f.interrupts++
	h := f.history[id]
	running := len(h) > 0 && h[len(h)-1].Role != harness.RoleRun
	emit := running && f.emitOnStop
	if emit {
		f.history[id] = append(h, harness.Message{Role: harness.RoleRun, Text: "interrupted"})
	}
	f.mu.Unlock()
	if emit {
		f.emit(harness.Event{Session: id, Type: harness.RunInterrupted})
	}
	return running, nil
}

func (f *fakeHarness) Messages(_ context.Context, id string, limit int) ([]harness.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.history[id]
	if !ok {
		return nil, harness.ErrNotFound
	}
	if len(h) > limit {
		h = h[len(h)-limit:]
	}
	return append([]harness.Message{}, h...), nil
}

func (f *fakeHarness) Permissions(_ context.Context, id string) ([]harness.Permission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]harness.Permission{}, f.perms[id]...), nil
}

func (f *fakeHarness) Reply(_ context.Context, id, permissionID string, d harness.Decision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.perms[id] {
		if p.ID == permissionID {
			f.replies = append(f.replies, permissionID+" "+string(d))
			return nil
		}
	}
	return harness.ErrNotFound
}

func (f *fakeHarness) Subscribe(ctx context.Context) (<-chan harness.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.subscribeOK {
		return nil, harness.ErrUnavailable
	}
	ch := make(chan harness.Event, 64)
	f.stream = ch
	go func() {
		<-ctx.Done()
		f.closeStream(ch)
	}()
	f.subscribed <- struct{}{}
	return ch, nil
}

func (f *fakeHarness) closeStream(ch chan harness.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stream == ch && ch != nil {
		close(ch)
		f.stream = nil
	}
}

// breakStream fails the current subscription.
func (f *fakeHarness) breakStream() {
	f.mu.Lock()
	ch := f.stream
	f.mu.Unlock()
	f.closeStream(ch)
}

func (f *fakeHarness) emit(e harness.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stream != nil {
		f.stream <- e
	}
}

func (f *fakeHarness) record(id string, msgs ...harness.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history[id] = append(f.history[id], msgs...)
}

func (f *fakeHarness) promptList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.prompts...)
}

// end records an answer and a successful end, then emits the event.
func (f *fakeHarness) end(chatID string, text ...string) {
	id := harness.SessionID(chatID)
	for _, t := range text {
		f.record(id, harness.Message{Role: harness.RoleAgent, Text: t})
	}
	f.record(id, harness.Message{Role: harness.RoleRun, Text: "succeeded"})
	f.emit(harness.Event{Session: id, Type: harness.RunSucceeded})
}

func (f *fakeHarness) waitSubscribed(t *testing.T) {
	t.Helper()
	select {
	case <-f.subscribed:
	case <-time.After(5 * time.Second):
		t.Fatal("the API did not subscribe to the worker")
	}
}

func newSessionFixture(t *testing.T) (*chatFixture, *fakeHarness) {
	t.Helper()
	vaults, err := projects.OpenVaults(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []projects.Project{{ID: "desk", Name: "Desk organizer"}, {ID: "shed", Name: "Shed"}} {
		if err := vaults.Create(p); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(secret, []byte(strings.Repeat("p", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := agents.ParseConfig([]byte(`{"routerSocket":"/run/router.sock",
		"workers":[{"id":"opencode","harness":"opencode","url":"http://opencode-worker:4096","passwordFile":"` + secret + `"}],
		"agents":[{"id":"chat-fixture","name":"Offline fixture"},{"id":"coder","name":"Coder","worker":"opencode"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeHarness()
	f := &chatFixture{vaults: vaults, runner: &fakeRunner{calls: make(chan *runCall, 4)}}
	fast := func(r *ChatRuns) { r.retry, r.settle = 10*time.Millisecond, 100*time.Millisecond }
	f.runs = NewChatRuns(config, f.runner, slog.New(slog.NewTextHandler(io.Discard, nil)), WithHarness("opencode", fake), fast)
	t.Cleanup(func() { f.runs.Close(2 * time.Second) })
	f.noRole = NewDeploymentHandler("test", vaults, nil, nil, WithChatRuns(f.runs))
	f.handler = as(auth.RoleUser, f.noRole)
	fake.waitSubscribed(t)
	return f, fake
}

func (f *chatFixture) createAgentChat(t *testing.T, project, agent string) projects.Chat {
	t.Helper()
	rr := f.do(t, "POST", "/v1/projects/"+project+"/chats", `{"agent":"`+agent+`"}`)
	expect(t, rr, http.StatusCreated, "")
	return decode[projects.Chat](t, rr)
}

func chatPath(project, chatID string) string { return "/v1/projects/" + project + "/chats/" + chatID }

func TestSessionRunCopiesTranscript(t *testing.T) {
	f, fake := newSessionFixture(t)
	listed := decode[struct{ Agents []agentInfo }](t, f.do(t, "GET", "/v1/agents", ""))
	if len(listed.Agents) != 2 || listed.Agents[0].Mode != "turn" || listed.Agents[1].Mode != "session" {
		t.Fatalf("agents = %+v", listed.Agents)
	}
	chat := f.createAgentChat(t, "desk", "coder")
	rr := f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"Plan a bracket"}`)
	expect(t, rr, http.StatusAccepted, "")
	accepted := decode[chatView](t, rr)
	if accepted.Mode != "session" || !accepted.Running || len(accepted.Messages) != 2 || accepted.Messages[1].Status != projects.StatusPending {
		t.Fatalf("accepted = %+v", accepted)
	}
	sid := harness.SessionID(chat.ID)
	if got := fake.promptList(); len(got) != 1 || got[0] != sid+": Plan a bracket" || !fake.opened[sid] {
		t.Fatalf("prompts = %v", got)
	}
	shown := decode[chatView](t, f.do(t, "GET", chatPath("desk", chat.ID), ""))
	if !shown.Running || shown.Mode != "session" || shown.Messages[1].Status != projects.StatusPending {
		t.Fatalf("while running = %+v", shown)
	}
	fake.record(sid, harness.Message{Role: harness.RoleTool, Text: "read: completed"})
	fake.end(chat.ID, "Use PETG.", "Print it flat.")
	done := f.settled(t, chat.ID)
	if m := done.Messages[1]; m.Status != projects.StatusCompleted || m.Content != "Use PETG.\n\nPrint it flat." {
		t.Fatalf("answer = %+v", m)
	}
	if view := decode[chatView](t, f.do(t, "GET", chatPath("desk", chat.ID), "")); view.Running {
		t.Fatal("still running after the end")
	}

	// The next message starts a new run whose answer is only its own.
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"And outdoors?"}`), http.StatusAccepted, "")
	fake.end(chat.ID, "ASA.")
	if done := f.settled(t, chat.ID); len(done.Messages) != 4 || done.Messages[3].Content != "ASA." {
		t.Fatalf("second run = %+v", done.Messages)
	}

	// A failed run shows a fixed message.
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"again"}`), http.StatusAccepted, "")
	fake.record(sid, harness.Message{Role: harness.RoleRun, Text: "failed"})
	fake.emit(harness.Event{Session: sid, Type: harness.RunFailed})
	if m := f.settled(t, chat.ID).Messages[5]; m.Status != projects.StatusFailed || m.Error != sessionFailed || m.Content != "" {
		t.Fatalf("failed run = %+v", m)
	}
}

func TestSessionSteer(t *testing.T) {
	f, fake := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "coder")
	path := chatPath("desk", chat.ID) + "/messages"
	expect(t, f.do(t, "POST", path, `{"content":"Review the code"}`), http.StatusAccepted, "")
	rr := f.do(t, "POST", path, `{"content":"Focus on errors"}`)
	expect(t, rr, http.StatusAccepted, "")
	steered := decode[chatView](t, rr)
	if len(steered.Messages) != 3 || steered.Messages[1].Status != projects.StatusPending || steered.Messages[2].Content != "Focus on errors" || !steered.Running {
		t.Fatalf("steered = %+v", steered)
	}
	expect(t, f.do(t, "POST", path, `{"content":"`+strings.Repeat("x", maxChatMessage+1)+`"}`), http.StatusRequestEntityTooLarge, "message_too_large")
	if got := fake.promptList(); len(got) != 2 || !strings.HasSuffix(got[1], ": Focus on errors") {
		t.Fatalf("prompts = %v", got)
	}
	sid := harness.SessionID(chat.ID)

	// The run ended, but the steer started another one before the event
	// was handled: wait for that run's end.
	fake.mu.Lock()
	fake.history[sid] = []harness.Message{
		{Role: harness.RoleUser, Text: "Review the code"}, {Role: harness.RoleAgent, Text: "Looks fine."},
		{Role: harness.RoleRun, Text: "succeeded"}, {Role: harness.RoleUser, Text: "Focus on errors"},
	}
	fake.mu.Unlock()
	fake.emit(harness.Event{Session: sid, Type: harness.RunSucceeded})
	time.Sleep(50 * time.Millisecond)
	if !f.runs.running(chatKey("desk", chat.ID)) {
		t.Fatal("finished on the end of a run that a steer continued")
	}
	fake.end(chat.ID, "One unchecked error.")
	done := f.settled(t, chat.ID)
	if m := done.Messages[1]; m.Content != "Looks fine.\n\nOne unchecked error." || m.Status != projects.StatusCompleted {
		t.Fatalf("answer = %+v", m)
	}
}

func TestSessionInterruptAndDeadline(t *testing.T) {
	f, fake := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "coder")
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"long task"}`), http.StatusAccepted, "")
	rr := f.do(t, "POST", chatPath("desk", chat.ID)+"/interrupt", "")
	expect(t, rr, http.StatusOK, "")
	if body := strings.TrimSpace(rr.Body.String()); body != `{"interrupted":true}` {
		t.Fatalf("interrupt = %s", body)
	}
	if m := f.settled(t, chat.ID).Messages[1]; m.Status != projects.StatusFailed || m.Error != agents.UserMessage(agents.CodeInterrupted) {
		t.Fatalf("interrupted = %+v", m)
	}
	rr = f.do(t, "POST", chatPath("desk", chat.ID)+"/interrupt", "")
	if body := strings.TrimSpace(rr.Body.String()); body != `{"interrupted":false}` {
		t.Fatalf("idle interrupt = %s", body)
	}

	// After the deadline the API interrupts the session.
	f.runs.mu.Lock()
	f.runs.sessionDeadline = 20 * time.Millisecond
	f.runs.mu.Unlock()
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"slow"}`), http.StatusAccepted, "")
	if m := f.settled(t, chat.ID).Messages[3]; m.Error != agents.UserMessage(agents.CodeTimeout) {
		t.Fatalf("expired = %+v", m)
	}
	// Even when the worker never reports the end.
	fake.mu.Lock()
	fake.emitOnStop = false
	fake.mu.Unlock()
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"stuck"}`), http.StatusAccepted, "")
	if m := f.settled(t, chat.ID).Messages[5]; m.Error != agents.UserMessage(agents.CodeTimeout) {
		t.Fatalf("expired without an event = %+v", m)
	}
}

func TestSessionPermissions(t *testing.T) {
	f, fake := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "coder")
	sid := harness.SessionID(chat.ID)
	fake.mu.Lock()
	fake.perms[sid] = []harness.Permission{{ID: "per_1", Action: "bash", Resources: []string{"go test ./..."}}}
	fake.mu.Unlock()
	rr := f.do(t, "GET", chatPath("desk", chat.ID)+"/permissions", "")
	expect(t, rr, http.StatusOK, "")
	if body := strings.TrimSpace(rr.Body.String()); body != `{"permissions":[{"id":"per_1","action":"bash","resources":["go test ./..."]}]}` {
		t.Fatalf("permissions = %s", body)
	}
	base := chatPath("desk", chat.ID) + "/permissions/"
	expect(t, f.do(t, "POST", base+"per_1", `{"decision":"always"}`), http.StatusBadRequest, "invalid_decision")
	expect(t, f.do(t, "POST", base+"per_1", `{"decision":"once","scope":"all"}`), http.StatusBadRequest, "invalid_decision")
	expect(t, f.do(t, "POST", base+"per_2", `{"decision":"once"}`), http.StatusNotFound, "permission_not_found")
	expect(t, f.do(t, "POST", base+"per.1", `{"decision":"once"}`), http.StatusNotFound, "permission_not_found")
	expect(t, f.do(t, "POST", base+"per_1", `{"decision":"once"}`), http.StatusNoContent, "")
	if len(fake.replies) != 1 || fake.replies[0] != "per_1 once" {
		t.Fatalf("replies = %v", fake.replies)
	}

	turn := f.createAgentChat(t, "desk", "chat-fixture")
	rr = f.do(t, "GET", chatPath("desk", turn.ID)+"/permissions", "")
	if strings.TrimSpace(rr.Body.String()) != `{"permissions":[]}` {
		t.Fatalf("turn permissions = %s", rr.Body)
	}
	expect(t, f.do(t, "POST", chatPath("desk", turn.ID)+"/permissions/per_1", `{"decision":"once"}`), http.StatusNotFound, "permission_not_found")
	expect(t, f.do(t, "GET", chatPath("desk", "0123456789abcdef")+"/permissions", ""), http.StatusNotFound, "chat_not_found")
}

// sseReader reads data lines from a chat's event stream.
type sseReader struct {
	lines chan string
	body  io.Closer
}

func openEvents(t *testing.T, server *httptest.Server, project, chatID string) *sseReader {
	t.Helper()
	res, err := server.Client().Get(server.URL + chatPath(project, chatID) + "/events")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/event-stream" || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("events answered %d %v", res.StatusCode, res.Header)
	}
	r := &sseReader{lines: make(chan string, 64), body: res.Body}
	t.Cleanup(func() { res.Body.Close() })
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				r.lines <- data
			}
		}
	}()
	return r
}

func (r *sseReader) next(t *testing.T) string {
	t.Helper()
	select {
	case line, ok := <-r.lines:
		if !ok {
			t.Fatal("event stream ended")
		}
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return ""
	}
}

func TestSessionEventsPerChat(t *testing.T) {
	f, fake := newSessionFixture(t)
	server := httptest.NewServer(f.handler)
	t.Cleanup(server.Close) // after the streams close; Close waits for them
	mine, other := f.createAgentChat(t, "desk", "coder"), f.createAgentChat(t, "desk", "coder")
	stream := openEvents(t, server, "desk", mine.ID)
	if got := stream.next(t); got != `{"type":"connected"}` {
		t.Fatalf("first event = %s", got)
	}
	fake.emit(harness.Event{Session: harness.SessionID(other.ID), Type: harness.TextDelta, Delta: "not yours"})
	fake.emit(harness.Event{Session: "ses_review", Type: harness.TextDelta, Delta: "not Ember's"})
	fake.emit(harness.Event{Session: harness.SessionID(mine.ID), Type: harness.TextDelta, Delta: "progress"})
	fake.emit(harness.Event{Session: harness.SessionID(mine.ID), Type: harness.PermissionAsked})
	if got := stream.next(t); got != `{"type":"text.delta","delta":"progress"}` {
		t.Fatalf("event = %s", got)
	}
	if got := stream.next(t); got != `{"type":"permission.asked"}` {
		t.Fatalf("event = %s", got)
	}
	// A run end reaches the browser after the transcript is written.
	expect(t, f.do(t, "POST", chatPath("desk", mine.ID)+"/messages", `{"content":"go"}`), http.StatusAccepted, "")
	fake.end(mine.ID, "done")
	if got := stream.next(t); got != `{"type":"run.succeeded"}` {
		t.Fatalf("event = %s", got)
	}
	if stored, _ := f.vaults.Chat("desk", mine.ID); stored.Messages[1].Content != "done" {
		t.Fatalf("event before transcript: %+v", stored.Messages)
	}
	// The upstream stream fails: the browser keeps its connection.
	fake.breakStream()
	if got := stream.next(t); got != `{"type":"resync"}` {
		t.Fatalf("event = %s", got)
	}
	fake.waitSubscribed(t)
	if got := stream.next(t); got != `{"type":"resync"}` {
		t.Fatalf("event after resubscribing = %s", got)
	}
	fake.emit(harness.Event{Session: harness.SessionID(mine.ID), Type: harness.ToolCalled})
	if got := stream.next(t); got != `{"type":"tool.called"}` {
		t.Fatalf("event = %s", got)
	}

	// A turn agent's chat streams run events only.
	turn := f.createAgentChat(t, "desk", "chat-fixture")
	turnStream := openEvents(t, server, "desk", turn.ID)
	turnStream.next(t)
	expect(t, f.do(t, "POST", chatPath("desk", turn.ID)+"/messages", `{"content":"hi"}`), http.StatusAccepted, "")
	if got := turnStream.next(t); got != `{"type":"run.started"}` {
		t.Fatalf("turn event = %s", got)
	}
	f.runner.next(t).reply <- runReply{result: agents.Result{Output: "ok"}}
	if got := turnStream.next(t); got != `{"type":"run.succeeded"}` {
		t.Fatalf("turn event = %s", got)
	}

	expect(t, f.do(t, "GET", chatPath("shed", mine.ID)+"/events", ""), http.StatusNotFound, "chat_not_found")

	// A graceful shutdown ends the streams instead of waiting for them.
	f.runs.EndStreams()
	for range stream.lines {
	}
	expect(t, f.do(t, "GET", chatPath("desk", mine.ID)+"/events", ""), http.StatusServiceUnavailable, "agents_unavailable")
}

func TestSessionWorkerLimit(t *testing.T) {
	f, fake := newSessionFixture(t)
	var chats []projects.Chat
	for range maxWorkerRuns + 1 {
		chats = append(chats, f.createAgentChat(t, "desk", "coder"))
	}
	for _, c := range chats[:maxWorkerRuns] {
		expect(t, f.do(t, "POST", chatPath("desk", c.ID)+"/messages", `{"content":"work"}`), http.StatusAccepted, "")
	}
	last := chats[maxWorkerRuns]
	expect(t, f.do(t, "POST", chatPath("desk", last.ID)+"/messages", `{"content":"more"}`), http.StatusTooManyRequests, "agent_busy")
	if stored, _ := f.vaults.Chat("desk", last.ID); len(stored.Messages) != 0 {
		t.Fatal("a refused message was saved")
	}
	// Steering an active run is not limited.
	expect(t, f.do(t, "POST", chatPath("desk", chats[0].ID)+"/messages", `{"content":"steer"}`), http.StatusAccepted, "")
	fake.end(chats[0].ID, "ok")
	f.settled(t, chats[0].ID)
	expect(t, f.do(t, "POST", chatPath("desk", last.ID)+"/messages", `{"content":"more"}`), http.StatusAccepted, "")
}

func TestSessionWorkerUnavailable(t *testing.T) {
	f, fake := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "coder")
	fake.mu.Lock()
	fake.failPrompt = true
	fake.mu.Unlock()
	rr := f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"hello"}`)
	expect(t, rr, http.StatusBadGateway, "worker_unavailable")
	if strings.Contains(rr.Body.String(), "sk-secret") {
		t.Fatal("worker error reached the user")
	}
	if stored, _ := f.vaults.Chat("desk", chat.ID); len(stored.Messages) != 0 || stored.Title != "" {
		t.Fatalf("a message that never reached the agent was kept: %+v", stored)
	}
	if f.runs.running(chatKey("desk", chat.ID)) {
		t.Fatal("the failed start still holds the chat")
	}
}

func TestSessionRecoveryAfterRestart(t *testing.T) {
	f, fake := newSessionFixture(t)
	pending := func(prompt string) projects.Chat {
		chat := f.createAgentChat(t, "desk", "coder")
		// As if the API had stopped while the worker was answering.
		if _, err := f.vaults.UpdateChat("desk", chat.ID, 0, func(c *projects.Chat) error {
			c.Messages = append(c.Messages,
				projects.ChatMessage{ID: projects.NewID(), Role: "user", Content: prompt},
				projects.ChatMessage{ID: projects.NewID(), Role: "agent", Status: projects.StatusPending})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return chat
	}

	idle := pending("finished while away")
	fake.record(harness.SessionID(idle.ID),
		harness.Message{Role: harness.RoleUser, Text: "finished while away"},
		harness.Message{Role: harness.RoleAgent, Text: "Recovered answer."},
		harness.Message{Role: harness.RoleRun, Text: "succeeded"})
	view := decode[chatView](t, f.do(t, "GET", chatPath("desk", idle.ID), ""))
	if view.Running || view.Messages[1].Status != projects.StatusCompleted || view.Messages[1].Content != "Recovered answer." {
		t.Fatalf("recovered = %+v", view)
	}
	if stored, _ := f.vaults.Chat("desk", idle.ID); stored.Messages[1].Content != "Recovered answer." {
		t.Fatal("the recovered answer was not stored")
	}

	busy := pending("still working")
	fake.record(harness.SessionID(busy.ID), harness.Message{Role: harness.RoleUser, Text: "still working"})
	view = decode[chatView](t, f.do(t, "GET", chatPath("desk", busy.ID), ""))
	if !view.Running || view.Messages[1].Status != projects.StatusPending {
		t.Fatalf("tracked again = %+v", view)
	}
	fake.end(busy.ID, "Late answer.")
	if m := f.settled(t, busy.ID).Messages[1]; m.Content != "Late answer." {
		t.Fatalf("after tracking again = %+v", m)
	}

	// A session the worker does not know: shown as interrupted, as before.
	lost := pending("lost")
	view = decode[chatView](t, f.do(t, "GET", chatPath("desk", lost.ID), ""))
	if view.Running || view.Messages[1].Error != agents.UserMessage(agents.CodeInterrupted) {
		t.Fatalf("lost = %+v", view)
	}

	// An idle session whose history lacks the prompt holds an earlier
	// run's answer, which must not be copied.
	unsent := pending("never sent")
	fake.record(harness.SessionID(unsent.ID),
		harness.Message{Role: harness.RoleUser, Text: "earlier"},
		harness.Message{Role: harness.RoleAgent, Text: "Earlier answer."},
		harness.Message{Role: harness.RoleRun, Text: "succeeded"})
	f.do(t, "GET", chatPath("desk", unsent.ID), "")
	if stored, _ := f.vaults.Chat("desk", unsent.ID); stored.Messages[1].Status != projects.StatusFailed || stored.Messages[1].Content != "" {
		t.Fatalf("unsent = %+v", stored.Messages[1])
	}
}

func TestSessionRunsEndedWhileUnsubscribed(t *testing.T) {
	f, fake := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "coder")
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"work"}`), http.StatusAccepted, "")
	fake.mu.Lock()
	fake.subscribeOK = false
	fake.mu.Unlock()
	fake.breakStream()
	// The run ends while the API cannot hear it.
	sid := harness.SessionID(chat.ID)
	fake.record(sid, harness.Message{Role: harness.RoleAgent, Text: "Done meanwhile."}, harness.Message{Role: harness.RoleRun, Text: "succeeded"})
	time.Sleep(30 * time.Millisecond)
	fake.mu.Lock()
	fake.subscribeOK = true
	fake.mu.Unlock()
	fake.waitSubscribed(t)
	if m := f.settled(t, chat.ID).Messages[1]; m.Content != "Done meanwhile." {
		t.Fatalf("reconciled = %+v", m)
	}
}

func TestTurnAgentsBesideSessions(t *testing.T) {
	f, _ := newSessionFixture(t)
	chat := f.createAgentChat(t, "desk", "chat-fixture")
	rr := f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"hello"}`)
	expect(t, rr, http.StatusAccepted, "")
	if v := decode[chatView](t, rr); v.Mode != "turn" || !v.Running {
		t.Fatalf("accepted = %+v", v)
	}
	call := f.runner.next(t)
	rr = f.do(t, "POST", chatPath("desk", chat.ID)+"/interrupt", "")
	if strings.TrimSpace(rr.Body.String()) != `{"interrupted":true}` {
		t.Fatalf("interrupt = %s", rr.Body)
	}
	if call.ctx.Err() == nil {
		t.Fatal("interrupt did not cancel the turn run")
	}
	if m := f.settled(t, chat.ID).Messages[1]; m.Status != projects.StatusFailed || m.Error != agents.UserMessage(agents.CodeInterrupted) {
		t.Fatalf("interrupted turn = %+v", m)
	}
	if v := decode[chatView](t, f.do(t, "GET", chatPath("desk", chat.ID), "")); v.Running || v.Mode != "turn" {
		t.Fatalf("after = %+v", v)
	}
	rr = f.do(t, "POST", chatPath("desk", chat.ID)+"/interrupt", "")
	if strings.TrimSpace(rr.Body.String()) != `{"interrupted":false}` {
		t.Fatalf("idle interrupt = %s", rr.Body)
	}
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"again"}`), http.StatusAccepted, "")
	f.runner.next(t).reply <- runReply{result: agents.Result{Output: "ok"}}
	if m := f.settled(t, chat.ID).Messages[3]; m.Content != "ok" {
		t.Fatalf("second turn = %+v", m)
	}
}

// Without a router socket, turn runs fail with the router's fixed message.
func TestTurnRunWithoutRouter(t *testing.T) {
	f := newChatFixture(t, true)
	f.runs.runner = nil
	chat := f.createChat(t)
	expect(t, f.do(t, "POST", chatPath("desk", chat.ID)+"/messages", `{"content":"hello"}`), http.StatusAccepted, "")
	if m := f.settled(t, chat.ID).Messages[1]; m.Error != agents.UserMessage(agents.CodeRouterUnavailable) {
		t.Fatalf("without router = %+v", m)
	}
}

func TestReadAnswer(t *testing.T) {
	u := func(s string) harness.Message { return harness.Message{Role: harness.RoleUser, Text: s} }
	a := func(s string) harness.Message { return harness.Message{Role: harness.RoleAgent, Text: s} }
	run := func(s string) harness.Message { return harness.Message{Role: harness.RoleRun, Text: s} }
	for name, tc := range map[string]struct {
		msgs []harness.Message
		want answer
	}{
		"running":           {[]harness.Message{u("q"), a("partial")}, answer{text: "partial", found: true}},
		"done":              {[]harness.Message{u("old"), a("x"), run("succeeded"), u("q"), a("y"), run("succeeded")}, answer{text: "y", found: true, idle: true, outcome: outcomeSucceeded}},
		"steered in run":    {[]harness.Message{u("q"), a("a"), u("s"), a("b"), run("succeeded")}, answer{text: "a\n\nb", found: true, idle: true, outcome: outcomeSucceeded}},
		"steered after":     {[]harness.Message{u("q"), a("a"), run("succeeded"), u("s")}, answer{text: "a", found: true, restarted: true}},
		"same prompt":       {[]harness.Message{u("q"), a("x"), run("succeeded"), u("q"), a("y"), run("interrupted")}, answer{text: "y", found: true, idle: true, outcome: outcomeInterrupted}},
		"scrolled out":      {[]harness.Message{a("x"), run("succeeded"), a("y"), run("error")}, answer{text: "y", idle: true, outcome: outcomeFailed}},
		"no markers at all": {[]harness.Message{u("q"), a("y")}, answer{text: "y", found: true}},
	} {
		if got := readAnswer(tc.msgs, "q"); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
	if got := clip("ab"+strings.Repeat("é", 10), 5); got != "abé" {
		t.Errorf("clip = %q", got)
	}
}
