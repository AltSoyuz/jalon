package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ReviewOptions struct {
	TaskID       string
	Next         bool // take the oldest task queued with status: measure on origin
	KeepWorktree bool
}

type ReviewResult struct {
	TaskID   string
	PR       string
	Cost     float64 // USD over every phase; -1 when a phase did not report it
	Worktree string  // set only when the worktree was kept, for inspection
}

// reviewDir holds what the phases produce, inside the worktree. It is never
// committed: only the task file is staged, so these die with the worktree.
const reviewDir = ".jalon-review"

// Review turns one task into a measured proposal, in a pull request.
//
// The order is the whole point: facts, then an attempt to refute them, then the
// task. A plan that arrives before the measurement gets believed, and then the
// measurement never happens.
//
// The task it takes is a stub a person queued: a title and a Context saying
// what they think, written by hand or by `jalon new -issue`, with its status
// set to measure. The review rewrites that same file, so one id runs from
// capture to done and no second identifier has to survive the trip.
func Review(ctx context.Context, env Env, opt ReviewOptions) (ReviewResult, error) {
	var res ReviewResult

	// The queue is read before the preflight, for the reason given in Work.
	cfg, err := Load(env.Root)
	if err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	if err := fetchDefault(ctx, env.Root, cfg); err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	if opt.Next {
		id, err := nextTask(ctx, env.Root, StatusMeasure, "task", "review")
		if err != nil {
			return res, fmt.Errorf("review: cannot read the queue: %w", err)
		}
		if id == "" {
			fmt.Fprintf(env.Stderr, "jalon: no task with status: %s on origin %s that is not already published or parked, nothing to review\n", StatusMeasure, cfg.Review.DefaultBranch)
			return res, nil
		}
		opt.TaskID = id
	}
	task, err := readTask(ctx, env.Root, opt.TaskID)
	if err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	res.TaskID = opt.TaskID
	id := opt.TaskID
	taskPath := ".tasks/" + id + ".md"

	report := Doctor(ctx, env, Options{})
	if n := report.Failed(); n > 0 {
		return res, fmt.Errorf("review: %d of %d preflight checks failed; a red base gets no job, the fix for each is on stderr above", n, len(report.Checks))
	}
	cfg = report.Config

	if err := takeJobSlot(env.Root, cfg); err != nil {
		return res, fmt.Errorf("review: %w", err)
	}

	wt, err := newWorktree(ctx, env.Root, cfg, "review-"+id)
	if err != nil {
		return res, fmt.Errorf("review: %w", err)
	}
	// Every failure from here keeps the worktree, parked out of the way, and
	// out of the queue for as long as the wreck is there.
	keep := func(err error) (ReviewResult, error) {
		res.Worktree = failJob(ctx, env, cfg, wt, err)
		return res, err
	}
	if _, err := materializeSkills(wt.path); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	if err := os.MkdirAll(filepath.Join(wt.path, reviewDir), 0o755); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	input := "Task " + id + " (the file is " + taskPath + "):\n\n--- task starts ---\n" + task + "--- task ends ---\n"

	// Phase 1: the probes. The model has read tools and no shell: it prints
	// the commands worth running, one per line, and jalon runs the ones on the
	// allowlist itself, with no shell, and writes facts.md from what each
	// really printed. Writing before measuring is impossible by construction,
	// and a fabricated command block is impossible too: there is no block the
	// model wrote, only output jalon captured.
	planRes, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "facts",
		skill: "jalon-review-facts",
		tools: []string{"Read", "Grep", "Glob"},
		allow: []string{"Read", "Grep", "Glob"},
		stdin: input + probeList(cfg),
		prompt: "Choose the probes that will establish the measured facts about the task on stdin, following the " +
			"jalon-review-facts method. Print one command per line and nothing else; do not write any file.",
	})
	if werr := os.WriteFile(filepath.Join(wt.path, reviewDir, "probes.md"), []byte(planRes.out), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	facts, ran, refused := runProbes(ctx, cfg, wt.path, id, planRes.out)
	factsPath := filepath.Join(wt.path, reviewDir, "facts.md")
	if werr := os.WriteFile(factsPath, []byte(facts), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	// The gate: at least one probe ran. It is the whole gate, and it is
	// jalon's own count rather than a pattern in the model's prose.
	if ran == 0 {
		return keep(fmt.Errorf("review: the gathering phase proposed no command on the probe list, so nothing was measured and a review written on nothing is worth nothing (%d line(s) refused; what it printed is in %s)",
			len(refused), filepath.Join(wt.rel, reviewDir, "probes.md")))
	}
	for _, r := range refused {
		fmt.Fprintf(env.Stderr, "jalon: probe refused: %s\n", r)
	}
	fmt.Fprintf(env.Stderr, "jalon: %d probe(s) run, %d refused, facts written, %d bytes\n", ran, len(refused), len(facts))

	// The skeptic. One pass, separate process, read only: its whole job is to
	// try to make the premise false.
	skepticRes, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "skeptic",
		skill: "jalon-review-skeptic",
		tools: []string{"Read", "Grep", "Glob", "Bash"},
		allow: append(probeTools(cfg.Probes), "Read", "Grep", "Glob"),
		stdin: input + probeList(cfg) + "\n--- facts start ---\n" + facts + "\n--- facts end ---\n",
		prompt: "Try to refute the premise of the task on stdin with a command, following the " +
			"jalon-review-skeptic method. One pass. Print your answer; do not write any file.",
	})
	skeptic := skepticRes.out
	if werr := os.WriteFile(filepath.Join(wt.path, reviewDir, "skeptic.md"), []byte(skeptic), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	fmt.Fprintf(env.Stderr, "jalon: skeptic answered, %d bytes\n", len(skeptic))

	// Phase 2: the task file, and nothing else. The model gets no git, no gh
	// and no push; jalon does every mutation itself, below.
	//
	// Edit(.tasks/**) and no Write rule: Claude Code matches file permissions
	// against Edit rules only, and covers every file-editing tool with them. A
	// Write rule is never in force, and offering one makes the model try a path
	// that gets denied.
	taskRes, err := runPhase(ctx, cfg, wt.path, phase{
		name:  "task",
		skill: "jalon-review-task",
		tools: []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"},
		allow: []string{"Bash(jalon append:*)", "Bash(jalon list:*)", "Bash(jalon digest:*)",
			"Read", "Grep", "Glob", "Edit(.tasks/**)"},
		stdin: input +
			"\n--- facts start ---\n" + facts + "\n--- facts end ---\n" +
			"\n--- skeptic start ---\n" + skeptic + "\n--- skeptic end ---\n",
		prompt: "Rewrite the task on stdin from the facts and the skeptic's answer, in place, " +
			"following the jalon-review-task method. Measured facts first, the plan last.",
	})
	// Written before the error is checked, like the facts. This phase failed on
	// a real job and left nothing to read, so the cause stayed hidden until the
	// invocation was replayed by hand.
	if werr := os.WriteFile(filepath.Join(wt.path, reviewDir, "task.md"), []byte(taskRes.out), 0o644); werr != nil {
		return keep(fmt.Errorf("review: %w", werr))
	}
	if err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	// git says what the phase did to .tasks, not the phase: exactly this one
	// file, rewritten. A phase that wrote nothing, or created a second task,
	// is caught here with a message that says which.
	if err := onlyThisTaskChanged(ctx, wt, taskPath); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	// jalon sets the status, so it is proposed whatever the phase wrote: this
	// task is a proposal awaiting agreement, and merging is the agreement.
	if err := setStatus(filepath.Join(wt.path, taskPath), "proposed"); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}

	issue := issueOf(task)
	msg := fmt.Sprintf("[%s] propose %s\n\nMeasured by jalon review. Facts before plan.\n", id, strings.TrimPrefix(id, idDatePrefix(id)+"-"))
	if issue != "" {
		msg += "\nRefs #" + issue + "\n"
	}
	if err := publish(ctx, wt, "task/"+id, []string{taskPath}, msg); err != nil {
		return keep(fmt.Errorf("review: %w", err))
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Measured proposal for `%s`, written by `jalon review`.\n\nMerging is the agreement; this branch holds the task file only. Set its status to %s to have it built.\n\n", id, StatusImplement)
	if len(refused) > 0 {
		body.WriteString("Probes the gathering phase asked for and jalon refused, so nothing rests on them; add to `probes.allowed` and re-run if one matters:\n\n")
		for _, r := range refused {
			fmt.Fprintf(&body, "- `%s`\n", r)
		}
		body.WriteString("\n")
	}
	res.Cost = sumCost(planRes.cost, skepticRes.cost, taskRes.cost)
	fmt.Fprintf(&body, "Cost: %s over three model calls.\n", formatCost(res.Cost))
	if issue != "" {
		fmt.Fprintf(&body, "\nRefs #%s\n", issue)
	}
	res.PR, _ = createPR(ctx, wt.path, "Task: "+id, body.String())

	fmt.Fprintf(env.Stdout, "%s %s\n", id, res.PR)
	notify(ctx, env, cfg, fmt.Sprintf("jalon review %s: %s\ncost %s", id, res.PR, formatCost(res.Cost)))

	if opt.KeepWorktree {
		res.Worktree = wt.rel
		return res, nil
	}
	if err := wt.remove(ctx); err != nil {
		fmt.Fprintf(env.Stderr, "jalon: %v\n", err)
	}
	return res, nil
}

