# Profile Inheritance via [bases] Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `[bases]` table of abstract profile fragments that profiles opt into with `extends`, so related profiles state shared env and flags once.

**Architecture:** Bases are parsed and layered exactly like profiles (wholesale per name) into a new `Config.Bases` map, but generate no commands or tokens. A profile's `extends` is resolved inside `applyConfigData` at the moment its declaring file is applied — flattened into a plain `Profile` and then forgotten, so `resolveToken`, linking, previews, launch, and trust code never see it (early binding). Spec: `docs/superpowers/specs/2026-08-07-profile-inheritance-design.md`, issue #1.

**Tech Stack:** Go stdlib + github.com/BurntSushi/toml (the only dependency). Plain `testing` tests.

## Global Constraints

- No new dependencies.
- Config error style is `<path>: <subject>: <detail>`, e.g. `fmt.Errorf("%s: profile %q: extends unknown base %q", path, name, parent)`. Match it exactly.
- `applyConfigData` must never mutate the maps of its `cfg` argument (documented invariant, tested by `TestApplyConfigFileDoesNotMutateBase`).
- Unknown TOML keys must remain load errors (the `md.Undecoded()` check is last in `applyConfigData`; do not bypass it).
- Flatten semantics (from the spec, fixed by interview): scalars (`model`, `effort`, `claude`) — child wins where set; `flags` — base's first, child's appended; `env` — key-wise, child wins per key. No unset mechanism.
- Bases never produce commands, tokens, list rows, or shadow warnings.
- Commit messages: sentence-case imperative, no prefix (house style, e.g. "Spec profile inheritance via an abstract [bases] table").
- Run `gofmt -l .` (expect no output) and `go test ./...` before every commit.
- README and starter-config prose hard-wraps at ~72 columns, matching surrounding text.

---

### Task 1: Parse and layer `[bases]`

`Config` gains a `Bases` map filled from a new `[bases]` TOML table, validated like profiles, replaced wholesale per name across layers, and inert otherwise.

**Files:**
- Modify: `config.go` (struct `Config` ~line 27, `defaultConfig` ~line 34, `rawConfig` ~line 84, `applyConfigData` ~lines 122–192)
- Test: `config_test.go`

**Interfaces:**
- Consumes: existing `Profile` struct, `cloneMap`, `profileNameRe`, `applyConfigFile(cfg Config, path string) (Config, error)`.
- Produces: `Config.Bases map[string]Profile` — populated, validated, layered. Task 2 reads `cfg.Bases[name]` during profile flattening. A base is stored as a plain `Profile` (it has the same five fields; there is no separate base type).

- [ ] **Step 1: Write the failing tests**

Add to `config_test.go`:

```go
func TestLoadConfigParsesBases(t *testing.T) {
	p := writeConfig(t, `
[bases.ollama]
flags = ["--bare", "--strict-mcp-config"]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434" }
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Bases["ollama"]
	if len(b.Flags) != 2 || b.Flags[0] != "--bare" {
		t.Errorf("flags = %v", b.Flags)
	}
	if b.Env["ANTHROPIC_BASE_URL"] != "http://ollama.lan:11434" {
		t.Errorf("env = %v", b.Env)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("bases must not create profiles: %v", cfg.Profiles)
	}
}

func TestApplyConfigFileReplacesBasesWholesale(t *testing.T) {
	base, err := loadConfig(writeConfig(t, `
[bases.ollama]
model = "gemma4:12b"
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434" }
`))
	if err != nil {
		t.Fatal(err)
	}
	project := writeConfig(t, `
[bases.ollama]
env = { ANTHROPIC_BASE_URL = "http://team-gpu-box:11434" }
`)
	layered, err := applyConfigFile(base, project)
	if err != nil {
		t.Fatal(err)
	}
	b := layered.Bases["ollama"]
	if b.Env["ANTHROPIC_BASE_URL"] != "http://team-gpu-box:11434" {
		t.Errorf("env = %v", b.Env)
	}
	if b.Model != "" {
		t.Errorf("wholesale replace must drop model, got %q", b.Model)
	}
	if base.Bases["ollama"].Model != "gemma4:12b" {
		t.Errorf("layering mutated the base config: %+v", base.Bases["ollama"])
	}
}
```

And two rows in the `TestLoadConfigRejectsBadInput` table (after the "bad profile name" row):

```go
		{"bad base name", "[bases.Bad_Name]\nmodel = \"x\"\n", "base name"},
		{"extends inside base", "[bases.b]\nextends = \"other\"\n", "extends"},
