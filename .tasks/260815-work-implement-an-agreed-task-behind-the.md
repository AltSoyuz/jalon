---
status: done
created: 2026-08-15
links: []
---

# work: implement an agreed task behind the criterion

## Context

## Decisions

- 2026-08-15 altsoyuz: work takes a task id, never an issue number: the merged task is the agreement, and reading an issue directly would duplicate review and remove the human gate that makes the whole design worth anything
- 2026-08-15 altsoyuz: the criterion is the gate, and it is the whole gate: review needed Go code to check that the model had measured, because that cannot be verified mechanically; here sh -c criterion exits 0 or it does not, so no heuristic on model output is introduced
- 2026-08-15 altsoyuz: no retry loop in jalon: the criterion is handed to the model as an allowed command so it iterates inside its own invocation, and jalon verifies once at the end; a retry in jalon would be an unbounded cost with no bound on quality, and the plan already refused automatic retries
- 2026-08-15 altsoyuz: no -next and no timer for work: at one agreed task a week a queue has nothing to drain, and unsupervised implementation is a far larger blast radius than unsupervised measurement; the verb is run by hand until a measured rate justifies otherwise
- 2026-08-15 altsoyuz: a trivial issue does not get a shortcut through work: the fix for paying three model calls to delete one line is jalon new plus a sentence, which already exists, not a second entry path into work

## Log

- 2026-08-15 altsoyuz: publish is parameterised by branch, pathspec and message rather than duplicated: review commits .tasks only, work commits the checkout minus .jalon-work and .claude
- 2026-08-15 altsoyuz: the criterion is handed to the model through probes.allowed rather than as its own permission rule: a compound criterion like npm ci && npm run build cannot be expressed as a prefix rule, and both site configs already list its components
- 2026-08-15 altsoyuz: both gates proven non vacuous by neutering them: TestWorkStopsWhenTheCriterionFails and TestWorkStopsWhenNothingChanged fail without the checks they assert
- 2026-08-15 altsoyuz: shipped in pull request 16 and used twice since; the closes trailer was in its commit but the post-merge hook read only the tip of a pull that carried several merges, which is the bug fixed in 260815-the-post-merge-hook-closes-only-the-last
- 2026-08-15 altsoyuz: closed.
