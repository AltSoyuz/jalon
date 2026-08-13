package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reviewTOML allows the probe the stubbed gathering phase reports running, and
// a criterion that passes, so each test can change one thing.
var reviewTOML = strings.Replace(doctorTOML,
	"  \"jalon digest\",\n  \"make check\",\n",
	"  \"jalon digest\",\n  \"curl -s http://localhost:8080/healthz\",\n", 1)

func runReview(t *testing.T, root string) (ReviewResult, string, string, error) {
	t.Helper()
	var out, errb strings.Builder
	res, err := Review(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{Issue: 42, TasksDir: filepath.Join(root, ".tasks")})
	return res, out.String(), errb.String(), err
}

// claudeWith swaps the facts the gathering phase prints, keeping every other
// phase working, so a test can aim at the gate alone.
func claudeWith(facts string) string {
	return `case "$*" in
*--version*) echo "9.9.9" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*)
  cat <<'FACTS'
` + facts + `
FACTS
  ;;
*jalon-review-skeptic*) echo "The premise stands." ;;
*jalon-review-task*)
  mkdir -p .tasks
  echo "---" > .tasks/260813-t.md
  echo "status: proposed" >> .tasks/260813-t.md
  echo "---" >> .tasks/260813-t.md
  echo "" >> .tasks/260813-t.md
  echo "# T" >> .tasks/260813-t.md ;;
esac`
}

func TestReviewHappyPath(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)

	res, out, _, err := runReview(t, root)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if res.TaskID != "260813-health-endpoint-is-not-slow" {
		t.Errorf("task id = %q", res.TaskID)
	}
	if res.PR != "https://example.invalid/pull/7" {
		t.Errorf("pr = %q", res.PR)
	}
	if !strings.Contains(out, res.TaskID) {
		t.Errorf("stdout must carry the id, got:\n%s", out)
	}
	// A successful review leaves nothing behind.
	if res.Worktree != "" {
		t.Errorf("worktree = %q, want it removed on success", res.Worktree)
	}
	if _, err := os.Stat(filepath.Join(root, ".jalon", "worktrees", "review-42")); !os.IsNotExist(err) {
		t.Error("the worktree directory survived a successful review")
	}
	// Three phases, three separate processes.
	if n := len(s.models(t)); n != 3 {
		t.Errorf("the model was called %d times, want 3 phases", n)
	}
	// The task file is on the branch that was pushed, and nothing else is.
	if out := gitOut(t, root, "show", "--stat", "--format=", "origin/task/"+res.TaskID); !strings.Contains(out, ".tasks/") {
		t.Errorf("the pushed commit does not carry the task file:\n%s", out)
	} else if strings.Contains(out, ".jalon-review") {
		t.Errorf("the pushed commit carries the working files:\n%s", out)
	}
}

// The tool policy is the security surface, so it is asserted on the argv that
// was actually passed, not on the function that builds it.
func TestReviewToolPolicy(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)

	if _, _, _, err := runReview(t, root); err != nil {
		t.Fatal(err)
	}
	calls := s.models(t)
	if len(calls) != 3 {
		t.Fatalf("want 3 model calls, got %d", len(calls))
	}
	facts, skeptic, task := calls[0], calls[1], calls[2]

	// Nothing may ever bypass permissions. This is the regression test for the
	// one mistake that would quietly remove the whole policy.
	for i, c := range calls {
		for _, forbidden := range []string{"--dangerously-skip-permissions", "bypassPermissions", "--allow-dangerously-skip-permissions"} {
			if strings.Contains(c, forbidden) {
				t.Errorf("call %d carries %s:\n%s", i, forbidden, c)
			}
		}
		if !strings.Contains(c, "--permission-mode dontAsk") {
			t.Errorf("call %d does not run in dontAsk:\n%s", i, c)
		}
		if !strings.Contains(c, "--max-budget-usd 3") {
			t.Errorf("call %d carries no budget:\n%s", i, c)
		}
		if !strings.Contains(c, "--disallowed-tools WebFetch,WebSearch,Task") {
			t.Errorf("call %d does not refuse the network and subagents:\n%s", i, c)
		}
	}

	// The gathering phase gets the probes and no write tool at all.
	if !strings.Contains(facts, "Bash(jalon digest:*),Bash(curl -s http://localhost:8080/healthz:*),Read,Grep,Glob") {
		t.Errorf("the facts phase policy is not derived from the allowlist:\n%s", facts)
	}
	if strings.Contains(facts, "--tools Read,Grep,Glob,Bash\n") && strings.Contains(facts, "Write") {
		t.Errorf("the facts phase must have no write tool:\n%s", facts)
	}
	if strings.Contains(skeptic, "Write") {
		t.Errorf("the skeptic must have no write tool:\n%s", skeptic)
	}
	// The writing phase may write the task and run jalon, and nothing else.
	for _, want := range []string{"Bash(jalon new:*)", "Write(.tasks/**)"} {
		if !strings.Contains(task, want) {
			t.Errorf("the task phase is missing %s:\n%s", want, task)
		}
	}
	// It is never handed git, gh or a push.
	for _, forbidden := range []string{"Bash(git", "Bash(gh"} {
		if strings.Contains(task, forbidden) {
			t.Errorf("the task phase was handed %s:\n%s", forbidden, task)
		}
	}
}

