//go:build qualification

package main

import (
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

// Runs in a separate probe image under exactly the worker's runtime flags.
func TestBoundary(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("running as root")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil || !strings.Contains(string(status), "CapEff:\t0000000000000000\n") ||
		!strings.Contains(string(status), "NoNewPrivs:\t1\n") {
		t.Fatal("missing capability or no-new-privileges restriction")
	}
	if _, err := os.ReadFile(inputPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{inputPath, "/unexpected"} {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		if err == nil {
			f.Close()
		}
		if !errors.Is(err, syscall.EROFS) {
			t.Fatalf("%s: expected read-only filesystem, got %v", path, err)
		}
	}
	for _, path := range []string{os.Getenv("CANARY_PATH"), "/run/podman/podman.sock", "/var/run/docker.sock"} {
		if path == "" {
			t.Fatal("missing canary path")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s: expected absent, got %v", path, err)
		}
	}
	if entries, err := os.ReadDir("/run/secrets"); !os.IsNotExist(err) && (err != nil || len(entries) != 0) {
		t.Fatal("unexpected secret mount")
	}
	if _, ok := os.LookupEnv("IMMICH_API_KEY"); ok {
		t.Fatal("inherited host credential variable")
	}
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) != 1 || ifaces[0].Flags&net.FlagLoopback == 0 {
		t.Fatalf("expected only loopback, got %v (%v)", ifaces, err)
	}
}
