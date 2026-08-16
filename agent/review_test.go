package agent

import (
	"context"
	"fmt"
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

const reviewTaskID = "260813-health-endpoint-is-slow"

// newReviewRepo is newRepo plus one queued task, which is the state jalon
// review starts from: a stub a person wrote, status measure, on origin.
func newReviewRepo(t *testing.T, cfgTOML string, tasks ...string) string {
	t.Helper()
	root := newRepo(t, cfgTOML)
	if len(tasks) == 0 {
		tasks = []string{reviewTaskID}
	}
	for _, id := range tasks {
		queueTask(t, root, id, StatusMeasure, "")
	}
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "queue")
	mustGit(t, root, "push", "-q", "origin", "main")
	return root
}

// queueTask writes a task stub with the given status. It does not commit: the
// caller decides what origin holds.
func queueTask(t *testing.T, root, id, status, frontMatter string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, ".tasks", id+".md"),
		"---\nstatus: "+status+"\ncreated: 2026-08-13\n"+frontMatter+"links: []\n---\n\n# Health endpoint is slow\n\n## Context\n\nIt feels slow.\n\n## Decisions\n\n## Log\n")
}

func runReview(t *testing.T, root string) (ReviewResult, string, string, error) {
	t.Helper()
	var out, errb strings.Builder
	res, err := Review(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{TaskID: reviewTaskID})
	return res, out.String(), errb.String(), err
}

func runReviewNext(t *testing.T, root string) (ReviewResult, string, string, error) {
	t.Helper()
	var out, errb strings.Builder
	res, err := Review(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{Next: true})
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
*jalon-review-task*) ` + rewriteTask + ` ;;
esac`
}

func TestReviewHappyPath(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	res, out, _, err := runReview(t, root)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if res.TaskID != reviewTaskID {
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
	if _, err := os.Stat(filepath.Join(root, ".jalon", "worktrees", "review-"+reviewTaskID)); !os.IsNotExist(err) {
		t.Error("the worktree directory survived a successful review")
	}
	// Three phases, three separate processes.
	if n := len(s.models(t)); n != 3 {
		t.Errorf("the model was called %d times, want 3 phases", n)
	}
	// The same task file, rewritten, is on the branch that was pushed, and
	// nothing else is.
	branch := "origin/task/" + reviewTaskID
	if out := gitOut(t, root, "show", "--stat", "--format=", branch); !strings.Contains(out, ".tasks/"+reviewTaskID+".md") {
		t.Errorf("the pushed commit does not carry the task file:\n%s", out)
	} else if strings.Contains(out, ".jalon-review") {
		t.Errorf("the pushed commit carries the working files:\n%s", out)
	}
	// jalon set the status: this is a proposal, and merging is the agreement.
	file := gitOut(t, root, "show", branch+":.tasks/"+reviewTaskID+".md")
	if !strings.Contains(file, "\nstatus: proposed\n") {
		t.Errorf("the task on the branch is not proposed:\n%s", file)
	}
	if !strings.Contains(file, "Measured: the endpoint answers in 4 ms") {
		t.Errorf("the phase's rewrite is not on the branch:\n%s", file)
	}
	// The local checkout was never touched.
	local, _ := os.ReadFile(filepath.Join(root, ".tasks", reviewTaskID+".md"))
	if strings.Contains(string(local), "proposed") {
		t.Error("the review wrote into the local checkout")
	}
}

// The tool policy is the security surface, so it is asserted on the argv that
// was actually passed, not on the function that builds it.
func TestReviewToolPolicy(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

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

	// The gathering phase has read tools and no shell at all: it chooses the
	// probes, jalon runs them.
	if !strings.Contains(facts, "--tools Read,Grep,Glob --allowed-tools Read,Grep,Glob") || strings.Contains(facts, "Bash") {
		t.Errorf("the facts phase must have read tools and no shell:\n%s", facts)
	}
	// The skeptic gets the probes and no write tool.
	if !strings.Contains(skeptic, "Bash(jalon digest:*),Bash(curl -s http://localhost:8080/healthz:*),Read,Grep,Glob") {
		t.Errorf("the skeptic policy is not derived from the allowlist:\n%s", skeptic)
	}
	if strings.Contains(skeptic, "Write") {
		t.Errorf("the skeptic must have no write tool:\n%s", skeptic)
	}
	// The writing phase may edit the task and run jalon append, and nothing
	// else. It creates no task: the id already exists.
	for _, want := range []string{"Bash(jalon append:*)", "Edit(.tasks/**)"} {
		if !strings.Contains(task, want) {
			t.Errorf("the task phase is missing %s:\n%s", want, task)
		}
	}
	if strings.Contains(task, "jalon new") {
		t.Errorf("the task phase may create tasks, and there is one already:\n%s", task)
	}
	// A Write rule is never matched by Claude Code's file permission checks, so
	// offering one sends the model down a path that gets denied. This cost a
	// real job, which failed having created nothing.
	if strings.Contains(task, "Write(") {
		t.Errorf("the task phase offers a Write rule, which is never in force:\n%s", task)
	}
	// It is never handed git, gh or a push.
	for _, forbidden := range []string{"Bash(git", "Bash(gh"} {
		if strings.Contains(task, forbidden) {
			t.Errorf("the task phase was handed %s:\n%s", forbidden, task)
		}
	}
	// Every phase is given the task as the person wrote it.
	for i, c := range calls {
		if !strings.Contains(c, "It feels slow.") {
			t.Errorf("call %d was not given the task:\n%s", i, c)
		}
	}
}

// A phase whose output is discarded cannot be diagnosed when it fails. This one
// failed on a real job having created nothing, and the cause was only found by
// replaying the invocation by hand.
func TestReviewKeepsWhatTheWritingPhasePrinted(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*) printf '%s' '`+factsPlan+`' ;;
*jalon-review-skeptic*) echo "The premise does not hold." ;;
*jalon-review-task*) echo "A Write rule was offered, denied, and I stopped." ;;
esac`)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("a writing phase that changes no task must fail the review")
	}
	if !strings.Contains(err.Error(), "changed nothing under .tasks") {
		t.Errorf("the failure does not say what the phase failed to do: %v", err)
	}
	if res.Worktree == "" {
		t.Fatal("the worktree must be kept, or there is nothing left to read")
	}
	saved := filepath.Join(root, res.Worktree, reviewDir, "task.md")
	b, rerr := os.ReadFile(saved)
	if rerr != nil {
		t.Fatalf("the writing phase output was discarded: %v", rerr)
	}
	if !strings.Contains(string(b), "denied") {
		t.Errorf("task.md does not hold what the phase printed:\n%s", b)
	}
	if !strings.Contains(err.Error(), reviewDir+"/task.md") {
		t.Errorf("the failure does not say where to read it: %v", err)
	}
}

