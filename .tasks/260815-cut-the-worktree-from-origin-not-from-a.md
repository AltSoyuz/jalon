---
status: done
created: 2026-08-15
links: []
---

# cut the worktree from origin, not from a local branch nothing updates

## Context

## Decisions

- 2026-08-15 altsoyuz: the fetch goes in newWorktree rather than in each verb: review and work both call it, both need the same thing, and one place cannot drift from the other
- 2026-08-15 altsoyuz: the worktree is cut from FETCH_HEAD rather than from origin/<branch>: it is exactly what the fetch just retrieved, independent of how the clone refspec is configured, and it still never touches the local branch or the working tree
- 2026-08-15 altsoyuz: a failed fetch is fatal, unlike the units non fatal jalon pull: running on the code you already have is the defect being fixed here, so a tick that fails loudly beats a tick that quietly measures a stale tree

## Log

- 2026-08-15 altsoyuz: readTask still reads the local .tasks, so work requires the task to exist in your checkout; that failure is loud and its fix is git pull, unlike a stale base which is silent
- 2026-08-15 altsoyuz: closed.
