---
status: proposed
created: 2026-08-15
links: [agent/gh.go, agent/review.go, agent/work.go, AGENTS.md]
issue: 26
---

# the GitHub coupling in review and work is a decision, not a gap

## Context

Issue 26 has only a title, no body: "make this measure, implement system
working without GitHub." Measuring the tree at `e663438` (`make check` green:
`go vet`, `go test -race`, dogfood on the repo's own 25 tasks) shows the core
verbs — `new`, `list`, `digest`, `append`, `compact`, `render`, `close` —
already run without GitHub: `github.go` confines every `gh` call there, and
`AGENTS.md` states `gh` is optional everywhere in the core, best-effort with a
stderr warning. Only `agent/`'s `review` and `work` verbs are GitHub-hard:
`agent/gh.go:27-122` shells out for the entire queue (`nextIssue`, `unlabel`)
and the entire publish step (`createPR`); `agent/review.go` and
`agent/work.go` call `fetchIssue`/`nextIssue` first and `createPR` last with
no fallback; and `doctor`'s `gh`/`gh-scopes` checks are Fail-blocking, so a
dead `gh auth status` alone stops both verbs even when claude, git, criterion
and skills all pass.

That coupling is not an oversight. `jalon digest 260806-github-bridge-without-a-sync`
records "no synchronization. Two writable stores would need a token, a mapping
table, a conflict rule and a schedule, and would break 'the files are the
truth'... 'all gh usage confined to `github.go`, so the coupling to a forge is
one deletable file.'" `jalon digest 260813-agent-orchestration-config-doctor-and-re`
goes further: "A GitHub issue becomes a measured task proposal," the
`measure`/`implement` labels are the queue, `closes #N` is the publish
mechanism, and the digest names this as a rule it deliberately crosses and
rescopes, not one it forgot. No task, decision or commit in the tracker
proposes a non-GitHub queue or publish mechanism for `review`/`work`; the
issue reads as an untested assumption against a design that was reasoned
through and recorded twice.

Removing the coupling would mean discarding the queue and the publish
mechanism those two decisions are built on — a redesign of `review`/`work`
from scratch, with a token, a mapping table and a conflict rule to replace
what a forge gives for free, which is exactly the cost
`260806-github-bridge-without-a-sync` measured and rejected. That cost is not
what a one-line issue title argues for, and no failure report or incident
motivates paying it now.

There is no deliverable here. The core already satisfies the issue's literal
request; the orchestration layer's GitHub dependency is a standing decision,
not a gap; and nothing measured shows a reason to revisit it. Close issue 26
on that basis.

## Decisions

- 2026-08-15 jalon-agent: the premise is refuted: do nothing beyond closing issue 26, because review/work's dependence on GitHub (queue via issue labels, publish via pull request) is a decision already recorded in 260806-github-bridge-without-a-sync and rescoped explicitly in 260813-agent-orchestration-config-doctor-and-re, and undoing it would mean discarding the queue and publish mechanism those decisions are built on, a far larger proposal than the issue's title implies
- 2026-08-15 jalon-agent: the core (new/list/digest/append/compact/render/close) already meets the issue's literal request: it works without GitHub today, per github.go confining all gh usage to one deletable file and AGENTS.md codifying gh as optional everywhere in the core; only the agent/ orchestration layer is GitHub-hard, and that hardness is scoped to review and work by design

## Log

- 2026-08-15 jalon-agent: measured: jalon list has no prior task for 'without GitHub' and git log has no commit addressing it; make check is green on e663438 (go vet, go test -race, dogfood on 25 tasks)
- 2026-08-15 jalon-agent: measured: reading agent/gh.go, agent/review.go, agent/work.go and agent/doctor.go shows review -next and work -next call nextIssue/fetchIssue before anything else, both verbs finish with createPR unconditionally, and doctor's gh/gh-scopes checks are Fail-blocking, so a failing gh auth status alone stops review and work entirely even though claude, git, criterion and skills would all pass
- 2026-08-15 jalon-agent: could not measure: whether gh is installed or authenticated on this machine (which/gh not on the allowed command list), and no live review or work run was performed, so the exact abort path on a gh failure is established by reading the code, not by observing a run
- 2026-08-15 jalon-agent: could not find: any prior task, decision or commit proposing a non-GitHub queue or publish mechanism for review/work; the issue's premise that this is a missing mode reads as untested, contradicted by 260806-github-bridge-without-a-sync's explicit 'no synchronization' decision and 260813-agent-orchestration-config-doctor-and-re's explicit rescoping of the same rule