// onlyThisTaskChanged asks git what happened under .tasks: the one file,
// modified, and nothing else.
func onlyThisTaskChanged(ctx context.Context, wt *worktree, taskPath string) error {
	out, err := git(ctx, wt.path, "status", "--porcelain", "--", ".tasks")
	if err != nil {
		return err
	}
	var changed []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) != "" {
			changed = append(changed, line)
		}
	}
	switch {
	case len(changed) == 0:
		return fmt.Errorf("the writing phase changed nothing under .tasks; it was told to rewrite %s and did not, and what it printed instead is in %s/task.md", taskPath, reviewDir)
	case len(changed) == 1 && changed[0] == " M "+taskPath:
		return nil
	default:
		return fmt.Errorf("the writing phase was to rewrite %s and nothing else, and git sees: %s", taskPath, strings.Join(changed, ", "))
	}
}

// setStatus rewrites the one front matter line jalon owns in a review. It is
// a line replacement and not a second front matter parser: the file keeps
// every other byte as the phase left it.
func setStatus(path, status string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return fmt.Errorf("%s has no front matter, so its status cannot be set", path)
	}
	fm, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return fmt.Errorf("%s has an unterminated front matter", path)
	}
	lines := strings.Split(fm, "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "status:") {
			lines[i] = "status: " + status
			found = true
		}
	}
	if !found {
		lines = append([]string{"status: " + status}, lines...)
	}
	return os.WriteFile(path, []byte("---\n"+strings.Join(lines, "\n")+"\n---"+body), 0o644)
}

