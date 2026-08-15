---
status: todo
created: 2026-08-15
links: []
---

# the pull request body is a digest, with the job cost

## Context

## Decisions

- 2026-08-15 altsoyuz: work's pull request body is, in this order: jalon digest -offline <id> run inside the worktree, git diff --stat, the last line of the criterion output, the job cost in USD; review's body already orders measured, contradicted, cost, plan and gains the cost line only. No new verb and no new flag in the core: the agent composes what exists
- 2026-08-15 altsoyuz: the phases switch to claude --output-format json so total_cost_usd is read per phase and summed; text output carries no cost, and today no file in agent/ records a dollar amount although the kill criterion is stated in dollars per merged branch
- 2026-08-15 altsoyuz: the cost line also goes to JALON_METRICS as one field on the agent verbs' line, so docs/measuring.md can read it with jq like everything else; no aggregation in the binary

## Log

- 2026-08-15 altsoyuz: measured problem: the human review of a work PR is the most expensive minute of the loop and the body says only the criterion passed and a file count. Depends on nothing; second in the order because it makes every later job cheaper to read.
