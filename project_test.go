package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverProjectFindsNearest(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(root, ".clau.toml")
	if err := os.WriteFile(outer, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != outer {
		t.Errorf("discoverProject = %q, want %q", got, outer)
	}
	inner := filepath.Join(root, "a", ".clau.toml")
	if err := os.WriteFile(inner, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != inner {
		t.Errorf("nearest must win: got %q, want %q", got, inner)
	}
}

func TestDiscoverProjectNone(t *testing.T) {
	if got := discoverProject(t.TempDir()); got != "" {
		t.Errorf("discoverProject = %q, want empty", got)
	}
}

func TestDiscoverProjectSkipsDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(nested, ".clau.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, ".clau.toml")
	if err := os.WriteFile(real, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := discoverProject(nested); got != real {
		t.Errorf("a directory named .clau.toml must be skipped: got %q, want %q", got, real)
	}
}

func TestTrustPathXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join("/tmp", "state"))
	want := filepath.Join("/tmp", "state", "clau", "trust.toml")
	if got := trustPath(); got != want {
		t.Errorf("trustPath = %q, want %q", got, want)
	}
}

func TestTrustRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "deep", "trust.toml")
	store := map[string]string{"/proj/.clau.toml": "abc123"}
	if err := saveTrust(p, store); err != nil {
		t.Fatal(err)
	}
	got, corrupt := loadTrust(p)
	if corrupt {
		t.Fatal("round-tripped store reported corrupt")
	}
	if got["/proj/.clau.toml"] != "abc123" {
		t.Errorf("store = %v", got)
	}
}

func TestLoadTrustMissingIsEmpty(t *testing.T) {
	store, corrupt := loadTrust(filepath.Join(t.TempDir(), "none.toml"))
	if corrupt || len(store) != 0 {
		t.Errorf("missing store: got %v corrupt=%v, want empty/false", store, corrupt)
	}
}

func TestLoadTrustCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trust.toml")
	if err := os.WriteFile(p, []byte("[trusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, corrupt := loadTrust(p)
	if !corrupt || len(store) != 0 {
		t.Errorf("corrupt store: got %v corrupt=%v, want empty/true", store, corrupt)
	}
}

func TestHashFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	got, err := hashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("hashFile = %q, want %q", got, want)
	}
}

// projectFixture writes a global config, a project dir with .clau.toml,
// and points CLAU_CONFIG and XDG_STATE_HOME at per-test locations.
// It returns the project dir and the project file path.
func projectFixture(t *testing.T, globalTOML, projectTOML string) (string, string) {
	t.Helper()
	t.Setenv("CLAU_CONFIG", writeConfig(t, globalTOML))
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	proj := filepath.Join(dir, ".clau.toml")
	if err := os.WriteFile(proj, []byte(projectTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, proj
}

func trustNow(t *testing.T, proj string) {
	t.Helper()
	hash, err := hashFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := loadTrust(trustPath())
	store[proj] = hash
	if err := saveTrust(trustPath(), store); err != nil {
		t.Fatal(err)
	}
}

func TestLoadEffectiveConfigTrustedApplies(t *testing.T) {
	dir, proj := projectFixture(t, "[profiles.rev]\nmodel = \"opus\"\n", "[profiles.rev]\nmodel = \"haiku\"\n")
	trustNow(t, proj)
	cfg, st, err := loadEffectiveConfig(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Trusted || !st.Applied || st.Path != proj {
		t.Errorf("status = %+v", st)
	}
	if cfg.Profiles["rev"].Model != "haiku" {
		t.Errorf("project layer not applied: %+v", cfg.Profiles["rev"])
	}
}

func TestLoadEffectiveConfigUntrustedEnforceErrors(t *testing.T) {
	dir, proj := projectFixture(t, "", "[profiles.rev]\nmodel = \"haiku\"\n")
	_, _, err := loadEffectiveConfig(dir, true)
	if err == nil {
		t.Fatal("expected error for untrusted project config")
	}
	if !strings.Contains(err.Error(), proj) || !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadEffectiveConfigUntrustedNoEnforce(t *testing.T) {
	dir, proj := projectFixture(t, "[profiles.rev]\nmodel = \"opus\"\n", "[profiles.rev]\nmodel = \"haiku\"\n")
	cfg, st, err := loadEffectiveConfig(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Path != proj || st.Trusted || st.Applied || st.Changed {
		t.Errorf("status = %+v", st)
	}
	if cfg.Profiles["rev"].Model != "opus" {
		t.Errorf("untrusted layer leaked into config: %+v", cfg.Profiles["rev"])
	}
}

func TestLoadEffectiveConfigChangedHash(t *testing.T) {
	dir, proj := projectFixture(t, "", "[profiles.rev]\nmodel = \"haiku\"\n")
	trustNow(t, proj)
	if err := os.WriteFile(proj, []byte("[profiles.rev]\nmodel = \"opus\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, st, err := loadEffectiveConfig(dir, true)
	if err == nil || !strings.Contains(err.Error(), "changed since it was trusted") {
		t.Errorf("err = %v", err)
	}
	if !st.Changed {
		t.Errorf("status = %+v", st)
	}
}

func TestLoadEffectiveConfigEnvSkips(t *testing.T) {
	dir, _ := projectFixture(t, "", "[profiles.rev]\nmodel = \"haiku\"\n")
	t.Setenv("CLAU_NO_PROJECT", "1")
	cfg, st, err := loadEffectiveConfig(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Path != "" {
		t.Errorf("status = %+v, want no project", st)
	}
	if _, ok := cfg.Profiles["rev"]; ok {
		t.Error("project profile applied despite CLAU_NO_PROJECT")
	}
}

func TestLoadEffectiveConfigGlobalIsProjectFile(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, ".clau.toml")
	if err := os.WriteFile(proj, []byte("[profiles.rev]\nmodel = \"haiku\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAU_CONFIG", proj)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg, st, err := loadEffectiveConfig(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Path != "" {
		t.Errorf("global file must not double-apply as project layer: %+v", st)
	}
	if cfg.Profiles["rev"].Model != "haiku" {
		t.Errorf("global config lost: %+v", cfg.Profiles)
	}
}

func TestLoadEffectiveConfigTrustedParseError(t *testing.T) {
	dir, proj := projectFixture(t, "", "[models\n")
	trustNow(t, proj)
	_, _, err := loadEffectiveConfig(dir, true)
	if err == nil || !strings.Contains(err.Error(), proj) {
		t.Errorf("err = %v, want parse error naming the project file", err)
	}
}

func TestProjectDeclarations(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".clau.toml")
	body := "[models]\ng = \"glm-5.2\"\n\n[profiles.deploy]\nflags = [\"-p\"]\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	models, profiles, err := projectDeclarations(p)
	if err != nil {
		t.Fatal(err)
	}
	if !models["g"] || len(models) != 1 {
		t.Errorf("models = %v", models)
	}
	if !profiles["deploy"] || len(profiles) != 1 {
		t.Errorf("profiles = %v", profiles)
	}
}
