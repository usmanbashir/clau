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
	Token, Command, Linked, Preview string
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

func listRows(cfg Config, dir, clauPath, goos string) []listRow {
	var rows []listRow
	for _, token := range tokensOf(cfg) {
		command := "c" + token
		linked := "no"
		if isOwned(filepath.Join(dir, linkFileName(command, goos)), clauPath) {
			linked = "yes"
		}
		rows = append(rows, listRow{Token: token, Command: command, Linked: linked, Preview: previewFor(cfg, token)})
	}
	return rows
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tokens := fs.Bool("tokens", false, "print bare tokens only")
	dir := fs.String("dir", defaultLinkDir(), "directory checked for links")
	fs.Parse(args)
	cfg := mustLoadConfig()
	if *tokens {
		for _, t := range tokensOf(cfg) {
			fmt.Println(t)
		}
		return
	}
	clauPath, err := clauExecutable()
	if err != nil {
		clauPath = ""
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TOKEN\tCOMMAND\tLINKED\tLAUNCHES")
	for _, r := range listRows(cfg, *dir, clauPath, runtime.GOOS) {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Token, r.Command, r.Linked, r.Preview)
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
# Add your own — one line yields <letter>, <letter>1..5 and their commands:
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

# Backend/account switch:
#[profiles.work]
#env = { ANTHROPIC_BASE_URL = "https://gateway.corp.example" }

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

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	dir := fs.String("dir", defaultLinkDir(), "directory checked for links")
	fs.Parse(args)
	cfg, cfgErr := loadConfig(configPath())
	clauPath, err := clauExecutable()
	if err != nil {
		clauPath = ""
	}
	failed := false
	for _, f := range doctorFindings(cfg, cfgErr, *dir, clauPath, runtime.GOOS) {
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