// The gate is the reason this design is worth anything: no facts, no task.
func TestReviewGateRefusesNarration(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith(strings.Repeat("This looks slow to me and I believe it should be improved. ", 8)))
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("narration was accepted")
	}
	if !strings.Contains(err.Error(), "no executed command block") {
		t.Errorf("err = %v, want it to name the missing command block", err)
	}
	assertStopped(t, s, root, res, 1)
}

func TestReviewGateRefusesAThinDocument(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith("nothing to report"))
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "stopped too early") {
		t.Fatalf("err = %v, want a refusal naming the short document", err)
	}
	assertStopped(t, s, root, res, 1)
}

// A command block naming something outside the allowlist means the facts are
// not what they claim, whether the phase ran it or invented it.
func TestReviewGateRefusesAnUnlistedCommand(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith("# Facts\n\n"+fence+"console\n$ psql -c 'drop table users'\ndone\n"+fence+
		"\n\nPadding so that this document is comfortably past the two hundred byte minimum the gate applies, "+
		"because the point of this test is the allowlist and not the length.\n"))
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("an unlisted command was accepted")
	}
	if !strings.Contains(err.Error(), "psql") || !strings.Contains(err.Error(), "probes.allowed") {
		t.Errorf("err = %v, want it to name the command and the key", err)
	}
	assertStopped(t, s, root, res, 1)
}

// assertStopped checks the three things every failed review must be true of: it
// wrote no task, it kept its evidence, and it did not run the phases after the
// one that failed.
func assertStopped(t *testing.T, s *stubs, root string, res ReviewResult, wantCalls int) {
	t.Helper()
	if res.Worktree == "" {
		t.Error("a failed review must keep its worktree for inspection")
	}
	facts := filepath.Join(root, res.Worktree, reviewDir, "facts.md")
	if _, err := os.Stat(facts); err != nil {
		t.Errorf("the evidence is missing: %v", err)
	}
	if n := len(s.models(t)); n != wantCalls {
		t.Errorf("the model was called %d times, want %d: the run did not stop at the gate", n, wantCalls)
	}
	if out := gitOut(t, root, "branch", "--list", "task/*"); strings.TrimSpace(out) != "" {
		t.Errorf("a failed review left a branch behind: %q", out)
	}
}

// An existing worktree is either a concurrent review or the wreck of a failed
// one. Guessing which is how evidence gets destroyed.
func TestReviewRefusesAnExistingWorktree(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, reviewTOML)
	mustWrite(t, filepath.Join(root, ".jalon", "worktrees", "review-42", "facts.md"), "earlier evidence")

	_, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("an existing worktree was reused")
	}
	// doctor catches it first, which is the point of running it before a job.
	if !strings.Contains(err.Error(), "preflight") {
		t.Errorf("err = %v, want the preflight to catch it", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made despite the collision")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".jalon", "worktrees", "review-42", "facts.md")); string(b) != "earlier evidence" {
		t.Error("the earlier evidence was destroyed")
	}
}

func TestReviewRefusesAClosedIssue(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", `case "$1 $2" in
"auth status") echo "Logged in" ;;
"api -i")      echo "X-Oauth-Scopes: repo" ;;
"issue view")  printf '{"number":42,"title":"x","body":"y","url":"u","state":"CLOSED"}' ;;
esac`)
	root := newRepo(t, reviewTOML)

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("err = %v, want a refusal naming the closed issue", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made on a closed issue")
	}
}

// The cap is a plain text counter you can read with cat, and it has to hold
// before anything is spent.
func TestReviewDailyCap(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, strings.Replace(reviewTOML, "daily_job_cap = 10", "daily_job_cap = 1", 1))

	if _, _, _, err := runReview(t, root); err != nil {
		t.Fatalf("the first review failed: %v", err)
	}
	counter := filepath.Join(root, ".jalon", "agent-jobs-today")
	b, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(b)), " 1") {
		t.Errorf("counter = %q, want it to end in the count", b)
	}

	_, _, _, err = runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "daily cap") {
		t.Fatalf("err = %v, want the cap to refuse the second job", err)
	}
	if !strings.Contains(err.Error(), "daily_job_cap") {
		t.Errorf("err = %v, want it to name the key to raise", err)
	}
}

// A review with no configuration must refuse before it touches anything.
func TestReviewRefusesWithoutConfig(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	root := newRepo(t, "")

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("err = %v, want a preflight refusal", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made on a red base")
	}
}

func TestNotifyRunsOneCommand(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	// Shell builtins only: PATH is restricted in these tests, and the point here
	// is that the message reaches the command's stdin, not that coreutils exist.
	root := newRepo(t, strings.Replace(reviewTOML,
		"[probes]", "[notify]\ncommand = 'while IFS= read -r l; do echo \"$l\" >> notified.txt; done'\n\n[probes]", 1))

	if _, _, _, err := runReview(t, root); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "notified.txt"))
	if err != nil {
		t.Fatalf("the notification command did not run: %v", err)
	}
	if !strings.Contains(string(b), "260813-health-endpoint-is-not-slow") {
		t.Errorf("the notification does not name the task: %q", b)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}
