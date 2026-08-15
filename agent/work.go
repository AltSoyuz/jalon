package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WorkOptions struct {
	TaskID       string
	Next         bool // take the oldest task queued with status: implement on origin
	KeepWorktree bool
}

type WorkResult struct {
	TaskID   string
	PR       string
	Cost     float64 // USD; -1 when the phase did not report it
	Worktree string  // set only when the worktree was kept, for inspection
}

// workDir holds what the phase printed, inside the worktree, beside the code it
// wrote. Never committed.
const workDir = ".jalon-work"

// Work implements one already agreed task, behind the repository's own criterion.
//
// The split with Review is the point. Review is for an idea you doubt: it
// measures, tries to refute, and proposes. Work is for a task you have already
// agreed to by merging it, and it writes code.
//
// It never takes an issue as the thing to build: the merged task is the
// agreement, and -next takes the oldest task a person queued by setting its
// status to implement. That is a button a person pressed, delivered by the
// timer; jalon never chooses what to implement.
//
// Where Review needed Go code to check that the model had measured, because that
// cannot be verified mechanically, Work has something better and already written:
// the criterion either exits 0 or it does not.
func Work(ctx context.Context, env Env, opt WorkOptions) (WorkResult, error) {
	var res WorkResult

	// The queue is read before the preflight: Doctor runs the repository's
	// criterion in full, and a tick that finds nothing queued would otherwise
	// pay for a test suite and throw the result away. Load is what Doctor
	// would do first anyway.
	cfg, err := Load(env.Root)
	if err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	if err := fetchDefault(ctx, env.Root, cfg); err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	if opt.Next {
		id, err := nextTask(ctx, env.Root, StatusImplement, "work", "work")
		if err != nil {
			return res, fmt.Errorf("work: cannot read the queue: %w", err)
		}
		if id == "" {
			// An empty queue is the normal state of a timer between jobs, and
			// a unit that failed hourly on it would train everyone to ignore it.
			fmt.Fprintf(env.Stderr, "jalon: no task with status: %s on origin %s that is not already published or parked, nothing to implement\n", StatusImplement, cfg.Review.DefaultBranch)
			return res, nil
		}
		opt.TaskID = id
	}
	task, err := readTask(ctx, env.Root, opt.TaskID)
	if err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	res.TaskID = opt.TaskID

	// Doctor runs the criterion before the model touches anything, which is what
	// makes the same criterion meaningful afterwards: a red result at the end is
	// attributable to this job rather than to a base that was already broken.
	report := Doctor(ctx, env, Options{})
	if n := report.Failed(); n > 0 {
		return res, fmt.Errorf("work: %d of %d preflight checks failed; a red base gets no job, the fix for each is on stderr above", n, len(report.Checks))
	}
	cfg = report.Config

	if err := takeJobSlot(env.Root, cfg); err != nil {
		return res, fmt.Errorf("work: %w", err)
	}

	wt, err := newWorktree(ctx, env.Root, cfg, "work-"+opt.TaskID)
	if err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	// Parked out of the way, and out of the queue for as long as the wreck is
	// there: one failure must cost one item, not the night, and a failing task
	// is retried by a person and not by the clock.
	keep := func(err error) (WorkResult, error) {
		res.Worktree = failJob(ctx, env, cfg, wt, err)
		return res, err
	}
	if _, err := materializeSkills(wt.path); err != nil {
		return keep(fmt.Errorf("work: %w", err))
	}
	if err := os.MkdirAll(filepath.Join(wt.path, workDir), 0o755); err != nil {
		return keep(fmt.Errorf("work: %w", err))
	}

	// One phase. The model may edit the checkout and run the listed commands,
	// which is how it iterates on the criterion inside its own invocation. jalon
	// runs no loop of its own: a retry here would be an unbounded cost with no
	// bound on quality.
	//
	// Edit and not Write: Claude Code matches file permissions against Edit rules
	// only, and one Edit rule covers every file-editing tool.
	phaseRes, perr := runPhase(ctx, cfg, wt.path, phase{
		name:  "work",
		skill: "jalon-work",
		tools: []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"},
		allow: append(probeTools(cfg.Probes), "Read", "Grep", "Glob", "Edit"),
		stdin: task + probeList(cfg) +
			"\n--- the criterion this work must satisfy ---\n" + cfg.Criterion + "\n",
		prompt: "Implement the task on stdin, following the jalon-work method. " +
			"Change nothing the task did not ask for, and make the criterion pass.",
	})
	res.Cost = phaseRes.cost
	if werr := os.WriteFile(filepath.Join(wt.path, workDir, "work.md"), []byte(phaseRes.out), 0o644); werr != nil {
		return keep(fmt.Errorf("work: %w", werr))
	}
	if perr != nil {
		return keep(fmt.Errorf("work: %w", perr))
	}

	// Asking git rather than reading the model's prose, for the same reason
	// newTaskID asks the filesystem: a phase that says it implemented something
	// and changed nothing is caught here and not by a reader.
	changed, err := changedFiles(ctx, wt)
	if err != nil {
		return keep(fmt.Errorf("work: %w", err))
	}
	if len(changed) == 0 {
		return keep(fmt.Errorf("work: the phase changed no file, so there is nothing to open a pull request with; what it printed instead is in %s/work.md", workDir))
	}

	// The gate, and the whole gate. No judgement on what the model wrote: the
	// repository's own command decides.
	verdict, err := runCriterion(ctx, cfg, wt.path)
	if err != nil {
		return keep(fmt.Errorf("work: %w; the %d changed file(s) are in the worktree for you to read", err, len(changed)))
	}

	branch := "work/" + opt.TaskID
	msg := fmt.Sprintf("[%s] %s\n\nImplemented by jalon work, behind %q.\n\ncloses %s\n",
		opt.TaskID, strings.TrimPrefix(opt.TaskID, idDatePrefix(opt.TaskID)+"-"), cfg.Criterion, opt.TaskID)
	if err := publish(ctx, wt, branch, []string{".", ":!" + workDir, ":!.claude"}, msg); err != nil {
		return keep(fmt.Errorf("work: %w", err))
	}

	// The body is what a person reads on a phone in the morning, so it is
	// ordered the way the digest is: the task first, then what changed, then
	// what the criterion said, then what it cost. Nothing here is a new
	// mechanism: the digest is the core's verb, run in the worktree.
	body := prBody(ctx, wt, opt.TaskID, cfg, len(changed), verdict, res.Cost)
	// The forge's own mechanism does the closing: a merged pull request saying
	// this closes the issue. jalon carries the number and nothing else, so there
	// is no polling and no state to reconcile. A task written by hand carries no
	// issue, and then there is nothing to close.
	if n := issueOf(task); n != "" {
		body += "\nCloses #" + n + "\n"
	}
	res.PR, _ = createPR(ctx, wt.path, "Work: "+opt.TaskID, body)

	fmt.Fprintf(env.Stdout, "%s %s\n", opt.TaskID, res.PR)
	notify(ctx, env, cfg, fmt.Sprintf("jalon work %s: %s\ncost %s", opt.TaskID, res.PR, formatCost(res.Cost)))

	if opt.KeepWorktree {
		res.Worktree = wt.rel
		return res, nil
	}
	if err := wt.remove(ctx); err != nil {
		fmt.Fprintf(env.Stderr, "jalon: %v\n", err)
	}
	return res, nil
}

