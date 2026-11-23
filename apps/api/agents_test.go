package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
)

func TestAgentsNeedSignIn(t *testing.T) {
	config := &agents.Config{RouterSocket: "/run/router.sock", Agents: []agents.Agent{{ID: "a", Name: "A"}}}
	if err := agentsAllowed(nil, "none"); err != nil {
		t.Fatalf("no agents file: %v", err)
	}
	if err := agentsAllowed(config, "oidc"); err != nil {
		t.Fatalf("with sign-in: %v", err)
	}
	if err := agentsAllowed(config, "none"); err == nil || !strings.Contains(err.Error(), "allowWithoutSignIn") {
		t.Fatalf("without sign-in: %v", err)
	}
	config.AllowWithoutSignIn = true
	if err := agentsAllowed(config, "none"); err != nil {
		t.Fatalf("explicit loopback opt-in: %v", err)
	}
}

func TestRunRefusesAgentsWithoutSignIn(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agents.json")
	if err := os.WriteFile(file, []byte(`{"routerSocket":"/run/router.sock","agents":[{"id":"a","name":"A"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBER_PROJECT_ROOT", t.TempDir())
	t.Setenv("EMBER_AUTH_CONFIG_FILE", "")
	t.Setenv("EMBER_AGENTS_CONFIG_FILE", file)
	err := run(context.Background(), "127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "sign-in") {
		t.Fatalf("run = %v, want a sign-in error before serving", err)
	}
}