// probeList tells a phase what it may run, instead of leaving it to find out by
// being denied. The first live runs spent turns discovering the allowlist the
// hard way, and then wrote the refused commands into the document as though
// they had run, which is the one thing a facts document must never do.
func probeList(cfg *Config) string {
	var b strings.Builder
	b.WriteString("\n--- the only shell commands you may run ---\n")
	for _, p := range cfg.Probes {
		b.WriteString(p)
		b.WriteString("\n")
	}
	b.WriteString("--- anything else is denied: report it in prose, never in a $ block ---\n")
	return b.String()
}

// runProbes runs what the gathering phase asked for, one process per line and
// no shell, and returns the facts document built from the real output, the
// number of probes that ran, and the lines it refused. A line is refused when
// it is not on the allowlist or when it is composed of several commands: the
// allowlist matches one command, so a composed line cannot be checked against
// it. Refusals are listed in the document and reported, never silent.
func runProbes(ctx context.Context, cfg *Config, dir, id, plan string) (string, int, []string) {
	var doc strings.Builder
	var refused []string
	ran := 0
	fmt.Fprintf(&doc, "# Facts for %s\n\nEvery block below is a command jalon ran in the worktree, with what it printed. Nothing here was written by a model.\n\n", id)
	for line := range strings.SplitSeq(plan, "\n") {
		cmd := strings.TrimSpace(line)
		if cmd == "" {
			continue
		}
		if !cfg.Allowed(cmd) || strings.ContainsAny(cmd, composeMeta) {
			refused = append(refused, cmd)
			continue
		}
		words := strings.Fields(cmd)
		if _, lerr := exec.LookPath(words[0]); lerr != nil {
			// Not a measurement: nothing started. Refused, and the reason is
			// in the document, so "the probe list names a program this
			// machine lacks" is read rather than inferred.
			refused = append(refused, cmd+" (not installed: "+words[0]+")")
			continue
		}
		res, err := run(ctx, runOpts{dir: dir, name: words[0], args: words[1:], timeout: cfg.Agent.Timeout, maxOut: 32 << 10})
		ran++
		fmt.Fprintf(&doc, "```console\n$ %s\n%s", cmd, res.stdout)
		if !strings.HasSuffix(res.stdout, "\n") && res.stdout != "" {
			doc.WriteString("\n")
		}
		if err != nil {
			// The failure is a measurement too: a probe that cannot run says
			// something about the system, and its stderr is part of that.
			fmt.Fprintf(&doc, "(exit %d: %s)\n", res.code, strings.TrimSpace(res.stderr))
		}
		doc.WriteString("```\n\n")
	}
	if len(refused) > 0 {
		doc.WriteString("## Refused\n\nAsked for by the gathering phase, not run: not on `probes.allowed`, or composed of several commands.\n\n")
		for _, r := range refused {
			fmt.Fprintf(&doc, "- `%s`\n", r)
		}
		doc.WriteString("\n")
	}
	return doc.String(), ran, refused
}

