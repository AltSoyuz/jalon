---
status: done
created: 2026-08-15
links: []
---

# carry the issue number from review to work so the forge closes the issue

## Context

## Decisions

- 2026-08-15 altsoyuz: the closing mechanism is the forge's own: a merged pull request saying Closes #N closes the issue. jalon carries the number and nothing else, so there is no polling, no state and no synchronisation to reconcile
- 2026-08-15 altsoyuz: review records the number by having the writing phase run jalon new -issue N, the core verb that already owns that front matter key, rather than agent writing front matter itself: one implementation of the format, not two
- 2026-08-15 altsoyuz: a missing issue key warns and never fails: the key is a convenience for a later work run, and refusing a whole measured review over it would be disproportionate
- 2026-08-15 altsoyuz: a refuted premise still closes by hand: telling do this from do nothing would mean judging the model's prose in Go, which this design refuses everywhere else, and the label is already removed so the issue is out of the queue

## Log

- 2026-08-15 altsoyuz: closed.
