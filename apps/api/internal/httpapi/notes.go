package httpapi

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/ParityH4sher/ember/apps/api/internal/projects"
)

func listNotes(v *projects.Vaults) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notes, err := v.Notes(r.PathValue("projectId"))
		if err != nil {
			noteError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"notes": notes})
	}
}

func saveNote(v *projects.Vaults) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var change struct {
			Path     string  `json:"path"`
			Content  *string `json:"content"`
			Previous *string `json:"previous"`
		}
		if err := decodeJSON(w, r, &change); err != nil || change.Content == nil {
			noteError(w, fs.ErrInvalid)
			return
		}
		if err := v.Save(r.PathValue("projectId"), change.Path, *change.Content, change.Previous); err != nil {
			noteError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func noteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, projects.ErrNotFound):
		writeError(w, 404, "project_not_found", "project does not exist")
	case errors.Is(err, fs.ErrInvalid):
		writeError(w, 400, "invalid_note", "use a relative Markdown path and text content")
	case errors.Is(err, projects.ErrLimit):
		writeError(w, 413, "vault_limit", "note or project exceeds the vault limit")
	case errors.Is(err, projects.ErrConflict):
		writeError(w, 409, "note_conflict", err.Error())
	default:
		writeError(w, 500, "storage_error", "could not access project notes")
	}
}
