package projectfiles

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReopenAfterInterruptedReplacement(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	const name = "artifacts/part.step"
	if err := s.Write(name, strings.NewReader("first export")); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("interrupted")
	src := io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(interrupted))
	if err := s.Write(name, src); !errors.Is(err, interrupted) {
		t.Fatalf("write error: %v", err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	f, err := reopened.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "first export" {
		t.Fatalf("reopened file: %q, %v", got, err)
	}
	files, err := os.ReadDir(filepath.Join(root, "artifacts"))
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected files after interruption: %v, %v", files, err)
	}
}

func TestRejectNonProjectPaths(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for link, dest := range map[string]string{"leaf": target, "dir": outside} {
		if err := os.Symlink(dest, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"", ".", "../target", target, "C:/target", `..\target`, ".ember-stage-private", "leaf", "dir/target"} {
		if err := s.Write(path, strings.NewReader("changed")); err == nil {
			t.Errorf("write accepted %q", path)
		}
		if f, err := s.Read(path); err == nil {
			f.Close()
			t.Errorf("read accepted %q", path)
		}
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "untouched" {
		t.Fatalf("outside file: %q, %v", got, err)
	}
}
