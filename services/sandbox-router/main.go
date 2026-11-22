// sandbox-router is an operator-local, synchronous agent launcher for Linux.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type agent struct {
	Profile string `json:"profile"`
	Image   string `json:"image"`
}

func decode(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}

func policy(r io.Reader) (map[string]agent, error) {
	var agents map[string]agent
	if err := decode(r, &agents); err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, errors.New("configure at least one agent")
	}
	id := regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	image := regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	for name, a := range agents {
		if !id.MatchString(name) || !image.MatchString(a.Image) || (a.Profile != "trusted" && a.Profile != "isolated") {
			return nil, fmt.Errorf("invalid name, image ID or profile for agent %q", name)
		}
	}
	return agents, nil
}

type launchFunc func(context.Context, agent, string, string) (string, error)

func handler(agents map[string]agent, launch launchFunc) http.Handler {
	mux := http.NewServeMux()
	slot := make(chan struct{}, 1)
	dispatch := func(w http.ResponseWriter, r *http.Request) {
		snapshot := r.URL.Path == "/v1/snapshot-runs"
		limit := int64(32 << 10)
		if snapshot {
			limit = 1 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		var req struct {
			Agent string `json:"agent"`
			Input string `json:"input"`
		}
		if err := decode(r.Body, &req); err != nil || req.Input == "" {
			http.Error(w, "expected an agent and non-empty text input within the request limit", http.StatusBadRequest)
			return
		}
		a, ok := agents[req.Agent]
		if !ok {
			http.Error(w, "unknown agent", http.StatusNotFound)
			return
		}
		if snapshot && a.Profile != "isolated" {
			http.Error(w, "snapshot runs require an isolated agent", http.StatusForbidden)
			return
		}
		select {
		case slot <- struct{}{}:
			defer func() { <-slot }()
		default:
			http.Error(w, "runner busy", http.StatusTooManyRequests)
			return
		}
		var nonce [16]byte
		rand.Read(nonce[:])
		id := "ember-run-" + hex.EncodeToString(nonce[:])
		w.Header().Set("X-Ember-Run-ID", id)
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		output, err := launch(ctx, a, id, req.Input)
		log.Printf("run=%s agent=%s profile=%s image=%s success=%t", id, req.Agent, a.Profile, a.Image, err == nil)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"run_id": id, "profile": a.Profile, "output": output})
	}
	mux.HandleFunc("POST /v1/runs", dispatch)
	mux.HandleFunc("POST /v1/snapshot-runs", dispatch)
	return mux
}

func serve(config, socket string) error {
	if os.Geteuid() == 0 {
		return errors.New("run as a non-root operator")
	}
	f, err := os.Open(config)
	if err != nil {
		return err
	}
	defer f.Close()
	agents, err := policy(f)
	if err != nil {
		return err
	}
	dir, err := os.Lstat(filepath.Dir(socket))
	if err != nil {
		return err
	}
	if !dir.IsDir() || dir.Mode().Perm() != 0700 || dir.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return errors.New("socket directory must be operator-owned and mode 0700")
	}
	// Empty Docker client config: do not inject proxies from ~/.docker/config.json.
	clientDir, err := os.MkdirTemp("", "ember-docker-client-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(clientDir)
	l, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &http.Server{
		Handler:     handler(agents, runtimeLauncher{clientDir}.run),
		BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: 5 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 10 * time.Second,
	}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s.Shutdown(shutdown)
		close(done)
	}()
	log.Printf("listening on %s", socket)
	if err := s.Serve(l); err != http.ErrServerClosed {
		return err
	}
	<-done
	return nil
}

func main() {
	config := flag.String("config", "", "operator agent policy JSON file")
	socket := flag.String("socket", "", "Unix socket path under a private directory")
	flag.Parse()
	if *config == "" || *socket == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := serve(*config, *socket); err != nil {
		log.Fatal(err)
	}
}
