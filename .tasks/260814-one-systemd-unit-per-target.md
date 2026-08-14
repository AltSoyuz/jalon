---
status: done
created: 2026-08-14
links: []
---

# one systemd unit per target

## Context

## Decisions

- 2026-08-14 altsoyuz: the emitted unit is named after the target directory rather than fixed at jalon-agent. A fixed name meant setting up a second repository silently replaced the first one's unit and timer: on a machine already running a job, the root block would have swapped the target out from under it without saying so.

## Log

- 2026-08-14 altsoyuz: found while adding a second target on a server already running a job, before the root block was executed. The directory name is sanitised, since a name systemd refuses to load would only be discovered at install time.
- 2026-08-14 altsoyuz: closed.
