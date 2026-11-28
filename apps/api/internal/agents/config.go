// Package agents connects project chat to the operator's sandbox router
// (services/sandbox-router). It reads the operator's agents configuration,
// builds the chat-turn envelope an agent receives on stdin, and calls the
// router's POST /v1/runs over its Unix socket.
//
// The agents file also names harness workers (ADR 015): an agent with a
// worker is a session agent whose chats are long-lived harness sessions
// (package harness); one without is a turn agent run once per message.
//
// Ember defines the envelope, not the agent: an agent is whatever image the
// operator registered in the router policy. Nothing here grants an agent
// network access, tools or files; the router enforces the sandbox.
package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Agent is a chat agent. Without Worker it is a turn agent: an image
// registered in the sandbox router's policy, and ID is its policy name. With
// Worker it is a session agent served by that harness worker. Name is the
// label users see.
type Agent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Worker string `json:"worker,omitempty"`
}

// Worker is a harness server that holds chat sessions, such as an OpenCode
// worker. Its address and password are operator decisions, never browser
// inputs.
type Worker struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	// URL is http://host[:port], with no path, query or user info.
	URL          string `json:"url"`
	PasswordFile string `json:"passwordFile"`
	// Directory is the session working directory inside the worker.
	Directory string `json:"directory,omitempty"`

	password string // read from PasswordFile when the file is loaded
}

// Password is the worker's password, read when the agents file was loaded.
func (w Worker) Password() string { return w.password }

// Harnesses lists the harnesses a worker can run.
var Harnesses = []string{"opencode"}

// Config is the operator's agents file, named by EMBER_AGENTS_CONFIG_FILE.
type Config struct {
	// RouterSocket is the absolute path of the sandbox router's Unix socket.
	// It is required only when a turn agent is configured; without it, turn
	// runs fail with router_unavailable.
	RouterSocket string `json:"routerSocket,omitempty"`
	// AllowWithoutSignIn lets chat runs start while OpenID Connect sign-in is
	// off. Only for loopback development: anyone who can reach the API could
	// then start containers on the operator's host.
	AllowWithoutSignIn bool     `json:"allowWithoutSignIn"`
	Workers            []Worker `json:"workers,omitempty"`
	Agents             []Agent  `json:"agents"`
}

const (
	maxConfigBytes = 64 << 10
	maxAgents      = 16
	maxAgentName   = 80
	maxWorkers     = 4
	// minPassword matches the worker deployment's requirement.
	minPassword     = 32
	maxPasswordFile = 4 << 10
	defaultDir      = "/workspace"
)

// The sandbox router accepts the same policy names.
var agentID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// LoadConfig reads the agents file. An empty path means chat runs are off and
// returns a nil Config.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("agents configuration: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("agents configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return nil, errors.New("agents configuration: file exceeds 64 KiB")
	}
	return ParseConfig(data)
}

// ParseConfig decodes and validates an agents configuration document.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("agents configuration: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("agents configuration: expected one JSON object")
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("agents configuration: %w", err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.RouterSocket != "" && (!filepath.IsAbs(c.RouterSocket) || filepath.Clean(c.RouterSocket) != c.RouterSocket) {
		return errors.New("routerSocket must be a clean absolute path")
	}
	if len(c.Workers) > maxWorkers {
		return fmt.Errorf("configure at most %d workers", maxWorkers)
	}
	workers := make(map[string]bool, len(c.Workers))
	for i := range c.Workers {
		w := &c.Workers[i]
		if err := w.validate(); err != nil {
			return err
		}
		if workers[w.ID] {
			return fmt.Errorf("worker id %q is listed twice", w.ID)
		}
		workers[w.ID] = true
	}
	// No agents is valid: admins can then add them from the web app.
	if len(c.Agents) > maxAgents {
		return fmt.Errorf("configure at most %d agents", maxAgents)
	}
	seen := make(map[string]bool, len(c.Agents))
	for _, a := range c.Agents {
		if a.Worker != "" && !workers[a.Worker] {
			return fmt.Errorf("agent %q names worker %q, which is not configured", a.ID, a.Worker)
		}
		if a.Worker == "" && c.RouterSocket == "" {
			return fmt.Errorf("agent %q is a router agent and needs routerSocket", a.ID)
		}
		if !agentID.MatchString(a.ID) {
			return fmt.Errorf("agent id %q must match %s", a.ID, agentID)
		}
		if seen[a.ID] {
			return fmt.Errorf("agent id %q is listed twice", a.ID)
		}
		seen[a.ID] = true
		if !displayText(a.Name, maxAgentName) {
			return fmt.Errorf("agent %q needs a name of 1 to %d printable characters", a.ID, maxAgentName)
		}
	}
	return nil
}

