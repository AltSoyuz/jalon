# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Optional agent layer, in its own `agent/` package: `jalon doctor` checks that
  a machine can run an agent job, and `jalon review <issue>` turns a GitHub
  issue into a measured task proposal in a pull request. It reads
  `.jalon/agent.toml`, and it is the only part of jalon that calls a model. The
  core still holds none: `TestCoreDoesNotImportAgent` fails if a core file ever
  imports the layer, so `rm -rf agent agent_cmd.go` leaves a working task
  manager. Still no dependency; the configuration is read by a small TOML subset
  parser that refuses everything outside its grammar with the line and the fix.
  See `docs/agent.md`.
- `jalon agent-init` prints the configuration, a systemd unit and a timer for
  one machine, and installs nothing: the privileged half sits behind an explicit
  `--root` flag, so the output is read before it runs and diffs against what is
  deployed. The units are embedded in the binary because the thing you copy to a
  server is the binary, not the repository. `jalon review -next` takes the oldest
  open issue labelled `measure`, and an empty queue succeeds quietly so an idle
  timer never turns red.
- `jalon review -next` resolves the queue before running the preflight, so an
  hourly tick that finds nothing labelled no longer runs the repository's whole
  criterion and throws the result away. Measured at ~3.6s per tick on a real
  server. `jalon doctor` also checks that git has an identity: without one a
  review writes the task and dies at the commit, after three model calls have
  been paid for.

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
- CI: the gitleaks step runs the released binary pinned by version instead of
  the action, which exited 1 on a scan reporting no leaks. Every action bumped
  to its current major, and `cache: false` on setup-go since a module with no
  dependency has no `go.sum` to cache.
- `jalon close -from-merge` no longer stops at the first unusable reference. An
  ambiguous id in a merge message is reported on stderr and skipped, and the
  other ids in the same message are still closed. It used to abort the loop and
  silently discard valid references after the bad one, after the merge had
  already happened.
- A commit tag now matches a task when it is a **prefix** of its id, replacing
  the two special cases of "the full id" and "the six digit date". `[260806]`,
  `[260806-list]` and the full id all reach the same task, so subjects stay
  inside the fifty characters git recommends. Every commit written under the
  previous rule keeps matching.
- `jalon list` prints one line per task, with `-status` to filter. It is the
  cheap half of orientation and the zero argument command a harness session
  start hook needs: stdout stays one line per task so no shell glue is needed
  around it.
- Opt in self observability: with `JALON_METRICS` set to a path, every
  invocation appends one JSON line with the verb, task, size, duration,
  degraded state and error. Unset, nothing is written. No aggregation verb
  ships with it; `docs/measuring.md` reads the file with `jq`.

### Notes

- Building from source requires Go 1.26.5, declared in `go.mod` and read from
  there by every workflow. The released binaries need nothing: a single static
  file per platform.
