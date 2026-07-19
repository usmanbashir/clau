package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Claude != "claude" {
		t.Errorf("Claude = %q, want claude", cfg.Claude)
	}
	if got := cfg.Models["o"]; got.Model != "opus" || !got.Efforts {
		t.Errorf("o = %+v, want opus/efforts", got)
	}
	if got := cfg.Models["h"]; got.Model != "haiku" || got.Efforts {
		t.Errorf("h = %+v, want haiku/no-efforts", got)
	}
	if cfg.Efforts["5"] != "max" || cfg.Efforts["1"] != "low" {
		t.Errorf("ladder = %v", cfg.Efforts)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("default profiles = %v, want none", cfg.Profiles)
	}
}

func TestLoadConfigMissingFileGivesDefaults(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models["s"].Model != "sonnet" {
		t.Errorf("defaults not applied: %+v", cfg.Models)
	}
}

func TestLoadConfigMergesModels(t *testing.T) {
	p := writeConfig(t, "[models]\ng = \"glm-5.2\"\no = \"claude-opus-4-8\"\n")
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models["g"].Model != "glm-5.2" || !cfg.Models["g"].Efforts {
		t.Errorf("g = %+v", cfg.Models["g"])
	}
	if cfg.Models["o"].Model != "claude-opus-4-8" {
		t.Errorf("o not replaced: %+v", cfg.Models["o"])
	}
	if cfg.Models["s"].Model != "sonnet" {
		t.Errorf("s lost in merge: %+v", cfg.Models)
	}
}

func TestLoadConfigModelTableForm(t *testing.T) {
	p := writeConfig(t, "[models]\nk = { model = \"kimi\", efforts = false }\n")
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Models["k"]; got.Model != "kimi" || got.Efforts {
		t.Errorf("k = %+v", got)
	}
}

func TestLoadConfigProfilesAndGlobals(t *testing.T) {
	p := writeConfig(t, `
claude = "ccr"
[profiles.rev]
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "Review code."]
env = { ANTHROPIC_BASE_URL = "https://x" }
claude = "claude-alt"
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Claude != "ccr" {
		t.Errorf("Claude = %q", cfg.Claude)
	}
	pr := cfg.Profiles["rev"]
	if pr.Model != "opus" || pr.Effort != "high" || pr.Claude != "claude-alt" {
		t.Errorf("rev = %+v", pr)
	}
	if len(pr.Flags) != 2 || pr.Flags[0] != "--append-system-prompt" {
		t.Errorf("flags = %v", pr.Flags)
	}
	if pr.Env["ANTHROPIC_BASE_URL"] != "https://x" {
		t.Errorf("env = %v", pr.Env)
	}
}

func TestLoadConfigEffortsOverrideReplacesWholesale(t *testing.T) {
	p := writeConfig(t, "[efforts]\n1 = \"minimal\"\n2 = \"peak\"\n")
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Efforts["1"] != "minimal" || cfg.Efforts["2"] != "peak" {
		t.Errorf("efforts = %v", cfg.Efforts)
	}
	if _, ok := cfg.Efforts["5"]; ok {
		t.Error("built-in ladder leaked through wholesale replace")
	}
}

func TestLoadConfigRejectsBadInput(t *testing.T) {
	cases := []struct{ name, body, wantErr string }{
		{"parse error", "[models\n", "config.toml"},
		{"bad model key", "[models]\nO5 = \"x\"\n", "model key"},
		{"empty table model", "[models]\nz = { efforts = false }\n", "model"},
		{"bad effort key", "[efforts]\n10 = \"ten\"\n", "effort key"},
		{"empty effort value", "[efforts]\n1 = \"\"\n", "effort"},
		{"bad profile name", "[profiles.Bad_Name]\nmodel = \"opus\"\n", "profile name"},
		{"unknown top key", "clude = \"typo\"\n", "clude"},
		{"unknown model field", "[models]\nz = { model = \"x\", effrots = false }\n", "effrots"},
		{"unknown profile field", "[profiles.p]\nmodle = \"x\"\n", "modle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadConfig(writeConfig(t, c.body))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want mention of %q", err, c.wantErr)
			}
		})
	}
}

func TestConfigPath(t *testing.T) {
	t.Setenv("CLAU_CONFIG", "/tmp/override.toml")
	if got := configPath(); got != "/tmp/override.toml" {
		t.Errorf("override: %q", got)
	}
	t.Setenv("CLAU_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	want := filepath.Join("/xdg", "clau", "config.toml")
	if got := configPath(); got != want {
		t.Errorf("xdg: %q, want %q", got, want)
	}
}

func TestApplyConfigFileLayersProject(t *testing.T) {
	global := writeConfig(t, `
[models]
g = "glm-5.2"

[profiles.rev]
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "Review code."]

[profiles.work]
env = { ANTHROPIC_BASE_URL = "https://gw.example" }
`)
	base, err := loadConfig(global)
	if err != nil {
		t.Fatal(err)
	}
	project := writeConfig(t, `
claude = "claude-wrapper"

[models]
g = "glm-6.0"

[efforts]
1 = "minimal"

[profiles.rev]
model = "haiku"

[profiles.deploy]
flags = ["-p"]
`)
	layered, err := applyConfigFile(base, project)
	if err != nil {
		t.Fatal(err)
	}
	if layered.Claude != "claude-wrapper" {
		t.Errorf("claude = %q, want claude-wrapper", layered.Claude)
	}
	if layered.Models["g"].Model != "glm-6.0" {
		t.Errorf("g = %+v, want glm-6.0", layered.Models["g"])
	}
	if layered.Models["o"].Model != "opus" {
		t.Errorf("o lost in layering: %+v", layered.Models["o"])
	}
	rev := layered.Profiles["rev"]
	if rev.Model != "haiku" || rev.Effort != "" || len(rev.Flags) != 0 {
		t.Errorf("project rev must replace global rev wholesale, got %+v", rev)
	}
	if layered.Profiles["work"].Env["ANTHROPIC_BASE_URL"] != "https://gw.example" {
		t.Errorf("global-only profile lost: %+v", layered.Profiles["work"])
	}
	if _, ok := layered.Profiles["deploy"]; !ok {
		t.Error("project-only profile missing")
	}
	if layered.Efforts["1"] != "minimal" || layered.Efforts["5"] != "" {
		t.Errorf("non-empty project [efforts] must replace the whole ladder, got %v", layered.Efforts)
	}
}

func TestApplyConfigFileDoesNotMutateBase(t *testing.T) {
	base, err := loadConfig(writeConfig(t, "[profiles.rev]\nmodel = \"opus\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	project := writeConfig(t, "[models]\nz = \"zeta\"\n\n[profiles.rev]\nmodel = \"haiku\"\n")
	if _, err := applyConfigFile(base, project); err != nil {
		t.Fatal(err)
	}
	if _, ok := base.Models["z"]; ok {
		t.Error("layering leaked a model key into the base config")
	}
	if base.Profiles["rev"].Model != "opus" {
		t.Errorf("layering mutated base profile: %+v", base.Profiles["rev"])
	}
}

func TestApplyConfigFileMissingFileIsNoop(t *testing.T) {
	base := defaultConfig()
	got, err := applyConfigFile(base, filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Models["o"].Model != "opus" || len(got.Profiles) != 0 {
		t.Errorf("missing file must be a no-op, got %+v", got)
	}
}
