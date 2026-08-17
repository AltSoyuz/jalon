# The agent layer

jalon's core holds no model and calls none. This layer does, and it is a
separate package for exactly that reason: `agent/` is opt in, and the task
manager works with no agent, no network and no key.

Removing it is `rm -rf agent agent_cmd.go`, dropping six cases from the switch
in `main.go`. `TestCoreDoesNotImportAgent` is what keeps that true.

## What it is for

Two things never happen on their own: an idea is never confronted with measured
reality before it is designed, and the backlog is never worked down. This layer
automates the first one.

A task stub becomes a **measured task proposal**: what was observed, what that
contradicts in the original idea, what it would cost, and only then a proposed
line of work. Never the plan first.

The human keeps three gates, and two of them are structural rather than
conventional:

| Gate | Enforced by |
|---|---|
| the decision to implement | `jalon work` builds only what a person named, by id or by setting a task's status to `implement` |
| the merge | branch protection on `main`, enforced on administrators too, and jalon has no merge call |
| production | deploy needs sudo; the agent user has none |

## The stages

```
task stub ──▶ review ──▶ [a person merges] ──▶ work ──▶ [merge] ──▶ [deploy]
 status:      (built)     and sets status:    (built)
 measure                  implement
```

The `status` field of the task file is the queue, and it is the whole remote
control. A person sets it to `measure` or `implement` by editing one line and
committing it to `main`, from the forge's phone app or a shell; the timer only
delivers it. Nothing here decides on its own what to measure or what to build.

The queue is read from origin's default branch with git alone: `git grep` on
`FETCH_HEAD` for the status, `git ls-remote` for what is already published.
There is no label, no forge call and no state outside git, so there is nothing
to synchronize and nothing to reconcile. The first version used two issue
labels and carried a second identifier through every job; reconciling the two
cost a whole task, and one id now runs from capture to done.

A stub is a title and a Context saying what you think, written by hand or by
`jalon new -issue N`, which seeds it from a forge issue in one call. Push it
with `status: measure` and the next tick measures it.

The phone's way in is `jalon capture`: one ntfy topic, one line per idea,
read at every tick and turned into a stub in the repository the line names.
The same topic is where jalon talks back, so the phone reads one thread:
what a person writes has no title, what jalon writes always has one
(`Title: jalon`), and capture skips every titled message. Under your line
come the acknowledgement (`captured: compass 260817-... (measure)`), then
the review's pull request, then the work's, in order.

```
compass: let a guest rate a run          -> a stub with status: measure
compass!: remove the neosync entry       -> status: implement, no review
buy milk                                 -> comes back: no repository in it
```

The same thread takes what a person has to say about a task that exists,
which is the three things a status edit or `jalon append` would do from a
shell; the id may be a prefix:

```
compass build 260817-on-the-start                -> status: implement, built at the next tick
compass decide 260817-on-the-start: <the choice, with its reason>   -> jalon append -decision
compass drop 260817-on-the-start                 -> jalon close
```

So agreeing to a proposal is one merge, and ordering the build is one line;
recording the arbitration the review asked for is one line too, and it lands
in the task's Decisions before `work` reads it.

No model chooses the repository: the first word does, because a word you
already type costs less than a wrong guess found a day later. `altsoyuz`
answers for `altsoyuz.com`. A line without a known prefix is sent back
through `-notify` and creates nothing, so nothing is silently lost and nothing
is silently invented. The stub is written in a fresh worktree of the target,
committed as `[<id>] capture: <line>` and pushed to the default branch; a
protected branch gets a `capture/<id>` branch and a pull request instead. The
cursor is one line in a file, the id of the last message handled, so a run
that fails on a push retries the same line next tick and a burst is bounded
to twenty lines per tick.

```sh
jalon capture -inbox https://ntfy.example/inbox -notify '<the same curl>' \
  /home/jalon-agent/target-one /home/jalon-agent/target-two
```

`$NTFY_TOKEN` is the bearer that may read and write the topic. From the
phone, the ntfy web app (added to the home screen) publishes to the topic
and shows the thread; the native app carries the push; an Apple Shortcut or
Siri can post one line in one gesture.

