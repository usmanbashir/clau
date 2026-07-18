package main

import (
	"reflect"
	"testing"
)

func TestVersionString(t *testing.T) {
	old := version
	defer func() { version = old }()
	version = "1.2.3"
	if got := versionString(); got != "1.2.3" {
		t.Errorf("stamped: %q, want 1.2.3", got)
	}
	version = "dev"
	// Test binaries carry no module version ("" or "(devel)"), so the
	// fallback must yield "dev" here; go-install builds get Main.Version.
	if got := versionString(); got != "dev" {
		t.Errorf("unstamped: %q, want dev", got)
	}
}

func TestInvocationName(t *testing.T) {
	cases := map[string]string{
		"/usr/local/bin/clau": "clau",
		`C:\bin\clau.exe`:     "clau",
		"co5.cmd":             "co5",
		"./c":                 "c",
		"crev":                "crev",
	}
	for in, want := range cases {
		if got := invocationName(in); got != want {
			t.Errorf("invocationName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDispatch(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want action
	}{
		{"clau", []string{"link", "--force"}, action{kind: "management", verb: "link", args: []string{"--force"}}},
		{"clau", []string{"run", "o5", "hi"}, action{kind: "management", verb: "run", args: []string{"o5", "hi"}}},
		{"clau", []string{"__launch", "hi"}, action{kind: "management", verb: "__launch", args: []string{"hi"}}},
		{"clau", []string{"o5", "hi"}, action{kind: "launch", args: []string{"o5", "hi"}}},
		{"clau", []string{}, action{kind: "launch", args: []string{}}},
		{"clau", []string{"--help"}, action{kind: "management", verb: "help", args: []string{}}},
		{"clau", []string{"--version"}, action{kind: "management", verb: "version", args: []string{}}},
		{"c", []string{"-c"}, action{kind: "launch", args: []string{"-c"}}},
		{"c", []string{}, action{kind: "launch", args: []string{}}},
		{"co5", []string{"x"}, action{kind: "named", token: "o5", args: []string{"x"}}},
		{"crev", []string{}, action{kind: "named", token: "rev", args: []string{}}},
		{"weird", []string{}, action{kind: "badname"}},
	}
	for _, c := range cases {
		t.Run(c.name+"/"+joinArgs(c.args), func(t *testing.T) {
			got := dispatch(c.name, c.args)
			if got.kind != c.want.kind || got.verb != c.want.verb || got.token != c.want.token {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			if c.want.args != nil && !reflect.DeepEqual(got.args, c.want.args) {
				t.Errorf("args = %v, want %v", got.args, c.want.args)
			}
		})
	}
}

func joinArgs(a []string) string {
	out := ""
	for _, s := range a {
		out += s + ","
	}
	return out
}
