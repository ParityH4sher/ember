package main

import (
	"context"
	_ "embed"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/httpapi"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/gitea"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
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
	defer listener.Close()
	root := os.Getenv("EMBER_PROJECT_ROOT")
	if root == "" {
		root = "projects"
	}
	vaults, err := projects.OpenVaults(root)
	if err != nil {
		return err
	}
	services, err := httpapi.ParseServiceLinks(os.Getenv("EMBER_SERVICE_LINKS"))
	if err != nil {
		return err
	}
	var extra []plugins.Registration
	if repo := os.Getenv("EMBER_GITEA_REPOSITORY_URL"); repo != "" {
		registration, err := gitea.Register(repo)
		if err != nil {
			return err
		}
		extra = append(extra, registration)
	}
	// Sign-in stays disabled unless an OpenID Connect configuration file is supplied.
	authConfig, err := auth.LoadConfig(os.Getenv("EMBER_AUTH_CONFIG_FILE"), os.Getenv("EMBER_OIDC_CLIENT_SECRET_FILE"))
	if err != nil {
		return err
	}
	authn := auth.New(authConfig, auth.Options{Logger: logger})
	// Chat runs stay off unless the operator names an agents configuration file.
	agentsConfig, err := agents.LoadConfig(os.Getenv("EMBER_AGENTS_CONFIG_FILE"))
	if err != nil {
		return err
	}
	if err := agentsAllowed(agentsConfig, authn.Mode()); err != nil {
		return err
	}
	var options []httpapi.Option
	var chatRuns *httpapi.ChatRuns
	agentCount := 0
	if agentsConfig != nil {
		chatRuns = httpapi.NewChatRuns(agentsConfig, agents.NewRouter(agentsConfig.RouterSocket), logger)
		options = append(options, httpapi.WithChatRuns(chatRuns))
		agentCount = len(agentsConfig.Agents)
	}
	srv := &http.Server{
		Handler:           accessLog(logger, authn.Wrap(httpapi.NewDeploymentHandler(version, vaults, services, extra, options...))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	logger.Info("api listening", "address", listener.Addr().String(), "version", version, "auth", authn.Mode(), "agents", agentCount)
	// Leave time for a forced close before Compose's default ten-second stop deadline.
	err = serve(ctx, srv, listener, 5*time.Second)
	if chatRuns != nil {
		// Stops agent containers still running and records them as interrupted.
		chatRuns.Close(3 * time.Second)
	}
	if err != nil {
		return err
	}
	logger.Info("api stopped")
	return nil
}

// agentsAllowed refuses chat runs while sign-in is off, unless the operator
// opted in for loopback development: anyone who can reach the API could
// otherwise start containers on the operator's host.
func agentsAllowed(c *agents.Config, authMode string) error {
	if c == nil || authMode != "none" || c.AllowWithoutSignIn {
		return nil
	}
	return errors.New("EMBER_AGENTS_CONFIG_FILE needs OpenID Connect sign-in (EMBER_AUTH_CONFIG_FILE); " +
		"set allowWithoutSignIn in the agents file only for loopback development")
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
