---
status: doing
created: 2026-08-07
links: [.github/workflows/ci.yml]
---

# CI green on a clean runner

## Context

Runners came back after the GitHub outage and CI ran for the first time. Five
jobs passed, including dogfood, which was the one most likely to expose an
assumption true only on the author's machine. One job failed and two warnings
appeared.

gitleaks logged `no leaks found in partial scan` and then exited 1. The commit
it tried to diff against, 0266763, no longer exists: it belongs to the history
rewritten earlier. The trigger was self inflicted, but exiting 1 on a scan that
found nothing is indefensible whatever the cause, and that job is the one whose
entire purpose is trust.

Separately, pushes stopped creating runs again: 520d383 and 44e58ea produced
none at all, while 9e466cb did. That part is not in this repository's control.

## Decisions

- 2026-08-07 altsoyuz: replace the gitleaks action with the released binary, pinned by version. An opaque exit code is a poor fit for the security job. Verified locally first: `gitleaks git .` scans the whole history and exits 0 on this repository.
- 2026-08-07 altsoyuz: the price is named, dependabot no longer bumps gitleaks. GITLEAKS_VERSION is now bumped by hand, which is the cost of owning the step instead of renting it.
- 2026-08-07 altsoyuz: `cache: false` on every setup-go. No dependency means no go.sum, so the cache warned about a missing dependencies file in every job.
- 2026-08-07 altsoyuz: bump every action to its current major, which removes the Node 20 deprecation warnings rather than waiting for them to become errors.

## Log

- 2026-08-07 altsoyuz: first real CI run: dogfood, staticcheck, crosscompile and test on ubuntu and macos all green. Only gitleaks failed.
