package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTokensOf(t *testing.T) {
	cfg := defaultConfig()
	cfg.Models["k"] = ModelSpec{Model: "kimi"} // efforts=false: no k1..k5
	cfg.Profiles["rev"] = Profile{Model: "opus"}
	got := strings.Join(tokensOf(cfg), " ")
	want := "f f1 f2 f3 f4 f5 h h1 h2 h3 h4 h5 k o o1 o2 o3 o4 o5 rev s s1 s2 s3 s4 s5"
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
	if len(cfg.Bases) != 0 {
		t.Errorf("starter config defines active bases: %v (examples must be commented out)", cfg.Bases)
	}
	if !reflect.DeepEqual(cfg.Models, defaultConfig().Models) {
		t.Errorf("starter config changed default models: %+v", cfg.Models)
	}
}

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
		if got := tokenSource(token, projModels, projProfiles, false); got != want {
			t.Errorf("tokenSource(%q) = %q, want %q", token, got, want)
		}
	}
	if got := tokenSource("rev", nil, nil, false); got != "global" {
		t.Errorf("nil maps must mean global, got %q", got)
	}

	// A project [efforts] replaces the whole ladder, so every digit token
	// is project-sourced then, even a digit the global ladder also had;
	// bare letters keep their own provenance.
	ladder := map[string]string{
		"o6":  "project",
		"o5":  "project", // remapped, not inherited
		"g5":  "project",
		"o":   "global",
		"s":   "global",
		"g":   "project",
		"rev": "project",
	}
	for token, want := range ladder {
		if got := tokenSource(token, projModels, projProfiles, true); got != want {
			t.Errorf("tokenSource(%q, efforts) = %q, want %q", token, got, want)
		}
	}
}

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
			fs := projectFindings(c.st, map[string]string{missing: "x"}, true)
			var text strings.Builder
			for _, f := range fs {
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
			for _, f := range fs {
				if f.Level == "fail" {
					t.Errorf("project findings must never fail: %s", f.Msg)
				}
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

func TestRowBadges(t *testing.T) {
	pal := palette{} // plain: rules readable as bytes
	cases := []struct {
		name                      string
		project, linkable, linked bool
		want                      string
	}{
		{"linked global", false, true, true, ""},
		{"unlinked global", false, true, false, "not linked — run clau link"},
		{"orphaned link", false, false, true, ""}, // linked, but no longer in the global set
		{"project linked", true, true, true, "project"},
		{"project unlinked global", true, true, false, "project · not linked — run clau link"},
		{"project orphaned link", true, false, true, "project"},
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
	linkable, linked := familyStates("o", vc)
	if !linkable || linked {
		t.Errorf("unlinked family: linkable=%v linked=%v, want true false", linkable, linked)
	}

	// Link the whole o family: co, co1..co5.
	for _, tok := range []string{"o", "o1", "o2", "o3", "o4", "o5"} {
		if err := os.Symlink(clauPath, filepath.Join(dir, "c"+tok)); err != nil {
			t.Fatal(err)
		}
	}
	linkable, linked = familyStates("o", vc)
	if !linkable || !linked {
		t.Errorf("linked family: linkable=%v linked=%v, want true true", linkable, linked)
	}

	// A letter without effort digits is a family of one.
	cfg.Models["k"] = ModelSpec{Model: "kimi"}
	vc.globalTokens["k"] = true
	linkable, linked = familyStates("k", vc)
	if !linkable || linked {
		t.Errorf("unlinked k: linkable=%v linked=%v, want true false", linkable, linked)
	}
	if err := os.Symlink(clauPath, filepath.Join(dir, "ck")); err != nil {
		t.Fatal(err)
	}
	linkable, linked = familyStates("k", vc)
	if !linkable || !linked {
		t.Errorf("linked k: linkable=%v linked=%v, want true true", linkable, linked)
	}

	// Links that are no longer in the global set (a project-declared
	// letter, or orphans) are not linkable, but still count as linked:
	// the badge then shows nothing extra.
	vc2 := vc
	vc2.globalTokens = map[string]bool{}
	linkable, linked = familyStates("o", vc2)
	if linkable || !linked {
		t.Errorf("orphaned family: linkable=%v linked=%v, want false true", linkable, linked)
	}

	// A project [efforts] ladder replaces the effective digits, but the
	// family clau link manages is still co, co1..co5: all present means
	// nothing to warn about, and a missing one is still a warning.
	vc3 := vc
	vc3.cfg.Efforts = map[string]string{"6": "max"}
	linkable, linked = familyStates("o", vc3)
	if !linkable || !linked {
		t.Errorf("project ladder, global family linked: linkable=%v linked=%v, want true true", linkable, linked)
	}
	if err := os.Remove(filepath.Join(dir, "co5")); err != nil {
		t.Fatal(err)
	}
	linkable, linked = familyStates("o", vc3)
	if !linkable || linked {
		t.Errorf("project ladder, co5 missing: linkable=%v linked=%v, want true false", linkable, linked)
	}
}

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
	cfg.Models["k"] = ModelSpec{Model: "kimi"} // opted out of effort digits
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
	if l := lineWith(t, lines, "kimi"); !strings.Contains(l, "no effort digits") {
		t.Errorf("k row missing efforts note: %q", l)
	}
	if l := lineWith(t, lines, "haiku"); strings.Contains(l, "no effort digits") {
		t.Errorf("h row has an efforts note: %q", l)
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

	// 24 global grammar tokens, none linked.
	if got := footerLine(vc); got != "0 of 24 commands linked in "+contractHome(vc.dir)+" — run clau link · 1 project-only · details: clau show <token>" {
		t.Errorf("unlinked footer = %q", got)
	}

	for tok := range vc.globalTokens {
		if err := os.Symlink(vc.clauPath, filepath.Join(vc.dir, "c"+tok)); err != nil {
			t.Fatal(err)
		}
	}
	if got := footerLine(vc); got != "all 24 commands linked in "+contractHome(vc.dir)+" · 1 project-only · details: clau show <token>" {
		t.Errorf("linked footer = %q", got)
	}

	// A project [efforts] ladder (digit 6 only) swaps the effective
	// digits, but the denominator stays the 24 commands clau link
	// manages; f6 h6 o6 s6 join gma as project-only.
	vc.cfg.Efforts = map[string]string{"6": "max"}
	if got := footerLine(vc); got != "all 24 commands linked in "+contractHome(vc.dir)+" · 5 project-only · details: clau show <token>" {
		t.Errorf("project ladder footer = %q", got)
	}
	if err := os.Remove(filepath.Join(vc.dir, "co5")); err != nil {
		t.Fatal(err)
	}
	if got := footerLine(vc); got != "23 of 24 commands linked in "+contractHome(vc.dir)+" — run clau link · 5 project-only · details: clau show <token>" {
		t.Errorf("project ladder footer with co5 missing = %q", got)
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

func TestMaskedEnvName(t *testing.T) {
	cases := map[string]bool{
		"ANTHROPIC_AUTH_TOKEN":           true,
		"OPENAI_API_KEY":                 true,
		"MY_SECRET":                      true,
		"DB_PASSWORD":                    true,
		"anthropic_auth_token":           true,
		"TOKEN_EXPIRY":                   true,  // the segment can sit anywhere
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS": false, // segment is TOKENS, not TOKEN
		"KEYS":                           false,
		"SECRETS":                        false,
		"PASSWORDS":                      false,
		"ANTHROPIC_BASE_URL":             false,
		"MONKEYS":                        false,
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
	cfg.Models["k"] = ModelSpec{Model: "kimi"} // opted out of effort digits
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
		"h":  "model letter: haiku · effort digits 1-5",
		"k":  "model letter: kimi · no effort digits",
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

func TestShowViewPlainProfileKind(t *testing.T) {
	vc := showVC(t)
	vc.cfg.Profiles["rev"] = Profile{Model: "opus", Effort: "high"}
	lines, err := showView(vc, "rev")
	if err != nil {
		t.Fatal(err)
	}
	if l := lineWith(t, lines, "kind"); l != "  kind      profile" {
		t.Errorf("plain profile kind row = %q, want exactly profile", l)
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

	if _, err := showView(vc, "k3"); err == nil ||
		!strings.Contains(err.Error(), `efforts = false is set for "k" in [models]`) {
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
	if !strings.Contains(out, "project shortcuts launch as c gem") {
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

func TestShowViewProjectLadderSource(t *testing.T) {
	vc := showVC(t)
	vc.cfg.Efforts = map[string]string{"6": "max"}
	vc.st = ProjectStatus{Path: "/repo/.clau.toml", Trusted: true, Applied: true}
	vc.projEfforts = true
	lines, err := showView(vc, "o6")
	if err != nil {
		t.Fatal(err)
	}
	if out := strings.Join(lines, "\n"); !strings.Contains(out, "source    project (/repo/.clau.toml)") {
		t.Errorf("project-ladder digit token must be project-sourced:\n%s", out)
	}
	lines, err = showView(vc, "o")
	if err != nil {
		t.Fatal(err)
	}
	if out := strings.Join(lines, "\n"); !strings.Contains(out, "source    global (") {
		t.Errorf("bare global letter must stay global-sourced:\n%s", out)
	}
}
