package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/harness"
)

// Opt-in protocol check: a fresh foreground OpenCode, no inherited accounts,
// and a scripted text-only model. This checks wire compatibility, not isolation.
func TestOpenCodeProtocol(t *testing.T) {
	binary := os.Getenv("OPENCODE_TEST_BINARY")
	if binary == "" {
		t.Skip("set OPENCODE_TEST_BINARY to the OpenCode 2.3.2 executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Model    string
			Stream   bool
			Messages []struct{ Content any }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Model != "fixture" {
			http.Error(w, "wrong model", 400)
			return
		}
		const answer = "Fixture review complete."
		if !request.Stream {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": answer}, "finish_reason": "stop"}}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":null}]}\n\n", answer)
		w.(http.Flusher).Flush()
		if strings.Contains(fmt.Sprint(request.Messages), "wait-for-stop") {
			select {
			case <-r.Context().Done():
			case <-ctx.Done():
			}
			return
		}
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	home := t.TempDir()
	config, err := os.ReadFile("../../../../../services/ai-worker/opencode/worker.json")
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte(strings.Replace(string(config), "http://model-gateway:8080", model.URL, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	port, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	port.Close()
	cmd := exec.CommandContext(ctx, binary, "serve", "--hostname", "127.0.0.1", "--port", strings.Split(address, ":")[1])
	cmd.Dir = home
	const password = "private-protocol-fixture"
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "OPENCODE_PASSWORD=" + password, "REVIEW_MODEL=fixture", "REVIEW_WORKLOAD_ID=fixture"}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	c := New("http://"+address, password)
	for {
		if err := c.call(ctx, "GET", "/api/info", nil, nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("worker did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
	const sessionID = "ses_ember_0123456789abcdef"
	// No tools and no host workspace. Open must resume this session.
	create := map[string]any{"id": sessionID, "location": map[string]string{"directory": home}, "permissions": []any{map[string]string{"action": "*", "resource": "*", "effect": "deny"}}}
	if err := c.call(ctx, "POST", "/api/session", create, nil); err != nil {
		t.Fatal(err)
	}
	s := harness.Session{ID: sessionID, Directory: home}
	if err := c.Open(ctx, s); err != nil {
		t.Fatal(err)
	}
	events, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wait := func(want string) {
		t.Helper()
		for e := range events {
			if e.Session == sessionID && e.Type == want {
				return
			}
		}
		t.Fatalf("stream ended waiting for %q", want)
	}
	if err := c.Prompt(ctx, sessionID, "Review the fixture"); err != nil {
		t.Fatal(err)
	}
	wait(harness.TextDelta)
	wait(harness.RunSucceeded)
	if err := c.Open(ctx, s); err != nil {
		t.Fatal(err)
	}
	history, err := c.Messages(ctx, sessionID, 50)
	if err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprint(history)
	promptAt, answerAt := strings.Index(text, "Review the fixture"), strings.Index(text, "Fixture review complete.")
	if promptAt < 0 || answerAt <= promptAt || history[len(history)-1].Role != harness.RoleRun {
		t.Fatalf("unexpected history: %v", history)
	}
	if err := c.Prompt(ctx, sessionID, "wait-for-stop"); err != nil {
		t.Fatal(err)
	}
	wait(harness.TextDelta)
	if err := c.Prompt(ctx, sessionID, "Focus on errors"); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.Interrupt(ctx, sessionID); err != nil || !ok {
		t.Fatalf("interrupt = %v, %v", ok, err)
	}
	wait(harness.RunInterrupted)
}
