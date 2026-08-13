package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The agent layer's forge calls. github.go plays this role for the core, and
// this file deliberately repeats its shape rather than sharing code with it:
// package agent cannot import package main, and extracting the core's version
// into a third package would be a refactor of working code in service of new
// work. Two small wrappers are cheaper to own than that.
//
// Like the core's, the coupling is one file you can delete.

type issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

func fetchIssue(ctx context.Context, dir string, number int) (*issue, error) {
	res, err := run(ctx, runOpts{
		dir: dir, name: "gh", timeout: 60 * time.Second,
		args: []string{"issue", "view", fmt.Sprint(number), "--json", "number,title,body,url,state"},
	})
	if err != nil {
		return nil, err
	}
	var i issue
	if err := json.Unmarshal([]byte(res.stdout), &i); err != nil {
		return nil, fmt.Errorf("gh issue view %d returned unreadable json: %w", number, err)
	}
	if i.Number == 0 {
		i.Number = number
	}
	return &i, nil
}

// text is what a phase is given about the issue: enough to work from, with the
// body clearly bounded so a body full of markdown headings cannot be mistaken
// for the instructions around it.
func (i *issue) text() string {
	body := strings.TrimSpace(i.Body)
	if body == "" {
		body = "(the issue has no body)"
	}
	return fmt.Sprintf("Issue #%d: %s\n%s\n\n--- issue body starts ---\n%s\n--- issue body ends ---\n",
		i.Number, i.Title, i.URL, body)
}

// LabelMeasure is the label triage applies to an issue that needs a measured
// review, and the one `review -next` picks up. It exists before triage does so
// that a person can apply it by hand today and the timer has real work.
const LabelMeasure = "measure"

// nextIssue returns the oldest open issue labelled for review, or 0 when there
// is none. A timer that finds nothing to do has succeeded, not failed.
func nextIssue(ctx context.Context, dir string) (int, error) {
	res, err := run(ctx, runOpts{
		dir: dir, name: "gh", timeout: 60 * time.Second,
		args: []string{"issue", "list", "--label", LabelMeasure, "--state", "open",
			"--json", "number", "--limit", "50"},
	})
	if err != nil {
		return 0, err
	}
	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &issues); err != nil {
		return 0, fmt.Errorf("gh issue list returned unreadable json: %w", err)
	}
	oldest := 0
	for _, i := range issues {
		if oldest == 0 || i.Number < oldest {
			oldest = i.Number
		}
	}
	return oldest, nil
}

func createPR(ctx context.Context, dir, title, body string) (string, error) {
	res, err := run(ctx, runOpts{
		dir: dir, name: "gh", timeout: 2 * time.Minute,
		args: []string{"pr", "create", "--title", title, "--body", body},
	})
	if err != nil {
		return "", err
	}
	// gh prints the URL of the pull request it created, last line.
	lines := strings.Fields(strings.TrimSpace(res.stdout))
	if len(lines) == 0 {
		return "", nil
	}
	return lines[len(lines)-1], nil
}
