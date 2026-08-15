# The agent layer

jalon's core holds no model and calls none. This layer does, and it is a
separate package for exactly that reason: `agent/` is opt in, and the task
manager works with no agent, no network and no key.

Removing it is `rm -rf agent agent_cmd.go`, dropping two cases from the switch
in `main.go`. `TestCoreDoesNotImportAgent` is what keeps that true.

## What it is for

Two things never happen on their own: an idea is never confronted with measured
reality before it is designed, and the backlog is never worked down. This layer
automates the first one.

An issue becomes a **measured task proposal**: what was observed, what that
contradicts in the original idea, what it would cost, and only then a proposed
line of work. Never the plan first.

The human keeps three gates, and two of them are structural rather than
conventional:

| Gate | Enforced by |
|---|---|
| the decision to implement | `jalon work` builds only what a person named, by id or by the `implement` label |
| the merge | branch protection on `main`, enforced on administrators too, and jalon has no merge call |
| production | deploy needs sudo; the agent user has none |

## The stages

```
issue ──▶ triage ──▶ review ──▶ [a person merges] ──▶ work ──▶ [merge] ──▶ [deploy]
 measure  (not yet)  (built)     and labels it        (built)
                                    implement
```

The two labels are the queues, and they are the whole remote control. A person
adds one from the forge's phone app; the timer only delivers it. Nothing here
decides on its own what to measure or what to build.

Issues are the queue between servers. They carry no state beyond labels, so
there is nothing to synchronize and nothing to reconcile.

**Built today: `jalon doctor`, `jalon review` and `jalon work`.** `triage` is
designed but not written; see the task in `.tasks/`.

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

## `jalon review <issue-number>`

Runs on the server that hosts the target app, so the probes are local `curl`
calls against a running instance. That locality is the feature: it is what makes
the facts facts.

1. Load the config, run the `doctor` checks, refuse on any failure.
2. Read the issue: one `gh issue view`.
3. `git fetch origin <default_branch>`, then create an ephemeral git worktree
   detached from what that retrieved, and materialize the embedded skills into
   its `.claude/skills/`. The job runs on what the forge has, never on what this
   clone happens to hold: nothing updates a target repository's checkout, since
   the unit pulls the jalon checkout and only that. A failed fetch is fatal,
   unlike that pull, because running on the tree you already have is the defect
   rather than a reasonable answer to a blip. Your own branch and working tree
   are never touched.
4. **Phase 1, facts.** `claude -p` with no write tool at all. Its Bash policy is
   built from `probes.allowed` and nothing else. jalon captures its stdout and
   writes `facts.md` itself.
5. **The gate.** Go code, not a prompt, checks that `facts.md` is not a stub and
   holds at least one executed command block. Writing before facts is impossible
   by construction. It stops the job for narration and nothing else; a command
   that is not a probe, or one composed of several, is reported on stderr and in
   the pull request body for a person to check, because three live runs stopped
   on how a command was written while the measurements were sound every time.
6. **The skeptic.** A second, separate read-only invocation whose only task is
   to refute the issue's premise with a command. One pass, no loop, no debate.
7. **Phase 2, the task file.** The model may now write, and writes exactly one
   thing: the jalon task, ordered measured / contradicted / cost / plan. Its
   file permission is `Edit(.tasks/**)`; a `Write(...)` rule is never matched by
   Claude Code's checks, so offering one only sends the model down a path that
   gets denied.
8. jalon does every mutation itself: commit, push the branch, open the pull
   request. The model is never handed a push.

The result is a pull request containing only the task file, with
`status: proposed`. That is not a new mechanism: it is the one already
documented in [`workflow.md`](./workflow.md) for agreeing on a task before
anyone writes code. Merging **is** the agreement.

Each phase leaves what it printed in `.jalon-review/`, as `facts.md`,
`skeptic.md` and `task.md`, written before the phase's error is checked. A phase
whose output is discarded cannot be diagnosed when it fails, and the writing
phase failed on a real job having created nothing readable at all.

On failure the worktree is kept, parked under `.jalon/failed/<job>`, and the
error names its path and the command that removes it. The evidence of what went
wrong is the only thing worth having at that point, and deleting it to look tidy
would throw away the bug report. Parking it is what keeps the evidence without
freezing the machine: `.jalon/worktrees/` is empty again, so the next tick runs
the next item, and the failed issue leaves the queue with a message saying how
to put it back. `doctor` warns while wrecks are kept; at ten, the next job
refuses until some are removed, because a machine failing every tick must stop
within a night and not fill the disk.

## `jalon work <task-id>`

Implements one task you have already agreed to, and opens a pull request only if
the repository's own criterion passes.

It never takes an issue number as the thing to build: the merged task **is** the
agreement, and reading an issue directly would duplicate `review` and remove the
gate the whole design rests on.

