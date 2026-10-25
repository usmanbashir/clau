package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

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

func tokensOf(cfg Config) []string {
	var tokens []string
	digits := sortedKeys(cfg.Efforts)
	for letter, spec := range cfg.Models {
		tokens = append(tokens, letter)
		if spec.Efforts {
			for _, d := range digits {
				tokens = append(tokens, letter+d)
			}
		}
	}
	seen := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		seen[tok] = true
	}
	for name := range cfg.Profiles {
		if !seen[name] { // a profile shadowing a grammar token is one shortcut, not two
			tokens = append(tokens, name)
		}
	}
	sort.Strings(tokens)
	return tokens
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

// viewContext carries everything the list and show renderers need,
// assembled once per command so the renderers stay pure and testable.
type viewContext struct {
	cfg          Config
	st           ProjectStatus
	globalPath   string // loaded global config path (show's source line)
	dir          string // link dir being inspected
	clauPath     string // this binary for isOwned ("" if unknown)
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
// The digits come from the effective ladder, so a project [efforts]
// replacement leaves the badge conservative: it can warn about family
// commands the global-only `clau link` will never create.
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
		out = append(out, row("linked", "no "+pal.dim+"— project shortcuts launch as c "+token+pal.reset))
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

const starterConfig = `# clau configuration — https://github.com/usmanbashir/clau
# Shortcut grammar: <letter><digit>. Letters come from [models]; the digit
# 1..5 maps to effort low medium high xhigh max. co5 = opus at max effort.

# Binary to exec (name on PATH or absolute path). Default: claude
#claude = "claude"

[models]
o = "opus"
s = "sonnet"
f = "fable"
h = "haiku"
# Add your own — one line yields <letter>, <letter>1..5 and, after
# "clau link", their commands. The model must be one your backend
# serves; for another backend use a profile (see below).
#g = "glm-5.2"
# A model without effort support opts out of the digits — clau then
# rejects k1..k5 rather than let claude drop the effort silently:
#k = { model = "kimi", efforts = false }

# The digit ladder is configurable too — a non-empty [efforts] table
# maps digits 1..9 to --effort values and replaces the default five:
#[efforts]
#1 = "low"
#2 = "medium"
#3 = "high"
#4 = "xhigh"
#5 = "max"

# Profiles: named shortcuts carrying flags and env.
# Run "clau link" after editing to sync shortcut commands.

# Persona:
#[profiles.rev]
#model = "opus"
#effort = "high"
#flags = ["--append-system-prompt", "You are a meticulous code reviewer."]

# Cheap one-shot pipeline tool:
#[profiles.q]
#model = "haiku"
#flags = ["-p", "--max-budget-usd", "0.25"]

# LLM gateway / account switch:
#[profiles.work]
#env = { ANTHROPIC_BASE_URL = "https://gateway.corp.example" }

# Local backend — anything speaking the Anthropic Messages API works
# (Ollama, LM Studio, LiteLLM): https://code.claude.com/docs/en/llm-gateway
# Shared backend settings live in [bases]; profiles opt in with
# extends. A base is never a command itself. env merges per key,
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

# Lean startup (skip hooks, MCP, plugins):
#[profiles.lean]
#flags = ["--bare", "--strict-mcp-config"]
`

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite an existing config")
	fs.Parse(args)
	path := configPath()
	if _, err := os.Stat(path); err == nil && !*force {
		fatal("%s already exists (use --force to overwrite)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal("%v", err)
	}
	if err := os.WriteFile(path, []byte(starterConfig), 0o644); err != nil {
		fatal("%v", err)
	}
	fmt.Printf("wrote %s\n", path)
}

type finding struct {
	Level string // "ok", "warn", "fail"
	Msg   string
}

func doctorFindings(cfg Config, cfgErr error, dir, clauPath, goos string) []finding {
	if cfgErr != nil {
		return []finding{{"fail", fmt.Sprintf("config: %v", cfgErr)}}
	}
	var fs []finding
	fs = append(fs, finding{"ok", fmt.Sprintf("config: %s", configPath())})
	if _, err := exec.LookPath(cfg.Claude); err != nil {
		fs = append(fs, finding{"fail", fmt.Sprintf("exec target %q not found on PATH", cfg.Claude)})
	} else {
		fs = append(fs, finding{"ok", fmt.Sprintf("exec target %q found", cfg.Claude)})
	}
	onPath := false
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d != "" && filepath.Clean(d) == filepath.Clean(dir) {
			onPath = true
		}
	}
	if !onPath {
		fs = append(fs, finding{"warn", fmt.Sprintf("link dir %s is not on PATH", dir)})
	} else {
		fs = append(fs, finding{"ok", fmt.Sprintf("link dir %s is on PATH", dir)})
	}
	profiles := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	for _, name := range profiles {
		if _, found, _ := resolveToken(Config{Models: cfg.Models, Efforts: cfg.Efforts}, name); found {
			fs = append(fs, finding{"warn", fmt.Sprintf("profile %q shadows grammar token %q", name, name)})
		}
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".cmd")
			if name == "clau" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if !isOwned(path, clauPath) {
				continue
			}
			token := strings.TrimPrefix(name, "c")
			if token == "" {
				continue
			}
			if _, found, err := resolveToken(cfg, token); !found || err != nil {
				fs = append(fs, finding{"warn", fmt.Sprintf("stale link %s (token %q no longer resolves); run `clau link`", e.Name(), token)})
			}
		}
	}
	for _, name := range linkNames(cfg) {
		if foreign := foreignInPath(name, dir, clauPath, goos); foreign != "" {
			fs = append(fs, finding{"warn", fmt.Sprintf("command %s would collide with %s", name, foreign)})
		}
	}
	return fs
}

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

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	dir := fs.String("dir", defaultLinkDir(), "directory checked for links")
	fs.Parse(args)
	cfg, cfgErr := loadConfig(configPath())
	clauPath, err := clauExecutable()
	if err != nil {
		clauPath = ""
	}
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
		fmt.Printf("%-4s %s\n", strings.ToUpper(f.Level), f.Msg)
		if f.Level == "fail" {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

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
	data, err := os.ReadFile(proj)
	if err != nil {
		fatal("%v", err)
	}
	if _, err := applyConfigData(global, proj, data); err != nil {
		fatal("refusing to trust: %v", err)
	}
	hash := hashBytes(data)
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