// One id, rewritten in place. A phase that creates a second task file has
// misunderstood the job, and git catches it, not a reader.
func TestReviewRefusesASecondTaskFile(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*) printf '%s' '`+factsPlan+`' ;;
*jalon-review-skeptic*) echo "The premise does not hold." ;;
*jalon-review-task*) `+rewriteTask+`; printf -- '---\nstatus: proposed\n---\n\n# Second\n' > .tasks/260813-second.md ;;
esac`)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "nothing else") {
		t.Fatalf("err = %v, want a refusal naming the extra change", err)
	}
	if out := gitOut(t, root, "branch", "--list", "task/*"); strings.TrimSpace(out) != "" {
		t.Errorf("a refused review left a branch behind: %q", out)
	}
}

// A failing skeptic must still leave what it printed on disk, for the same
// reason the writing phase does: the cause of a failure is otherwise found
// only by replaying the invocation by hand.
func TestReviewKeepsWhatTheSkepticPrintedOnFailure(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*) printf '%s' '`+factsPlan+`' ;;
*jalon-review-skeptic*) echo "The budget ran out before I could finish."; exit 1 ;;
esac`)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("a skeptic phase that fails must fail the review")
	}
	if res.Worktree == "" {
		t.Fatal("the worktree must be kept, or there is nothing left to read")
	}
	saved := filepath.Join(root, res.Worktree, reviewDir, "skeptic.md")
	b, rerr := os.ReadFile(saved)
	if rerr != nil {
		t.Fatalf("the skeptic phase output was discarded: %v", rerr)
	}
	if !strings.Contains(string(b), "budget ran out") {
		t.Errorf("skeptic.md does not hold what the phase printed:\n%s", b)
	}
}

