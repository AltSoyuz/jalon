package agent

import (
	"context"
	"strings"
	"testing"
)

// doctorTOML is validTOML with a criterion that always passes, so a test can
// change one thing at a time.
var doctorTOML = strings.Replace(validTOML, `command = "make check"`, `command = "true"`, 1)

func runDoctor(t *testing.T, root string) (Report, string, string) {
	t.Helper()
	var out, errb strings.Builder
	r := Doctor(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb})
	return r, out.String(), errb.String()
}

func names(r Report) []string {
	out := make([]string, len(r.Checks))
	for i, c := range r.Checks {
		out[i] = c.Name
	}
	return out
}

func state(t *testing.T, r Report, name string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %v", name, names(r))
	return Check{}
}

func TestDoctorAllGreen(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, out, _ := runDoctor(t, root)
	if r.Failed() != 0 {
		t.Fatalf("failed = %d, want 0:\n%s", r.Failed(), out)
	}
	// The order is documented, so it is asserted as an order.
	want := []string{"config", "claude", "claude-flags", "gh", "gh-scopes", "git", "criterion", "skills"}
	got := names(r)
	if len(got) != len(want) {
		t.Fatalf("checks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("checks = %v, want %v", got, want)
		}
	}
	// stdout is one aligned record per check, meant for awk.
	if n := strings.Count(strings.TrimSpace(out), "\n") + 1; n != len(want) {
		t.Errorf("stdout has %d lines, want %d:\n%s", n, len(want), out)
	}
}

// The point of doctor is that a fresh server is fixed in one pass, not one
// error per run. Every check has to run even after a failure.
func TestDoctorRunsEveryCheckAfterAFailure(t *testing.T) {
	s := newStubs(t)
	s.add(t, "gh", happyGH) // no claude at all
	root := newRepo(t, doctorTOML)

	r, out, errb := runDoctor(t, root)
	if got := state(t, r, "claude").State; got != Fail {
		t.Errorf("claude = %s, want fail", got)
	}
	if got := state(t, r, "claude-flags").State; got != Skip {
		t.Errorf("claude-flags = %s, want skip", got)
	}
	// A skipped check names what it was waiting for: silently skipped and
	// passing must not look alike.
	if fix := state(t, r, "claude-flags").Fix; !strings.Contains(fix, "claude") {
		t.Errorf("the skip does not name its prerequisite: %q", fix)
	}
	// The checks that do not depend on claude still ran.
	for _, name := range []string{"gh", "git", "criterion", "skills"} {
		if got := state(t, r, name).State; got == Skip {
			t.Errorf("%s was skipped, but it does not depend on claude:\n%s", name, out)
		}
	}
	if !strings.Contains(errb, "install the Claude Code CLI") {
		t.Errorf("stderr must carry the fix, got:\n%s", errb)
	}
	// A service does not inherit a shell's PATH, and that is the actual cause
	// on a server.
	if !strings.Contains(errb, "systemd") {
		t.Errorf("the fix should mention the systemd PATH, got:\n%s", errb)
	}
}

// A bad configuration must not spend a model call, and must not make the checks
// that do not need it look broken.
func TestDoctorWithoutConfig(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, "")

	r, _, errb := runDoctor(t, root)
	if got := state(t, r, "config").State; got != Fail {
		t.Errorf("config = %s, want fail", got)
	}
	for _, name := range []string{"git", "criterion"} {
		if got := state(t, r, name).State; got != Skip {
			t.Errorf("%s = %s, want skip when the config is unusable", name, got)
		}
	}
	if !strings.Contains(errb, "docs/agent.md") {
		t.Errorf("the fix must point at the template, got:\n%s", errb)
	}
}

