package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/AltSoyuz/jalon/agent"
)

// This file is the only bridge to the agent package, and it holds no logic: it
// parses flags with the same helpers every other verb uses, resolves the
// directories the core knows how to resolve, and hands them over as plain
// values.
//
// The boundary is the design. package agent cannot import package main, and
// TestCoreDoesNotImportAgent refuses the other direction, so deleting this file
// and the agent directory leaves a working task manager.

// signalCtx cancels on the first interrupt so a job in flight stops at the next
// process boundary rather than being killed mid write, and restores the default
// behaviour on the second one.
func signalCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func cmdDoctor(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("doctor", stderr)
	dir := dirFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signalCtx()
	defer stop()

	report := agent.Doctor(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr})
	m.Checks = len(report.Checks)
	if n := report.Failed(); n > 0 {
		return fmt.Errorf("doctor: %d of %d checks failed; the fix for each is on stderr above", n, len(report.Checks))
	}
	return nil
}

func cmdReview(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("review", stderr)
	dir := dirFlag(fs)
	keep := fs.Bool("keep-worktree", false, "keep the worktree even when the review succeeds, to inspect what the model saw")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("review: usage: jalon review <issue number>")
	}
	issue, err := issueNumber(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signalCtx()
	defer stop()

	res, err := agent.Review(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr},
		agent.ReviewOptions{Issue: issue, TasksDir: d, KeepWorktree: *keep})
	m.ID = res.TaskID
	if res.Worktree != "" {
		fmt.Fprintf(stderr, "jalon: the review worktree is kept at %s; read its facts.md, then: git worktree remove --force %s\n",
			res.Worktree, res.Worktree)
	}
	return err
}

func issueNumber(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%q is not an issue number", s)
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return 0, fmt.Errorf("%q is not an issue number", s)
	}
	return n, nil
}
