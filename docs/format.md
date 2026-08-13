# The file format

This is a compatibility surface, not an implementation detail. A file written
by any version of jalon must keep parsing in every later version. Changes are
additive: a new front matter key, a new section, never a rename in place.

The format is also designed to be usable **without** jalon at all. Everything
below is readable and editable by hand, rendered by any forge, and greppable.

## The file

One task is one markdown file in `.tasks/`, named `<id>.md`.

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

## The id

`YYMMDD-slug`, for example `260806-migration-auth`.

- `YYMMDD` is the creation date. Six digits, not four: `MMDD` repeats every
  year, which would make `git log --grep '[0806]'` return several years of
  unrelated commits from the second year of use.
- `slug` is the title, lowercased, accents folded, every other character
  collapsed to `-`, capped at 40 characters.
- No counter and no central registry, so two people can create tasks on two
  branches without coordinating. Two tasks created the same day with the same
  slug collide, and jalon refuses rather than overwriting.
- Commands accept any prefix that designates **exactly one** task
  (`jalon digest 260806-migration`). A prefix matching several tasks is a
  blocking error listing the candidates, never a guess: these commands are
  about to write to a file.

Note the deliberate asymmetry with commit tags, which are read only and
therefore generous: `[260806]` matches every task it is a prefix of and the
commit is listed on all of them, with a warning from `jalon render`. The same
string is refused as a command argument and accepted as a tag, because guessing
wrong costs nothing when reading and corrupts a file when writing.

## The front matter

A `---` fenced block of `key: value` lines at the very top of the file. It is
valid YAML, so a forge renders it as a table, but jalon parses it with a small
scanner rather than depending on a YAML library.

| Key | Meaning |
|---|---|
| `status` | `todo`, `doing` or `done` by convention. Any other value is accepted and gets its own group in the rendered index. |
| `created` | `YYYY-MM-DD`. Informational; the id already carries the date. |
| `links` | Repo relative paths, as a YAML flow sequence: `[a.go, b.md]`. `jalon digest` inlines their content. |
| `issue` | A GitHub issue number, set by `jalon new -issue N`. `jalon digest` shows the thread. |

**Keys jalon does not know are preserved as written.** Add your own; a rewrite
by `append`, `close` or `compact` will not drop them. Values are normalized to
`key: value` (single space), which is the only reformatting that happens.

## The sections

Three `##` headings, in the order an agent reads them. The order is the point:
one sequential read, most useful part first.

- **`## Context`** is bounded and *rewritten*. It answers "what is this and
  where does it stand" in a few paragraphs. It is the only section meant to be
  replaced rather than appended to.
- **`## Decisions`** is append only, one line per arbitration, with the reason.
  This is what an agent reads to avoid relitigating a settled question.
- **`## Log`** is append only, one line per event, and is truncated by
  `jalon compact`. The full history stays in git, which is what makes truncation
  safe.

Sections may hold any markdown. Headings inside fenced code blocks are ignored
by the scanner, so a task containing a markdown snippet does not confuse it.

## Entries

One entry is **one line**: `- YYYY-MM-DD <signature>: <text>`.

The one line rule is what makes `grep` predictable and truncation mechanical.
`jalon append` collapses newlines to keep it. If you wrap an entry by hand over
several lines, `jalon compact` drops the continuation lines together with their
entry rather than leaving orphan prose behind.

## The commit convention

The format's other half lives in commit messages:

```
[260806] fix the refresh token path
```

**A tag matches a task when it is a prefix of its id.** One rule: `[260806]`,
`[260806-migration]` and the full `[260806-migration-auth]` all reach the same
task, and you pick the shortest form that is unambiguous. `jalon render` warns
when a tag is a prefix of several tasks.

The whole message is searched, not just the subject, so one commit can carry
tags for the other tasks it touches in its body without crowding the first line.
The subject is what gets displayed.

Prefer the shortest unambiguous form: it keeps commit subjects inside the fifty
characters git recommends. The named cost is retroactive ambiguity, since a task
created later can share a prefix with a tag already written; the warning says so
and the parade is a longer tag on the day.

This convention is what makes the task to code link atomic, inside the commit
graph, and portable across any change of platform. It is also the part that
survives if you ever delete the binary.

## What jalon never does to your files

- It never rewrites a section it was not asked to change.
- It never drops an unknown front matter key.
- It never reorders sections or reflows prose.
- It never deletes a file.
- It never writes outside the tasks directory and the render output directory.

The agent layer (`jalon review`, see [`agent.md`](./agent.md)) is the one thing
that writes elsewhere, and only inside a throwaway git worktree it created and
removes: its working files never touch your checkout, and the only thing it
commits is a task file, on its own branch, in a pull request you merge or
discard.
