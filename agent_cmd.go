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
	live := fs.Bool("live", false, "spend one real model call to prove the model answers; costs money, so it is off by default")
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

	report := agent.Doctor(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr}, agent.Options{Live: *live})
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
	next := fs.Bool("next", false, "review the oldest open issue labelled "+agent.LabelMeasure+" (what the timer runs)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	var issue int
	switch {
	case *next && fs.NArg() > 0:
		return errors.New("review: -next picks the issue, so it takes no argument")
	case !*next && fs.NArg() != 1:
		return errors.New("review: usage: jalon review <issue number>, or jalon review -next")
	case !*next:
		var err error
		if issue, err = issueNumber(fs.Arg(0)); err != nil {
			return fmt.Errorf("review: %w", err)
		}
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signalCtx()
	defer stop()

	res, err := agent.Review(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr},
		agent.ReviewOptions{Issue: issue, Next: *next, TasksDir: d, KeepWorktree: *keep})
	m.ID = res.TaskID
	if res.Worktree != "" {
		fmt.Fprintf(stderr, "jalon: the review worktree is kept at %s; read its facts.md, then: git worktree remove --force %s\n",
			res.Worktree, res.Worktree)
	}
	return err
}

// cmdAgentInit writes the configuration, the systemd unit and the timer to
// stdout. It installs nothing: the output is meant to be read, then piped to sh
// or to a file. The privileged half is a marked block a person runs.
func cmdAgentInit(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("agent-init", stderr)
	repo := fs.String("repo", "", "absolute path of the repository the agent works in")
	user := fs.String("user", "jalon-agent", "the dedicated system user, which must have no sudo")
	port := fs.Int("port", 0, "local port of the app to probe, 0 for none")
	branch := fs.String("branch", "main", "the branch every review worktree is cut from")
	jalonRepo := fs.String("jalon", "", "absolute path of the jalon checkout the unit pulls, builds and runs (default: the target, which is only right when the target is jalon itself)")
	envFile := fs.String("env-file", "", "absolute path of a machine secrets file the unit reads, 0600 and outside the repository (for a CLAUDE_CODE_OAUTH_TOKEN); jalon references it, never writes it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	if err := agent.Init(stdout, version, agent.InitOptions{
		Repo: *repo, User: *user, Port: *port, Branch: *branch, Jalon: *jalonRepo, EnvFile: *envFile,
	}); err != nil {
		return fmt.Errorf("agent-init: %w", err)
	}
	return nil
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
