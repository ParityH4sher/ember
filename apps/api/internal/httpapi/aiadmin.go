package httpapi

import (
	"errors"
	"net/http"

	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
)

// aiAdmin serves the admin routes for model providers, models and agents.
// Every route requires the admin role; provider keys are accepted but never
// returned.
type aiAdmin struct {
	catalog  *aicatalog.Store
	client   *http.Client
	chats    *chatAPI
	operator func() []agentInfo // agents from the operator's agents file
}

func (a *aiAdmin) get(w http.ResponseWriter, _ *http.Request) {
	view := a.catalog.View()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"persisted":      view.Persisted,
		"chatEnabled":    a.chats.runs != nil,
		"providerKinds":  aicatalog.Kinds,
		"harnesses":      aicatalog.Harnesses,
		"operatorAgents": a.operator(),
		"providers":      view.Providers,
		"models":         view.Models,
		"agents":         view.Agents,
	})
}

func (a *aiAdmin) putProvider(w http.ResponseWriter, r *http.Request) {
	var in aicatalog.ProviderInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "send name, kind, baseURL, local and an optional apiKey")
		return
	}
	p, err := a.catalog.PutProvider(r.PathValue("id"), in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *aiAdmin) deleteProvider(w http.ResponseWriter, r *http.Request) {
	if err := a.catalog.DeleteProvider(r.PathValue("id")); err != nil {
		catalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *aiAdmin) discover(w http.ResponseWriter, r *http.Request) {
	p, ok := a.catalog.Provider(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no provider with this ID")
		return
	}
	ids, err := aicatalog.Discover(r.Context(), a.client, p)
	if err != nil {
		var d *aicatalog.DiscoverError
		if errors.As(err, &d) {
			writeError(w, http.StatusBadGateway, "discovery_failed", d.Msg)
			return
		}
		writeError(w, http.StatusBadGateway, "discovery_failed", "the provider's models could not be listed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": ids})
}

func (a *aiAdmin) putModel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Upstream string `json:"upstream"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "send name, provider and upstream")
		return
	}
	m, err := a.catalog.PutModel(r.PathValue("id"), aicatalog.Model{Name: in.Name, Provider: in.Provider, Upstream: in.Upstream})
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *aiAdmin) deleteModel(w http.ResponseWriter, r *http.Request) {
	if err := a.catalog.DeleteModel(r.PathValue("id")); err != nil {
		catalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *aiAdmin) putAgent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name         string   `json:"name"`
		Harness      string   `json:"harness"`
		RouterAgent  string   `json:"routerAgent"`
		Models       []string `json:"models"`
		DefaultModel string   `json:"defaultModel"`
		Enabled      bool     `json:"enabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "send name, harness, routerAgent, models, defaultModel and enabled")
		return
	}
	ag, err := a.catalog.PutAgent(r.PathValue("id"), aicatalog.Agent{
		Name: in.Name, Harness: in.Harness, RouterAgent: in.RouterAgent,
		Models: in.Models, DefaultModel: in.DefaultModel, Enabled: in.Enabled,
	})
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ag)
}

func (a *aiAdmin) deleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := a.catalog.DeleteAgent(r.PathValue("id")); err != nil {
		catalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func catalogError(w http.ResponseWriter, err error) {
	var invalid *aicatalog.ValidationError
	var inUse *aicatalog.InUseError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_configuration", invalid.Msg)
	case errors.As(err, &inUse):
		writeError(w, http.StatusConflict, "in_use", inUse.Msg)
	case errors.Is(err, aicatalog.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "nothing with this ID is configured")
	default:
		writeError(w, http.StatusInternalServerError, "catalog_unavailable", "the AI catalog could not be saved")
	}
}
