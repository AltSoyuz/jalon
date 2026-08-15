package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workTOML carries a criterion that is green on a clean tree and red once one
// file exists, so a single configuration exercises both sides of the gate
// without stubbing the criterion away. Doctor runs it too, which is the point:
// it proves the base was green before the model touched anything.
var workTOML = strings.Replace(doctorTOML, `command = "true"`, `command = "test ! -f BROKEN"`, 1)

const workTaskID = "260815-remove-the-neosync-entry"

// claudeWorks answers doctor's probing calls, then does what the work phase is
// told to do: change the checkout and print what it changed.
func claudeWorks(body string) string {
	return `case "$*" in
*--version*) echo "9.9.9 (Claude Code)" ;;
*--help*)    echo "--print --model --output-format --permission-mode --allowed-tools --disallowed-tools --tools --append-system-prompt --max-budget-usd" ;;
*jalon-work*)
` + body + `
  ;;
esac`
}

// newWorkRepo is newRepo plus one agreed task, which is the state jalon work
// starts from: a task file already on the default branch.
func newWorkRepo(t *testing.T, status string) string {
	t.Helper()
	root := newRepo(t, workTOML)
	mustWrite(t, filepath.Join(root, ".tasks", workTaskID+".md"),
		"---\nstatus: "+status+"\ncreated: 2026-08-15\nlinks: []\n---\n\n# Remove the Neosync entry\n\n## Context\n\nOne line, in one file.\n\n## Decisions\n\n## Log\n")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "agree the task")
	mustGit(t, root, "push", "-q", "origin", "main")
	return root
}

func runWork(t *testing.T, root string) (WorkResult, string, string, error) {
	t.Helper()
	var out, errb strings.Builder
	res, err := Work(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		WorkOptions{TaskID: workTaskID})
	return res, out.String(), errb.String(), err
}

func TestWorkHappyPath(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo "- a note" > NOTE.md
  echo "changed NOTE.md. test ! -f BROKEN passes."`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "proposed")

	res, out, _, err := runWork(t, root)
	if err != nil {
		t.Fatalf("work failed: %v", err)
	}
	if res.PR != "https://example.invalid/pull/7" {
		t.Errorf("pr = %q", res.PR)
	}
	if !strings.Contains(out, workTaskID) {
		t.Errorf("stdout must carry the id, got:\n%s", out)
	}
	// One phase, not three: work does not measure and does not refute.
	if n := len(s.models(t)); n != 1 {
		t.Errorf("the model was called %d times, want 1", n)
	}
	// A successful run leaves nothing behind.
	if res.Worktree != "" {
		t.Errorf("worktree = %q, want it removed on success", res.Worktree)
	}
	if _, err := os.Stat(filepath.Join(root, ".jalon", "worktrees", "work-"+workTaskID)); !os.IsNotExist(err) {
		t.Error("the worktree directory survived a successful run")
	}

	branch := "origin/work/" + workTaskID
	files := gitOut(t, root, "show", "--stat", "--format=", branch)
	if !strings.Contains(files, "NOTE.md") {
		t.Errorf("the pushed commit does not carry the change:\n%s", files)
	}
	// jalon's own scratch never reaches the branch.
	for _, forbidden := range []string{workDir, ".claude"} {
		if strings.Contains(files, forbidden) {
			t.Errorf("the pushed commit carries %s:\n%s", forbidden, files)
		}
	}
	// The merge hook closes the task, so the trailer has to be in the commit
	// message and not in the pull request body, which GitHub does not use.
	if msg := gitOut(t, root, "log", "-1", "--format=%B", branch); !strings.Contains(msg, "closes "+workTaskID) {
		t.Errorf("the commit does not close the task, so merging it will not:\n%s", msg)
	}
}

// The criterion is the gate and the whole gate: no judgement on what the model
// wrote, just the repository's own command.
func TestWorkStopsWhenTheCriterionFails(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo broken > BROKEN
  echo "I changed things and did not check."`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "proposed")

	res, _, _, err := runWork(t, root)
	if err == nil {
		t.Fatal("a failing criterion must not produce a pull request")
	}
	if !strings.Contains(err.Error(), "test ! -f BROKEN") {
		t.Errorf("the failure does not name the criterion: %v", err)
	}
	if res.PR != "" {
		t.Errorf("a pull request was opened anyway: %q", res.PR)
	}
	for _, c := range s.of(t, "gh") {
		if strings.Contains(c, "pr create") {
			t.Error("gh pr create ran despite a red criterion")
		}
	}
	// The worktree is the evidence: the diff that failed is the bug report.
	if res.Worktree == "" {
		t.Fatal("the worktree must be kept so the failing diff can be read")
	}
	if _, err := os.Stat(filepath.Join(root, res.Worktree, "BROKEN")); err != nil {
		t.Errorf("the failing change is not in the kept worktree: %v", err)
	}
}

