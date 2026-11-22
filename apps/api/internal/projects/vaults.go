package projects

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ParityH4sher/ember/apps/api/cmd/ember-notes/starter"
	"github.com/ParityH4sher/ember/apps/api/internal/projectfiles"
)

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var ErrConflict = errors.New("note has changed; reopen it before saving")
var ErrLimit = errors.New("notes exceed vault size limit")

// Vaults owns a private project root. Only active projects have a record file.
type Vaults struct {
	root string
	mu   sync.Mutex
	*MemoryStore
}

func OpenVaults(root string) (*Vaults, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if _, err := projectfiles.Open(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	v := &Vaults{root: root, MemoryStore: NewMemoryStore()}
	for _, e := range entries {
		if !e.IsDir() || !validID.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		store, err := projectfiles.Open(dir)
		if err != nil {
			return nil, err
		}
		f, err := store.Read("record.json")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var p Project
		err = json.NewDecoder(f).Decode(&p)
		f.Close()
		if err != nil || p.ID != e.Name() {
			return nil, fs.ErrInvalid
		}
		if err := starter.Ensure(dir); err != nil {
			return nil, err
		}
		if err := v.MemoryStore.Create(p); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (v *Vaults) Create(p Project) error {
	if !validID.MatchString(p.ID) {
		return fs.ErrInvalid
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	dir := filepath.Join(v.root, p.ID)
	if err := os.Mkdir(dir, 0700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrAlreadyExists
		}
		return err
	}
	if err := func() error {
		if err := starter.Ensure(dir); err != nil {
			return err
		}
		store, err := projectfiles.Open(dir)
		if err != nil {
			return err
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return store.Write("record.json", strings.NewReader(string(data)))
	}(); err != nil {
		os.RemoveAll(dir) // exclusively created above, never a preexisting project
		return err
	}
	return v.MemoryStore.Create(p)
}

func (v *Vaults) active(id string) bool {
	v.MemoryStore.mu.RLock()
	defer v.MemoryStore.mu.RUnlock()
	_, ok := v.projects[id]
	return ok
}

func (v *Vaults) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(id) {
		return ErrNotFound
	}
	if err := os.Remove(filepath.Join(v.root, id, "record.json")); err != nil {
		return err
	}
	return v.MemoryStore.Delete(id)
}

func notePath(path string) bool {
	if !fs.ValidPath(path) || !strings.HasSuffix(path, ".md") || strings.ContainsAny(path, "\\:\x00\r\n") {
		return false
	}
	for _, p := range strings.Split(path, "/") {
		if strings.HasPrefix(p, ".") {
			return false
		}
	}
	return true
}

const maxNote = 256 << 10

func readNote(store *projectfiles.Store, path string) (string, error) {
	f, err := store.Read(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxNote+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxNote || !utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0 {
		return "", ErrLimit
	}
	return string(b), nil
}

// Notes returns a bounded Markdown snapshot for the browser's link index.
func (v *Vaults) Notes(id string) (map[string]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(id) {
		return nil, ErrNotFound
	}
	return v.snapshot(id)
}

func (v *Vaults) snapshot(id string) (map[string]string, error) {
	dir := filepath.Join(v.root, id, "notes")
	store, err := projectfiles.Open(dir)
	if err != nil {
		return nil, err
	}
	notes := map[string]string{}
	total := 0
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if !d.Type().IsRegular() || !notePath(name) {
			return nil
		}
		if len(notes) >= 256 {
			return ErrLimit
		}
		text, err := readNote(store, name)
		if err != nil {
			return err
		}
		total += len(text)
		if total > 4<<20 {
			return ErrLimit
		}
		notes[name] = text
		return nil
	})
	return notes, err
}

func (v *Vaults) Save(id, path, text string, previous *string) error {
	if !notePath(path) || strings.IndexByte(text, 0) >= 0 {
		return fs.ErrInvalid
	}
	if len(text) > maxNote {
		return ErrLimit
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.active(id) {
		return ErrNotFound
	}
	notes, err := v.snapshot(id)
	if err != nil {
		return err
	}
	old, exists := notes[path]
	if exists != (previous != nil) || (exists && old != *previous) {
		return ErrConflict
	}
	if !exists && len(notes) >= 256 {
		return ErrLimit
	}
	total := len(text) - len(old)
	for _, body := range notes {
		total += len(body)
	}
	if total > 4<<20 {
		return ErrLimit
	}
	store, err := projectfiles.Open(filepath.Join(v.root, id, "notes"))
	if err != nil {
		return err
	}
	return store.Write(path, strings.NewReader(text))
}
