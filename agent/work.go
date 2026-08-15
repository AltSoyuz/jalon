package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type WorkOptions struct {
	TaskID       string
	Next         bool // take the oldest open issue labelled for implementation
	KeepWorktree bool
}

type WorkResult struct {
	TaskID   string
	PR       string
	Worktree string // set only when the worktree was kept, for inspection
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
// It never takes an issue number as the thing to build: that would skip the
// human gate the whole design rests on. What -next accepts is an issue a person
// labelled, which then names its task; the label is the decision, and the task
// is still what gets implemented.
//
// Where Review needed Go code to check that the model had measured, because that
// cannot be verified mechanically, Work has something better and already written:
// the criterion either exits 0 or it does not.
func Work(ctx context.Context, env Env, opt WorkOptions) (WorkResult, error) {
	var res WorkResult

	// The queue is resolved before the preflight, for the same reason review
	// does it: Doctor runs the repository's criterion in full, and a tick that
	// finds nothing labelled would otherwise pay for a test suite and throw the
	// result away.
	//
	// This is not autonomy. The label is a button a person pressed, from a phone
	// or anywhere else, and the timer only delivers it: jalon never chooses what
	// to implement.
	issueNo := 0
	if opt.Next {
		n, err := nextIssue(ctx, env.Root, LabelImplement)
		if err != nil {
			return res, fmt.Errorf("work: cannot read the queue: %w; run jalon doctor", err)
		}
		if n == 0 {
			fmt.Fprintf(env.Stderr, "jalon: no open issue labelled %q, nothing to implement\n", LabelImplement)
			return res, nil
		}
		id, err := taskForIssue(env.Root, n)
		if err != nil {
			return res, fmt.Errorf("work: %w", err)
		}
		issueNo, opt.TaskID = n, id
	}

	// Doctor runs the criterion before the model touches anything, which is what
	// makes the same criterion meaningful afterwards: a red result at the end is
	// attributable to this job rather than to a base that was already broken.
	report := Doctor(ctx, env, Options{})
	if n := report.Failed(); n > 0 {
		return res, fmt.Errorf("work: %d of %d preflight checks failed; a red base gets no job, the fix for each is on stderr above", n, len(report.Checks))
	}
	cfg := report.Config

	task, err := readTask(env.Root, opt.TaskID)
	if err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	res.TaskID = opt.TaskID

	if err := takeJobSlot(env.Root, cfg); err != nil {
		return res, fmt.Errorf("work: %w", err)
	}

	wt, err := newWorktree(ctx, env.Root, cfg, "work-"+opt.TaskID)
	if err != nil {
		return res, fmt.Errorf("work: %w", err)
	}
	keep := func(err error) (WorkResult, error) {
		res.Worktree = wt.rel
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
	written, perr := runPhase(ctx, cfg, wt.path, phase{
		name:  "work",
		skill: "jalon-work",
		tools: []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"},
		allow: append(probeTools(cfg.Probes), "Read", "Grep", "Glob", "Edit"),
		stdin: task + probeList(cfg) +
			"\n--- the criterion this work must satisfy ---\n" + cfg.Criterion + "\n",
		prompt: "Implement the task on stdin, following the jalon-work method. " +
			"Change nothing the task did not ask for, and make the criterion pass.",
	})
	if werr := os.WriteFile(filepath.Join(wt.path, workDir, "work.md"), []byte(written), 0o644); werr != nil {
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
	if err := runCriterion(ctx, cfg, wt.path); err != nil {
		return keep(fmt.Errorf("work: %w; the %d changed file(s) are in the worktree for you to read", err, len(changed)))
	}

	branch := "work/" + opt.TaskID
	msg := fmt.Sprintf("[%s] %s\n\nImplemented by jalon work, behind %q.\n\ncloses %s\n",
		opt.TaskID, strings.TrimPrefix(opt.TaskID, idDatePrefix(opt.TaskID)+"-"), cfg.Criterion, opt.TaskID)
	if err := publish(ctx, wt, branch, []string{".", ":!" + workDir, ":!.claude"}, msg); err != nil {
		return keep(fmt.Errorf("work: %w", err))
	}

	body := fmt.Sprintf("Implementation of `%s`, written by `jalon work`.\n\nThe criterion `%s` passed on this branch before the pull request existed. That is the only thing it proves: read the diff.\n\nFiles changed: %d\n",
		opt.TaskID, cfg.Criterion, len(changed))
	// The forge's own mechanism does the closing: a merged pull request saying
	// this closes the issue. jalon carries the number and nothing else, so there
	// is no polling and no state to reconcile. A task written by hand carries no
	// issue, and then there is nothing to close.
	if n := issueOf(task); n != "" {
		body += "\nCloses #" + n + "\n"
	}
	res.PR, _ = createPR(ctx, wt.path, "Work: "+opt.TaskID, body)

	// Out of the queue before anything else is reported. The label is the queue,
	// and an issue left in it is implemented again on the next tick, which would
	// spend the whole daily cap building the same thing.
	if issueNo != 0 {
		if err := unlabel(ctx, wt.path, issueNo, LabelImplement); err != nil {
			fmt.Fprintf(env.Stderr, "jalon: the work is published but issue #%d still carries the %q label, so the next tick will implement it again: remove it by hand with gh issue edit %d --remove-label %s (%v)\n",
				issueNo, LabelImplement, issueNo, LabelImplement, err)
		}
	}

	fmt.Fprintf(env.Stdout, "%s %s\n", opt.TaskID, res.PR)
	notify(ctx, env, cfg, fmt.Sprintf("jalon work %s: %s", opt.TaskID, res.PR))

	if opt.KeepWorktree {
		res.Worktree = wt.rel
		return res, nil
	}
	if err := wt.remove(ctx); err != nil {
		fmt.Fprintf(env.Stderr, "jalon: %v\n", err)
	}
	return res, nil
}

// readTask returns the task file the work implements. It is read here rather
// than left to the model to find, so a wrong id fails before a single token is
// spent.
func readTask(root, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("a task id is required: jalon work <task-id>")
	}
	if strings.ContainsAny(id, `*?[]\/`) {
		return "", fmt.Errorf("%q is not a task id", id)
	}
	path := filepath.Join(root, ".tasks", id+".md")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no task %s in .tasks; run jalon list to see the ids", id)
	}
	// A done task is either already implemented or was closed on purpose, and
	// re-implementing it would open a pull request nobody asked for.
	if strings.Contains(string(b), "\nstatus: done\n") {
		return "", fmt.Errorf("task %s is done; work implements a task that is still open", id)
	}
	return string(b), nil
}

// taskForIssue finds the task that names this issue. The label says "implement
// this"; which task that means is written in the task itself, and jalon does not
// invent it. Two tasks naming the same issue is a person's mistake to resolve,
// not something to pick between.
func taskForIssue(root string, number int) (string, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".tasks", "*.md"))
	if err != nil {
		return "", err
	}
	want := strconv.Itoa(number)
	var found []string
	for _, p := range paths {
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return "", rerr
		}
		if issueOf(string(b)) == want {
			found = append(found, strings.TrimSuffix(filepath.Base(p), ".md"))
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("issue #%d is labelled %q but no task in .tasks names it; merge the review's pull request first, or add \"issue: %d\" to the task by hand",
			number, LabelImplement, number)
	default:
		return "", fmt.Errorf("issue #%d is named by %d tasks (%s); leave the one to implement and remove the issue key from the others",
			number, len(found), strings.Join(found, ", "))
	}
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

// runCriterion runs the repository's own command, the same way doctor does.
func runCriterion(ctx context.Context, cfg *Config, dir string) error {
	res, err := run(ctx, runOpts{
		dir: dir, name: "sh", args: []string{"-c", cfg.Criterion},
		timeout: cfg.Agent.Timeout, maxOut: 1 << 20,
	})
	if err != nil {
		return fmt.Errorf("the criterion %q failed, so this work does not get a pull request: %s", cfg.Criterion, lastLines(res.stderr+res.stdout, 12))
	}
	return nil
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
