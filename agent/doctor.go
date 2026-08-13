package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Env is everything this package needs from its caller. The core owns path
// resolution, flags and metrics; this package owns the model, the probes and
// the worktree. Nothing here is a jalon type, which is what lets the two stay
// independent.
type Env struct {
	Root   string    // repository root: the parent of .tasks
	Stdout io.Writer // machine readable, one record per line
	Stderr io.Writer // human diagnostics, every line prefixed "jalon: "
}

type State string

const (
	Ok   State = "ok"
	Warn State = "warn"
	Fail State = "fail"
	Skip State = "skip"
)

// Check is one preflight result. Detail is what was observed and goes on
// stdout; Fix is imperative and goes on stderr. A check that is not Ok always
// carries a Fix, because a failure message that is not an instruction leaves
// the reader exactly where they started.
type Check struct {
	Name   string
	State  State
	Detail string
	Fix    string
}

type Report struct {
	Checks []Check
	Config *Config // nil when the configuration did not load
}

func (r Report) Failed() int {
	n := 0
	for _, c := range r.Checks {
		if c.State == Fail {
			n++
		}
	}
	return n
}

// Doctor runs every check, in order, printing each as it finishes so that a
// slow one shows progress.
//
// Every check runs even after a failure. Fixing a fresh server one error per
// run is the ergonomic failure this verb exists to prevent, so a check whose
// prerequisite failed reports Skip and names what it was waiting for: a
// silently skipped check is indistinguishable from a passing one.
//
// A failed check is data, not an error. The caller owns the exit status.
func Doctor(ctx context.Context, env Env) Report {
	var r Report
	add := func(c Check) Check {
		r.Checks = append(r.Checks, c)
		fmt.Fprintf(env.Stdout, "%-5s %-12s %s\n", c.State, c.Name, c.Detail)
		if c.State != Ok {
			fmt.Fprintf(env.Stderr, "jalon: %s: %s\n", c.Name, c.Fix)
		}
		return c
	}
	skip := func(name, because string) {
		add(Check{Name: name, State: Skip, Fix: "skipped because " + because + " failed; fix that first"})
	}

	cfg, cerr := Load(env.Root)
	if cerr != nil {
		add(Check{Name: "config", State: Fail, Detail: "not usable", Fix: cerr.Error()})
	} else {
		r.Config = cfg
		add(Check{Name: "config", State: Ok,
			Detail: fmt.Sprintf("%s, %d probes", filepath.Join(configDir, configName), len(cfg.Probes))})
	}

	// claude does not depend on the configuration: reporting it missing on a
	// server whose config is also wrong saves a round trip.
	claudeOK := false
	if res, err := run(ctx, runOpts{dir: env.Root, name: "claude", args: []string{"--version"}, timeout: 30 * time.Second}); err != nil {
		add(Check{Name: "claude", State: Fail, Detail: "not usable", Fix: claudeFix(err)})
	} else {
		claudeOK = true
		add(Check{Name: "claude", State: Ok, Detail: strings.TrimSpace(res.stdout)})
	}

	if !claudeOK {
		skip("claude-flags", "claude")
	} else {
		add(flagsCheck(ctx, env.Root))
	}

	ghOK := false
	if _, err := run(ctx, runOpts{dir: env.Root, name: "gh", args: []string{"auth", "status"}, timeout: 30 * time.Second}); err != nil {
		add(Check{Name: "gh", State: Fail, Detail: "not authenticated",
			Fix: `run "gh auth login" and pick the host this repository lives on (install it from https://cli.github.com if it is missing)`})
	} else {
		ghOK = true
		add(Check{Name: "gh", State: Ok, Detail: "authenticated"})
	}

	if !ghOK {
		skip("gh-scopes", "gh")
	} else {
		add(scopesCheck(ctx, env.Root))
	}

	if cfg == nil {
		skip("git", "config")
		skip("criterion", "config")
	} else {
		add(gitCheck(ctx, env.Root, cfg))
		add(criterionCheck(ctx, env.Root, cfg))
	}

	add(skillsCheck())
	return r
}

func claudeFix(err error) string {
	if strings.Contains(err.Error(), "not installed or not on PATH") {
		return "install the Claude Code CLI (https://claude.com/claude-code); under systemd, set PATH in the unit, a service does not inherit your shell's"
	}
	return fmt.Sprintf(`run "claude --version" by hand and fix what it reports; jalon runs it exactly like that (%v)`, err)
}

// claudeFlags is every flag jalon passes. Checking them against the installed
// CLI's own help turns "these flags probably exist" into a check that names the
// missing one on the machine that has the problem. It is kept beside the code
// that passes them so the two cannot disagree.
var claudeFlags = []string{
	"--print", "--model", "--output-format", "--permission-mode",
	"--allowed-tools", "--disallowed-tools", "--tools",
	"--append-system-prompt", "--max-budget-usd",
}

func flagsCheck(ctx context.Context, root string) Check {
	res, err := run(ctx, runOpts{dir: root, name: "claude", args: []string{"--help"}, timeout: 30 * time.Second})
	if err != nil {
		return Check{Name: "claude-flags", State: Fail, Detail: "unreadable",
			Fix: fmt.Sprintf(`run "claude --help" by hand and fix what it reports (%v)`, err)}
	}
	var missing []string
	for _, f := range claudeFlags {
		if !strings.Contains(res.stdout, f) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return Check{Name: "claude-flags", State: Fail, Detail: strings.Join(missing, " "),
			Fix: fmt.Sprintf("upgrade the Claude Code CLI, this one does not accept %s; compare \"claude --help\" with the flag list in docs/agent.md",
				strings.Join(missing, ", "))}
	}
	return Check{Name: "claude-flags", State: Ok, Detail: fmt.Sprintf("%d flags present", len(claudeFlags))}
}

