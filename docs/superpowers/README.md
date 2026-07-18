# Design and build records

Point-in-time artifacts from clau's v1 development, kept as history:

- `specs/` — the approved design: grammar, resolution precedence, config
  schema, and the reasoning behind the decisions (exec-only, GPLv3,
  Homebrew cask, symlink ownership rules).
- `plans/` — the task-by-task implementation plan the v1 build followed,
  executed by Claude Code subagents with a code review between tasks.

**The code is authoritative; these documents are not updated.** Where a
record and the source disagree, the source has moved on — post-v1
changes (the dangling-only ownership fallback refinement, the
formula→cask migration, later polish) are visible in git history, not
here.