// Nothing updates a target repository's checkout: the unit pulls the jalon
// checkout and only that. A job therefore has to run on what the forge has, or
// it measures stale code and commits on top of an ancestor.
func TestTheWorktreeIsCutFromOriginNotFromTheLocalBranch(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	// A commit that is on origin and not in this checkout, which is the state
	// every target repository is in a few days after it is cloned.
	mustWrite(t, filepath.Join(root, "MERGED_ELSEWHERE.md"), "landed while this clone was not looking\n")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "merged elsewhere")
	mustGit(t, root, "push", "-q", "origin", "main")
	mustGit(t, root, "reset", "-q", "--hard", "HEAD~1")
	if _, err := os.Stat(filepath.Join(root, "MERGED_ELSEWHERE.md")); !os.IsNotExist(err) {
		t.Fatal("the fixture failed to put the checkout behind origin")
	}

	var out, errb strings.Builder
	res, err := Review(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{TaskID: reviewTaskID, KeepWorktree: true})
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, res.Worktree, "MERGED_ELSEWHERE.md")); err != nil {
		t.Errorf("the worktree was cut from the local branch, so the job ran on stale code: %v", err)
	}
	// And the local checkout is still untouched, which is the reason this is a
	// fetch and a detached worktree rather than a pull.
	if _, err := os.Stat(filepath.Join(root, "MERGED_ELSEWHERE.md")); !os.IsNotExist(err) {
		t.Error("the fetch moved the local working tree, which jalon must never do")
	}
}

// The task is read from origin too, for the same reason: what this checkout
// holds is not what the worktree will hold. A task that was never pushed is
// refused before a token is spent, and the message says to push it.
func TestReviewReadsTheTaskFromOrigin(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newRepo(t, reviewTOML)
	queueTask(t, root, reviewTaskID, StatusMeasure, "")
	// Local only: written, not committed, not pushed.

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "push it first") {
		t.Fatalf("err = %v, want a refusal saying the task is not on origin", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made for a task origin does not have")
	}
}

// Fatal, unlike the unit's pull: running on the tree you already have is the
// defect this fetch exists to remove.
func TestAJobRefusesToRunWhenItCannotReachOrigin(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)
	mustGit(t, root, "remote", "remove", "origin")

	_, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("a job that cannot reach origin must refuse to start")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("the failure does not say why it refuses: %v", err)
	}
	// No phase ran, so nothing was spent on a tree that could not be trusted.
	if n := len(s.models(t)); n != 0 {
		t.Errorf("the model was called %d times despite an unreachable origin", n)
	}
}

// The gate is the reason this design is worth anything: no measurement, no
// task. A phase that narrates instead of naming probes has every line refused,
// nothing runs, and the job stops with the refusals named.
func TestReviewGateRefusesNarration(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith(strings.Repeat("This looks slow to me and I believe it should be improved.\n", 3)))
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	res, _, _, err := runReview(t, root)
	if err == nil {
		t.Fatal("narration was accepted")
	}
	if !strings.Contains(err.Error(), "no command on the probe list") || !strings.Contains(err.Error(), "3 line(s) refused") {
		t.Errorf("err = %v, want it to say nothing ran and count the refusals", err)
	}
	assertStopped(t, s, root, res, 1)
	// What the phase printed is kept for reading, beside the facts.
	if b, rerr := os.ReadFile(filepath.Join(root, res.Worktree, reviewDir, "probes.md")); rerr != nil || !strings.Contains(string(b), "looks slow") {
		t.Errorf("probes.md does not hold what the phase printed: %v %q", rerr, b)
	}
}

