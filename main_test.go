package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runOK runs a command and fails the test if it errors.
func runOK(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errb strings.Builder
	if err := run(args, &out, &errb); err != nil {
		t.Fatalf("run %v: %v (stderr: %s)", args, err, errb.String())
	}
	return out.String(), errb.String()
}

func runErr(t *testing.T, args ...string) error {
	t.Helper()
	var out, errb strings.Builder
	err := run(args, &out, &errb)
	if err == nil {
		t.Fatalf("run %v: want an error, got none (stdout: %s)", args, out.String())
	}
	return err
}

func TestEndToEnd(t *testing.T) {
	t.Setenv("JALON_SIG", "gt")
	root, tasksDir := newRepo(t)
	today := time.Now().Format(dateLayout)
	id := time.Now().Format(idLayout) + "-migration-auth"

	stdout, _ := runOK(t, "new", "-dir", tasksDir, "migration auth")
	path := strings.TrimSpace(stdout)
	if filepath.Base(path) != id+".md" {
		t.Fatalf("new wrote %q, want %s.md", path, id)
	}

	runOK(t, "append", "-dir", tasksDir, id, "blocked on the refresh path")
	runOK(t, "append", "-dir", tasksDir, "-decision", id, "cookie sessions over JWT")

	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := task.Entries(sectionLog); len(got) != 1 || !strings.Contains(got[0], today+" gt: blocked on the refresh path") {
		t.Errorf("Log entries = %v", got)
	}
	if got := task.Entries(sectionDecisions); len(got) != 1 || !strings.Contains(got[0], "cookie sessions over JWT") {
		t.Errorf("Decisions entries = %v", got)
	}

	gitCommit(t, root, "["+id+"] fix the refresh token path")
	stdout, stderr := runOK(t, "digest", "-dir", tasksDir, "-offline", id)
	for _, want := range []string{"# task " + id, "## Decisions", "## Log", "fix the refresh token path"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("digest is missing %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "# digest "+id) || !strings.Contains(stderr, "tokens (bytes/4)") {
		t.Errorf("digest must report its own size on stderr, got %q", stderr)
	}
	if i, j := strings.Index(stdout, "## Decisions"), strings.Index(stdout, "## Log"); i > j {
		t.Error("digest must put Decisions before Log")
	}

	runOK(t, "render", "-dir", tasksDir)
	if _, err := os.Stat(filepath.Join(tasksDir, "site", "index.html")); err != nil {
		t.Errorf("render did not write the index: %v", err)
	}

	stdout, _ = runOK(t, "close", "-dir", tasksDir, id)
	if !strings.Contains(stdout, statusDone) {
		t.Errorf("close said %q", stdout)
	}
	task, err = ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != statusDone {
		t.Errorf("status = %q after close", task.Status())
	}
	if entries := task.Entries(sectionLog); len(entries) != 2 {
		t.Errorf("close must leave a trace in the Log, entries = %v", entries)
	}
}

func TestCloseFromMerge(t *testing.T) {
	t.Setenv("JALON_SIG", "gt")
	_, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: todo\n---\n\n# Auth\n\n## Log\n")

	stdin := filepath.Join(t.TempDir(), "msg")
	mustWrite(t, stdin, "Merge pull request #12 from feat/auth\n\ncloses 260806\n")
	f, err := os.Open(stdin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = saved }()

	stdout, _ := runOK(t, "close", "-dir", tasksDir, "-from-merge")
	if !strings.Contains(stdout, "260806-auth done") {
		t.Fatalf("close -from-merge said %q", stdout)
	}
}

func TestCloseFromMergeKeepsGoingAfterABadReference(t *testing.T) {
	// The merge already happened. One ambiguous reference used to abort the
	// loop, silently discarding the valid ones after it.
	t.Setenv("JALON_SIG", "gt")
	_, tasksDir := newRepo(t)
	for _, name := range []string{"260807-auth", "260807-billing"} {
		mustWrite(t, filepath.Join(tasksDir, name+".md"), "---\nstatus: doing\n---\n\n# T\n\n## Log\n")
	}

	stdin := filepath.Join(t.TempDir(), "msg")
	mustWrite(t, stdin, "Merge pull request #12\n\ncloses 260807\ncloses 260807-billing\n")
	f, err := os.Open(stdin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = saved }()

	stdout, stderr := runOK(t, "close", "-dir", tasksDir, "-from-merge")
	if !strings.Contains(stdout, "260807-billing done") {
		t.Errorf("the valid reference must still be closed, got %q", stdout)
	}
	if !strings.Contains(stderr, "matches 2 tasks") || !strings.Contains(stderr, "longer id") {
		t.Errorf("the ambiguous reference must be reported with the fix, got %q", stderr)
	}
	task, err := ParseTask(filepath.Join(tasksDir, "260807-billing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != statusDone {
		t.Errorf("260807-billing status = %q", task.Status())
	}
	// The ambiguous one is untouched: jalon never guesses which task to write to.
	other, err := ParseTask(filepath.Join(tasksDir, "260807-auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	if other.Status() != "doing" {
		t.Errorf("260807-auth must be left alone, status = %q", other.Status())
	}
}

func TestClosesRefs(t *testing.T) {
	msg := "Merge pull request #12\n\nCloses 260806 and closes [260807-billing].\ncloses 260806\nnot a close: 260808\n"
	got := closesRefs(msg)
	want := []string{"260806", "260807-billing"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("closesRefs = %v, want %v", got, want)
	}
}

func TestAmbiguousPrefixIsRefused(t *testing.T) {
	_, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: todo\n---\n\n# Auth\n\n## Log\n")
	mustWrite(t, filepath.Join(tasksDir, "260806-billing.md"), "---\nstatus: todo\n---\n\n# Billing\n\n## Log\n")

	err := runErr(t, "append", "-dir", tasksDir, "260806", "x")
	if !strings.Contains(err.Error(), "matches 2 tasks") {
		t.Errorf("want an ambiguity error, got %v", err)
	}
	err = runErr(t, "append", "-dir", tasksDir, "999999", "x")
	if !strings.Contains(err.Error(), "no task matching") {
		t.Errorf("want a not found error, got %v", err)
	}
	// A pasted commit tag must not be read as a glob character class.
	err = runErr(t, "append", "-dir", tasksDir, "[260806]", "x")
	if !strings.Contains(err.Error(), "not allowed in an id") {
		t.Errorf("want a rejected id, got %v", err)
	}
}

func TestFlagAfterArgIsRefused(t *testing.T) {
	// The flag package would silently append "-decision" to the entry text.
	_, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: todo\n---\n\n# Auth\n\n## Log\n")

	err := runErr(t, "append", "-dir", tasksDir, "260806", "-decision", "x")
	if !strings.Contains(err.Error(), "must come before") {
		t.Errorf("want a flag placement error, got %v", err)
	}
}

func TestCompact(t *testing.T) {
	_, tasksDir := newRepo(t)
	path := filepath.Join(tasksDir, "260806-auth.md")
	var b strings.Builder
	b.WriteString("---\nstatus: todo\n---\n\n# Auth\n\n## Log\n\n")
	for i := range 25 {
		b.WriteString("- 2026-08-0")
		b.WriteByte(byte('1' + i%9))
		b.WriteString(" gt: an entry long enough to weigh something in the token budget.\n")
	}
	mustWrite(t, path, b.String())

	err := runErr(t, "compact", "-dir", tasksDir, "-check", "-max-tokens", "10", "260806")
	if !strings.Contains(err.Error(), "over the token budget") {
		t.Errorf("check must fail over budget, got %v", err)
	}
	if before := readFile(t, path); !strings.Contains(before, "an entry long enough") || strings.Contains(before, "truncated") {
		t.Error("check must not modify the file")
	}

	stdout, _ := runOK(t, "compact", "-dir", tasksDir, "-keep", "5", "260806")
	if !strings.Contains(stdout, "20 log entries truncated") {
		t.Errorf("compact said %q", stdout)
	}
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := task.Entries(sectionLog); len(got) != 6 {
		t.Errorf("after compact: %d entries, want 5 plus the marker", len(got))
	}
	runOK(t, "compact", "-dir", tasksDir, "-check", "-max-tokens", "10000", "260806")
}

func TestFindTasksDirWalksUp(t *testing.T) {
	root, tasksDir := newRepo(t)
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findTasksDir(nested)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, _ := filepath.EvalSymlinks(got); resolved != mustEval(t, tasksDir) {
		t.Errorf("findTasksDir = %q, want %q", got, tasksDir)
	}
	if _, err := findTasksDir(t.TempDir()); err == nil {
		t.Error("findTasksDir must fail when there is no .tasks anywhere up the tree")
	}
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUnknownCommand(t *testing.T) {
	if err := runErr(t, "frobnicate"); !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("got %v", err)
	}
	if err := runErr(t); !strings.Contains(err.Error(), "missing command") {
		t.Errorf("got %v", err)
	}
}
