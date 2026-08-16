package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	next := fs.Bool("next", false, "review the oldest task with status: "+agent.StatusMeasure+" on origin (what the timer runs)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	switch {
	case *next && fs.NArg() > 0:
		return errors.New("review: -next picks the task, so it takes no argument")
	case !*next && fs.NArg() != 1:
		return errors.New("review: usage: jalon review <task id>, or jalon review -next")
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	var id string
	if !*next {
		// The same resolution every other verb uses, so a prefix works here too.
		t, terr := resolveTask(d, fs.Arg(0))
		if terr != nil {
			return fmt.Errorf("review: %w", terr)
		}
		id = t.ID
	}
	ctx, stop := signalCtx()
	defer stop()

	res, err := agent.Review(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr},
		agent.ReviewOptions{TaskID: id, Next: *next, KeepWorktree: *keep})
	m.ID = res.TaskID
	m.Cost = max(res.Cost, 0)
	if res.Worktree != "" {
		fmt.Fprintf(stderr, "jalon: the review worktree is kept at %s; read its facts.md, then: git worktree remove --force %s\n",
			res.Worktree, res.Worktree)
	}
	return err
}

func cmdWork(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("work", stderr)
	dir := dirFlag(fs)
	keep := fs.Bool("keep-worktree", false, "keep the worktree even when the work succeeds, to inspect what the model saw")
	next := fs.Bool("next", false, "implement the oldest task with status: "+agent.StatusImplement+" on origin (what the timer runs)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	switch {
	case *next && fs.NArg() > 0:
		return errors.New("work: -next picks the task, so it takes no argument")
	case !*next && fs.NArg() != 1:
		return errors.New("work: usage: jalon work <task id>, or jalon work -next")
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	var id string
	if !*next {
		// The same resolution every other verb uses, so a prefix works here too.
		t, terr := resolveTask(d, fs.Arg(0))
		if terr != nil {
			return fmt.Errorf("work: %w", terr)
		}
		id = t.ID
	}
	ctx, stop := signalCtx()
	defer stop()

	res, err := agent.Work(ctx, agent.Env{Root: repoRoot(d), Stdout: stdout, Stderr: stderr},
		agent.WorkOptions{TaskID: id, Next: *next, KeepWorktree: *keep})
	m.ID = res.TaskID
	m.Cost = max(res.Cost, 0)
	if res.Worktree != "" {
		fmt.Fprintf(stderr, "jalon: the work worktree is kept at %s; read its diff and .jalon-work/work.md, then: git worktree remove --force %s\n",
			res.Worktree, res.Worktree)
	}
	return err
}

// cmdRecap prints the weekly read of what waits on a person, over several
// repositories on this machine, and hands it to one command if asked.
func cmdRecap(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("recap", stderr)
	days := fs.Int("days", 7, "the window, in days")
	metrics := fs.String("metrics", os.Getenv(metricsEnv), "the JALON_METRICS file to read the week's jobs and cost from")
	notify := fs.String("notify", "", "a command that receives the recap on stdin; absent, stdout only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("recap: usage: jalon recap [-days N] [-metrics FILE] [-notify CMD] <one or more repository roots>")
	}
	ctx, stop := signalCtx()
	defer stop()
	return agent.Recap(ctx, agent.Env{Root: "", Stdout: stdout, Stderr: stderr},
		agent.RecapOptions{Days: *days, Metrics: *metrics, Notify: *notify, Repos: fs.Args()})
}

// cmdCapture turns the lines of an ntfy inbox topic into task stubs in the
// repositories they name.
func cmdCapture(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("capture", stderr)
	inbox := fs.String("inbox", "", "the ntfy topic URL to read, e.g. https://ntfy.example/inbox")
	token := fs.String("token", os.Getenv("NTFY_TOKEN"), "bearer token that may read the topic (default $NTFY_TOKEN)")
	home, _ := os.UserHomeDir()
	cursor := fs.String("cursor", filepath.Join(home, ".jalon-inbox-cursor"), "file holding the id of the last message handled")
	notify := fs.String("notify", "", "a command that receives what could not be routed, on stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("capture: usage: jalon capture -inbox URL [-cursor FILE] [-notify CMD] <one or more repository roots>")
	}
	ctx, stop := signalCtx()
	defer stop()
	res, err := agent.Capture(ctx, agent.Env{Stdout: stdout, Stderr: stderr},
		agent.CaptureOptions{Inbox: *inbox, Token: *token, Cursor: *cursor, Notify: *notify, Repos: fs.Args()})
	m.Tasks = len(res.Captured)
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
