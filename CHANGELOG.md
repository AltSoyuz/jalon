# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- A failed `review` or `work` parks its worktree under `.jalon/failed/` and
  takes the issue out of the queue, so the next tick runs the next item instead
  of the machine waiting for a person; `doctor` warns while wrecks are kept and
  the eleventh job refuses.

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
- `jalon doctor -live` spends one real, tool-less model call to prove the model
  answers. Off by default: a single invocation costs more than a few cents, so
  a check on every tick would be a standing bill. Everything else doctor checks
  establishes that a binary exists and takes the right flags, which does not
  catch an expired token.
- `jalon work -next` takes the oldest open issue labelled `implement` and builds
  the task that names it, through the `issue:` key the review wrote. The two
  labels are now the whole remote control: with the forge's phone app you can
  capture an idea, agree to a task and order it built, with no shell, no SSH key
  and no route to the server. It is not autonomy, and jalon still chooses
  nothing: the label is a button a person pressed and the timer only delivers
  it, an issue no task names is a refusal rather than a guess, and the label is
  removed on publish so a tick never builds the same thing twice. The emitted
  unit runs both stages in one tick, since a `oneshot` service takes several
  `ExecStart` lines.
- An issue whose work ships now closes itself. `jalon review` has its writing
  phase record the issue number on the task (`jalon new -issue N`, the core verb
  that already owns that key), and `jalon work` puts `Closes #N` in its pull
  request body, so the merge that lands the implementation closes the issue.
  That is the forge's own mechanism and jalon's own hook doing the work: jalon
  carries two identifiers and keeps no state about either, so there is still
  nothing to synchronize. A task written by hand names no issue and closes
  none, and a review whose premise was refuted is still closed by a person,
  because telling "do this" from "do nothing" would mean judging the model's
  prose in Go.
- `hooks/post-merge` reads every message a pull brought in (`ORIG_HEAD..HEAD`)
  instead of only the tip commit. One `git pull` routinely lands several merges,
  and the old form closed the last one's task while the others stayed open with
  their work already on the branch. Found that way: two pull requests landed
  together and one task was left behind.
- A job's worktree is cut from `origin/<default_branch>` after an explicit
  fetch, instead of from whatever the local branch holds. Nothing updates a
  target repository's checkout: the systemd unit pulls the jalon checkout and
  only that, so a target drifts from the day it is cloned. A review then
  measured stale code, and `work` would commit on top of an ancestor, which is
  how an implementation reverts something already merged. The fetch is fatal on
  failure, unlike the unit's pull, and it never touches your branch or your
  working tree.
- `jalon work <task-id>` implements one already agreed task and opens a pull
  request only if the repository's own `[criterion].command` passes. It takes a
  task id and never an issue number, because the merged task is the agreement;
  there is no `-next` and no timer, because unsupervised implementation has a
  blast radius unsupervised measurement does not. Where `review` needed Go code
  to check that the model had measured, this gate either exits 0 or it does not,
  so no judgement on model output is introduced anywhere. The commit carries
  `closes <id>`, so merging closes the task through the hook that already
  existed. See `docs/agent.md`.
- `jalon review` keeps what its writing phase printed, in `.jalon-review/task.md`
  beside the facts and the skeptic, written before the phase's error is checked.
  That phase used to be handed a `Write(.tasks/**)` permission, which Claude Code
  never matches against file permission checks, so the model would try it, be
  denied and sometimes give up; the run then failed having discarded the only
  evidence of why. The rule is gone, `Edit(.tasks/**)` already covered it, and a
  failure now names the file to read.

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
