# clau list UX redesign + clau show — design

Date: 2026-08-12
Status: approved (interview + design review in session)

## Problem

`clau list` prints one tabwriter row per token with a LAUNCHES column
holding the full command line plus an `[env: …]` name dump. With a
realistic config (~44 tokens: three effort families, ~24 profiles
sharing one Ollama base) that means:

- ~230-char lines that wrap 2–3× in a normal terminal, destroying the
  table alignment entirely.
- The same `--bare --strict-mcp-config [env: ANTHROPIC_…]` text
  repeated on ~24 rows.
- 18 rows spent enumerating three effort families (`o o1..o5`, …).
- A COMMAND column that is always `c`+token and a LINKED column that
  is `yes` on nearly every row.
- Alphabetical sort interleaving unrelated tokens (`f5`, `fab`, `g12`).

## Goals (ranked, from user interview)

1. **Recall** — answer "what did I call the gemma one?" at a glance.
2. **Inventory/health** — project layer, trust state, link state.
3. **Showpiece** — the output appears in the README demo gif.

Non-goal: per-token verification detail in the list. That moves to a
new `clau show <token>`.

## Non-goals

- No `--full`/verbose list mode (rejected in interview; `show` covers it).
- No change to `clau list --tokens` (the machine interface; completions
  depend on it) or to `--dir`.
- No `clau doctor` changes.
- No backend host in group headers (declined in interview).
- Regenerating `docs/demo.gif` is a follow-up, not part of this change.

## Design: `clau list` (default, human view)

Sections in order, blank-line separated, each aligned independently
(one tabwriter per section):

1. **Project header** — only when a project file exists. Wording is
   unchanged from today; trust state gains color:
   `project: /path/.clau.toml (trusted)` — `(trusted)` green;
   the `changed, NOT applied — run \`clau trust\` to re-allow` and
   `NOT trusted, NOT applied — run \`clau trust\`` states render bold
   yellow. These must stay loud.
2. **MODELS** — one row per `[models]` letter (alphabetical), not per
   grammar token. Header carries the grammar legend once, derived from
   the effective `[efforts]` table:
   `MODELS    letter alone = model, +digit 1-5 = effort low..max`
   - Digit span: sorted digits, rendered `1-5` when contiguous, else a
     comma list (`1,3,7`). Endpoint labels are the first and last
     digits' effort values.
   - Row: `<letter>  <model>`; letters with `efforts = false` get a
     dim `no effort digits` note.
3. **One section per base** (bases alphabetical) for profiles that
   extend it: header is the base name uppercased plus a dim `base`
   tag (`OLLAMA  base`). Profiles within: alphabetical by token.
4. **PROFILES** — base-less profiles, alphabetical. Omitted when empty,
   as are base sections with no members.
5. **Footer** — one dim line:
   - all linked: `all 43 commands linked in ~/.local/bin · 1 project-only · details: clau show <token>`
   - missing links: the count segment turns yellow and gains the fix:
     `40 of 43 commands linked in ~/.local/bin — run clau link · …`
   - The `N project-only` segment is omitted when zero. The directory
     is the `--dir` value.

**Profile rows**: `<token>  <model>` plus, when set, a dim `(<effort>)`
after the model (persona profiles like `rev` pin effort — that is
recall-relevant). A profile with no model (env/flags-only, e.g. a
gateway switch) renders a dim `(env/flags)` in the model cell.

**Badges** (after the model cell):

- `project` (dim) — token defined or overridden by the applied project
  layer (same rule as today's SOURCE column, i.e. `tokenSource`).
  Never shown when the project layer isn't applied (its tokens don't
  exist then).
- Link state. *Linkable* means `clau link` would create the command:
  the token is in the **global** config's token set (`cmdLink` links
  from `mustLoadConfig()`, global only).
  - Linkable and not linked → yellow `not linked — run clau link`.
  - Not linkable (project-only) and not linked → dim `· c <token>`
    launch hint instead — expected state, not a warning.
  - Linked → no badge. Linked is `isOwned` on `<dir>/<linkFileName>`,
    as today.

**Sorting** is alphabetical everywhere (TOML map order is not
preserved by the parser; determinism wins).

Rendering example (user-approved mockup, excerpted):

    project: .clau.toml (trusted)

    MODELS            letter alone = model, +digit 1-5 = effort low..max
      o     opus
      s     sonnet
      f     fable
      h     haiku     no effort digits

    OLLAMA  base
      fab   fable-fusion:iq2m-48k
      g12   gemma4:12b
      qw35  qwen3.6-35b-a3b:q2kxl-64k

    PROFILES
      gma   gemma4:12b   project · c gma

    all 43 commands linked in ~/.local/bin · 1 project-only · details: clau show <token>

## Design: color policy

Hand-rolled 8-color ANSI, no new dependency. Palette and uses:

- bold — section headers
- bold cyan — tokens (the recall anchor)
- dim — legends, notes, badges, footer
- green — `(trusted)`
- yellow — actionable problems only: trust warnings, missing links

Color is enabled only when all hold: stdout is a character device
(`os.Stdout.Stat()`), `NO_COLOR` is unset, `TERM != "dumb"`. On
Windows, additionally require an ANSI-capable terminal heuristic
(`WT_SESSION` set, or `TERM_PROGRAM == "vscode"`, or `TERM` set) —
otherwise plain. Disabled color means empty escape strings: the piped
layout is byte-identical to the plain mockup, same structure as TTY.
Output uses UTF-8 literals (`·`, `→`, `•••`), consistent with the
modern-terminal assumption the tool already makes.

