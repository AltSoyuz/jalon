---
status: done
created: 2026-08-15
links: []
---

# the kill criterion is computed from metrics, not remembered

## Context

## Decisions

- 2026-08-15 altsoyuz: no aggregation in the binary, as today: docs/measuring.md gets one jq recipe over JALON_METRICS plus gh pr list --state merged that yields, over the last twenty work jobs, the share of branches merged with no human commit after jalon's and the dollars per merged branch, the two numbers written in advance in docs/agent.md under What would kill it
- 2026-08-15 altsoyuz: jalon proposes and never flips: if one day a repository's numbers hold and auto-merge is wanted, the unlock is a pull request editing .jalon/agent.toml that a person merges; nothing in the binary changes its own rights on a measurement, which keeps the rule that no automatism alters availability or rights without an explicit owner decision

## Log

- 2026-08-15 altsoyuz: depends on the cost field from 260815-the-pull-request-body. Fifth in the order; it is a documentation recipe and one field, not a feature.
- 2026-08-15 altsoyuz: implemented: two recipes in docs/measuring.md (merged work branches with exactly one commit over the last twenty, from gh pr list; dollars per merged branch from JALON_METRICS cost_usd joined on the id with awk), the kill section of docs/agent.md points at them and states that jalon proposes and never flips. Both recipes dry-run on synthetic data.
- 2026-08-15 altsoyuz: closed.
