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

func TestTrustPathXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join("/tmp", "state"))
	want := filepath.Join("/tmp", "state", "clau", "trust.toml")
	if got := trustPath(); got != want {
		t.Errorf("trustPath = %q, want %q", got, want)
	}
}

func TestTrustRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "deep", "trust.toml")
	store := map[string]string{"/proj/.clau.toml": "abc123"}
	if err := saveTrust(p, store); err != nil {
		t.Fatal(err)
	}
	got, corrupt := loadTrust(p)
	if corrupt {
		t.Fatal("round-tripped store reported corrupt")
	}
	if got["/proj/.clau.toml"] != "abc123" {
		t.Errorf("store = %v", got)
	}
}

func TestLoadTrustMissingIsEmpty(t *testing.T) {
	store, corrupt := loadTrust(filepath.Join(t.TempDir(), "none.toml"))
	if corrupt || len(store) != 0 {
		t.Errorf("missing store: got %v corrupt=%v, want empty/false", store, corrupt)
	}
}

func TestLoadTrustCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trust.toml")
	if err := os.WriteFile(p, []byte("[trusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, corrupt := loadTrust(p)
	if !corrupt || len(store) != 0 {
		t.Errorf("corrupt store: got %v corrupt=%v, want empty/true", store, corrupt)
	}
}

func TestHashFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	got, err := hashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("hashFile = %q, want %q", got, want)
	}
}
