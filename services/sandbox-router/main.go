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
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
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

func serve(config, socket string, clientGID int) error {
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
	if err := checkSocketDir(dir, os.Geteuid(), clientGID); err != nil {
		return err
	}
	// Empty Docker client config: do not inject proxies from ~/.docker/config.json.
	clientDir, err := os.MkdirTemp("", "ember-docker-client-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(clientDir)
	// Before listening, so a signal never interrupts an unattended socket.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The socket is created as 0600 whatever the inherited umask.
	old := syscall.Umask(0177)
	l, err := net.Listen("unix", socket)
	syscall.Umask(old)
	if err != nil {
		return err
	}
	defer l.Close()
	// The group is set before the mode grants it access.
	mode := os.FileMode(0600)
	if clientGID >= 0 {
		if err := os.Chown(socket, -1, clientGID); err != nil {
			return fmt.Errorf("give the socket to the client group (the operator must be a member): %w", err)
		}
		mode = 0660
	}
	if err := os.Chmod(socket, mode); err != nil {
		return err
	}
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
	clientGroup := flag.String("client-group", "", "group name or GID that may also connect (socket 0660, directory 0710)")
	flag.Parse()
	if *config == "" || *socket == "" {
		flag.Usage()
		os.Exit(2)
	}
	clientGID, err := lookupGroup(*clientGroup)
	if err != nil {
		log.Fatal(err)
	}
	if err := serve(*config, *socket, clientGID); err != nil {
		log.Fatal(err)
	}
}

// lookupGroup resolves -client-group. An empty name means no client group: -1.
func lookupGroup(name string) (int, error) {
	if name == "" {
		return -1, nil
	}
	g, err := user.LookupGroup(name)
	if err != nil {
		if _, numeric := err.(user.UnknownGroupError); !numeric {
			return 0, err
		}
		if g, err = user.LookupGroupId(name); err != nil {
			return 0, fmt.Errorf("client group %q: %w", name, err)
		}
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil || gid <= 0 {
		return 0, fmt.Errorf("client group %q must not be root's group", name)
	}
	return gid, nil
}

// checkSocketDir requires a directory owned by the operator that nobody else
// can list. Without a client group it is mode 0700. With one, it is mode 0710
// and owned by that group, so its members can reach the socket but not list,
// create or remove anything in the directory.
func checkSocketDir(dir os.FileInfo, euid, clientGID int) error {
	st, ok := dir.Sys().(*syscall.Stat_t)
	if !dir.IsDir() || !ok || st.Uid != uint32(euid) {
		return errors.New("socket directory must be an operator-owned directory")
	}
	if clientGID < 0 {
		if dir.Mode().Perm() != 0700 {
			return errors.New("socket directory must be mode 0700")
		}
		return nil
	}
	if dir.Mode().Perm() != 0710 || st.Gid != uint32(clientGID) {
		return fmt.Errorf("with -client-group, the socket directory must be mode 0710 and owned by group %d", clientGID)
	}
	return nil
}
