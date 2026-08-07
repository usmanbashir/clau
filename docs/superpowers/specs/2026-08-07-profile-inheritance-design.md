# Profile inheritance via [bases] — design

A `[bases]` table of abstract profile fragments that profiles opt into
with `extends`, so a family of related profiles (five Ollama models,
one backend) states the shared env and flags once. Target release:
v0.3.0.

Origin: issue #1. Pointing several profiles at one alternate backend
repeats the same `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, and
`flags` in every profile — ~30 lines where 12 would do, and a host
change means editing every block.

Decisions fixed in session interview (2026-08-07): bases are always
abstract (never runnable, never linked — hence a dedicated table, not
profile-to-profile extends); early binding (`extends` resolves when the
declaring file is applied, then flattens away); flags concatenate
(base's first, child's appended).

## Schema

```toml
[bases.ollama]
flags = ["--bare", "--strict-mcp-config"]
env = { ANTHROPIC_BASE_URL = "http://ollama.lan:11434", ANTHROPIC_AUTH_TOKEN = "ollama" }

[profiles.q27]
extends = "ollama"
model = "qwen3.6:27b"
env = { ANTHROPIC_DEFAULT_HAIKU_MODEL = "qwen3.6:27b" }
```

- `[bases.<name>]` carries the same five fields as a profile —
  `model`, `effort`, `flags`, `env`, `claude` — and nothing else. A
  base has no `extends` of its own, so chains are structurally
  impossible.
- Base names use the profile name rule (`^[a-z][a-z0-9-]*$`) but live
  in their own namespace: bases generate no commands, no tokens, no
  list rows, and no shadowing warnings. A base and a profile may share
  a name without interacting.
- `profiles.<name>.extends` names a base — never a profile, never a
  model letter.

## Flattening semantics

Resolution happens in `applyConfigData`, at the moment the declaring
file is applied:

- `model`, `effort`, `claude`: the child's value where set, else the
  base's.
- `flags`: the base's list, then the child's appended. claude's own
  parsing lets later flags win, so a child can override a base flag by
  restating it.
- `env`: key-wise — the base's keys, the child's overriding per key.

The `extends` link is then discarded. The in-memory `Profile` struct is
unchanged from today, so `resolveToken`, linking, previews, launch, and
trust code are untouched by construction.

Within one file, the file's `[bases]` entries first replace accumulated
bases wholesale per name; then the file's profiles flatten against that
accumulated base set (earlier layers plus this file).

## Layering

- Bases replace wholesale per name across layers (global → project),
  the same rule as profiles.
- Early binding: global profiles flatten against global bases at global
  load. A project base override therefore reaches only profiles the
  project file itself declares — the trust invariant that a project
  file can only alter profiles it declares is preserved. A repo that
  wants its own `q27` declares `q27` (with `extends`) itself.
- A project profile may extend a global base: resolution sees
  everything accumulated so far. The reverse cannot happen — a global
  profile extending a base no file has yet declared is a load-time
  error; the global config must be self-contained.

## Errors

All at load time, naming the file, in the style of existing config
errors:

- Unknown base: `profile "q27": extends unknown base "ollma"`. When
  the name matches a declared profile, say so: `"rev" is a profile;
  extends references [bases]`.
- Empty `extends = ""`: an error, like other empty-value errors.
- `extends` inside a `[bases.*]` block: rejected by the existing
  unknown-key check (`md.Undecoded`) for free.

## Trust

No changes. Early binding keeps `trust --show`'s raw-bytes review
honest: what the project file declares is exactly what it can affect.
Inherited env surfaces where env already surfaces — the LAUNCHES
preview in `clau list` — because profiles are flattened before display.

## Surface changes

- `config.go`: `Config.Bases`, `rawConfig.Bases`, `rawProfile.Extends`,
  the flatten step, and the new errors.
- Starter config (`commands.go`): rework the commented Ollama example
  into a base plus two children — the feature's own motivating case.
- README: the Profiles section gains a base+child example; the
  project-layer merge rules gain "bases replace wholesale per name".
- Untouched: `link.go`, `completions.go`, `resolve.go`, doctor
  findings, provenance (`SOURCE` column), `projectDeclarations`.

## Tests

- `config_test.go`: scalar fill-in vs child-wins; flag order; env
  key-wise merge; unknown-base error and its profile-name variant;
  empty `extends`; `extends` rejected inside `[bases.*]`.
- `project_test.go`: project profile extends a global base; a project
  base override does not reach global children; it does reach children
  the project file declares; wholesale base replacement across layers.
- `link_test.go`: no `c<base>` command is generated.
- `integration_test.go`: one layered end-to-end fixture — global bases
  and children, project override on top.

## Non-goals

- No removal mechanism: a child can override inherited values but
  cannot unset an env key or drop a flag. (Consequence: a child of a
  base that sets `model` cannot return to model-less.)
- No chains (bases cannot extend), no inheritance among `[models]`,
  no unused-base diagnostics.
