// Package aicatalog holds the models and agents an Ember admin configures
// from the web app (ADR 015).
//
// Providers have one owner: the model gateway (LiteLLM, DEC-007). Provider
// endpoints and keys live in the gateway's configuration, never here. A
// catalog model offers one gateway model under a display name; its ID is the
// gateway's model name. An agent is either a harness image registered in the
// sandbox router policy (a turn agent) or a session on an operator-configured
// harness worker (a session agent), plus the models it may use.
//
// The catalog file holds no secrets. It is still written with mode 0600
// because older versions of it held provider keys.
package aicatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
)

// Schema identifies the catalog file format.
const Schema = "ember.ai-catalog/v2"

// schemaV1 is the format that also held providers and their keys. Open
// migrates it.
const schemaV1 = "ember.ai-catalog/v1"

// HarnessOpenCode is the harness of session agents: they run on a worker
// from the operator's agents file instead of the sandbox router.
const HarnessOpenCode = "opencode"

// Harnesses lists the agent harnesses an admin can pick. For router agents
// the harness is a label for people and for the image author; the router
// image is what runs.
var Harnesses = []string{"claude-code", "codex", HarnessOpenCode, "custom"}

const (
	maxModels     = 128
	maxAgents     = 16
	maxAgentModel = 16
	maxName       = 80
	maxModelID    = 200
	maxFileBytes  = 1 << 20
)

// modelID accepts a gateway model name such as "claude-sonnet",
// "openai/gpt-4o" or "qwen3:8b". The character set keeps names safe in a URL
// path segment (a "/" is sent percent-encoded) and in logs.
var modelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]*$`)

// ValidModelID reports whether id can name a gateway model in the catalog.
func ValidModelID(id string) bool {
	return len(id) <= maxModelID && modelID.MatchString(id)
}

// Model offers one gateway model under a display name.
type Model struct {
	ID   string `json:"id"` // the gateway's model name
	Name string `json:"name"`
}

// Agent is a turn agent (RouterAgent set, run by the sandbox router) or a
// session agent (Worker set, harness opencode) and the models it may use.
// DefaultModel is the one named in each chat turn.
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Harness      string   `json:"harness"`
	RouterAgent  string   `json:"routerAgent"`
	Worker       string   `json:"worker"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	Enabled      bool     `json:"enabled"`
}

type file struct {
	Schema string  `json:"schema"`
	Models []Model `json:"models"`
	Agents []Agent `json:"agents"`
}

// View is the whole catalog.
type View struct {
	Persisted bool    `json:"persisted"`
	Models    []Model `json:"models"`
	Agents    []Agent `json:"agents"`
}

// Migration describes what Open changed when it upgraded a v1 file. The zero
// value means nothing was migrated.
type Migration struct {
	From string // the old schema, empty when nothing was migrated
	// DroppedProviders and DroppedKeys count the providers and API keys
	// removed from the file. The keys are gone: operators configure them in
	// the model gateway instead.
	DroppedProviders int
	DroppedKeys      int
	// Relabelled lists v1 agents with harness opencode that ran as router
	// images; they keep running that way under harness custom.
	Relabelled []string
}

// Errors returned by Store methods. A *ValidationError explains bad input.
var (
	ErrNotFound = errors.New("not found")
	ErrInUse    = errors.New("in use")
)

// ValidationError is input an admin must correct; its text is safe to show.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// InUseError names what still refers to the item being deleted.
type InUseError struct{ Msg string }

func (e *InUseError) Error() string { return e.Msg }
func (e *InUseError) Unwrap() error { return ErrInUse }

// Store is the catalog, saved to a file after every change. Without a path it
// lives in memory and is lost when the API stops.
type Store struct {
	mu   sync.RWMutex
	path string
	c    file
	// reserved reports agent IDs taken by the operator's agents file.
	reserved func(string) bool
	// worker reports the worker IDs configured in the operator's agents file.
	worker    func(string) bool
	migration Migration
}

func never(string) bool { return false }

