---
status: proposed
created: 2026-08-13
links: []
---

# jalon list: avoid full-file reads of every task on each call

## Context

`jalon list` in this repo returns 10 lines from 10 files under `.tasks/`, too few to make repeated parsing observable at a shell prompt. Reading `task.go` directly confirms the mechanism the issue describes: `cmdList` (main.go:478-518) calls `LoadTasks(dir)` unconditionally, which globs `*.md` and calls `ParseTask` on every match; `ParseTask` does `os.ReadFile` of the *entire file* (front matter and full body, including commits/decisions/log entries — one task's own digest runs to 18076 bytes) and then runs `parseFrontMatter` on just the front-matter slice. There is no partial read, no cache, and no index anywhere in `task.go`, and this has been true since the tool's first working version (`358627a`). `list_test.go` has no benchmark and no fixture large enough to exercise the concern. The skeptic tried to refute the premise from the same source and could not: every command available corroborated it instead.

What neither of us could produce is a number. This repo has no few-hundred-task fixture, `go test -bench` has nothing for `LoadTasks` (the only benchmark in the suite is `BenchmarkRenderSite`), and the sandbox's command allowlist had no way to fabricate one or to set `JALON_METRICS` inline on a real `list` call to read the `ms`/`tasks` fields the binary already emits. So the claim in the issue — full parse on every shell prompt, at "a few hundred tasks" — is confirmed as a mechanism, not as a measured cost. It is very plausibly real (a few hundred `ReadFile` syscalls plus a few hundred small string scans, once per prompt), but "very plausibly" is not the same as measured, and the honest thing is to say so before proposing work.

Cost of the fix the issue asks for (a small index file next to `.tasks/`) is not free: it is a second source of truth that has to be kept in sync with every `jalon new`/`append`/status change, needs a staleness or corruption fallback, and adds a new failure mode (stale index says "doing", file says "done") to a tool whose whole design intent, per `260806-list-verb-for-harness-orientation`'s digest, was "one line per task on stdout, nothing else." That original task's decisions never mention caching or an index — this is new scope on top of it.

Given that `ParseTask` already reads the whole file just to throw away the body, the cheaper and lower-risk fix is likely a partial read in `ParseTask` (or a `ParseTaskMeta` used only by `list`) that stops at the closing `\n---\n` delimiter instead of an index file: no second source of truth, no invalidation logic, and it removes the actual measured waste (bytes read per file scales with total body size, not just front-matter size) rather than adding a cache on top of it. Before landing anything, this should be scoped only if a fixture at real scale (a few hundred tasks) shows `jalon list` is actually slow enough to matter for a session-start hook — the issue's premise is unverified at magnitude, even though the mechanism is real.

## Decisions

- 2026-08-13 leybardie: Prefer a partial-read fix in ParseTask (stop at the closing front-matter delimiter) over an index file, because an index is a second source of truth that can drift from .tasks/ and needs its own invalidation, while the measured waste is bytes read per file (whole body, including commits/decisions/log) not parse-time CPU.

## Log

- 2026-08-13 leybardie: Traced jalon list to cmdList -> LoadTasks -> ParseTask: os.ReadFile of the whole file, then parseFrontMatter on the front-matter block only; no partial read, no cache, no index, confirmed unchanged since the tool's first version (358627a).
- 2026-08-13 leybardie: Could not measure actual cost at scale: this repo's own .tasks/ has 10 files, no Benchmark* exists for LoadTasks, and the sandboxed command allowlist had no way to generate a few-hundred-task fixture or set JALON_METRICS inline to capture ms/tasks for a real list call.
- 2026-08-13 leybardie: Skeptic could not refute the premise: source inspection confirms the mechanism matches the issue's claim exactly, full read plus parse of every .md file on every list call, with no cheaper path anywhere in task.go.
