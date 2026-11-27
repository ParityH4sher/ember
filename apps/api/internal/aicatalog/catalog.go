// Package aicatalog holds the model providers, models and agents an Ember
// admin configures from the web app.
//
// A provider is an endpoint and its credential (Anthropic, OpenAI, Ollama or
// any OpenAI-compatible server). A model is an Ember alias for one upstream
// model of a provider. An agent is a harness image registered in the sandbox
// router policy (Claude Code, Codex, OpenCode or a custom image) plus the
// models it may use.
//
// Provider credentials stay in this process and its catalog file: they are
// never returned by the API and never put in an agent's envelope. Agents learn
// only the model alias they should ask for; reaching the model goes through
// the model gateway (DEC-007), not through the agent holding a key.
package aicatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/ParityH4sher/ember/apps/api/internal/agents"
)

// Schema identifies the catalog file format.
const Schema = "ember.ai-catalog/v1"

// Provider kinds.
const (
	KindAnthropic        = "anthropic"
	KindOpenAI           = "openai"
	KindOllama           = "ollama"
	KindOpenAICompatible = "openai-compatible"
)

// Kinds lists the provider kinds in display order.
var Kinds = []string{KindAnthropic, KindOpenAI, KindOllama, KindOpenAICompatible}

// defaultBaseURL is used when an admin leaves the base URL empty. Ollama and
// OpenAI-compatible servers live wherever the operator runs them.
var defaultBaseURL = map[string]string{
	KindAnthropic: "https://api.anthropic.com/v1",
	KindOpenAI:    "https://api.openai.com/v1",
}

// Harnesses lists the agent harnesses an admin can pick. The harness is a
// label for people and for the image author; the router image is what runs.
var Harnesses = []string{"claude-code", "codex", "opencode", "custom"}

const (
	maxProviders  = 32
	maxModels     = 128
	maxAgents     = 16
	maxAgentModel = 16
	maxName       = 80
	maxUpstream   = 200
	maxBaseURL    = 300
	maxAPIKey     = 1024
	maxFileBytes  = 1 << 20
)

// Provider is a model endpoint and its credential.
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"baseURL"`
	// Local marks an endpoint on the operator's network. Hosted providers
	// (Anthropic, OpenAI) are never local.
	Local  bool   `json:"local"`
	APIKey string `json:"apiKey,omitempty"`
}

// Model is an Ember alias for one upstream model of a provider.
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Upstream string `json:"upstream"`
}

// Agent is a harness image in the sandbox router policy and the models it
// may use. DefaultModel is the one named in each chat turn.
type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Harness      string   `json:"harness"`
	RouterAgent  string   `json:"routerAgent"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	Enabled      bool     `json:"enabled"`
}

type file struct {
	Schema    string     `json:"schema"`
	Providers []Provider `json:"providers"`
	Models    []Model    `json:"models"`
	Agents    []Agent    `json:"agents"`
}

