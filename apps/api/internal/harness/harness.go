// Package harness is Ember's interface to agent harnesses that keep a
// long-lived session per chat (ADR 015). The harness owns the agent loop,
// history, tools, permissions and streaming; Ember keeps sign-in, project
// scoping and a copy of each transcript.
//
// The interface is modelled on OpenCode's session API so that other
// harnesses can sit behind it later. Implementations keep upstream error
// text (which can include provider responses) to themselves: callers only
// see ErrNotFound or ErrUnavailable.
package harness

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Harness is one worker's session API.
type Harness interface {
	// Open creates the session, or resumes it if it exists. Idempotent.
	Open(ctx context.Context, s Session) error
	// Prompt starts a run, or steers the active run with more input.
	Prompt(ctx context.Context, sessionID, text string) error
	// Interrupt stops the active run and reports whether one was stopped.
	Interrupt(ctx context.Context, sessionID string) (bool, error)
	// Messages returns up to limit of the newest messages, oldest first.
	Messages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	Permissions(ctx context.Context, sessionID string) ([]Permission, error)
	Reply(ctx context.Context, sessionID, permissionID string, d Decision) error
	// Subscribe streams events for all sessions of this worker. The channel
	// closes when the stream fails; the caller resubscribes and resyncs.
	Subscribe(ctx context.Context) (<-chan Event, error)
}

// Session names a harness session and its working directory in the worker.
type Session struct{ ID, Directory string }

// Message roles.
const (
	RoleUser  = "user"
	RoleAgent = "agent"
	RoleTool  = "tool"
	RoleRun   = "run" // the end of a run; Text is its outcome
)

// Message is one entry of a session's history.
type Message struct {
	Role string // "user", "agent", "tool", "run"
	Text string // text, "name: status" for tools, outcome for runs
}

// Permission is a tool call waiting for a person's decision.
type Permission struct {
	ID        string   `json:"id"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
}

// Decision answers a permission request.
type Decision string

const (
	Once   Decision = "once"
	Reject Decision = "reject"
)

// Event types. These are Ember's names, not the harness's.
const (
	TextDelta         = "text.delta"
	ToolCalled        = "tool.called"
	ToolSucceeded     = "tool.succeeded"
	ToolFailed        = "tool.failed"
	RunStarted        = "run.started"
	RunSucceeded      = "run.succeeded"
	RunFailed         = "run.failed"
	RunInterrupted    = "run.interrupted"
	PermissionAsked   = "permission.asked"
	PermissionReplied = "permission.replied"
)

// Event is one live event of a session.
type Event struct {
	Session string `json:"-"`
	Type    string `json:"type"`
	Delta   string `json:"delta,omitempty"`
}

// RunEnded reports whether t ends a run.
func RunEnded(t string) bool {
	return t == RunSucceeded || t == RunFailed || t == RunInterrupted
}

var (
	// ErrNotFound means the session or permission does not exist.
	ErrNotFound = errors.New("harness: not found")
	// ErrUnavailable means the worker did not answer, refused the request or
	// answered with something unexpected. The cause is never user-safe.
	ErrUnavailable = errors.New("harness: worker unavailable")
)

const sessionPrefix = "ses_ember_"

var chatIDForm = regexp.MustCompile(`^[0-9a-f]{16}$`)

// SessionID is the harness session of a chat.
func SessionID(chatID string) string { return sessionPrefix + chatID }

// ChatID returns the chat of an Ember session ID, or false for any other
// session (a worker may hold sessions Ember did not create).
func ChatID(sessionID string) (string, bool) {
	id, ok := strings.CutPrefix(sessionID, sessionPrefix)
	return id, ok && chatIDForm.MatchString(id)
}
