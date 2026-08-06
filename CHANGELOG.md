# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- First working version: the `new`, `append`, `digest`, `compact`, `render` and
  `close` verbs over `.tasks/*.md`, with no external dependency.
- File format: `YYMMDD-slug` ids, a `key: value` front matter that stays valid
  YAML and preserves unknown keys, and the `Context`, `Decisions`, `Log`
  sections in agent reading order.
- `digest` reports its own size and duration on stderr, which is the
  measurement used to compare it against a forge.
- `render` writes a static site with hand written CSS and no JavaScript, from a
  documented markdown subset covered by a golden test.
- `hooks/post-merge` sample hook, closing tasks named by `closes <id>` in a
  merge message.
- One way GitHub bridge, stateless and on demand: `jalon new -issue N` seeds a
  task from an issue, `digest` shows the thread when `issue:` is set, and
  `-offline` skips every `gh` call. There is no synchronization, by design.
- `make release` builds every published platform into `dist/` with one archive
  and one sha256 file each: linux amd64/arm64/arm/386, darwin amd64/arm64,
  windows amd64/arm64, freebsd amd64. `make brew-formula` writes the Homebrew
  formula from those checksums.
- `make dogfood` runs the freshly built binary against this repository's own
  `.tasks/`, and CI runs it on every pull request.

### Notes

- Building from source requires Go 1.26.5, declared in `go.mod` and read from
  there by every workflow. The released binaries need nothing: a single static
  file per platform.
