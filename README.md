# clau

[![CI](https://github.com/usmanbashir/clau/actions/workflows/ci.yml/badge.svg)](https://github.com/usmanbashir/clau/actions/workflows/ci.yml)

Model×effort shortcodes and launch profiles for [Claude Code](https://code.claude.com).

`co5` launches Opus at max effort. `cs1` is Sonnet on the cheap. `crev` is
your code-review persona. One static binary, no runtime, no shell config —
it resolves a shortcut to `claude` flags and env, execs, and disappears
(on Windows, where exec doesn't exist, it spawns and waits instead).

    co5                  # claude --model opus --effort max
    c s3 "explain this"  # same grammar, argument style
    c "fix this bug"     # unknown token? everything passes through
    crev                 # your own profile: model + flags + env

## Install

    brew install usmanbashir/tap/clau               # macOS and Linux
    go install github.com/usmanbashir/clau@latest   # anywhere with Go

Prebuilt archives for six platforms are on the
[releases page](https://github.com/usmanbashir/clau/releases). Note that
`go install` builds report `clau dev` — version stamping happens in
release builds.

Then:

    clau init            # write a starter config
    clau link            # generate shortcut commands in ~/.local/bin
    clau list            # see every shortcut and what it launches

`clau link` refuses to shadow real binaries (a profile named `at` will not
silently take over `cat`) — it warns, skips, and moves on.

## The grammar

`<letter><digit>`: letters come from `[models]` in your config, the digit
1–5 maps to effort low/medium/high/xhigh/max. Letter alone = model only.
Defaults: `o` opus, `s` sonnet, `f` fable, `h` haiku (haiku takes no
effort digit — the CLI would silently downgrade it, so clau errors instead).

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

Your own flags always win: `crev --model sonnet` runs the rev profile on
Sonnet. `c -- anything` skips resolution entirely.

## Config

`$CLAU_CONFIG` or `${XDG_CONFIG_HOME:-~/.config}/clau/config.toml`.
Missing file = defaults.
Your `[models]` entries merge over the defaults. See `clau init`'s output
for the full commented reference.

## Management

    clau link [--dir DIR] [--force]   sync shortcut commands (symlinks; .cmd shims on Windows)
    clau unlink [--dir DIR]           remove everything clau created
    clau list [--tokens] [--dir DIR]  every token → what it launches
    clau run <token> [args...]        explicit launcher (errors on unknown token)
    clau doctor [--dir DIR]           config, PATH, collisions, stale links
    clau completions fish|zsh|bash    tab completion

For fish, persist completions with
`clau completions fish > ~/.config/fish/conf.d/clau.fish` — `conf.d`, not
`completions/`, because the file registers completions for both `clau`
and `c`, and the `completions/` autoloader only loads a file for the
command matching its filename.

## License

GPL-3.0-or-later. See [COPYING](COPYING).
