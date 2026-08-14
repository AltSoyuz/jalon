---
status: done
created: 2026-08-14
links: []
---

# the jalon checkout is not the target

## Context

## Decisions

- 2026-08-14 altsoyuz: agent-init takes -jalon, the checkout the unit pulls, builds and runs, separately from -repo, the target the agent works in. They are the same repository only when the target happens to be jalon; the flag defaults to the target so the single target case stays a one liner.

## Log

- 2026-08-14 altsoyuz: found before it ran, while adding two Astro sites as targets: neither has a Makefile build target, so the fatal ExecStartPre would have failed at every tick and ExecStart pointed at a binary that does not exist. The self updating unit was written when the only target was jalon itself, and conflated the two roles.
- 2026-08-14 altsoyuz: closed.
