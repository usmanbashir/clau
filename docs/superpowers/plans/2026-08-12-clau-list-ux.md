# clau list UX redesign + clau show — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat `clau list` table with a grouped, recall-first, minimally colored view, and add `clau show <token>` for per-token detail.

**Architecture:** Config loading records which `[bases]` entry each profile extended (`Profile.Extends`, display-only). A tiny hand-rolled ANSI palette (new `style.go`) styles two pure renderers in `commands.go` — `listView` (sectioned summary) and `showView` (one token, everything) — both driven by a shared `viewContext` built once per command. `cmdList` keeps `--tokens` and `--dir` untouched; `cmdShow` is a new management verb wired through `reservedVerbs`, help, and completions.

**Tech Stack:** Go 1.22, stdlib + BurntSushi/toml only. Tests are plain `go test` table/unit tests matching the existing pure-function style.

**Spec:** `docs/superpowers/specs/2026-08-12-clau-list-ux-design.md` — normative for all output shapes.

## Global Constraints

- No new dependencies: `go.mod` stays `github.com/BurntSushi/toml v1.6.0` only.
- `clau list --tokens` output must stay byte-identical (completions and scripts consume it).
- Project trust wording stays exactly: `(trusted)`, `(changed, NOT applied — run ` + backtick + `clau trust` + backtick + ` to re-allow)`, `(NOT trusted, NOT applied — run ` + backtick + `clau trust` + backtick + `)` — color only wraps these strings.
- Piped/`NO_COLOR`/`TERM=dumb` output = same layout with zero escape codes (palette fields all empty).
- UTF-8 literals in human output: `·` separator, `→` arrow, `•••` mask.
- Errors go through `fatal()` (stderr, exit 1). No new exit codes.
- Commit style: short imperative sentence, no prefix convention (match `git log`).
- After every task: `go build ./...` and `go test ./...` green, `gofmt -l .` empty.

---

### Task 1: Record `extends` provenance on Profile

**Files:**
- Modify: `config.go:19-25` (Profile struct), `config.go:250` (flatten site)
- Test: `config_test.go`

**Interfaces:**
- Consumes: existing `flattenProfile`, `applyConfigData`.
- Produces: `Profile.Extends string` — `""` for profiles without `extends` and for all `Config.Bases` values; the base name for flattened profiles. Later tasks group by this field.

- [ ] **Step 1: Write the failing tests** — append to `config_test.go`:

```go
func TestLoadConfigRecordsExtendsProvenance(t *testing.T) {
	p := writeConfig(t, `
[bases.ollama]
flags = ["--bare"]

[profiles.gem]
extends = "ollama"
model = "gemma4:12b"

[profiles.solo]
model = "opus"
`)
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["gem"].Extends; got != "ollama" {
		t.Errorf("gem.Extends = %q, want \"ollama\"", got)
	}
	if got := cfg.Profiles["solo"].Extends; got != "" {
		t.Errorf("solo.Extends = %q, want empty", got)
	}
	if got := cfg.Bases["ollama"].Extends; got != "" {
		t.Errorf("bases must never carry provenance, got %q", got)
	}
}

func TestLayeringResetsExtendsProvenance(t *testing.T) {
	global := writeConfig(t, `
[bases.ollama]
flags = ["--bare"]

[profiles.gem]
extends = "ollama"
model = "gemma4:12b"
`)
	cfg, err := loadConfig(global)
	if err != nil {
		t.Fatal(err)
	}
	// Project file replaces gem wholesale without extends: provenance clears.
	cfg, err = applyConfigData(cfg, "proj", []byte("[profiles.gem]\nmodel = \"other\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["gem"].Extends; got != "" {
		t.Errorf("wholesale replace must reset Extends, got %q", got)
	}
	// A project profile extending a global base records it.
	cfg, err = applyConfigData(cfg, "proj2", []byte("[profiles.p2]\nextends = \"ollama\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["p2"].Extends; got != "ollama" {
		t.Errorf("project profile extending global base: Extends = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestLoadConfigRecordsExtendsProvenance|TestLayeringResetsExtendsProvenance' ./...`
Expected: FAIL — `cfg.Profiles["gem"].Extends undefined (type Profile has no field or method Extends)` (compile error).

- [ ] **Step 3: Implement** — in `config.go`, extend the struct:

```go
type Profile struct {
	Model  string
	Effort string
	Flags  []string
	Env    map[string]string
	Claude string
	// Extends records the [bases] name this profile was flattened
	// against — display provenance only; resolution never reads it.
	Extends string
}
```

and at the flatten site in `applyConfigData` (currently `p = flattenProfile(base, p)`):

```go
			p = flattenProfile(base, p)
			p.Extends = parent
```

