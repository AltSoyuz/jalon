package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	_, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: doing\n---\n\n# Auth\n")
	mustWrite(t, filepath.Join(tasksDir, "260807-billing.md"), "---\nstatus: done\n---\n\n# Billing\n")
	mustWrite(t, filepath.Join(tasksDir, "260808-nostatus.md"), "---\ncreated: 2026-08-08\n---\n\n# No status\n")

	stdout, _ := runOK(t, "list", "-dir", tasksDir)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want one per task:\n%s", len(lines), stdout)
	}
	// Newest first, like the rendered index.
	for i, want := range []string{"260808-nostatus", "260807-billing", "260806-auth"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want it to start with %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[0], "todo") {
		t.Errorf("a missing status must display as todo: %q", lines[0])
	}
	if !strings.Contains(lines[2], "Auth") {
		t.Errorf("the title must be on the line: %q", lines[2])
	}

	// Every line must stay parsable as id, status, then the rest.
	for _, line := range lines {
		if len(strings.Fields(line)) < 3 {
			t.Errorf("line %q is not three whitespace separated fields", line)
		}
	}
}

func TestListFilterAndEmptyCase(t *testing.T) {
	_, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: doing\n---\n\n# Auth\n")
	mustWrite(t, filepath.Join(tasksDir, "260807-billing.md"), "---\nstatus: done\n---\n\n# Billing\n")

	stdout, _ := runOK(t, "list", "-dir", tasksDir, "-status", "doing")
	if strings.Count(stdout, "\n") != 1 || !strings.Contains(stdout, "260806-auth") {
		t.Fatalf("the filter kept the wrong tasks:\n%s", stdout)
	}

	// This is what a session start hook sees when nothing is in flight: empty
	// stdout, an explanation on stderr, and a success exit.
	stdout, stderr := runOK(t, "list", "-dir", tasksDir, "-status", "blocked")
	if stdout != "" {
		t.Errorf("stdout must stay empty when nothing matches, got %q", stdout)
	}
	if !strings.Contains(stderr, `no task with status "blocked"`) {
		t.Errorf("the empty case must be explained on stderr, got %q", stderr)
	}
}
