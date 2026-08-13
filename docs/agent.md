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
| the decision to implement | `jalon work` only runs when a person triggers it |
| the merge | branch protection on `main`; the agent's token cannot merge |
| production | deploy needs sudo; the agent user has none |

## The stages

```
issue ──▶ triage ──▶ review ──▶ [a person says go] ──▶ work ──▶ [merge] ──▶ [deploy]
          (not yet)  (built)                           (not yet)
```

Issues are the queue between servers. They carry no state beyond labels, so
there is nothing to synchronize and nothing to reconcile.

**Built today: `jalon doctor` and `jalon review`.** `triage` and `work` are
designed but not written; see the task in `.tasks/`.

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

## `jalon review <issue-number>`

Runs on the server that hosts the target app, so the probes are local `curl`
calls against a running instance. That locality is the feature: it is what makes
the facts facts.

1. Load the config, run the `doctor` checks, refuse on any failure.
2. Read the issue: one `gh issue view`.
3. Create an ephemeral git worktree, detached from `default_branch`, and
   materialize the embedded skills into its `.claude/skills/`.
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
   thing: the jalon task, ordered measured / contradicted / cost / plan.
8. jalon does every mutation itself: commit, push the branch, open the pull
   request. The model is never handed a push.

The result is a pull request containing only the task file, with
`status: proposed`. That is not a new mechanism: it is the one already
documented in [`workflow.md`](./workflow.md) for agreeing on a task before
anyone writes code. Merging **is** the agreement.

On failure the worktree is kept, and the error names its path and the command
that removes it. The evidence of what went wrong is the only thing worth having
at that point, and deleting it to look tidy would throw away the bug report.

## The configuration

`.jalon/agent.toml`, versioned in each target repository. Every key is required
except `[notify]`. There are no defaults: a missing key is a refusal naming the
line to write, because a default that hides a misconfiguration is the failure
mode this whole file exists to avoid.

```toml
# .jalon/agent.toml

[agent]
model_triage           = "haiku"   # an alias, or a pinned id like "claude-haiku-4-5-20251001"
model_review           = "opus"
model_work             = "sonnet"
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
   fail.
2. **The token and branch protection.** A fine-grained PAT limited to the
   owner's repositories, with `issues:write` and `contents:write`. It cannot
   merge. The agent user has no sudo, and no `NOPASSWD` exception exists.
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
- **Model ids rot.** The template uses aliases (`opus`, `sonnet`, `haiku`)
  because a pinned id in a template is stale in six months. A pinned id works
  and is the better choice on a server you want reproducible.

## Deployment

One binary per server, one `scp`. No state outside git and the systemd units:
the counter file is one line holding an integer, and diagnosis is
`journalctl -u jalon-agent`, `cat` on that counter, and the last `facts.md`.

The systemd unit and timer are a later task; nothing here runs on a schedule
yet.
