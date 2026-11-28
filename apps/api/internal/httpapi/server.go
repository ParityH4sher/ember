package httpapi

import (
	"net/http"
	"sync"

	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
	"github.com/ParityH4sher/ember/apps/api/internal/auth"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/immich"
	"github.com/ParityH4sher/ember/apps/api/internal/plugins/weather"
	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

type pluginState struct {
	definition plugins.Definition
	active     bool
	configured bool
	values     map[string]any
	adapter    plugins.Adapter
}

type Server struct {
	mu       sync.RWMutex
	plugins  map[string]*pluginState
	projects projects.Store
	version  string
}

// NewHandler creates the API with the plugins compiled into this deployment.
func NewHandler(version string, vaults *projects.Vaults, services ...ServiceLink) http.Handler {
	return NewDeploymentHandler(version, vaults, services, nil)
}

// Option adds an optional capability to the deployment handler.
type Option func(*options)

type options struct {
	chatRuns *ChatRuns
	catalog  *aicatalog.Store
	gateway  *aicatalog.Gateway
	workers  []WorkerInfo
}

// WithChatRuns lets chats start agent runs. Without it, chats are read-only
// and GET /v1/agents reports enabled: false.
func WithChatRuns(runs *ChatRuns) Option {
	return func(o *options) { o.chatRuns = runs }
}

// WithAICatalog uses store for the models and agents admins configure.
// Without it, the handler keeps an in-memory catalog.
func WithAICatalog(store *aicatalog.Store) Option {
	return func(o *options) { o.catalog = store }
}

// WithModelGateway lets admins list the model gateway's models. Without it
// (or with nil), GET /v1/admin/ai/gateway/models answers 503.
func WithModelGateway(gateway *aicatalog.Gateway) Option {
	return func(o *options) { o.gateway = gateway }
}

// WithWorkers names the harness workers from the operator's agents file, for
// the admin page. The catalog's own worker check is the callback passed to
// aicatalog.Open.
func WithWorkers(workers []WorkerInfo) Option {
	return func(o *options) { o.workers = append([]WorkerInfo{}, workers...) }
}

func NewDeploymentHandler(version string, vaults *projects.Vaults, services []ServiceLink, extra []plugins.Registration, opts ...Option) http.Handler {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return newHandler(version, vaults, append([]plugins.Registration{
		{Definition: immich.Definition(), Adapter: immich.New()},
		{Definition: weather.Definition(), Adapter: weather.New()},
	}, extra...), services, o)
}

// NewHandlerWithPlugins is also useful for deterministic adapter tests.
func NewHandlerWithPlugins(version string, registrations []plugins.Registration) http.Handler {
	return newHandler(version, projects.NewMemoryStore(), registrations, nil, options{})
}

func newHandler(version string, store projects.Store, registrations []plugins.Registration, services []ServiceLink, o options) http.Handler {
	server := &Server{
		plugins:  make(map[string]*pluginState, len(registrations)),
		projects: store,
		version:  version,
	}
	for _, registration := range registrations {
		definition := registration.Definition
		server.plugins[definition.ID] = &pluginState{
			definition: definition,
			values:     map[string]any{},
			configured: len(requiredFields(definition.ConfigurationSchema)) == 0,
			adapter:    registration.Adapter,
		}
	}

	// Each route names the least role that may call it. auth.Wrap sets the
	// caller's role; without one, every route except /healthz refuses.
	mux := http.NewServeMux()
	route := func(pattern string, least auth.Role, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, allow(least, handler))
	}
	mux.HandleFunc("GET /healthz", server.health)
	route("GET /v1/services", auth.RoleUser, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"services": append([]ServiceLink{}, services...)})
	})
	route("GET /v1/plugins", auth.RoleUser, server.listPlugins)
	route("GET /v1/plugins/{pluginId}", auth.RoleUser, server.getPlugin)
	route("GET /v1/plugins/{pluginId}/configuration", auth.RoleUser, server.getPluginConfiguration)
	route("PUT /v1/plugins/{pluginId}/configuration", auth.RoleAdmin, server.setPluginConfiguration)
	route("PUT /v1/plugins/{pluginId}/activation", auth.RoleAdmin, server.setPluginActivation)
	route("GET /v1/plugins/{pluginId}/api", auth.RoleUser, server.listPluginOperations)
	route("POST /v1/plugins/{pluginId}/api/{apiVersion}/{operationId}", auth.RoleUser, server.invokePluginOperation)
	route("POST /v1/projects", auth.RoleUser, server.createProject)
	route("GET /v1/projects", auth.RoleUser, server.listProjects)
	route("DELETE /v1/projects/{projectId}", auth.RoleUser, server.deleteProject)
	if o.catalog == nil {
		o.catalog, _ = aicatalog.Open("", nil, nil) // in memory; cannot fail
	}
	chats := &chatAPI{runs: o.chatRuns, catalog: o.catalog, projects: store}
	route("GET /v1/agents", auth.RoleUser, chats.listAgents)
	ai := &aiAdmin{catalog: o.catalog, gateway: o.gateway, workers: o.workers, chats: chats, operator: func() []agentInfo {
		list := []agentInfo{}
		if chats.runs != nil {
			for _, a := range chats.runs.config.Agents {
				list = append(list, agentInfo{ID: a.ID, Name: a.Name})
			}
		}
		return list
	}}
	route("GET /v1/admin/ai", auth.RoleAdmin, ai.get)
	route("GET /v1/admin/ai/gateway/models", auth.RoleAdmin, ai.gatewayModels)
	route("PUT /v1/admin/ai/models/{id}", auth.RoleAdmin, ai.putModel)
	route("DELETE /v1/admin/ai/models/{id}", auth.RoleAdmin, ai.deleteModel)
	route("PUT /v1/admin/ai/agents/{id}", auth.RoleAdmin, ai.putAgent)
	route("DELETE /v1/admin/ai/agents/{id}", auth.RoleAdmin, ai.deleteAgent)
	if vaults, ok := store.(*projects.Vaults); ok {
		route("GET /v1/projects/{projectId}/notes", auth.RoleUser, listNotes(vaults))
		route("PUT /v1/projects/{projectId}/notes", auth.RoleUser, saveNote(vaults))
		// Chat runs start containers on the router's host. Roles are not yet
		// scoped to projects, so any user may chat in any project.
		chats.vaults = vaults
		route("GET /v1/projects/{projectId}/chats", auth.RoleUser, chats.list)
		route("POST /v1/projects/{projectId}/chats", auth.RoleUser, chats.create)
		route("GET /v1/projects/{projectId}/chats/{chatId}", auth.RoleUser, chats.get)
		route("POST /v1/projects/{projectId}/chats/{chatId}/messages", auth.RoleUser, chats.send)
	}
	return mux
}

// allow runs handler only for callers holding at least the least role.
func allow(least auth.Role, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth.RoleFrom(r.Context()) < least {
			writeError(w, http.StatusForbidden, "insufficient_role", "this action needs the Ember "+least.String()+" role")
			return
		}
		handler(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}
