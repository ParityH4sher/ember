package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var body projects.Project
	if err := decodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.ID) == "" || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id and name are required")
		return
	}
	if err := s.projects.Create(body); err != nil {
		if errors.Is(err, fs.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "invalid_request", "use a project ID of 1–64 letters, digits, underscores or hyphens, starting with a letter or digit")
			return
		}
		if errors.Is(err, projects.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "project_exists", "project id already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create project")
		return
	}
	writeJSON(w, http.StatusCreated, body)
}

func (s *Server) listProjects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"projects": s.projects.List()})
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	err := s.projects.Delete(r.PathValue("projectId"))
	if errors.Is(err, projects.ErrNotFound) {
		writeError(w, http.StatusNotFound, "project_not_found", "project does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not delete project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
