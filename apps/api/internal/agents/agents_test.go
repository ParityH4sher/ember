package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validConfig = `{"routerSocket":"/run/ember-router/router.sock","agents":[{"id":"chat-fixture","name":"Offline fixture"}]}`

func TestParseConfig(t *testing.T) {
	c, err := ParseConfig([]byte(validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.AllowWithoutSignIn {
		t.Fatal("allowWithoutSignIn must default to false")
	}
	if a, ok := c.Agent("chat-fixture"); !ok || a.Name != "Offline fixture" {
		t.Fatalf("Agent lookup = %+v, %v", a, ok)
	}
	if _, ok := c.Agent("other"); ok {
		t.Fatal("found an agent that is not configured")
	}
	// An operator may list no agents and leave them to admins (aicatalog).
	if c, err := ParseConfig([]byte(`{"routerSocket":"/r.sock","agents":[]}`)); err != nil || len(c.Agents) != 0 {
		t.Fatalf("no agents = %v, %v; want accepted", c, err)
	}
	many := make([]string, 17)
	for i := range many {
		many[i] = fmt.Sprintf(`{"id":"a%d","name":"A"}`, i)
	}
	for name, doc := range map[string]string{
		"unknown field":   strings.Replace(validConfig, `"agents"`, `"profile":"trusted","agents"`, 1),
		"trailing value":  validConfig + `{}`,
		"relative socket": strings.Replace(validConfig, "/run/ember-router/router.sock", "router.sock", 1),
		"unclean socket":  strings.Replace(validConfig, "/run/ember-router/", "/run/../run/ember-router/", 1),
		"too many agents": `{"routerSocket":"/r.sock","agents":[` + strings.Join(many, ",") + `]}`,
		"bad id":          strings.Replace(validConfig, "chat-fixture", "Chat_Fixture", 1),
		"duplicate id":    `{"routerSocket":"/r.sock","agents":[{"id":"a","name":"A"},{"id":"a","name":"B"}]}`,
		"empty name":      strings.Replace(validConfig, "Offline fixture", "", 1),
		"padded name":     strings.Replace(validConfig, "Offline fixture", " Offline", 1),
		"control in name": strings.Replace(validConfig, "Offline fixture", `Off\nline`, 1),
		"long name":       strings.Replace(validConfig, "Offline fixture", strings.Repeat("x", 81), 1),
	} {
		if _, err := ParseConfig([]byte(doc)); err == nil {
			t.Errorf("%s: accepted %s", name, doc)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	if c, err := LoadConfig(""); c != nil || err != nil {
		t.Fatalf("empty path = %v, %v; want chat runs off", c, err)
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "agents.json")
	if err := os.WriteFile(good, []byte(validConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadConfig(good); err != nil || len(c.Agents) != 1 {
		t.Fatalf("LoadConfig = %v, %v", c, err)
	}
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, []byte(validConfig+strings.Repeat(" ", maxConfigBytes)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(large); err == nil {
		t.Fatal("accepted a file over 64 KiB")
	}
	if _, err := LoadConfig(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("accepted a missing file")
	}
}

func writeSecret(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWorkersConfig(t *testing.T) {
	secret := strings.Repeat("s", 32)
	password := writeSecret(t, secret+"\n")
	doc := func(workers, agents string) string {
		return `{"workers":[` + workers + `],"agents":[` + agents + `]}`
	}
	worker := func(id, url, file string) string {
		return fmt.Sprintf(`{"id":%q,"harness":"opencode","url":%q,"passwordFile":%q}`, id, url, file)
	}
	good := worker("opencode", "http://opencode-worker:4096", password)
	// Session agents alone need no router socket.
	c, err := ParseConfig([]byte(doc(good, `{"id":"coder","name":"Coder","worker":"opencode"}`)))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := c.Worker("opencode")
	if !ok || w.Password() != secret || w.Directory != "/workspace" || w.URL != "http://opencode-worker:4096" {
		t.Fatalf("worker = %+v, %v", w, ok)
	}
	if ids := c.WorkerIDs(); len(ids) != 1 || ids[0] != "opencode" {
		t.Fatalf("WorkerIDs = %v", ids)
	}
	if a, _ := c.Agent("coder"); a.Worker != "opencode" {
		t.Fatalf("agent = %+v", a)
	}
	var none *Config
	if ids := none.WorkerIDs(); len(ids) != 0 {
		t.Fatal("nil config has workers")
	}
	if _, ok := none.Worker("opencode"); ok {
		t.Fatal("nil config found a worker")
	}
	withDir := strings.Replace(good, `"harness"`, `"directory":"/srv/work","harness"`, 1)
	if c, err := ParseConfig([]byte(doc(withDir, ""))); err != nil || c.Workers[0].Directory != "/srv/work" {
		t.Fatalf("directory = %v, %v", c, err)
	}
	// The password never leaves through JSON.
	if b, _ := json.Marshal(c); strings.Contains(string(b), secret) {
		t.Fatal("password marshalled")
	}

	five := []string{}
	for i := range 5 {
		five = append(five, worker(fmt.Sprintf("w%d", i), "http://w:1", password))
	}
	for name, d := range map[string]string{
		"router agent without socket": doc(good, `{"id":"fixture","name":"F"}`),
		"unknown worker":              doc(good, `{"id":"coder","name":"Coder","worker":"other"}`),
		"too many workers":            doc(strings.Join(five, ","), ""),
		"duplicate worker":            doc(good+","+good, ""),
		"bad worker id":               doc(worker("Open_Code", "http://w:1", password), ""),
		"other harness":               doc(strings.Replace(good, `"opencode","url"`, `"codex","url"`, 1), ""),
		"https":                       doc(worker("w", "https://w:1", password), ""),
		"path":                        doc(worker("w", "http://w:1/api", password), ""),
		"trailing slash":              doc(worker("w", "http://w:1/", password), ""),
		"query":                       doc(worker("w", "http://w:1?x=1", password), ""),
		"user info":                   doc(worker("w", "http://u:p@w:1", password), ""),
		"no host":                     doc(worker("w", "http://:1", password), ""),
		"relative directory":          doc(strings.Replace(good, `"harness"`, `"directory":"work","harness"`, 1), ""),
		"missing password file":       doc(worker("w", "http://w:1", "/nonexistent/secret"), ""),
		"no password file":            doc(worker("w", "http://w:1", ""), ""),
		"short password":              doc(worker("w", "http://w:1", writeSecret(t, strings.Repeat("s", 31)+"\n")), ""),
		"two-line password":           doc(worker("w", "http://w:1", writeSecret(t, secret+"\n"+secret)), ""),
		"unknown worker field":        doc(strings.Replace(good, `"harness"`, `"token":"x","harness"`, 1), ""),
	} {
		if _, err := ParseConfig([]byte(d)); err == nil {
			t.Errorf("%s: accepted %s", name, d)
		}
	}
}

func turnWith(contents ...string) Turn {
	t := Turn{Project: ProjectRef{ID: "desk", Name: "Desk"}, Chat: ChatRef{ID: "0123456789abcdef", Title: "Hi"}}
	for i, c := range contents {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		t.Messages = append(t.Messages, Message{Role: role, Content: c})
	}
	return t
}

func TestFitKeepsShortConversations(t *testing.T) {
	fitted, err := Fit("chat-fixture", turnWith("one", "two", "three <&>"))
	if err != nil {
		t.Fatal(err)
	}
	if fitted.Schema != TurnSchema || fitted.OmittedMessages != 0 || len(fitted.Messages) != 3 {
		t.Fatalf("fitted = %+v", fitted)
	}
	body, err := encodeRequest("chat-fixture", fitted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `three <&>`) {
		t.Fatalf("HTML characters were escaped: %s", body)
	}
}

func TestFitLeavesOutOldestMessages(t *testing.T) {
	var contents []string
	for i := range 12 {
		contents = append(contents, fmt.Sprintf("%02d %s", i, strings.Repeat(`"quoted" `, 400)))
	}
	fitted, err := Fit("chat-fixture", turnWith(contents...))
	if err != nil {
		t.Fatal(err)
	}
	if fitted.OmittedMessages == 0 || fitted.OmittedMessages+len(fitted.Messages) != 12 {
		t.Fatalf("omitted %d, kept %d", fitted.OmittedMessages, len(fitted.Messages))
	}
	if last := fitted.Messages[len(fitted.Messages)-1].Content; !strings.HasPrefix(last, "11 ") {
		t.Fatalf("newest message was not kept: %.10q", last)
	}
	body, _ := encodeRequest("chat-fixture", fitted)
	if len(body) > MaxRequest {
		t.Fatalf("request is %d bytes", len(body))
	}
	// Minimal: keeping one more message would not fit.
	more := fitted
	more.Messages = turnWith(contents...).Messages[fitted.OmittedMessages-1:]
	more.OmittedMessages--
	if body, _ := encodeRequest("chat-fixture", more); len(body) <= MaxRequest {
		t.Fatalf("left out more messages than needed (%d bytes would fit)", len(body))
	}
	// The router's own decoder sees a body within its limit and the original text.
	var req struct{ Agent, Input string }
	if err := json.Unmarshal(body, &req); err != nil || req.Agent != "chat-fixture" {
		t.Fatal(err, req.Agent)
	}
	var decoded Turn
	if err := json.Unmarshal([]byte(req.Input), &decoded); err != nil || decoded.Messages[len(decoded.Messages)-1].Content != contents[11] {
		t.Fatal("envelope did not round-trip", err)
	}
}

func TestFitRejectsOversizedNewestMessage(t *testing.T) {
	if _, err := Fit("chat-fixture", turnWith(strings.Repeat(`"`, 12<<10))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if _, err := Fit("chat-fixture", Turn{}); err == nil {
		t.Fatal("accepted a turn without messages")
	}
}

// fakeRouter serves handler on a Unix socket, as the sandbox router does.
func fakeRouter(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ember-agents-") // short: socket paths are limited to ~108 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "router.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return socket
}

const testRunID = "ember-run-0123456789abcdef0123456789abcdef"

func TestRouterRun(t *testing.T) {
	socket := fakeRouter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req struct{ Agent, Input string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		var turn Turn
		if err := json.Unmarshal([]byte(req.Input), &turn); err != nil || req.Agent != "chat-fixture" || turn.Messages[0].Content != "hello" {
			t.Errorf("router received agent %q, input %q", req.Agent, req.Input)
		}
		w.Header().Set("X-Ember-Run-ID", testRunID)
		json.NewEncoder(w).Encode(map[string]string{"run_id": testRunID, "profile": "trusted", "output": "\n  Hello\x00 \xff back \n"})
	})
	turn, _ := Fit("chat-fixture", turnWith("hello"))
	result, err := NewRouter(socket).Run(context.Background(), "chat-fixture", turn)
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID != testRunID || result.Output != "Hello � back" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRouterFailures(t *testing.T) {
	turn, _ := Fit("chat-fixture", turnWith("hello"))
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		code    string
	}{
		{"unknown agent", status(http.StatusNotFound), CodeUnknownAgent},
		{"busy", status(http.StatusTooManyRequests), CodeRunnerBusy},
		{"router deadline", status(http.StatusGatewayTimeout), CodeTimeout},
		{"launch failed", status(http.StatusBadGateway), CodeFailed},
		{"bad request", status(http.StatusBadRequest), CodeFailed},
		{"empty output", reply(`{"run_id":"` + testRunID + `","output":" \n "}`), CodeEmptyOutput},
		{"not JSON", reply(`run finished`), CodeFailed},
		{"oversized reply", reply(`{"output":"` + strings.Repeat("x", maxResponse) + `"}`), CodeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRouter(fakeRouter(t, tc.handler)).Run(context.Background(), "chat-fixture", turn)
			var runErr *RunError
			if !errors.As(err, &runErr) || runErr.Code != tc.code {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			if strings.Contains(UserMessage(runErr.Code), "/") || UserMessage(runErr.Code) == "" {
				t.Fatalf("user message %q", UserMessage(runErr.Code))
			}
		})
	}
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ember-Run-ID", testRunID)
		http.Error(w, "trusted launch or agent failed: exit status 125 /var/lib/containers", code)
	}
}

func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) }
}

func TestRouterUnavailableAndCancelled(t *testing.T) {
	turn, _ := Fit("chat-fixture", turnWith("hello"))
	_, err := NewRouter(filepath.Join(t.TempDir(), "missing.sock")).Run(context.Background(), "chat-fixture", turn)
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.Code != CodeRouterUnavailable {
		t.Fatalf("missing socket: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	// Like the real router, read the body first: only then does net/http
	// notice the client going away and cancel the request context.
	socket := fakeRouter(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err = NewRouter(socket).Run(ctx, "chat-fixture", turn)
	if !errors.As(err, &runErr) || runErr.Code != CodeInterrupted {
		t.Fatalf("cancelled: %v", err)
	}

	socket = fakeRouter(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = NewRouter(socket).Run(ctx, "chat-fixture", turn)
	if !errors.As(err, &runErr) || runErr.Code != CodeTimeout {
		t.Fatalf("deadline: %v", err)
	}
}
