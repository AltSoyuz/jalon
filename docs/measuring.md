# Measuring whether this is worth it

jalon exists on a bet: that one `jalon digest` orients an agent for less than a
forge or free exploration costs. The bet has a refutation condition, and this
document is how you settle it.

## What jalon can and cannot see

jalon measures **its own cost**: what it emitted, how long it took, how many
linked files and commits it composed, whether it ran degraded. It cannot measure
the **counterfactual**, which is what the agent would have spent without it. Tool
calls, greps and file reads happen in your agent harness, not in this process.

No amount of built in observability changes that. Half the comparison has to be
recorded by hand, and that half is described at the bottom of this page. Skip it
and you will own a clean data file that cannot answer the question.

## Turning the recording on

Opt in with an environment variable. Unset, jalon writes nothing.

```sh
export JALON_METRICS="$HOME/.jalon-metrics.jsonl"
```

One JSON line per invocation, appended. Keep it outside the repository: it is
local to a machine, and committing it would conflict on every run for data
nobody reads in a diff.

```json
{"time":"2026-08-06T20:14:03Z","version":"v0.1.0","verb":"digest","id":"260806-migration-auth","ms":14,"bytes":4312,"tokens":1078,"files":2,"commits":6,"git":"ok","gh":"ok"}
```

The agent verbs add `cost_usd`, what the model calls of that job cost as the
CLI reported it; absent when it did not report one.

A write failure warns on stderr and never fails the command. Concurrent loops
are safe: one short line under `O_APPEND` does not interleave on POSIX.

## The three numbers that actually mean something

Read them with `jq`. There is no `jalon stats` verb on purpose: which averages
matter is not known yet, and a summary written today would freeze that guess
into code. If you type the same one liner a third time, that is when the verb
earns itself.

**Digests per task before it closes.** An agent that orients in one read digests
once per iteration. One that digests three times in a row is not orienting, it is
groping, and the digest is failing at its only job.

```sh
jq -r 'select(.verb=="digest") | .id' ~/.jalon-metrics.jsonl | sort | uniq -c | sort -rn
```

**The cost of one orientation.** The distribution matters more than the mean: a
median of 1000 tokens with a tail at 12000 means a few tasks link too much.

```sh
jq -s 'map(select(.verb=="digest") | .tokens) | sort | {n:length, min:.[0], median:.[length/2|floor], max:.[-1]}' ~/.jalon-metrics.jsonl
```

**Where the tokens go.** If the digest is many times the task file, you are
paying for linked files, and the fix is `links`, not the tool.

```sh
jq -r 'select(.verb=="digest") | "\(.id) \(.tokens) tokens \(.files) files \(.commits) commits"' ~/.jalon-metrics.jsonl
```

Two more worth a look when something feels wrong: degraded runs
(`select(.git=="absent" or .gh=="absent")`) explain a digest that looks
suspiciously cheap, and errors (`select(.err)`) show what you keep getting wrong
on the command line.

## The half that is not automatic

Over two weeks, on real tasks, record what your agent harness reports:

1. Take ten tasks. Start the iteration with `jalon digest <id>` as the first and
   only orientation step. Note the tokens the harness reports for that turn.
2. Take ten comparable tasks. Let the agent orient itself however it wants, from
   a forge issue or from the repository. Note the same number.
3. Write both numbers in a task file, one line per measurement, in the Log.

Twenty lines and fifteen minutes of discipline. That is the whole protocol.

## Reading the verdict

If `digest` does not clearly win, the honest conclusion is that the product
reduces to its conventions: the id in the commit message, the file format, the
three sections. Those cost nothing and stay. The verbs would then be worth
deleting, not defending.

If it wins, the number to publish is the one from your own repositories, not a
benchmark built to flatter it.
