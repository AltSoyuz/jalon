---
status: done
created: 2026-08-13
links: [agent/review.go, agent/doctor.go, agent/setup/jalon-agent.service.tmpl, .github/workflows/ci.yml]
---

# Skip criterion in doctor when review is about to run it

## Context

`Doctor()` runs `criterionCheck` (`agent/doctor.go:230-244`, `agent/doctor.go:405-411`)
whenever the config loads, with no way to opt out. `Review()` calls `Doctor()`
unconditionally as the first thing it does (`agent/review.go:98`), ahead of the
`opt.Next` empty-queue check and ahead of `takeJobSlot`'s guard. The systemd
unit that fires this hourly (`agent/setup/jalon-agent.timer.tmpl:6`,
`OnCalendar=hourly`) runs `jalon review -next`, and its own template comment
states the ordering is deliberate: "review runs doctor first and refuses on a
red base, so a failure here is a real failure and not a half-finished job"
(`agent/setup/jalon-agent.service.tmpl:26-28`). `make check` measured twice on
this checkout: `go test -race` reports ~1.4s for the root package and ~2.8-2.9s
for `agent`, on top of `fmt`, `vet`, `build`, and `dogfood`, none of which
print their own timing. Total wall-clock of one `make check` invocation and
the real production tick frequency/cost could not be measured — no `time` or
`date` was on the allowed command list, and there is no deployed timer or
historical log in this checkout to sample from.

The skeptic tried to refute the premise by reading `git log -p` on
`agent/review.go`, the service template, and the timer template, and found
nothing that weakens it: the unconditional-before-empty-queue-check ordering
is confirmed by the code, the hourly cadence is confirmed by the template, and
no `.jalon/agent.toml` is checked into the repo to suggest a deployed
criterion cheaper than the `make check` shown in `docs/agent.md` and the
setup template. `.github/workflows/ci.yml` already runs the equivalent work
(`fmt`, `vet`, `test`, `dogfood`) as separate jobs on every pull request
(`.github/workflows/ci.yml:3-6, 39-44, 60-61`), so the same cost is paid twice
per change: once in CI, once on the next tick after merge, and then again on
every tick with no work to do until the next change lands.

The cost of the fix is small and localized: `Doctor()` needs a way to skip
`criterionCheck` (a parameter or option struct), and `Review()` needs to pass
it when it is about to run the criterion itself as part of the real job — or,
per the issue's alternative, the criterion in `docs/agent.md` and
`agent/setup/agent.toml.tmpl` could be swapped for something cheaper like
`go build ./...`, at the cost of doctor no longer catching what `make check`
catches (vet, race-detected tests, dogfood) between ticks. Given the
unconditional-before-empty-queue-check finding, the cheaper fix is likely to
have `Doctor()` accept a flag to skip the criterion, and have `Review()` set
it: the criterion should still run once, as part of the real job when there
is work, not twice per tick and not skipped from doctor's other checks (git,
config) which stay cheap regardless.

## Decisions

- 2026-08-13 jalon-agent: Premise confirmed, not refuted: Doctor() runs criterionCheck unconditionally at the top of Review(), before opt.Next's empty-queue check and before takeJobSlot's guard, so every hourly tick pays the full criterion cost even when there is no issue to review or the job slot is locked -- git log -p on agent/review.go and the generated jalon-agent.service.tmpl confirm this is deliberate, acknowledged ordering, not an oversight

## Log

- 2026-08-13 jalon-agent: Measured make check twice on this checkout: go test -race reports ~1.4s for the root package and ~2.8-2.9s for agent, on top of unmeasured fmt/vet/build/dogfood overhead -- total wall-clock and real production tick frequency/cost could not be measured, no time/date on the allowed command list and no live deployed timer to sample
- 2026-08-15 altsoyuz: duplicate of 260813-skip-criterion-in-doctor-when-review-run, which shipped: two reviews measured the same issue because review did not remove the measure label yet, so the queue handed it out twice
- 2026-08-15 altsoyuz: closed.