// validate checks the worker and reads its password file.
func (w *Worker) validate() error {
	if !agentID.MatchString(w.ID) {
		return fmt.Errorf("worker id %q must match %s", w.ID, agentID)
	}
	if w.Harness != "opencode" {
		return fmt.Errorf("worker %q: harness must be opencode", w.ID)
	}
	if !workerURL(w.URL) {
		return fmt.Errorf("worker %q: url must be http://host[:port] with no path, query or user info", w.ID)
	}
	if w.Directory == "" {
		w.Directory = defaultDir
	}
	if !filepath.IsAbs(w.Directory) || filepath.Clean(w.Directory) != w.Directory {
		return fmt.Errorf("worker %q: directory must be a clean absolute path", w.ID)
	}
	if w.PasswordFile == "" {
		return fmt.Errorf("worker %q needs a passwordFile", w.ID)
	}
	password, err := readPassword(w.PasswordFile)
	if err != nil {
		return fmt.Errorf("worker %q: %w", w.ID, err)
	}
	w.password = password
	return nil
}

func workerURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Opaque != "" || u.User != nil || u.Hostname() == "" ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	return u.String() == raw
}

// readPassword reads a secret file. Surrounding whitespace, such as the
// newline editors add, is not part of the password.
func readPassword(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("passwordFile cannot be read")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPasswordFile+1))
	if err != nil || len(data) > maxPasswordFile {
		return "", errors.New("passwordFile cannot be read")
	}
	password := strings.TrimSpace(string(data))
	if len(password) < minPassword {
		return "", fmt.Errorf("passwordFile must hold at least %d bytes", minPassword)
	}
	for _, r := range password {
		if unicode.IsControl(r) {
			return "", errors.New("passwordFile must hold a single line")
		}
	}
	return password, nil
}

// ValidID reports whether id is a valid agent ID, which is also the form the
// sandbox router accepts for policy names.
func ValidID(id string) bool { return agentID.MatchString(id) }

// ValidLabel reports whether s is a trimmed, non-empty, single-line label of
// at most maxRunes printable characters.
func ValidLabel(s string, maxRunes int) bool { return displayText(s, maxRunes) }

// Agent returns the configured agent with this ID.
func (c *Config) Agent(id string) (Agent, bool) {
	for _, a := range c.Agents {
		if a.ID == id {
			return a, true
		}
	}
	return Agent{}, false
}

// Worker returns the configured worker with this ID. It is safe to call on
// a nil Config, which has no workers.
func (c *Config) Worker(id string) (Worker, bool) {
	if c == nil {
		return Worker{}, false
	}
	for _, w := range c.Workers {
		if w.ID == id {
			return w, true
		}
	}
	return Worker{}, false
}

// WorkerIDs lists the configured workers' IDs, for example to validate admin
// catalog agents that name a worker. It is safe to call on a nil Config.
func (c *Config) WorkerIDs() []string {
	if c == nil {
		return []string{}
	}
	ids := make([]string, 0, len(c.Workers))
	for _, w := range c.Workers {
		ids = append(ids, w.ID)
	}
	return ids
}

// displayText accepts a trimmed, non-empty single-line label.
func displayText(s string, maxRunes int) bool {
	if s == "" || s != strings.TrimSpace(s) || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
