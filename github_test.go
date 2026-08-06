package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH puts a stub `gh` first on PATH. The bridge is one process call, so a
// script is enough to test it, and the tests stay offline.
func fakeGH(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// noGH narrows PATH to git alone, which is the degraded mode most users will
// actually run in: a git repository, no forge CLI.
func noGH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for this test")
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

const issueJSON = `{"number":42,"title":"digest is slow on big repos","body":"Steps:\n\n1. create 500 tasks\n2. run digest","url":"https://github.com/AltSoyuz/jalon/issues/42","state":"OPEN"}`

func TestNewFromIssue(t *testing.T) {
	fakeGH(t, `case "$1 $2" in
"issue view") printf '%s' '`+issueJSON+`' ;;
*) echo "unexpected: $*" >&2; exit 1 ;;
esac`)
	_, tasksDir := newRepo(t)

	stdout, _ := runOK(t, "new", "-dir", tasksDir, "-issue", "42")
	task, err := ParseTask(strings.TrimSpace(stdout))
	if err != nil {
		t.Fatal(err)
	}

	if got := task.FM.Get(fmKeyIssue); got != "42" {
		t.Errorf("issue key = %q, want 42", got)
	}
	if got := task.Title(); got != "digest is slow on big repos" {
		t.Errorf("title = %q, want the issue title", got)
	}
	if !strings.HasSuffix(task.ID, "-digest-is-slow-on-big-repos") {
		t.Errorf("id = %q, want a slug built from the issue title", task.ID)
	}
	ctx := task.Section(sectionContext)
	for _, want := range []string{"https://github.com/AltSoyuz/jalon/issues/42", "create 500 tasks"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("Context is missing %q:\n%s", want, ctx)
		}
	}
	// The rest of the file must be untouched by the seeding.
	if task.Section(sectionDecisions) != "" || task.Section(sectionLog) != "" {
		t.Error("seeding must only fill Context")
	}
}

func TestNewFromIssueKeepsAnExplicitTitle(t *testing.T) {
	fakeGH(t, `printf '%s' '`+issueJSON+`'`)
	_, tasksDir := newRepo(t)

	stdout, _ := runOK(t, "new", "-dir", tasksDir, "-issue", "42", "render is slow")
	if !strings.HasSuffix(strings.TrimSpace(stdout), "-render-is-slow.md") {
		t.Errorf("an explicit title must win over the issue title, got %q", stdout)
	}
}

func TestNewFromIssueFailsLoudly(t *testing.T) {
	noGH(t)
	_, tasksDir := newRepo(t)

	err := runErr(t, "new", "-dir", tasksDir, "-issue", "42")
	if !strings.Contains(err.Error(), "gh is not installed") {
		t.Errorf("want an explicit missing gh error, got %v", err)
	}
	// Nothing half created.
	matches, _ := filepath.Glob(filepath.Join(tasksDir, "*.md"))
	if len(matches) != 0 {
		t.Errorf("a failed seeding must not leave a task behind: %v", matches)
	}
}

func TestDigestIncludesTheIssueThread(t *testing.T) {
	fakeGH(t, `case "$*" in
*"--comments"*) echo "the thread as gh renders it" ;;
*) exit 1 ;;
esac`)
	root, tasksDir := newRepo(t)
	path := filepath.Join(tasksDir, "260806-t.md")
	mustWrite(t, path, "---\nstatus: todo\nissue: 42\n---\n\n# T\n\n## Context\n\nx\n")
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}

	var out, warn strings.Builder
	if err := digest(&out, &warn, root, task, digestOpts{logKeep: 10, maxFileBytes: 1024, withIssue: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## Issue #42") || !strings.Contains(out.String(), "the thread as gh renders it") {
		t.Errorf("the issue thread is missing:\n%s", out.String())
	}

	// Offline must not call gh at all.
	out.Reset()
	if err := digest(&out, &warn, root, task, digestOpts{logKeep: 10, maxFileBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "## Issue") {
		t.Error("offline must skip the issue thread")
	}
}

func TestDigestSurvivesAMissingGH(t *testing.T) {
	noGH(t)
	root, tasksDir := newRepo(t)
	path := filepath.Join(tasksDir, "260806-t.md")
	mustWrite(t, path, "---\nstatus: todo\nissue: 42\n---\n\n# T\n\n## Context\n\nstill useful\n")
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}

	var out, warn strings.Builder
	if err := digest(&out, &warn, root, task, digestOpts{logKeep: 10, maxFileBytes: 1024, withPRs: true, withIssue: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "still useful") {
		t.Error("a missing gh must not stop the digest")
	}
	if !strings.Contains(warn.String(), "gh is not installed") {
		t.Errorf("a missing gh must be reported: %q", warn.String())
	}
}
