---
status: done
created: 2026-08-15
links: []
---

# the queue is the status field, read from origin with git alone

## Context

## Decisions

- 2026-08-15 altsoyuz: work -next resolves its queue with git alone: fetch origin <default_branch>, git grep -l '^status: implement$' FETCH_HEAD -- .tasks, oldest id first, skipping any id whose work/<id> branch already exists on origin (git ls-remote); no label, no gh call, no state outside git
- 2026-08-15 altsoyuz: review -next does the same with status: measure and rewrites that task in place (Context becomes measured, contradicted, cost, plan) rather than creating a second file, so one id runs from capture to done; review <issue> stays as the shell entry and writes the stub through jalon new -issue -status measure
- 2026-08-15 altsoyuz: the phone button is editing one front matter line in the forge's app and committing to main, the same gesture that already merges a task PR; if a week of use shows the button pressed less often than the label was, the label comes back as a shortcut in front of the file, and the file stays the queue
- 2026-08-15 altsoyuz: issue: stays optional and only feeds Closes #N in the PR body; nextIssue, unlabel, taskForIssue and LabelMeasure/LabelImplement are removed, and agent/gh.go keeps fetchIssue and createPR
- 2026-08-15 altsoyuz: review takes a task id, not an issue: capture is jalon new -issue N -status measure, which the core already owns, so the agent layer holds no issue reading at all and no second copy of the format; a wreck parked in .jalon/failed keeps its task out of the queue until removed, so a failing task is retried by a person and not by the clock

## Log

- 2026-08-15 altsoyuz: measured problem: 260815-carry-the-issue-number-from-review-to-work spent a day reconciling two identifiers across the forge seam; this removes the second identifier. Third in the order because task 1's 'leave the queue' step changes shape once the queue is the file.
- 2026-08-15 altsoyuz: implemented: agent/queue.go holds nextTask (git grep on FETCH_HEAD, git ls-remote for published branches, .jalon/failed for wrecks), fetchDefault and readTask from FETCH_HEAD; review takes a task id, feeds the task file to the phases, checks with git that exactly that file changed, sets status: proposed itself and pushes task/<id>; work -next reads status: implement; gh.go keeps createPR only; skills, unit template, setup script and docs updated. Tests rewritten around a queued task fixture.
- 2026-08-15 altsoyuz: closed.
