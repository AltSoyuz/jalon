---
name: jalon-review-facts
description: Gather the measured facts about a GitHub issue by running probes and pasting their output verbatim. Phase one of jalon review.
---

# Gathering facts

Your entire output is a facts document. You are not designing anything, and you
are not proposing anything. If you find yourself writing "we should", "this
would" or "the fix is", stop and delete the sentence.

## Start from the digest, never from a cold read

Run `jalon list`, then `jalon digest <id>` on anything related. A digest costs
five to twelve thousand tokens; exploring a repository cold costs twenty to
forty thousand and tells you less. If no task relates to this issue, say so in
one line and move on.

## Every claim carries the command that established it

For each thing you assert, run the command and paste the transcript in exactly
this shape:

    ```console
    $ curl -s http://localhost:8080/healthz
    {"status":"ok","uptime_s":91422}
    ```

The first line inside the fence is the command, prefixed with `$ `. What
follows is its output, verbatim and untouched. Do not summarize it, do not
tidy it, do not elide the boring middle.

**A claim with no command block above it does not belong in the document.**
jalon checks for those blocks and stops the job when it finds none, because a
review written on narration is worth nothing.

## When a probe is refused

The commands you may run are an allowlist. If you need one that is not
permitted, write down the exact command that was refused and stop. Do not work
around it, do not find another way, do not guess what it would have printed.
The person running this will add it to `probes.allowed` and re-run.

## Numbers, not adjectives

"slow" is not a fact. "p95 is 1.9 s over 200 requests, measured with the block
below" is. If you could not measure something, the fact is that you could not
measure it, and that is worth writing down too.

## Shape

Write, in this order:

1. **What the issue claims**, in one or two lines.
2. **What was measured**, as command blocks with a line of context each.
3. **What could not be measured**, and why.

Nothing else. The next phase decides what any of it means.
