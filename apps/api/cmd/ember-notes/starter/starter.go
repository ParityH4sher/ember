// Package starter seeds the same Markdown vault for the API and local CLI.
package starter

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed *.md
var templates embed.FS

// Ensure fills missing starter files in a trusted project, preserving edits.
func Ensure(project string) error {
	dir := filepath.Join(project, "notes")
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fs.ErrInvalid
	}
	for _, name := range []string{"index.md", "handoff.md"} {
		path := filepath.Join(dir, name)
		body, err := templates.ReadFile(name)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, fs.ErrExist) {
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				return fs.ErrInvalid
			}
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.Write(body)
		if err := errors.Join(err, f.Close()); err != nil {
			os.Remove(path)
			return err
		}
	}
	return nil
}