// PublicProvider is a provider as the API shows it: the key is replaced by
// whether one is set.
type PublicProvider struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"baseURL"`
	Local     bool   `json:"local"`
	HasAPIKey bool   `json:"hasAPIKey"`
}

// View is the whole catalog without secrets.
type View struct {
	Persisted bool             `json:"persisted"`
	Providers []PublicProvider `json:"providers"`
	Models    []Model          `json:"models"`
	Agents    []Agent          `json:"agents"`
}

// ProviderInput is an admin's change to a provider. A nil APIKey keeps the
// stored key; ClearAPIKey removes it.
type ProviderInput struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	BaseURL     string  `json:"baseURL"`
	Local       bool    `json:"local"`
	APIKey      *string `json:"apiKey,omitempty"`
	ClearAPIKey bool    `json:"clearApiKey,omitempty"`
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
}

// Open loads the catalog at path, or starts an empty one if the file does not
// exist yet. An empty path keeps the catalog in memory. reserved, if not nil,
// reports agent IDs an admin may not use.
func Open(path string, reserved func(string) bool) (*Store, error) {
	s := &Store{path: path, c: file{Schema: Schema}, reserved: reserved}
	if s.reserved == nil {
		s.reserved = func(string) bool { return false }
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
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	if len(data) > maxFileBytes {
		return nil, errors.New("AI catalog: file exceeds 1 MiB")
	}
	if err := json.Unmarshal(data, &s.c); err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	if s.c.Schema != Schema {
		return nil, fmt.Errorf("AI catalog: unsupported schema %q", s.c.Schema)
	}
	if err := s.c.validate(s.reserved); err != nil {
		return nil, fmt.Errorf("AI catalog: %w", err)
	}
	return s, nil
}

// View returns the catalog without secrets.
func (s *Store) View() View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := View{
		Persisted: s.path != "",
		Providers: make([]PublicProvider, 0, len(s.c.Providers)),
		Models:    append([]Model{}, s.c.Models...),
		Agents:    make([]Agent, 0, len(s.c.Agents)),
	}
	for _, p := range s.c.Providers {
		v.Providers = append(v.Providers, PublicProvider{
			ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, Local: p.Local, HasAPIKey: p.APIKey != "",
		})
	}
	for _, a := range s.c.Agents {
		a.Models = append([]string{}, a.Models...)
		v.Agents = append(v.Agents, a)
	}
	return v
}

// Provider returns a provider including its key, for calls made by the API.
func (s *Store) Provider(id string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i := slices.IndexFunc(s.c.Providers, func(p Provider) bool { return p.ID == id })
	if i < 0 {
		return Provider{}, false
	}
	return s.c.Providers[i], true
}

// Resolved is an enabled agent ready for a chat turn.
type Resolved struct {
	Agent    Agent
	Model    *Model // nil when the agent has no model
	Provider *PublicProvider
}

// Agent returns an enabled agent with its default model and provider.
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
		if p, ok := s.c.provider(m.Provider); ok {
			r.Provider = &PublicProvider{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, Local: p.Local, HasAPIKey: p.APIKey != ""}
		}
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

// PutProvider creates or replaces the provider with this ID.
func (s *Store) PutProvider(id string, in ProviderInput) (PublicProvider, error) {
	var out PublicProvider
	err := s.change(func(c *file) error {
		p := Provider{ID: id, Name: strings.TrimSpace(in.Name), Kind: in.Kind, BaseURL: strings.TrimSpace(in.BaseURL), Local: in.Local}
		i := slices.IndexFunc(c.Providers, func(p Provider) bool { return p.ID == id })
		if i >= 0 {
			p.APIKey = c.Providers[i].APIKey
		}
		switch {
		case in.ClearAPIKey:
			p.APIKey = ""
		case in.APIKey != nil:
			p.APIKey = strings.TrimSpace(*in.APIKey)
		}
		if p.BaseURL == "" {
			p.BaseURL = defaultBaseURL[p.Kind]
		}
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		if p.Kind == KindAnthropic || p.Kind == KindOpenAI {
			p.Local = false
		}
		// A stored key goes only to the origin it was entered for: moving the
		// provider without re-entering the key drops it, so nobody can send
		// it to another server by editing the URL.
		if i >= 0 && in.APIKey == nil && (c.Providers[i].BaseURL != p.BaseURL || c.Providers[i].Kind != p.Kind) {
			p.APIKey = ""
		}
		if i >= 0 {
			c.Providers[i] = p
		} else {
			c.Providers = append(c.Providers, p)
		}
		out = PublicProvider{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, Local: p.Local, HasAPIKey: p.APIKey != ""}
		return nil
	})
	return out, err
}

// DeleteProvider removes a provider no model uses.
func (s *Store) DeleteProvider(id string) error {
	return s.change(func(c *file) error {
		i := slices.IndexFunc(c.Providers, func(p Provider) bool { return p.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		for _, m := range c.Models {
			if m.Provider == id {
				return &InUseError{Msg: fmt.Sprintf("model %q uses this provider; delete or move it first", m.ID)}
			}
		}
		c.Providers = slices.Delete(c.Providers, i, i+1)
		return nil
	})
}

// PutModel creates or replaces the model with this ID.
func (s *Store) PutModel(id string, m Model) (Model, error) {
	m.ID, m.Name, m.Upstream = id, strings.TrimSpace(m.Name), strings.TrimSpace(m.Upstream)
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
	if err := next.validate(s.reserved); err != nil {
		return err
	}
	if err := s.save(next); err != nil {
		return err
	}
	s.c = next
	return nil
}

// save writes the catalog atomically, readable only by this process's user:
// it holds provider keys.
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
		Providers: append([]Provider{}, c.Providers...),
		Models:    append([]Model{}, c.Models...),
		Agents:    make([]Agent, 0, len(c.Agents)),
	}
	for _, a := range c.Agents {
		a.Models = append([]string{}, a.Models...)
		n.Agents = append(n.Agents, a)
	}
	return n
}

func (c *file) provider(id string) (Provider, bool) {
	i := slices.IndexFunc(c.Providers, func(p Provider) bool { return p.ID == id })
	if i < 0 {
		return Provider{}, false
	}
	return c.Providers[i], true
}

func (c *file) model(id string) (Model, bool) {
	i := slices.IndexFunc(c.Models, func(m Model) bool { return m.ID == id })
	if i < 0 {
		return Model{}, false
	}
	return c.Models[i], true
}

func (c *file) validate(reserved func(string) bool) error {
	if len(c.Providers) > maxProviders {
		return invalid("configure at most %d providers", maxProviders)
	}
	if len(c.Models) > maxModels {
		return invalid("configure at most %d models", maxModels)
	}
	if len(c.Agents) > maxAgents {
		return invalid("configure at most %d agents", maxAgents)
	}
	providers := map[string]bool{}
	for _, p := range c.Providers {
		if err := p.validate(); err != nil {
			return err
		}
		if providers[p.ID] {
			return invalid("provider %q is listed twice", p.ID)
		}
		providers[p.ID] = true
	}
	models := map[string]bool{}
	for _, m := range c.Models {
		if !agents.ValidID(m.ID) {
			return invalid("model ID %q: use 1 to 63 lowercase letters, digits and hyphens, starting with a letter", m.ID)
		}
		if models[m.ID] {
			return invalid("model %q is listed twice", m.ID)
		}
		models[m.ID] = true
		if !agents.ValidLabel(m.Name, maxName) {
			return invalid("model %q needs a name of 1 to %d characters on one line", m.ID, maxName)
		}
		if !providers[m.Provider] {
			return invalid("model %q names provider %q, which does not exist", m.ID, m.Provider)
		}
		if !validUpstream(m.Upstream) {
			return invalid("model %q needs the provider's model ID: 1 to %d characters without spaces", m.ID, maxUpstream)
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
		if !agents.ValidID(a.RouterAgent) {
			return invalid("agent %q: the router agent must be a policy name from the sandbox router (lowercase letters, digits and hyphens)", a.ID)
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

// validUpstream accepts a provider's model ID, such as "qwen3:8b".
func validUpstream(s string) bool {
	return agents.ValidLabel(s, maxUpstream) && strings.IndexFunc(s, unicode.IsSpace) < 0
}

func (p Provider) validate() error {
	if !agents.ValidID(p.ID) {
		return invalid("provider ID %q: use 1 to 63 lowercase letters, digits and hyphens, starting with a letter", p.ID)
	}
	if !agents.ValidLabel(p.Name, maxName) {
		return invalid("provider %q needs a name of 1 to %d characters on one line", p.ID, maxName)
	}
	if !slices.Contains(Kinds, p.Kind) {
		return invalid("provider %q: kind must be one of %s", p.ID, strings.Join(Kinds, ", "))
	}
	if p.BaseURL == "" {
		return invalid("provider %q needs a base URL", p.ID)
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || len(p.BaseURL) > maxBaseURL || (u.Scheme != "http" && u.Scheme != "https") ||
		u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return invalid("provider %q: the base URL must be an http or https URL without credentials, query or fragment", p.ID)
	}
	if !p.Local && u.Scheme != "https" {
		return invalid("provider %q: a hosted provider needs an https base URL; mark it local if it runs on your network", p.ID)
	}
	if len(p.APIKey) > maxAPIKey {
		return invalid("provider %q: the API key is longer than %d bytes", p.ID, maxAPIKey)
	}
	for _, r := range p.APIKey {
		if r <= ' ' || r > '~' {
			return invalid("provider %q: the API key may contain only printable ASCII without spaces", p.ID)
		}
	}
	return nil
}
