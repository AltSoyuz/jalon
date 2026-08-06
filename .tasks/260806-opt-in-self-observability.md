---
status: done
created: 2026-08-06
links: [metrics.go, docs/measuring.md]
---

# Opt in self observability

## Context

The instrument for 260806-measure-digest-against-a-forge. jalon prints one
measurement line on stderr per digest, but nothing accumulated it, so the two
week comparison had no data behind it.

What jalon can observe is its own cost. It cannot observe the counterfactual:
tool calls and free exploration happen in the agent harness, not in this
process. That half of the comparison is a manual protocol, written down in
docs/measuring.md rather than pretended away in code.

This task was written after the code, not before. See the last log entry.

## Decisions

- 2026-08-06 altsoyuz: record the raw line, ship no aggregation verb. Which averages matter is not known yet, and a summary written today would freeze that guess into code. `jalon stats` earns itself the day the same jq one liner is typed a third time.
- 2026-08-06 altsoyuz: opt in through $JALON_METRICS, nothing written when unset. A tool whose pitch is that no data leaves the repository does not get to write telemetry nobody asked for.
- 2026-08-06 altsoyuz: the file lives outside the repository by default. A measurement is local to a machine, and committing it would conflict on every run for data nobody reads in a diff.
- 2026-08-06 altsoyuz: a metrics write failure warns on stderr and never fails the command, with a test that proves it. The measurement observes the tool, it does not get to break it.
- 2026-08-06 altsoyuz: one short line under O_APPEND is atomic on POSIX, so parallel loops cannot interleave. No lock, no lock file.

## Log

- 2026-08-06 altsoyuz: landed with tests; the first reading already showed 260806-measure costs 3461 tokens against 2308 for the others, entirely from linking README.md.
- 2026-08-06 altsoyuz: process failure worth recording. I wrote the code before creating this task, never ran digest to orient myself, and wrote the decisions after implementing them. The repository claims to be jalon's first user and its AGENTS.md never said so; that gap is now closed in the same commit.
- 2026-08-06 altsoyuz: entries are kept on one physical line here, as jalon append writes them. Hand wrapping them broke inline code spans twice.
