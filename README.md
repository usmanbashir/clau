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

<!-- demo: 20s asciinema/GIF goes here -->

## Install

    brew install usmanbashir/tap/clau               # macOS
    go install github.com/usmanbashir/clau@latest   # anywhere with Go

Prebuilt archives for six platforms are on the
[releases page](https://github.com/usmanbashir/clau/releases).
(`go install` builds report `clau dev` — version stamping happens in
release builds.)

## Quick start

    clau init            # write a starter config
    clau link            # generate shortcut commands in ~/.local/bin
    co5                  # you're in Opus at max effort

If a shortcut isn't found, run `clau doctor` — it checks your config,
PATH, collisions, and links, and says what to fix.

## The grammar

`<letter><digit>`: the letter picks a model from `[models]` in your
config, the digit 1–5 picks effort low/medium/high/xhigh/max. Letter
alone = model only. Built-in letters: `o` opus, `s` sonnet, `f` fable,
`h` haiku (haiku takes no effort digit — the CLI would silently
downgrade it, so clau errors instead).

Add a model in one line — `g = "glm-4.7"` — and `g`, `g1`…`g5`, plus
commands `cg`, `cg1`…`cg5` all exist.

## Profiles

Named shortcuts that carry flags and environment. Four directions:

```toml
[profiles.rev]           # persona
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "You are a meticulous code reviewer."]

[profiles.q]             # cheap one-shot pipeline tool: git diff | cq -p ...
model = "haiku"
flags = ["-p", "--max-budget-usd", "0.25"]

[profiles.work]          # backend/account switch
env = { ANTHROPIC_BASE_URL = "https://gateway.corp.example" }

[profiles.lean]          # context loadout: fast, hookless startup
flags = ["--bare", "--strict-mcp-config"]
```

Your own flags always win: `crev --model sonnet` runs the rev profile
on Sonnet. `c -- anything` skips resolution entirely.

## Why not shell aliases?

clau began as fish functions in the author's dotfiles, generalized so
the same idea works without fish — or any shell config at all:

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
Missing file = defaults. Your `[models]` entries merge over the
defaults. `clau init` writes a fully commented reference config.

## Commands

    clau link [--dir DIR] [--force]   sync shortcut commands (symlinks; .cmd shims on Windows)
    clau unlink [--dir DIR]           remove everything clau created
    clau list [--tokens] [--dir DIR]  every token → what it launches
    clau run <token> [args...]        explicit launcher (errors on unknown token)
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
