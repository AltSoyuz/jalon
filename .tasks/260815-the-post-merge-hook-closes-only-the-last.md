---
status: done
created: 2026-08-15
links: []
---

# the post-merge hook closes only the last merge of a pull

## Context

## Decisions

- 2026-08-15 altsoyuz: the range is ORIG_HEAD..HEAD, which is what the merge moved from, with a -1 HEAD fallback when ORIG_HEAD does not exist: a bare git log HEAD would walk the whole history and try to close every id ever written in a message

## Log

- 2026-08-15 altsoyuz: found by using it: pull requests 19 and 20 landed in one git pull, and only the task named by the tip commit was closed; the other stayed todo while its implementation was already on main
- 2026-08-15 altsoyuz: fixed by hand rather than by jalon work: nothing covers this shell script, so make check would have proven nothing and the gate would have been theatre
- 2026-08-15 altsoyuz: closed.
