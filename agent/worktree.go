package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A job runs in a throwaway worktree so the model has a real checkout to read
// and nothing of yours to damage: your working tree keeps its uncommitted
// changes, and the worst a runaway phase can do is dirty a directory jalon is
// about to delete.
//
// This is the real boundary of the whole design. The tool policy is a guardrail
// that a determined shell line can walk around; the worktree is not.
type worktree struct {
	root string // repository root
	path string // <root>/<review.worktrees>/<name>
	rel  string // the same path, relative, for messages a person will type
}

// failedDir is where a failed job's worktree is parked, beside the config,
// out of the way of the next job: the evidence is kept, the queue is not
// frozen. It is not configurable because .jalon already exists wherever the
// agent runs, and a second knob for a directory nobody writes to by hand would
// be one more line to hold in the head.
const failedDir = configDir + "/failed"

// maxFailed bounds the wrecks kept. A machine failing every tick would
// otherwise fill the disk with checkouts before anyone reads the first one, and
// ten is more than anyone will read.
const maxFailed = 10

func newWorktree(ctx context.Context, root string, cfg *Config, name string) (*worktree, error) {
	rel := filepath.Join(filepath.FromSlash(cfg.Review.Worktrees), name)
	path := filepath.Join(root, rel)

	if n := countFailed(root); n >= maxFailed {
		return nil, fmt.Errorf("%s holds %d failed jobs, which is the cap: read them, then remove them with git worktree remove --force %s/<name>", failedDir, n, failedDir)
	}
	if _, err := os.Stat(path); err == nil {
		// Deliberately not reused and not suffixed: an existing worktree is
		// either a review running right now or the wreck of one that failed,
		// and jalon never guesses which.
		return nil, fmt.Errorf("%s already exists; another review is running, or one failed and kept it for inspection: read it, then git worktree remove --force %s", rel, rel)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// The job runs on what the forge has, never on what this clone happens to
	// hold. Nothing updates a target repository's checkout: the systemd unit
	// pulls the jalon checkout and only that, so a target drifts from the day it
	// is cloned. A review then measures stale code, and work commits on top of an
	// ancestor, which is how an implementation reverts something already merged.
	//
	// Fatal on failure, unlike the unit's pull, which is deliberately not: there,
	// running the binary you already have is a reasonable answer to a network
	// blip. Here, running on the tree you already have is the defect.
	if _, err := git(ctx, root, "fetch", "--quiet", "origin", cfg.Review.DefaultBranch); err != nil {
		return nil, fmt.Errorf("cannot fetch origin %s, and a job on a stale checkout is worse than no job: %w", cfg.Review.DefaultBranch, err)
	}
	// FETCH_HEAD is exactly what that fetch retrieved, whatever the clone's
	// refspec is configured to track. Detached, so nothing here is on a branch
	// until there is something to commit, and your own branch and working tree
	// are never touched.
	if _, err := git(ctx, root, "worktree", "add", "--detach", path, "FETCH_HEAD"); err != nil {
		return nil, fmt.Errorf("cannot create the worktree from origin %s: %w", cfg.Review.DefaultBranch, err)
	}
	return &worktree{root: root, path: path, rel: rel}, nil
}

func countFailed(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(failedDir)))
	if err != nil {
		return 0
	}
	return len(entries)
}

// park moves a failed job's worktree under failedDir and returns its new
// relative path. The evidence is kept: the facts, the skeptic's answer, the
// diff that failed the criterion are the only bug report there is. But it is
// kept out of the way, so the next tick runs the next item instead of the
// machine waiting for a person; before this, one failure froze the timer for
// the night.
//
// If the move itself fails the worktree stays where it is, which is the old
// behaviour, and the returned path says so.
func (w *worktree) park(ctx context.Context) (string, error) {
	ctx = context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	dest := filepath.Join(w.root, filepath.FromSlash(failedDir), filepath.Base(w.path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return w.rel, err
	}
	if _, err := os.Stat(dest); err == nil {
		// The same job failed before and its wreck was not read. Two wrecks of
		// one job are not twice the evidence, so the older one goes.
		if _, rerr := git(ctx, w.root, "worktree", "remove", "--force", dest); rerr != nil {
			return w.rel, fmt.Errorf("cannot clear the older wreck of this job at %s: %w; the next job is blocked until you run git worktree remove --force %s", dest, rerr, w.rel)
		}
	}
	if _, err := git(ctx, w.root, "worktree", "move", w.path, dest); err != nil {
		return w.rel, fmt.Errorf("cannot move %s out of the way: %w; the next job is blocked until you run git worktree remove --force %s", w.rel, err, w.rel)
	}
	w.path = dest
	w.rel = filepath.Join(filepath.FromSlash(failedDir), filepath.Base(dest))
	return w.rel, nil
}

// remove is called on success only. A failed job is parked, not removed: see
// park.
func (w *worktree) remove(ctx context.Context) error {
	// The caller's context may already be cancelled by an interrupt, and
	// cleanup still has to run.
	ctx = context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if _, err := git(ctx, w.root, "worktree", "remove", "--force", w.path); err != nil {
		if rerr := os.RemoveAll(w.path); rerr != nil {
			return fmt.Errorf("cannot remove %s: %w; remove it by hand: git worktree remove --force %s", w.rel, err, w.rel)
		}
		// The directory is gone but git still holds the registration.
		if _, perr := git(ctx, w.root, "worktree", "prune"); perr != nil {
			return fmt.Errorf("removed %s but git still lists it: %w; run git worktree prune", w.rel, perr)
		}
	}
	return nil
}
