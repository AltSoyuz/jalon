package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type digestOpts struct {
	logKeep      int
	maxFileBytes int
	withPRs      bool
	withIssue    bool
}

// digest writes everything an agent needs about one task, in reading order:
// what it is, what was decided, what happened, the linked files, then the
// commits and pull requests that carry the id.
//
// The whole point is the count printed on stderr at the end: it makes the
// comparison against a forge free, since it falls out of normal usage.
func digest(stdout, stderr io.Writer, root string, t *Task, opt digestOpts) error {
	start := time.Now()
	var b bytes.Buffer

	fmt.Fprintf(&b, "# task %s\n", t.ID)
	for _, k := range t.FM.Keys() {
		fmt.Fprintf(&b, "%s: %s\n", k, t.FM.Get(k))
	}

	writeSection(&b, sectionContext, t.Section(sectionContext))
	writeSection(&b, sectionDecisions, t.Section(sectionDecisions))

	entries := t.Entries(sectionLog)
	shown := entries
	heading := sectionLog
	if opt.logKeep > 0 && len(entries) > opt.logKeep {
		shown = entries[len(entries)-opt.logKeep:]
		heading = fmt.Sprintf("%s (last %d of %d, the rest is in git)", sectionLog, opt.logKeep, len(entries))
	}
	writeSection(&b, heading, strings.Join(shown, "\n"))

	files := t.FM.Links()
	if len(files) > 0 {
		fmt.Fprintf(&b, "\n## Linked files\n")
		for _, rel := range files {
			writeLinkedFile(&b, stderr, root, rel, opt.maxFileBytes)
		}
	}

	nCommits := 0
	if !isGitRepo(root) {
		fmt.Fprintf(stderr, "jalon: %s is not a git repository: no commits, no pull requests\n", root)
	} else {
		out, err := gitOutput(root, "log", "--fixed-strings", "--grep", "["+t.ID+"]",
			"--grep", "["+idPrefix(t.ID)+"]", "--format=%h %ad %s", "--date=short")
		if err != nil {
			fmt.Fprintf(stderr, "jalon: git log failed: %v\n", err)
		} else if out != "" {
			nCommits = strings.Count(out, "\n") + 1
			fmt.Fprintf(&b, "\n## Commits\n\n%s\n", out)
		}
		if opt.withPRs {
			writePRs(&b, stderr, root, t.ID)
		}
	}

	// The issue thread is where the requirements were argued: for an agent it
	// is often worth more than the commits.
	if issue := strings.TrimSpace(t.FM.Get(fmKeyIssue)); issue != "" && opt.withIssue {
		writeIssue(&b, stderr, root, strings.TrimPrefix(issue, "#"))
	}

	n, err := stdout.Write(b.Bytes())
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "# digest %s: %d bytes, ~%d tokens (bytes/4), %d linked files, %d commits, %s\n",
		t.ID, n, EstimateTokens(n), len(files), nCommits, time.Since(start).Round(time.Millisecond))
	return nil
}

func writeSection(w io.Writer, heading, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	fmt.Fprintf(w, "\n## %s\n\n%s\n", heading, content)
}

// idPrefix is the YYMMDD part, so that a commit tagged with the short form
// [260806] is found as well as one tagged with the full id.
func idPrefix(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	return id
}

func writeLinkedFile(w io.Writer, stderr io.Writer, root, rel string, max int) {
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "jalon: linked file %s: %v\n", rel, err)
		fmt.Fprintf(w, "\n### %s (unreadable: %v)\n", rel, err)
		return
	}
	note := ""
	if max > 0 && len(data) > max {
		note = fmt.Sprintf(" (truncated to %d of %d bytes)", max, len(data))
		data = data[:max]
	}
	fmt.Fprintf(w, "\n### %s%s\n\n```\n%s\n```\n", rel, note, strings.TrimRight(string(data), "\n"))
}