// scopesCheck reads the scope header of a classic token. A fine grained token
// sends no such header, which is not a failure: it is what a correctly scoped
// modern token looks like. Reporting that as broken would train people to
// ignore this verb, so it warns and says what to check by hand.
func scopesCheck(ctx context.Context, root string) Check {
	res, err := run(ctx, runOpts{dir: root, name: "gh", args: []string{"api", "-i", "user"}, timeout: 30 * time.Second})
	if err != nil {
		return Check{Name: "gh-scopes", State: Warn, Detail: "unknown",
			Fix: fmt.Sprintf("could not read the token's scopes (%v); check by hand that it can read this repository's issues", err)}
	}
	for line := range strings.SplitSeq(res.stdout, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "x-oauth-scopes") {
			continue
		}
		scopes := strings.TrimSpace(value)
		if strings.Contains(scopes, "repo") {
			return Check{Name: "gh-scopes", State: Ok, Detail: scopes}
		}
		return Check{Name: "gh-scopes", State: Fail, Detail: scopes,
			Fix: `run "gh auth refresh -s repo" so the review can read issues and push a branch`}
	}
	return Check{Name: "gh-scopes", State: Warn, Detail: "no scope header",
		Fix: "gh sent no scope header, which is what a fine grained token does; check by hand that it has issues:write and contents:write on this repository"}
}

func gitCheck(ctx context.Context, root string, cfg *Config) Check {
	if _, err := git(ctx, root, "rev-parse", "--git-dir"); err != nil {
		return Check{Name: "git", State: Fail, Detail: "not a repository",
			Fix: fmt.Sprintf("run jalon doctor from inside the repository: git rev-parse failed in %s", root)}
	}
	branch := cfg.Review.DefaultBranch
	if _, err := git(ctx, root, "rev-parse", "--verify", branch); err != nil {
		return Check{Name: "git", State: Fail, Detail: "no " + branch,
			Fix: fmt.Sprintf("create or fetch %q, which review.default_branch names and every review branches from: git rev-parse --verify %s failed", branch, branch)}
	}
	// A stale worktree blocks the next review, so it is worth catching here
	// rather than halfway through a job.
	stale := filepath.Join(root, filepath.FromSlash(cfg.Review.Worktrees))
	if entries, err := os.ReadDir(stale); err == nil && len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return Check{Name: "git", State: Fail, Detail: "stale worktree " + strings.Join(names, " "),
			Fix: fmt.Sprintf("a previous review kept its worktree for inspection; read it, then: git worktree remove --force %s",
				filepath.Join(cfg.Review.Worktrees, names[0]))}
	}
	// A dirty tree is deliberately a warning. Every review runs in its own
	// detached worktree branched from the default branch, so uncommitted work
	// here does not affect it; failing on it would make doctor red for anyone
	// mid task, and a doctor that is always red gets ignored.
	if out, err := git(ctx, root, "status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
		n := len(strings.Split(strings.TrimSpace(out), "\n"))
		return Check{Name: "git", State: Warn, Detail: fmt.Sprintf("%s, %d uncommitted", branch, n),
			Fix: "the working tree has uncommitted changes; a review is unaffected because it branches its own worktree, but \"git status\" is worth a look"}
	}
	return Check{Name: "git", State: Ok, Detail: branch + ", clean"}
}

func criterionCheck(ctx context.Context, root string, cfg *Config) Check {
	res, err := run(ctx, runOpts{
		dir: root, name: "sh", args: []string{"-c", cfg.Criterion},
		timeout: cfg.Agent.Timeout, maxOut: 4096,
	})
	if err != nil {
		detail := fmt.Sprintf("exited %d", res.code)
		if res.code < 0 {
			detail = "did not run"
		}
		return Check{Name: "criterion", State: Fail, Detail: detail,
			Fix: fmt.Sprintf("make the criterion pass before reviewing: run %q and fix what it reports (criterion.command in %s)",
				cfg.Criterion, filepath.Join(configDir, configName))}
	}
	return Check{Name: "criterion", State: Ok, Detail: cfg.Criterion}
}

func skillsCheck() Check {
	dir, err := os.MkdirTemp("", "jalon-skills-")
	if err != nil {
		return Check{Name: "skills", State: Fail, Detail: "no temp dir",
			Fix: fmt.Sprintf("make a temporary directory available, or point $TMPDIR at a writable one: %v", err)}
	}
	defer os.RemoveAll(dir)

	written, err := materializeSkills(dir)
	if err != nil {
		return Check{Name: "skills", State: Fail, Detail: "not writable", Fix: err.Error()}
	}
	for _, p := range written {
		fi, err := os.Stat(p)
		if err != nil || fi.Size() == 0 {
			return Check{Name: "skills", State: Fail, Detail: filepath.Base(filepath.Dir(p)),
				Fix: "an embedded skill is empty, which means the binary was built wrong; rebuild it with make build"}
		}
	}
	return Check{Name: "skills", State: Ok, Detail: fmt.Sprintf("%d materialized", len(written))}
}
