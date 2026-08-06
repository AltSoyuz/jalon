# Contributing

Thanks for your interest in `github.com/AltSoyuz/jalon`.

## Ground rules

1. **Zero dependency is a feature.** `go.mod` has no `require` block. A pull
   request that adds one needs a measured problem, not a hypothetical one, and
   has to say what owning the code instead would cost.
2. **The file format is a compatibility surface.** Ids, section names, front
   matter keys and the `[id]` commit convention do not change in place. Files
   written by an older version must keep parsing. It is specified in
   [docs/format.md](docs/format.md); read it before proposing a change.
3. **Small on purpose.** Every new verb, flag or section has to name the
   present problem it solves. "It would be nice to have" is a no. The README
   lists the ceilings that are deliberately not built and what would earn each
   of them.
4. **Tests are required.** New behavior comes with a test, bug fixes come with
   a regression test.

## Development

```sh
git clone git@github.com:AltSoyuz/jalon.git
cd jalon
make check     # gofmt, vet, go test -race, and jalon run against its own .tasks/
```

`make dogfood` runs the binary you just built against this repository's own
tasks. It is the fastest way to find out that a change broke real files, and CI
runs it on every pull request.

Tests never touch the network. Anything involving `gh` uses the `fakeGH` stub in
`github_test.go`, which puts a shell script first on `PATH`.

With [`staticcheck`](https://staticcheck.dev/) installed, `make staticcheck`
runs it; CI runs it as its own job either way. It refuses modules newer than the
toolchain it was built with, so reinstall it after a Go upgrade.

The required Go version lives in `go.mod` and nowhere else: every workflow reads
it from there with `go-version-file`.

The markdown subset has a golden file. After an intentional change:

```sh
go test -run TestMarkdownSubsetGolden -update ./...
```

and read the diff of `testdata/subset.html` before committing it.

## Pull requests

- One logical change per pull request.
- Commit messages in the imperative mood (`add foo`, not `added foo`), with the
  task id in brackets when there is one: `[260806] add the compact verb`.
- Update `CHANGELOG.md` under `## [Unreleased]` for any user visible change.
- CI must be green before review.

## Releases

Releases are cut by tagging `main`:

```sh
git tag -s vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

Move the `## [Unreleased]` entries into a new `## [X.Y.Z]` section as part of
the release commit.
