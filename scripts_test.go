package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The weekly recap is a shell script, so its test is a run: a repository with
// a stale doing task, an agreed task nobody queued, a wreck and a closed task
// whose linked file moved, against a forge stub that has nothing open and
// nothing merged. What is asserted is that every section that names a
// decision for a person is there.
func TestWeeklyRecapScript(t *testing.T) {
	for _, tool := range []string{"jq", "git", "sh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for this test", tool)
		}
	}
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "jalon"), ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@example.invalid")
	git("config", "user.name", "t")
	git("remote", "add", "origin", "git@github.com:example/target.git")
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	task := func(id, status, links, log string) {
		write(".tasks/"+id+".md", "---\nstatus: "+status+"\ncreated: 2026-01-01\nlinks: ["+links+"]\n---\n\n# "+id+"\n\n## Context\n\nx\n\n## Decisions\n\n## Log\n"+log)
	}
	task("250101-stale-doing", "doing", "", "")
	task("260101-agreed-and-forgotten", "proposed", "", "")
	task("260102-queued", "implement", "", "")
	task("240101-closed-long-ago", "done", "moved.go", "- 2024-01-02 t: closed.\n")
	write("moved.go", "package x\n")
	git("add", "-A")
	git("commit", "-q", "-m", "seed")
	write("moved.go", "package x // moved since the task closed\n")
	git("commit", "-q", "-am", "move the ground")
	write(".jalon/failed/work-260102-queued/x", "")

	metrics := filepath.Join(t.TempDir(), "m.jsonl")
	if err := os.WriteFile(metrics, []byte(`{"time":"2099-01-01T00:00:00Z","verb":"work","id":"a","cost_usd":1.5}`+"\n"+
		`{"time":"2099-01-01T00:00:00Z","verb":"review","id":"b","err":"boom"}`+"\n"+
		// A tick that found nothing queued: no id, and not a job.
		`{"time":"2099-01-01T00:00:00Z","verb":"work","ms":3}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notified := filepath.Join(t.TempDir(), "notified.md")

	cmd := exec.Command("sh", "scripts/weekly-recap.sh", "-days", "3650", "-metrics", metrics,
		"-notify", "cat > "+notified, root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("recap: %v:\n%s", err, out)
	}
	for _, want := range []string{
		"## " + filepath.Base(root),
		"- open: 1 doing, 0 todo",
		"doing for ", // the stale one, with its age
		"250101-stale-doing",
		"- proposed, not queued: 260101-agreed-and-forgotten",
		"- queued implement: 260102-queued",
		"- wreck to read: .jalon/failed/work-260102-queued",
		"- decision ground moved: 240101-closed-long-ago closed 2024-01-02, 2 commit(s) since on moved.go",
		"- work branches merged: none yet",
		"- agent jobs: 2 (1 published, 1 failed)",
		"- cost: 1.5 USD reported (1 job(s) reported none)",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the recap lacks %q:\n%s", want, out)
		}
	}
	// The notify command got the same text.
	b, rerr := os.ReadFile(notified)
	if rerr != nil || !strings.Contains(string(b), "- open: 1 doing, 0 todo") {
		t.Errorf("the notify command did not receive the recap: %v %q", rerr, b)
	}
}
