package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// Runner runs one chat turn with an agent and returns its answer.
type Runner interface {
	Run(ctx context.Context, agent string, turn Turn) (Result, error)
}

// Result is a completed run.
type Result struct {
	RunID  string // the sandbox router's run ID, for its log
	Output string // the agent's answer: trimmed, valid UTF-8, never empty
}

// Failure codes. Each has a fixed message that is safe to show users; the
// router's own error text, which can name runtimes and exit codes, is dropped.
const (
	CodeRouterUnavailable = "router_unavailable"
	CodeUnknownAgent      = "unknown_agent"
	CodeRunnerBusy        = "runner_busy"
	CodeTimeout           = "agent_timeout"
	CodeFailed            = "agent_failed"
	CodeEmptyOutput       = "empty_output"
	CodeInterrupted       = "interrupted"
)

var messages = map[string]string{
	CodeRouterUnavailable: "The sandbox router is not reachable. Ask the operator to start it.",
	CodeUnknownAgent:      "This agent is not registered with the sandbox router.",
	CodeRunnerBusy:        "The sandbox runner is busy with another run. Try again shortly.",
	CodeTimeout:           "The agent did not answer within the time limit.",
	CodeFailed:            "The agent run failed. The operator can look up the run ID in the router log.",
	CodeEmptyOutput:       "The agent finished without an answer.",
	CodeInterrupted:       "The run was interrupted before the agent answered.",
}

// UserMessage returns the user-facing text for a failure code.
func UserMessage(code string) string {
	if m, ok := messages[code]; ok {
		return m
	}
	return messages[CodeFailed]
}

// RunError is a failed run. Status is the router's HTTP status, or 0 when the
// router did not answer.
type RunError struct {
	Code   string
	Status int
	RunID  string
}

func (e *RunError) Error() string { return "agent run failed: " + e.Code }

// maxResponse bounds the router's reply: 64 KiB of output can grow sixfold
// when JSON-escaped.
const maxResponse = 1 << 20

var runID = regexp.MustCompile(`^ember-run-[0-9a-f]{32}$`)

// Router calls the sandbox router's POST /v1/runs over its Unix socket.
type Router struct {
	client *http.Client
}

// NewRouter returns a client for the router listening on socket. It opens one
// connection per run, uses no proxy and follows no redirects. The caller's
// context sets the deadline.
func NewRouter(socket string) *Router {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives:  true,
		DisableCompression: true,
		MaxConnsPerHost:    1,
	}
	return &Router{client: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Run sends one turn to the router and waits for the agent's answer.
func (r *Router) Run(ctx context.Context, agent string, turn Turn) (Result, error) {
	body, err := encodeRequest(agent, turn)
	if err != nil || len(body) > MaxRequest {
		return Result{}, ErrTooLarge
	}
	// The host name is a placeholder; the transport always dials the socket.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://sandbox-router/v1/runs", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return Result{}, &RunError{Code: contextCode(ctx, CodeRouterUnavailable)}
	}
	defer resp.Body.Close()
	id := resp.Header.Get("X-Ember-Run-ID")
	if !runID.MatchString(id) {
		id = ""
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return Result{}, &RunError{Code: statusCode(resp.StatusCode), Status: resp.StatusCode, RunID: id}
	}
	var reply struct {
		RunID  string `json:"run_id"`
		Output string `json:"output"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return Result{}, &RunError{Code: contextCode(ctx, CodeFailed), Status: resp.StatusCode, RunID: id}
	}
	if len(data) > maxResponse || json.Unmarshal(data, &reply) != nil {
		return Result{}, &RunError{Code: CodeFailed, Status: resp.StatusCode, RunID: id}
	}
	if runID.MatchString(reply.RunID) {
		id = reply.RunID
	}
	output := CleanOutput(reply.Output)
	if output == "" {
		return Result{}, &RunError{Code: CodeEmptyOutput, Status: resp.StatusCode, RunID: id}
	}
	return Result{RunID: id, Output: output}, nil
}

// CleanOutput makes an agent's stdout safe to store as message text: valid
// UTF-8, no NUL bytes, no surrounding whitespace.
func CleanOutput(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

func statusCode(status int) string {
	switch status {
	case http.StatusNotFound:
		return CodeUnknownAgent
	case http.StatusTooManyRequests:
		return CodeRunnerBusy
	case http.StatusGatewayTimeout:
		return CodeTimeout
	}
	return CodeFailed
}

// contextCode distinguishes our own deadline or cancellation from a fault.
func contextCode(ctx context.Context, fallback string) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return CodeTimeout
	case ctx.Err() != nil:
		return CodeInterrupted
	}
	return fallback
}
