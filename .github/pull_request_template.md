<!-- Remove the checklist before submitting if it does not apply. -->

## What this changes

<!-- One or two sentences. If it fixes an issue, write "closes #NN" here so
     GitHub closes it on merge. If it advances a task in .tasks/, write
     "closes <task id>" too: the post-merge hook closes that one. -->

## Checklist

- [ ] `make check` passes (gofmt, vet, `go test -race`, dogfood).
- [ ] New behavior has a test; a bug fix has a regression test.
- [ ] `go.mod` still has no `require` block. If this adds a dependency, the
      description says which measured problem it solves and what owning the
      code instead would cost.
- [ ] The file format is unchanged, or the change is additive and old files
      still parse. See [docs/format.md](../docs/format.md).
- [ ] `CHANGELOG.md` has an entry under `## [Unreleased]` for any user visible
      change.
- [ ] Commits carry the task id when there is one: `[260806] fix the ...`.
