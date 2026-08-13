---
status: done
created: 2026-08-13
links: []
---

# the deployment updates itself

## Context

## Decisions

- 2026-08-13 altsoyuz: refuse a jalon update verb: it would duplicate git and gh, already on the machine, in around 150 lines of Go with a network client, checksum verification and atomic self replacement, for one usage. The privileged write objection dissolves once the binary lives in the agent's home, but the duplication does not.
- 2026-08-13 altsoyuz: refuse releases as the update channel while the target is jalon itself: the criterion is make check, so Go is required on the server anyway and a prebuilt binary saves nothing, while tagging costs a human step per deploy. Revisit the day a target repository has no Go, and the answer will be gh release download in an ExecStartPre, still not a verb.
- 2026-08-13 altsoyuz: the unit pulls and rebuilds before each tick and runs the binary from the agent's own checkout. Accepted cost: the owner chooses what ships by merging, not when it ships. Mitigated by the criterion, which still gates the job, and by JALON_METRICS, which records the stamped version that produced each task.

## Log

- 2026-08-13 altsoyuz: the closes trailer of the first commit named a task id that does not exist, because jalon new prints the id and it was discarded. Corrected in a follow up commit rather than a rewrite, since the branch is already pushed and under review.
- 2026-08-13 altsoyuz: closed.
