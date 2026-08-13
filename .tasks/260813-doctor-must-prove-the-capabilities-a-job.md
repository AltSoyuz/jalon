---
status: doing
created: 2026-08-13
links: []
---

# doctor must prove the capabilities a job uses

## Context

## Decisions

- 2026-08-13 altsoyuz: doctor checks the environment, not the capabilities a job exercises, and two real failures on a fresh server proved the cost: a missing model login and an unset git identity both passed eight green checks and failed after three model calls had been paid for. A check earns its place when it prevents paying to discover something.

## Log

- 2026-08-13 altsoyuz: git-identity check added: a fresh system user has no git identity, so review writes the task and dies at the commit. Found on homeserver exactly that way. agent-init now sets an identity in the unprivileged block, distinct from the owner's so the agent's commits are recognisable in git log, and only when unset so a re-run never clobbers a chosen one.
- 2026-08-13 altsoyuz: still missing: doctor does not prove the model answers. The plan named a doctor -live that spends a capped call, and it is the remaining gap of the same class. Measured floor for one claude -p invocation on the server is above 0.05 USD, so a live check costs real money and must stay opt in.
