# AGENTS.md

Short guide for AI coding assistants working on this repository.

## Scope

`github.com/AltSoyuz/jalon` is a file first task manager. The **core** is a
single `package main` at the repository root, six source files split by group of
verbs. Beside it sits one subpackage, `agent/`, which is the only part of jalon
that calls a model.

| File | Holds |
|---|---|
| `main.go` | Command dispatch, the `new`, `list`, `append`, `close` and `compact` verbs, directory and id resolution, the git wrapper. |
| `task.go` | The file format: front matter, sections, entries, ids, creation. |
| `digest.go` | The `digest` verb: assembling one block from the task, its links, git and the forge. |
| `render.go` | The markdown subset, the HTML templates, the CSS, the `render` verb. |
| `github.go` | The only place the core shells out to `gh`. The whole coupling to a forge is one file you can delete. |
| `metrics.go` | The opt in JSONL line written per invocation. No aggregation: `docs/measuring.md` reads it with `jq`. |
| `agent_cmd.go` | The only bridge to `agent/`: the `doctor`, `review` and `work` verbs, flag parsing and nothing else. |
| `agent/` | The orchestration layer: config, preflight, the measured review, the implementation behind the criterion. Its own package, on purpose. `docs/agent.md`. |

## Commands

```sh
make check        # gofmt + go vet + go test -race + dogfood
make dogfood      # run the freshly built binary against this repo's own .tasks/
make staticcheck  # separate: needs the tool, and CI runs it as its own job
make bench        # the render measurement, 500 tasks
make build        # bin/jalon
make release VERSION=vX.Y.Z   # every published platform into dist/
go test -run TestMarkdownSubsetGolden -update ./...   # rewrite the golden file
```

The Go version lives in `go.mod` and nowhere else: every workflow uses
`go-version-file: go.mod`, so bumping the toolchain is one line in one file.
After a Go upgrade, rebuild staticcheck (`go install
honnef.co/go/tools/cmd/staticcheck@latest`); it refuses modules newer than the
toolchain it was built with.

## This repository is jalon's first user

Track your work here with jalon, not beside it. Anything that will produce a
commit starts with a task and ends inside it.

1. `jalon list` to see what exists, then `jalon digest <id>` before touching
   anything, even when you think you already know the task. If that read does
   not orient you, the tool has failed at its only job, and that is the most
   valuable bug report this project can get.
2. `jalon new "<title>"` when no task covers the work. One coherent deliverable,
   one task.
3. `jalon append -decision <id> "..."` **when the arbitration happens**, not
   after it is implemented. A decision recorded afterwards is a conclusion, and
   conclusions do not stop the next agent from relitigating.
4. `jalon append <id> "..."` for what happened, including what failed.
5. Carry the id in the commit message, shortest unambiguous prefix:
   `[260806-slug] add the thing`.

Entries stay on one physical line, the way `jalon append` writes them. Do not
hand wrap them: a wrapped line breaks inline code spans in the rendered view.

## Rules

- **No dependency.** `go.mod` has no `require` block and must keep none. A
  proposal that adds one has to state which measured problem it solves and what
  owning that code instead would cost.
- **The file format is a compatibility surface.** Ids, section names, front
  matter keys and the `[id]` commit convention never change in place. Files
  written by an older version must keep parsing.
- **Unknown front matter keys are preserved.** Anything a user hand wrote
  survives a rewrite by `jalon`.
- **Failures are explicit.** No silent catch, no default that hides a
  misconfiguration. Missing git, missing `gh` and missing linked files are
  reported on stderr and never fatal.
- **Every cap is visible in the output** it applies to: a truncated log, a
  truncated file, an ambiguous id.
- **The markdown subset is a contract.** Extending it means extending
  `testdata/subset.md` and its golden output. Anything unsupported stays escaped
  literal text; nothing is ever guessed into HTML.
- **The core holds no model and calls none.** Rewriting `Context` is the user's
  job, by design. The agent layer is the exception and it is fenced: it lives in
  `agent/`, it is opt in, it needs `.jalon/agent.toml`, and a core file that
  imports it fails `TestCoreDoesNotImportAgent`.
- **The agent layer is removable.** `rm -rf agent agent_cmd.go`, drop three cases
  from the switch, and the task manager is intact: no model, no network, no key.
  That is what the boundary test protects, and it is the whole reason `agent/`
  is a package rather than six more files at the root.
- **No synchronization with a forge.** The bridge is one way, on demand and
  stateless, and in the core it lives entirely in `github.go`. Anything that
  needs a mapping table is out of scope; the reasoning is in `docs/workflow.md`.
  `agent/gh.go` is the equivalent for the agent layer: it reads issues and opens
  pull requests, and it still pushes no task state back to a forge.
- **`gh` is optional everywhere.** Its absence is reported on stderr and never
  fatal, except in `jalon new -issue`, where it is the whole point of the call.
- New behavior needs a test. Bug fixes need a regression test. Tests that touch
  `gh` use the `fakeGH` stub in `github_test.go` and stay offline; the agent
  layer uses the same technique in `agent/stub_test.go`, where `PATH` is the
  stub directory alone so that "this machine has no `claude`" is expressible.
- **The agent layer never gets a capability it can be denied instead.** jalon
  performs every git and `gh` mutation itself, so no phase is handed a commit, a
  push or a merge. `--allowed-tools` is a guardrail, not a sandbox; the worktree
  is the boundary.
- **This repository is published, so it names no particular setup.** Say what
  makes a fact checkable by someone with no access to the machine it was found
  on: "a real server", "a live site", "a target that is not jalon". A hostname,
  a username, an absolute home path or another repository's name is a coordinate
  that exists only in one place, and it survives forever once pushed. This
  applies to **commit messages as much as to files**, and they are the harder
  half: a message body is not in the diff a reviewer reads, which is how the
  rule was broken after it had been agreed.
- User visible changes get a `CHANGELOG.md` entry under `## [Unreleased]`. A tag
  whose section is missing fails the release workflow.
- The file format is documented in `docs/format.md`; a change there and a change
  in `task.go` go together.

## Release

Full procedure, artifacts and the Homebrew question: `docs/release.md`. Short
version: move the `[Unreleased]` entries to the target version, then tag.

```sh
git tag -s vX.Y.Z -m "vX.Y.Z"
git push origin main vX.Y.Z
```