// Open loads the catalog at path, or starts an empty one if the file does not
// exist yet. An empty path keeps the catalog in memory. reserved, if not nil,
// reports agent IDs an admin may not use; worker, if not nil, reports the
// worker IDs a session agent may name. A v1 file is migrated and rewritten;
// Migration reports what changed.
func Open(path string, reserved, worker func(string) bool) (*Store, error) {
	s := &Store{path: path, c: file{Schema: Schema}, reserved: reserved, worker: worker}
	if s.reserved == nil {
		s.reserved = never
	}
	if s.worker == nil {
		s.worker = never
	}
	if path == "" {
		return s, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("AI catalog: the file path must be absolute")
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	if len(data) > maxFileBytes {
		return nil, errors.New("AI catalog: file exceeds 1 MiB")
	}
	var head struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	switch head.Schema {
	case Schema:
		if err := strictUnmarshal(data, &s.c); err != nil {
			return nil, fmt.Errorf("AI catalog: %w", err)
		}
	case schemaV1:
		c, m, err := migrateV1(data)
		if err != nil {
			return nil, fmt.Errorf("AI catalog: %w", err)
		}
		s.c, s.migration = c, m
	default:
		return nil, fmt.Errorf("AI catalog: unsupported schema %q", head.Schema)
	}
	if err := s.c.validate(s.reserved, s.worker); err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	if s.migration.From != "" {
		// Rewrite now so the keys do not stay on disk until the next change.
		if err := s.save(s.c); err != nil {
			return nil, fmt.Errorf("AI catalog: migrating from %s: %w", s.migration.From, err)
		}
	}
	return s, nil
}

// strictUnmarshal rejects unknown fields, so a v2 file that still carries
// provider keys (for example, copied by hand) is not silently accepted.
func strictUnmarshal(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// migrateV1 keeps model IDs and names and the agents of a v1 file and drops
// its providers and their keys. v1 model IDs were already the names agents
// asked the gateway for, so they stay valid.
func migrateV1(data []byte) (file, Migration, error) {
	var v1 struct {
		Providers []struct {
			APIKey string `json:"apiKey"`
		} `json:"providers"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
		Agents []Agent `json:"agents"`
	}
	if err := json.Unmarshal(data, &v1); err != nil {
		return file{}, Migration{}, err
	}
	m := Migration{From: schemaV1, DroppedProviders: len(v1.Providers)}
	for _, p := range v1.Providers {
		if p.APIKey != "" {
			m.DroppedKeys++
		}
	}
	c := file{Schema: Schema, Models: []Model{}, Agents: []Agent{}}
	for _, x := range v1.Models {
		c.Models = append(c.Models, Model{ID: x.ID, Name: x.Name})
	}
	for _, a := range v1.Agents {
		a.Worker = ""
		if a.Models == nil {
			a.Models = []string{}
		}
		// In v1, opencode was only a label on a router image. In v2 it means
		// a session on a worker, so keep the image running as before.
		if a.Harness == HarnessOpenCode {
			a.Harness = "custom"
			m.Relabelled = append(m.Relabelled, a.ID)
		}
		c.Agents = append(c.Agents, a)
	}
	return c, m, nil
}

// Migration reports what Open changed when it upgraded an older file.
func (s *Store) Migration() Migration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.migration
	m.Relabelled = slices.Clone(m.Relabelled)
	return m
}

// View returns the whole catalog.
func (s *Store) View() View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.c.clone()
	return View{Persisted: s.path != "", Models: c.Models, Agents: c.Agents}
}

// Resolved is an enabled agent ready for a chat.
type Resolved struct {
	Agent Agent
	Model *Model // nil when the agent has no model
}

// Agent returns an enabled agent with its default model.
func (s *Store) Agent(id string) (Resolved, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := slices.IndexFunc(s.c.Agents, func(a Agent) bool { return a.ID == id })
	if i < 0 || !s.c.Agents[i].Enabled {
		return Resolved{}, false
	}
	r := Resolved{Agent: s.c.Agents[i]}
	r.Agent.Models = append([]string{}, r.Agent.Models...)
	if m, ok := s.c.model(r.Agent.DefaultModel); ok {
		r.Model = &m
	}
	return r, true
}

// EnabledAgents lists the agents users may chat with.
func (s *Store) EnabledAgents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Agent
	for _, a := range s.c.Agents {
		if a.Enabled {
			a.Models = append([]string{}, a.Models...)
			out = append(out, a)
		}
	}
	return out
}

// PutModel creates or replaces the model with this ID.
func (s *Store) PutModel(id string, m Model) (Model, error) {
	m.ID, m.Name = id, strings.TrimSpace(m.Name)
	err := s.change(func(c *file) error {
		if i := slices.IndexFunc(c.Models, func(x Model) bool { return x.ID == id }); i >= 0 {
			c.Models[i] = m
		} else {
			c.Models = append(c.Models, m)
		}
		return nil
	})
	return m, err
}

// DeleteModel removes a model no agent uses.
func (s *Store) DeleteModel(id string) error {
	return s.change(func(c *file) error {
		i := slices.IndexFunc(c.Models, func(m Model) bool { return m.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		for _, a := range c.Agents {
			if slices.Contains(a.Models, id) {
				return &InUseError{Msg: fmt.Sprintf("agent %q uses this model; remove it from the agent first", a.ID)}
			}
		}
		c.Models = slices.Delete(c.Models, i, i+1)
		return nil
	})
}

// PutAgent creates or replaces the agent with this ID. An empty DefaultModel
// becomes the first of its models.
func (s *Store) PutAgent(id string, a Agent) (Agent, error) {
	a.ID, a.Name = id, strings.TrimSpace(a.Name)
	a.RouterAgent, a.Worker = strings.TrimSpace(a.RouterAgent), strings.TrimSpace(a.Worker)
	if a.Models == nil {
		a.Models = []string{}
	}
	if a.DefaultModel == "" && len(a.Models) > 0 {
		a.DefaultModel = a.Models[0]
	}
	err := s.change(func(c *file) error {
		if i := slices.IndexFunc(c.Agents, func(x Agent) bool { return x.ID == id }); i >= 0 {
			c.Agents[i] = a
		} else {
			c.Agents = append(c.Agents, a)
		}
		return nil
	})
	return a, err
}

// DeleteAgent removes an agent. Chats that used it stay readable.
func (s *Store) DeleteAgent(id string) error {
	return s.change(func(c *file) error {
		i := slices.IndexFunc(c.Agents, func(a Agent) bool { return a.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		c.Agents = slices.Delete(c.Agents, i, i+1)
		return nil
	})
}

// change applies edit to a copy, validates the result and saves it. The
// catalog is unchanged if any step fails.
func (s *Store) change(edit func(*file) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.c.clone()
	if err := edit(&next); err != nil {
		return err
	}
	if err := next.validate(s.reserved, s.worker); err != nil {
		return err
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.c = next
	return nil
}

// save writes the catalog atomically, readable only by this process's user.
// Renaming over the old file is not a secure erase: after a v1 migration the
// old keys may linger in free disk blocks, so operators should rotate them.
func (s *Store) save(c file) error {
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".ai-catalog-*.tmp")
	if err != nil {
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("saving the AI catalog: %w", err)
	}
	return nil
}

func (c file) clone() file {
	n := file{Schema: Schema,
		Models: append([]Model{}, c.Models...),
		Agents: make([]Agent, 0, len(c.Agents)),
	}
	for _, a := range c.Agents {
		a.Models = append([]string{}, a.Models...)
		n.Agents = append(n.Agents, a)
	}
	return n
}

func (c *file) model(id string) (Model, bool) {
	i := slices.IndexFunc(c.Models, func(m Model) bool { return m.ID == id })
	if i < 0 {
		return Model{}, false
	}
	return c.Models[i], true
}

func (c *file) validate(reserved, worker func(string) bool) error {
	if len(c.Models) > maxModels {
		return invalid("configure at most %d models", maxModels)
	}
	if len(c.Agents) > maxAgents {
		return invalid("configure at most %d agents", maxAgents)
	}
	models := map[string]bool{}
	for _, m := range c.Models {
		if !ValidModelID(m.ID) {
			return invalid("model ID %q: use the gateway's model name, 1 to %d letters, digits and . _ : / @ - characters, starting with a letter or digit", m.ID, maxModelID)
		}
		if models[m.ID] {
			return invalid("model %q is listed twice", m.ID)
		}
		models[m.ID] = true
		if !agents.ValidLabel(m.Name, maxName) {
			return invalid("model %q needs a name of 1 to %d characters on one line", m.ID, maxName)
		}
	}
	seen := map[string]bool{}
	for _, a := range c.Agents {
		if !agents.ValidID(a.ID) {
			return invalid("agent ID %q: use 1 to 63 lowercase letters, digits and hyphens, starting with a letter", a.ID)
		}
		if reserved(a.ID) {
			return invalid("agent ID %q is already used by the operator's agents file", a.ID)
		}
		if seen[a.ID] {
			return invalid("agent %q is listed twice", a.ID)
		}
		seen[a.ID] = true
		if !agents.ValidLabel(a.Name, maxName) {
			return invalid("agent %q needs a name of 1 to %d characters on one line", a.ID, maxName)
		}
		if !slices.Contains(Harnesses, a.Harness) {
			return invalid("agent %q: harness must be one of %s", a.ID, strings.Join(Harnesses, ", "))
		}
		if a.Harness == HarnessOpenCode {
			if a.RouterAgent != "" {
				return invalid("agent %q: an opencode agent runs on a worker; leave the router agent empty", a.ID)
			}
			if !agents.ValidID(a.Worker) || !worker(a.Worker) {
				return invalid("agent %q: choose a worker from the operator's agents file", a.ID)
			}
		} else {
			if a.Worker != "" {
				return invalid("agent %q: only opencode agents run on a worker; leave the worker empty", a.ID)
			}
			if !agents.ValidID(a.RouterAgent) {
				return invalid("agent %q: the router agent must be a policy name from the sandbox router (lowercase letters, digits and hyphens)", a.ID)
			}
		}
		if len(a.Models) > maxAgentModel {
			return invalid("agent %q may list at most %d models", a.ID, maxAgentModel)
		}
		listed := map[string]bool{}
		for _, m := range a.Models {
			if !models[m] {
				return invalid("agent %q names model %q, which does not exist", a.ID, m)
			}
			if listed[m] {
				return invalid("agent %q lists model %q twice", a.ID, m)
			}
			listed[m] = true
		}
		if a.DefaultModel != "" && !listed[a.DefaultModel] {
			return invalid("agent %q: the default model must be one of its models", a.ID)
		}
		if a.DefaultModel == "" && len(a.Models) > 0 {
			return invalid("agent %q needs a default model", a.ID)
		}
	}
	return nil
}
