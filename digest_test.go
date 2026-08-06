package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDigestTruncationIsVisible(t *testing.T) {
	root, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(root, "big.txt"), strings.Repeat("x", 100))
	mustWrite(t, filepath.Join(root, "small.txt"), "small\n")

	var b strings.Builder
	b.WriteString("---\nstatus: todo\nlinks: [big.txt, small.txt, gone.txt]\n---\n\n# T\n\n## Log\n\n")
	for i := range 20 {
		b.WriteString("- entry ")
		b.WriteByte(byte('a' + i))
		b.WriteString("\n")
	}
	path := filepath.Join(tasksDir, "260806-t.md")
	mustWrite(t, path, b.String())
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}

	var out, warn strings.Builder
	if err := digest(&out, &warn, root, task, digestOpts{logKeep: 3, maxFileBytes: 10}); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	if !strings.Contains(got, "## Log (last 3 of 20, the rest is in git)") {
		t.Errorf("the log truncation must be stated in the output:\n%s", got)
	}
	if strings.Contains(got, "- entry a") || !strings.Contains(got, "- entry t") {
		t.Error("the digest must keep the newest log entries")
	}
	if !strings.Contains(got, "big.txt (truncated to 10 of 100 bytes)") {
		t.Errorf("the file truncation must be stated in the output:\n%s", got)
	}
	if !strings.Contains(got, "small.txt\n") || strings.Contains(got, "small.txt (truncated") {
		t.Error("a file under the cap must not be reported as truncated")
	}
	if !strings.Contains(got, "gone.txt (unreadable") || !strings.Contains(warn.String(), "gone.txt") {
		t.Error("a missing linked file must be reported in both the output and the warnings")
	}
	if !strings.Contains(warn.String(), "3 linked files") {
		t.Errorf("the measurement line is missing: %q", warn.String())
	}
}

func TestDigestWithoutGit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "260806-t.md")
	mustWrite(t, path, "---\nstatus: todo\n---\n\n# T\n\n## Context\n\nStill useful.\n")
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}

	var out, warn strings.Builder
	if err := digest(&out, &warn, dir, task, digestOpts{logKeep: 10, maxFileBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Still useful.") {
		t.Error("the digest must work without git")
	}
	if !strings.Contains(warn.String(), "not a git repository") {
		t.Errorf("the missing repository must be reported: %q", warn.String())
	}
	if strings.Contains(out.String(), "## Commits") {
		t.Error("no repository means no commits section")
	}
}

func TestIDPrefix(t *testing.T) {
	for in, want := range map[string]string{
		"260806-migration-auth": "260806",
		"260806":                "260806",
	} {
		if got := idPrefix(in); got != want {
			t.Errorf("idPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
