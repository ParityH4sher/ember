// Package agents connects project chat to the operator's sandbox router
// (services/sandbox-router). It reads the operator's agents configuration,
// builds the chat-turn envelope an agent receives on stdin, and calls the
// router's POST /v1/runs over its Unix socket.
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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Agent is an image registered in the sandbox router's policy. ID is its
// router policy name; Name is the label users see.
type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Config is the operator's agents file, named by EMBER_AGENTS_CONFIG_FILE.
type Config struct {
	// RouterSocket is the absolute path of the sandbox router's Unix socket.
	RouterSocket string `json:"routerSocket"`
	// AllowWithoutSignIn lets chat runs start while OpenID Connect sign-in is
	// off. Only for loopback development: anyone who can reach the API could
	// then start containers on the operator's host.
	AllowWithoutSignIn bool    `json:"allowWithoutSignIn"`
	Agents             []Agent `json:"agents"`
}

const (
	maxConfigBytes = 64 << 10
	maxAgents      = 16
	maxAgentName   = 80
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
	if !filepath.IsAbs(c.RouterSocket) || filepath.Clean(c.RouterSocket) != c.RouterSocket {
		return errors.New("routerSocket must be a clean absolute path")
	}
	// No agents is valid: admins can then add them from the web app.
	if len(c.Agents) > maxAgents {
		return fmt.Errorf("configure at most %d agents", maxAgents)
	}
	seen := make(map[string]bool, len(c.Agents))
	for _, a := range c.Agents {
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