(`flattenProfile` copies the child, whose `Extends` is always zero at this point, so no other change is needed; `rawBase` has no extends field, so bases stay `""`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS (all packages).

- [ ] **Step 5: Commit**

```bash
git add config.go config_test.go
git commit -m "Record extends provenance on flattened profiles"
```

---

### Task 2: Palette, color decision, and text helpers

**Files:**
- Create: `style.go`
- Test: Create `style_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces (all used by Tasks 4-8):
  - `type palette struct { bold, dim, cyan, green, yellow, reset string }`
  - `func newPalette(enabled bool) palette` — zero-value fields when disabled
  - `func colorChoice(isTTY bool, goos string, getenv func(string) string) bool`
  - `func stdoutIsTTY() bool`
  - `func visibleLen(s string) int` — rune length ignoring ANSI escapes
  - `func padCell(s string, w int) string` — right-pad to visible width w
  - `func contractHome(p string) string` — `~`-contract a path for display

- [ ] **Step 1: Write the failing tests** — create `style_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPalette(t *testing.T) {
	if p := newPalette(false); p != (palette{}) {
		t.Errorf("disabled palette must be all-empty: %+v", p)
	}
	p := newPalette(true)
	if p.cyan != "\033[1;36m" || p.reset != "\033[0m" || p.dim != "\033[2m" ||
		p.bold != "\033[1m" || p.green != "\033[32m" || p.yellow != "\033[33m" {
		t.Errorf("palette codes wrong: %+v", p)
	}
}

func TestColorChoice(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		name string
		tty  bool
		goos string
		env  map[string]string
		want bool
	}{
		{"tty linux", true, "linux", nil, true},
		{"piped", false, "linux", nil, false},
		{"NO_COLOR", true, "linux", map[string]string{"NO_COLOR": "1"}, false},
		{"dumb term", true, "linux", map[string]string{"TERM": "dumb"}, false},
		{"windows bare conhost", true, "windows", nil, false},
		{"windows terminal", true, "windows", map[string]string{"WT_SESSION": "x"}, true},
		{"windows vscode", true, "windows", map[string]string{"TERM_PROGRAM": "vscode"}, true},
		{"windows with TERM", true, "windows", map[string]string{"TERM": "xterm"}, true},
	}
	for _, c := range cases {
		if got := colorChoice(c.tty, c.goos, env(c.env)); got != c.want {
			t.Errorf("%s: colorChoice = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVisibleLenAndPad(t *testing.T) {
	pal := newPalette(true)
	s := pal.cyan + "qw35f" + pal.reset
	if got := visibleLen(s); got != 5 {
		t.Errorf("visibleLen = %d, want 5", got)
	}
	if got := padCell(s, 8); visibleLen(got) != 8 || got != s+"   " {
		t.Errorf("padCell = %q", got)
	}
	if got := padCell("abc", 2); got != "abc" {
		t.Errorf("padCell must never truncate: %q", got)
	}
}

func TestContractHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := contractHome(filepath.Join(home, ".local", "bin")); got != filepath.Join("~", ".local", "bin") {
		t.Errorf("contractHome = %q", got)
	}
	if got := contractHome("/opt/x"); got != "/opt/x" {
		t.Errorf("non-home path must pass through: %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestNewPalette|TestColorChoice|TestVisibleLenAndPad|TestContractHome' ./...`
Expected: FAIL — `undefined: newPalette` (compile error).

- [ ] **Step 3: Implement** — create `style.go`:

```go
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
func contractHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add style.go style_test.go
git commit -m "Add the ANSI palette and display text helpers"
```

---

### Task 3: Digit-ladder helpers and grammar legend

**Files:**
- Modify: `commands.go` (add helpers near `tokensOf`; refactor `tokensOf` to use them)
- Test: `commands_test.go`

**Interfaces:**
- Consumes: `Config.Efforts`.
- Produces:
  - `func sortedKeys[V any](m map[string]V) []string`
  - `func digitSpan(digits []string) string` — `"1-5"` contiguous, `"1,3,7"` otherwise, `"3"` for one
  - `func effortsLegend(efforts map[string]string) string` — the MODELS header note

- [ ] **Step 1: Write the failing tests** — append to `commands_test.go`:

```go
func TestEffortsLegend(t *testing.T) {
	cases := []struct {
		name    string
		efforts map[string]string
		want    string
	}{
		{"default ladder", defaultConfig().Efforts,
			"letter alone = model, +digit 1-5 = effort low..max"},
		{"custom contiguous", map[string]string{"1": "minimal", "2": "peak"},
			"letter alone = model, +digit 1-2 = effort minimal..peak"},
		{"non-contiguous", map[string]string{"1": "low", "3": "high", "7": "max"},
			"letter alone = model, +digit 1,3,7 = effort low..max"},
		{"single digit", map[string]string{"3": "high"},
			"letter alone = model, +digit 3 = effort high"},
		{"empty", map[string]string{}, "letter alone = model"},
	}
	for _, c := range cases {
		if got := effortsLegend(c.efforts); got != c.want {
			t.Errorf("%s: legend = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDigitSpan(t *testing.T) {
	if got := digitSpan([]string{"1", "2", "3"}); got != "1-3" {
		t.Errorf("contiguous span = %q", got)
	}
	if got := digitSpan([]string{"1", "3"}); got != "1,3" {
		t.Errorf("gapped span = %q", got)
	}
	if got := digitSpan([]string{"4"}); got != "4" {
		t.Errorf("single span = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestEffortsLegend|TestDigitSpan' ./...`
Expected: FAIL — `undefined: effortsLegend`.

- [ ] **Step 3: Implement** — in `commands.go`, add above `tokensOf`:

```go
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// digitSpan renders sorted ladder digits compactly: "1-5" when
// contiguous, else "1,3,7".
func digitSpan(digits []string) string {
	contiguous := len(digits) > 1
	for i := 1; i < len(digits); i++ {
		if digits[i][0] != digits[i-1][0]+1 {
			contiguous = false
			break
		}
	}
	if contiguous {
		return digits[0] + "-" + digits[len(digits)-1]
	}
	return strings.Join(digits, ",")
}

// effortsLegend is the MODELS header note that teaches the grammar
// once, instead of one row per effort token.
func effortsLegend(efforts map[string]string) string {
	digits := sortedKeys(efforts)
	if len(digits) == 0 {
		return "letter alone = model"
	}
	if len(digits) == 1 {
		return fmt.Sprintf("letter alone = model, +digit %s = effort %s", digits[0], efforts[digits[0]])
	}
	return fmt.Sprintf("letter alone = model, +digit %s = effort %s..%s",
		digitSpan(digits), efforts[digits[0]], efforts[digits[len(digits)-1]])
}
```

Then refactor the digit-sorting block inside `tokensOf` — replace

```go
	digits := make([]string, 0, len(cfg.Efforts))
	for d := range cfg.Efforts {
		digits = append(digits, d)
	}
	sort.Strings(digits)
```

with

```go
	digits := sortedKeys(cfg.Efforts)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS — including the untouched `TestTokensOf` (behavior identical).

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go
git commit -m "Add the digit-span helpers and grammar legend"
```

---

### Task 4: viewContext and row builders

**Files:**
- Modify: `commands.go`
- Test: `commands_test.go`

**Interfaces:**
- Consumes: `palette` (Task 2), `Profile.Extends` (Task 1), existing `isOwned`, `linkFileName`.
- Produces (used by Tasks 5-8):

```go
type viewContext struct {
	cfg          Config
	st           ProjectStatus
	globalPath   string          // loaded global config path (show's source line)
	dir          string          // link dir being inspected
	clauPath     string          // this binary for isOwned ("" if unknown)
	goos         string
	projModels   map[string]bool // model letters the project file declares
	projProfiles map[string]bool // profile names the project file declares
	globalTokens map[string]bool // tokens `clau link` would create (global set)
	pal          palette
}
func (vc viewContext) linked(token string) bool
func joinBadges(pal palette, parts ...string) string   // dim " · " join, skips empties
func rowBadges(token string, project, linkable, linked bool, pal palette) string
func profileCell(p Profile, pal palette) string
func familyStates(letter string, spec ModelSpec, vc viewContext) (linkable, linked bool)
```

- [ ] **Step 1: Write the failing tests** — append to `commands_test.go`:

```go
func TestRowBadges(t *testing.T) {
	pal := palette{} // plain: rules readable as bytes
	cases := []struct {
		name                       string
		project, linkable, linked  bool
		want                       string
	}{
		{"linked global", false, true, true, ""},
		{"unlinked global", false, true, false, "not linked — run clau link"},
		{"project linked", true, true, true, "project"},
		{"project only unlinked", true, false, false, "project · c gma"},
		{"unlinkable unlinked", false, false, false, "c gma"},
	}
	for _, c := range cases {
		if got := rowBadges("gma", c.project, c.linkable, c.linked, pal); got != c.want {
			t.Errorf("%s: badges = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestProfileCell(t *testing.T) {
	pal := palette{}
	if got := profileCell(Profile{Model: "opus"}, pal); got != "opus" {
		t.Errorf("model cell = %q", got)
	}
	if got := profileCell(Profile{Model: "opus", Effort: "high"}, pal); got != "opus (high)" {
		t.Errorf("effort-pinned cell = %q", got)
	}
	if got := profileCell(Profile{Env: map[string]string{"A": "1"}}, pal); got != "(env/flags)" {
		t.Errorf("model-less cell = %q", got)
	}
}

func TestFamilyStates(t *testing.T) {
	cfg := defaultConfig()
	dir := t.TempDir()
	clauPath := "/nonexistent/clau"
	vc := viewContext{cfg: cfg, dir: dir, clauPath: clauPath, goos: "linux",
		globalTokens: map[string]bool{}}
	for _, tok := range tokensOf(cfg) {
		vc.globalTokens[tok] = true
	}

	// Nothing linked yet: linkable but not linked.
	linkable, linked := familyStates("o", cfg.Models["o"], vc)
	if !linkable || linked {
		t.Errorf("unlinked family: linkable=%v linked=%v, want true false", linkable, linked)
	}

	// Link the whole o family: co, co1..co5.
	for _, tok := range []string{"o", "o1", "o2", "o3", "o4", "o5"} {
		if err := os.Symlink(clauPath, filepath.Join(dir, "c"+tok)); err != nil {
			t.Fatal(err)
		}
	}
	linkable, linked = familyStates("o", cfg.Models["o"], vc)
	if !linkable || !linked {
		t.Errorf("linked family: linkable=%v linked=%v, want true true", linkable, linked)
	}

	// A project-declared letter is not linkable and not linked.
	vc2 := vc
	vc2.globalTokens = map[string]bool{}
	linkable, linked = familyStates("o", cfg.Models["o"], vc2)
	if linkable {
		t.Errorf("project family must not be linkable")
	}
	_ = linked // linked=true here (symlinks exist) is acceptable: badge shows nothing extra
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRowBadges|TestProfileCell|TestFamilyStates' ./...`
Expected: FAIL — `undefined: rowBadges`.

- [ ] **Step 3: Implement** — in `commands.go`, below `tokenSource`:

```go
// viewContext carries everything the list and show renderers need,
// assembled once per command so the renderers stay pure and testable.
type viewContext struct {
	cfg          Config
	st           ProjectStatus
	globalPath   string          // loaded global config path (show's source line)
	dir          string          // link dir being inspected
	clauPath     string          // this binary for isOwned ("" if unknown)
	goos         string
	projModels   map[string]bool // model letters the project file declares
	projProfiles map[string]bool // profile names the project file declares
	globalTokens map[string]bool // tokens `clau link` would create (global set)
	pal          palette
}

func (vc viewContext) linked(token string) bool {
	return isOwned(filepath.Join(vc.dir, linkFileName("c"+token, vc.goos)), vc.clauPath)
}

// joinBadges dim-joins non-empty annotation parts with " · ".
func joinBadges(pal palette, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, pal.dim+" · "+pal.reset)
}

// rowBadges renders the annotations after a row's model cell. Missing
// links are a yellow warning only when `clau link` could fix them;
// project-only tokens get the argument-style launch hint instead.
func rowBadges(token string, project, linkable, linked bool, pal palette) string {
	var parts []string
	if project {
		parts = append(parts, pal.dim+"project"+pal.reset)
	}
	if !linked {
		if linkable {
			parts = append(parts, pal.yellow+"not linked — run clau link"+pal.reset)
		} else {
			parts = append(parts, pal.dim+"c "+token+pal.reset)
		}
	}
	return joinBadges(pal, parts...)
}

// profileCell is the middle column of a profile row: the recall anchor.
func profileCell(p Profile, pal palette) string {
	switch {
	case p.Model == "":
		return pal.dim + "(env/flags)" + pal.reset
	case p.Effort != "":
		return p.Model + " " + pal.dim + "(" + p.Effort + ")" + pal.reset
	}
	return p.Model
}

// familyStates aggregates link state over a model letter's grammar
// family (the letter plus its effort digits): linkable when any family
// token is in the global set, linked when every family command exists.
func familyStates(letter string, spec ModelSpec, vc viewContext) (linkable, linked bool) {
	toks := []string{letter}
	if spec.Efforts {
		for _, d := range sortedKeys(vc.cfg.Efforts) {
			toks = append(toks, letter+d)
		}
	}
	linked = true
	for _, t := range toks {
		if vc.globalTokens[t] {
			linkable = true
		}
		if !vc.linked(t) {
			linked = false
		}
	}
	return linkable, linked
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go
git commit -m "Add the view context and list row builders"
```

---

### Task 5: listView — sections, project header, footer

**Files:**
- Modify: `commands.go`
- Test: `commands_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2-4.
- Produces (used by Task 6):

```go
type listRowV struct{ token, cell, badges string }
func renderSection(header string, rows []listRowV, pal palette) []string
func footerLine(vc viewContext) string
func listView(vc viewContext) []string   // complete output lines, no trailing newline handling
```

- [ ] **Step 1: Write the failing tests** — append to `commands_test.go`:

```go
// plainListVC builds a no-color viewContext over cfg with nothing linked.
func plainListVC(t *testing.T, cfg Config) viewContext {
	t.Helper()
	vc := viewContext{cfg: cfg, dir: t.TempDir(), clauPath: "/nonexistent/clau",
		goos: "linux", globalTokens: map[string]bool{}, globalPath: "/g/config.toml"}
	for _, tok := range tokensOf(cfg) {
		vc.globalTokens[tok] = true
	}
	return vc
}

func lineWith(t *testing.T, lines []string, sub string) string {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return l
		}
	}
	t.Fatalf("no line contains %q in:\n%s", sub, strings.Join(lines, "\n"))
	return ""
}

func lineIndex(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestRenderSectionAlignment(t *testing.T) {
	rows := []listRowV{
		{token: "g12", cell: "gemma4:12b"},
		{token: "qw35f", cell: "q", badges: "B"},
	}
	lines := renderSection("H", rows, palette{})
	want := []string{
		"H",
		"  g12    gemma4:12b",
		"  qw35f  q            B",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestListViewSectionsAndOrder(t *testing.T) {
	cfg := defaultConfig()
	cfg.Bases["ollama"] = Profile{Flags: []string{"--bare"}}
	cfg.Profiles["gem"] = Profile{Model: "gemma4:12b", Extends: "ollama"}
	cfg.Profiles["work"] = Profile{Env: map[string]string{"A": "1"}}
	lines := listView(plainListVC(t, cfg))

	models := lineIndex(lines, "MODELS")
	ollama := lineIndex(lines, "OLLAMA  base")
	profiles := lineIndex(lines, "PROFILES")
	footer := lineIndex(lines, "details: clau show <token>")
	if models == -1 || !(models < ollama && ollama < profiles && profiles < footer) {
		t.Fatalf("section order wrong (MODELS=%d OLLAMA=%d PROFILES=%d footer=%d):\n%s",
			models, ollama, profiles, footer, strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[models], "letter alone = model, +digit 1-5 = effort low..max") {
		t.Errorf("MODELS header missing legend: %q", lines[models])
	}
	if l := lineWith(t, lines, "haiku"); !strings.Contains(l, "no effort digits") {
		t.Errorf("h row missing efforts note: %q", l)
	}
	if l := lineWith(t, lines, "gem"); !strings.Contains(l, "gemma4:12b") {
		t.Errorf("gem row = %q", l)
	}
	if l := lineWith(t, lines, "work"); !strings.Contains(l, "(env/flags)") {
		t.Errorf("work row = %q", l)
	}
	if gem := lineIndex(lines, "gem"); !(ollama < gem && gem < profiles) {
		t.Errorf("gem must sit inside the OLLAMA section")
	}
}

func TestListViewProjectHeaderAndBadges(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["gma"] = Profile{Model: "gemma4:12b"}
	vc := plainListVC(t, cfg)
	delete(vc.globalTokens, "gma") // project-only profile
	vc.st = ProjectStatus{Path: "/repo/.clau.toml", Trusted: true, Applied: true}
	vc.projProfiles = map[string]bool{"gma": true}
	lines := listView(vc)

	if lines[0] != "project: /repo/.clau.toml (trusted)" {
		t.Errorf("header = %q", lines[0])
	}
	if l := lineWith(t, lines, "gma"); !strings.Contains(l, "project · c gma") {
		t.Errorf("gma badges = %q, want project · c gma", l)
	}
	if l := lineWith(t, lines, "opus"); !strings.Contains(l, "not linked — run clau link") {
		t.Errorf("unlinked model row must warn: %q", l)
	}
}

func TestListViewUntrustedHeader(t *testing.T) {
	cfg := defaultConfig()
	vc := plainListVC(t, cfg)
	vc.st = ProjectStatus{Path: "/repo/.clau.toml"}
	lines := listView(vc)
	if !strings.Contains(lines[0], "NOT trusted, NOT applied — run `clau trust`") {
		t.Errorf("untrusted header = %q", lines[0])
	}
}

func TestFooterLine(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["gma"] = Profile{Model: "g"}
	vc := plainListVC(t, cfg)
	delete(vc.globalTokens, "gma")

	// 19 global grammar tokens, none linked.
	if got := footerLine(vc); got != "0 of 19 commands linked in "+contractHome(vc.dir)+" — run clau link · 1 project-only · details: clau show <token>" {
		t.Errorf("unlinked footer = %q", got)
	}

	for tok := range vc.globalTokens {
		if err := os.Symlink(vc.clauPath, filepath.Join(vc.dir, "c"+tok)); err != nil {
			t.Fatal(err)
		}
	}
	if got := footerLine(vc); got != "all 19 commands linked in "+contractHome(vc.dir)+" · 1 project-only · details: clau show <token>" {
		t.Errorf("linked footer = %q", got)
	}
}

func TestListViewColorSmoke(t *testing.T) {
	vc := plainListVC(t, defaultConfig())
	vc.pal = newPalette(true)
	out := strings.Join(listView(vc), "\n")
	if !strings.Contains(out, "\033[1;36mo") {
		t.Errorf("tokens must be cyan when color is on:\n%q", out)
	}
	plain := strings.Join(listView(plainListVC(t, defaultConfig())), "\n")
	if strings.Contains(plain, "\033[") {
		t.Errorf("plain palette must emit no escapes:\n%q", plain)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRenderSection|TestListView|TestFooterLine' ./...`
Expected: FAIL — `undefined: listRowV`.

- [ ] **Step 3: Implement** — in `commands.go`, below the Task 4 builders:

```go
// listRowV is one row of the sectioned list: token, recall cell, badges.
type listRowV struct{ token, cell, badges string }

// renderSection aligns a section's rows by visible width (ANSI codes in
// cells make tabwriter miscount, so padding is manual).
func renderSection(header string, rows []listRowV, pal palette) []string {
	lines := []string{header}
	tokW, cellW := 0, 0
	for _, r := range rows {
		if n := visibleLen(r.token); n > tokW {
			tokW = n
		}
		if n := visibleLen(r.cell); n > cellW {
			cellW = n
		}
	}
	for _, r := range rows {
		line := "  " + padCell(pal.cyan+r.token+pal.reset, tokW) + "  " +
			padCell(r.cell, cellW) + "   " + r.badges
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

// footerLine summarizes link health: linkable commands are the global
// token set (what `clau link` manages); the rest are project-only.
func footerLine(vc viewContext) string {
	linkable, linked, projectOnly := 0, 0, 0
	for _, t := range tokensOf(vc.cfg) {
		if vc.globalTokens[t] {
			linkable++
			if vc.linked(t) {
				linked++
			}
		} else {
			projectOnly++
		}
	}
	dir := contractHome(vc.dir)
	var b strings.Builder
	if linked == linkable {
		b.WriteString(vc.pal.dim + fmt.Sprintf("all %d commands linked in %s", linkable, dir) + vc.pal.reset)
	} else {
		b.WriteString(vc.pal.yellow + fmt.Sprintf("%d of %d commands linked in %s — run clau link", linked, linkable, dir) + vc.pal.reset)
	}
	if projectOnly > 0 {
		b.WriteString(vc.pal.dim + fmt.Sprintf(" · %d project-only", projectOnly) + vc.pal.reset)
	}
	b.WriteString(vc.pal.dim + " · details: clau show <token>" + vc.pal.reset)
	return b.String()
}

// listView renders the whole sectioned list as lines.
func listView(vc viewContext) []string {
	pal := vc.pal
	var out []string

	if vc.st.Path != "" {
		state := pal.green + "(trusted)" + pal.reset
		switch {
		case vc.st.Changed:
			state = pal.bold + pal.yellow + "(changed, NOT applied — run `clau trust` to re-allow)" + pal.reset
		case !vc.st.Trusted:
			state = pal.bold + pal.yellow + "(NOT trusted, NOT applied — run `clau trust`)" + pal.reset
		}
		out = append(out, "project: "+vc.st.Path+" "+state, "")
	}

	var mrows []listRowV
	for _, letter := range sortedKeys(vc.cfg.Models) {
		spec := vc.cfg.Models[letter]
		linkable, linked := familyStates(letter, spec, vc)
		note := ""
		if !spec.Efforts {
			note = pal.dim + "no effort digits" + pal.reset
		}
		badges := joinBadges(pal, note,
			rowBadges(letter, vc.projModels[letter], linkable, linked, pal))
		mrows = append(mrows, listRowV{token: letter, cell: spec.Model, badges: badges})
	}
	header := pal.bold + "MODELS" + pal.reset + "    " +
		pal.dim + effortsLegend(vc.cfg.Efforts) + pal.reset
	out = append(out, renderSection(header, mrows, pal)...)

	groups := map[string][]string{}
	for name, p := range vc.cfg.Profiles {
		groups[p.Extends] = append(groups[p.Extends], name)
	}
	profileRow := func(name string) listRowV {
		p := vc.cfg.Profiles[name]
		return listRowV{
			token: name,
			cell:  profileCell(p, pal),
			badges: rowBadges(name, vc.projProfiles[name],
				vc.globalTokens[name], vc.linked(name), pal),
		}
	}
	for _, base := range sortedKeys(groups) {
		if base == "" {
			continue
		}
		sort.Strings(groups[base])
		var rows []listRowV
		for _, name := range groups[base] {
			rows = append(rows, profileRow(name))
		}
		header := pal.bold + strings.ToUpper(base) + pal.reset + "  " + pal.dim + "base" + pal.reset
		out = append(out, "")
		out = append(out, renderSection(header, rows, pal)...)
	}
	if names := groups[""]; len(names) > 0 {
		sort.Strings(names)
		var rows []listRowV
		for _, name := range names {
			rows = append(rows, profileRow(name))
		}
		out = append(out, "")
		out = append(out, renderSection(pal.bold+"PROFILES"+pal.reset, rows, pal)...)
	}

	out = append(out, "", footerLine(vc))
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go
git commit -m "Render the sectioned recall-first list view"
```

---

### Task 6: Wire cmdList to the sectioned view; delete the flat table

**Files:**
- Modify: `commands.go` (`cmdList`, delete `listRow`/`listRows`/`previewFor`, drop `text/tabwriter` import)
- Modify: `commands_test.go` (delete `TestListRows`, `TestListRowsSource`)
- Modify: `integration_test.go:409-438` (`TestIntegrationListShowsProvenance`)

**Interfaces:**
- Consumes: `listView`, `viewContext` (Task 5), `colorChoice`/`stdoutIsTTY`/`newPalette` (Task 2).
- Produces: `func newViewContext(cfg Config, st ProjectStatus, dir string) viewContext` — Task 8's `cmdShow` reuses it verbatim.

- [ ] **Step 1: Update the integration test first** — replace the body of `TestIntegrationListShowsProvenance` assertions (keep the setup lines) so the old-shape checks become new-shape checks:

```go
	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list untrusted: %v\n%s", err, out)
	}
	// Untrusted: loud banner, global shape only — no project badge anywhere.
	if !strings.Contains(out, "NOT trusted") || strings.Contains(out, "project ·") {
		t.Errorf("untrusted list must warn and stay in the global shape:\n%s", out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "opus") {
		t.Errorf("untrusted rev row must show the global model:\n%s", out)
	}

	seedTrust(t, state, proj)
	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list trusted: %v\n%s", err, out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "haiku") || !strings.Contains(line, "project") {
		t.Errorf("trusted rev row must show the project override and badge:\n%s", out)
	}
	if !strings.Contains(out, "MODELS") {
		t.Errorf("list must render the sectioned view:\n%s", out)
	}
```

(Adjust only assertion lines; if the old test asserted `SOURCE`, that string no longer exists.)

- [ ] **Step 2: Rewrite `cmdList` and add `newViewContext`** — replace `cmdList` (and delete `listRow`, `listRows`, `previewFor`) in `commands.go`:

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
	for _, line := range listView(newViewContext(cfg, st, *dir)) {
		fmt.Println(line)
	}
}

// newViewContext assembles the renderer inputs from the loaded config
// and the live environment (TTY, links, project declarations).
func newViewContext(cfg Config, st ProjectStatus, dir string) viewContext {
	vc := viewContext{
		cfg: cfg, st: st, globalPath: configPath(), dir: dir, goos: runtime.GOOS,
		pal: newPalette(colorChoice(stdoutIsTTY(), runtime.GOOS, os.Getenv)),
	}
	if p, err := clauExecutable(); err == nil {
		vc.clauPath = p
	}
	globalCfg := cfg
	if st.Applied {
		if g, err := loadConfig(configPath()); err == nil {
			globalCfg = g
		}
		vc.projModels, vc.projProfiles, _ = projectDeclarations(st.Path)
	}
	vc.globalTokens = map[string]bool{}
	for _, t := range tokensOf(globalCfg) {
		vc.globalTokens[t] = true
	}
	return vc
}
```

Also delete `TestListRows` and `TestListRowsSource` from `commands_test.go` (superseded by the Task 5 listView tests), and remove the now-unused `text/tabwriter` import (nothing else in the package uses it — verify with `grep -rn tabwriter *.go`).

- [ ] **Step 3: Build and run everything**

Run: `go build ./... && go test ./...`
Expected: PASS, no unused-import or undefined errors. `go vet ./...` clean.

- [ ] **Step 4: Manual smoke check**

Run: `go run . list | head -30` (piped → plain) and `go run . list --tokens | head -5`.
Expected: sectioned plain view; `--tokens` output unchanged (one bare token per line, sorted).

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go integration_test.go
git commit -m "Replace the flat list table with the sectioned view"
```

---

### Task 7: showView — per-token detail lines

**Files:**
- Modify: `commands.go`
- Test: `commands_test.go`

**Interfaces:**
- Consumes: `viewContext` (Task 4), `digitSpan`/`sortedKeys` (Task 3), `contractHome` (Task 2), the `plainListVC` test helper (Task 5's test code), existing `resolveToken`, `tokenSource`.
- Produces (used by Task 8):

```go
func showView(vc viewContext, arg string) ([]string, error)
func kindLine(vc viewContext, token string) string
func maskedEnvName(name string) bool
func uniquePrefixMatch(cfg Config, arg string) string
```

- [ ] **Step 1: Write the failing tests** — append to `commands_test.go`:

```go
func TestMaskedEnvName(t *testing.T) {
	cases := map[string]bool{
		"ANTHROPIC_AUTH_TOKEN":            true,
		"OPENAI_API_KEY":                  true,
		"MY_SECRET":                       true,
		"DB_PASSWORD":                     true,
		"anthropic_auth_token":            true,
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS":  false, // segment is TOKENS, not TOKEN
		"ANTHROPIC_BASE_URL":              false,
		"MONKEYS":                         false,
	}
	for name, want := range cases {
		if got := maskedEnvName(name); got != want {
			t.Errorf("maskedEnvName(%q) = %v, want %v", name, got, want)
		}
	}
}

// showVC builds a plain viewContext with one flattened ollama profile.
func showVC(t *testing.T) viewContext {
	t.Helper()
	cfg := defaultConfig()
	cfg.Bases["ollama"] = Profile{Flags: []string{"--bare"}}
	cfg.Profiles["gem"] = Profile{
		Model: "gemma4:12b", Extends: "ollama",
		Flags: []string{"--bare", "--strict-mcp-config"},
		Env: map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "ollama",
			"ANTHROPIC_BASE_URL":   "http://localhost:11434",
		},
	}
	return plainListVC(t, cfg)
}

func TestShowViewProfile(t *testing.T) {
	lines, err := showView(showVC(t), "gem")
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	for _, want := range []string{
		"gem → cgem",
		"kind      profile · extends base: ollama",
		"source    global (" + contractHome("/g/config.toml") + ")",
		"linked    no",
		"launches  claude --model gemma4:12b --bare --strict-mcp-config",
		"ANTHROPIC_AUTH_TOKEN=•••",
		"ANTHROPIC_BASE_URL=http://localhost:11434",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ANTHROPIC_AUTH_TOKEN=ollama") {
		t.Errorf("auth token value leaked:\n%s", out)
	}
}

func TestShowViewKinds(t *testing.T) {
	vc := showVC(t)
	cases := map[string]string{
		"o":  "model letter: opus · effort digits 1-5",
		"h":  "model letter: haiku · no effort digits",
		"o3": "grammar: o (opus) + 3 (effort high)",
	}
	for token, want := range cases {
		lines, err := showView(vc, token)
		if err != nil {
			t.Fatalf("%s: %v", token, err)
		}
		if out := strings.Join(lines, "\n"); !strings.Contains(out, want) {
			t.Errorf("%s: missing %q in:\n%s", token, want, out)
		}
	}
}

func TestShowViewFallbacksAndErrors(t *testing.T) {
	vc := showVC(t)

	lines, err := showView(vc, "cgem") // command-name fallback
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "gem → cgem") {
		t.Errorf("c-prefix fallback failed:\n%s", strings.Join(lines, "\n"))
	}

	if _, err := showView(vc, "zz"); err == nil ||
		!strings.Contains(err.Error(), `unknown token "zz"`) ||
		!strings.Contains(err.Error(), "clau list") {
		t.Errorf("unknown-token error = %v", err)
	}

	if _, err := showView(vc, "ge"); err == nil ||
		!strings.Contains(err.Error(), `did you mean "gem"?`) {
		t.Errorf("did-you-mean missing: %v", err)
	}

	if _, err := showView(vc, "h3"); err == nil ||
		!strings.Contains(err.Error(), "supports no effort levels") {
		t.Errorf("resolver error must surface: %v", err)
	}
}

func TestShowViewProjectSourceAndLinked(t *testing.T) {
	vc := showVC(t)
	vc.st = ProjectStatus{Path: "/repo/.clau.toml", Trusted: true, Applied: true}
	vc.projProfiles = map[string]bool{"gem": true}
	delete(vc.globalTokens, "gem")
	lines, err := showView(vc, "gem")
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "source    project (/repo/.clau.toml)") {
		t.Errorf("project source missing:\n%s", out)
	}
	if !strings.Contains(out, "project profiles launch as c gem") {
		t.Errorf("project-only linked hint missing:\n%s", out)
	}

	if err := os.Symlink(vc.clauPath, filepath.Join(vc.dir, "cgem")); err != nil {
		t.Fatal(err)
	}
	lines, err = showView(vc, "gem")
	if err != nil {
		t.Fatal(err)
	}
	if out := strings.Join(lines, "\n"); !strings.Contains(out, "linked    yes → ") {
		t.Errorf("linked-yes line missing:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestMaskedEnvName|TestShowView' ./...`
Expected: FAIL — `undefined: maskedEnvName`.

- [ ] **Step 3: Implement** — in `commands.go`, below `listView`:

```go
// maskedEnvName reports whether an env var's value must render masked:
// any _-separated segment equal to TOKEN, KEY, SECRET, or PASSWORD.
// Plural segments (TOKENS) stay visible — they are sizes, not secrets.
func maskedEnvName(name string) bool {
	for _, seg := range strings.Split(strings.ToUpper(name), "_") {
		switch seg {
		case "TOKEN", "KEY", "SECRET", "PASSWORD":
			return true
		}
	}
	return false
}

// uniquePrefixMatch returns the single token arg prefixes, or "".
func uniquePrefixMatch(cfg Config, arg string) string {
	match := ""
	for _, t := range tokensOf(cfg) {
		if strings.HasPrefix(t, arg) {
			if match != "" {
				return ""
			}
			match = t
		}
	}
	return match
}

// kindLine explains what shape of shortcut a resolvable token is.
func kindLine(vc viewContext, token string) string {
	cfg := vc.cfg
	if p, ok := cfg.Profiles[token]; ok {
		if p.Extends != "" {
			return "profile " + vc.pal.dim + "· extends base: " + p.Extends + vc.pal.reset
		}
		return "profile"
	}
	if spec, ok := cfg.Models[token]; ok {
		if !spec.Efforts {
			return fmt.Sprintf("model letter: %s · no effort digits", spec.Model)
		}
		return fmt.Sprintf("model letter: %s · effort digits %s",
			spec.Model, digitSpan(sortedKeys(cfg.Efforts)))
	}
	d := string(token[len(token)-1])
	letter := token[:len(token)-1]
	return fmt.Sprintf("grammar: %s (%s) + %s (effort %s)",
		letter, cfg.Models[letter].Model, d, cfg.Efforts[d])
}

// showView renders everything about one token. The error message is
// ready for fatal() verbatim.
func showView(vc viewContext, arg string) ([]string, error) {
	pal := vc.pal
	token := arg
	res, found, err := resolveToken(vc.cfg, token)
	if !found && err == nil && len(arg) > 1 && strings.HasPrefix(arg, "c") {
		if r2, f2, e2 := resolveToken(vc.cfg, arg[1:]); f2 {
			token, res, found, err = arg[1:], r2, f2, e2
		}
	}
	if err != nil {
		return nil, err
	}
	if !found {
		msg := fmt.Sprintf("unknown token %q", arg)
		if hint := uniquePrefixMatch(vc.cfg, arg); hint != "" {
			msg += fmt.Sprintf(" — did you mean %q?", hint)
		}
		return nil, fmt.Errorf("%s; run `clau list` to see every token", msg)
	}

	row := func(label, value string) string {
		return fmt.Sprintf("  %-8s  %s", label, value)
	}
	out := []string{pal.cyan + token + pal.reset + " → c" + token}
	out = append(out, row("kind", kindLine(vc, token)))

	src := "global (" + contractHome(vc.globalPath) + ")"
	if tokenSource(token, vc.projModels, vc.projProfiles) == "project" {
		src = "project (" + vc.st.Path + ")"
	}
	out = append(out, row("source", src))

	command := "c" + token
	linkPath := filepath.Join(vc.dir, linkFileName(command, vc.goos))
	switch {
	case isOwned(linkPath, vc.clauPath):
		out = append(out, row("linked", "yes → "+contractHome(linkPath)))
	case vc.globalTokens[token]:
		out = append(out, row("linked", "no "+pal.yellow+"— run clau link"+pal.reset))
	default:
		out = append(out, row("linked", "no "+pal.dim+"— project profiles launch as c "+token+pal.reset))
	}

	target := vc.cfg.Claude
	if res.Claude != "" {
		target = res.Claude
	}
	out = append(out, row("launches", strings.Join(append([]string{target}, res.Flags...), " ")))

	for i, k := range sortedKeys(res.Env) {
		v := res.Env[k]
		if maskedEnvName(k) {
			v = "•••"
		}
		label := ""
		if i == 0 {
			label = "env"
		}
		out = append(out, row(label, k+"="+v))
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add commands.go commands_test.go
git commit -m "Render the per-token show view with env masking"
```

---

### Task 8: cmdShow wiring — verb, dispatch, help

**Files:**
- Modify: `main.go` (`reservedVerbs`, `runManagement`, `helpText`), `commands.go` (`cmdShow`)
- Test: `main_test.go` (`TestDispatch`)

**Interfaces:**
- Consumes: `showView`, `newViewContext` (Tasks 6-7).
- Produces: `clau show <token> [--dir DIR]` end-to-end.

- [ ] **Step 1: Write the failing test** — add one case to the `TestDispatch` table in `main_test.go`:

```go
		{"clau", []string{"show", "o5"}, action{kind: "management", verb: "show", args: []string{"o5"}}},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDispatch ./...`
Expected: FAIL — show dispatches as `kind: "launch"` because the verb is not reserved.

- [ ] **Step 3: Implement** — in `main.go`:

`reservedVerbs` gains `"show": true` (put it on the first line, after `"list": true`):

```go
var reservedVerbs = map[string]bool{
	"link": true, "unlink": true, "list": true, "show": true, "run": true,
	"init": true, "trust": true, "untrust": true,
	"doctor": true, "completions": true,
	"version": true, "help": true, "__launch": true,
}
```

`runManagement` gains, after the `case "list":` block:

```go
	case "show":
		cmdShow(args)
```

`helpText` — replace the list line and insert show after it:

```
  clau list [--tokens] [--dir DIR] every shortcut at a glance, grouped
  clau show <token> [--dir DIR]    everything about one shortcut
```

And in `commands.go`, below `cmdList`:

```go
func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	dir := fs.String("dir", defaultLinkDir(), "directory checked for links")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fatal("usage: clau show <token> [--dir DIR]")
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	cfg, st, err := loadEffectiveConfig(cwd, false)
	if err != nil {
		fatal("%v", err)
	}
	lines, err := showView(newViewContext(cfg, st, *dir), fs.Arg(0))
	if err != nil {
		fatal("%v", err)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}
```

(Flags parse before the positional, Go-style: `clau show --dir X gem` and `clau show gem` both work.)

- [ ] **Step 4: Run tests and smoke-check**

Run: `go test ./...` — PASS.
Run: `go run . show o3` — expect the grammar kind line and `launches  claude --model opus --effort high`.
Run: `go run . show nope; echo "exit=$?"` — expect `clau: unknown token "nope"...` and `exit=1`.
Run: `go run . help | grep show` — the new help line renders aligned.

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go commands.go
git commit -m "Add clau show and reserve the verb"
```

---

### Task 9: Completions gain show

**Files:**
- Modify: `completions.go` (all three scripts)
- Test: `completions_test.go`

**Interfaces:**
- Consumes: nothing new (scripts shell out to `clau list --tokens`).
- Produces: `show` completes as a verb, and its argument position completes tokens, in fish, zsh, and bash.

- [ ] **Step 1: Write the failing test** — append to `completions_test.go`:

```go
func TestCompletionsIncludeShow(t *testing.T) {
	for _, shell := range []string{"fish", "zsh", "bash"} {
		s, err := completionScript(shell)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s, "show") {
			t.Errorf("%s completions missing the show verb", shell)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCompletionsIncludeShow ./...`
Expected: FAIL for all three shells.

- [ ] **Step 3: Implement** — edit the three script constants in `completions.go`:

fish — verb list line becomes:

```
complete -c clau -f -n 'test (count (commandline -opc)) -eq 1' -a 'link unlink list show run init trust untrust doctor completions version help'
```

fish — the run token line completes for show too (`contains` accepts multiple values):

```
complete -c clau -f -n 'contains -- (commandline -opc)[2] run show; and test (count (commandline -opc)) -eq 2' -a '(clau list --tokens 2>/dev/null)'
```

bash — verb list gains show, and the token branch covers both verbs:

```
    COMPREPLY=($(compgen -W "link unlink list show run init trust untrust doctor completions version help $(clau list --tokens 2>/dev/null)" -- "${COMP_WORDS[1]}"))
  elif { [ "${COMP_WORDS[1]}" = run ] || [ "${COMP_WORDS[1]}" = show ]; } && [ "$COMP_CWORD" -eq 2 ]; then
```

zsh — verb list gains show, and the token branch becomes:

```
    items=(link unlink list show run init trust untrust doctor completions version help)
```
```
  elif [[ ${words[2]} == (run|show) ]] && (( CURRENT == 3 )); then
```

- [ ] **Step 4: Run tests and syntax-check the scripts**

Run: `go test ./...` — PASS.
Run: `go run . completions bash | bash -n` — no syntax errors.
Run: `go run . completions fish | head -3` and `go run . completions zsh | head -3` — scripts render (fish/zsh syntax is exercised by eye here; both edits mirror existing constructs).

- [ ] **Step 5: Commit**

```bash
git add completions.go completions_test.go
git commit -m "Complete the show verb and its token argument"
```

---

### Task 10: README, docs, and final verification

**Files:**
- Modify: `README.md`

**Interfaces:** none — documentation and a full sweep.

- [ ] **Step 1: README edits** — three changes:

1. Commands block (`## Commands`): replace

   ```
       clau list [--tokens] [--dir DIR]  every token → what it launches
   ```

   with

   ```
       clau list [--tokens] [--dir DIR]  every shortcut at a glance, grouped
       clau show <token> [--dir DIR]     everything about one shortcut
   ```

2. Per-project section: change the sentence

   ```
   `clau list` and `clau doctor` still work untrusted and show
   that a project file is present.
   ```

   to

   ```
   `clau list`, `clau show`, and `clau doctor` still work untrusted
   and show that a project file is present.
   ```

3. Help-block in README's demo caption area needs no change; the gif
   itself is stale — do NOT regenerate it in this change (spec
   non-goal). Add nothing else.

- [ ] **Step 2: Full verification sweep**

```bash
gofmt -l .            # expect: no output
go vet ./...          # expect: clean
go test ./...         # expect: all pass (unit + integration)
go run . list         # piped: plain sectioned view, no escapes
go run . list --tokens | wc -l   # unchanged token count
go run . show f5      # grammar token detail
go run . doctor       # untouched, still healthy
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Document the grouped list and clau show"
```

---

## Self-Review Notes (already applied)

- Spec coverage: sections/order → Task 5; legend → Task 3; badges incl. the gma fix → Tasks 4-5; footer → Task 5; trust wording + color → Task 5; color policy incl. Windows → Task 2; show incl. masking/fallback/did-you-mean/`--dir` → Tasks 7-8; provenance incl. project-layer reset → Task 1; `reservedVerbs` shadowing note → Task 8; completions → Task 9; README → Task 10; `--tokens` untouched → Tasks 6 asserts.
- Type consistency: `viewContext`/`listRowV`/`palette` names match across Tasks 4-8; `rowBadges(token, project, linkable, linked, pal)` order is uniform.