// composeMeta is what would chain a second command if a shell were involved.
// jalon runs a probe with exec and no shell, so nothing here can compose;
// these are still refused because a line like `git log a | wc -l` handed to
// git as literal arguments measures nothing and reads as if it had. Not in
// the list: parentheses and dollars, which are ordinary bytes in a path
// (`routes/(app)/start`) or a curl format (`-w %{time_total}`), and quotes,
// which group arguments. The first live review lost the file it needed to a
// refused `(app)`.
const composeMeta = "|&;<>`\n"

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

// publish is every mutation of a run, done by jalon rather than by the model.
// No phase is ever handed a commit or a push, which removes that blast radius
// instead of policing it.
//
// The pathspec is what separates the two verbs: a review commits the task file
// and nothing else, work commits the checkout minus jalon's own scratch.
// Whatever is left out stays uncommitted and dies with the worktree.
func publish(ctx context.Context, wt *worktree, branch string, pathspec []string, msg string) error {
	if _, err := git(ctx, wt.path, "switch", "-c", branch); err != nil {
		return err
	}
	if _, err := git(ctx, wt.path, append([]string{"add", "--"}, pathspec...)...); err != nil {
		return err
	}
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

// failJob is what every failure after the worktree exists goes through: the
// worktree is parked under failedDir, the failure is notified with the path
// to read, and the returned path is what the result reports. It never hides
// the original error; it adds where the evidence went.
func failJob(ctx context.Context, env Env, cfg *Config, wt *worktree, cause error) string {
	rel, err := wt.park(ctx)
	if err != nil {
		fmt.Fprintf(env.Stderr, "jalon: %v\n", err)
	}
	notify(ctx, env, cfg, fmt.Sprintf("jalon %s failed: %v\nkept at %s", filepath.Base(wt.path), cause, rel))
	return rel
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
		// Terminated, so a command reading it line by line gets the last line.
		stdin: strings.TrimRight(msg, "\n") + "\n", timeout: 60 * time.Second,
	}); err != nil {
		// The work is done and pushed; a failed notification must not undo it.
		fmt.Fprintf(env.Stderr, "jalon: the notification command failed: %v\n", err)
		fmt.Fprintln(env.Stdout, msg)
	}
}
