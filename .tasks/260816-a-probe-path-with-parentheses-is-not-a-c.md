---
status: done
created: 2026-08-16
links: []
---

# a probe path with parentheses is not a composed command

## Context

## Decisions

- 2026-08-16 altsoyuz: probes run with exec and no shell, so parentheses and dollars are ordinary bytes and are no longer refused; pipes, ampersands, semicolons, redirections and backticks still are, because handed to a program as literal arguments they measure nothing and read as if they had. Measured on the first live review of a SvelteKit repository, where routes/(app)/start was the file that held the feature and its probe was refused, and the premise was then refuted on directory names

## Log

- 2026-08-16 altsoyuz: closed.
