---
status: done
created: 2026-08-07
links: [docs/workflow.md]
---

# Team agreement through pull requests

## Context

Question raised from a review: several developers on one repository need to
settle what a task is before anyone writes code. The tempting answer was a
`publish` verb pushing the task into an issue to host the discussion.

It is the wrong answer for this case. People with write access to the
repository already have the file; what they lack is a discussion venue, and git
already has one. A pull request containing only the task file gives threads,
mentions, notifications, approvals and mobile access, all of which jalon
deliberately refuses to build.

So this was a documentation gap, not a missing feature. `status: proposed`
already works because any status is accepted.

## Decisions

- 2026-08-07 altsoyuz: no `publish` verb for this case. Its client is the requester without repository access, not a colleague who has cloned it. Adding it here would create a second place where the discussion can happen, which is the problem being avoided.
- 2026-08-07 altsoyuz: the proposal cycle is `status: proposed`, a pull request holding only the task file, merge as the agreement, then `status: doing`. Zero code: free form statuses and the forge already cover it.
- 2026-08-07 altsoyuz: what lands in the file is the Decisions lines, distilled by hand. The thread is disposable, the arbitration is permanent, and no tool can do that distillation.

## Log

- 2026-08-07 altsoyuz: verified that a free form status works end to end, list filters on it and the rendered index gives it its own section.
- 2026-08-07 altsoyuz: documented in docs/workflow.md, including the conflict trap when a proposal branch and main both touch the same task file.
- 2026-08-07 altsoyuz: `publish` stays a named ceiling at about twenty five lines, whose trigger is copying a task's state into an issue by hand for the third time.
