package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The one place this package starts a process. External tools are executed,
// never linked, which is the same choice github.go makes in the core: the whole
// coupling to `claude`, `gh` and `git` is a few functions you can read, and a
// stub on PATH is enough to test them.

type runOpts struct {
	dir     string
	name    string
	args    []string
	stdin   string
	timeout time.Duration // 0 leaves the bound to the caller's context
	maxOut  int           // 0 is unbounded
}

type runResult struct {
	stdout string
	stderr string
	code   int
}

// run executes a command and returns its output. A non zero exit is an error:
// this package has no best effort call, unlike digest, because everything it
// runs is either a probe whose result is the point or a mutation that must not
// be assumed to have happened.
func run(ctx context.Context, o runOpts) (runResult, error) {
	var res runResult
	if _, err := exec.LookPath(o.name); err != nil {
		return res, fmt.Errorf("%s is not installed or not on PATH: %w", o.name, err)
	}
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, o.name, o.args...)
	cmd.Dir = o.dir
	if o.stdin != "" {
		cmd.Stdin = strings.NewReader(o.stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	// A child that ignores the kill and holds the pipe open would otherwise hang
	// Wait forever, which is the one failure a timeout must not have.
	cmd.WaitDelay = 5 * time.Second

	err := cmd.Run()
	res.stdout = truncate(out.String(), o.maxOut)
	res.stderr = truncate(errb.String(), o.maxOut)
	res.code = cmd.ProcessState.ExitCode()

	if ctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("%s timed out after %s; raise timeout_seconds in %s or narrow the task",
			o.name, o.timeout, filepath.Join(configDir, configName))
	}
	if err != nil {
		if msg := strings.TrimSpace(res.stderr); msg != "" {
			return res, fmt.Errorf("%s %s: %w: %s", o.name, strings.Join(o.args, " "), err, msg)
		}
		return res, fmt.Errorf("%s %s: %w", o.name, strings.Join(o.args, " "), err)
	}
	return res, nil
}

// truncate cuts and says so, because every cap is visible in the output it
// applies to.
func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n(truncated to %d of %d bytes by jalon)\n", max, len(s))
}

// git runs a git command in dir. Unlike the core's gitOutput, a failure here is
// never "no git": the agent layer creates worktrees and commits, so a git that
// does not work is a stop.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	res, err := run(ctx, runOpts{dir: dir, name: "git", args: args, timeout: 2 * time.Minute})
	return strings.TrimRight(res.stdout, "\n"), err
}
