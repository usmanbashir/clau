package main

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestTokensOf(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{Model: "opus"}
	got := strings.Join(tokensOf(cfg), " ")
	want := "f f1 f2 f3 f4 f5 h o o1 o2 o3 o4 o5 rev s s1 s2 s3 s4 s5"
	if got != want {
		t.Errorf("tokens = %q\nwant     %q", got, want)
	}
}

func TestTokensOfDedupesShadowingProfile(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["o5"] = Profile{Model: "shadow"}
	count := 0
	for _, tok := range tokensOf(cfg) {
		if tok == "o5" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("o5 appears %d times, want 1", count)
	}
}

func TestDoctorShadowWarningsSorted(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["s3"] = Profile{Model: "x"}
	cfg.Profiles["o5"] = Profile{Model: "y"}
	var shadows []string
	for _, f := range doctorFindings(cfg, nil, t.TempDir(), "/nonexistent/clau", runtime.GOOS) {
		if f.Level == "warn" && strings.Contains(f.Msg, "shadows grammar token") {
			shadows = append(shadows, f.Msg)
		}
	}
	if len(shadows) != 2 || !strings.Contains(shadows[0], `"o5"`) || !strings.Contains(shadows[1], `"s3"`) {
		t.Errorf("shadow warnings not sorted: %v", shadows)
	}
}

func TestListRows(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["work"] = Profile{Env: map[string]string{"B": "2", "A": "1"}}
	rows := listRows(cfg, t.TempDir(), "/nonexistent/clau", "linux")
	byToken := map[string]listRow{}
	for _, r := range rows {
		byToken[r.Token] = r
	}
	if r := byToken["o5"]; r.Command != "co5" || r.Preview != "claude --model opus --effort max" || r.Linked != "no" {
		t.Errorf("o5 row = %+v", r)
	}
	if r := byToken["work"]; r.Preview != "claude [env: A, B]" {
		t.Errorf("work row = %+v", r)
	}
	if _, ok := byToken["h3"]; ok {
		t.Error("h3 must not be listed (efforts=false)")
	}
}

func TestDoctorFindings(t *testing.T) {
	cfg := defaultConfig()
	cfg.Claude = "definitely-not-a-real-binary-xyz"
	cfg.Profiles["o5"] = Profile{Model: "shadow"}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	fs := doctorFindings(cfg, nil, dir, "/nonexistent/clau", "linux")
	var fails, warns int
	var text strings.Builder
	for _, f := range fs {
		text.WriteString(f.Level + " " + f.Msg + "\n")
		switch f.Level {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	out := text.String()
	if fails == 0 || !strings.Contains(out, "definitely-not-a-real-binary-xyz") {
		t.Errorf("missing claude-not-found fail:\n%s", out)
	}
	if warns == 0 || !strings.Contains(out, "o5") {
		t.Errorf("missing profile-shadows-grammar warn:\n%s", out)
	}
}

func TestDoctorConfigError(t *testing.T) {
	fs := doctorFindings(Config{}, errors.New("boom at line 3"), t.TempDir(), "/x/clau", "linux")
	if len(fs) != 1 || fs[0].Level != "fail" || !strings.Contains(fs[0].Msg, "boom") {
		t.Errorf("findings = %+v", fs)
	}
}

func TestStarterConfigIsValidAndInert(t *testing.T) {
	p := writeConfig(t, starterConfig)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatalf("starter config invalid: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("starter config defines active profiles: %v (examples must be commented out)", cfg.Profiles)
	}
	if cfg.Models["o"].Model != "opus" {
		t.Errorf("starter config changed defaults: %+v", cfg.Models)
	}
}
