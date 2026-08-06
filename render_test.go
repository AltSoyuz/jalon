package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestMarkdownSubsetGolden(t *testing.T) {
	src, err := os.ReadFile("testdata/subset.md")
	if err != nil {
		t.Fatal(err)
	}
	got := mdToHTML(string(src))
	golden := "testdata/subset.html"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("mdToHTML differs from %s (run: go test -run TestMarkdownSubsetGolden -update)\n--- got ---\n%s", golden, got)
	}
}

func TestMarkdownNeverEmitsRawHTML(t *testing.T) {
	// The subset renderer owns its escaping: this is the property that keeps
	// hand written task files from becoming an injection vector.
	cases := []string{
		`<script>alert(1)</script>`,
		"`<script>alert(1)</script>`",
		`[x](javascript:alert)`,
		`[x](JaVaScRiPt:alert)`,
		`[x](data:text/html,<script>alert(1)</script>)`,
		`[x"onmouseover=alert(1)](https://a.b)`,
		"```\n<img src=x onerror=alert(1)>\n```",
		`- <iframe src="evil"></iframe>`,
		`# <svg onload=alert(1)>`,
	}
	for _, in := range cases {
		out := strings.ToLower(mdToHTML(in))
		// Escaped text may still read like an attribute; what must never appear
		// is an open tag or an unsafe href, which is what an escape failure
		// would produce.
		for _, bad := range []string{"<script", "<iframe", "<img", "<svg", `href="javascript`, `href="data`, `href="vbscript`} {
			if strings.Contains(out, bad) {
				t.Errorf("mdToHTML(%q) leaked %q:\n%s", in, bad, out)
			}
		}
	}
}

func TestSafeURL(t *testing.T) {
	for u, want := range map[string]bool{
		"../docs/a.md":        true,
		"https://example.com": true,
		"http://example.com":  true,
		"mailto:a@b.c":        true,
		"path/with:colon":     true,
		"javascript:alert(1)": false,
		"JAVASCRIPT:alert(1)": false,
		"data:text/html,x":    false,
		"vbscript:msgbox":     false,
	} {
		if got := safeURL(u); got != want {
			t.Errorf("safeURL(%q) = %v, want %v", u, got, want)
		}
	}
}

// newRepo builds a git repository with a .tasks directory, which is what every
// end to end test needs.
func newRepo(t *testing.T) (root, tasksDir string) {
	t.Helper()
	root = t.TempDir()
	tasksDir = filepath.Join(root, tasksDirName)
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"commit", "-q", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return root, tasksDir
}

func gitCommit(t *testing.T, root, message string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "--allow-empty", "-m", message)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}

func TestRenderSite(t *testing.T) {
	root, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"),
		"---\nstatus: todo\ncreated: 2026-08-06\n---\n\n# Auth\n\n## Context\n\nWork in progress.\n")
	mustWrite(t, filepath.Join(tasksDir, "260807-billing.md"),
		"---\nstatus: done\ncreated: 2026-08-07\n---\n\n# Billing\n")
	gitCommit(t, root, "[260806] fix the refresh token path")
	gitCommit(t, root, "[260806-auth] add the revocation table")
	gitCommit(t, root, "[260807] unrelated to auth")

	out := filepath.Join(root, "site")
	n, err := renderSite(io.Discard, tasksDir, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rendered %d tasks, want 2", n)
	}

	page := readFile(t, filepath.Join(out, "260806-auth.html"))
	for _, want := range []string{"fix the refresh token path", "add the revocation table", "Related commits", "Work in progress."} {
		if !strings.Contains(page, want) {
			t.Errorf("task page is missing %q", want)
		}
	}
	if strings.Contains(page, "unrelated to auth") {
		t.Error("a commit of another task leaked into the page")
	}

	index := readFile(t, filepath.Join(out, "index.html"))
	if !strings.Contains(index, ">todo</h2>") || !strings.Contains(index, ">done</h2>") {
		t.Error("index is not grouped by status")
	}
	if i, j := strings.Index(index, ">todo</h2>"), strings.Index(index, ">done</h2>"); i > j {
		t.Error("todo must come before done in the index")
	}
	if !strings.Contains(index, `href="260806-auth.html"`) {
		t.Error("index does not link the task page")
	}
}

func TestRenderSiteWithoutGit(t *testing.T) {
	// Degraded mode: no repository, no commits, a warning, and still a site.
	dir := t.TempDir()
	tasksDir := filepath.Join(dir, tasksDirName)
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: todo\n---\n\n# Auth\n")

	var warn strings.Builder
	if _, err := renderSite(&warn, tasksDir, filepath.Join(dir, "site")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warn.String(), "not a git repository") {
		t.Errorf("the missing repository must be reported, got %q", warn.String())
	}
	if page := readFile(t, filepath.Join(dir, "site", "260806-auth.html")); !strings.Contains(page, "Auth") {
		t.Error("the page must still be generated without git")
	}
}

func TestRenderSiteWarnsOnSharedPrefix(t *testing.T) {
	root, tasksDir := newRepo(t)
	mustWrite(t, filepath.Join(tasksDir, "260806-auth.md"), "---\nstatus: todo\n---\n\n# Auth\n")
	mustWrite(t, filepath.Join(tasksDir, "260806-billing.md"), "---\nstatus: todo\n---\n\n# Billing\n")
	gitCommit(t, root, "[260806] ambiguous on purpose")

	var warn strings.Builder
	if _, err := renderSite(&warn, tasksDir, filepath.Join(root, "site")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warn.String(), "share the prefix 260806") {
		t.Errorf("an ambiguous short id must be reported, got %q", warn.String())
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// BenchmarkRenderSite is the measurement behind "a full rewrite every time is
// cheap enough": 500 tasks of about 4 KB, which is more than any repo this tool
// targets. Run: make bench.
func BenchmarkRenderSite(b *testing.B) {
	dir := b.TempDir()
	tasksDir := filepath.Join(dir, tasksDirName)
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		b.Fatal(err)
	}
	var body strings.Builder
	for range 60 {
		body.WriteString("- 2026-08-06 gt: an entry with `code`, a [link](https://example.com) and some prose.\n")
	}
	for i := range 500 {
		content := fmt.Sprintf("---\nstatus: todo\ncreated: 2026-08-06\n---\n\n# Task %d\n\n## Context\n\nSome context.\n\n## Log\n\n%s", i, body.String())
		path := filepath.Join(tasksDir, fmt.Sprintf("2608%02d-task-%d.md", i%28+1, i))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := renderSite(io.Discard, tasksDir, filepath.Join(dir, "site")); err != nil {
			b.Fatal(err)
		}
	}
}
