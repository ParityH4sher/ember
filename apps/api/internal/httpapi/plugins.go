package httpapi

import (
	"net/http"
	"sort"
)

func (s *Server) listPlugins(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.plugins))
	for id := range s.plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	plugins := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		plugins = append(plugins, s.pluginResponse(s.plugins[id]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
}

func (s *Server) getPlugin(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	writeJSON(w, http.StatusOK, s.pluginResponse(p))
}

func (s *Server) getPluginConfiguration(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"values":              publicValues(p),
		"configurationSchema": p.definition.ConfigurationSchema,
	})
}

func (s *Server) setPluginConfiguration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values map[string]any `json:"values"`
	}
	if err := decodeJSON(w, r, &body); err != nil || body.Values == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "body must contain an object field named values")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	for key, property := range properties(p.definition.ConfigurationSchema) {
		if isWriteOnly(property) {
			if _, supplied := body.Values[key]; !supplied {
				if existing, ok := p.values[key]; ok {
					body.Values[key] = existing
				}
			}
		}
	}
	if err := validateSchema(p.definition.ConfigurationSchema, body.Values); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_configuration", err.Error())
		return
	}
	p.values = body.Values
	p.configured = true
	writeJSON(w, http.StatusOK, map[string]any{
		"values":              publicValues(p),
		"configurationSchema": p.definition.ConfigurationSchema,
	})
}

func (s *Server) setPluginActivation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := decodeJSON(w, r, &body); err != nil || body.Active == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "body must contain a boolean field named active")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	if *body.Active && !p.configured {
		writeError(w, http.StatusUnprocessableEntity, "plugin_not_configured", "plugin must be configured before activation")
		return
	}
	p.active = *body.Active
	writeJSON(w, http.StatusOK, s.pluginResponse(p))
}

func (s *Server) pluginResponse(p *pluginState) map[string]any {
	return map[string]any{
		"id":                  p.definition.ID,
		"name":                p.definition.Name,
		"version":             p.definition.Version,
		"apiVersion":          p.definition.APIVersion,
		"description":         p.definition.Description,
		"active":              p.active,
		"configured":          p.configured,
		"capabilities":        p.definition.Capabilities,
		"configurationSchema": p.definition.ConfigurationSchema,
		"configuration":       publicValues(p),
	}
}

func publicValues(p *pluginState) map[string]any {
	values := make(map[string]any)
	for key, value := range p.values {
		if !isWriteOnly(properties(p.definition.ConfigurationSchema)[key]) {
			values[key] = value
		}
	}
	return values
}
