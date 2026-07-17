# clau — design

A standalone launcher for Claude Code: shortcodes for model × effort
(`co5` = Opus at max effort), plus user-defined launch profiles that carry
flags and environment variables. One static Go binary, strictly exec.

Origin: the `c<model><effort>` fish functions in
[thedotfiles](https://github.com/usmanbashir/thedotfiles) `fish/config.fish`,
generalized so people who don't run fish — or don't want shell config at
all — can use the same concept.

## Goals

- Launch `claude` with a model/effort pair in one short command, in any shell.
- Let users define their own mappings and profiles in one config file.
- Support two invocation styles with identical semantics:
  - argument style: `c o5 "prompt"`, `c rev`, `clau run rev`
  - command style: `co5 "prompt"`, `crev` — generated symlinks/shims
- Strictly a launcher: resolve args + env, `exec claude`, disappear.

## Non-goals (v1)

- No wrapper behaviors: no logging, timing, or output capture. (A later
  version may add opt-in logging for `-p` profiles via spawn-and-wait; the
  config schema must not preclude it, but v1 ships nothing.)
- No interactive pickers or TUI.
- No auto-update.
- No management of claude itself (installation, auth, versions).

## Prior art

- [claude-launcher](https://github.com/paddo/claude-launcher) — Bun,
  interactive model picker, Anthropic/OpenRouter backend switching.
- [claude-code-modes](https://github.com/nklisch/claude-code-modes) — Bun,
  launcher with behaviorally tuned system prompts.
- [claunch](https://github.com/0xkaz/claunch) — session manager with tmux
  persistence; unrelated concept, but it owns the obvious name.

None offers the shortcode grammar, argv[0] dispatch, config-defined
profiles, or a dependency-free static binary. That combination is the
identity of this tool.

## Naming

Binary and repo: `clau` (`github.com/usmanbashir/clau`). The name `c` is
never the binary itself — it is one of the generated links. Management
subcommands exist only under `clau`, so user-defined shortcut names can
never collide with them.

A Rust SDK library crate named `clau` exists on crates.io; it is a library,
not a binary, and we do not publish to crates.io. Accepted.

## Configuration

One TOML file. Location: `$CLAU_CONFIG` if set, else
`${XDG_CONFIG_HOME:-~/.config}/clau/config.toml` on every platform
(macOS included — this is a terminal tool; `~/.config` is where its
audience keeps things). A missing file means built-in defaults.

```toml
# Binary to exec. Default "claude". A name resolved via PATH or an
# absolute path. Lets routers/wrappers substitute their own entry point.
claude = "claude"

[models]                # letter(s) → what --model receives, verbatim
o = "opus"
s = "sonnet"
f = "fable"
h = { model = "haiku", efforts = false }
# g = "glm-4.7"         # example user addition

# Built-in effort ladder: 1..5 = low medium high xhigh max.
# Optional override if the CLI's levels ever change:
# [efforts]
# 1 = "low"
# 2 = "medium"
# 3 = "high"
# 4 = "xhigh"
# 5 = "max"

[profiles.rev]          # persona
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "You are a meticulous code reviewer."]

[profiles.q]            # cheap one-shot pipeline tool
model = "haiku"
flags = ["-p", "--max-budget-usd", "0.25"]

[profiles.work]         # backend/account switch
env = { ANTHROPIC_BASE_URL = "https://gateway.corp.example", CLAUDE_CODE_USE_BEDROCK = "1" }

[profiles.lean]         # context loadout
flags = ["--bare", "--strict-mcp-config"]
```

Schema:

- `claude` (string, optional): global exec target, default `"claude"`.
- `[models]`: key → string (shorthand) or table
  `{ model = <string>, efforts = <bool, default true> }`.
  Keys: lowercase letters only (`^[a-z]+$`), validated at load.
  User keys merge over built-in defaults per key; defining `g` does not
  erase `o`/`s`/`f`/`h`. Redefining a default key replaces it.
- `[efforts]` (optional): key → level string. Keys must be a single
  digit `1`–`9` (the grammar reads one trailing digit). Replaces the
  built-in ladder wholesale when present.
- `[profiles.<name>]`: any subset of `model` (string), `effort` (string),
  `flags` (array of strings, passed verbatim), `env` (table of
  string → string), `claude` (string, overrides the global exec target).
  Names: `^[a-z][a-z0-9-]*$`.

Config errors (parse failure, invalid key, unknown field) fail loud with
file and line, on every code path — including plain passthrough (`c "hi"`).
A broken config never half-applies.

A profile name that shadows a grammar token (a profile literally named
`o5`) is legal; profiles win at resolution. `clau doctor` warns about it.

## Invocation and resolution

Every shortcut command name is `c` + token. `co5` carries token `o5`,
`crev` carries `rev`, bare `c` carries none. One resolver serves both
entry points.

Dispatch on `basename(argv[0])`, with any `.exe`/`.cmd` extension
stripped first (Windows):

1. `clau` — if `argv[1]` is a reserved verb (`link`, `unlink`, `list`,
   `run`, `init`, `doctor`, `completions`, `version`, `help`), run
   management. Otherwise fall through to the launcher with `argv[1:]`
   (so `clau o5 …` works with zero links installed).
   `clau run <token> [args…]` is the explicit launcher form: the token is
   required and must resolve, or it errors. Windows shims use it.
2. `c` — launcher with `argv[1:]`.
3. `c<rest>` — launcher with token `<rest>` from the name and `argv[1:]`
   as extra args. If the token does not resolve: error naming the token,
   hinting `clau list` and `clau link` (a stale link, not a passthrough —
   a generated name carries intent).
4. Any other name — error: unrecognized invocation name.

Launcher resolution, given a candidate first token:

1. `--` — everything after it goes to claude verbatim. Escape hatch.
2. Exact profile-name match — apply the profile; remaining args appended.
3. Grammar parse — if the token's last character is a digit in the effort
   ladder, the remainder must exactly match a `[models]` key; otherwise
   the whole token must. Match yields `--model <value>` and, with a
   digit, `--effort <level>`. A digit on a model with `efforts = false`
   is an error ("haiku supports no effort levels"), never a silent
   downgrade.
4. No match — nothing is consumed; all args pass to claude untouched.
   So `c "fix this bug"`, `c -c`, `c --resume` behave exactly like
   `claude …`. (Flags can never match: tokens are `[a-z0-9-]` shaped.)

Bare `c` with no args execs plain `claude`.

## Exec semantics

- argv = `[<claude>, <shortcut flags…>, <user args…>]`. For profiles,
  shortcut flags are `--model`/`--effort` (when set) followed by `flags`.
- The claude CLI (commander) takes the last occurrence of a repeated
  flag, so `c rev --model sonnet` overrides the profile's model for
  free. **Verify during implementation**; if last-wins does not hold,
  the resolver dedupes explicitly, keeping the user's occurrence.
- env = process environment overlaid with the profile's `env` (profile
  wins per key).
- Unix: `syscall.Exec` — clau replaces itself; no resident process.
  Windows: spawn, inherit stdio, wait, exit with the child's code.
- Exec target comes from profile `claude`, else global `claude`, else
  `"claude"`; resolved via PATH unless absolute. Not found → clear error.

## Management commands

- `clau link [--dir DIR] [--force]` — sync shortcut commands into DIR
  (default `~/.local/bin`; the user-profile equivalent on Windows, where
  `clau doctor` flags it if it is not on PATH):
  - Names: `c`, `c<letter>` per model, `c<letter><digit>` per model ×
    ladder (skipped when `efforts = false`), `c<profile>` per profile.
  - Collision policy: before creating a name, check PATH (and the target
    dir) for an existing foreign executable of that name. Foreign →
    warn and skip; `--force` overrides. This is the `col` lesson: a
    profile named `at` must not silently shadow `cat`.
  - Sync: create missing, prune clau-owned links whose token no longer
    resolves, report created/skipped/pruned. Ownership = the link's
    immediate target is the clau binary (by path, or basename `clau`
    after brew upgrades move the real file). Never touch foreign files.
  - Windows: write `.cmd` shims instead of symlinks:
    `@"<abs-path>\clau.exe" run <token> %*`, plus a marker comment line
    identifying clau as the owner.
- `clau unlink [--dir DIR]` — remove all clau-owned links/shims in DIR.
- `clau list` — the debugging view: every token → final claude argv
  preview and env keys it would set, and whether a link exists.
  `clau list --tokens` emits bare tokens (completion helper).
- `clau init` — write a commented starter config; refuse to overwrite
  without `--force`.
- `clau doctor` — config parses; `claude` resolvable; link dir on PATH;
  stale or foreign-collision links; profiles shadowing grammar tokens.
- `clau completions fish|zsh|bash` — completion scripts for `clau`
  (verbs) and `c` (tokens, queried live via `clau list --tokens`).
  Generated shortcut commands need no completions of their own — they
  take claude args, and default file completion suffices.
- `clau version`, `clau help`.

## Errors

All errors are one clear sentence naming the thing that failed and the
likely fix. Specifically covered: config parse failure (file:line);
stale link name; effort digit on `efforts = false` model; exec target
not found; unrecognized invocation name.

## Distribution

- GPLv3 (`COPYING` in repo). GPL does not cross the exec boundary, so
  launching the proprietary claude binary is untouched. The sole
  dependency (BurntSushi/toml, MIT) is GPL-compatible.
- goreleaser on tag push (GitHub Actions): static builds for
  linux/darwin/windows × amd64/arm64, GitHub Releases.
- Homebrew tap from day one: goreleaser publishes the formula to
  `usmanbashir/homebrew-tap` (created alongside this repo; the clau
  repo's Actions get a repo-scoped token for it).
  Install: `brew install usmanbashir/tap/clau`.
- `go install github.com/usmanbashir/clau@latest` works from the first
  push.
- README leads with the pitch and shows the four profile directions as
  copy-paste examples: persona, backend/env switch, pipeline tool,
  context loadout.

## Implementation shape

Stdlib plus one dependency (BurntSushi/toml). No CLI framework —
passthrough semantics fight cobra-style parsers. Layout:

- `main.go` — argv[0] dispatch, reserved verbs
- `config.go` — load, validate, merge defaults
- `resolve.go` — token → argv + env
- `link.go` — link/unlink/sync, collision checks, Windows shims
- `commands.go` — list, init, doctor, completions, version, help

## Testing

- Table-driven unit tests: grammar parse, resolution precedence, config
  merge, env overlay, collision policy.
- Integration: a fake `claude` on PATH (script dumping argv + env),
  golden-file assertions across invocation styles; covers exec on unix
  and spawn on Windows.
- CI matrix: ubuntu / macos / windows. goreleaser config validated by a
  dry run in CI.

## To verify during implementation

- Commander last-flag-wins behavior in the claude CLI (drives the
  override story; fallback is explicit dedupe).
- Current `--effort` level names against `claude --help` at release
  time (ladder defaults must match).