// A composed line, a command outside the list, and a program this machine
// lacks are refused and named, not run and not fatal: as long as one probe
// ran there are facts. The refusals reach the document, stderr and the pull
// request, so a reader knows what nothing rests on.
func TestReviewRefusesProbesWithoutStopping(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith("psql -c drop\njalon digest x 2>&1 || true\ncurl -s http://localhost:8080/healthz\njalon digest "+reviewTaskID+"\n"))
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	// No jalon stub: the fourth line names a program this machine lacks.
	root := newReviewRepo(t, reviewTOML)

	res, _, errb, err := runReview(t, root)
	if err != nil {
		t.Fatalf("a refused probe must not stop the job while another ran: %v", err)
	}
	for _, want := range []string{"probe refused: psql -c drop", "probe refused: jalon digest x 2>&1 || true", "not installed: jalon", "1 probe(s) run, 3 refused"} {
		if !strings.Contains(errb, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errb)
		}
	}
	body := ""
	for _, c := range s.of(t, "gh") {
		if strings.Contains(c, "pr create") {
			body = c
		}
	}
	if !strings.Contains(body, "jalon refused") || !strings.Contains(body, "psql -c drop") {
		t.Errorf("the pull request body does not carry the refusals:\n%s", body)
	}
	if !strings.Contains(body, "Cost: 0.75 USD") {
		t.Errorf("the pull request body does not carry the cost of three phases:\n%s", body)
	}
	if res.TaskID == "" {
		t.Error("the review produced no task")
	}
}

// The facts are jalon's own output: the command as asked, what it printed,
// and its exit status when it failed. The skeptic and the writing phase read
// exactly that, and no block in it was ever written by a model.
func TestTheFactsAreWhatTheProbesPrinted(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith("curl -s http://localhost:8080/healthz\ncurl -s http://localhost:8080/missing\n"))
	s.add(t, "gh", happyGH)
	s.add(t, "curl", `case "$*" in
*missing*) echo "curl: (22) The requested URL returned error: 404" >&2; exit 22 ;;
*) echo '{"status":"ok"}' ;;
esac`)
	// A wider prefix, so both curl lines are on the list.
	root := newReviewRepo(t, strings.Replace(reviewTOML, "curl -s http://localhost:8080/healthz", "curl -s", 1))

	var out, errb strings.Builder
	res, err := Review(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{TaskID: reviewTaskID, KeepWorktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, rerr := os.ReadFile(filepath.Join(root, res.Worktree, reviewDir, "facts.md"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	facts := string(b)
	for _, want := range []string{
		"$ curl -s http://localhost:8080/healthz\n{\"status\":\"ok\"}\n",
		"$ curl -s http://localhost:8080/missing\n(exit 22: curl: (22)",
		"Nothing here was written by a model",
	} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts.md lacks %q:\n%s", want, facts)
		}
	}
	// The skeptic was given those facts, not the plan.
	calls := s.models(t)
	if !strings.Contains(calls[1], "(exit 22:") {
		t.Errorf("the skeptic did not receive the facts jalon wrote:\n%s", calls[1])
	}
	// The probes ran in the worktree, which is what the model reads.
	for _, c := range s.of(t, "curl") {
		if !strings.Contains(c, "curl -s http://localhost:8080/") {
			t.Errorf("unexpected curl call: %s", c)
		}
	}
}

// assertStopped checks the three things every failed review must be true of: it
// pushed nothing, it kept its evidence, and it did not run the phases after the
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
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)
	mustWrite(t, filepath.Join(root, ".jalon", "worktrees", "review-"+reviewTaskID, "facts.md"), "earlier evidence")

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
	if b, _ := os.ReadFile(filepath.Join(root, ".jalon", "worktrees", "review-"+reviewTaskID, "facts.md")); string(b) != "earlier evidence" {
		t.Error("the earlier evidence was destroyed")
	}
}

// A done task is either already implemented or was closed on purpose; a review
// of it would propose work nobody asked for.
func TestReviewRefusesADoneTask(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newRepo(t, reviewTOML)
	queueTask(t, root, reviewTaskID, "done", "")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "done")
	mustGit(t, root, "push", "-q", "origin", "main")

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "is done") {
		t.Fatalf("err = %v, want a refusal naming the done task", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made on a done task")
	}
}

