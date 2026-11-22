package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicy(t *testing.T) {
	image := "sha256:" + strings.Repeat("a", 64)
	valid := `{"probe":{"profile":"isolated","image":"` + image + `"}}`
	if _, err := policy(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"{}", valid + "{}", strings.Replace(valid, image, "name:latest", 1), strings.Replace(valid, "isolated", "default", 1)} {
		if _, err := policy(strings.NewReader(invalid)); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
}

func TestOnlyOperatorSelectsProfile(t *testing.T) {
	a := agent{Profile: "isolated", Image: "test-image"}
	calls := 0
	h := handler(map[string]agent{"coding": a}, func(_ context.Context, selected agent, id, input string) (string, error) {
		calls++
		if selected != a || input != "--privileged" || !strings.HasPrefix(id, "ember-run-") {
			t.Fatal("wrong dispatch", selected, id, input)
		}
		return "done", nil
	})
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"agent":"coding","input":"--privileged"}`, http.StatusOK},
		{`{"agent":"coding","input":"x","profile":"trusted"}`, http.StatusBadRequest},
		{`{"agent":"coding","input":"x","image":"other"}`, http.StatusBadRequest},
		{`{"agent":"coding","input":"x"} {}`, http.StatusBadRequest},
		{`{"agent":"missing","input":"x"}`, http.StatusNotFound},
		{`{"agent":"coding","input":"` + strings.Repeat("x", 32769) + `"}`, http.StatusBadRequest},
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/runs", strings.NewReader(tc.body)))
		if rr.Code != tc.code {
			t.Fatalf("status %d, want %d", rr.Code, tc.code)
		}
	}
	if calls != 1 {
		t.Fatalf("launched %d jobs", calls)
	}
}

func TestKataFailureNeverRetriesInPodman(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("TRACE", filepath.Join(dir, "trace"))
	for name, script := range map[string]string{
		"docker": `echo "docker $*" >> "$TRACE"
case " $* " in
  *" image inspect "*) echo 0 ;;
  *" run "*) exit 125 ;;
esac`,
		"podman": `echo podman >> "$TRACE"`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, err := (runtimeLauncher{dir}).run(context.Background(), agent{"isolated", "image"}, "run-id", "task")
	if err == nil {
		t.Fatal("unavailable runtime reported success")
	}
	b, err := os.ReadFile(os.Getenv("TRACE"))
	if err != nil {
		t.Fatal(err)
	}
	trace := string(b)
	if strings.Contains(trace, "podman") || !strings.Contains(trace, "--runtime=io.containerd.kata.v2") || !strings.Contains(trace, "rm -f run-id") {
		t.Fatal(trace)
	}
}

func TestOutputLimitCancelsRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &cappedOutput{cancel: cancel}
	_, err := io.Copy(b, bytes.NewReader(bytes.Repeat([]byte("x"), 65537)))
	if err == nil || !b.overflow || ctx.Err() == nil || b.buf.Len() > 65536 {
		t.Fatal("output limit did not cancel run")
	}
}

func TestSnapshotRequiresVMAndAllowsLargerInput(t *testing.T) {
	calls := 0
	h := handler(map[string]agent{"tool": {Profile: "trusted"}, "coder": {Profile: "isolated"}},
		func(_ context.Context, a agent, _, _ string) (string, error) {
			calls++
			if a.Profile != "isolated" {
				t.Fatal("snapshot dispatched to shared kernel")
			}
			return "ok", nil
		})
	for _, tc := range []struct {
		agent string
		size  int
		want  int
	}{
		{"tool", 100, http.StatusForbidden},
		{"coder", 64000, http.StatusOK},
		{"coder", 1 << 20, http.StatusBadRequest},
	} {
		rr := httptest.NewRecorder()
		body := `{"agent":"` + tc.agent + `","input":"` + strings.Repeat("x", tc.size) + `"}`
		h.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/snapshot-runs", strings.NewReader(body)))
		if rr.Code != tc.want {
			t.Fatal(tc.agent, rr.Code, rr.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("unexpected launch count", calls)
	}
}
