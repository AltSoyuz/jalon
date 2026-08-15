---
name: jalon-review-facts
description: Choose the probes that will establish the measured facts about a queued task. You propose commands; jalon runs them and writes the facts document from their real output. Phase one of jalon review.
---

# Choosing the probes

Your entire output is a list of commands, one per line. jalon runs every line
that is on the probe list, exactly as written, with no shell, and writes the
facts document itself from what each command really printed. You are not
designing anything, you are not proposing anything, and you are not writing the
facts: you are choosing which measurements to take.

## Start from the task, then from the tree

Read `.tasks/<id>.md`, the file you were given on stdin. Its title and Context
are what a person thinks; that is the premise to measure. Read the files it
links, and grep the tree for what it names. You have `Read`, `Grep` and `Glob`
and nothing else: you cannot run a command yourself, so what you learn by
reading is what tells you which commands are worth running.

## What to print

One command per line, and nothing else on the line. No prose, no headings, no
`$ `, no code fence, no comment: a line that is not a command is refused and
listed in the facts document as refused, and a document where nothing ran
stops the job.

**The exact list of commands you may run is given to you on stdin.** Every
line you print must start with one of them, followed by its arguments. Anything
else is refused, and there is no way around that: no other shell, no pipeline,
no `&&`, no redirection. If you want a count, ask for the listing; the next
phase counts.

Good:

    jalon digest 260813-health-endpoint-is-slow
    curl -s http://localhost:8080/healthz
    curl -s -o /dev/null -w %{time_total} http://localhost:8080/report

Refused:

    ls .tasks | wc -l
    $ curl -s http://localhost:8080/healthz
    Then I would check the logs.

## Choose measurements, not confirmations

Most task stubs assert something that was never checked: a thing is slow, a
case is hit, a component behaves some way. Pick the commands whose output
would settle that assertion either way. Numbers, not adjectives: a probe that
prints a duration or a count is worth three that print "ok".

If a measurement you need is not on the list, do not fake it and do not print
it: it will be refused and named in the document, and the person running this
adds it to `probes.allowed` and re-runs. Three to eight commands is the usual
size; twenty means you are exploring, and exploring is what `Read` is for.