```

The second row needs no code of its own: `rawBase` (below) has no `Extends` field, so the key lands in `md.Undecoded()` and the existing unknown-key error fires.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestLoadConfigParsesBases|TestApplyConfigFileReplacesBasesWholesale|TestLoadConfigRejectsBadInput' ./...`
Expected: compile FAILURE — `cfg.Bases undefined (type Config has no field or method Bases)`.

- [ ] **Step 3: Implement**

In `config.go`, add `Bases` to `Config` (between `Efforts` and `Profiles`):

```go
type Config struct {
	Claude   string
	Models   map[string]ModelSpec
	Efforts  map[string]string
	Bases    map[string]Profile
	Profiles map[string]Profile
}
```

In `defaultConfig()`, add alongside `Profiles`:

```go
		Bases:    map[string]Profile{},
		Profiles: map[string]Profile{},
```

Add `rawBase` next to `rawProfile` — the same five fields, deliberately without `extends` so a base carrying one is an unknown-key error:

```go
// rawBase is rawProfile without extends: bases cannot chain, and the
// missing field makes `extends` inside [bases.*] an unknown-key error.
type rawBase struct {
	Model  string            `toml:"model"`
	Effort string            `toml:"effort"`
	Flags  []string          `toml:"flags"`
	Env    map[string]string `toml:"env"`
	Claude string            `toml:"claude"`
}
```

Add to `rawConfig`:

```go
	Bases    map[string]rawBase        `toml:"bases"`
```

In `applyConfigData`, clone the map next to the existing clones:

```go
	cfg.Models = cloneMap(cfg.Models)
	cfg.Bases = cloneMap(cfg.Bases)
	cfg.Profiles = cloneMap(cfg.Profiles)
```

