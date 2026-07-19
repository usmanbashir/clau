# Per-Project Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A repo-committable `.clau.toml` layered over the global config, gated by TOFU trust (`clau trust`/`clau untrust`), with provenance in `list`/`doctor` — per `docs/superpowers/specs/2026-07-19-per-project-config-design.md`.

**Architecture:** Config loading gains one layer (defaults ← global ← project) by extracting the existing single-file loader into `applyConfigFile`. A new `project.go` owns discovery (walk cwd → root for `.clau.toml`), the trust store (path → SHA-256 in `$XDG_STATE_HOME/clau/trust.toml`), and `loadEffectiveConfig`, which launch paths call with enforcement on and read-only commands call with enforcement off. Resolution, linking, and exec are untouched.

**Tech Stack:** Go stdlib (`crypto/sha256`, `encoding/hex`) + the existing `github.com/BurntSushi/toml` (its `Encoder` writes the trust store). No new dependencies.

## Global Constraints

- No new dependencies; stdlib + BurntSushi/toml only.
- With no `.clau.toml` present, behavior is byte-identical to v0.1.x (spec: "Errors").
- Error strings start lowercase and are printed via `fatal()`, which prefixes `clau: `.
- `CLAU_NO_PROJECT` non-empty skips discovery entirely; documented as `CLAU_NO_PROJECT=1`.
- Tests never `os.Chdir`; unit code takes `cwd`/paths as parameters, integration tests set `cmd.Dir`.
- All files `gofmt`-clean; commit messages imperative, single line, no attribution trailers (house style).
- Windows-safe: `filepath` everywhere; integration tests that need symlinks follow the existing `t.Skip` pattern.
- The fake claude harness (`testdata/fakeclaude`, built by `TestMain`) records `ARGV\t...` and `ENV\tK=V` lines into `$CLAU_TEST_OUT`; integration tests read it back via the existing `recorded` struct.

---

### Task 1: Extract `applyConfigFile` (layered loading primitive)

**Files:**
- Modify: `config.go` (function `loadConfig`, lines 91–163)
- Test: `config_test.go`

**Interfaces:**
- Consumes: existing `Config`, `defaultConfig()`, `rawConfig` decode logic.
- Produces: `applyConfigFile(cfg Config, path string) (Config, error)` — decodes `path` and merges it over `cfg` with the existing semantics (models per-key, profiles per-name wholesale, non-empty efforts table replaces, claude if set; missing file returns `cfg` unchanged). Never mutates the maps of the `cfg` argument. `loadConfig(path)` becomes `applyConfigFile(defaultConfig(), path)`. Also `cloneMap[V any](m map[string]V) map[string]V`.

- [ ] **Step 1: Write the failing tests**

Append to `config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestApplyConfigFile`
Expected: FAIL — `undefined: applyConfigFile` (build error).

- [ ] **Step 3: Refactor `config.go`**

Replace the current `loadConfig` (lines 91–163) with:

```go
func loadConfig(path string) (Config, error) {
	return applyConfigFile(defaultConfig(), path)
}

func cloneMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// applyConfigFile decodes path and merges it over cfg: models per key,
// profiles per name (wholesale), a non-empty efforts table replaces the
// ladder, claude if set. A missing file returns cfg unchanged. The maps
// of the cfg argument are never mutated.
func applyConfigFile(cfg Config, path string) (Config, error) {
	var raw rawConfig
	md, err := toml.DecodeFile(path, &raw)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Config{}, fmt.Errorf("%s: %s", path, pe.ErrorWithPosition())
		}
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Models = cloneMap(cfg.Models)
	cfg.Profiles = cloneMap(cfg.Profiles)
	if raw.Claude != "" {
		cfg.Claude = raw.Claude
	}
	for key, prim := range raw.Models {
		if !modelKeyRe.MatchString(key) {
			return Config{}, fmt.Errorf("%s: invalid model key %q (lowercase letters only)", path, key)
		}
		var name string
		if err := md.PrimitiveDecode(prim, &name); err == nil {
			if name == "" {
				return Config{}, fmt.Errorf("%s: model %q: empty model name", path, key)
			}
			cfg.Models[key] = ModelSpec{Model: name, Efforts: true}
			continue
		}
		var rm rawModel
		if err := md.PrimitiveDecode(prim, &rm); err != nil {
			return Config{}, fmt.Errorf("%s: model %q: %w", path, key, err)
		}
		if rm.Model == "" {
			return Config{}, fmt.Errorf("%s: model %q: missing model name", path, key)
		}
		spec := ModelSpec{Model: rm.Model, Efforts: true}
		if rm.Efforts != nil {
			spec.Efforts = *rm.Efforts
		}
		cfg.Models[key] = spec
	}
	if len(raw.Efforts) > 0 {
		cfg.Efforts = map[string]string{}
		for key, level := range raw.Efforts {
			if !effortKeyRe.MatchString(key) {
				return Config{}, fmt.Errorf("%s: invalid effort key %q (single digit 1-9)", path, key)
			}
			if level == "" {
				return Config{}, fmt.Errorf("%s: effort %q: empty level", path, key)
			}
			cfg.Efforts[key] = level
		}
	}
	for name, rp := range raw.Profiles {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid profile name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		cfg.Profiles[name] = Profile{
			Model: rp.Model, Effort: rp.Effort, Flags: rp.Flags,
			Env: rp.Env, Claude: rp.Claude,
		}
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		keys := make([]string, len(undec))
		for i, k := range undec {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("%s: unknown key(s): %v", path, keys)
	}
	return cfg, nil
}
```

