---
status: todo
created: 2026-08-15
links: [agent/review.go]
---

# write the skeptic output before its error is checked

## Context

`agent/review.go` runs three phases and saves what each printed into
`.jalon-review/`. Two of them write the file **before** checking the phase's
error, so a phase that failed still leaves its output on disk for whoever has to
diagnose it. The skeptic phase does the opposite: it returns on the error first
and writes `skeptic.md` after, so a failing skeptic leaves nothing to read.

This is the same defect that cost a real job earlier: the writing phase
discarded its output, failed, and the cause could only be found by replaying the
invocation by hand. The fix there was to write before checking. The skeptic was
left alone at the time because it was a separate change, and this is that
change.

The deliverable is the reordering in the skeptic block, and a test that fails
without it: a skeptic phase that errors must still leave `skeptic.md` behind.
Nothing else in `review.go` moves.

## Decisions

- 2026-08-15 altsoyuz: the reordering is one line and matches what the facts and task phases already do: write the file, then check the error, so a failed phase always leaves its output behind

## Log
