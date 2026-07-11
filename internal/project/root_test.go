package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	got := Resolve(sub)
	if got != root {
		t.Fatalf("got %q want %q", got, root)
	}
}

func TestResolveNoGit(t *testing.T) {
	dir := t.TempDir()
	got := Resolve(dir)
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Fatalf("got %q want %q", got, abs)
	}
}
