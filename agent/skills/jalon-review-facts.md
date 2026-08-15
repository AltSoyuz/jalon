---
name: jalon-review-facts
description: Gather the measured facts about a queued task by running probes and pasting their output verbatim. Phase one of jalon review.
---

# Gathering facts

Your entire output is a facts document. You are not designing anything, and you
are not proposing anything. If you find yourself writing "we should", "this
would" or "the fix is", stop and delete the sentence.

## Start from the digest, never from a cold read

Run `jalon digest <id>` on the task you were given: it inlines its linked
files, its commits and its issue thread when it names one. Then `jalon list`
and a digest of anything related. A digest costs five to twelve thousand
tokens; exploring a repository cold costs twenty to forty thousand and tells
you less. If no other task relates, say so in one line and move on.

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

### A `$ ` block means a shell command you actually ran

This is the rule people get wrong, and jalon stops the job over it.

A `$ ` line is a claim that you executed exactly that command in a shell. Only
write one when that is true.

Reading a file, searching the tree or listing paths with your own tools is not
a shell command. Report those in **prose, with a file and a line number**:

> `resolveTask` globs the id and parses only the matched file
> (`main.go:171-198`), so nothing re-reads every task per call.

Never dress a file read or a search up as `$ cat ...` or `$ grep ...`. That
turns a thing you did into a claim about a command that never ran, and jalon
refuses the whole document for it.

### One command per block, no shell plumbing

No pipes, no `;`, no `&&`, no subshells, no redirection. The allowlist matches
one command, so a composed line cannot be checked against it and jalon refuses
it. If you want a count, run the command and count in prose:

    ```console
    $ ls .tasks
    260806-a.md
    260807-b.md
    ```

    Two task files.

not `$ ls .tasks | wc -l`.

## When a probe is refused

**The exact list of commands you may run is given to you on stdin.** Read it
before you start. Anything outside it is denied, and there is no way around
that: no other shell, no rewriting it as a pipeline, no guessing the output.

If you need something that is not on the list, say so in **prose**, in the
"what could not be measured" section:

> `which` is not on the probe list, so I could not establish whether the
> binary on `PATH` is prebuilt or compiled per call.

**Never put a refused command in a `$ ` block.** A block means "I ran this and
here is the output". A command you were denied is the opposite of that, and
jalon stops the whole job when it finds one, because it cannot tell a denied
command from an invented one.

The person running this will add it to `probes.allowed` and re-run.

## Numbers, not adjectives

"slow" is not a fact. "p95 is 1.9 s over 200 requests, measured with the block
below" is. If you could not measure something, the fact is that you could not
measure it, and that is worth writing down too.

## Shape

Write, in this order:

1. **What the task claims**, in one or two lines: its title and its Context are what a person thinks; that is the premise.
2. **What was measured**, as command blocks with a line of context each.
3. **What could not be measured**, and why.

Nothing else. The next phase decides what any of it means.
