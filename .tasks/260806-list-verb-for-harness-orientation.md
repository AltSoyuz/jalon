---
status: done
created: 2026-08-06
links: [main.go, docs/workflow.md]
---

# List verb for harness orientation

## Context

There is no zero argument command answering "where are we". `digest` needs an
id, `render` writes files. Anyone wiring jalon into an agent harness therefore
starts with shell glue: grep `status: doing`, extract the id, call digest. That
glue would live in every user's config, duplicate, and break when the format
moves, which is exactly what this tool claims to remove.

Two independent paths hit the same wall on the same day: asking how to look at
the tasks got answered with "open a browser", and the missing session start hook
has nowhere to point. That is the evidence, not a hunch.

`list` is the cheap half of a two step orientation: about fifteen tokens per
task to see the menu, two thousand to digest the one that matters. Injecting
full digests at every session start would be the waste jalon exists to remove.

## Decisions

- 2026-08-06 altsoyuz: one line per task on stdout, nothing else, so a harness hook is `jalon list -status doing` with no glue. The empty case notes on stderr and keeps stdout clean.
- 2026-08-06 altsoyuz: no current task state, no `jalon use <id>`. It would save six characters and introduce "why did it write to the wrong task". Hidden state contradicts a tool debugged with cat.
- 2026-08-06 altsoyuz: jalon enforces nothing. A task manager that refuses a command because you did not digest first is a task manager people uninstall. Enforcement belongs to harness hooks, which run whatever the agent decides.
- 2026-08-06 altsoyuz: the hook recipe ships as a documented command shape, not as a vendor specific file. Any harness can run a command and inject its stdout.

## Log

- 2026-08-06 altsoyuz: task written before the code this time, which is the point of the AGENTS.md rules added in the previous commit.
- 2026-08-07 altsoyuz: landed with tests; the empty case is stdout empty, stderr explains, exit 0, which is what a hook needs
