---
status: done
created: 2026-08-16
links: []
---

# capture: one ntfy topic is the phone's way in

## Context

## Decisions

- 2026-08-16 altsoyuz: the inbox is an ntfy topic, not a note or a repository: the phone already has the app, a shortcut posts one line in one gesture, and the machine reads it with one HTTP poll; the first word names the repository (altsoyuz for altsoyuz.com), a bang skips the review, and a line without a known prefix comes back as a notification instead of being guessed into a task
- 2026-08-16 altsoyuz: the stub is pushed to the default branch by jalon on unprotected repositories and becomes a task/<id> branch with a pull request where main is protected; the cursor is one line holding the id of the last handled message, so a failed push retries the same line next tick and a burst is bounded to twenty per tick

## Log

- 2026-08-16 altsoyuz: closed.
- 2026-08-16 altsoyuz: one topic: capture reads the same topic jalon writes to, skips titled messages (jalon's voice), and acknowledges every line in the thread; the phone reads one conversation and the inbox topic is retired
