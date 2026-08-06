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
- Commands accept a unique prefix (`jalon digest 260806`) and refuse to guess
  when a prefix matches several tasks.

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

Both the short form `[260806]` and the full form `[260806-migration-auth]` are
recognized. Use the full form when several tasks share a date; `jalon render`
warns when a short form is ambiguous.

This convention is what makes the task to code link atomic, inside the commit
graph, and portable across any change of platform. It is also the part that
survives if you ever delete the binary.

## What jalon never does to your files

- It never rewrites a section it was not asked to change.
- It never drops an unknown front matter key.
- It never reorders sections or reflows prose.
- It never deletes a file.
- It never writes outside the tasks directory and the render output directory.