**Built today: `jalon doctor`, `jalon review` and `jalon work`.** There is no
`triage`: queueing a stub is one line a person edits, and a model deciding what
deserves measuring would be the one thing this layer refuses to automate.

The two model-spending verbs answer two different questions, and using one for
the other's job is waste rather than error. `review` is for an idea you doubt:
it measures, tries to refute, and proposes. `work` is for a task you have
already agreed to. A directive you are sure about (`remove this entry`) does not
need a review: write the task yourself with `jalon new` and a sentence, then run
`work` on it. The first time that distinction was missed, a review spent three
model calls and an `npm ci` to locate a line that `grep` finds instantly.

## `jalon doctor`

Preflight, and the answer to a measured problem: of three manual agent runs,
three needed a human intervention, and all three were environmental rather than
judgment. `doctor` pays that debt once per server.

```sh
jalon doctor
```

Every check runs even after one fails, because fixing a fresh server one error
per run is the exact ergonomic failure this verb exists to prevent. A check
whose prerequisite failed prints `skip` and names what it was waiting for: a
silently skipped check is indistinguishable from a passing one.

stdout is one aligned record per check, meant for `awk '$1=="fail"'`. stderr
carries the fix for each non-`ok` check. The exit status is 1 if anything
failed.

`jalon review` runs the same checks and refuses to start on any failure. A red
base gets no job and no workaround.

**`jalon doctor -live` spends one real model call** to prove the model answers.
It is off by default because it costs money: the measured floor for a single
`claude` invocation is above five cents, so a check that ran on every hourly
tick would be a standing bill. Tools are disabled for that call, so it cannot
enter a loop and bill like a job.

Run it after setting up a machine, and after anything that touches credentials.
Everything else `doctor` checks establishes that a binary exists and takes the
right flags; none of that catches an expired token. On a real server this gap
cost two failures in a row, both discovered only after a review had written its
facts and paid for three model calls.

## `jalon review <task-id>`

Runs on the server that hosts the target app, so the probes are local `curl`
calls against a running instance. That locality is the feature: it is what makes
the facts facts.

1. Load the config, `git fetch origin <default_branch>`, and read the task
   from what that retrieved: `-next` takes the oldest task whose status is
   `measure` that has no `task/<id>` branch on origin and no wreck parked in
   `.jalon/failed/`. Nothing queued is a success, not a failure. Then the
   `doctor` checks, refuse on any failure.
2. Create an ephemeral git worktree
   detached from what that retrieved, and materialize the embedded skills into
   its `.claude/skills/`. The job runs on what the forge has, never on what this
   clone happens to hold: nothing updates a target repository's checkout, since
   the unit pulls the jalon checkout and only that. A failed fetch is fatal,
   unlike that pull, because running on the tree you already have is the defect
   rather than a reasonable answer to a blip. Your own branch and working tree
   are never touched.
