---
status: done
created: 2026-08-14
links: []
---

# the setup must not lie to you

## Context

## Decisions

- 2026-08-14 altsoyuz: doctor stops counting untracked files as a dirty tree. The configuration it reads is itself untracked on a machine that does not version it, so every tick warned forever, and a warning that is always on is one nobody reads. This is the same reasoning that made a dirty tree a warning rather than a failure.
- 2026-08-14 altsoyuz: agent-init writes the configuration beside an existing one rather than over it, and prints the diff command. The script tells the reader to edit the probes, so a regeneration that overwrote the file would destroy exactly the work it asked for.

## Log

- 2026-08-14 altsoyuz: the alternative for the untracked warning was to commit .jalon/agent.toml in this repository, which the design says is versioned in each target repo. Rejected: the file is untracked in the agent's clone, so committing it would make git pull --ff-only refuse to fast forward, and the unit ignores pull failures, so the server would have gone silently stale.
- 2026-08-14 altsoyuz: closed.
