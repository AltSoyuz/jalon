package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ReviewOptions struct {
	Issue        int
	TasksDir     string // the resolved .tasks directory of the real repository
	KeepWorktree bool
}

type ReviewResult struct {
	TaskID   string
	PR       string
	Worktree string // set only when the worktree was kept, for inspection
}

// reviewDir holds what the phases produce, inside the worktree. It is never
// committed: only .tasks/ is staged, so these die with the worktree.
const reviewDir = ".jalon-review"

// Review turns one issue into a measured task proposal, in a pull request.
//
// The order is the whole point: facts, then an attempt to refute them, then the
// task. A plan that arrives before the measurement gets believed, and then the
// measurement never happens.
func Review(ctx context.Context, env Env, opt ReviewOptions) (ReviewResult, error) {
	var res ReviewResult

	report := Doctor(ctx, env)
	if n := report.Failed(); n > 0 {
		return res, fmt.Errorf("review: %d of %d preflight checks failed; a red base gets no job, the fix for each is on stderr above", n, len(report.Checks))
	}
	cfg := report.Config

	if err := takeJobSlot(env.Root, cfg); err != nil {
		return res, fmt.Errorf("review: %w", err)
	}

	iss, err := fetchIssue(ctx, env.Root, opt.Issue)
	if err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	if strings.EqualFold(iss.State, "CLOSED") {
		return res, fmt.Errorf("review: issue #%d is closed; reopen it, or pick another", opt.Issue)
	}

	wt, err := newWorktree(ctx, env.Root, cfg, fmt.Sprintf("review-%d", opt.Issue))
	if err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	// Every failure from here keeps the worktree, and every path out says so.
	keep := func(err error) (ReviewResult, error) {
		res.Worktree = wt.rel
		return res, err
	}
	if _, err := materializeSkills(wt.path); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	if err := os.MkdirAll(filepath.Join(wt.path, reviewDir), 0o755); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}

	// Phase 1: facts. No write tool exists at all, so jalon writes the file
	// from what the phase printed. The gate then checks content jalon itself
	// produced rather than something the model could have shaped to pass.
	facts, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "facts",
		skill: "jalon-review-facts",
		tools: []string{"Read", "Grep", "Glob", "Bash"},
		allow: append(probeTools(cfg.Probes), "Read", "Grep", "Glob"),
		stdin: iss.text(),
		prompt: "Gather the measured facts about the issue on stdin, following the jalon-review-facts method. " +
			"Print the facts document; do not write any file.",
	})
	factsPath := filepath.Join(wt.path, reviewDir, "facts.md")
	if werr := os.WriteFile(factsPath, []byte(facts), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	if err := gate(facts, cfg); err != nil {
		return keep(fmt.Errorf("review: %w (the output is in %s)", err, filepath.Join(wt.rel, reviewDir, "facts.md")))
	}
	fmt.Fprintf(env.Stderr, "jalon: facts gathered, %d bytes\n", len(facts))

	// The skeptic. One pass, separate process, read only: its whole job is to
	// try to make the premise false.
	skeptic, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "skeptic",
		skill: "jalon-review-skeptic",
		tools: []string{"Read", "Grep", "Glob", "Bash"},
		allow: append(probeTools(cfg.Probes), "Read", "Grep", "Glob"),
		stdin: iss.text() + "\n--- facts start ---\n" + facts + "\n--- facts end ---\n",
		prompt: "Try to refute the premise of the issue on stdin with a command, following the " +
			"jalon-review-skeptic method. One pass. Print your answer; do not write any file.",
	})
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	if werr := os.WriteFile(filepath.Join(wt.path, reviewDir, "skeptic.md"), []byte(skeptic), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	fmt.Fprintf(env.Stderr, "jalon: skeptic answered, %d bytes\n", len(skeptic))

	// Phase 2: the task file, and nothing else. The model gets no git, no gh
	// and no push; jalon does every mutation itself, below.
	before, err := taskFiles(wt.path)
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	if _, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "task",
		skill: "jalon-review-task",
		tools: []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"},
		allow: []string{"Bash(jalon new:*)", "Bash(jalon append:*)", "Bash(jalon list:*)",
			"Read", "Grep", "Glob", "Write(.tasks/**)", "Edit(.tasks/**)"},
		stdin: iss.text() +
			"\n--- facts start ---\n" + facts + "\n--- facts end ---\n" +
			"\n--- skeptic start ---\n" + skeptic + "\n--- skeptic end ---\n",
		prompt: "Write one jalon task from the issue, the facts and the skeptic's answer on stdin, " +
			"following the jalon-review-task method. Measured facts first, the plan last.",
	}); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}

	id, err := newTaskID(wt.path, before)
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	res.TaskID = id

	if err := publish(ctx, wt, id, iss); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	res.PR, _ = createPR(ctx, wt.path,
		"Task: "+id,
		fmt.Sprintf("Measured proposal for #%d, written by `jalon review`.\n\nMerging is the agreement; this branch holds the task file only.\n\nRefs #%d\n", iss.Number, iss.Number))

	fmt.Fprintf(env.Stdout, "%s %s\n", id, res.PR)
	notify(ctx, env, cfg, fmt.Sprintf("jalon review #%d: %s\n%s", iss.Number, id, res.PR))

	if opt.KeepWorktree {
		res.Worktree = wt.rel
		return res, nil
	}
	if err := wt.remove(ctx); err != nil {
		fmt.Fprintf(env.Stderr, "jalon: %v\n", err)
	}
	return res, nil
}