## Design: `clau show <token>`

New management verb. Exactly one positional arg plus `--dir DIR`
(default `defaultLinkDir()`, used for the linked line).

    qw35f → cqw35f
      kind      profile · extends base: ollama
      source    global (~/.config/clau/config.toml)
      linked    yes → ~/.local/bin/cqw35f
      launches  claude --model qwen3.6-35b-a3b:q2kxl-256k --bare --strict-mcp-config
      env       ANTHROPIC_AUTH_TOKEN=•••
                ANTHROPIC_BASE_URL=http://localhost:11434
                ANTHROPIC_DEFAULT_HAIKU_MODEL=qwen3.6-35b-a3b:q2kxl-256k
                CLAUDE_CODE_MAX_CONTEXT_TOKENS=64000

Line rules:

- **header** — `<token> → c<token>`, token in cyan.
- **kind** — one of:
  - `model letter: opus · effort digits 1-5` (or `· no effort digits`)
  - `grammar: o (opus) + 3 (effort high)`
  - `profile` plus ` · extends base: <name>` when provenance exists.
- **source** — `global (<loaded global config path>)` or
  `project (<.clau.toml path>)`, per `tokenSource`.
- **linked** — `yes → <path>` when `isOwned`; else `no` plus, for
  linkable tokens, yellow `— run clau link`, and for project-only
  tokens, dim `— project profiles launch as c <token>`.
- **launches** — resolved binary + flags (`resolveToken` output, the
  old `previewFor` body minus env). No manual wrapping; the terminal
  wraps and the line stays copy-pasteable.
- **env** — omitted when empty; sorted keys, one `KEY=value` per line.
  **Masking**: a value renders as `•••` when any `_`-separated
  segment of its variable name equals (case-insensitive) `TOKEN`,
  `KEY`, `SECRET`, or `PASSWORD` — so `ANTHROPIC_AUTH_TOKEN` masks
  while `CLAUDE_CODE_MAX_CONTEXT_TOKENS` (segment `TOKENS`) does not.
  Masking applies in every mode — TTY or piped — so a pasted
  `clau show` never leaks a gateway credential. No unmask flag; the
  config file is the source of truth.

Argument handling:

- Literal token resolved first; if absent and the arg starts with `c`
  and the rest is a token, show that (`clau show cqw35f` works).
- Unknown token → `fatal` (stderr, exit 1) with `run \`clau list\``
  hint, plus `did you mean "<t>"?` when exactly one token has the arg
  as a prefix.
- A token that exists but fails `resolveToken` → `fatal` with the
  resolver's error.
- Zero or multiple positional args → `fatal` usage message.

Adding `show` to `reservedVerbs` shadows a hypothetical profile named
`show` for the `clau show …` invocation only — the same standing
precedent as `list`, `run`, etc.; `c show` still launches it.

## Internals

- **Provenance**: `Profile` gains `Extends string`, set where a
  profile's `extends` is flattened against `[bases]` at file-apply
  time. Display-only; resolution behavior unchanged. Layering rule
  holds automatically: a project profile replacing a global one
  carries its own provenance (possibly empty) because profiles replace
  wholesale per file.
- **Palette helper**: small struct of escape-code strings (bold, dim,
  cyan, green, yellow, reset); constructor returns all-empty when
  color is off. Lives with the commands (~30 lines).
- **List refactor**: `cmdList` builds sections directly from
  `cfg.Models` / `cfg.Profiles` (grouped by `Extends`) — it no longer
  calls `resolveToken` per token. `previewFor` moves into `show` as
  the launches line. The `listRow` flat table, and the TOKEN / COMMAND
  / LINKED / SOURCE / LAUNCHES columns, are deleted. `tokensOf` is
  unchanged (used by `--tokens`, completions, link).
- **Dispatch**: `show` added to `reservedVerbs`, `runManagement`, the
  help text's Management block, and completions (fish/zsh/bash) with
  token completion in the `show` argument position, mirroring `run`.

## Errors

Unchanged for `list` (config-load failures already `fatal`). `show`
errors per the argument-handling rules above. Exit codes: 0 success,
1 via `fatal` — the existing convention.

## Testing

Extend the existing table-driven tests:

- Section grouping and order: models, bases A→Z, base-less profiles;
  empty sections omitted.
- Legend derivation: default ladder, custom contiguous, custom
  non-contiguous digits.
- Rows: effort-pinned profile, model-less profile, `efforts = false`
  note.
- Badges: project, linkable-unlinked (yellow), project-only-unlinked
  (hint), linked (none).
- Footer counts: all-linked, some-missing, zero project-only.
- Color: palette on/off produces colored vs byte-identical-plain
  output (inject the palette / env in tests).
- `Extends` provenance: survives global load, project wholesale
  replace resets it; recorded for project profile extending global
  base.
- `show`: each kind line, source, linked variants, launches, env
  masking (name-pattern cases), `c`-prefix fallback, did-you-mean,
  unknown token, arg-count errors.
- Completions scripts mention `show`.

## Docs

- README: `clau list` line and Commands table gain `show`; the
  list description shifts from "every token → what it launches" to
  recall phrasing; short `show` mention where verification is
  discussed (trust/per-project sections reference reading launches).
- `helpText` in main.go: add
  `clau show <token> [--dir DIR]   everything about one shortcut`.
- Demo gif regeneration: follow-up task, out of scope here.
