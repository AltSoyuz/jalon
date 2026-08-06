# Releasing

The build lives in the `Makefile`, not in CI. `make release` produces the exact
artifacts a tag publishes, on any laptop, which is what makes a release
reproducible and debuggable.

## Cutting a release

1. Move the `## [Unreleased]` entries in `CHANGELOG.md` into a new
   `## [X.Y.Z]` section, and commit.
2. Tag and push:

   ```sh
   git tag -s vX.Y.Z -m "vX.Y.Z"
   git push origin main vX.Y.Z
   ```

3. `.github/workflows/release.yml` runs `make release`, takes the release notes
   from the `## [X.Y.Z]` section of the changelog, and publishes the archives.
   **A missing changelog section fails the release** rather than publishing
   empty notes.

## What gets published

`make release VERSION=vX.Y.Z` writes into `dist/`, one archive and one checksum
file per platform:

```
jalon-<goos>-<goarch>-vX.Y.Z.tar.gz              (.zip on windows)
jalon-<goos>-<goarch>-vX.Y.Z.tar.gz_checksums.txt
```

Platforms are the `RELEASE_PLATFORMS` line of the `Makefile`: linux
amd64/arm64/arm/386, darwin amd64/arm64, windows amd64/arm64, freebsd amd64.
Adding one is a one word change. `CGO_ENABLED=0`, so every target cross compiles
from any host, and `-trimpath` keeps paths out of the binary.

Every pull request builds the whole matrix (the `crosscompile` job), so a
platform specific break is caught before the tag, not during it.

## Verifying a download

```sh
tar -xzf jalon-linux-amd64-vX.Y.Z.tar.gz
sha256sum -c jalon-linux-amd64-vX.Y.Z.tar.gz_checksums.txt
./jalon version
```

## Homebrew

**There is no tap yet, on purpose.** A tap is a second repository to operate and
a formula to resynchronize at every release. Until someone other than the author
installs jalon, it would be machinery serving nobody, and `go install` plus the
release archives already cover every platform.

The formula generator exists so that the day it is worth it costs five minutes:

```sh
make release VERSION=vX.Y.Z     # produces dist/ and the checksums
make brew-formula VERSION=vX.Y.Z # writes packaging/homebrew/jalon.rb
```

Then, once:

```sh
gh repo create AltSoyuz/homebrew-tap --public --description "Homebrew formulae"
git clone git@github.com:AltSoyuz/homebrew-tap.git
mkdir -p homebrew-tap/Formula
cp packaging/homebrew/jalon.rb homebrew-tap/Formula/
cd homebrew-tap && git add . && git commit -m "add jalon vX.Y.Z" && git push
```

and users get `brew install AltSoyuz/tap/jalon`. Repeat the two `make` commands
and the copy at each release; automating that cross repository push would need a
personal token stored as a secret, which is a failure mode (an expired token, a
silently skipped step) worth avoiding until the manual version becomes real
friction.

Homebrew core is a different question: it has a notability bar, and if jalon
ever clears it, Homebrew maintains the formula instead of us. That is the
outcome to prefer.

## Installing without a package manager

```sh
go install github.com/AltSoyuz/jalon@latest
```

or download the archive for your platform from the
[releases page](https://github.com/AltSoyuz/jalon/releases), check the sha256,
and drop the binary on your `PATH`. It is a single static file with no
dependency; uninstalling is `rm`.