(This is the existing body with the `defaultConfig()` seed removed and the two `cloneMap` lines added after decode. Everything else is verbatim.)

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS (new tests and the entire existing suite — the refactor must not change `loadConfig` behavior).

- [ ] **Step 5: Commit**

```bash
git add config.go config_test.go
git commit -m "Extract applyConfigFile for layered config loading"
```

---

### Task 2: Project discovery walk

**Files:**
- Create: `project.go`
- Create: `project_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `discoverProject(dir string) string` — walks from `dir` up to the filesystem root; returns the nearest regular file named `.clau.toml`, or `""`.

- [ ] **Step 1: Write the failing tests**

Create `project_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestDiscoverProject`
Expected: FAIL — `undefined: discoverProject`.

- [ ] **Step 3: Implement**

Create `project.go`:

```go
package main

import (
	"os"
	"path/filepath"
)

// discoverProject walks from dir toward the filesystem root and returns
// the nearest regular file named .clau.toml, or "" if there is none.
func discoverProject(dir string) string {
	for {
		p := filepath.Join(dir, ".clau.toml")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestDiscoverProject`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add project.go project_test.go
git commit -m "Add project config discovery walk"
```

---

### Task 3: Trust store primitives

**Files:**
- Modify: `project.go`
- Test: `project_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces:
  - `trustPath() string` — `$XDG_STATE_HOME/clau/trust.toml`, falling back to `~/.local/state/clau/trust.toml` (mirrors `configPath()`'s fallback style).
  - `loadTrust(path string) (map[string]string, bool)` — store plus `corrupt`; missing file → empty store, `corrupt=false`; unparseable file → empty store, `corrupt=true`. Never an error.
  - `saveTrust(path string, store map[string]string) error` — creates parent dirs, writes TOML `[trusted]` table.
  - `hashFile(path string) (string, error)` — lowercase hex SHA-256 of content.

- [ ] **Step 1: Write the failing tests**

Append to `project_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestTrust|TestLoadTrust|TestHashFile'`
Expected: FAIL — `undefined: trustPath` (and friends).

- [ ] **Step 3: Implement**

Append to `project.go` (extend the import block to `crypto/sha256`, `encoding/hex`, `errors`, and `github.com/BurntSushi/toml`):

```go
// trustPath returns the trust store location. Like configPath, the
// XDG fallback (~/.local/state) is used on every platform.
func trustPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "clau", "trust.toml")
}

type trustFile struct {
	Trusted map[string]string `toml:"trusted"`
}

// loadTrust reads the trust store. A missing file is an empty store;
// corrupt=true means the file existed but did not parse (treated as
// empty — never a crash).
func loadTrust(path string) (map[string]string, bool) {
	var tf trustFile
	if _, err := toml.DecodeFile(path, &tf); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, false
		}
		return map[string]string{}, true
	}
	if tf.Trusted == nil {
		tf.Trusted = map[string]string{}
	}
	return tf.Trusted, false
}

func saveTrust(path string, store map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(trustFile{Trusted: store})
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestTrust|TestLoadTrust|TestHashFile'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add project.go project_test.go
git commit -m "Add TOFU trust store primitives"
```

---

### Task 4: `loadEffectiveConfig` (discovery + trust + layering)

**Files:**
- Modify: `project.go`
- Test: `project_test.go`

**Interfaces:**
- Consumes: `loadConfig`, `applyConfigFile` (Task 1); `discoverProject` (Task 2); `trustPath`, `loadTrust`, `hashFile` (Task 3); `configPath()` from `config.go`.
- Produces:
  - `type ProjectStatus struct { Path string; Trusted, Changed, Applied bool }` — `Path` empty means no project layer in play.
  - `loadEffectiveConfig(cwd string, enforce bool) (Config, ProjectStatus, error)` — global config layered with the project file when trusted. `enforce=true`: untrusted/changed project file is an error. `enforce=false`: returns the global-only config plus status. `CLAU_NO_PROJECT` non-empty, no discovery. Discovered file identical to the global config file: layer skipped.

- [ ] **Step 1: Write the failing tests**

Append to `project_test.go` (extend imports with `"strings"`):

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestLoadEffectiveConfig`
Expected: FAIL — `undefined: loadEffectiveConfig` (and `ProjectStatus`).

- [ ] **Step 3: Implement**

Append to `project.go` (add `"fmt"` to imports):

```go
type ProjectStatus struct {
	Path    string // discovered .clau.toml; "" if none in play
	Trusted bool   // content hash matches the trust store
	Changed bool   // in the store, but content differs
	Applied bool   // layer merged into the returned Config
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// loadEffectiveConfig loads the global config and, when a trusted
// .clau.toml is discovered walking up from cwd, layers it on top.
// enforce=true makes an untrusted or changed project file a hard error
// (launch paths); enforce=false returns the global-only view plus
// status (list, doctor). CLAU_NO_PROJECT (non-empty) skips discovery.
func loadEffectiveConfig(cwd string, enforce bool) (Config, ProjectStatus, error) {
	global, err := loadConfig(configPath())
	if err != nil {
		return Config{}, ProjectStatus{}, err
	}
	if os.Getenv("CLAU_NO_PROJECT") != "" {
		return global, ProjectStatus{}, nil
	}
	proj := discoverProject(cwd)
	if proj == "" || sameFile(proj, configPath()) {
		return global, ProjectStatus{}, nil
	}
	st := ProjectStatus{Path: proj}
	hash, err := hashFile(proj)
	if err != nil {
		return Config{}, st, fmt.Errorf("project config %s: %v", proj, err)
	}
	store, _ := loadTrust(trustPath())
	switch stored, known := store[proj]; {
	case known && stored == hash:
		st.Trusted = true
	case known:
		st.Changed = true
	}
	if !st.Trusted {
		if enforce {
			reason := "is not trusted"
			if st.Changed {
				reason = "changed since it was trusted"
			}
			return Config{}, st, fmt.Errorf(
				"project config %s %s; review with `clau trust --show`, then `clau trust` to allow (CLAU_NO_PROJECT=1 skips project configs)",
				proj, reason)
		}
		return global, st, nil
	}
	layered, err := applyConfigFile(global, proj)
	if err != nil {
		return Config{}, st, err
	}
	st.Applied = true
	return layered, st, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestLoadEffectiveConfig`
Expected: PASS (7 tests). Then `go test ./...` — full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add project.go project_test.go
git commit -m "Add loadEffectiveConfig with TOFU enforcement"
```

---

### Task 5: Wire launch paths through the project layer

**Files:**
- Modify: `main.go` (`mustLoadConfig` callers: `runLauncher` line 96, `runNamed` line 115, `cmdRun` line 130)
- Test: `integration_test.go`

**Interfaces:**
- Consumes: `loadEffectiveConfig` (Task 4).
- Produces: `mustLoadEffectiveConfig() Config` in `main.go` — cwd-aware, enforcing, `fatal()` on error. `mustLoadConfig` remains for `cmdLink` (linking stays global-only per spec).

- [ ] **Step 1: Write the failing integration tests**

Append to `integration_test.go` (extend imports with `"crypto/sha256"` and `"encoding/hex"`):

```go
// runInProject runs bin with cwd set to dir, CLAU_CONFIG pointing at a
// global config written from globalTOML, XDG_STATE_HOME at state, and
// the fake claude on PATH. It returns combined output, the error, and
// whatever the fake claude recorded (zero recorded if it never ran).
func runInProject(t *testing.T, bin, dir, globalTOML, state string, extraEnv []string, args ...string) (string, error, recorded) {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out.txt")
	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte(globalTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+fakeClaudeDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CLAU_TEST_OUT="+outFile,
		"CLAU_CONFIG="+cfgFile,
		"XDG_STATE_HOME="+state,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	rec := recorded{env: map[string]string{}}
	if data, rerr := os.ReadFile(outFile); rerr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			fields := strings.Split(line, "\t")
			switch fields[0] {
			case "ARGV":
				rec.argv = fields[1:]
			case "ENV":
				k, v, _ := strings.Cut(fields[1], "=")
				rec.env[k] = v
			}
		}
	}
	return string(out), err, rec
}

// seedTrust writes a trust store in state covering the given project file.
func seedTrust(t *testing.T, state, proj string) {
	t.Helper()
	data, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	dir := filepath.Join(state, "clau")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := fmt.Sprintf("[trusted]\n%q = %q\n", proj, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(dir, "trust.toml"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProject(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	proj := filepath.Join(dir, ".clau.toml")
	if err := os.WriteFile(proj, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, proj
}

const projectGlobal = "[profiles.rev]\nmodel = \"opus\"\n"
const projectLayer = "[profiles.rev]\nmodel = \"haiku\"\n"

func TestIntegrationProjectTrustedOverrides(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()
	seedTrust(t, state, proj)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err != nil {
		t.Fatalf("launch failed: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\thaiku" {
		t.Errorf("argv = %q, want project override --model haiku", got)
	}
}

func TestIntegrationProjectUntrustedFails(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, projectLayer)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, t.TempDir(), nil, "rev")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "not trusted") || !strings.Contains(out, "clau trust") {
		t.Errorf("stderr = %s", out)
	}
	if len(rec.argv) != 0 || len(rec.env) != 0 {
		t.Errorf("claude must not run untrusted, recorded %+v", rec)
	}
}

func TestIntegrationProjectChangedFails(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()
	seedTrust(t, state, proj)
	if err := os.WriteFile(proj, []byte("[profiles.rev]\nmodel = \"sonnet\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "changed since it was trusted") {
		t.Errorf("stderr = %s", out)
	}
}

func TestIntegrationNoProjectEnvSkips(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, projectLayer)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, t.TempDir(),
		[]string{"CLAU_NO_PROJECT=1"}, "rev")
	if err != nil {
		t.Fatalf("launch failed: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\topus" {
		t.Errorf("argv = %q, want global --model opus", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestIntegrationProject`
Expected: `TestIntegrationProjectTrustedOverrides` FAILs with argv `--model opus` (project layer ignored), and `TestIntegrationProjectUntrustedFails` / `TestIntegrationProjectChangedFails` FAIL because the launch succeeds. (`TestIntegrationNoProjectEnvSkips` may already pass — that is fine.)

- [ ] **Step 3: Implement**

In `main.go`, add below `mustLoadConfig` (line 80):

```go
// mustLoadEffectiveConfig is mustLoadConfig plus the trusted project
// layer; launch paths use it so an untrusted .clau.toml is a hard error.
func mustLoadEffectiveConfig() Config {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	cfg, _, err := loadEffectiveConfig(cwd, true)
	if err != nil {
		fatal("%v", err)
	}
	return cfg
}
```

Then change exactly three call sites — `runLauncher`, `runNamed`, and `cmdRun` — from `cfg := mustLoadConfig()` to `cfg := mustLoadEffectiveConfig()`. `cmdLink` keeps `mustLoadConfig()` (linking stays global-only).

Finally, make the pre-existing integration tests hermetic: they run the child with `cmd.Dir` unset (the repo dir) and would pick up a stray `.clau.toml` in any ancestor directory on a developer's machine. Append `"CLAU_NO_PROJECT=1"` to the three env slices that don't go through `runInProject`:

- in `runShortcut` (the `cmd.Env = append(os.Environ(), ...)` block, line ~89):

```go
		"CLAU_CONFIG="+cfgFile,
		"CLAU_NO_PROJECT=1",
```

- in `TestIntegrationStaleNameErrors` (line ~194):

```go
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+filepath.Join(dir, "none.toml"), "CLAU_NO_PROJECT=1")
```

- in `TestIntegrationBrokenConfigFailsEvenOnPassthrough` (line ~210):

```go
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+cfg, "CLAU_NO_PROJECT=1")
```

`runInProject` must NOT get this variable — it exists to exercise the layer.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS, including all four new integration tests and every pre-existing test (now explicitly hermetic against ancestor `.clau.toml` files).

- [ ] **Step 5: Commit**

```bash
git add main.go integration_test.go
git commit -m "Layer trusted project config into launch paths"
```

---

### Task 6: `clau trust` and `clau untrust`

**Files:**
- Modify: `main.go` (`reservedVerbs` line 28, `runManagement` line 141, `helpText` line 233)
- Modify: `commands.go` (new `cmdTrust`, `cmdUntrust`)
- Modify: `completions.go` (verb lists: fish line 10, bash line 20, zsh line 38)
- Test: `main_test.go`, `integration_test.go`, `completions_test.go`

**Interfaces:**
- Consumes: `discoverProject`, `trustPath`, `loadTrust`, `saveTrust`, `hashFile` (Tasks 2–3); `applyConfigFile`, `loadConfig` (Task 1); test helpers `writeProject`, `runInProject` and consts `projectGlobal`/`projectLayer` from Task 5's integration tests.
- Produces: `cmdTrust(args []string)` (flag `--show`), `cmdUntrust(args []string)`. Verbs `trust` and `untrust` reserved and completable.

- [ ] **Step 1: Write the failing tests**

In `main_test.go`, add two cases to the `TestDispatch` table (after the `run` case, line 45):

```go
		{"clau", []string{"trust", "--show"}, action{kind: "management", verb: "trust", args: []string{"--show"}}},
		{"clau", []string{"untrust"}, action{kind: "management", verb: "untrust", args: []string{}}},
```

In `completions_test.go`, append:

```go
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
```

In `integration_test.go`, append:

```go
func TestIntegrationTrustVerbFlow(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()

	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "trust", "--show")
	if err != nil {
		t.Fatalf("trust --show: %v\n%s", err, out)
	}
	if !strings.Contains(out, proj) || !strings.Contains(out, "haiku") {
		t.Errorf("trust --show output = %s", out)
	}

	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "trust")
	if err != nil {
		t.Fatalf("trust: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trusted "+proj) {
		t.Errorf("trust output = %s", out)
	}

	out, err, rec := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err != nil {
		t.Fatalf("launch after trust: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\thaiku" {
		t.Errorf("argv = %q, want --model haiku", got)
	}

	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "untrust")
	if err != nil {
		t.Fatalf("untrust: %v\n%s", err, out)
	}
	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err == nil {
		t.Fatalf("launch after untrust must fail, got: %s", out)
	}
}

func TestIntegrationTrustRefusesBrokenFile(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, "[models\n")
	out, err, _ := runInProject(t, bin, dir, projectGlobal, t.TempDir(), nil, "trust")
	if err == nil {
		t.Fatalf("expected refusal, got: %s", out)
	}
	if !strings.Contains(out, "refusing to trust") {
		t.Errorf("stderr = %s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestDispatch|TestCompletionsIncludeTrustVerbs|TestIntegrationTrust'`
Expected: FAIL — dispatch treats `trust` as a launch token; completions lack the verbs; `clau trust` errors with `unknown shortcut token` in the integration flow.

- [ ] **Step 3: Implement**

`main.go` — add to `reservedVerbs` (line 28):

```go
var reservedVerbs = map[string]bool{
	"link": true, "unlink": true, "list": true, "run": true,
	"init": true, "trust": true, "untrust": true,
	"doctor": true, "completions": true,
	"version": true, "help": true, "__launch": true,
}
```

Add to the `runManagement` switch (after the `"init"` case):

```go
	case "trust":
		cmdTrust(args)
	case "untrust":
		cmdUntrust(args)
```

In `helpText`, directly after the line `  clau init [--force]              write a starter config` insert:

```
  clau trust [--show]              allow (or print) the project .clau.toml
  clau untrust                     revoke trust for the project config
```

and directly after the line `Config: $CLAU_CONFIG or ~/.config/clau/config.toml` insert:

```
Per-project: nearest .clau.toml once allowed via clau trust (CLAU_NO_PROJECT=1 skips)
```

`commands.go` — append:

```go
func cmdTrust(args []string) {
	fs := flag.NewFlagSet("trust", flag.ExitOnError)
	show := fs.Bool("show", false, "print the project config instead of trusting it")
	fs.Parse(args)
	cwd, err := os.Getwd()
	if err != nil {
		fatal("%v", err)
	}
	proj := discoverProject(cwd)
	if proj == "" {
		fatal("no .clau.toml found from %s upward", cwd)
	}
	if *show {
		data, err := os.ReadFile(proj)
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("-- %s\n", proj)
		os.Stdout.Write(data)
		return
	}
	global, err := loadConfig(configPath())
	if err != nil {
		fatal("%v", err)
	}
	if _, err := applyConfigFile(global, proj); err != nil {
		fatal("refusing to trust: %v", err)
	}
	hash, err := hashFile(proj)
	if err != nil {
		fatal("%v", err)
	}
	store, corrupt := loadTrust(trustPath())
	if corrupt {
		fmt.Fprintf(os.Stderr, "clau: trust store was unreadable; rewriting %s\n", trustPath())
	}
	store[proj] = hash
	if err := saveTrust(trustPath(), store); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("trusted %s (%s)\n", proj, hash[:12])
}

func cmdUntrust(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fatal("%v", err)
	}
	proj := discoverProject(cwd)
	if proj == "" {
		fatal("no .clau.toml found from %s upward", cwd)
	}
	store, _ := loadTrust(trustPath())
	if _, ok := store[proj]; !ok {
		fatal("%s is not in the trust store", proj)
	}
	delete(store, proj)
	if err := saveTrust(trustPath(), store); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("untrusted %s\n", proj)
}
```

`completions.go` — in all three scripts, extend the verb list `link unlink list run init doctor completions version help` to `link unlink list run init trust untrust doctor completions version help` (fish line 10, bash line 20's `compgen -W` string, zsh line 38's `items=(...)`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS, including the full trust→launch→untrust integration flow.

- [ ] **Step 5: Commit**

```bash
git add main.go commands.go completions.go main_test.go completions_test.go integration_test.go
git commit -m "Add trust and untrust verbs with completions"
```

---

### Task 7: `clau list` provenance (SOURCE column)

**Files:**
- Modify: `commands.go` (`listRow` line 43, `listRows` line 69, `cmdList` line 82)
- Modify: `project.go` (new `projectDeclarations`)
- Test: `commands_test.go`, `project_test.go`, `integration_test.go`

**Interfaces:**
- Consumes: `loadEffectiveConfig`, `ProjectStatus` (Task 4); test helpers `writeProject`, `runInProject`, `seedTrust` and consts `projectGlobal`/`projectLayer` from Task 5's integration tests.
- Produces:
  - `projectDeclarations(path string) (models, profiles map[string]bool, err error)` in `project.go` — the model keys and profile names the project file itself declares.
  - `tokenSource(token string, projModels, projProfiles map[string]bool) string` in `commands.go` — `"project"` or `"global"`.
  - `listRow` gains `Source string`; `listRows` gains the two map parameters (nil maps ⇒ everything `"global"`).

- [ ] **Step 1: Write the failing tests**

Append to `project_test.go`:

```go
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
```

Append to `commands_test.go`:

```go
func TestTokenSource(t *testing.T) {
	projModels := map[string]bool{"g": true}
	projProfiles := map[string]bool{"rev": true}
	cases := map[string]string{
		"rev": "project", // declared profile, even if it shadows a global one
		"g":   "project", // project model letter
		"g5":  "project", // grammar token of a project model
		"o5":  "global",
		"s":   "global",
	}
	for token, want := range cases {
		if got := tokenSource(token, projModels, projProfiles); got != want {
			t.Errorf("tokenSource(%q) = %q, want %q", token, got, want)
		}
	}
	if got := tokenSource("rev", nil, nil); got != "global" {
		t.Errorf("nil maps must mean global, got %q", got)
	}
}

func TestListRowsSource(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{Model: "haiku"}
	rows := listRows(cfg, t.TempDir(), "/nonexistent/clau", "linux",
		nil, map[string]bool{"rev": true})
	byToken := map[string]listRow{}
	for _, r := range rows {
		byToken[r.Token] = r
	}
	if byToken["rev"].Source != "project" {
		t.Errorf("rev row = %+v", byToken["rev"])
	}
	if byToken["o5"].Source != "global" {
		t.Errorf("o5 row = %+v", byToken["o5"])
	}
}
```

Also update the existing `TestListRows` (line 49) call to the new signature:

```go
	rows := listRows(cfg, t.TempDir(), "/nonexistent/clau", "linux", nil, nil)
```

Append to `integration_test.go`:

```go
func TestIntegrationListShowsProvenance(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()

	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list untrusted: %v\n%s", err, out)
	}
	// The default h model row always contains "haiku", so assert on the
	// rev row specifically, not on the whole output.
	if !strings.Contains(out, "NOT trusted") || strings.Contains(out, "SOURCE") {
		t.Errorf("untrusted list must warn and stay in the global shape:\n%s", out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "opus") {
		t.Errorf("untrusted rev row must show the global launch:\n%s", out)
	}

	seedTrust(t, state, proj)
	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list trusted: %v\n%s", err, out)
	}
	if !strings.Contains(out, "SOURCE") {
		t.Errorf("trusted list must show the SOURCE column:\n%s", out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "haiku") || !strings.Contains(line, "project") {
		t.Errorf("trusted rev row must show the project override and source:\n%s", out)
	}
}

// lineContaining returns the first output line containing substr.
func lineContaining(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestProjectDeclarations|TestTokenSource|TestListRows|TestIntegrationListShowsProvenance'`
Expected: FAIL — `undefined: projectDeclarations`, `undefined: tokenSource`, and `listRows` signature mismatch (build errors).

- [ ] **Step 3: Implement**

Append to `project.go`:

```go
// projectDeclarations reports which model keys and profile names the
// project file itself declares, for provenance display. The file has
// already been validated by the time this is called.
func projectDeclarations(path string) (map[string]bool, map[string]bool, error) {
	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, nil, err
	}
	models, profiles := map[string]bool{}, map[string]bool{}
	for k := range raw.Models {
		models[k] = true
	}
	for k := range raw.Profiles {
		profiles[k] = true
	}
	return models, profiles, nil
}
```

In `commands.go`, change `listRow` and `listRows`, and add `tokenSource`:

```go
type listRow struct {
	Token, Command, Linked, Source, Preview string
}

// tokenSource reports where a token's effective definition comes from:
// "project" for profiles the project file declares and for grammar
// tokens of project-declared model letters, else "global".
func tokenSource(token string, projModels, projProfiles map[string]bool) string {
	if projProfiles[token] {
		return "project"
	}
	letters := token
	if last := token[len(token)-1]; last >= '0' && last <= '9' {
		letters = token[:len(token)-1]
	}
	if projModels[letters] {
		return "project"
	}
	return "global"
}

func listRows(cfg Config, dir, clauPath, goos string, projModels, projProfiles map[string]bool) []listRow {
	var rows []listRow
	for _, token := range tokensOf(cfg) {
		command := "c" + token
		linked := "no"
		if isOwned(filepath.Join(dir, linkFileName(command, goos)), clauPath) {
			linked = "yes"
		}
		rows = append(rows, listRow{
			Token: token, Command: command, Linked: linked,
			Source:  tokenSource(token, projModels, projProfiles),
			Preview: previewFor(cfg, token),
		})
	}
	return rows
}
```

Rewrite `cmdList`:

```go
func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tokens := fs.Bool("tokens", false, "print bare tokens only")
	dir := fs.String("dir", defaultLinkDir(), "directory checked for links")
	fs.Parse(args)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	cfg, st, err := loadEffectiveConfig(cwd, false)
	if err != nil {
		fatal("%v", err)
	}
	if *tokens {
		for _, t := range tokensOf(cfg) {
			fmt.Println(t)
		}
		return
	}
	var projModels, projProfiles map[string]bool
	if st.Path != "" {
		state := "trusted"
		switch {
		case st.Changed:
			state = "changed, NOT applied — run `clau trust` to re-allow"
		case !st.Trusted:
			state = "NOT trusted, NOT applied — run `clau trust`"
		}
		fmt.Printf("project: %s (%s)\n", st.Path, state)
		if st.Applied {
			projModels, projProfiles, _ = projectDeclarations(st.Path)
		}
	}
	clauPath, err := clauExecutable()
	if err != nil {
		clauPath = ""
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	if st.Applied {
		fmt.Fprintln(w, "TOKEN\tCOMMAND\tLINKED\tSOURCE\tLAUNCHES")
	} else {
		fmt.Fprintln(w, "TOKEN\tCOMMAND\tLINKED\tLAUNCHES")
	}
	for _, r := range listRows(cfg, *dir, clauPath, runtime.GOOS, projModels, projProfiles) {
		if st.Applied {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Token, r.Command, r.Linked, r.Source, r.Preview)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Token, r.Command, r.Linked, r.Preview)
		}
	}
	w.Flush()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS. (`clau list --tokens` now reflects the effective config, so completions offer project tokens inside trusted projects — intended.)

- [ ] **Step 5: Commit**

```bash
git add commands.go project.go commands_test.go project_test.go integration_test.go
git commit -m "Show project provenance in clau list"
```

---

### Task 8: `clau doctor` project findings

**Files:**
- Modify: `commands.go` (new `projectFindings`; `cmdDoctor` line 225)
- Test: `commands_test.go`

**Interfaces:**
- Consumes: `ProjectStatus`, `loadEffectiveConfig`, `loadTrust`, `trustPath`, `projectDeclarations` (Tasks 4, 7).
- Produces: `projectFindings(st ProjectStatus, store map[string]string, corrupt bool) []finding` — appended to `doctorFindings`' output by `cmdDoctor`. Untrusted/changed are `warn` (the block is by design, not a broken setup); corrupt store and stale entries are `warn`; nothing here is `fail`.

- [ ] **Step 1: Write the failing tests**

Append to `commands_test.go` (extend imports with `"os"` and `"path/filepath"`):

```go
func TestProjectFindings(t *testing.T) {
	present := filepath.Join(t.TempDir(), ".clau.toml")
	if err := os.WriteFile(present, []byte("[profiles.d]\nflags=[\"-p\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "gone", ".clau.toml")
	cases := []struct {
		name string
		st   ProjectStatus
		want string
	}{
		{"none", ProjectStatus{}, "no project config"},
		{"applied", ProjectStatus{Path: present, Trusted: true, Applied: true}, "trusted and applied"},
		{"untrusted", ProjectStatus{Path: present}, "not trusted"},
		{"changed", ProjectStatus{Path: present, Changed: true}, "changed since"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var text strings.Builder
			for _, f := range projectFindings(c.st, map[string]string{missing: "x"}, true) {
				text.WriteString(f.Level + " " + f.Msg + "\n")
			}
			out := text.String()
			if !strings.Contains(out, c.want) {
				t.Errorf("missing %q in:\n%s", c.want, out)
			}
			if !strings.Contains(out, "unreadable") {
				t.Errorf("missing corrupt-store warn in:\n%s", out)
			}
			if !strings.Contains(out, missing) {
				t.Errorf("missing stale-entry warn in:\n%s", out)
			}
			if strings.Contains(out, "fail ") {
				t.Errorf("project findings must never fail:\n%s", out)
			}
		})
	}
}

func TestProjectFindingsListsDeclarations(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".clau.toml")
	if err := os.WriteFile(p, []byte("[models]\ng = \"glm-5.2\"\n\n[profiles.deploy]\nflags=[\"-p\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, f := range projectFindings(ProjectStatus{Path: p, Trusted: true, Applied: true}, nil, false) {
		text.WriteString(f.Msg + "\n")
	}
	out := text.String()
	if !strings.Contains(out, "deploy") || !strings.Contains(out, "g") {
		t.Errorf("declarations not reported:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestProjectFindings`
Expected: FAIL — `undefined: projectFindings`.

- [ ] **Step 3: Implement**

Append to `commands.go`:

```go
// projectFindings reports the project layer's state for doctor. Nothing
// here is a "fail": an untrusted file blocking launches is the designed
// safe state, not a broken installation.
func projectFindings(st ProjectStatus, store map[string]string, corrupt bool) []finding {
	var fs []finding
	if corrupt {
		fs = append(fs, finding{"warn", fmt.Sprintf("trust store %s is unreadable; treating it as empty", trustPath())})
	}
	switch {
	case st.Path == "":
		fs = append(fs, finding{"ok", "no project config in effect"})
	case st.Applied:
		fs = append(fs, finding{"ok", fmt.Sprintf("project config %s trusted and applied", st.Path)})
		if models, profiles, err := projectDeclarations(st.Path); err == nil {
			var parts []string
			if len(models) > 0 {
				keys := make([]string, 0, len(models))
				for k := range models {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				parts = append(parts, "models: "+strings.Join(keys, ", "))
			}
			if len(profiles) > 0 {
				names := make([]string, 0, len(profiles))
				for n := range profiles {
					names = append(names, n)
				}
				sort.Strings(names)
				parts = append(parts, "profiles: "+strings.Join(names, ", "))
			}
			if len(parts) > 0 {
				fs = append(fs, finding{"ok", "project layer defines " + strings.Join(parts, "; ")})
			}
		}
	case st.Changed:
		fs = append(fs, finding{"warn", fmt.Sprintf("project config %s changed since it was trusted; launches will fail until `clau trust`", st.Path)})
	default:
		fs = append(fs, finding{"warn", fmt.Sprintf("project config %s is not trusted; launches will fail until `clau trust`", st.Path)})
	}
	paths := make([]string, 0, len(store))
	for p := range store {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			fs = append(fs, finding{"warn", fmt.Sprintf("trust entry for missing file %s", p)})
		}
	}
	return fs
}
```

In `cmdDoctor`, after the `clauPath` lookup and before the findings loop, gather the extra findings; replace the loop's source with the combined slice:

```go
	findings := doctorFindings(cfg, cfgErr, *dir, clauPath, runtime.GOOS)
	if cfgErr == nil {
		if cwd, err := os.Getwd(); err == nil {
			if _, st, effErr := loadEffectiveConfig(cwd, false); effErr != nil {
				findings = append(findings, finding{"fail", fmt.Sprintf("project config: %v", effErr)})
			} else {
				store, corrupt := loadTrust(trustPath())
				findings = append(findings, projectFindings(st, store, corrupt)...)
			}
		}
	}
	failed := false
	for _, f := range findings {
```

(The `effErr != nil` branch fires when a *trusted* project file no longer parses — that genuinely breaks launches, so it is a `fail`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS. Note `TestDoctorFindings`/`TestDoctorShadowWarningsSorted` still call `doctorFindings` directly and are unaffected.

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go
git commit -m "Report project config trust in clau doctor"
```

---

### Task 9: Documentation (README, version note)

**Files:**
- Modify: `README.md` (new section between “Profiles” and “Why not shell aliases?”; two rows in “Commands”)

**Interfaces:**
- Consumes: the behavior shipped in Tasks 1–8.
- Produces: user-facing docs. This feature anchors v0.2.0 (tagged by the maintainer, not by this plan).

- [ ] **Step 1: Add the README section**

Insert after the “Profiles” section (after the `c -- anything` paragraph):

```markdown
## Per-project config

A repo can carry its own `.clau.toml` — same format — and clau layers
it over your global config when you launch from inside the project:
`[models]` merge per key, a project profile replaces its global
namesake wholesale. Commit the file and the whole team shares the
project's launch shapes.

Because a repo file can set env and flags, nothing applies until you
allow it once:

    clau trust --show    # read what the project wants
    clau trust           # allow it (re-asks whenever the file changes)

Untrusted or changed files are a hard error at launch — never silently
applied. `clau list` and `clau doctor` still work untrusted and show
what would change. Project-only profiles stay argument-style
(`c deploy`); `clau link` links global tokens only, and a linked
global command like `crev` picks up the project's override
automatically. `CLAU_NO_PROJECT=1` skips the layer entirely.
```

- [ ] **Step 2: Extend the Commands table**

After the `clau init [--force]` row add:

```
    clau trust [--show]               allow (or print) the project .clau.toml
    clau untrust                      revoke that trust
```

- [ ] **Step 3: Verify docs match reality**

Run: `go run . help` and compare the Management block to the README's Commands section — every verb present in both.
Run: `go test ./...`
Expected: PASS (docs-only change; the suite guards against accidental code edits).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "Document per-project config in README"
```

---

## Final verification (after all tasks)

- [ ] `go test ./...` — full suite PASS.
- [ ] `gofmt -l .` — no output.
- [ ] Manual smoke (optional but recommended): in a scratch repo, write `.clau.toml` with a profile, run `clau list` (untrusted banner) → `clau trust --show` → `clau trust` → `clau list` (SOURCE column) → `c <profile>` → `clau doctor` → `clau untrust`.
