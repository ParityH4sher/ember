package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// CI stamps this file in the source bundle. A checkout identifies itself as development.
//
//go:embed version.txt
var versionFile string

var version = strings.TrimSpace(versionFile)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	address := os.Getenv("EMBER_LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	if err := run(ctx, address, logger); err != nil {
		logger.Error("api failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, address string, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           accessLog(logger, httpHandler()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	logger.Info("api listening", "address", listener.Addr().String(), "version", version)
	// Leave time for a forced close before Compose's default ten-second stop deadline.
	if err := serve(ctx, srv, listener, 5*time.Second); err != nil {
		return err
	}
	logger.Info("api stopped")
	return nil
}

func serve(ctx context.Context, srv *http.Server, listener net.Listener, grace time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// A cancelled signal context cannot be used to wait for active requests.
		drain, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		err := srv.Shutdown(drain)
		if err != nil {
			_ = srv.Close()
		}
		if serveErr := <-done; !errors.Is(serveErr, http.ErrServerClosed) {
			return errors.Join(err, serveErr)
		}
		return err
	}
}

func httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version,
		})
	})
	return mux
}
