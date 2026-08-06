package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// This file is the only place that shells out to `gh`, so the whole coupling
// to a forge is one file you can delete.
//
// The bridge is deliberately one way and stateless: a GitHub issue can seed a
// task, and a task can show its issue, but nothing synchronizes. Two writable
// stores would need a token, a mapping table, a conflict rule and a daemon, and
// would break the three properties that make this tool worth using: the files
// are the truth, nothing hits the network on the hot path, and `cat` plus
// `git log` are enough to debug it.
//
// Closing needs no code at all: a pull request body holding both
// "closes #42" and "closes 260806" closes the issue through GitHub and the task
// through the post-merge hook, each by its own mechanism.

// fmKeyIssue is the front matter key linking a task to a GitHub issue.
const fmKeyIssue = "issue"

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

func ghAvailable() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh is not installed (see https://cli.github.com): %w", err)
	}
	return nil
}

func ghRun(dir string, args ...string) ([]byte, error) {
	if err := ghAvailable(); err != nil {
		return nil, err
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// fetchIssue reads one issue. It is called once, by `jalon new -issue`, and
// never again for that task: what lands in the file is a snapshot, and the file
// is what the tool reads afterwards.
func fetchIssue(dir string, number int) (*ghIssue, error) {
	out, err := ghRun(dir, "issue", "view", fmt.Sprint(number), "--json", "number,title,body,url,state")
	if err != nil {
		return nil, err
	}
	var issue ghIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("gh issue view %d returned unreadable json: %w", number, err)
	}
	if issue.Number == 0 {
		issue.Number = number
	}
	return &issue, nil
}

// writeIssue appends the issue thread to a digest. Best effort: a missing gh,
// a missing forge or a network failure is reported and never fatal.
func writeIssue(w io.Writer, stderr io.Writer, root, number string) {
	out, err := ghRun(root, "issue", "view", number, "--comments")
	if err != nil {
		fmt.Fprintf(stderr, "jalon: issue %s: %v\n", number, err)
		return
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		fmt.Fprintf(w, "\n## Issue #%s\n\n%s\n", number, text)
	}
}

// writePRs lists the open pull requests carrying the id.
func writePRs(w io.Writer, stderr io.Writer, root, id string) {
	out, err := ghRun(root, "pr", "list", "--search", idPrefix(id), "--state", "open", "--limit", "20")
	if err != nil {
		fmt.Fprintf(stderr, "jalon: no pull requests: %v\n", err)
		return
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		fmt.Fprintf(w, "\n## Open pull requests\n\n%s\n", text)
	}
}
