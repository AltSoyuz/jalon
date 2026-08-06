---
status: done
created: 2026-08-07
links: [main.go, docs/format.md]
---

# Ambiguous task references

## Context

A reader got confused by the two prefix mechanisms, which behave differently on
purpose: a command argument must designate exactly one task and an ambiguous
one is a blocking error, while a commit tag is read only and generous, matching
every task it is a prefix of.

Reproducing that confusion surfaced a real defect rather than a trap to
document. In `close -from-merge`, one unusable reference aborted the loop, so
valid references after it were silently discarded. Verified: a merge message
holding `closes 260807` (ambiguous) followed by `closes 260807-billing` closed
nothing at all, after the merge had already happened.

The documentation was also lying by omission: every example wrote a bare
`jalon close 260806`, which only works in a repository holding one task that
day. That is the same defect fixed on the commit tag side, on the argument side.

## Decisions

- 2026-08-07 altsoyuz: the asymmetry stays and gets written down instead of being smoothed over. Guessing wrong costs nothing when reading and corrupts a file when writing, so tags are generous and arguments are strict.
- 2026-08-07 altsoyuz: `-from-merge` reports a bad reference and keeps going, because the merge already happened and nothing can be aborted; typing an id by hand still fails hard, since retyping is free.
- 2026-08-07 altsoyuz: examples use `260806-migration` rather than a bare date, so they stay correct in the normal case of several tasks per day.

## Log

- 2026-08-07 altsoyuz: reproduced on a scratch repository before touching anything; the valid reference was indeed being dropped.
- 2026-08-07 altsoyuz: fixed with a regression test asserting both that the valid id closes and that the ambiguous one is left untouched.
