package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// doctorTOML is validTOML with a criterion that always passes, so a test can
// change one thing at a time.
var doctorTOML = strings.Replace(validTOML, `command = "make check"`, `command = "true"`, 1)

func runDoctor(t *testing.T, root string, opts ...Options) (Report, string, string) {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	t.Helper()
	var out, errb strings.Builder
	r := Doctor(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb}, opt)
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
	want := []string{"config", "claude", "claude-flags", "gh", "gh-scopes", "git-identity", "git", "criterion", "skills", "model"}
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

// A fresh system user has no git identity, and a review only discovers it after
// spending three model calls, at the commit. Found on a real machine exactly
// that way, so the check exists to make it free.
func TestDoctorFailsWithoutAGitIdentity(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)
	// newRepo sets a local identity; unset it to be the fresh-user case.
	mustGit(t, root, "config", "--unset", "user.email")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	r, _, errb := runDoctor(t, root)
	c := state(t, r, "git-identity")
	if c.State != Fail {
		t.Fatalf("git-identity = %s, want fail with no identity set", c.State)
	}
	if !strings.Contains(errb, "git config --global user.name") {
		t.Errorf("the fix must be the command to run, got:\n%s", errb)
	}
	if !strings.Contains(errb, "fail at the commit") {
		t.Errorf("the fix must say what would break, got:\n%s", errb)
	}
}

func TestDoctorReportsTheGitIdentity(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, _, _ := runDoctor(t, root)
	c := state(t, r, "git-identity")
	if c.State != Ok {
		t.Fatalf("git-identity = %s, want ok", c.State)
	}
	// Whose identity it is matters: an agent committing as you is a surprise.
	if !strings.Contains(c.Detail, "t@example.invalid") {
		t.Errorf("detail = %q, want it to name the identity", c.Detail)
	}
}

// The model check is the only one that proves the job can do its actual work,
// and the only one that costs money. Two real failures on a server (an absent
// login, then an unset git identity) passed every other check and died after
// three model calls had been paid for; this is the remaining half of that gap.
func TestDoctorLiveProvesTheModelAnswers(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude+`
case "$*" in *"reply with the single word ok"*) echo "ok" ;; esac`)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, _, _ := runDoctor(t, root, Options{Live: true})
	c := state(t, r, "model")
	if c.State != Ok {
		t.Fatalf("model = %s, want ok", c.State)
	}
	if !strings.Contains(c.Detail, "ok") {
		t.Errorf("detail = %q, want it to carry what the model answered", c.Detail)
	}
	// A health check that can enter a tool loop can bill like a job.
	for _, call := range s.models(t) {
		if !strings.Contains(call, "--tools ") {
			t.Errorf("the live check runs with tools enabled:\n%s", call)
		}
	}
}

// Off by default, and visibly so: a check you cannot see is one you assume
// passed.
func TestDoctorSkipsTheModelCheckByDefault(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, _, errb := runDoctor(t, root)
	c := state(t, r, "model")
	if c.State != Skip {
		t.Errorf("model = %s, want skip without -live", c.State)
	}
	if !strings.Contains(errb, "-live") {
		t.Errorf("the skip must name the flag that runs it, got:\n%s", errb)
	}
	if len(s.models(t)) > 0 {
		t.Error("doctor spent a model call without -live")
	}
}

// An expired or absent login is the failure this check exists for, so its fix
// has to be the command that repairs it.
func TestDoctorLiveNamesAMissingLogin(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude+`
case "$*" in *"reply with the single word ok"*) echo "Not logged in - Please run /login" >&2; exit 1 ;; esac`)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	r, _, errb := runDoctor(t, root, Options{Live: true})
	if state(t, r, "model").State != Fail {
		t.Fatal("an unauthenticated model was accepted")
	}
	if !strings.Contains(errb, "claude setup-token") {
		t.Errorf("the fix must be the command that repairs it, got:\n%s", errb)
	}
}

// A review must never spend the live call: it is about to do real work, and a
// health check inside a job is a second bill.
func TestReviewNeverSpendsTheLiveCheck(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, doctorTOML)

	var out, errb strings.Builder
	Doctor(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb}, Options{})
	if strings.Contains(out.String(), "ok    model") {
		t.Error("the default preflight ran the live check")
	}
}
