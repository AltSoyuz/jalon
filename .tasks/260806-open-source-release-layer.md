---
status: done
created: 2026-08-06
links: [Makefile, docs/release.md]
---

# Open source release layer

## Context

The repository needed everything an open source project is judged on before
anyone can use it: distribution on every platform, issue and pull request
templates, a security policy, a code of conduct, dependabot, CodeQL, and a
release procedure that a stranger can audit.

The model is VictoriaMetrics: the build lives in the Makefile and CI only calls
it, so a release is reproducible on a laptop. Archives are one tarball plus one
sha256 file per platform, and the release notes come from the changelog section
for the tag, which fails loudly when that section is missing.

## Decisions

- 2026-08-06 altsoyuz: no goreleaser. The Makefile matrix is twenty lines we own
  and can run locally; a release tool would be a build dependency whose
  behaviour we do not control, for artifacts we already produce.
- 2026-08-06 altsoyuz: no Homebrew tap yet. A tap is a second repository to
  operate and a formula to resynchronize at every release, for nobody.
  `make brew-formula` generates it, so the day it is worth doing costs five
  minutes, documented in docs/release.md.
- 2026-08-06 altsoyuz: the whole release matrix is built on every pull request,
  so a platform specific break is caught before the tag.
- 2026-08-06 altsoyuz: Pages deploys on release and manual dispatch only, never
  on push, so a repository without Pages enabled does not fail on every commit.
- 2026-08-06 altsoyuz: `make dogfood` runs the built binary against this repo's
  own tasks, which makes them a regression corpus rather than decoration.
- 2026-08-06 altsoyuz: the Go version lives in go.mod alone; every workflow
  reads it with go-version-file, so a toolchain bump is one line in one file

## Log

- 2026-08-06 altsoyuz: release matrix produces 9 platforms, verified locally at
  v0.1.0 with checksums and a generated Homebrew formula.
- 2026-08-06 altsoyuz: CI has test, dogfood, crosscompile, staticcheck, gitleaks
  and CodeQL. Release notes are taken from the changelog and fail without a
  section.
- 2026-08-06 altsoyuz: toolchain moved to go 1.26.5, the latest stable;
  staticcheck rebuilt against it and now clean
- 2026-08-06 altsoyuz: signatures normalized to the account identity
- 2026-08-06 altsoyuz: repo published at github.com/AltSoyuz/jalon; the initial push predated workflow registration, so CI got a manual trigger
