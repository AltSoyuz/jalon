---
status: done
created: 2026-08-13
links: [agent/review.go, agent/doctor.go, Makefile]
---

# skip criterion in doctor when review runs it right after

## Context

`make check` was timed twice back to back with a warm Go build cache: 3.684s and 3.634s wall, split between `go test -race` on the root package (1.4s) and on `agent/` (2.9-3.0s), with `dogfood` against the repository's 10 task files adding a sub-second remainder (`Makefile:40`, `Makefile:34`, `Makefile:55-63`). Both `Doctor` and the skeptic's independent rerun reproduce the same ~3.6s. That is the per-tick cost, warm; a cold `GOCACHE` under the fresh service user that `agent-init`'s `ProtectSystem=strict` unit would run as was not measured, since clearing the shared build cache was out of scope for a probe.

The issue's framing as "doctor duplicates what CI already runs" does not hold structurally the way it reads: `criterionCheck` only runs inside `Doctor` (`agent/doctor.go:121-126, 230-244`), and nothing downstream in `Review` re-invokes it, so a single review job pays for it once, not twice. What does hold, and what the skeptic could not refute across three commits of history and two independent runs of `make check`, is narrower and cheaper to fix: `Doctor` runs unconditionally as the first line of `Review` (`agent/review.go:40`), before `opt.Next` is resolved (`agent/review.go:46-59`). An hourly tick that finds no issue labelled for review (confirmed live: `jalon list` currently shows no issue-backed task ready to pick up) still pays the full ~3.6s `make check` before `nextIssue` ever runs, for a result that then gets discarded at line 55-56.

At 24 ticks/day this is roughly a minute a day of wasted preflight in the worst case (every tick finds nothing), which is not the "CI duplication" the issue worried about but is real, unconditional, and free to stop paying for: `nextIssue` (`agent/review.go:47`) needs only `ctx` and `env.Root`, not `cfg`, so it does not depend on anything `Doctor` produces.

The fix is a reorder, not a new mechanism: when `opt.Next` is set, resolve `nextIssue` first and return the existing "nothing to review" no-op (`agent/review.go:51-57`) before calling `Doctor`, so an empty queue costs one `gh` query instead of a full `make check`. When an issue is found, or when `opt.Issue` is passed directly, `Doctor` still runs before any real work, unchanged. This keeps the criterion at full strength — the alternative of swapping it for something cheaper like `go build ./...` was rejected below, since it would weaken the one guarantee `doctor` exists to give for a check that is already under 4 seconds.

## Decisions

- 2026-08-13 jalon-agent: reject making the criterion itself cheaper (e.g. go build ./... instead of make check): it would weaken doctor's only guarantee, that the base is actually green, for a check that already costs under 4 seconds warm; accept reordering Review so nextIssue resolves before Doctor runs, so an empty queue costs one gh query instead of a full make check

## Log

- 2026-08-13 jalon-agent: measured make check twice locally (warm cache): ~3.6-3.7s wall, dominated by go test -race across the two packages; confirmed structurally that Doctor (and its unconditional criterionCheck) runs before ReviewOptions.Next is resolved, so an hourly tick with nothing labelled for review still pays the full cost; could not measure a cold-start tick on a live deployed timer, since agent-init only landed in this same branch
- 2026-08-13 altsoyuz: implemented as proposed: nextIssue resolves before Doctor on the -next path only. The regression this introduces is diagnostic, not functional, and is handled: a forge failure on that path now happens before the preflight could name it, so the error points at jalon doctor. Tests assert the criterion leaves no trace on an empty queue, rather than assuming it.
- 2026-08-13 altsoyuz: closed.
