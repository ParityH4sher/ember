package main

import (
	"bufio"
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
			output(w, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": answer}, "finish_reason": "stop"}}})
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
	config, err := os.ReadFile("../../../../services/ai-worker/opencode/worker.json")
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
	p := &panel{upstream: "http://" + address, operator: "test-operator", password: password, client: &http.Client{}}
	for {
		if _, err := p.call(ctx, "GET", "/api/info", nil, nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("worker did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
	// No tools and no host workspace. Connect must resume this session.
	body, _ := json.Marshal(map[string]any{"id": sessionID, "location": map[string]string{"directory": home}, "permissions": []any{map[string]string{"action": "*", "resource": "*", "effect": "deny"}}})
	if _, err := p.call(ctx, "POST", "/api/session", strings.NewReader(string(body)), nil); err != nil {
		t.Fatal(err)
	}
	h := p.handler()
	post := func(path, body string, want int) string {
		t.Helper()
		w := visit(h, "POST", path, body, true, true)
		if w.Code != want {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	post("connect", "", 204)
	s := httptest.NewServer(h)
	defer s.Close()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL+prefix+"events", nil)
	req.SetBasicAuth("operator", "test-operator")
	stream, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	sc := bufio.NewScanner(stream.Body)
	wait := func(marker string) {
		t.Helper()
		for sc.Scan() {
			if strings.Contains(sc.Text(), marker) {
				return
			}
		}
		t.Fatalf("stream ended waiting for %q: %v", marker, sc.Err())
	}
	wait("connected")
	post("steer", `{"text":"Review the fixture"}`, 202)
	wait("session.text.delta")
	wait("session.execution.succeeded")
	post("connect", "", 204)
	history := visit(h, "GET", "messages", "", true, false).Body.String()
	promptAt, answerAt := strings.Index(history, "Review the fixture"), strings.Index(history, "Fixture review complete.")
	if promptAt < 0 || answerAt <= promptAt {
		t.Fatalf("unexpected history: %s", history)
	}
	post("steer", `{"text":"wait-for-stop"}`, 202)
	wait("session.text.delta")
	post("steer", `{"text":"Focus on errors"}`, 202)
	if result := post("interrupt", "", 200); !strings.Contains(result, `"interrupted":true`) {
		t.Fatal(result)
	}
}
