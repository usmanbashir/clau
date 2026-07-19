# Per-project config — design

A repo-committable `.clau.toml` that layers over the global config, so
the same shortcut launches with project-appropriate model, flags, and
environment — and a team can share launch profiles by committing the
file. Target release: v0.2.0.

Origin: not in the v1 spec (whose only deferred item was opt-in `-p`
logging). Motivated by the original pain — different projects need
different launches — and by making clau useful to teams, not just one
person's dotfiles replacement.

Decisions fixed in session interview (2026-07-19): team scope
(repo-committed file), TOFU trust via `clau trust`, and no linking of
project-only tokens.

## File and discovery

- Filename: `.clau.toml`, at the project root. Same TOML schema as the
  global config — `claude`, `[models]`, `[efforts]`, `[profiles]` —
  with the same validation; unknown keys are errors.
- Discovery: at launch, walk from the current directory up to the
  filesystem root. The nearest `.clau.toml` wins and is the only one
  applied. No stacking of files found further up.
- `CLAU_NO_PROJECT=1` skips discovery entirely (scripts, CI, escape
  hatch).
- Edge: if the discovered file is the same file as the resolved global
  config (e.g. `CLAU_CONFIG` points into the repo), the project layer
  is skipped — one file never applies twice.

## Merge semantics

Effective config = defaults ← global ← project. Each rule extends the
existing global-over-defaults behavior one layer:

- `[models]`: per-key merge. A project key replaces that key only.
- `[profiles]`: per-name; a project profile with the same name as a
  global one replaces it wholesale. No field splicing — what the
  project file says is what runs.
- `[efforts]`: a non-empty table replaces the entire ladder (matches
  the existing global behavior).
- `claude`: project value wins if set.

Token resolution is untouched; layering happens entirely at config
load. A globally linked command (`crev`) picks up the project's `rev`
override automatically because resolution is cwd-aware.

## Trust (TOFU)

Threat: a repo-committed file that can set env
(`ANTHROPIC_BASE_URL` exfiltration), inject flags
(`--dangerously-skip-permissions`), or swap the exec target
(`claude = "./evil"`). Cloning a repo and typing `cs3` must not be an
attack. Same problem direnv has; same solution.

- State: `${XDG_STATE_HOME:-~/.local/state}/clau/trust.toml` — clau's
  first state file. A `[trusted]` table mapping the config file's
  absolute path to the SHA-256 of its content.
- Launch paths (`c`, `c<token>`, `clau <token>`, `clau run`) with a
  discovered project file:
  - trusted and hash matches → layer applies;
  - unknown, or content changed → hard error naming the path, the
    reason, and the fix (`clau trust`, after reviewing with
    `clau trust --show`).
- Content-hash pinning means edits and symlink-target swaps re-prompt.
  An empty file still gates: one rule, no exceptions.
- Read-only commands are exempt: `clau list` and `clau doctor` work
  untrusted but announce "project config present, NOT trusted" and
  show the global-only view. Diagnostics must not require trust;
  launching does.
- Trust-store corruption: treated as empty, with a doctor warning.
  Never a crash.

### New verbs

- `clau trust` — pin the discovered project file (path + content
  hash); prints what was trusted. `--show` prints the file's path and
  content for review without trusting. Errors if no project file is
  discovered.
- `clau untrust` — remove the entry for the discovered file. Errors if
  none.
- Both join `reservedVerbs`. A user profile named `trust` keeps
  working as `c trust` / `ctrust`; only the `clau trust` spelling is
  claimed.

## Provenance UX

- `clau list`: when a project file is discovered, a header line names
  it and its trust state, and the table gains a `SOURCE` column
  (`global` / `project`) showing where each token's effective
  definition comes from — a project-overridden global profile reads
  `project`. With no project file, output is unchanged from v0.1.x.
- `clau doctor`: reports the discovered file, trust status (including
  hash-changed), which tokens the project layer adds or overrides, and
  warns about trust entries whose files no longer exist.

## Linking

Unchanged. `clau link` links global-config tokens only. Project-only
profiles are reachable argument-style (`c deploy`, `clau run deploy`)
inside the project. Nothing to prune on `untrust`; teams wanting
command-style ergonomics mirror the profile name in their global
config and let the project layer override it.

## Errors

- Project file parse/validation error: hard error naming the file,
  identical in shape to global config errors.
- Untrusted / changed: hard error as above (launch paths only).
- All existing error paths unchanged when no `.clau.toml` exists —
  v0.1.x behavior is byte-identical.

## Testing

- Unit: discovery walk (nested cwd, root, symlinks, `CLAU_NO_PROJECT`,
  global==project edge); each merge rule (models per-key, profiles
  wholesale, efforts whole-table, claude); trust store round-trip,
  hash mismatch, corrupt store; error texts.
- Integration: existing fake-claude argv harness with a temp global
  config plus a project file — trusted launch picks up overrides,
  untrusted launch fails, `CLAU_NO_PROJECT=1` ignores the layer;
  `clau list` SOURCE column and untrusted banner; doctor findings.
- Windows: same paths logic (`~/.local/state` fallback mirrors the
  existing `~/.config` choice); shims unaffected.

## Non-goals (v0.2)

- No stacking of multiple project files up the tree.
- No linking of project-only tokens; no per-project bin dirs.
- No auto-trust, trust-by-glob, or trust enumeration command (doctor's
  stale-entry warning covers hygiene for now).
- No new config keys; the schema is the existing one, in a second
  location.
- `CLAU_CONFIG` semantics unchanged (global override only).

## Approved

Design presented and approved in session, 2026-07-19.
