---
status: doing
created: 2026-08-06
links: [README.md, digest.go]
---

# Measure digest against a forge

## Context

This task carries the measurement that validates or kills the product: tokens
and tool calls until an agent is oriented, `jalon digest` against `gh issue view`
against free exploration, on real tasks, over two weeks.

The counting is half free already: `digest` prints its own size and duration on
stderr on every run. What has to be collected by hand is the forge side and the
number of tool calls an agent makes when left to explore on its own.

Refutation condition: if `digest` does not clearly win, the product reduces to
its conventions (the id in the commit message), which cost nothing and stay.
The verbs would then be worth deleting, not defending.

## Decisions

- 2026-08-06 altsoyuz: the id is YYMMDD-slug, not MMDD; a date that repeats
  every year would break `git log --grep` from the second year of use.
- 2026-08-06 altsoyuz: no markdown parser dependency; render implements a
  documented subset and leaves everything else as escaped literal text.
- 2026-08-06 altsoyuz: four source files rather than one, split by group of
  verbs; the criterion is holding the tool in your head, not the file count.
- 2026-08-06 altsoyuz: compact truncates the Log but never rewrites Context;
  this tool holds no model and calls none.
- 2026-08-06 altsoyuz: record the raw line, ship no stats verb: which averages
  matter is unknown, and a summary written today would freeze that guess into
  code

## Log

- 2026-08-06 altsoyuz: first working version, six verbs, no dependency.
- 2026-08-06 altsoyuz: render measured at about 120 ms for 500 tasks of about 5
  KB on an Apple M3, so a full rewrite every time stands and incremental is not
  earned.
- 2026-08-06 altsoyuz: measurement not started yet; it needs two weeks of real
  tasks.
- 2026-08-06 altsoyuz: the instrument for this measurement is
  [[260806-opt-in-self-observability]]; the first four lines it recorded are its
  own tests, not real orientations, and must be dropped before reading.
