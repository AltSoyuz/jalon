---
status: done
created: 2026-08-13
links: []
---

# review must leave the queue when it is done

## Context

## Decisions

- 2026-08-13 altsoyuz: the measure label is the queue and a finished review removes it, rather than tracking reviewed issues in task front matter. Front matter would need a second writer of a documented compatibility surface inside agent/, which cannot import the core parser; the label is one gh call, already a primitive, and leaves the state visible in the forge where a human can undo it.
- 2026-08-13 altsoyuz: a failed review keeps its label and its worktree on purpose: the stale worktree makes doctor refuse the next job, so a repeatedly failing issue stops the timer instead of retrying hourly.

## Log

- 2026-08-13 altsoyuz: found live before the timer was armed: issue #5 stayed labelled after being measured, merged and implemented, and a second run produced a near duplicate task. -next was written assuming triage would remove the label, and triage does not exist.
- 2026-08-13 altsoyuz: closed.