// A phase that says it did the work and changed nothing is caught by git, not
// by a reader.
func TestWorkStopsWhenNothingChanged(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo "The task rests on something that is not true, so I changed nothing."`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "proposed")

	res, _, _, err := runWork(t, root)
	if err == nil {
		t.Fatal("a run that changed no file must fail")
	}
	if !strings.Contains(err.Error(), workDir+"/work.md") {
		t.Errorf("the failure does not say where to read what the phase printed: %v", err)
	}
	if res.PR != "" {
		t.Errorf("a pull request was opened for an empty change: %q", res.PR)
	}
	saved, rerr := os.ReadFile(filepath.Join(root, res.Worktree, workDir, "work.md"))
	if rerr != nil {
		t.Fatalf("the phase output was discarded: %v", rerr)
	}
	if !strings.Contains(string(saved), "not true") {
		t.Errorf("work.md does not hold what the phase printed:\n%s", saved)
	}
}

// The tool policy is the security surface, so it is asserted on the argv that
// was actually passed.
func TestWorkToolPolicy(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo "- a note" > NOTE.md
  echo done`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "proposed")

	if _, _, _, err := runWork(t, root); err != nil {
		t.Fatal(err)
	}
	calls := s.models(t)
	if len(calls) != 1 {
		t.Fatalf("want 1 model call, got %d", len(calls))
	}
	c := calls[0]

	for _, forbidden := range []string{"--dangerously-skip-permissions", "bypassPermissions"} {
		if strings.Contains(c, forbidden) {
			t.Errorf("the work phase carries %s:\n%s", forbidden, c)
		}
	}
	for _, want := range []string{"--permission-mode dontAsk", "--disallowed-tools WebFetch,WebSearch,Task", "Edit"} {
		if !strings.Contains(c, want) {
			t.Errorf("the work phase is missing %s:\n%s", want, c)
		}
	}
	// A Write rule is never matched by Claude Code's file permission checks.
	if strings.Contains(c, "Write(") {
		t.Errorf("the work phase offers a Write rule, which is never in force:\n%s", c)
	}
	// It writes code, but it is never handed the forge or git: jalon does every
	// mutation itself.
	for _, forbidden := range []string{"Bash(git", "Bash(gh"} {
		if strings.Contains(c, forbidden) {
			t.Errorf("the work phase was handed %s:\n%s", forbidden, c)
		}
	}
	// The commands it may run are the probes, which is what lets it iterate on a
	// criterion made of several of them.
	if !strings.Contains(c, "Bash(make check:*)") {
		t.Errorf("the work phase policy is not derived from the allowlist:\n%s", c)
	}
}

// Refused before a single token is spent, which is the difference between a
// typo and a bill.
func TestWorkRefusesAnUnknownTask(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo "should never run"`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "proposed")

	var out, errb strings.Builder
	_, err := Work(context.Background(),
		Env{Root: root, Stdout: &out, Stderr: &errb},
		WorkOptions{TaskID: "260815-no-such-task"})
	if err == nil {
		t.Fatal("an unknown task id must fail")
	}
	if n := len(s.models(t)); n != 0 {
		t.Errorf("the model was called %d times for an unknown task", n)
	}
}

func TestWorkRefusesADoneTask(t *testing.T) {
	s := newStubs(t)
	s.add(t, "claude", claudeWorks(`  echo "should never run"`))
	s.add(t, "gh", happyGH)
	root := newWorkRepo(t, "done")

	_, _, _, err := runWork(t, root)
	if err == nil {
		t.Fatal("a done task must not be implemented again")
	}
	if !strings.Contains(err.Error(), "done") {
		t.Errorf("the failure does not say why: %v", err)
	}
	if n := len(s.models(t)); n != 0 {
		t.Errorf("the model was called %d times for a done task", n)
	}
}
