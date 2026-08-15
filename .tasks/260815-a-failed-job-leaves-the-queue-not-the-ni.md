---
status: todo
created: 2026-08-15
links: []
---

# a failed job leaves the queue, not the night

## Context

## Decisions

- 2026-08-15 altsoyuz: a failed review or work moves its worktree with git worktree move to .jalon/failed/<name>, prints the path and the last lines of the error, notifies, and exits non-zero; doctor keeps refusing on .jalon/worktrees only, so the next tick takes the next item instead of the machine waiting for a person
- 2026-08-15 altsoyuz: .jalon/failed holds at most ten entries and the eleventh job refuses naming the directory: a machine failing in a loop stops within one night rather than filling the disk, and ten wrecks are more than anyone will read
- 2026-08-15 altsoyuz: a failed job leaves the queue like a published one (label removed today, status untouched once the queue is the file): a job that stays queued after failing is retried every tick and spends the daily cap on one item, which is the same defect the unlabel step already fixes for success
- 2026-08-15 altsoyuz: the owner's working tree and task files stay untouched on failure, as today; the evidence is the kept directory and the message, never a write into the real checkout

## Log

- 2026-08-15 altsoyuz: measured problem: docs/agent.md states that a failed review keeps its worktree and doctor refuses the next job; on a timer this turns one failure into a frozen night. Order of the plan: this first, it is what returns nights.
- 2026-08-15 altsoyuz: implemented: worktree.park moves a wreck to .jalon/failed with git worktree move, failJob notifies and reports the path, both verbs unlabel on failure and say how to re-queue, doctor warns on kept wrecks, newWorktree refuses at ten. Tests: TestAFailedReviewIsParkedAndTheNextJobRuns, TestTheFailedDirectoryHasACap, TestAFailedWorkLeavesTheQueueAndIsParked.
