package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The same technique as fakeGH in github_test.go, tightened because this
// package shells out to more than one program: PATH becomes the stub directory
// and nothing else, with git and sh symlinked in because the tests exercise the
// real worktree lifecycle.
//
// Making PATH exclusive is what lets a test express "this machine has no
// claude", which is the situation doctor exists for and the one a prepended
// directory cannot produce.
//
// Every stub records its argv and its stdin, which is what lets a test assert
// on the tool policy that was actually passed rather than on the code that
// builds it.

type stubs struct {
	dir   string
	calls string
}

func newStubs(t *testing.T) *stubs {
	t.Helper()
	s := &stubs{dir: t.TempDir(), calls: t.TempDir()}
	for _, real := range []string{"git", "sh"} {
		p, err := exec.LookPath(real)
		if err != nil {
			t.Skipf("%s is required for this test", real)
		}
		if err := os.Symlink(p, filepath.Join(s.dir, real)); err != nil {
			t.Fatal(err)
		}
	}
	// The stubs themselves are shell scripts and need the ordinary tools, so
	// they restore a real PATH for their own body. Only the lookups jalon does
	// are restricted, which is exactly what is being tested.
	t.Setenv("JALON_TEST_REALPATH", os.Getenv("PATH"))
	t.Setenv("PATH", s.dir)
	t.Setenv("JALON_TEST_CALLS", s.calls)
	return s
}

const record = `PATH="$JALON_TEST_REALPATH"
n=$(ls "$JALON_TEST_CALLS" | wc -l | tr -d ' ')
{ echo "$0 $@"; echo "--- stdin"; cat; } > "$JALON_TEST_CALLS/$(printf %03d "$n").call"
exec 0</dev/null
`

func (s *stubs) add(t *testing.T, name, script string) {
	t.Helper()
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+record+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// calls returns every recorded invocation, in order.
func (s *stubs) recorded(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(s.calls, "*.call"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	return out
}

// models returns the calls that actually spent tokens. doctor runs
// "claude --version" and "claude --help", which are not model calls, so a test
// counting phases has to filter for --print.
func (s *stubs) models(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, c := range s.of(t, "claude") {
		if strings.Contains(c, "--print") {
			out = append(out, c)
		}
	}
	return out
}

// of returns the recorded calls to one program.
func (s *stubs) of(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	for _, c := range s.recorded(t) {
		if strings.HasPrefix(c, filepath.Join(s.dir, name)+" ") {
			out = append(out, c)
		}
	}
	return out
}

// fence is the markdown code fence. A Go raw string cannot contain one, and the
// stubs have to emit it because it is the shape the gate looks for.
const fence = "```"

// factsBlock is what a gathering phase that did its job prints: a command block
// in the documented shape, padded past the gate's minimum length.
const factsBlock = "# Facts\n\nThe issue claims the health endpoint is slow.\n\n" +
	fence + "console\n$ curl -s http://localhost:8080/healthz\n{\"status\":\"ok\"}\n" + fence + "\n\n" +
	"That is the measurement, and this line pads the document past the gate minimum of two hundred bytes.\n"

// happyClaude answers every phase the way a working run would: it prints the
// facts, prints the skeptic's answer, and creates one task file.
var happyClaude = `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*)
  cat <<'FACTS'
` + factsBlock + `FACTS
  ;;
*jalon-review-skeptic*)
  echo "The premise does not hold. The endpoint answers in 4 ms." ;;
*jalon-review-task*)
  mkdir -p .tasks
  cat > .tasks/260813-health-endpoint-is-not-slow.md <<'TASK'
---
status: proposed
created: 2026-08-13
links: []
---

# Health endpoint is not slow

## Context

Measured.

## Decisions

## Log
TASK
  ;;
esac`

// happyGH answers the two forge calls a review makes.
const happyGH = `case "$1 $2" in
"auth status") echo "Logged in to github.com" ;;
"api -i")      echo "X-Oauth-Scopes: repo, read:org" ;;
"issue view")  printf '{"number":42,"title":"health is slow","body":"it feels slow","url":"https://example.invalid/42","state":"OPEN"}' ;;
"pr create")   echo "https://example.invalid/pull/7" ;;
"issue edit")  echo "edited" ;;
*) echo "unexpected gh: $*" >&2; exit 1 ;;
esac`

// newRepo builds a repository with an origin to push to, a .tasks directory and
// the configuration, which is the state every agent job starts from. Git is
// never stubbed: the worktree lifecycle is most of what these tests are for.
func newRepo(t *testing.T, cfgTOML string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for this test")
	}
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	root := filepath.Join(base, "repo")

	mustGit(t, base, "init", "-q", "--bare", "-b", "main", origin)
	mustGit(t, base, "init", "-q", "-b", "main", root)
	mustGit(t, root, "config", "user.email", "t@example.invalid")
	mustGit(t, root, "config", "user.name", "t")
	mustGit(t, root, "remote", "add", "origin", origin)

	mustWrite(t, filepath.Join(root, ".tasks", ".gitkeep"), "")
	if cfgTOML != "" {
		mustWrite(t, filepath.Join(root, configDir, configName), cfgTOML)
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "init")
	mustGit(t, root, "push", "-q", "-u", "origin", "main")
	return root
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
