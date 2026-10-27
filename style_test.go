package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewPalette(t *testing.T) {
	if p := newPalette(false); p != (palette{}) {
		t.Errorf("disabled palette must be all-empty: %+v", p)
	}
	p := newPalette(true)
	if p.cyan != "\033[1;36m" || p.reset != "\033[0m" || p.dim != "\033[2m" ||
		p.bold != "\033[1m" || p.green != "\033[32m" || p.yellow != "\033[33m" {
		t.Errorf("palette codes wrong: %+v", p)
	}
}

func TestColorChoice(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		name string
		tty  bool
		goos string
		env  map[string]string
		want bool
	}{
		{"tty linux", true, "linux", nil, true},
		{"piped", false, "linux", nil, false},
		{"NO_COLOR", true, "linux", map[string]string{"NO_COLOR": "1"}, false},
		{"dumb term", true, "linux", map[string]string{"TERM": "dumb"}, false},
		{"windows bare conhost", true, "windows", nil, false},
		{"windows terminal", true, "windows", map[string]string{"WT_SESSION": "x"}, true},
		{"windows vscode", true, "windows", map[string]string{"TERM_PROGRAM": "vscode"}, true},
		{"windows with TERM", true, "windows", map[string]string{"TERM": "xterm"}, true},
	}
	for _, c := range cases {
		if got := colorChoice(c.tty, c.goos, env(c.env)); got != c.want {
			t.Errorf("%s: colorChoice = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVisibleLenAndPad(t *testing.T) {
	pal := newPalette(true)
	s := pal.cyan + "qw35f" + pal.reset
	if got := visibleLen(s); got != 5 {
		t.Errorf("visibleLen = %d, want 5", got)
	}
	if got := padCell(s, 8); visibleLen(got) != 8 || got != s+"   " {
		t.Errorf("padCell = %q", got)
	}
	if got := padCell("abc", 2); got != "abc" {
		t.Errorf("padCell must never truncate: %q", got)
	}
}

func TestContractHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := contractHome(filepath.Join(home, ".local", "bin")); got != filepath.Join("~", ".local", "bin") {
		t.Errorf("contractHome = %q", got)
	}
	if got := contractHome(home); got != "~" {
		t.Errorf("home itself = %q, want ~", got)
	}
	if got := contractHome("/opt/x"); got != "/opt/x" {
		t.Errorf("non-home path must pass through: %q", got)
	}
}

// TestContractHomeThroughSymlink covers a home reached via a different
// symlinked path, in both directions. On macOS t.TempDir() already sits
// under the /var → /private/var link, so even the raw strings differ.
func TestContractHomeThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges and UserHomeDir reads USERPROFILE")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("~", ".local", "bin")

	t.Setenv("HOME", home)
	if got := contractHome(filepath.Join(alias, ".local", "bin")); got != want {
		t.Errorf("path via alias, HOME real: %q, want %q", got, want)
	}
	if got := contractHome(filepath.Join(alias, ".local", "bin") + string(filepath.Separator)); got != want {
		t.Errorf("trailing separator via alias: %q, want %q", got, want)
	}
	// The leaf is the thing being named: a link file must not turn into
	// its target (co5 → ~/bin/clau) when the parent is resolved.
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "clau"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "bin", "clau"), filepath.Join(bin, "co5")); err != nil {
		t.Fatal(err)
	}
	if got, want := contractHome(filepath.Join(alias, ".local", "bin", "co5")), filepath.Join(want, "co5"); got != want {
		t.Errorf("link file via alias: %q, want %q", got, want)
	}
	t.Setenv("HOME", alias)
	if got := contractHome(bin); got != want {
		t.Errorf("real path, HOME via alias: %q, want %q", got, want)
	}
	if got := contractHome(alias); got != "~" {
		t.Errorf("alias itself with HOME via alias: %q, want ~", got)
	}
	// Outside home stays untouched even when it does not exist.
	if p := filepath.Join(t.TempDir(), "nope"); contractHome(p) != p {
		t.Errorf("non-home path must pass through: %q", contractHome(p))
	}
}