`jalon work -next` takes the oldest open issue labelled `implement` and builds
**the task that names it**, through the `issue:` key the review wrote. That is
not autonomy: the label is a button a person pressed, and the timer only
delivers it. jalon still never chooses what to build, and an issue whose number
no task carries is a refusal naming both rather than a guess.

The point of it is reach, not automation. The labels are the whole remote
control: with the forge's phone app you can capture an idea, agree to a task and
order it built, without a shell, an SSH key, or a network route to the server.

1. On `-next`, resolve the queue first, before the preflight: a tick that finds
   nothing labelled must not pay for the repository's whole test suite and throw
   the result away.
2. The `doctor` checks, refuse on any failure. This runs the criterion **before**
   the model touches anything, which is what makes the same criterion meaningful
   afterwards: a red result at the end belongs to this job, not to a base that
   was already broken.
3. Read the task from `.tasks/<id>.md`. A missing or `done` task fails here,
   before a single token is spent.
4. Ephemeral worktree, skills materialized, exactly as `review`.
5. **One phase.** It may edit the checkout and run the commands in
   `probes.allowed`, which is how it iterates on the criterion inside its own
   invocation. jalon runs no loop of its own: a retry here would be an unbounded
   cost with no bound on quality. Its file permission is `Edit`, never a `Write`
   rule, for the reason given above.
6. **git decides whether anything happened.** A phase that says it implemented
   something and changed no file is caught here, not by a reader.
7. **The criterion is the gate, and the whole gate.** Where `review` needed Go
   code to check that the model had measured, because that cannot be verified
   mechanically, this either exits 0 or it does not. No judgement on the model's
   prose anywhere.
8. jalon commits the checkout minus its own scratch, pushes, opens the pull
   request. The commit message carries `closes <id>`, so merging closes the task
   through the existing hook, and the pull request body carries `Closes #N` when
   the task names an issue, so the same merge closes that too. Both are the
   existing mechanisms of jalon and of the forge; jalon carries two identifiers
   and keeps no state about either. The model is never handed git or `gh`.

   A task written by hand names no issue, and then there is nothing to close. A
   review's task names one because the writing phase runs `jalon new -issue N`;
   if it forgets, the review warns and still publishes, because a measured
   review is worth more than a missing convenience line.

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
   `docs/`). jalon never carries or duplicates it; the agent finds it where it
   works.
3. **Machine configuration** is `.jalon/agent.toml`, above.

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
   every shell metacharacter in a probe, and the gate refuses a *reported*
   command containing one, so the facts cannot claim a composed line. Neither
   stops a model from composing one at runtime. Anything reading this design as
   "the model is sandboxed by the allowlist" has it wrong.

Probes are local `curl` by design, which means the agent user can reach whatever
listens on localhost. Keep the allowlist to read-only endpoints, and put a
comment on any probe that is not.

## Known limits

- **The gate does not prove execution.** It proves the phase produced a command
  block rather than narration, which is the common failure. A fabricated block
  passes, and a live run produced exactly that: a `$ which gh` block with
  plausible output for a command that was not a probe, where the document itself
  cannot say whether it ran or was inferred from the environment. Removing that
  class means having jalon run the probes itself and build `facts.md` from its
  own output; that is the intended next step, not a thing that is done.
- **A `$ ` block is a claim about a shell command.** The gathering phase also
  has read tools, and its first live run rendered a `Grep` tool call as
  `$ grep -rn ...`, which the gate then refused because `grep` was not a probe.
  The skill now reserves `$ ` blocks for real shell commands and asks for prose
  with a file and a line number otherwise. If a review fails on a command you
  did not put in `probes.allowed`, check which of the two it was.
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

The timer runs `jalon review -next`: the oldest open issue labelled `measure`.
Nothing labelled is a success, not a failure, so an idle queue does not turn the
unit red every hour. Until `jalon triage` exists, apply that label by hand.

**The label is the queue, and a finished review removes it.** Without that the
same issue is measured again on the next tick, forever, spending the daily cap
on one question — which is exactly what happened here before the timer was even
armed: one issue, two near-identical task proposals. If the label cannot be
removed the work is still published, and the message says so and gives the
command, because the alternative is a silent loop.

A failed job leaves the queue too, and its wreck is parked out of `doctor`'s
way. The first version kept the label and the worktree in place so that a
failure would stop the timer; on a real timer that turned one failure into a
frozen night. Now one failure costs one item, the wreck is still there to read,
and the cap of ten wrecks is what stops a machine failing in a loop.

Two things the runbook will remind you of, because both are easy to miss:

- **The agent user needs its own model credentials.** Yours live in your home
  directory and a system user cannot read them. Run `claude setup-token` as
  that user, or give it an API key through an `EnvironmentFile`.
- **Reverting is two commands**, and the script prints them:
  `systemctl disable --now jalon-agent.timer && rm /etc/systemd/system/jalon-agent.*`