// commandBlock is the shape the gathering skill is told to paste for every
// command it runs.
var commandBlock = regexp.MustCompile("(?m)^```console\\s*\\n\\$ ([^\\n]+)")

// gate is the structural check between measuring and writing, and it is Go
// rather than a prompt: writing before facts is impossible by construction, not
// by instruction.
//
// What it proves: the phase produced a document holding at least one command
// block in the documented shape, whose command is on the allowlist. What it
// does not prove: that the command ran. A fabricated block would pass. Removing
// that class means having jalon run the probes itself and build the document
// from its own output, which is the intended next step and is written down in
// docs/agent.md as not done.
func gate(facts string, cfg *Config) error {
	if n := len(strings.TrimSpace(facts)); n < 200 {
		return fmt.Errorf("the gathering phase produced %d bytes, which is not a facts document; it stopped too early", n)
	}
	matches := commandBlock.FindAllStringSubmatch(facts, -1)
	if len(matches) == 0 {
		return errors.New("the gathering phase produced no executed command block, so it narrated instead of measuring, and a review written on narration is worth nothing (the shape is a ```console block whose first line starts with \"$ \")")
	}
	for _, m := range matches {
		if cmd := strings.TrimSpace(m[1]); !cfg.Allowed(cmd) {
			return fmt.Errorf("the facts report running %q, which is not in probes.allowed; add it to %s or drop the claim",
				cmd, filepath.Join(configDir, configName))
		}
	}
	return nil
}

// takeJobSlot enforces the daily cap with a plain text counter: one line, one
// integer, readable with cat. That is the point of it. The date is stored beside
// the count so the file resets itself without anything scheduled to reset it.
func takeJobSlot(root string, cfg *Config) error {
	path := filepath.Join(root, filepath.FromSlash(cfg.Agent.CounterFile))
	today := time.Now().Format("2006-01-02")
	count := 0

	switch b, err := os.ReadFile(path); {
	case err == nil:
		if day, n, ok := strings.Cut(strings.TrimSpace(string(b)), " "); ok && day == today {
			count, _ = strconv.Atoi(n)
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	if count >= cfg.Agent.DailyJobCap {
		return fmt.Errorf("the daily cap of %d jobs is reached (%s); raise daily_job_cap in %s, or wait for tomorrow",
			cfg.Agent.DailyJobCap, cfg.Agent.CounterFile, filepath.Join(configDir, configName))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, fmt.Appendf(nil, "%s %d\n", today, count+1), 0o644)
}

func taskFiles(root string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".tasks", "*.md"))
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[filepath.Base(p)] = true
	}
	return set, nil
}

// newTaskID finds what the writing phase created. Asking the filesystem rather
// than parsing the model's prose is what makes this reliable: a phase that
// wrote nothing, or wrote two tasks, is caught here with a message that says
// which.
func newTaskID(root string, before map[string]bool) (string, error) {
	after, err := taskFiles(root)
	if err != nil {
		return "", err
	}
	var added []string
	for name := range after {
		if !before[name] {
			added = append(added, strings.TrimSuffix(name, ".md"))
		}
	}
	switch len(added) {
	case 1:
		return added[0], nil
	case 0:
		return "", errors.New("the writing phase created no task file; it was told to run jalon new and did not")
	default:
		return "", fmt.Errorf("the writing phase created %d task files (%s); one coherent deliverable is one task",
			len(added), strings.Join(added, ", "))
	}
}

// publish is every mutation of the run, done by jalon rather than by the model.
// The model is never handed a commit or a push, which removes that blast radius
// instead of policing it.
func publish(ctx context.Context, wt *worktree, id string, iss *issue) error {
	branch := "task/" + id
	if _, err := git(ctx, wt.path, "switch", "-c", branch); err != nil {
		return err
	}
	// Only the task file. Whatever else the phases left in the worktree stays
	// uncommitted and dies with it.
	if _, err := git(ctx, wt.path, "add", "--", ".tasks"); err != nil {
		return err
	}
	msg := fmt.Sprintf("[%s] propose %s\n\nMeasured from #%d by jalon review. Facts before plan.\n\nRefs #%d\n",
		id, strings.TrimPrefix(id, idDatePrefix(id)+"-"), iss.Number, iss.Number)
	if _, err := git(ctx, wt.path, "commit", "-m", msg); err != nil {
		return fmt.Errorf("nothing to commit, or the commit failed: %w", err)
	}
	if _, err := git(ctx, wt.path, "push", "-u", "origin", branch); err != nil {
		return fmt.Errorf("cannot push %s: %w; the token needs contents:write on this repository", branch, err)
	}
	return nil
}

func idDatePrefix(id string) string {
	if len(id) >= 6 {
		return id[:6]
	}
	return id
}

// notify hands the message to one command on stdin. jalon does not know your
// chat service; it knows one command. Absent, the message goes to stdout, which
// a journal already records.
func notify(ctx context.Context, env Env, cfg *Config, msg string) {
	if cfg.Notify == "" {
		fmt.Fprintln(env.Stdout, msg)
		return
	}
	if _, err := run(ctx, runOpts{
		dir: env.Root, name: "sh", args: []string{"-c", cfg.Notify},
		stdin: msg, timeout: 60 * time.Second,
	}); err != nil {
		// The work is done and pushed; a failed notification must not undo it.
		fmt.Fprintf(env.Stderr, "jalon: the notification command failed: %v\n", err)
		fmt.Fprintln(env.Stdout, msg)
	}
}
