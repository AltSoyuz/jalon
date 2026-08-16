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

## The two numbers that decide whether `work` lives

`docs/agent.md` states the kill criterion for `jalon work` in advance: over the
first 20 jobs, at least 12 branches merged without human correction and at most
about 5 USD per merged branch. Both numbers are read, not remembered, and
neither lives in the binary: one comes from the forge, one from this file.

**Branches merged without correction.** jalon pushes exactly one commit per
`work/<id>` branch, so a merged pull request with more than one commit is one a
person had to fix before merging. Over the last twenty merged work branches:

```sh
# Two steps: asking for the commits of 200 pull requests in one query exceeds
# the forge's node limit, so list cheaply, then read the twenty that matter.
for pr in $(gh pr list --state merged --limit 200 --json number,headRefName,mergedAt \
             --jq '[.[] | select(.headRefName | startswith("work/"))] | sort_by(.mergedAt) | .[-20:] | .[].number'); do
  gh pr view "$pr" --json number,commits --jq '"\(.number) \(.commits | length)"'
done | awk '{n++; if ($2 == 1) u++} END {printf "%d merged, %d untouched\n", n, u}'
```

`jalon recap` prints the same number every week, per target, with everything
else that waits on a person; see below.

`untouched` at 12 or above out of 20 is the bar. A closed-without-merge pull
request does not appear here; count those by hand with `--state closed`, and
count them against.

**Dollars per merged branch.** Every agent job appends a metrics line with
`cost_usd` when the CLI reported one, so the cost of the merged branches is a
join on the id:

```sh
gh pr list --state merged --limit 200 --json headRefName \
  --jq '.[] | .headRefName | select(startswith("work/")) | sub("^work/"; "")' \
  | sort -u > /tmp/merged-ids
jq -r 'select(.verb=="work" and .id!="") | "\(.id) \(.cost_usd // 0)"' ~/jalon-metrics.jsonl \
  | awk 'NR==FNR {merged[$1]=1; next} ($1 in merged) {sum+=$2; n++}
         END {printf "%d merged branches, %.2f USD total, %.2f USD per branch\n", n, sum, (n? sum/n : 0)}' /tmp/merged-ids -
```

Failed jobs cost money too and merge nothing; the same file has them
(`select(.verb=="work" and .err)`), and their cost belongs in any honest total.

When both numbers hold and you want the agent to do more, jalon proposes and
never flips: the unlock is a pull request that edits `.jalon/agent.toml`, which
a person merges. Nothing in the binary changes its own rights on a measurement.

## The weekly recap

`jalon recap` is the Sunday evening read: for each target on a machine, what
waits on a person, and the two numbers above. No model: it runs `jalon list`,
`gh` and `git` and reads the metrics file, one markdown block on stdout.

```sh
jalon recap -days 7 -metrics ~/jalon-metrics.jsonl \
  -notify 'curl -s --data-binary @- https://ntfy.example/topic' \
  ~/target-one ~/target-two
```

Per target: tasks `doing` for more than a fortnight (drift, not count), tasks
`proposed` that nobody queued (an agreement waiting silently), what is queued,
pull requests the agent opened and nobody merged or closed, wrecks in
`.jalon/failed/`, done tasks whose linked files changed since they closed (the
ground under a decision moved), and the merged work branches untouched by a
person. Then, machine wide, the jobs and the dollars of the week; a tick that
found nothing queued has no id and is not a job.

`-notify` is one command receiving the recap on stdin, the same idea as
`[notify]` in `agent.toml`; absent, the recap goes to stdout and the journal
has it. The recap spans whatever repositories you name, private ones
included, so it is never posted anywhere by default. Nothing here proposes
anything: what to do next comes out of what moved and what waits, read by a
person.

A unit and a timer for it, beside the hourly ones:

```ini
# /etc/systemd/system/jalon-recap.service
[Unit]
Description=jalon weekly recap
[Service]
Type=oneshot
User=jalon-agent
Environment=PATH=/home/jalon-agent/jalon/bin:/home/jalon-agent/.local/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
Environment=HOME=/home/jalon-agent
ExecStart=/home/jalon-agent/jalon/bin/jalon recap -metrics /home/jalon-agent/jalon-metrics.jsonl /home/jalon-agent/target-one /home/jalon-agent/target-two

# /etc/systemd/system/jalon-recap.timer
[Unit]
Description=run the jalon weekly recap on Sunday evening
[Timer]
OnCalendar=Sun 18:00
Persistent=true
[Install]
WantedBy=timers.target
```

Reverting is `systemctl disable --now jalon-recap.timer && rm
/etc/systemd/system/jalon-recap.*`.

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
