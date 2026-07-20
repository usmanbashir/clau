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
	"text/tabwriter"
)

func tokensOf(cfg Config) []string {
	var tokens []string
	digits := make([]string, 0, len(cfg.Efforts))
	for d := range cfg.Efforts {
		digits = append(digits, d)
	}
	sort.Strings(digits)
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

type listRow struct {
	Token, Command, Linked, Source, Preview string
}

func previewFor(cfg Config, token string) string {
	res, found, err := resolveToken(cfg, token)
	if err != nil || !found {
		return "(unresolvable)"
	}
	target := cfg.Claude
	if res.Claude != "" {
		target = res.Claude
	}
	parts := append([]string{target}, res.Flags...)
	preview := strings.Join(parts, " ")
	if len(res.Env) > 0 {
		keys := make([]string, 0, len(res.Env))
		for k := range res.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		preview += " [env: " + strings.Join(keys, ", ") + "]"
	}
	return preview
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

const starterConfig = `# clau configuration — https://github.com/usmanbashir/clau
# Shortcut grammar: <letter><digit>. Letters come from [models]; the digit
# 1..5 maps to effort low medium high xhigh max. co5 = opus at max effort.

# Binary to exec (name on PATH or absolute path). Default: claude
#claude = "claude"

[models]
o = "opus"
s = "sonnet"
f = "fable"
h = { model = "haiku", efforts = false }
# Add your own — one line yields <letter>, <letter>1..5 and, after
# "clau link", their commands. The model must be one your backend
# serves; for another backend use a profile (see below).
#g = "glm-5.2"

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
# BASE_URL carries no /v1 suffix; the auth token is a throwaway the
# server ignores, Claude Code just needs one set; DEFAULT_HAIKU_MODEL keeps
# background tasks on the local model too.
#[profiles.gem]
#model = "gemma4:12b"
#[profiles.gem.env]
#ANTHROPIC_BASE_URL = "http://localhost:11434"
#ANTHROPIC_AUTH_TOKEN = "ollama"
#ANTHROPIC_DEFAULT_HAIKU_MODEL = "gemma4:12b"

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
