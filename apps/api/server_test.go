package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	err = run(context.Background(), occupied.Addr().String(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("server reported success binding an occupied address")
	}
}

func TestShutdownWithActiveRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		grace time.Duration
		drain bool
	}{
		{"complete", 3 * time.Second, true},
		{"deadline", 50 * time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			release := make(chan struct{})
			stopping := make(chan struct{})
			finished := make(chan struct{})
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				close(started)
				select {
				case <-release:
					_, _ = io.WriteString(w, "complete")
				case <-r.Context().Done():
				}
			})}
			defer srv.Close()
			srv.RegisterOnShutdown(func() { close(stopping) })
			stopped := make(chan error, 1)
			go func() { stopped <- serve(ctx, srv, listener, tc.grace) }()
			response := make(chan string, 1)
			go func() {
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Get("http://" + listener.Addr().String())
				if err != nil {
					response <- ""
					return
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				response <- string(body)
			}()
			await(t, started)
			cancel()
			await(t, stopping)
			if tc.drain {
				close(release)
			}
			select {
			case err := <-stopped:
				if (tc.drain && err != nil) || (!tc.drain && !errors.Is(err, context.DeadlineExceeded)) {
					t.Fatalf("unexpected shutdown result: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("server failed to stop")
			}
			await(t, finished)
			if got := <-response; tc.drain && got != "complete" {
				t.Errorf("response %q: request did not drain", got)
			}
		})
	}
}

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lifecycle event")
	}
}