// issueOf reads the one front matter key work needs, and returns "" when the
// task carries none. Scanning for a single key rather than calling the core's
// parser is the accepted cost of the package boundary, the same trade as the
// gh wrapper in gh.go: agent cannot import package main, and a second full
// front matter implementation here would be a second thing to keep true.
func issueOf(task string) string {
	rest, ok := strings.CutPrefix(task, "---\n")
	if !ok {
		return ""
	}
	frontMatter, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	for line := range strings.SplitSeq(frontMatter, "\n") {
		v, ok := strings.CutPrefix(line, "issue:")
		if !ok {
			continue
		}
		// Anything that is not a plain number is treated as absent rather than
		// pasted into a pull request body, where "Closes #<garbage>" would
		// either do nothing or name someone else's issue.
		v = strings.TrimSpace(v)
		if v == "" || strings.ContainsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
			return ""
		}
		return v
	}
	return ""
}

// changedFiles is what the phase actually did to the checkout, including files
// it created. The exclusions are jalon's own scratch: they are never committed.
func changedFiles(ctx context.Context, wt *worktree) ([]string, error) {
	out, err := git(ctx, wt.path, "status", "--porcelain", "--", ".", ":!"+workDir, ":!.claude")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			files = append(files, s)
		}
	}
	return files, nil
}

// runCriterion runs the repository's own command, the same way doctor does,
// and returns its last line on success: a test runner's verdict is there, and
// the pull request body quotes it.
func runCriterion(ctx context.Context, cfg *Config, dir string) (string, error) {
	res, err := run(ctx, runOpts{
		dir: dir, name: "sh", args: []string{"-c", cfg.Criterion},
		timeout: cfg.Agent.Timeout, maxOut: 1 << 20,
	})
	if err != nil {
		return "", fmt.Errorf("the criterion %q failed, so this work does not get a pull request: %s", cfg.Criterion, lastLines(res.stderr+res.stdout, 12))
	}
	return strings.TrimSpace(lastLines(res.stdout+res.stderr, 1)), nil
}

// prBody composes the pull request body from what already exists: the core's
// digest of the task, git's own summary of the change, the criterion's last
// line and the cost. A digest that cannot be produced is said, not hidden,
// because a body that silently lost its first section would read as a task
// with no context.
func prBody(ctx context.Context, wt *worktree, id string, cfg *Config, files int, verdict string, cost float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implementation of `%s`, written by `jalon work`. Read the diff; the criterion proves only what it proves.\n\n", id)

	digest, err := run(ctx, runOpts{dir: wt.path, name: "jalon", args: []string{"digest", "-offline", id}, timeout: 60 * time.Second, maxOut: 32 << 10})
	if err != nil {
		fmt.Fprintf(&b, "(digest unavailable: %v)\n\n", err)
	} else {
		fmt.Fprintf(&b, "<details><summary>Task digest</summary>\n\n%s\n</details>\n\n", strings.TrimSpace(digest.stdout))
	}

	stat, serr := git(ctx, wt.path, "diff", "--stat", "HEAD~1")
	if serr != nil {
		fmt.Fprintf(&b, "Files changed: %d (diff --stat unavailable: %v)\n\n", files, serr)
	} else {
		fmt.Fprintf(&b, "```\n%s\n```\n\n", stat)
	}
	fmt.Fprintf(&b, "Criterion `%s`: passed on this branch before the pull request existed; last line: `%s`\n\nCost: %s\n", cfg.Criterion, verdict, formatCost(cost))
	return b.String()
}

// lastLines keeps the tail of a failing command, which is where a test runner
// puts what actually failed.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return "\n" + strings.Join(lines, "\n")
}
