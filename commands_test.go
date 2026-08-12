package main

import (
	"errors"
	"os"
	"path/filepath"
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
	rows := listRows(cfg, t.TempDir(), "/nonexistent/clau", "linux", nil, nil)
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
	if len(cfg.Bases) != 0 {
		t.Errorf("starter config defines active bases: %v (examples must be commented out)", cfg.Bases)
	}
	if cfg.Models["o"].Model != "opus" {
		t.Errorf("starter config changed defaults: %+v", cfg.Models)
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
