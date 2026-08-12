package main

import (
	"os"
	"path/filepath"
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
	if got := contractHome("/opt/x"); got != "/opt/x" {
		t.Errorf("non-home path must pass through: %q", got)
	}
}
