package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestCompletionScript(t *testing.T) {
	for _, shell := range []string{"fish", "zsh", "bash"} {
		s, err := completionScript(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.Contains(s, "clau list --tokens") {
			t.Errorf("%s script does not query tokens live:\n%s", shell, s)
		}
	}
	if _, err := completionScript("powershell"); err == nil {
		t.Error("unknown shell must error")
	}
}

func TestCompletionsIncludeTrustVerbs(t *testing.T) {
	for _, shell := range []string{"fish", "zsh", "bash"} {
		s, err := completionScript(shell)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, "trust untrust") {
			t.Errorf("%s completions missing trust verbs", shell)
		}
	}
}

// TestCompletionsPlaceShow checks show is a first-word verb and that
// its second word completes tokens like run's does, per shell.
func TestCompletionsPlaceShow(t *testing.T) {
	cases := []struct {
		shell  string
		verbs  *regexp.Regexp // captures the literal verb list
		branch string         // the run/show token-completion guard
	}{
		{"fish", regexp.MustCompile(`-a '(link [^']*)'`), "contains -- (commandline -opc)[2] run show;"},
		{"bash", regexp.MustCompile(`compgen -W "(link [^"$]*)`), `[ "${COMP_WORDS[1]}" = show ]`},
		{"zsh", regexp.MustCompile(`items=\((link [^)]*)\)`), "${words[2]} == (run|show)"},
	}
	for _, c := range cases {
		s, err := completionScript(c.shell)
		if err != nil {
			t.Fatal(err)
		}
		m := c.verbs.FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("%s: verb list not found:\n%s", c.shell, s)
		}
		inVerbs := false
		for _, v := range strings.Fields(m[1]) {
			if v == "show" {
				inVerbs = true
			}
		}
		if !inVerbs {
			t.Errorf("%s: show missing from the verb list %q", c.shell, m[1])
		}
		if !strings.Contains(s, c.branch) {
			t.Errorf("%s: show missing from the token-completion branch (want %q):\n%s", c.shell, c.branch, s)
		}
	}
}
