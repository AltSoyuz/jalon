---
status: doing
created: 2026-08-13
links: [agent/review.go, agent/doctor.go, docs/agent.md]
---

# agent orchestration: config, doctor and review

## Context

jalon holds no model. Two things therefore never happen on their own: an idea
is never confronted with measured reality before it is designed, and the
backlog is never worked down. On one real day, four of eight designs were
killed by measurement only after they had been written out in full.

This adds an orchestration layer that automates the confrontation step. A
GitHub issue becomes a measured task proposal, produced on the server hosting
the target app so the probes are local curl calls against a running instance.
The human keeps three gates: the decision to implement, the merge, and
production. The merge gate is structural (branch protection); the production
gate is structural (deploy needs sudo, the agent user has none).

The full design is four stages: capture, `triage`, `review`, `work`. **Only
config loading, `doctor` and `review` land in this task.** `triage`, `work`
and the systemd units are their own tasks.

The layer lives in `agent/`, a separate package, so the core stays usable with
no agent, no network and no key. That boundary is the whole design; if a core
verb ever depends on `agent/`, this is broken.

This crosses three rules currently written down, and each is rescoped rather
than quietly contradicted: "this tool holds no model and calls none" and "no
synchronization with a forge" in AGENTS.md, and "it never writes outside the
tasks directory and the render output directory" in docs/format.md.

## Decisions

- 2026-08-13 altsoyuz: the agent layer is a separate package agent/, not files in package main: the boundary that core never imports it is then enforced by the compiler and by TestCoreDoesNotImportAgent, not by discipline. It costs the single package main line in AGENTS.md.
- 2026-08-13 altsoyuz: config is .jalon/agent.toml parsed by a hand written TOML subset. Zero dependency is absolute and stdlib has no TOML. Unknown and missing keys are both refusals naming the line and the fix, unlike front matter which preserves unknown keys: a config typo is a misconfiguration, a hand written front matter key is not.
- 2026-08-13 altsoyuz: review opens a pull request carrying only the task file, with status proposed; it never pushes to main. Branch protection makes pushing to main impossible by construction, and docs/workflow.md already documents the task-proposal-as-PR primitive, so no second mechanism is invented.
- 2026-08-13 altsoyuz: phase 1 of review gets no write tool at all: it runs read only and jalon writes facts.md from the captured stdout. The gate then checks content jalon itself produced, and no path scoped write permission has to be reasoned about.
- 2026-08-13 altsoyuz: jalon performs every git and gh mutation itself; the model writes the task file and nothing else. review therefore never grants a push, commit or PR creation capability, which removes that blast radius instead of policing it.
- 2026-08-13 altsoyuz: the gate stops the job on narration only, not on which commands the facts report. Three live runs stopped on how a command was written while the measurement was sound every time, and a facts document is evidence rather than something jalon executes: the bound on the job is the tool policy, the throwaway worktree, and the absence of any push capability. An unlisted or composed command is now reported on stderr and in the pull request body for a human to check.
- 2026-08-13 altsoyuz: jalon emits the setup rather than performing it: agent-init prints the config, the unit and the timer, with the privileged half behind an explicit --root flag. A jalon setup would need root in the one binary whose blast radius argument is that the agent user has no sudo, would put Debian specifics into a tool with no OS specific line and nine target platforms, and would run fewer than ten times ever. The units are embedded because the deployment unit is the scp'd binary, not the repository.

## Log

- 2026-08-13 altsoyuz: claude CLI flags verified against the installed binary before any code: --max-budget-usd, --allowed-tools, --disallowed-tools, --permission-mode, --tools, --append-system-prompt, --output-format and --model all exist. --max-turns does NOT exist, so bounding a phase is --max-budget-usd only. --permission-mode has no default value; the choices are acceptEdits, auto, bypassPermissions, manual, dontAsk, plan.
- 2026-08-13 altsoyuz: TOML subset parser and config loading land, with docs/agent.md written. The template in the doc is loaded by TestDocsTemplateLoads, so a renamed key fails the suite rather than rotting the doc. Two findings while testing: strconv accepts 1_000 and 0x3e8 as numbers (refused explicitly now), and a fix instruction rendered with %q is not pasteable (rendered with %s now).
- 2026-08-13 altsoyuz: doctor and review land, with the boundary test. Verified against the real machine: doctor reports the missing config, still runs every independent check, skips the config dependent ones by name, and validated 9 flags against the installed claude 2.1.229. make check, staticcheck and the linux amd64/arm64 cross builds are green.
- 2026-08-13 altsoyuz: live test against the real claude 2.1.229 on a throwaway clone with a local bare origin and a fake gh. The method works: the gathering phase started from jalon list and digest, measured digest at 11-34ms against the issue's claim of several seconds, refused the probes that were denied instead of working around them, and wrote an honest could-not-measure section.
- 2026-08-13 altsoyuz: the live run found two defects the stubs could not. The facts skill told the model to render every observation as a shell block, so an allowed Grep tool call came back as "$ grep -rn ..." and the gate rejected a legitimate measurement; the skill now reserves $ blocks for real shell commands and asks for prose with file:line otherwise. And "ls -la .tasks/*.md | wc -l" passed both the tool policy and the gate on its prefix, which is the shell composition hole, so the gate now refuses a reported command containing a composition metacharacter.
- 2026-08-13 altsoyuz: second live run failed the gate on "$ which gh", a command that is not a probe, reported in a block with plausible output. Whether it ran or was inferred from PATH cannot be told from the document, which is the gate's documented limit and the argument for having jalon run the probes itself. Root cause fixed instead: the phases now receive the allowlist on stdin rather than discovering it by denial, and the skill forbids putting a refused command in a block.
- 2026-08-13 altsoyuz: fourth live run went end to end: facts 6255 bytes, skeptic 1251, task written, branch pushed, PR opened, worktree removed, no suspect command. The task refutes both halves of the issue, including that LoadTasks is never in the digest call path, which is verifiable and correct, and proposes doing nothing. Cost about four runs of three sonnet calls each under a 2 USD per job cap.
- 2026-08-13 altsoyuz: models pinned to claude-sonnet-5 and claude-haiku-4-5 instead of the sonnet and haiku aliases, which retarget silently when a new model ships. review -next added so the emitted unit invokes a verb that exists: it takes the oldest open issue labelled measure, and an empty queue succeeds quietly rather than turning the timer red every hour.
