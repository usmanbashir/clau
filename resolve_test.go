package main

import (
	"reflect"
	"strings"
	"testing"
)

func testConfig() Config {
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{
		Model: "opus", Effort: "high",
		Flags: []string{"--append-system-prompt", "Review code."},
	}
	cfg.Profiles["work"] = Profile{
		Env:    map[string]string{"ANTHROPIC_BASE_URL": "https://x"},
		Claude: "claude-alt",
	}
	cfg.Profiles["o5"] = Profile{Model: "shadow"} // profile shadows grammar
	return cfg
}

func TestResolveToken(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		token     string
		wantFlags []string
		wantFound bool
		wantErr   string
	}{
		{"o", []string{"--model", "opus"}, true, ""},
		{"s3", []string{"--model", "sonnet", "--effort", "high"}, true, ""},
		{"f5", []string{"--model", "fable", "--effort", "max"}, true, ""},
		{"h", []string{"--model", "haiku"}, true, ""},
		{"h3", nil, true, "no effort levels"},
		{"o5", []string{"--model", "shadow"}, true, ""}, // profile wins
		{"rev", []string{"--model", "opus", "--effort", "high", "--append-system-prompt", "Review code."}, true, ""},
		{"x", nil, false, ""},
		{"x5", nil, false, ""},
		{"o9", nil, false, ""}, // 9 not in ladder, "o9" not a model key
		{"", nil, false, ""},
		{"-c", nil, false, ""},
		{"fix this bug", nil, false, ""},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			res, found, err := resolveToken(cfg, c.token)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v", found, c.wantFound)
			}
			if found && !reflect.DeepEqual(res.Flags, c.wantFlags) {
				t.Errorf("flags = %v, want %v", res.Flags, c.wantFlags)
			}
		})
	}
}

func TestResolveTokenProfileCarriesEnvAndClaude(t *testing.T) {
	res, found, err := resolveToken(testConfig(), "work")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if res.Env["ANTHROPIC_BASE_URL"] != "https://x" || res.Claude != "claude-alt" {
		t.Errorf("res = %+v", res)
	}
	if len(res.Flags) != 0 {
		t.Errorf("flags = %v, want none", res.Flags)
	}
}

func TestResolveTokenCustomLadder(t *testing.T) {
	cfg := defaultConfig()
	cfg.Efforts = map[string]string{"7": "turbo"}
	res, found, err := resolveToken(cfg, "o7")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	want := []string{"--model", "opus", "--effort", "turbo"}
	if !reflect.DeepEqual(res.Flags, want) {
		t.Errorf("flags = %v, want %v", res.Flags, want)
	}
}

func TestOverlayEnv(t *testing.T) {
	base := []string{"HOME=/home/u", "ANTHROPIC_BASE_URL=old", "PATH=/bin"}
	got := overlayEnv(base, map[string]string{
		"ANTHROPIC_BASE_URL": "https://x",
		"NEW_VAR":            "1",
	})
	want := map[string]bool{
		"HOME=/home/u": true, "ANTHROPIC_BASE_URL=https://x": true,
		"PATH=/bin": true, "NEW_VAR=1": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, kv := range got {
		if !want[kv] {
			t.Errorf("unexpected entry %q in %v", kv, got)
		}
	}
}

func TestOverlayEnvNilOverlayReturnsBase(t *testing.T) {
	base := []string{"A=1"}
	if got := overlayEnv(base, nil); !reflect.DeepEqual(got, base) {
		t.Errorf("got %v", got)
	}
}

func TestBuildLaunch(t *testing.T) {
	cfg := testConfig()
	res, _, _ := resolveToken(cfg, "s3")
	l := buildLaunch(cfg, res, []string{"hello", "-c"})
	if l.Target != "claude" {
		t.Errorf("target = %q", l.Target)
	}
	want := []string{"--model", "sonnet", "--effort", "high", "hello", "-c"}
	if !reflect.DeepEqual(l.Args, want) {
		t.Errorf("args = %v, want %v", l.Args, want)
	}
}

func TestBuildLaunchTargetPrecedence(t *testing.T) {
	cfg := testConfig()
	cfg.Claude = "ccr"
	if l := buildLaunch(cfg, TokenResolution{}, nil); l.Target != "ccr" {
		t.Errorf("global target = %q, want ccr", l.Target)
	}
	res, _, _ := resolveToken(cfg, "work")
	if l := buildLaunch(cfg, res, nil); l.Target != "claude-alt" {
		t.Errorf("profile target = %q, want claude-alt", l.Target)
	}
}

func TestBuildLaunchAppliesProfileEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "old")
	res, _, _ := resolveToken(testConfig(), "work")
	l := buildLaunch(testConfig(), res, nil)
	found := false
	for _, kv := range l.Env {
		if kv == "ANTHROPIC_BASE_URL=https://x" {
			found = true
		}
		if kv == "ANTHROPIC_BASE_URL=old" {
			t.Error("stale env entry survived overlay")
		}
	}
	if !found {
		t.Errorf("overlay missing from %d env entries", len(l.Env))
	}
}
