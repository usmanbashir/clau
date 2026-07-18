# README rewrite — design

The launch README explained clau to people who already understood it.
This rewrite makes a stranger understand the tool, its benefit, and how
to start, inside a minute — without losing the reference depth or voice.

## Diagnosis of the old README

- Tagline named the solution ("model×effort shortcodes"), not the
  problem. Meaningless to a visitor who doesn't yet know what "effort"
  is here.
- The pain — retyping flag incantations, or re-picking model/effort in
  the `/model` TUI for every project's needs — was never stated.
- Maintainer-brain detail too early: Windows exec semantics in sentence
  three, fish `conf.d` autoloader mechanics mid-doc.
- The before/after contrast (the sell) was buried in code comments.
- The obvious objection — "I'd just use shell aliases" — was never
  answered, though clau's whole existence answers it.
- Factual bug: `brew install` was marked "macOS and Linux", but the tap
  ships a homebrew *cask*; casks are macOS-only.

## Decisions (from interview)

- **Hook**: pain-first. Different tasks want different launches; that's
  long flags or another `/model` picker trip, every time. clau makes
  the choice the command name. Before/after block is the headline.
- **Demo**: yes — ~20s asciinema/GIF slot directly under the hook,
  HTML-comment placeholder until recorded. Recording script delivered
  alongside this rewrite (not in the repo).
- **Shape**: hybrid. First screen for the stranger (hook → demo →
  install → quick start), current density and voice below the fold
  (grammar → profiles → reference).

## Structure

1. Title + CI badge + release-version badge.
2. Hook: pain paragraph → "clau makes the choice the command name" →
   five-line shortcode block (`co5`, `cs1`, `crev`, `c s3`, passthrough).
3. Demo placeholder (HTML comment).
4. Install: brew (macOS only — cask), `go install`, releases page;
   `clau dev` stamping note shrinks to a parenthetical.
5. Quick start: `clau init` → `clau link` → `co5`, ending on the
   payoff; `clau doctor` as the fix-it pointer.
6. The grammar: unchanged in substance; keeps the haiku
   errors-instead-of-downgrades trust signal and the one-line model
   addition example.
7. Profiles: the four-example TOML block survives nearly untouched
   (best part of the old doc); flags-always-win and `c --` passthrough.
8. Why not shell aliases? (new): origin as fish functions, generalized;
   one binary + one TOML across shells and Windows; linked shortcuts
   are real commands usable by editors/scripts; profiles bundle
   model/effort/flags/env as data; completions; collision protection.
   Placed after Profiles, once the reader knows what they'd be
   aliasing — not up top where it interrupts install momentum.
9. Config + Commands: reference, tightened; fish `conf.d` note
   compressed to one clause.
10. How it works (new, short): argv[0] dispatch, strictly exec,
    no wrapper/runtime/telemetry; Windows spawn-and-wait parenthetical
    relocated here from the old intro.
11. License: unchanged.

## Out of scope

- No logo, no docs site, no restructuring of help text or starter
  config (they already align).
- Demo recording is the author's step; README carries a placeholder
  until then.

## Approved

Full draft presented and approved verbatim in session, 2026-07-19.
