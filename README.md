# jalon

A task manager for humans and agents where the files are the truth, git is the
sync and the history, one binary is the tool, and static HTML is the view.

No server, no daemon, no vendor, and no dependency: `go.mod` has no `require`
block, the standard library does all of it.

A *jalon* is the stake a surveyor plants in the ground to mark out a line. The
id carried in a commit message is exactly that: a marker planted in the commit
graph, which is why the link between a task and its code outlives any tool.

## Install

```sh
go install github.com/AltSoyuz/jalon@latest
```

Or take a single static binary from the
[releases](https://github.com/AltSoyuz/jalon/releases): linux, macOS, windows
and freebsd, amd64 and arm64 (plus arm and 386 on linux), each with its sha256.
Uninstalling is `rm`.

```sh
tar -xzf jalon-linux-amd64-v0.1.0.tar.gz && ./jalon version
```

Homebrew has no tap yet, and [that is deliberate](docs/release.md#homebrew).

## Why

Tracking a task in a forge costs an agent several network round trips and a few
thousand tokens before it is oriented. A task here is one markdown file, read
sequentially, most useful part first. `jalon digest` composes that file with the
files it points at, the commits that carry its id, and the open pull requests,
as a single block on stdout.

The links between a task and the code are commits, so they survive any change
of platform. They are not metadata in someone else's database.

## The file

`.tasks/260806-migration-auth.md`

```markdown
---
status: todo
created: 2026-08-06
links: [internal/auth/session.go, docs/adr-3.md]
---

# Migration auth

## Context

Bounded, rewritten at compaction, always current.

## Decisions

- 2026-08-06 gt: cookie sessions over JWT, revocation is required.

## Log

- 2026-08-06 gt: started, blocked on the refresh path.
```

- The id is `YYMMDD-slug`. No counter, no coordination, and it stays unique
  across years so that `git log --grep '[260806]'` keeps meaning one thing.
- The front matter is a small `key: value` block. It is valid YAML, so a forge
  renders it as a table. Keys `jalon` does not know are preserved as written.
- The sections are in the order an agent reads them: `Context` is bounded and
  rewritten, `Decisions` and `Log` are append only, one line per entry.
- `status` is conventionally `todo`, `doing` or `done`. Any other value is
  accepted and gets its own group in the view.

The full history is in git, for free. That is what makes truncation safe.

## The seven verbs

```sh
jalon new "migration auth"                       # creates the file, prints its path
jalon new -issue 42                              # or seeds it from a GitHub issue
jalon list -status doing                         # the cheap half of orientation
jalon append 260806 "blocked on the refresh path"
jalon append -decision 260806 "cookie sessions over JWT"
jalon digest 260806                              # the whole context, one block
jalon compact 260806                             # truncate the Log, report the budget
jalon render                                     # regenerate .tasks/site/
jalon close 260806
```

Every command takes an id or its unique prefix, and refuses to guess when a
prefix matches several tasks. Flags come before the arguments; a flag placed
after them is refused rather than silently read as text.

### list and digest

Orientation is two steps, and the sizes are why. `list` costs about fifteen
tokens per task and says what exists; its stdout is one line per task and
nothing else, so a harness hook is `jalon list -status doing` with no glue
around it. `digest` costs a couple of thousand tokens and says everything about
one task.

`digest` is the verb for the agent. It writes, in this order: the front matter, `Context`,
`Decisions`, the last N `Log` entries, the content of each linked file, the
commits carrying the id, the open pull requests, and the issue thread when
`issue:` is set. Every cap it applies is stated in the output, never silent.
`-offline` skips every `gh` call.

It reports its own size on stderr:

```
# digest 260806-migration-auth: 4312 bytes, ~1078 tokens (bytes/4), 2 linked files, 6 commits, 11ms
```

That line is the point: comparing this against a forge costs nothing, it falls
out of normal usage. `~tokens` is `bytes/4`, an estimate, not a tokenizer.

Set `JALON_METRICS=~/.jalon-metrics.jsonl` and every invocation appends one JSON
line there: verb, task, bytes, tokens, duration, degraded state, error. Unset,
nothing is written. See [docs/measuring.md](docs/measuring.md).

### compact

Truncates the `Log` to its last entries and replaces the older ones with one
line pointing at `git log`. It does not rewrite `Context`: this tool holds no
model and will never call one. It reports the token budget and tells you when
`Context` is what needs your attention.

`jalon compact -check <id>` changes nothing and exits 1 over budget, which is
what you want in a pre-commit hook.

### render

Regenerates everything, every time: an index grouped by status and one page per
task, with the commits that carry each id. Measured on an Apple M3: **500
tasks of about 5 KB render in about 120 ms** (`make bench`). An incremental
build would be a cache to invalidate for no gain anyone can measure.

Hand written CSS, no JavaScript. Read it over `file://`, serve it with
`python3 -m http.server`, or `rsync` it anywhere. The exit cost is one `rsync`.

## The conventions (they are half the product)

Carry the id in every commit message that touches the task:

```sh
git commit -m "[260806] fix the refresh token path"
```

This is what makes the task to code link atomic, inside the commit graph, and
portable. A tag matches when it is a **prefix** of the id, so `[260806]`,
`[260806-migration]` and the full id all reach the same task: use the shortest
unambiguous form and the subject stays inside git's fifty characters.

Commit task updates together with the code they describe. No long lived branch
on `.tasks/`.

`hooks/post-merge` is a sample hook that reads `closes 260806` in a merge
message and flips the status. Install it by hand or do not; nothing depends
on it.

## GitHub, without a synchronization

There is none, on purpose. Two writable stores would need a token, a mapping
table, a conflict rule and something scheduled to run, which would break the
three properties this tool exists for. The bridge is one way, on demand and
stateless: `jalon new -issue 42` seeds a task from an issue in one `gh` call,
and `digest` shows the thread afterwards.

Closing needs no code at all. A pull request body holding both lines:

```
closes #42
closes 260806
```

closes the issue through GitHub's own mechanism and the task through the
post-merge hook. Two systems, two mechanisms, no glue. The reasoning and the
condition to revisit it are in [docs/workflow.md](docs/workflow.md).

## The markdown subset (the one debt)

`render` implements a subset on purpose, rather than depending on a parser:
ATX headings 1 to 3, fenced code blocks, single level bullet lists, paragraphs,
inline code, and links.

**Anything outside the subset is emitted as escaped literal text, never
guessed.** Tables and nested lists show up as raw markdown in the local view.
The failure mode is "it looks like the source file", never broken HTML and
never an injection. `testdata/subset.md` is the contract, and its golden output
is checked in CI.

The forge renders full markdown anyway. The view is a comfort, never a
dependency.

## The bill, plainly

What you pay: about 1250 lines of Go to own (CSS and HTML templates included),
`gh` as an optional extra, and conventions a team has to keep.

What you give up: notifications, mentions, contributions from people without
repository access, the mobile comfort of a forge, and review threads, which
stay on the forge if you keep one.

What you get: no data outside the repository, no network on the hot path, a
format any human can read and any agent can digest in one pass, and task to
code links stronger than a forge's, because they are commits.

## Operations

Diagnosing this system is `cat` and `git log`. If those two stop being enough,
it has grown too big and the fix is to shrink it.

Degraded modes, all of them explicit on stderr and none of them fatal: outside
a git repository there are no commits and no pull requests; without `gh` there
are no pull requests; without the binary the files are still readable and
editable by hand; without the view the forge still renders the markdown.

Signature on entries, resolved in this order: `-sig`, then `$JALON_SIG`, then the
first word of `git config user.name` lowercased, then `$USER`, then `unknown`.

## The measurement that validates or kills this

Tokens and tool calls until an agent is oriented, `jalon digest` against
`gh issue view` against free exploration, on real tasks, over two weeks. If
`digest` does not clearly win, the product reduces to its conventions, which
cost nothing and stay.

## Ceilings, named and not built

None of these is written before its ceiling is hit **and measured**: browser
writing for non developers (a git based CMS over the same files), a search
index when `grep` and `links` stop covering it, a coordination layer if
conflicts on the same files become frequent and counted.

## Documentation

- [docs/format.md](docs/format.md) is the file format, treated as a
  compatibility surface: what jalon writes, what it preserves, what it will
  never do to your files.
- [docs/workflow.md](docs/workflow.md) is how a person, a team and an agent use
  it, hooks included, and where GitHub fits.
- [docs/measuring.md](docs/measuring.md) is how to settle the bet this tool is
  built on, including what it can never measure about itself.
- [docs/release.md](docs/release.md) is the release procedure, the artifacts and
  the Homebrew question.
- [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) are the rules
  for humans and for coding agents.

## This repository uses jalon

Its own tasks live in [`.tasks/`](.tasks/), and CI runs the freshly built binary
against them on every pull request: a format change that breaks real files fails
there before it reaches anyone. It is the dogfooding and the regression corpus
at once.

## License

MIT, see [LICENSE](./LICENSE).
