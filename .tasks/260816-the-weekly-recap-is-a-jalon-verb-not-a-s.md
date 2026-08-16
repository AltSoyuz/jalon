---
status: done
created: 2026-08-16
links: []
---

# the weekly recap is a jalon verb, not a shell script

## Context

## Decisions

- 2026-08-16 altsoyuz: the recap moves from scripts/weekly-recap.sh to jalon recap in the agent package: same sections, same one-command delivery, tested by TestRecap against a fixture; the owner wants Go where Go can be, and the only shell that remains is what must be shell, the git hook and the setup script agent-init emits because the binary writes nothing under /etc
- 2026-08-16 altsoyuz: jalon list is run as a process rather than parsed again: package agent does not import package main, and a second parser of the task list would be a second thing to keep true; jalon is on PATH wherever a job runs

## Log

- 2026-08-16 altsoyuz: closed.
