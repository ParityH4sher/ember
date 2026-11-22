package projects

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultRestartAndRetainedNotes(t *testing.T) {
	root := t.TempDir()
	open := func() *Vaults {
		t.Helper()
		v, err := OpenVaults(root)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := open()
	p := Project{ID: "desk", Name: "Desk"}
	if err := v.Create(p); err != nil {
		t.Fatal(err)
	}
	notes, err := v.Notes(p.ID)
	if err != nil || notes["index.md"] == "" || notes["handoff.md"] == "" {
		t.Fatalf("missing starter notes: %v", err)
	}
	const path, content = "handoff.md", "See [[materials]]."
	old := notes[path]
	if err := v.Save(p.ID, path, content, &old); err != nil {
		t.Fatal(err)
	}
	v = open()
	if err := v.Save(p.ID, path, "stale", &old); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if err := v.Save(p.ID, "../other.md", "outside", nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("unexpected path handling: %v", err)
	}
	if err := v.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	v = open()
	if _, err := v.Notes(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed vault remains visible: %v", err)
	}
	if err := v.Create(p); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("retained vault id was not reserved: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, p.ID, "notes", path))
	if err != nil || string(got) != content {
		t.Fatalf("retained note = %q, %v", got, err)
	}
}
