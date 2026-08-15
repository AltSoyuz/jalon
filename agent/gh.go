package agent

import (
	"context"
	"strings"
	"time"
)

// The agent layer's one forge call. github.go plays this role for the core, and
// this file deliberately repeats its shape rather than sharing code with it:
// package agent cannot import package main, and extracting the core's version
// into a third package would be a refactor of working code in service of new
// work. One small wrapper is cheaper to own than that.
//
// Everything else the layer needs from the forge, it gets through git: the
// queue is read from FETCH_HEAD and left by pushing a branch. Like the core's,
// the coupling is one file you can delete.

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
