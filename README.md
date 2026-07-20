# clau

[![CI](https://github.com/usmanbashir/clau/actions/workflows/ci.yml/badge.svg)](https://github.com/usmanbashir/clau/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/usmanbashir/clau)](https://github.com/usmanbashir/clau/releases)

Every task wants [Claude Code](https://code.claude.com) launched
differently — Opus at max effort for the hard bug, cheap Sonnet for a
quick question, your code-review setup for PRs. That's a long flag
incantation or another trip through the `/model` picker, every single
time.

clau makes the choice the command name:

    co5                  # claude --model opus --effort max
    cs1                  # sonnet on the cheap
    crev                 # your code-review persona: model + flags + env
    c s3 "explain this"  # same grammar, argument style
    c "fix this bug"     # unknown token? passes straight through

![Demo: co5 launches Opus at max effort; clau list shows every shortcut](docs/demo.gif)

## Install

    brew install usmanbashir/tap/clau               # macOS and Linux
    go install github.com/usmanbashir/clau@latest   # anywhere with Go

Prebuilt archives for six platforms are on the
[releases page](https://github.com/usmanbashir/clau/releases).

## Quick start

    clau link            # generate shortcut commands in ~/.local/bin
    co5                  # you're in Opus at max effort

Zero config needed — the built-in letters below work out of the box.
If a shortcut isn't found, run `clau doctor` — it checks your config,
PATH, collisions, and links, and says what to fix.

## The grammar

`<letter><digit>`: the letter picks a model from `[models]` in your
config, the digit 1–5 picks effort low/medium/high/xhigh/max. Letter
alone = model only. Built-in letters: `o` opus, `s` sonnet, `f` fable,
`h` haiku (haiku takes no effort digit — the CLI would silently
downgrade it, so clau errors instead). The digit ladder is itself
config: an `[efforts]` table remaps digits `1`–`9` to `--effort`
values, replacing the default five.

Add a model in one line — `g = "glm-5.2"` — and `g`, `g1`…`g5` all
resolve (re-run `clau link` and the commands `cg`, `cg1`…`cg5` exist
too). One line is enough when your backend serves the model; to aim a
shortcut at a different backend, use a profile — see
[Other backends](#other-backends).

## Profiles

Named shortcuts that carry flags and environment — the two things a
`[models]` letter can't hold. The name is the whole token (no effort
digits; pin `effort` here or pass `--effort` at launch). Four
directions:

```toml
[profiles.rev]           # persona
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "You are a meticulous code reviewer."]

[profiles.q]             # cheap one-shot pipeline tool: git diff | cq -p ...
model = "haiku"
flags = ["-p", "--max-budget-usd", "0.25"]

[profiles.work]          # LLM gateway / account switch
env = { ANTHROPIC_BASE_URL = "https://gateway.corp.example" }

[profiles.lean]          # context loadout: fast, hookless startup
flags = ["--bare", "--strict-mcp-config"]
```

Your own flags always win: `crev --model sonnet` runs the rev profile
on Sonnet. `c -- anything` skips resolution entirely.

## Other backends

Claude Code will talk to any endpoint that speaks the Anthropic
Messages API — Ollama and LM Studio serve it natively, LiteLLM
translates for everything else, and
[LLM gateways](https://code.claude.com/docs/en/llm-gateway) add org
auth and routing on top. Switching backends is environment, and
environment lives in profiles:

```toml
[profiles.gem]           # gemma on a local Ollama
model = "gemma4:12b"
flags = ["--bare", "--strict-mcp-config"]  # small models drown in big context

[profiles.gem.env]
# no /v1 suffix — Claude Code appends /v1/messages itself
ANTHROPIC_BASE_URL = "http://localhost:11434"
# throwaway: the server ignores it, Claude Code just needs one set
ANTHROPIC_AUTH_TOKEN = "ollama"
# background tasks stay on the local model too
ANTHROPIC_DEFAULT_HAIKU_MODEL = "gemma4:12b"
```

`cgem` runs Claude Code entirely against the local model; `co5` still
talks to Anthropic. A `[models]` letter can't switch backends — it
only picks a model name, served by whatever backend your environment
already points at. (Which is why, behind a LiteLLM router fronting
many models, one `[models]` line per model really is the whole job.)

## Per-project config

A repo can carry its own `.clau.toml` — same format — and clau layers
it over your global config when you launch from inside the project:
`[models]` merge per key, a project profile or `[efforts]` table
replaces its global namesake wholesale. Commit the file and the whole
team shares the project's launch shapes.

Because a repo file can set env and flags, nothing applies until you
allow it once:

    clau trust --show    # read what the project wants
    clau trust           # allow it (re-asks whenever the file changes)

Untrusted or changed files are a hard error at launch — never silently
applied. `clau list` and `clau doctor` still work untrusted and show
that a project file is present. Project-only profiles stay
argument-style (`c deploy`); `clau link` links global tokens only, and
a linked global command like `crev` picks up the project's override
automatically. `CLAU_NO_PROJECT=1` skips the layer entirely.

## Why not shell aliases?

clau began as fish functions in my dotfiles, generalized so the same
idea works without fish — or any shell config at all:

- One static binary and one TOML file, identical in fish, zsh, bash,
  and on Windows. No dotfiles to sync.
- Linked shortcuts are real commands, so editors, scripts, and anything
  else that execs can use them — not just your interactive shell.
- Profiles bundle model, effort, flags, *and* env as data, not shell
  syntax.
- Tab completion in fish, zsh, and bash.
- Collision protection: a profile named `at` will not silently shadow
  `cat` — `clau link` warns, skips, and moves on.

## Config

`$CLAU_CONFIG` or `${XDG_CONFIG_HOME:-~/.config}/clau/config.toml`.
Missing file = defaults. `[models]` entries merge over the built-in
letters, profiles are all yours (none are built in), and a
non-empty `[efforts]` table replaces the digit ladder wholesale.
`clau init` writes a fully commented reference config.

## Commands

    clau link [--dir DIR] [--force]   sync shortcut commands (symlinks; .cmd shims on Windows)
    clau unlink [--dir DIR]           remove everything clau created
    clau list [--tokens] [--dir DIR]  every token → what it launches
    clau run <token> [args...]        explicit launcher (errors on unknown token)
    clau init [--force]               write a starter config
    clau trust [--show]               allow (or print) the project .clau.toml
    clau untrust                      revoke that trust
    clau doctor [--dir DIR]           config, PATH, collisions, stale links
    clau completions fish|zsh|bash    tab completion

For fish, persist completions with
`clau completions fish > ~/.config/fish/conf.d/clau.fish` (`conf.d`,
not `completions/` — the file registers both `clau` and `c`).

## How it works

clau is strictly a launcher. It reads argv[0] to know which shortcut
you called, resolves it against your config, sets flags and env, and
execs claude — no wrapper process, no runtime, no telemetry, nothing
between you and your session. (On Windows, where exec doesn't exist,
it spawns and waits instead.)

## License

GPL-3.0-or-later. See [COPYING](COPYING).
