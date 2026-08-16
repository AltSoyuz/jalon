package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The recap is a read of what waits on a person, so its test is a fixture:
// a stale doing task, an agreed task nobody queued, a queued one, a wreck, a
// closed task whose linked file moved, and a metrics file with two jobs and
// an empty tick, against a forge stub that has nothing open and nothing
// merged. What is asserted is that every line naming a decision is there.
func TestRecap(t *testing.T) {
	// The real binary for jalon list, built before PATH is narrowed.
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "jalon"), "github.com/AltSoyuz/jalon").CombinedOutput(); err != nil {
		t.Skipf("cannot build jalon for the recap test: %v: %s", err, out)
	}
	s := newStubs(t)
	if err := os.Rename(filepath.Join(bin, "jalon"), filepath.Join(s.dir, "jalon")); err != nil {
		t.Fatal(err)
	}
	s.add(t, "gh", `echo '[]'`)

	root := newRepo(t, "")
	// A forge-shaped origin, so the pull request sections are exercised.
	mustGit(t, root, "remote", "set-url", "origin", "git@github.com:example/target.git")
	task := func(id, status, links, log string) {
		mustWrite(t, filepath.Join(root, ".tasks", id+".md"),
			"---\nstatus: "+status+"\ncreated: 2026-01-01\nlinks: ["+links+"]\n---\n\n# "+id+"\n\n## Context\n\nx\n\n## Decisions\n\n## Log\n"+log)
	}
	task("250101-stale-doing", "doing", "", "")
	task("260101-agreed-and-forgotten", "proposed", "", "")
	task("260102-queued", StatusImplement, "", "")
	task("240101-closed-long-ago", "done", "moved.go", "- 2024-01-02 t: closed.\n")
	mustWrite(t, filepath.Join(root, "moved.go"), "package x\n")
	mustGit(t, root, "add", "-A")
	mustGit(t, root, "commit", "-q", "-m", "seed")
	mustWrite(t, filepath.Join(root, "moved.go"), "package x // moved since the task closed\n")
	mustGit(t, root, "commit", "-q", "-am", "move the ground")
	mustWrite(t, filepath.Join(root, ".jalon", "failed", "work-260102-queued", "x"), "")

	metrics := filepath.Join(t.TempDir(), "m.jsonl")
	mustWrite(t, metrics, `{"time":"2026-08-10T00:00:00Z","verb":"work","id":"a","cost_usd":1.5}`+"\n"+
		`{"time":"2026-08-10T00:00:00Z","verb":"review","id":"b","err":"boom"}`+"\n"+
		`{"time":"2026-08-10T00:00:00Z","verb":"work","ms":3}`+"\n"+ // an empty tick, not a job
		`{"time":"2026-01-01T00:00:00Z","verb":"work","id":"old","cost_usd":9}`+"\n") // outside the window
	notified := filepath.Join(t.TempDir(), "notified.md")

	var out, errb strings.Builder
	err := Recap(context.Background(), Env{Stdout: &out, Stderr: &errb}, RecapOptions{
		// Shell builtins only: PATH is the stub directory here.
		Days: 7, Metrics: metrics, Notify: "while IFS= read -r l; do printf '%s\\n' \"$l\"; done > " + notified, Repos: []string{root},
		Now: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("recap: %v\n%s", err, errb.String())
	}
	got := out.String()
	for _, want := range []string{
		"## " + filepath.Base(root),
		"- open: 1 doing, 0 todo",
		"  - doing for 592 days: 250101-stale-doing",
		"- proposed, not queued: 260101-agreed-and-forgotten (260101-agreed-and-forgotten)",
		"- queued implement: 260102-queued",
		"- wreck to read: .jalon/failed/work-260102-queued",
		"- decision ground moved: 240101-closed-long-ago closed 2024-01-02, 2 commit(s) since on moved.go",
		"- work branches merged: none yet",
		"- agent jobs: 2 (1 published, 1 failed)",
		"- cost: 1.50 USD reported (1 job(s) reported none)",
		"- per verb: review 1, work 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the recap lacks %q:\n%s", want, got)
		}
	}
	// The notify command got the same text.
	if b, rerr := os.ReadFile(notified); rerr != nil || string(b) != got {
		t.Errorf("the notify command did not receive the recap: %v", rerr)
	}
	// The forge was asked, once per list, and never for a task's content.
	if n := len(s.of(t, "gh")); n != 2 {
		t.Errorf("gh was called %d times, want 2 (open and merged lists):\n%s", n, strings.Join(s.of(t, "gh"), "\n"))
	}
}
