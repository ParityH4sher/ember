// Package projectfiles stores portable project files in an instance-owned directory.
// Only the store may mutate its tree; worker-writable directories must be staged
// separately. A Store is scoped to one project, not the instance data root.
package projectfiles

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Store struct{ root string }

// Open uses an existing project directory selected by the trusted caller.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fs.ErrInvalid
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := checkDir(root); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func checkDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &fs.PathError{Op: "check directory", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}

// resolve walks canonical manifest paths without accepting symlinks, including
// ones pointing elsewhere within this project. Writes may create directories.
func (s *Store) resolve(name string, writing bool) (string, error) {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\:`) {
		return "", fs.ErrInvalid
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".ember-stage-") {
			return "", fs.ErrInvalid
		}
	}
	dir := s.root
	for _, part := range parts[:len(parts)-1] {
		dir = filepath.Join(dir, part)
		err := checkDir(dir)
		if writing && errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
				return "", err
			}
			err = checkDir(dir)
		}
		if err != nil {
			return "", err
		}
	}
	target := filepath.Join(dir, parts[len(parts)-1])
	info, err := os.Lstat(target)
	if writing && errors.Is(err, fs.ErrNotExist) {
		return target, nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", &fs.PathError{Op: "check file", Path: target, Err: fs.ErrInvalid}
	}
	return target, nil
}

// Read returns the complete file revision visible when opened. The caller closes it.
func (s *Store) Read(name string) (*os.File, error) {
	path, err := s.resolve(name, false)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Write streams a file into place. On the supported local Linux filesystem,
// rename publishes a complete revision even when replacing an existing file.
func (s *Store) Write(name string, src io.Reader) error {
	path, err := s.resolve(name, true)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ember-stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, copyErr := io.Copy(f, src)
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