4. **Phase 1, the probes.** `claude -p` with `Read`, `Grep` and `Glob` and no
   shell at all. It reads the task and the tree and prints the commands worth
   running, one per line. jalon then runs, itself and with no shell, every line
   that is on `probes.allowed` and not composed of several commands, and writes
   `facts.md` from the real output: the command as asked, what it printed, the
   exit status when it failed. No block in that document was written by a
   model, so a fabricated one is impossible rather than caught.
   jalon then appends its own grounding to `facts.md`: for each significant
   word of the task's title, the files whose content carries it (`git grep
   -il`, capped and said). Deterministic and free, and it is what the first
   live review lacked when it refuted a premise on directory names while the
   feature sat in a file it never opened.
5. **The gate.** At least one probe ran, counted by jalon. It is the whole gate.
   Writing before measuring is impossible by construction. Refused lines (not on
   the list, composed, or naming a program the machine lacks) are named in the
   document, on stderr and in the pull request body, and are never fatal while
   one probe ran, because three live runs stopped on how a command was written
   while the measurements were sound every time.
6. **The skeptic.** A second, separate read-only invocation whose only task is
   to refute the task's premise with a command. One pass, no loop, no debate.
7. **Phase 2, the task file.** The model may now write, and rewrites exactly
   one thing: the task it was given, in place, ordered measured / contradicted /
   cost / plan. Its file permission is `Edit(.tasks/**)`; a `Write(...)` rule
   is never matched by Claude Code's checks, so offering one only sends the
   model down a path that gets denied. git then says what happened under
   `.tasks/`: that one file modified and nothing else, or the job stops. jalon
   sets `status: proposed` itself.
8. jalon does every mutation itself: commit, push the `task/<id>` branch, open
   the pull request. The model is never handed a push. The branch is also what
   takes the task out of the queue: `main` is protected and jalon has no merge
   call, so a branch is the only mark it can leave, and a pull request closed
   without merging keeps the task out until someone deletes the branch, which
   is the right default because a person looked and said no.

The result is a pull request containing only the task file, with
`status: proposed`. That is not a new mechanism: it is the one already
documented in [`workflow.md`](./workflow.md) for agreeing on a task before
anyone writes code. Merging **is** the agreement; setting the status to
`implement` afterwards is the order to build.

Each phase leaves what it printed in `.jalon-review/`, as `probes.md`,
`skeptic.md` and `task.md`, beside the `facts.md` jalon wrote, all written
before the phase's error is checked. A phase
whose output is discarded cannot be diagnosed when it fails, and the writing
phase failed on a real job having created nothing readable at all.

On failure the worktree is kept, parked under `.jalon/failed/<job>`, and the
error names its path and the command that removes it. The evidence of what went
wrong is the only thing worth having at that point, and deleting it to look tidy
would throw away the bug report. Parking it is what keeps the evidence without
freezing the machine: `.jalon/worktrees/` is empty again, so the next tick runs
the next item, and the wreck keeps its task out of the queue until it is read
and removed, so a failing task is retried by a person and not by the clock. `doctor` warns while wrecks are kept; at ten, the next job
refuses until some are removed, because a machine failing every tick must stop
within a night and not fill the disk.

## `jalon work <task-id>`

Implements one task you have already agreed to, and opens a pull request only if
the repository's own criterion passes.

It never takes an issue as the thing to build: the merged task **is** the
agreement, and reading an issue directly would duplicate `review` and remove the
gate the whole design rests on.

`jalon work -next` takes the oldest task on origin whose status is `implement`
and that has no `work/<id>` branch and no parked wreck. That is not autonomy:
the status is a button a person pressed, and the timer only delivers it. jalon
still never chooses what to build.

The point of it is reach, not automation. The status field is the whole remote
control: with the forge's phone app you can capture an idea, agree to a task and
order it built, without a shell, an SSH key, or a network route to the server.

1. Fetch origin and read the task from what that retrieved, `-next` resolving
   the queue first, before the preflight: a tick that finds nothing queued must
   not pay for the repository's whole test suite and throw the result away. A
   task that is not on origin, or is `done`, fails here, before a single token
   is spent.
2. The `doctor` checks, refuse on any failure. This runs the criterion **before**
   the model touches anything, which is what makes the same criterion meaningful
   afterwards: a red result at the end belongs to this job, not to a base that
   was already broken.
3. Ephemeral worktree, skills materialized, exactly as `review`.
4. **One phase.** It may edit the checkout and run the commands in
   `probes.allowed`, which is how it iterates on the criterion inside its own
   invocation. jalon runs no loop of its own: a retry here would be an unbounded
   cost with no bound on quality. Its file permission is `Edit`, never a `Write`
   rule, for the reason given above.
5. **git decides whether anything happened.** A phase that says it implemented
   something and changed no file is caught here, not by a reader.
6. **The criterion is the gate, and the whole gate.** Where `review` needed Go
   code to check that the model had measured, because that cannot be verified
   mechanically, this either exits 0 or it does not. No judgement on the model's
   prose anywhere.
7. jalon commits the checkout minus its own scratch, pushes `work/<id>`, opens
   the pull request. The commit message carries `closes <id>`, so merging closes
   the task through the existing hook, and the pull request body carries
   `Closes #N` when the task's front matter names an issue, so the same merge
   closes that too. Both are the existing mechanisms of jalon and of the forge;
   jalon keeps no state about either. The model is never handed git or `gh`. As
   with `review`, the branch is what takes the task out of the queue.

The pull request body is what a person reads on a phone in the morning, so it
is ordered the way a digest is: `jalon digest -offline <id>` run in the
worktree, `git diff --stat`, the criterion's last line, and the cost of the job
in dollars. The cost comes from the CLI's JSON envelope (`--output-format json`,
`total_cost_usd`), summed over the phases; a `review` body carries it too. When
the CLI did not report one the body says `unknown` rather than printing zero,
and the same number lands in `JALON_METRICS` as `cost_usd`, because the kill
criterion below is stated in dollars per merged branch and until this was read
no file recorded a dollar amount.

What the criterion proves is exactly what it proves. On this repository that is
`make check`, which is a real bar. On a static site it is `npm ci && npm run
build`, which proves the thing compiles and nothing else: there, `work` will
produce green pull requests that can still be wrong, and the real gate is your
eyes on the diff.

The phase output lands in `.jalon-work/work.md`, and a failure parks the
worktree under `.jalon/failed/` so the diff that failed can be read, exactly as
`review` does.

### What would kill it

Written before the first line of it existed, and unchanged since: over the first
20 jobs, at least 12 branches merged without human correction and at most ~5 USD
per merged branch. Otherwise `work` is deleted and capture plus `review` stay.

Both numbers are computed, not remembered: `docs/measuring.md` has the two
one-liners, one over the forge's merged pull requests (a work branch carries
exactly one commit, so a second commit is a correction) and one over
`JALON_METRICS`, where every job leaves its `cost_usd`. If the numbers hold and
more autonomy is wanted, jalon proposes and never flips: the change is a pull
request on `.jalon/agent.toml` a person merges.

## The configuration

`.jalon/agent.toml`, versioned in each target repository. Every key is required
except `[notify]`. There are no defaults: a missing key is a refusal naming the
line to write, because a default that hides a misconfiguration is the failure
mode this whole file exists to avoid.

```toml
# .jalon/agent.toml

[agent]
# Pinned ids, not aliases. "sonnet" resolves to whatever the latest sonnet is,
# so an alias changes the model under you without anything saying so. On a
# server you want reproducible, name the model.
model_triage           = "claude-haiku-4-5"
model_review           = "claude-sonnet-5"
model_work             = "claude-sonnet-5"
max_budget_usd_per_job = 3.0
daily_job_cap          = 10
counter_file           = ".jalon/agent-jobs-today"
timeout_seconds        = 900

[review]
default_branch = "main"
worktrees      = ".jalon/worktrees"   # gitignored, inside the repo so it stays visible

[criterion]
command = "make check"   # must exit 0; never put "jalon doctor" here, it recurses

[probes]
# The only commands a phase may run. Prefix match: "jalon digest" covers
# "jalon digest 260813-x". No shell characters, so list one command per line.
allowed = [
  "jalon digest",
  "jalon list",
  "curl -s http://localhost:8080/healthz",
  "make check",
]

[notify]
# Optional. One command, receiving the message on stdin. jalon does not know
# your chat service; it knows one command. Absent, the message goes to stdout,
# which the journal already records.
command = "..."
```

`model_triage` and `model_work` are validated but unused until those verbs
land, so a server's file does not have to change twice.

Note the two dot directories: `.tasks/` is what jalon walks up to find, and
`.jalon/` sits beside it at the repository root.

## Skills: the method ships inside the binary

Three layers, three homes, no overlap:

1. **The method**, common to every repository, is `agent/skills/*.md`, embedded
   with `go:embed`. The skills version *is* the binary version, so one `scp`
   ships the tool and its method together, and they cannot drift.
2. **Repository knowledge** stays in the target repository (`CLAUDE.md`,
   `AGENTS.md`, `.claude/rules/`, `docs/`). jalon never carries or duplicates
   it; the agent finds it where it works, because the worktree is a full
   checkout and Claude Code loads those files from it as in any session.
3. **Machine configuration** is `.jalon/agent.toml`, above.

The method includes how to build, not only what to measure: `jalon-work`
carries the lens jalon itself is built with (smallest composition of what
exists, nothing for a hypothetical future, boring over clever, measure rather
than guess, explicit failures, reversible and observable, compatibility
sacred), so a `work` job applies it on every target without each repository
restating it. What a repository wants on top of that (a review rule for a
money path, a house style) is repository knowledge and goes in its own files.

The skills are materialized into the ephemeral worktree and die with it.
Nothing is installed under the agent user's home, because that is
out-of-git state that drifts between servers.

## Blast radius, honestly ranked

1. **The ephemeral worktree is the real boundary.** It is a detached checkout,
   deleted on success, and it is the layer that holds even if the other two
   fail. It matters more since `work` exists: `review` never let a phase write
   outside `.tasks/`, and `work` writes source code by definition. The
   containment is unchanged, the stake is not.
2. **Branch protection, enforced on administrators too.** Nothing reaches `main`
   outside a pull request with green checks, and that setting is the only thing
   in this list the forge enforces. It was measured rather than assumed: with
   administrators exempt, a direct push to `main` succeeded and printed
   `Bypassed rule violations`; with the exemption removed, the same push is
   refused with `GH006`.

   Say what the token is, because the earlier claim here was wrong. The agent
   authenticates as the repository owner, so it is not a lesser identity that
   the forge holds back: what keeps it from merging its own pull request is that
   **jalon contains no merge call at all**, and what keeps it off `main` is the
   protection above. The agent user has no sudo, and no `NOPASSWD` exception
   exists.
3. **`--allowed-tools` is a guardrail, not a sandbox.** Bash rules are matched
   as prefixes against the command string, so shell composition walks around
   them: the first live run of `jalon review` produced
   `ls -la .tasks/*.md | wc -l`, which an entry for `ls` covered. jalon refuses
   every shell metacharacter in a probe, and the probes it runs itself for the
   facts go through no shell at all. The skeptic and the work phase still hold
   a shell, and nothing stops a model from composing a line there at runtime.
   Anything reading this design as "the model is sandboxed by the allowlist"
   has it wrong.

Probes are local `curl` by design, which means the agent user can reach whatever
listens on localhost. Keep the allowlist to read-only endpoints, and put a
comment on any probe that is not.

## Known limits

- **The probes are chosen in one shot.** The gathering phase has no shell, so
  it cannot run a probe, read the answer and choose the next one; it reads the
  tree and names its measurements once. The skeptic still iterates with a
  shell. If a review keeps refusing the same line, the fix is one entry in
  `probes.allowed`, and the refused line is in the pull request body to copy.
  Earlier versions let the phase run the probes itself and checked its prose
  for command blocks; a live run produced a plausible `$ which gh` block for a
  command that never ran, which is the class this design removes.
- **POSIX only.** The criterion runs through `sh -c`, and the tests stub
  programs with `/bin/sh` scripts. Windows is already outside the test matrix.
- **Pinned model ids go stale, and that is the point.** An alias silently
  retargets when a new model ships, which is a change to the thing you measured
  with nothing in git to show it. A pinned id fails loudly or keeps behaving the
  same, and moving it is a commit you can revert. Re-pin deliberately, after a
  run you looked at.

## Deployment

One binary per server, one `scp`. No state outside git and the systemd units:
the counter file is one line holding an integer, and diagnosis is
`journalctl -u jalon-agent`, `cat` on that counter, and the last `facts.md`.

**The job updates itself.** The unit pulls and rebuilds before each tick, and
runs the binary it just built, out of the checkout the agent user owns. There is
no second copy in `/usr/local/bin` to keep in step, and no `sudo` anywhere in
the update path: a merge is in production at the next tick.

What you give up is choosing *when* the server changes version. You choose
*what*, by merging; the deployment follows within the hour. The criterion is
still the gate — a `main` that compiles but fails its tests blocks the job — and
`JALON_METRICS` records which stamped version produced which task, because with
the version moving on its own that is the only way to say afterwards.

Reverting is a `git revert` on `main`, picked up at the next tick, or removing
the two `ExecStartPre` lines to freeze the deployed version.

There is deliberately **no `jalon update` verb**. It would duplicate `git` and
`gh`, which are already on the machine, in a hundred and fifty lines of Go with
a network client, checksum verification and atomic self-replacement — for one
usage. Releases would also cost a tag per deploy, which is more maintenance, not
less. The day a target repository has no Go toolchain, the binary cannot be
built in place and `gh release download` in an `ExecStartPre` is the answer;
still not a verb.

**jalon emits the setup; it never performs it.**

```sh
# -repo is the target the agent works in; -jalon is the checkout the unit pulls,
# builds and runs. They differ for every target except jalon itself.
jalon agent-init -repo /srv/app -jalon /home/jalon-agent/jalon \
                 -user jalon-agent -port 8080 > setup.sh
less setup.sh            # read it
sh setup.sh              # unprivileged: writes .jalon/agent.toml and says what is next
sudo sh setup.sh --root  # only after doctor and one real review have passed
```

The agent user needs its own clone of the target repository, its own `gh` login
and its own model credentials: yours live in your home and a system user cannot
read them. Nothing needs to be installed system wide.

The output is a configuration file, a systemd unit, a timer, and the commands
that install them. The privileged half is behind an explicit `--root` flag, so
running the script the obvious way creates no user and writes nothing into
`/etc`. The units are embedded in the binary rather than shipped as files in
this repository, because the thing you copied to the server is the binary: a
unit file you would have to clone jalon to find is a unit file you do not have.

There is no `jalon setup` that does it for you, on purpose. It would need root
in the one binary whose entire blast-radius argument is that the agent user has
no sudo, and it would put Debian specifics into a tool that has no
operating-system-specific line and cross-compiles to nine platforms. It would
also run fewer than ten times over the life of the tool. Emitting text costs
sixty lines, is inspectable before it runs, and `diff`s against what is
deployed.

A second, faster timer (every fifteen minutes is plenty; it is one HTTP poll)
runs `jalon capture` over every target, so an idea typed at noon is a stub by
a quarter past and measured at the next hourly tick:

```ini
# /etc/systemd/system/jalon-capture.service
[Unit]
Description=jalon capture: inbox topic to task stubs
[Service]
Type=oneshot
User=jalon-agent
EnvironmentFile=/etc/jalon-agent.env
Environment=PATH=/home/jalon-agent/jalon/bin:/home/jalon-agent/.local/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
Environment=HOME=/home/jalon-agent
ExecStart=/home/jalon-agent/jalon/bin/jalon capture -inbox ${NTFY_URL}/jalon -notify 'curl -fsS -H "Authorization: Bearer $NTFY_TOKEN" -H "Title: jalon inbox" --data-binary @- "$NTFY_URL/jalon"' /home/jalon-agent/target-one /home/jalon-agent/target-two

# /etc/systemd/system/jalon-capture.timer
[Unit]
Description=run jalon capture every fifteen minutes
[Timer]
OnCalendar=*:0/15
Persistent=true
[Install]
WantedBy=timers.target
```

The timer runs `jalon review -next` then `jalon work -next`: the oldest task
with status `measure`, then the oldest with status `implement`, as origin has
them. Nothing queued is a success, not a failure, so an idle queue does not
turn the unit red every hour.

**The status is the queue, and a published branch leaves it.** Without that the
same task is measured again on the next tick, forever, spending the daily cap
on one question, which is exactly what happened here before the timer was even
armed: one issue, two near-identical task proposals. The branch is the mark
because it is the only one jalon can leave: `main` is protected and there is no
merge call.

A failed job leaves the queue too, for as long as its wreck is parked in
`.jalon/failed/`. The first version kept the worktree in place so that a
failure would stop the timer; on a real timer that turned one failure into a
frozen night. Now one failure costs one item, the wreck is still there to read,
and the cap of ten wrecks is what stops a machine failing in a loop.

Two things the runbook will remind you of, because both are easy to miss:

- **The agent user needs its own model credentials.** Yours live in your home
  directory and a system user cannot read them. Run `claude setup-token` as
  that user, or give it an API key through an `EnvironmentFile`.
- **Reverting is two commands**, and the script prints them:
  `systemctl disable --now jalon-agent.timer && rm /etc/systemd/system/jalon-agent.*`