// The cap is a plain text counter you can read with cat, and it has to hold
// before anything is spent.
func TestReviewDailyCap(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, strings.Replace(reviewTOML, "daily_job_cap = 10", "daily_job_cap = 1", 1))

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

	// A second review of the same task by hand: the cap refuses before the
	// worktree, so the branch on origin is not what stops it.
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
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, "")

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), "agent.toml") {
		t.Fatalf("err = %v, want a refusal naming the missing configuration", err)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made on a red base")
	}
}

func TestNotifyRunsOneCommand(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	// Shell builtins only: PATH is restricted in these tests, and the point here
	// is that the message reaches the command's stdin, not that coreutils exist.
	root := newReviewRepo(t, strings.Replace(reviewTOML,
		"[probes]", "[notify]\ncommand = 'while IFS= read -r l; do echo \"$l\" >> notified.txt; done'\n\n[probes]", 1))

	if _, _, _, err := runReview(t, root); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "notified.txt"))
	if err != nil {
		t.Fatalf("the notification command did not run: %v", err)
	}
	if !strings.Contains(string(b), reviewTaskID) {
		t.Errorf("the notification does not name the task: %q", b)
	}
	if !strings.Contains(string(b), "cost 0.75 USD") {
		t.Errorf("the notification does not carry the cost: %q", b)
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

// A phase is told what it may run rather than left to discover it by being
// denied. The first live runs spent turns on that and then reported the refused
// commands as though they had run.
func TestReviewTellsThePhaseItsProbes(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	if _, _, _, err := runReview(t, root); err != nil {
		t.Fatal(err)
	}
	calls := s.models(t)
	for i, name := range []string{"facts", "skeptic"} {
		if !strings.Contains(calls[i], "the only shell commands you may run") {
			t.Errorf("the %s phase was not given the allowlist:\n%s", name, calls[i])
		}
		if !strings.Contains(calls[i], "curl -s http://localhost:8080/healthz") {
			t.Errorf("the %s phase was not given the probes themselves:\n%s", name, calls[i])
		}
	}
}

// -next is what the timer runs. An empty queue is the normal state between
// jobs, so it must succeed quietly rather than fail hourly, and it must not pay
// for the criterion: doctor runs the repository's whole test suite, and a tick
// that finds nothing would throw that away. Measured at ~3.6s per tick on a
// real server before this was fixed.
func TestReviewNextWithNothingQueued(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	// A criterion that leaves a trace, so "did not run" is observable rather
	// than assumed.
	root := newRepo(t, strings.Replace(reviewTOML, `command = "true"`, `command = "touch criterion-ran"`, 1))
	queueTask(t, root, reviewTaskID, "todo", "")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "a task nobody queued")
	mustGit(t, root, "push", "-q", "origin", "main")

	res, _, errb, err := runReviewNext(t, root)
	if err != nil {
		t.Fatalf("an empty queue must not be a failure: %v", err)
	}
	if res.TaskID != "" {
		t.Errorf("task id = %q, want none", res.TaskID)
	}
	if !strings.Contains(errb, "nothing to review") {
		t.Errorf("stderr must say why nothing happened:\n%s", errb)
	}
	if len(s.models(t)) > 0 {
		t.Error("a model call was made with nothing queued")
	}
	// The whole point of the reorder.
	if _, err := os.Stat(filepath.Join(root, "criterion-ran")); !os.IsNotExist(err) {
		t.Error("an empty queue ran the criterion; the preflight must come after the queue on the -next path")
	}
	// No job was run, so no slot was taken.
	if _, err := os.Stat(filepath.Join(root, ".jalon", "agent-jobs-today")); !os.IsNotExist(err) {
		t.Error("an empty queue consumed a job from the daily cap")
	}
	// And no forge call at all: the queue is git.
	if n := len(s.of(t, "gh")); n != 0 {
		t.Errorf("reading the queue made %d gh calls, want 0", n)
	}
}

