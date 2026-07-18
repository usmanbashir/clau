package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

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

// Stubs implemented in later tasks.
func cmdLink(args []string)        { fatal("link: not implemented yet") }
func cmdUnlink(args []string)      { fatal("unlink: not implemented yet") }
func cmdList(args []string)        { fatal("list: not implemented yet") }
func cmdInit(args []string)        { fatal("init: not implemented yet") }
func cmdDoctor(args []string)      { fatal("doctor: not implemented yet") }
func cmdCompletions(args []string) { fatal("completions: not implemented yet") }
func cmdVersion()                  { fmt.Println("clau " + version) }
func cmdHelp()                     { fmt.Print(helpText) }

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
`