// The flag check is what turns "these flags probably exist" into something the
// machine with the problem reports.
func TestDoctorNamesAMissingClaudeFlag(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", `case "$*" in
*--version*) echo "1.0.0" ;;
*--help*)    echo "--print --model --output-format --permission-mode --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
esac`)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, _, errb := runDoctor(t, root)
	c := state(t, r, "claude-flags")
	if c.State != Fail {
		t.Fatalf("claude-flags = %s, want fail", c.State)
	}
	if !strings.Contains(c.Detail, "--allowed-tools") || !strings.Contains(errb, "--allowed-tools") {
		t.Errorf("the missing flag must be named, got detail %q and stderr:\n%s", c.Detail, errb)
	}
}

func TestDoctorCriterionFailure(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, strings.Replace(validTOML, `command = "make check"`, `command = "exit 3"`, 1))

	r, _, errb := runDoctor(t, root)
	c := state(t, r, "criterion")
	if c.State != Fail {
		t.Fatalf("criterion = %s, want fail", c.State)
	}
	if !strings.Contains(c.Detail, "3") {
		t.Errorf("detail = %q, want the exit status", c.Detail)
	}
	if !strings.Contains(errb, "criterion.command") {
		t.Errorf("the fix must name the key to change, got:\n%s", errb)
	}
}

// A dirty tree is a warning, not a failure: a review branches its own detached
// worktree, so uncommitted work here does not affect it, and a doctor that is
// red for anyone mid task gets ignored.
func TestDoctorDirtyTreeIsAWarning(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)
	mustWrite(t, root+"/dirty.txt", "uncommitted")

	r, _, _ := runDoctor(t, root)
	if got := state(t, r, "git").State; got != Warn {
		t.Errorf("git = %s, want warn on a dirty tree", got)
	}
	if r.Failed() != 0 {
		t.Errorf("a dirty tree must not fail doctor, failed = %d", r.Failed())
	}
}

// A worktree left behind by a failed review blocks the next one, so it is worth
// catching before a job starts rather than halfway through.
func TestDoctorFindsAStaleWorktree(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)
	mustWrite(t, root+"/.jalon/worktrees/review-42/facts.md", "x")

	r, _, errb := runDoctor(t, root)
	c := state(t, r, "git")
	if c.State != Fail {
		t.Fatalf("git = %s, want fail on a stale worktree", c.State)
	}
	if !strings.Contains(errb, "git worktree remove --force") {
		t.Errorf("the fix must be the removal command, got:\n%s", errb)
	}
}

// A fine grained token sends no scope header. Calling that broken would train
// people to ignore this verb.
func TestDoctorWarnsOnAnUnknownScope(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", `case "$1 $2" in
"auth status") echo "Logged in" ;;
"api -i")      echo "Content-Type: application/json" ;;
esac`)
	root := newRepo(t, doctorTOML)

	r, _, _ := runDoctor(t, root)
	if got := state(t, r, "gh-scopes").State; got != Warn {
		t.Errorf("gh-scopes = %s, want warn when no scope header is sent", got)
	}
	if r.Failed() != 0 {
		t.Errorf("an unknown scope must not fail doctor, failed = %d", r.Failed())
	}
}

func TestDoctorFailsOnInsufficientScopes(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", `case "$1 $2" in
"auth status") echo "Logged in" ;;
"api -i")      echo "X-Oauth-Scopes: gist, read:org" ;;
esac`)
	root := newRepo(t, doctorTOML)

	r, _, errb := runDoctor(t, root)
	if got := state(t, r, "gh-scopes").State; got != Fail {
		t.Errorf("gh-scopes = %s, want fail without repo", got)
	}
	if !strings.Contains(errb, "gh auth refresh") {
		t.Errorf("the fix must be the refresh command, got:\n%s", errb)
	}
}

// doctor spends no tokens: it checks that the CLI exists and what flags it
// takes, never that it answers.
func TestDoctorMakesNoModelCall(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	runDoctor(t, root)
	for _, c := range s.of(t, "claude") {
		if strings.Contains(c, "--print") {
			t.Errorf("doctor made a model call:\n%s", c)
		}
	}
}
