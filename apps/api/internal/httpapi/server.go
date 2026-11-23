package httpapi

import (
	"net/http"
	"sync"

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

func NewDeploymentHandler(version string, vaults *projects.Vaults, services []ServiceLink, extra []plugins.Registration) http.Handler {
	return newHandler(version, vaults, append([]plugins.Registration{
		{Definition: immich.Definition(), Adapter: immich.New()},
		{Definition: weather.Definition(), Adapter: weather.New()},
	}, extra...), services...)
}

// NewHandlerWithPlugins is also useful for deterministic adapter tests.
func NewHandlerWithPlugins(version string, registrations []plugins.Registration) http.Handler {
	return newHandler(version, projects.NewMemoryStore(), registrations)
}

func newHandler(version string, store projects.Store, registrations []plugins.Registration, services ...ServiceLink) http.Handler {
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /v1/services", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"services": append([]ServiceLink{}, services...)})
	})
	mux.HandleFunc("GET /v1/plugins", server.listPlugins)
	mux.HandleFunc("GET /v1/plugins/{pluginId}", server.getPlugin)
	mux.HandleFunc("GET /v1/plugins/{pluginId}/configuration", server.getPluginConfiguration)
	mux.HandleFunc("PUT /v1/plugins/{pluginId}/configuration", server.setPluginConfiguration)
	mux.HandleFunc("PUT /v1/plugins/{pluginId}/activation", server.setPluginActivation)
	mux.HandleFunc("GET /v1/plugins/{pluginId}/api", server.listPluginOperations)
	mux.HandleFunc("POST /v1/plugins/{pluginId}/api/{apiVersion}/{operationId}", server.invokePluginOperation)
	mux.HandleFunc("POST /v1/projects", server.createProject)
	mux.HandleFunc("GET /v1/projects", server.listProjects)
	mux.HandleFunc("DELETE /v1/projects/{projectId}", server.deleteProject)
	if vaults, ok := store.(*projects.Vaults); ok {
		mux.HandleFunc("GET /v1/projects/{projectId}/notes", listNotes(vaults))
		mux.HandleFunc("PUT /v1/projects/{projectId}/notes", saveNote(vaults))
	}
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}
