---
status: done
created: 2026-08-06
links: [github.go, docs/workflow.md]
---

# GitHub bridge without a sync

## Context

Open question when the open source layer was added: should tasks and GitHub
issues synchronize? Answer: no. A sync means two writable stores with no clear
owner, so a token, a mapping table, a conflict rule and something scheduled to
run. That contradicts the three properties the tool exists for, and it solves a
problem nobody has yet: zero external users today.

What is real is the friction of retyping an issue by hand. So the bridge is one
way, on demand, stateless, and confined to `github.go`: seed a task from an
issue, show the thread when reading. Closing needed no code at all, because
GitHub and the post-merge hook each already close their own side from a pull
request body.

## Decisions

- 2026-08-06 altsoyuz: no synchronization. Two writable stores would need a
  token, a mapping table, a conflict rule and a schedule, and would break "the
  files are the truth", "no network on the hot path" and "cat plus git log debug
  it".
- 2026-08-06 altsoyuz: `jalon new -issue N` is one gh call producing a snapshot;
  the file is what jalon reads afterwards, never the issue again.
- 2026-08-06 altsoyuz: closing stays convention only. A pull request body with
  `closes #42` and `closes 260806` closes both sides by their own mechanism.
- 2026-08-06 altsoyuz: all gh usage confined to `github.go`, so the coupling to
  a forge is one deletable file.
- 2026-08-06 altsoyuz: `-no-prs` became `-offline`, one flag for one concept:
  this run touches no network.

## Log

- 2026-08-06 altsoyuz: bridge implemented, tested offline with a fake gh on
  PATH.
- 2026-08-06 altsoyuz: reopening condition named in docs/workflow.md, if status
  has to be retyped into GitHub week after week that is the measurement.