// With several queued tasks it takes the oldest, so the queue drains in order;
// it skips the ones already published (a branch on origin) and the ones whose
// wreck is parked, without a label or a forge call for either.
func TestReviewNextTakesTheOldestUnpublished(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML,
		"260810-published", "260811-parked", "260812-oldest-free", "260813-younger")
	// 260810 has its proposal branch on origin already.
	mustGit(t, root, "push", "-q", "origin", "main:task/260810-published")
	// 260811 failed last time and its wreck sits in .jalon/failed.
	mustWrite(t, filepath.Join(root, ".jalon", "failed", "review-260811-parked", "x"), "")

	res, _, _, err := runReviewNext(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.TaskID != "260812-oldest-free" {
		t.Errorf("task = %q, want the oldest that is neither published nor parked", res.TaskID)
	}
	if n := len(s.of(t, "gh")); n != 3 {
		// auth status, api -i for doctor, pr create: none for the queue.
		t.Errorf("gh was called %d times, want 3 (none for the queue):\n%s", n, strings.Join(s.of(t, "gh"), "\n"))
	}
}

// A published review leaves the queue by existing as a branch: without that
// the same task is measured again on the next tick, forever, spending the
// daily cap on one question. Found live before the timer was even armed.
func TestReviewLeavesTheQueueWhenItIsDone(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	if res, _, _, err := runReviewNext(t, root); err != nil || res.TaskID != reviewTaskID {
		t.Fatalf("first tick: %v %q", err, res.TaskID)
	}
	res, _, errb, err := runReviewNext(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if res.TaskID != "" {
		t.Errorf("the second tick measured %q again", res.TaskID)
	}
	if !strings.Contains(errb, "already published or parked") {
		t.Errorf("stderr must say why the queued task was skipped:\n%s", errb)
	}
	if n := len(s.models(t)); n != 3 {
		t.Errorf("the model was called %d times over two ticks, want 3", n)
	}
}

// One failure must cost one item, not the night. Before this, a failed job
// kept its worktree where doctor looks, doctor refused the next job, and an
// hourly timer stayed frozen until a person came back. The wreck is now parked
// under .jalon/failed, which takes its task out of the queue, and the next
// tick runs the next item.
func TestAFailedReviewIsParkedAndTheNextJobRuns(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-review-facts*) echo "The budget ran out."; exit 1 ;;
esac`)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)

	res, _, _, err := runReviewNext(t, root)
	if err == nil {
		t.Fatal("a failing facts phase must fail the review")
	}
	if !strings.HasPrefix(res.Worktree, filepath.FromSlash(failedDir)) {
		t.Fatalf("worktree = %q, want it parked under %s", res.Worktree, failedDir)
	}
	if b, rerr := os.ReadFile(filepath.Join(root, res.Worktree, reviewDir, "probes.md")); rerr != nil || !strings.Contains(string(b), "budget") {
		t.Errorf("the evidence did not survive the move: %v %q", rerr, b)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".jalon", "worktrees")); len(entries) != 0 {
		t.Errorf("the running-jobs directory is not empty after the failure: %v", entries)
	}
	// The next job is not blocked: doctor warns, it does not fail.
	r, _, _ := runDoctor(t, root)
	if c := state(t, r, "git"); c.State != Warn {
		t.Errorf("git = %s after a parked failure, want warn: the next job must run", c.State)
	}
	// And the wreck keeps its task out of the queue: the next tick finds
	// nothing, rather than failing the same way again.
	if res, _, _, err := runReviewNext(t, root); err != nil || res.TaskID != "" {
		t.Errorf("second tick: err %v, task %q; the parked task must be skipped", err, res.TaskID)
	}
	if n := len(s.models(t)); n != 1 {
		t.Errorf("the model was called %d times over two ticks, want 1", n)
	}
	// Reviewing it by hand parks over the first wreck rather than beside it.
	if _, _, _, err := runReview(t, root); err == nil {
		t.Fatal("the run by hand must fail the same way")
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".jalon", "failed")); len(entries) != 1 {
		t.Errorf("want one wreck per job name, got %d", len(entries))
	}
}

// Ten wrecks are more than anyone reads; the eleventh job refuses before it
// spends a token, and names the directory.
func TestTheFailedDirectoryHasACap(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, reviewTOML)
	for i := 0; i < maxFailed; i++ {
		mustWrite(t, filepath.Join(root, ".jalon", "failed", fmt.Sprintf("review-%d", i), "x"), "")
	}

	_, _, _, err := runReview(t, root)
	if err == nil || !strings.Contains(err.Error(), failedDir) {
		t.Fatalf("want a refusal naming %s, got %v", failedDir, err)
	}
	if n := len(s.models(t)); n != 0 {
		t.Errorf("the model was called %d times before the refusal", n)
	}
}

// setStatus is a line replacement, not a parser: every other byte survives.
func TestSetStatus(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.md")
	mustWrite(t, p, "---\nstatus: measure\ncreated: 2026-08-13\nunknown: kept as is\nlinks: [a.go]\n---\n\n# T\n\n## Context\n\nbody: not front matter\n")
	if err := setStatus(p, "proposed"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	want := "---\nstatus: proposed\ncreated: 2026-08-13\nunknown: kept as is\nlinks: [a.go]\n---\n\n# T\n\n## Context\n\nbody: not front matter\n"
	if string(b) != want {
		t.Errorf("got:\n%s\nwant:\n%s", b, want)
	}
	mustWrite(t, p, "# no front matter\n")
	if err := setStatus(p, "proposed"); err == nil {
		t.Error("a file without front matter must be refused, not rewritten")
	}
}

// A path with parentheses is an ordinary probe argument: jalon runs probes
// with exec and no shell, so nothing can compose. The first live review lost
// the file it needed to a refused `routes/(app)/start`.
func TestProbesWithParenthesesRun(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWith("curl -s http://localhost:8080/healthz\ngit log --oneline -1 -- routes/(app)/start/+page.svelte\ngit log a | wc -l\n"))
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newReviewRepo(t, strings.Replace(reviewTOML, "  \"jalon digest\",\n", "  \"jalon digest\",\n  \"git log\",\n", 1))

	var out, errb strings.Builder
	res, err := Review(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{TaskID: reviewTaskID, KeepWorktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, res.Worktree, reviewDir, "facts.md"))
	if !strings.Contains(string(b), "$ git log --oneline -1 -- routes/(app)/start/+page.svelte\n") {
		t.Errorf("the parenthesised path was not run:\n%s", b)
	}
	if !strings.Contains(errb.String(), "probe refused: git log a | wc -l") {
		t.Errorf("the piped line was not refused:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), "2 probe(s) run, 1 refused") {
		t.Errorf("counts:\n%s", errb.String())
	}
}

// jalon grounds every review itself: where the words of the premise appear
// in file contents, before anything a model chose to look at.
func TestFactsCarryThePremiseGrep(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", happyClaude)
	s.add(t, "gh", happyGH)
	s.add(t, "curl", happyCurl)
	root := newRepo(t, reviewTOML)
	mustWrite(t, filepath.Join(root, "routes", "(app)", "start", "+page.svelte"), "<script>engine.startRecording()</script>\n")
	mustWrite(t, filepath.Join(root, "routes", "(app)", "recordings", "+page.svelte"), "<h1>Past sessions</h1>\n")
	mustWrite(t, filepath.Join(root, ".tasks", reviewTaskID+".md"),
		"---\nstatus: measure\ncreated: 2026-08-13\nlinks: []\n---\n\n# On the record screen, a button that loops the recording\n\n## Context\n\nx\n\n## Decisions\n\n## Log\n")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "fixture")
	mustGit(t, root, "push", "-q", "origin", "main")

	var out, errb strings.Builder
	res, err := Review(context.Background(), Env{Root: root, Stdout: &out, Stderr: &errb},
		ReviewOptions{TaskID: reviewTaskID, KeepWorktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, res.Worktree, reviewDir, "facts.md"))
	facts := string(b)
	if !strings.Contains(facts, "## Where the premise's words appear") {
		t.Fatalf("no premise grep in the facts:\n%s", facts)
	}
	// "record" is in the title; the file that records is under start, and the
	// grep says so, on content and not on names.
	if !strings.Contains(facts, "- record: routes/(app)/start/+page.svelte") {
		t.Errorf("the content grep did not find where the premise lives:\n%s", facts)
	}
	if !strings.Contains(facts, "- loops: no file") {
		t.Errorf("a word absent from the tree must say so:\n%s", facts)
	}
	// The skeptic reads it: the facts are on its stdin.
	if calls := s.models(t); !strings.Contains(calls[1], "Where the premise's words appear") {
		t.Errorf("the skeptic was not given the grep")
	}
}