And insert the bases loop after the `[efforts]` block, **before** the profiles loop (Task 2 depends on this ordering — a file's bases must be merged before its profiles flatten):

```go
	for name, rb := range raw.Bases {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid base name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		cfg.Bases[name] = Profile{
			Model: rb.Model, Effort: rb.Effort, Flags: rb.Flags,
			Env: rb.Env, Claude: rb.Claude,
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestLoadConfigParsesBases|TestApplyConfigFileReplacesBasesWholesale|TestLoadConfigRejectsBadInput' ./...`
Expected: PASS.

- [ ] **Step 5: Full suite and commit**

```bash
gofmt -l .          # expect no output
go test ./...       # expect all PASS
git add config.go config_test.go
git commit -m "Parse and layer an abstract [bases] table"
```

---

### Task 2: Flatten `extends` at file-apply time

A profile's `extends = "<base>"` merges the named base into the profile inside `applyConfigData`, then disappears — the stored `Profile` is plain. Unknown, empty, and profile-naming `extends` are load errors.

**Files:**
- Modify: `config.go` (`rawProfile` ~line 76, profiles loop in `applyConfigData` ~line 174, new function `flattenProfile`)
- Test: `config_test.go`

**Interfaces:**
- Consumes: `Config.Bases map[string]Profile` from Task 1 (merged before the profiles loop runs).
- Produces: `flattenProfile(base, child Profile) Profile` (pure function; fresh flag slice and env map, never mutates either argument) and `rawProfile.Extends *string` (`nil` = absent; pointer-to-empty = explicit `extends = ""`, an error — mirrors the existing `rawModel.Efforts *bool` pattern).

- [ ] **Step 1: Write the failing tests**

Add to `config_test.go`. Note `[profiles.child]` appears *before* `[bases.b]` in the second test — TOML decodes to maps and bases merge before profiles flatten, so textual order inside a file must not matter:

```go
func TestLoadConfigExtendsFlattens(t *testing.T) {
	p := writeConfig(t, `
[bases.ollama]
effort = "low"
flags = ["--bare", "--strict-mcp-config"]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434", ANTHROPIC_AUTH_TOKEN = "ollama" }

[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
effort = "high"
flags = ["-p"]
env = { ANTHROPIC_AUTH_TOKEN = "other", ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6:27b" }
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	q := cfg.Profiles["q27"]
	if q.Model != "qwen3.6:27b" {
		t.Errorf("model = %q", q.Model)
	}
	if q.Effort != "high" {
		t.Errorf("child effort must win, got %q", q.Effort)
	}
	want := "--bare --strict-mcp-config -p"
	if got := strings.Join(q.Flags, " "); got != want {
		t.Errorf("flags = %q, want %q (base first, child appended)", got, want)
	}
	if q.Env["ANTHROPIC_BASE_URL"] != "http://ollama.lan:11434" {
		t.Errorf("base env key lost: %v", q.Env)
	}
	if q.Env["ANTHROPIC_AUTH_TOKEN"] != "other" {
		t.Errorf("child env key must win per key: %v", q.Env)
	}
	if q.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "qwen3.6:27b" {
		t.Errorf("child-only env key lost: %v", q.Env)
	}
}

func TestLoadConfigExtendsScalarFillIn(t *testing.T) {
	p := writeConfig(t, `
[profiles.child]
extends = "b"

[bases.b]
model = "gemma4:12b"
effort = "low"
claude = "claude-alt"
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Profiles["child"]
	if c.Model != "gemma4:12b" || c.Effort != "low" || c.Claude != "claude-alt" {
		t.Errorf("child = %+v, want base scalars filled in", c)
	}
}

func TestLoadConfigExtendsDoesNotMutateBase(t *testing.T) {
	p := writeConfig(t, `
[bases.b]
flags = ["--bare"]
env = { A = "1" }

[profiles.x]
extends = "b"
flags = ["-p"]
env = { A = "2", B = "3" }
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Bases["b"]
	if len(b.Env) != 1 || b.Env["A"] != "1" || len(b.Flags) != 1 {
		t.Errorf("flatten mutated the stored base: %+v", b)
	}
}
```

And three rows in the `TestLoadConfigRejectsBadInput` table:

```go
		{"empty extends", "[profiles.p]\nextends = \"\"\n", "empty extends"},
		{"unknown base", "[profiles.p]\nextends = \"nope\"\n", "unknown base"},
		{"extends a profile", "[profiles.rev]\nmodel = \"opus\"\n\n[profiles.p]\nextends = \"rev\"\n", "references [bases]"},
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestLoadConfigExtends|TestLoadConfigRejectsBadInput' ./...`
Expected: FAIL — `TestLoadConfigExtendsFlattens` etc. error with `unknown key(s): [profiles.q27.extends ...]` (the field doesn't exist yet), and the three new error rows fail with the same unknown-key message instead of the wanted ones.

- [ ] **Step 3: Implement**

In `config.go`, add the field to `rawProfile`:

```go
type rawProfile struct {
	Model   string            `toml:"model"`
	Effort  string            `toml:"effort"`
	Flags   []string          `toml:"flags"`
	Env     map[string]string `toml:"env"`
	Claude  string            `toml:"claude"`
	Extends *string           `toml:"extends"`
}
```

Add `flattenProfile` below `cloneMap`:

```go
// flattenProfile fills child fields from base where the child leaves
// them unset: scalars fill in, flags concatenate base-first, env merges
// key-wise with the child winning. The result shares no slice or map
// storage with base, so later layers cannot mutate stored bases.
func flattenProfile(base, child Profile) Profile {
	out := child
	if out.Model == "" {
		out.Model = base.Model
	}
	if out.Effort == "" {
		out.Effort = base.Effort
	}
	if out.Claude == "" {
		out.Claude = base.Claude
	}
	if len(base.Flags) > 0 {
		out.Flags = append(append([]string{}, base.Flags...), child.Flags...)
	}
	if len(base.Env) > 0 {
		env := cloneMap(base.Env)
		for k, v := range child.Env {
			env[k] = v
		}
		out.Env = env
	}
	return out
}
```

Replace the body of the profiles loop in `applyConfigData` (currently the plain `cfg.Profiles[name] = Profile{...}` assignment) with:

```go
	for name, rp := range raw.Profiles {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid profile name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		p := Profile{
			Model: rp.Model, Effort: rp.Effort, Flags: rp.Flags,
			Env: rp.Env, Claude: rp.Claude,
		}
		if rp.Extends != nil {
			parent := *rp.Extends
			if parent == "" {
				return Config{}, fmt.Errorf("%s: profile %q: empty extends", path, name)
			}
			base, ok := cfg.Bases[parent]
			if !ok {
				_, inFile := raw.Profiles[parent]
				_, inCfg := cfg.Profiles[parent]
				if inFile || inCfg {
					return Config{}, fmt.Errorf("%s: profile %q: extends %q, which is a profile; extends references [bases]", path, name, parent)
				}
				return Config{}, fmt.Errorf("%s: profile %q: extends unknown base %q", path, name, parent)
			}
			p = flattenProfile(base, p)
		}
		cfg.Profiles[name] = p
	}
```

(The profile-hint check reads `raw.Profiles` — this file, order-independent — and `cfg.Profiles` — earlier layers. Mid-loop entries from this file can only add redundant hits with the identical message, so map iteration order never changes the output.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestLoadConfigExtends|TestLoadConfigRejectsBadInput' ./...`
Expected: PASS.

- [ ] **Step 5: Full suite and commit**

```bash
gofmt -l .          # expect no output
go test ./...       # expect all PASS
git add config.go config_test.go
git commit -m "Flatten profile extends against [bases] at file-apply time"
```

---

### Task 3: Lock in cross-layer and linking guarantees

Early binding and base invisibility are consequences of Tasks 1–2; this task encodes the spec's guarantees as regression tests. **These tests are expected to pass on first run** — a failure means Task 1 or 2 is wrong; stop and fix there.

**Files:**
- Test: `config_test.go`, `link_test.go`

**Interfaces:**
- Consumes: `applyConfigFile`, `loadConfig`, `linkNames`, `defaultConfig` — all existing; `Config.Bases` from Task 1.
- Produces: nothing new — behavioral guarantees only.

- [ ] **Step 1: Write the cross-layer tests**

Add to `config_test.go`:

```go
func TestApplyConfigFileProfileExtendsGlobalBase(t *testing.T) {
	base, err := loadConfig(writeConfig(t, `
[bases.ollama]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434" }
`))
	if err != nil {
		t.Fatal(err)
	}
	project := writeConfig(t, `
[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
`)
	layered, err := applyConfigFile(base, project)
	if err != nil {
		t.Fatal(err)
	}
	q := layered.Profiles["q27"]
	if q.Env["ANTHROPIC_BASE_URL"] != "http://ollama.lan:11434" {
		t.Errorf("project profile must flatten against the global base, got %+v", q)
	}
}

func TestApplyConfigFileBaseOverrideScopedToDeclaringFile(t *testing.T) {
	base, err := loadConfig(writeConfig(t, `
[bases.ollama]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434" }

[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
`))
	if err != nil {
		t.Fatal(err)
	}
	project := writeConfig(t, `
[bases.ollama]
env = { ANTHROPIC_BASE_URL = "http://team-gpu-box:11434" }

[profiles.team]
extends = "ollama"
model = "qwen3.6:27b"
`)
	layered, err := applyConfigFile(base, project)
	if err != nil {
		t.Fatal(err)
	}
	if got := layered.Profiles["q27"].Env["ANTHROPIC_BASE_URL"]; got != "http://ollama.lan:11434" {
		t.Errorf("global child rerouted by a project base override (early binding broken): %q", got)
	}
	if got := layered.Profiles["team"].Env["ANTHROPIC_BASE_URL"]; got != "http://team-gpu-box:11434" {
		t.Errorf("project child must see the project's base: %q", got)
	}
}
```

Add to `link_test.go`:

```go
func TestLinkNamesSkipBases(t *testing.T) {
	cfg := defaultConfig()
	cfg.Bases = map[string]Profile{"ollama": {Flags: []string{"--bare"}}}
	cfg.Profiles = map[string]Profile{"q27": {Model: "qwen3.6:27b"}}
	var sawProfile bool
	for _, n := range linkNames(cfg) {
		if n == "collama" {
			t.Fatal("a base must not produce a command")
		}
		if n == "cq27" {
			sawProfile = true
		}
	}
	if !sawProfile {
		t.Error("profile command cq27 missing")
	}
}
```

- [ ] **Step 2: Run them — expected PASS on first run**

Run: `go test -run 'TestApplyConfigFileProfileExtendsGlobalBase|TestApplyConfigFileBaseOverrideScopedToDeclaringFile|TestLinkNamesSkipBases' ./...`
Expected: PASS. If any fails, the defect is in Task 1 or 2 — fix it there before continuing.

- [ ] **Step 3: Full suite and commit**

```bash
gofmt -l .          # expect no output
go test ./...       # expect all PASS
git add config_test.go link_test.go
git commit -m "Lock in early-binding and no-link guarantees for bases"
```

---

### Task 4: Integration coverage

End-to-end through the real binary and fake claude: inherited env and flags reach the launched process, and a trusted project profile inherits from a global base.

**Files:**
- Test: `integration_test.go`

**Interfaces:**
- Consumes: existing helpers `buildClau(t)`, `runShortcut(t, bin, invokeAs, configTOML, args...)` → `recorded{argv, env}`, `writeProject(t, body)` → `(dir, proj)`, `seedTrust(t, state, proj)`, `runInProject(t, bin, dir, globalTOML, state, extraEnv, args...)` → `(out, err, recorded)`.
- Produces: nothing new — coverage only.

- [ ] **Step 1: Write the tests**

Add to `integration_test.go`:

```go
func TestIntegrationExtendsReachesLaunch(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "clau", `
[bases.ollama]
flags = ["--bare"]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434", ANTHROPIC_AUTH_TOKEN = "ollama" }

[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6:27b" }
`, "run", "q27")
	if got := strings.Join(rec.argv, "\t"); got != "--model\tqwen3.6:27b\t--bare" {
		t.Errorf("argv = %q, want model then inherited flags", got)
	}
	if rec.env["ANTHROPIC_BASE_URL"] != "http://ollama.lan:11434" {
		t.Errorf("inherited env missing: ANTHROPIC_BASE_URL = %q", rec.env["ANTHROPIC_BASE_URL"])
	}
	if rec.env["ANTHROPIC_AUTH_TOKEN"] != "ollama" {
		t.Errorf("inherited env missing: ANTHROPIC_AUTH_TOKEN = %q", rec.env["ANTHROPIC_AUTH_TOKEN"])
	}
	if rec.env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "qwen3.6:27b" {
		t.Errorf("child env missing: ANTHROPIC_DEFAULT_HAIKU_MODEL = %q", rec.env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
}

func TestIntegrationProjectProfileExtendsGlobalBase(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, "[profiles.team]\nextends = \"ollama\"\nmodel = \"qwen3.6:27b\"\n")
	state := t.TempDir()
	seedTrust(t, state, proj)
	global := "[bases.ollama]\nenv = { ANTHROPIC_BASE_URL = \"http://ollama.lan:11434\" }\n"
	out, err, rec := runInProject(t, bin, dir, global, state, nil, "team")
	if err != nil {
		t.Fatalf("launch failed: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\tqwen3.6:27b" {
		t.Errorf("argv = %q", got)
	}
	if rec.env["ANTHROPIC_BASE_URL"] != "http://ollama.lan:11434" {
		t.Errorf("project profile must inherit the global base env, got %q", rec.env["ANTHROPIC_BASE_URL"])
	}
}
```

- [ ] **Step 2: Run them — expected PASS on first run**

Run: `go test -run 'TestIntegrationExtendsReachesLaunch|TestIntegrationProjectProfileExtendsGlobalBase' ./...`
Expected: PASS (these exercise Tasks 1–2 through the built binary; a failure means a wiring defect there, not here).

- [ ] **Step 3: Full suite and commit**

```bash
gofmt -l .          # expect no output
go test ./...       # expect all PASS
git add integration_test.go
git commit -m "Cover [bases] inheritance end to end"
```

---

### Task 5: Starter config and README

Document the feature where users meet it: rework the starter config's Ollama example into a base plus two children (the motivating case), and update the README's Profiles section and merge rules.

**Files:**
- Modify: `commands.go` (`starterConfig` ~lines 200–210), `commands_test.go` (`TestStarterConfigIsValidAndInert` ~line 104), `README.md` (Profiles section ~line 79, Per-project config ~line 115, Config ~line 151)

**Interfaces:**
- Consumes: the flatten semantics wording fixed in the spec (env merges per key; flags concatenate base-first; scalars fill in where unset).
- Produces: docs only.

- [ ] **Step 1: Extend the starter-config inertness test**

In `TestStarterConfigIsValidAndInert` (commands_test.go), alongside the existing "no active profiles" assertion, add:

```go
	if len(cfg.Bases) != 0 {
		t.Errorf("starter config defines active bases: %v (examples must be commented out)", cfg.Bases)
	}
```

Run: `go test -run TestStarterConfigIsValidAndInert ./...` — expected PASS (guards the next step).

- [ ] **Step 2: Rework the starter config example**

In `commands.go` `starterConfig`, replace this block:

```
# Local backend — anything speaking the Anthropic Messages API works
# (Ollama, LM Studio, LiteLLM): https://code.claude.com/docs/en/llm-gateway
# BASE_URL carries no /v1 suffix; the auth token is a throwaway the
# server ignores, Claude Code just needs one set; DEFAULT_HAIKU_MODEL keeps
# background tasks on the local model too.
#[profiles.gem]
#model = "gemma4:12b"
#[profiles.gem.env]
#ANTHROPIC_BASE_URL = "http://localhost:11434"
#ANTHROPIC_AUTH_TOKEN = "ollama"
#ANTHROPIC_DEFAULT_HAIKU_MODEL = "gemma4:12b"
```

with:

```
# Local backend — anything speaking the Anthropic Messages API works
# (Ollama, LM Studio, LiteLLM): https://code.claude.com/docs/en/llm-gateway
# Shared backend settings live in [bases]; profiles opt in with
# `extends`. A base is never a command itself. env merges per key,
# flags concatenate (base's first), model/effort/claude fill in where
# the profile leaves them unset.
# BASE_URL carries no /v1 suffix; the auth token is a throwaway the
# server ignores, Claude Code just needs one set; DEFAULT_HAIKU_MODEL keeps
# background tasks on the local model too.
#[bases.ollama]
#flags = ["--bare", "--strict-mcp-config"]  # small models drown in big context
#[bases.ollama.env]
#ANTHROPIC_BASE_URL = "http://localhost:11434"
#ANTHROPIC_AUTH_TOKEN = "ollama"

#[profiles.gem]
#extends = "ollama"
#model = "gemma4:12b"
#env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "gemma4:12b" }

#[profiles.qwen]
#extends = "ollama"
#model = "qwen3.6:27b"
#env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6:27b" }
```

Run: `go test -run TestStarterConfigIsValidAndInert ./...` — expected PASS.

- [ ] **Step 3: Update the README**

Three edits, prose hard-wrapped like the surrounding text:

**(a) Profiles section** — after the "Your own flags always win…" paragraph (line ~79), insert:

````markdown
When several profiles share a backend, put the shared parts in a
base and point each profile at it with `extends` — a base is never a
command itself:

```toml
[bases.ollama]
flags = ["--bare", "--strict-mcp-config"]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434", ANTHROPIC_AUTH_TOKEN = "ollama" }

[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6:27b" }

[profiles.g12]
extends = "ollama"
model = "gemma4:12b"
env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "gemma4:12b" }
```

`env` merges per key and `flags` concatenate (base's first); `model`,
`effort`, and `claude` fill in only where the profile leaves them
unset. Change the host once and every profile built on it follows.
````

**(b) Per-project config section** — replace the sentence (line ~115):

> `[models]` merge per key, a project profile or `[efforts]` table
> replaces its global namesake wholesale.

with:

> `[models]` merge per key, a project profile, base, or `[efforts]`
> table replaces its global namesake wholesale. A project profile may
> extend a global base, but inheritance resolves when each file is
> read — overriding a base in a project file only affects profiles
> that file itself declares.

**(c) Config section** — in the summary sentence (line ~151), change
"profiles are all yours (none are built in)" to
"profiles and bases are all yours (none are built in)".

- [ ] **Step 4: Full suite and commit**

```bash
gofmt -l .          # expect no output
go test ./...       # expect all PASS
git add commands.go commands_test.go README.md
git commit -m "Document [bases] inheritance in the starter config and README"
```
