package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The queue is the status field of the task files, as origin's default branch
// has them, read with git alone. There is no label, no forge call and no state
// outside git: a person queues a task by editing one front matter line and
// committing it to main, from a phone or a shell, and the timer delivers it.
//
// The first version used two issue labels and carried a second identifier
// through every job; reconciling the two cost a full task
// (260815-carry-the-issue-number-from-review-to-work). One id now runs from
// capture to done.
const (
	// StatusMeasure queues a task for `review -next`: measure it, try to refute
	// it, and rewrite it as a proposal in a pull request.
	StatusMeasure = "measure"
	// StatusImplement queues an agreed task for `work -next`. Setting it is the
	// decision to implement; jalon never chooses what to build.
	StatusImplement = "implement"
)

// fetchDefault brings origin's default branch into FETCH_HEAD, which is what
// every read of the queue and every worktree is cut from. The job runs on what
// the forge has, never on what this clone happens to hold: nothing updates a
// target repository's checkout.
func fetchDefault(ctx context.Context, root string, cfg *Config) error {
	if _, err := git(ctx, root, "fetch", "--quiet", "origin", cfg.Review.DefaultBranch); err != nil {
		return fmt.Errorf("cannot fetch origin %s, and a job on a stale checkout is worse than no job: %w", cfg.Review.DefaultBranch, err)
	}
	return nil
}

// nextTask returns the oldest task on FETCH_HEAD whose status is the one given,
// skipping the ones already published (a <branch>/<id> branch on origin) and the
// ones whose last job left a wreck in failedDir (<job>-<id>). "" when there is
// none, which is the normal state of a timer between jobs.
//
// A published task leaves the queue by having a branch, not by a write to main:
// main is protected and jalon has no merge call, so the branch is the only
// mark it can leave. A pull request closed without merging keeps the task out
// of the queue until someone deletes the branch, which is the right default: a
// person looked and said no.
//
// A wreck keeps its task out of the queue until it is read and removed, so a
// failing task is retried by a person and not by the clock.
func nextTask(ctx context.Context, root string, status, branch, job string) (string, error) {
	out, err := git(ctx, root, "grep", "-l", "-e", "^status: "+status+"$", "FETCH_HEAD", "--", ".tasks")
	if err != nil {
		// grep exits 1 on no match, and run makes that an error; the file list
		// is empty either way, so an empty output is the answer.
		if strings.TrimSpace(out) == "" {
			return "", nil
		}
		return "", err
	}
	var ids []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		// FETCH_HEAD:.tasks/260815-x.md
		_, path, ok := strings.Cut(line, ":")
		if !ok || !strings.HasSuffix(path, ".md") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".md"))
	}
	if len(ids) == 0 {
		return "", nil
	}
	sort.Strings(ids) // YYMMDD-slug sorts oldest first

	published, err := remoteBranches(ctx, root, branch)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if published[branch+"/"+id] {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(failedDir), job+"-"+id)); err == nil {
			continue
		}
		return id, nil
	}
	return "", nil
}

// remoteBranches lists origin's branches under one prefix, in one call.
func remoteBranches(ctx context.Context, root, prefix string) (map[string]bool, error) {
	out, err := git(ctx, root, "ls-remote", "--heads", "origin", prefix+"/*")
	if err != nil {
		return nil, fmt.Errorf("cannot list origin's %s/ branches: %w", prefix, err)
	}
	set := make(map[string]bool)
	for line := range strings.SplitSeq(out, "\n") {
		if _, ref, ok := strings.Cut(line, "\trefs/heads/"); ok {
			set[strings.TrimSpace(ref)] = true
		}
	}
	return set, nil
}

// readTask returns the task file as origin has it. It is read from FETCH_HEAD
// rather than from this checkout, which nothing updates, so a wrong id fails
// before a token is spent and the phase gets the same text the worktree holds.
func readTask(ctx context.Context, root, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("a task id is required")
	}
	if strings.ContainsAny(id, `*?[]\/`) {
		return "", fmt.Errorf("%q is not a task id", id)
	}
	out, err := git(ctx, root, "show", "FETCH_HEAD:.tasks/"+id+".md")
	if err != nil {
		return "", fmt.Errorf("no task %s on origin's default branch; push it first, or run jalon list to see the ids", id)
	}
	// A done task is either already implemented or was closed on purpose, and
	// working on it would open a pull request nobody asked for.
	if strings.Contains(out+"\n", "\nstatus: done\n") {
		return "", fmt.Errorf("task %s is done; a job takes a task that is still open", id)
	}
	return out + "\n", nil
}
