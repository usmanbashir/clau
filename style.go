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

// stdoutIsTTY uses the char-device heuristic deliberately: it also says
// yes for /dev/null, but nobody can see the escapes sent there, and the
// alternative is a golang.org/x/term dependency for no visible effect.
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
// The raw strings are compared first, then the path's parent and home
// with symlinks resolved, so a home reached through a different
// symlinked path still contracts. Only the parent is resolved: the leaf
// is the thing being named, and a link file (co5 → clau) must not turn
// into its target. A parent that does not exist yet (the link dir
// before the first clau link) cannot be resolved and keeps the raw
// comparison.
func contractHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, ok := tildeRel(p, home); ok {
		return rel
	}
	clean := filepath.Clean(p)
	rd, errD := filepath.EvalSymlinks(filepath.Dir(clean))
	rh, errH := filepath.EvalSymlinks(home)
	if errD == nil && errH == nil {
		if rel, ok := tildeRel(filepath.Join(rd, filepath.Base(clean)), rh); ok {
			return rel
		}
	}
	return p
}

// tildeRel rewrites p as ~ or ~/... when it is home or inside it.
func tildeRel(p, home string) (string, bool) {
	if p == home {
		return "~", true
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):], true
	}
	return "", false
}
