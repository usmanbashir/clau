package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

var version = "dev"

// versionString prefers the goreleaser-stamped version, falling back to
// the module version for `go install` builds ("dev" for local builds,
// whose build info carries no usable version).
func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

var reservedVerbs = map[string]bool{
	"link": true, "unlink": true, "list": true, "run": true,
	"init": true, "doctor": true, "completions": true,
	"version": true, "help": true, "__launch": true,
}

type action struct {
	kind  string // "management", "launch", "named", "badname"
	verb  string
	token string
	args  []string
}

func invocationName(argv0 string) string {
	name := filepath.Base(filepath.ToSlash(argv0))
	if i := strings.LastIndexByte(argv0, '\\'); i >= 0 {
		name = argv0[i+1:]
	}
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".exe"), ".cmd")
	return name
}

func dispatch(name string, args []string) action {
	switch {
	case name == "clau":
		if len(args) > 0 {
			verb := args[0]
			switch verb {
			case "--help", "-h":
				verb = "help"
			case "--version", "-v":
				verb = "version"
			}
			if reservedVerbs[verb] {
				return action{kind: "management", verb: verb, args: args[1:]}
			}
		}
		return action{kind: "launch", args: args}
	case name == "c":
		return action{kind: "launch", args: args}
	case strings.HasPrefix(name, "c"):
		return action{kind: "named", token: name[1:], args: args}
	default:
		return action{kind: "badname"}
	}
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "clau: "+format+"\n", a...)
	os.Exit(1)
}

func mustLoadConfig() Config {
	cfg, err := loadConfig(configPath())
	if err != nil {
		fatal("%v", err)
	}
	return cfg
}

func launch(cfg Config, res TokenResolution, extra []string) {
	l := buildLaunch(cfg, res, extra)
	if err := execClaude(l); err != nil {
		fatal("cannot exec %q: %v (is it on PATH?)", l.Target, err)
	}
}

func runLauncher(args []string) {
	cfg := mustLoadConfig()
	if len(args) > 0 {
		if args[0] == "--" {
			launch(cfg, TokenResolution{}, args[1:])
			return
		}
		res, found, err := resolveToken(cfg, args[0])
		if err != nil {
			fatal("%v", err)
		}
		if found {
			launch(cfg, res, args[1:])
			return
		}
	}
	launch(cfg, TokenResolution{}, args)
}

func runNamed(token string, args []string) {
	cfg := mustLoadConfig()
	res, found, err := resolveToken(cfg, token)
	if err != nil {
		fatal("%v", err)
	}
	if !found {
		fatal("shortcut %q no longer exists in config; run `clau list` to see shortcuts, `clau link` to sync commands", token)
	}
	launch(cfg, res, args)
}

func cmdRun(args []string) {
	if len(args) == 0 {
		fatal("run requires a shortcut token; see `clau list`")
	}
	cfg := mustLoadConfig()
	res, found, err := resolveToken(cfg, args[0])
	if err != nil {
		fatal("%v", err)
	}
	if !found {
		fatal("unknown shortcut token %q; run `clau list`", args[0])
	}
	launch(cfg, res, args[1:])
}

func runManagement(verb string, args []string) {
	switch verb {
	case "link":
		cmdLink(args)
	case "unlink":
		cmdUnlink(args)
	case "list":
		cmdList(args)
	case "run":
		cmdRun(args)
	case "__launch":
		runLauncher(args)
	case "init":
		cmdInit(args)
	case "doctor":
		cmdDoctor(args)
	case "completions":
		cmdCompletions(args)
	case "version":
		cmdVersion()
	case "help":
		cmdHelp()
	}
}

func main() {
	a := dispatch(invocationName(os.Args[0]), os.Args[1:])
	switch a.kind {
	case "management":
		runManagement(a.verb, a.args)
	case "launch":
		runLauncher(a.args)
	case "named":
		runNamed(a.token, a.args)
	default:
		fatal("unrecognized invocation name %q; expected clau, c, or c<token>", invocationName(os.Args[0]))
	}
}

func cmdLink(args []string) {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	dir := fs.String("dir", defaultLinkDir(), "directory for shortcut commands")
	force := fs.Bool("force", false, "create even over collisions")
	fs.Parse(args)
	cfg := mustLoadConfig()
	clauPath, err := clauExecutable()
	if err != nil {
		fatal("%v", err)
	}
	rep, err := syncLinks(cfg, *dir, clauPath, runtime.GOOS, *force)
	if err != nil {
		fatal("%v", err)
	}
	for _, s := range rep.Skipped {
		fmt.Fprintf(os.Stderr, "clau: skipped %s\n", s)
	}
	fmt.Printf("linked %d, kept %d, skipped %d, pruned %d in %s\n",
		len(rep.Created), len(rep.Kept), len(rep.Skipped), len(rep.Pruned), *dir)
}

func cmdUnlink(args []string) {
	fs := flag.NewFlagSet("unlink", flag.ExitOnError)
	dir := fs.String("dir", defaultLinkDir(), "directory for shortcut commands")
	fs.Parse(args)
	clauPath, err := clauExecutable()
	if err != nil {
		fatal("%v", err)
	}
	removed, err := removeOwned(*dir, clauPath)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("removed %d clau-owned commands from %s\n", len(removed), *dir)
}

func cmdCompletions(args []string) {
	if len(args) != 1 {
		fatal("usage: clau completions fish|zsh|bash")
	}
	s, err := completionScript(args[0])
	if err != nil {
		fatal("%v", err)
	}
	fmt.Print(s)
}
func cmdVersion() {
	fmt.Println("clau " + versionString())
	fmt.Println("Copyright (C) 2026 Usman Bashir. License GPL-3.0-or-later.")
	fmt.Println("https://github.com/usmanbashir/clau")
}
func cmdHelp()    { fmt.Print(helpText) }

const helpText = `clau — model×effort shortcodes and launch profiles for Claude Code

Usage:
  clau <token> [claude args...]    launch via shortcut token (same as c <token>)
  c [<token>] [claude args...]     launch; unknown first arg passes through
  c<token> [claude args...]        launch via generated shortcut command
  c -- [claude args...]            force raw passthrough

Management (clau only):
  clau link [--dir DIR] [--force]  sync shortcut commands (symlinks/shims)
  clau unlink [--dir DIR]          remove clau-owned commands
  clau list [--tokens] [--dir DIR] show tokens, commands, and launches
  clau run <token> [args...]       explicit launcher (errors on unknown token)
  clau init [--force]              write a starter config
  clau doctor [--dir DIR]          check config, claude, PATH, links
  clau completions <shell>         fish|zsh|bash completion script
  clau version | help

Config: $CLAU_CONFIG or ~/.config/clau/config.toml
Project: https://github.com/usmanbashir/clau
`
