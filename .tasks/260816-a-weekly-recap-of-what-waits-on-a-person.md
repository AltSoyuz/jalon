---
status: done
created: 2026-08-16
links: []
---

# a weekly recap of what waits on a person, without a model

## Context

## Decisions

- 2026-08-16 altsoyuz: a shell script versioned in jalon, run by a weekly timer beside the hourly ones, no model: the material for a direction is already measured (stale doing tasks, proposals nobody queued, waiting pull requests, wrecks, decisions whose linked files moved, the two kill numbers) and only needs gathering; a model that reads digests and writes tasks would be the backlog nobody understands, and stays refused
- 2026-08-16 altsoyuz: delivery is one command on stdin (-notify), the same idea as [notify]; absent, stdout and the journal; the recap spans private repositories so it is never posted to a public place by default

## Log

- 2026-08-16 altsoyuz: the merged-branches recipe in docs/measuring.md asked the forge for the commits of 200 pull requests in one query, which exceeds its node limit; measured while writing the script, both now list cheaply and read twenty. Test: TestWeeklyRecapScript runs the script against a fixture with a stub gh.
- 2026-08-16 altsoyuz: closed.
