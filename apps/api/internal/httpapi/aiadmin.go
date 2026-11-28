package httpapi

import (
	"errors"
	"net/http"

	"github.com/ParityH4sher/ember/apps/api/internal/aicatalog"
)

// WorkerInfo is a harness worker from the operator's agents file, as the admin
// page shows it: what a session agent may name, without its URL or password.
type WorkerInfo struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
}

// aiAdmin serves the admin routes for models and agents (ADR 015). Every
// route requires the admin role. Providers and their keys are configured in
// the model gateway, never here.
type aiAdmin struct {
	catalog  *aicatalog.Store
	gateway  *aicatalog.Gateway // nil when no gateway is configured
	workers  []WorkerInfo
	chats    *chatAPI
	operator func() []agentInfo // agents from the operator's agents file
}

func (a *aiAdmin) get(w http.ResponseWriter, _ *http.Request) {
	view := a.catalog.View()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"persisted":      view.Persisted,
		"chatEnabled":    a.chats.runs != nil,
		"harnesses":      aicatalog.Harnesses,
		"workers":        append([]WorkerInfo{}, a.workers...),
		"gateway":        map[string]bool{"configured": a.gateway != nil},
		"operatorAgents": a.operator(),
		"models":         view.Models,
		"agents":         view.Agents,
	})
}

// gatewayModels lists the gateway's model names so an admin can offer them.
// Failures are 503: the gateway is a dependency, and the message is fixed
// text that never includes the gateway's own answer.
func (a *aiAdmin) gatewayModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.gateway == nil {
		writeError(w, http.StatusServiceUnavailable, "gateway_unavailable",
			"no model gateway is configured; set EMBER_MODEL_GATEWAY_URL and EMBER_MODEL_GATEWAY_KEY_FILE")
		return
	}
	ids, err := a.gateway.Models(r.Context())
	if err != nil {
		msg := "the model gateway's models could not be listed"
		var g *aicatalog.GatewayError
		if errors.As(err, &g) {
			msg = g.Msg
		}
		writeError(w, http.StatusServiceUnavailable, "gateway_unavailable", msg)
		return
	}
	type model struct {
		ID string `json:"id"`
	}
	models := make([]model, 0, len(ids))
	for _, id := range ids {
		models = append(models, model{ID: id})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (a *aiAdmin) putModel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "send the model's display name")
		return
	}
	m, err := a.catalog.PutModel(r.PathValue("id"), aicatalog.Model{Name: in.Name})
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
		Worker       string   `json:"worker"`
		Models       []string `json:"models"`
		DefaultModel string   `json:"defaultModel"`
		Enabled      bool     `json:"enabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "send name, harness, routerAgent or worker, models, defaultModel and enabled")
		return
	}
	ag, err := a.catalog.PutAgent(r.PathValue("id"), aicatalog.Agent{
		Name: in.Name, Harness: in.Harness, RouterAgent: in.RouterAgent, Worker: in.Worker,
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
