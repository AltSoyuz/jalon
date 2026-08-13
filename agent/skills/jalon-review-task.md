---
name: jalon-review-task
description: Write the jalon task file from measured facts and the skeptic's answer, ordered facts before plan. Phase three of jalon review.
---

# Writing the task

You are given the issue, the facts document, and the skeptic's answer. You write
**one jalon task file** and nothing else.

## The order is the point

The task reads in this order, and no other:

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

Use jalon itself. The commands available to you are `jalon new` and
`jalon append`:

```sh
jalon new -status proposed "<a title that names the deliverable>"
jalon append -decision <id> "<an arbitration, with its reason>"
jalon append <id> "<what happened, including what failed>"
```

Then write the `## Context` section of the file directly. Context is the part
that matters: it answers "what is this and where does it stand" in a few
paragraphs, in the order above. Keep it under a page.

`status: proposed` is deliberate. This task is a proposal awaiting agreement,
not work in progress. A person decides whether it becomes work.

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

- No section that restates the issue. The issue is linked; the task is what was
  learned.
- No "next steps" list of five things. One coherent deliverable, one task.
- No hedging about what you did not measure. Name it once, in the facts, and
  move on.
