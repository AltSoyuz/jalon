package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTask(t *testing.T, content string) *Task {
	t.Helper()
	path := filepath.Join(t.TempDir(), "260806-x.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	task, err := ParseTask(path)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestParseTaskRoundTrip(t *testing.T) {
	// An unknown key and a hand written body must survive a rewrite untouched.
	src := `---
status: doing
created: 2026-08-06
links: [a.go, b.md]
owner: gt
---

# Title

## Context

Some context.

## Log

- 2026-08-06 gt: started.
`
	task := writeTask(t, src)
	if got := string(task.Bytes()); got != src {
		t.Fatalf("round trip changed the file:\n--- got ---\n%s\n--- want ---\n%s", got, src)
	}
	if got := task.FM.Get("owner"); got != "gt" {
		t.Errorf("unknown key lost: owner = %q", got)
	}
	if got := task.Status(); got != "doing" {
		t.Errorf("Status() = %q, want doing", got)
	}
	if got := task.Title(); got != "Title" {
		t.Errorf("Title() = %q, want Title", got)
	}
	if got, want := task.FM.Links(), []string{"a.go", "b.md"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Links() = %v, want %v", got, want)
	}
	if got := task.Section(sectionContext); got != "Some context." {
		t.Errorf("Section(Context) = %q", got)
	}
}

func TestParseFrontMatterErrors(t *testing.T) {
	for name, src := range map[string]string{
		"no front matter":  "# Title\n",
		"unterminated":     "---\nstatus: todo\n",
		"not a key: value": "---\nstatus\n---\n",
	} {
		if _, _, err := parseFrontMatter(src); err == nil {
			t.Errorf("%s: want an error, got nil", name)
		}
	}
}

func TestSectionIgnoresHeadingsInFences(t *testing.T) {
	task := writeTask(t, "---\nstatus: todo\n---\n\n## Context\n\n```\n## Log\nnot a section\n```\n\nstill context.\n\n## Log\n\n- entry\n")
	got := task.Section(sectionContext)
	if !strings.Contains(got, "still context.") {
		t.Fatalf("Context section stopped at a fenced heading: %q", got)
	}
	if entries := task.Entries(sectionLog); len(entries) != 1 {
		t.Fatalf("Entries(Log) = %v, want one entry", entries)
	}
}

func TestAppendEntry(t *testing.T) {
	task := writeTask(t, "---\nstatus: todo\n---\n\n# T\n\n## Log\n\n## Decisions\n\n- 2026-08-01 gt: first.\n")

	if err := task.AppendEntry(sectionLog, "- 2026-08-06 gt: into an empty section."); err != nil {
		t.Fatal(err)
	}
	if err := task.AppendEntry(sectionDecisions, "- 2026-08-06 gt: after an existing entry."); err != nil {
		t.Fatal(err)
	}
	want := "---\nstatus: todo\n---\n\n# T\n\n## Log\n\n- 2026-08-06 gt: into an empty section.\n\n## Decisions\n\n- 2026-08-01 gt: first.\n- 2026-08-06 gt: after an existing entry.\n"
	if got := string(task.Bytes()); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if err := task.AppendEntry("Nope", "- x"); err == nil {
		t.Error("appending to a missing section must fail")
	}
}

func TestTruncateSection(t *testing.T) {
	var b strings.Builder
	b.WriteString("---\nstatus: todo\n---\n\n## Log\n\n")
	for i := range 12 {
		b.WriteString("- entry ")
		b.WriteByte(byte('a' + i))
		b.WriteString("\n")
	}
	task := writeTask(t, b.String())

	removed, err := task.TruncateSection(sectionLog, 10)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	entries := task.Entries(sectionLog)
	if len(entries) != 11 { // 10 kept plus the marker
		t.Fatalf("len(entries) = %d, want 11", len(entries))
	}
	if !strings.Contains(entries[0], "2 earlier entries truncated") {
		t.Errorf("first entry is not the marker: %q", entries[0])
	}
	if !strings.Contains(entries[0], "git log") {
		t.Errorf("the marker must point at git: %q", entries[0])
	}
	if entries[len(entries)-1] != "- entry l" {
		t.Errorf("last entry = %q, want the newest one", entries[len(entries)-1])
	}

	removed, err = task.TruncateSection(sectionLog, 100)
	if err != nil || removed != 0 {
		t.Fatalf("truncating under the threshold: removed = %d, err = %v", removed, err)
	}
}

func TestTruncateSectionDropsWrappedEntries(t *testing.T) {
	task := writeTask(t, "---\nstatus: todo\n---\n\n## Log\n\n- old entry\n  wrapped over two lines\n- new entry\n")
	if _, err := task.TruncateSection(sectionLog, 1); err != nil {
		t.Fatal(err)
	}
	if got := string(task.Bytes()); strings.Contains(got, "wrapped over two lines") {
		t.Fatalf("the continuation line of a dropped entry stayed behind:\n%s", got)
	}
}

func TestSlugAndID(t *testing.T) {
	for in, want := range map[string]string{
		"migration auth":           "migration-auth",
		"  Créé: refresh Token!  ": "cree-refresh-token",
		"a very long title that goes past the forty character limit": "a-very-long-title-that-goes-past-the-for",
		"---": "",
		"":    "",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}

	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	id, err := NewID(now, "migration auth")
	if err != nil {
		t.Fatal(err)
	}
	if id != "260806-migration-auth" {
		t.Errorf("NewID = %q", id)
	}
	if _, err := NewID(now, "***"); err == nil {
		t.Error("a title with no usable character must fail")
	}
}

func TestCreateTaskRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if _, err := CreateTask(dir, "260806-x", "X", statusTodo, now); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTask(dir, "260806-x", "X", statusTodo, now); err == nil {
		t.Fatal("creating the same id twice must fail")
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(4000); got != 1000 {
		t.Errorf("EstimateTokens(4000) = %d, want 1000", got)
	}
}
