package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// palette holds the ANSI escapes for the human-facing views. Every
// field is empty when color is off, so call sites format one way and
// degrade to plain text with zero branching.
type palette struct {
	bold, dim, cyan, green, yellow, reset string
}

func newPalette(enabled bool) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		bold:   "\033[1m",
		dim:    "\033[2m",
		cyan:   "\033[1;36m",
		green:  "\033[32m",
		yellow: "\033[33m",
		reset:  "\033[0m",
	}
}

// colorChoice decides whether to emit ANSI codes. Pure so tests cover
// every branch; callers pass the live TTY state and os.Getenv.
func colorChoice(isTTY bool, goos string, getenv func(string) string) bool {
	if !isTTY || getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return false
	}
	if goos == "windows" {
		// Classic conhost needs VT enabled per process; color only where
		// an ANSI-capable host announces itself.
		return getenv("WT_SESSION") != "" || getenv("TERM_PROGRAM") == "vscode" || getenv("TERM") != ""
	}
	return true
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

var ansiRe = regexp.MustCompile("\033\\[[0-9;]*m")

// visibleLen is the on-screen rune width of s, ignoring ANSI escapes.
func visibleLen(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// padCell right-pads s with spaces to visible width w (never truncates).
func padCell(s string, w int) string {
	if n := w - visibleLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// contractHome shortens a path under the user's home to ~/... for display.
func contractHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
