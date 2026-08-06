---
status: done
created: 2026-08-07
links: [digest.go, render.go, docs/format.md]
---

# Shorter commit tags by prefix matching

## Context

Measured on the first four real commits: subjects of 60, 61, 64 and 69
characters, of which 33 to 42 are the tag alone. Git recommends 50.

The cause is that the convention had two matching rules, the full id or the six
digit date, and nothing in between. With five tasks created the same day the
date form is ambiguous for all of them, so the full form is forced every time.
The short branch of the convention was dead from the start: it stops working as
soon as two tasks exist in one day, which is the normal case.

The fix removes a special case rather than adding one. A commit token matches a
task when it is a **prefix** of that task's id. The six digit date becomes one
prefix among others instead of a hard coded rule, and `[260806-list]` works.

Backward compatible without effort: a full id is a prefix of itself, so every
commit written under the old rule keeps matching.

## Decisions

- 2026-08-07 altsoyuz: generalize to prefix matching instead of adding a third form. One rule replaces two special cases, so the code shrinks and the convention gets simpler to state.
- 2026-08-07 altsoyuz: the named cost is retroactive ambiguity. Creating `260806-list-something-else` later would make an existing `[260806-list]` ambiguous. The existing warning covers it, and it is the same trade as the date form, less coarse.
- 2026-08-07 altsoyuz: digest keeps one git call. The date part bounds the grep, the prefix test happens in Go on the few matching lines.
- 2026-08-07 altsoyuz: rewrite the four existing commits to the short form. No tag, no published version, no clone: the module checksum problem that forbids this on lib does not exist here.

## Log

- 2026-08-07 altsoyuz: task written before the code, per the AGENTS.md rules.
- 2026-08-07 altsoyuz: landed; render lost a special case and digest keeps its single git call, bounded by the date part.
- 2026-08-07 altsoyuz: verifying the rewrite caught a regression I had just introduced, tags in a commit body stopped matching. digest and render now both search the whole message and display the subject, which also makes them agree for the first time.
