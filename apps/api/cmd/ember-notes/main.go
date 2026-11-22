// ember-notes is an operator-side handoff for one project's Markdown vault.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ParityH4sher/ember/apps/api/cmd/ember-notes/starter"
	"github.com/ParityH4sher/ember/apps/api/internal/projectfiles"
)

const usage = "usage: ember-notes init PROJECT | stage PROJECT NEW_WORKSPACE [NOTE.md ...] | publish PROJECT STOPPED_WORKSPACE NOTE.md ..."

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New(usage)
	}
	abs, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	project, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	vault := filepath.Join(project, "notes")
	if args[0] == "init" && len(args) == 2 {
		return starter.Ensure(project)
	}
	if len(args) < 3 || !(args[0] == "stage" || args[0] == "publish" && len(args) > 3) {
		return errors.New(usage)
	}
	workspace, err := filepath.Abs(args[2])
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(workspace))
	if err != nil {
		return err
	}
	workspace = filepath.Join(parent, filepath.Base(workspace))
	if within(project, workspace) || within(workspace, project) {
		return errors.New("use a task workspace separate from the project")
	}
	names := args[3:]
	if args[0] == "stage" {
		names = append([]string{"index.md", "handoff.md"}, names...)
	}
	for _, name := range names {
		if !fs.ValidPath(name) || !strings.HasSuffix(name, ".md") || strings.ContainsAny(name, `\:`) {
			return fmt.Errorf("invalid relative Markdown path %q", name)
		}
		for _, part := range strings.Split(name, "/") {
			if strings.HasPrefix(part, ".") {
				return fmt.Errorf("hidden note path %q", name)
			}
		}
	}
	if args[0] == "stage" {
		// Only a new directory is eligible to become the task's writable copy.
		if err := starter.Ensure(project); err != nil {
			return err
		}
		if err := os.Mkdir(workspace, 0700); err != nil {
			return err
		}
	}
	notes, err := projectfiles.Open(vault)
	if err != nil {
		return err
	}
	work, err := projectfiles.Open(workspace)
	if err != nil {
		return err
	}
	src, dst := notes, work
	if args[0] == "publish" {
		src, dst = work, notes
	}
	for _, name := range names {
		if err := copyNote(dst, src, name); err != nil {
			return fmt.Errorf("%s %s: %w", args[0], name, err)
		}
	}
	return nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyNote(dst, src *projectfiles.Store, name string) error {
	f, err := src.Read(name)
	if err != nil {
		return err
	}
	defer f.Close()
	const max = 256 << 10
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return err
	}
	if len(data) > max || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return errors.New("expected UTF-8 Markdown up to 256 KiB")
	}
	return dst.Write(name, bytes.NewReader(data))
}
