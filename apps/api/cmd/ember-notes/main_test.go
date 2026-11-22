package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeFromPublishedHandoff(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	first, next := filepath.Join(root, "first"), filepath.Join(root, "next")
	must := func(args ...string) {
		t.Helper()
		if err := run(args); err != nil {
			t.Fatal(err)
		}
	}
	must("stage", project, first)
	const handoff = "Next: check [material choice](material.md).\n"
	for name, text := range map[string]string{"handoff.md": handoff, "material.md": "Use birch.", "scratch.md": "discard"} {
		if err := os.WriteFile(filepath.Join(first, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	must("publish", project, first, "material.md", "handoff.md")
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	must("stage", project, next, "material.md")
	for name, want := range map[string]string{"index.md": "(handoff.md)", "handoff.md": handoff, "material.md": "Use birch."} {
		data, err := os.ReadFile(filepath.Join(next, name))
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "notes", "scratch.md")); !os.IsNotExist(err) {
		t.Fatal("unselected scratch note was published")
	}
	if err := run([]string{"stage", project, filepath.Join(project, "work")}); err == nil {
		t.Fatal("accepted workspace inside durable project")
	}
	if err := os.Symlink(filepath.Join(project, "notes", "index.md"), filepath.Join(next, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"publish", project, next, "link.md"}); err == nil {
		t.Fatal("published symlink")
	}
}
