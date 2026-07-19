package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectFindsNearest(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(root, ".clau.toml")
	if err := os.WriteFile(outer, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != outer {
		t.Errorf("discoverProject = %q, want %q", got, outer)
	}
	inner := filepath.Join(root, "a", ".clau.toml")
	if err := os.WriteFile(inner, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != inner {
		t.Errorf("nearest must win: got %q, want %q", got, inner)
	}
}

func TestDiscoverProjectNone(t *testing.T) {
	if got := discoverProject(t.TempDir()); got != "" {
		t.Errorf("discoverProject = %q, want empty", got)
	}
}

func TestDiscoverProjectSkipsDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(nested, ".clau.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, ".clau.toml")
	if err := os.WriteFile(real, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != real {
		t.Errorf("a directory named .clau.toml must be skipped: got %q, want %q", got, real)
	}
}
