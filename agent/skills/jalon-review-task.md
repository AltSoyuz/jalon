---
name: jalon-review-task
description: Rewrite the queued task file in place from measured facts and the skeptic's answer, ordered facts before plan. Phase three of jalon review.
---

# Rewriting the task

You are given a task file as a person wrote it, the facts document, and the
skeptic's answer. You rewrite **that one file** and nothing else. You do not
create a task: the id already exists, and it is the one that will run from here
to done.

## The order is the point

The `## Context` section reads in this order, and no other:

1. **What was measured.** The numbers, with the commands that produced them.
2. **What that contradicts** in the original idea. Cite the skeptic where it
   found something. If the premise was refuted, that goes here, in the second
   paragraph, not buried at the end.
3. **What it would cost.** Rough is fine; absent is not.
4. **Only then, a proposed line of work.**

Putting the plan first is the failure this whole stage exists to prevent. A plan
that arrives before the measurement gets believed, and then the measurement
never happens.

If the skeptic refuted the premise, **the proposal may well be "do nothing"**.
That is a good outcome, not a failed run. Say so plainly and say why.

## How to write it

Edit `.tasks/<id>.md` directly for the `## Context` section and the `links`
key. Replace what the person wrote; the original is one `git log` away and the
task is what was learned. Keep Context under a page.

For arbitrations, use jalon itself:

```sh
jalon append -decision <id> "<an arbitration, with its reason>"
jalon append <id> "<what happened, including what failed>"
```

Do not touch the `status` line: jalon sets it to `proposed` itself when you are
done, because this task is a proposal awaiting agreement, and merging is the
agreement. Do not touch `created` or `issue`.

### Fill `links` before you stop

Set the `links` front matter key to the repository-relative paths of the files
this task is actually about, as a flow sequence:

```
links: [task.go, main.go, docs/format.md]
```

**A task that names files in its prose and leaves `links: []` is not finished.**
`jalon digest` inlines the content of every linked file, so this key is the
difference between the next reader getting the two files that matter and
grepping the repository for them. If you cited a path anywhere in the Context,
it belongs here. Three or four paths, not ten: this is the shortlist, not a
bibliography.

## Conventions that are not negotiable

- **English**, always. Everything versioned in this project is in English.
- **One entry is one physical line.** `jalon append` collapses newlines to keep
  it that way; do not hand wrap.
- **No AI attribution anywhere.** No "generated with", no co-author trailer.
- Record a decision **at the moment of the arbitration**, with its reason. A
  decision written afterwards is a conclusion, and conclusions do not stop the
  next reader from relitigating.
- Do not commit, do not push, do not open a pull request. jalon does all of
  that itself once you are done.

## What not to write

- No section that restates what the person wrote. The task is what was
  learned.
- No "next steps" list of five things. One coherent deliverable, one task.
- No hedging about what you did not measure. Name it once, in the facts, and
  move on.
- No second task file. One id, rewritten in place; jalon refuses the run if
  anything else under `.tasks/` changed.
