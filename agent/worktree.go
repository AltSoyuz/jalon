package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// A review runs in a throwaway worktree so the model has a real checkout to read
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

func newWorktree(ctx context.Context, root string, cfg *Config, name string) (*worktree, error) {
	rel := filepath.Join(filepath.FromSlash(cfg.Review.Worktrees), name)
	path := filepath.Join(root, rel)

	if _, err := os.Stat(path); err == nil {
		// Deliberately not reused and not suffixed: an existing worktree is
		// either a review running right now or the wreck of one that failed,
		// and jalon never guesses which.
		return nil, fmt.Errorf("%s already exists; another review is running, or one failed and kept it for inspection: read it, then git worktree remove --force %s", rel, rel)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Detached: nothing here is on a branch until there is something to commit.
	if _, err := git(ctx, root, "worktree", "add", "--detach", path, cfg.Review.DefaultBranch); err != nil {
		return nil, fmt.Errorf("cannot create the review worktree from %s: %w", cfg.Review.DefaultBranch, err)
	}
	return &worktree{root: root, path: path, rel: rel}, nil
}

// remove is called on success only. A failed review keeps its worktree: the
// facts, the skeptic's answer and whatever the writing phase managed are the
// only evidence of what went wrong, and deleting them to look tidy would throw
// away the bug report.
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
