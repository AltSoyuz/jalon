# Security Policy

## Supported versions

The latest release. jalon is a single static binary with no dependency, so
upgrading is one `go install` or one `brew upgrade` away, and there is no
long term support branch to maintain.

## Reporting a vulnerability

Use [private vulnerability reporting](https://github.com/AltSoyuz/jalon/security/advisories/new)
rather than a public issue. Expect a first answer within a week.

## What the threat model actually is

jalon reads and writes markdown files in a repository you already control, and
shells out to `git` and, optionally, to `gh`. It listens on nothing, opens no
port, stores no credential and makes no network call of its own.

The parts worth reporting a flaw in:

- **The HTML view.** `jalon render` turns task files into HTML. Everything is
  escaped first and the supported markdown subset is deliberately small;
  anything outside it is emitted as literal text. A task file that produces
  executable HTML, or a link that produces a `javascript:` href, is a bug worth
  reporting.
- **Command construction.** Task ids and front matter values reach `git` and
  `gh` as arguments. They are passed as separate argv entries, never through a
  shell, and ids containing glob metacharacters are rejected. A value that
  escapes that is a bug worth reporting.
- **File writes.** jalon only writes inside the tasks directory and the render
  output directory. A crafted id or front matter value that makes it write
  elsewhere is a bug worth reporting.

Things that are not vulnerabilities: `gh` being absent or unauthenticated, a
task file being readable by anyone who can read the repository, and the
rendered site exposing the task content it was asked to render.
