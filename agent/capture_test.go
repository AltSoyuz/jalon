package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The inbox is an ntfy topic; here it is a small server that speaks the same
// JSON lines and remembers what since= it was asked. Three messages: one for
// a known repository, one directive with the bang, one without a home.
func TestCapture(t *testing.T) {
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "jalon"), "github.com/AltSoyuz/jalon").CombinedOutput(); err != nil {
		t.Skipf("cannot build jalon for the capture test: %v: %s", err, out)
	}
	s := newStubs(t)
	if err := os.Rename(filepath.Join(bin, "jalon"), filepath.Join(s.dir, "jalon")); err != nil {
		t.Fatal(err)
	}
	s.add(t, "gh", `echo "https://example.invalid/pull/9"`)

	var asked []string
	inbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("since"))
		if r.Header.Get("Authorization") != "Bearer tk_test" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("since") == "m3" {
			return // nothing after the last one
		}
		fmt.Fprintln(w, `{"id":"m1","time":1786890000,"event":"message","topic":"inbox","message":"repo: allow guests to rate a run"}`)
		fmt.Fprintln(w, `{"id":"m2","time":1786890060,"event":"message","topic":"inbox","message":"Repo!: remove the neosync entry"}`)
		fmt.Fprintln(w, `{"id":"m3","time":1786890120,"event":"message","topic":"inbox","message":"buy milk"}`)
	}))
	defer inbox.Close()

	root := newRepo(t, reviewTOML) // the fixture's base name is "repo"
	cursor := filepath.Join(t.TempDir(), "cursor")
	notified := filepath.Join(t.TempDir(), "notified.txt")
	var out, errb strings.Builder
	res, err := Capture(context.Background(), Env{Stdout: &out, Stderr: &errb}, CaptureOptions{
		Inbox: inbox.URL + "/inbox", Token: "tk_test", Cursor: cursor,
		Notify: "while IFS= read -r l; do printf '%s\\n' \"$l\"; done > " + notified,
		Repos:  []string{root},
	})
	if err != nil {
		t.Fatalf("capture: %v\n%s", err, errb.String())
	}
	if len(res.Captured) != 2 || len(res.Unrouted) != 1 {
		t.Fatalf("captured %d, unrouted %d, want 2 and 1:\n%s", len(res.Captured), len(res.Unrouted), out.String())
	}
	if res.Captured[0].Status != StatusMeasure || res.Captured[1].Status != StatusImplement {
		t.Errorf("statuses = %s, %s; want measure then implement (the bang)", res.Captured[0].Status, res.Captured[1].Status)
	}
	// The stubs are on origin's main, with the status the line asked for, and
	// the local checkout was never touched.
	for _, c := range res.Captured {
		file := gitOut(t, root, "show", "origin/main:.tasks/"+c.ID+".md")
		if !strings.Contains(file, "\nstatus: "+c.Status+"\n") {
			t.Errorf("%s on origin lacks status %s:\n%s", c.ID, c.Status, file)
		}
		if !strings.Contains(file, "captured from the inbox") {
			t.Errorf("%s carries no capture line:\n%s", c.ID, file)
		}
		if _, err := os.Stat(filepath.Join(root, ".tasks", c.ID+".md")); !os.IsNotExist(err) {
			t.Errorf("%s was written into the local checkout", c.ID)
		}
		if c.PR != "" {
			t.Errorf("a pull request was opened although main accepted the push: %s", c.PR)
		}
	}
	// The line without a home came back, and was not invented into a task.
	b, _ := os.ReadFile(notified)
	if !strings.Contains(string(b), `no repository in: "buy milk"`) || !strings.Contains(string(b), "known: repo") {
		t.Errorf("the unrouted line was not sent back: %q", b)
	}
	if n := len(strings.Fields(gitOut(t, root, "ls-tree", "--name-only", "origin/main", ".tasks/"))); n != 3 {
		t.Errorf("origin holds %d files under .tasks, want the two stubs and .gitkeep", n)
	}
	// The cursor is the last message, and the next run asks from there and
	// does nothing.
	if c, _ := os.ReadFile(cursor); strings.TrimSpace(string(c)) != "m3" {
		t.Errorf("cursor = %q, want m3", c)
	}
	res, err = Capture(context.Background(), Env{Stdout: &out, Stderr: &errb}, CaptureOptions{
		Inbox: inbox.URL + "/inbox", Token: "tk_test", Cursor: cursor, Repos: []string{root},
	})
	if err != nil || len(res.Captured) != 0 {
		t.Errorf("second run: err %v, captured %d, want nothing", err, len(res.Captured))
	}
	if len(asked) != 2 || asked[0] != "all" || asked[1] != "m3" {
		t.Errorf("since asked = %v, want [all m3]", asked)
	}
	// No worktree left behind.
	if entries, _ := os.ReadDir(filepath.Join(root, ".jalon", "worktrees")); len(entries) != 0 {
		t.Errorf("worktrees left behind: %v", entries)
	}
}

// A protected default branch refuses the push; the stub then goes to a
// task/<id> branch with a pull request, and is never lost.
func TestCaptureOnAProtectedBranchOpensAPullRequest(t *testing.T) {
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "jalon"), "github.com/AltSoyuz/jalon").CombinedOutput(); err != nil {
		t.Skipf("cannot build jalon for the capture test: %v: %s", err, out)
	}
	s := newStubs(t)
	if err := os.Rename(filepath.Join(bin, "jalon"), filepath.Join(s.dir, "jalon")); err != nil {
		t.Fatal(err)
	}
	s.add(t, "gh", `echo "https://example.invalid/pull/9"`)
	inbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"id":"m1","time":1786890000,"event":"message","topic":"inbox","message":"repo: a stub"}`)
	}))
	defer inbox.Close()
	root := newRepo(t, reviewTOML)
	// A pre-receive hook on the bare origin that refuses main, which is what a
	// protected branch looks like to git.
	origin := strings.TrimSpace(gitOut(t, root, "remote", "get-url", "origin"))
	mustWrite(t, filepath.Join(origin, "hooks", "pre-receive"), "#!/bin/sh\nwhile read old new ref; do [ \"$ref\" = refs/heads/main ] && { echo protected >&2; exit 1; }; done; exit 0\n")
	if err := os.Chmod(filepath.Join(origin, "hooks", "pre-receive"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out, errb strings.Builder
	res, err := Capture(context.Background(), Env{Stdout: &out, Stderr: &errb}, CaptureOptions{
		Inbox: inbox.URL + "/inbox", Cursor: filepath.Join(t.TempDir(), "c"), Repos: []string{root},
	})
	if err != nil {
		t.Fatalf("capture: %v\n%s", err, errb.String())
	}
	if len(res.Captured) != 1 || res.Captured[0].PR != "https://example.invalid/pull/9" {
		t.Fatalf("captured = %+v, want one stub with a pull request", res.Captured)
	}
	if out := gitOut(t, root, "ls-remote", "--heads", "origin", "capture/*"); !strings.Contains(out, "capture/"+res.Captured[0].ID) {
		t.Errorf("the stub branch is not on origin:\n%s", out)
	}
}
