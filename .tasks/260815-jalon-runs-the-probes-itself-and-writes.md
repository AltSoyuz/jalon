---
status: done
created: 2026-08-15
links: []
---

# jalon runs the probes itself and writes facts.md from its own output

## Context

## Decisions

- 2026-08-15 altsoyuz: the facts phase loses Bash: it reads with Read, Grep and Glob and prints the probe commands to run, one per line, no shell characters; jalon executes those matching probes.allowed with no shell, one process per line, and writes facts.md itself from command, exit status and captured output
- 2026-08-15 altsoyuz: the gate becomes 'at least one probe ran'; the console-block regexp, suspectCommands and the $-block convention in the skills go away, and the known limit 'the gate does not prove execution' leaves docs/agent.md because it is no longer true
- 2026-08-15 altsoyuz: the skeptic and task phases are unchanged and receive facts that provably ran; the review still spends three model calls, the facts one now without a tool loop, which should cost less and take less time; measure both before and after on one real issue
- 2026-08-15 altsoyuz: a probe naming a program the machine lacks is refused and named rather than counted as run: nothing started, so it is not a measurement, and the message says which program so the fix is one install and not a guess

## Log

- 2026-08-15 altsoyuz: measured problem: docs/agent.md records a live run whose facts.md held a plausible $ which gh block for a command that did not run; the survey of other agents found nobody enforcing measurement in code, so this step is what makes the differentiating claim provable. Fourth in the order because it touches the skills and the gate together.
- 2026-08-15 altsoyuz: implemented: the facts phase runs with Read, Grep, Glob and no Bash and prints commands; runProbes executes the allowed, uncomposed, installed ones with exec (no shell), writes facts.md with command, output and exit status, and lists refusals; the gate is ran > 0; commandBlock, gate() and suspectCommands are gone; refusals reach stderr and the PR body. Tests: TestReviewGateRefusesNarration, TestReviewRefusesProbesWithoutStopping, TestTheFactsAreWhatTheProbesPrinted.
- 2026-08-15 altsoyuz: closed.
