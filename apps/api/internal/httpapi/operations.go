package httpapi

import "net/http"

func (s *Server) listPluginOperations(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	operations := convertOperations(p.definition.Operations)
	writeJSON(w, http.StatusOK, map[string]any{
		"pluginVersion": p.definition.Version,
		"apiVersion":    p.definition.APIVersion,
		"operations":    operations,
	})
}

func (s *Server) invokePluginOperation(w http.ResponseWriter, r *http.Request) {
	var request struct {
		PathParams map[string]any `json:"pathParams"`
		Query      map[string]any `json:"query"`
		Body       map[string]any `json:"body"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}
	s.mu.RLock()
	p := s.plugins[r.PathValue("pluginId")]
	if p == nil {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "plugin_not_found", "plugin is not available")
		return
	}
	version := r.PathValue("apiVersion")
	if version != p.definition.APIVersion && version != "v"+p.definition.APIVersion {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "plugin_api_version_not_found", "plugin API version is not available")
		return
	}
	if !p.active || !p.configured {
		s.mu.RUnlock()
		writeError(w, http.StatusConflict, "plugin_inactive", "plugin must be active and configured")
		return
	}
	var selected *operation
	operations := convertOperations(p.definition.Operations)
	for i := range operations {
		if operations[i].OperationID == r.PathValue("operationId") {
			selected = &operations[i]
			break
		}
	}
	if selected == nil {
		s.mu.RUnlock()
		writeError(w, http.StatusNotFound, "operation_not_found", "plugin operation is not available")
		return
	}
	input := map[string]any{
		"pathParams": nonNilMap(request.PathParams),
		"query":      nonNilMap(request.Query),
		"body":       nonNilMap(request.Body),
	}
	if err := validateSchema(selected.InputSchema, input); err != nil {
		s.mu.RUnlock()
		writeError(w, http.StatusUnprocessableEntity, "invalid_operation_input", err.Error())
		return
	}
	adapter := p.adapter
	configuration := p.values
	operationID := selected.OperationID
	s.mu.RUnlock()
	if adapter == nil {
		writeError(w, http.StatusNotImplemented, "operation_unavailable", "plugin operation is declared but has no runtime adapter")
		return
	}
	result, err := adapter.Invoke(r.Context(), operationID, configuration, input)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "plugin service request failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
