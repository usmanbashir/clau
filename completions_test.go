package main

import (
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
