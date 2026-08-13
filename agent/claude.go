package agent

import (
	"context"
	"fmt"
	"strings"
)

// The argv for the model is built here and nowhere else, so the whole coupling
// to one CLI is a function you can read in a minute and a doctor check that
// verifies every flag in it against the installed binary.

// phase is one model invocation. Each is a fresh process with its own tool
// policy and its own budget; nothing is shared between them except files in the
// worktree, which is what makes the gate meaningful. The writing phase cannot
// cite anything the gathering phase did not write down.
type phase struct {
	name   string   // facts, skeptic, task
	skill  string   // the embedded skill whose body becomes the system prompt
	tools  []string // --tools: which built ins exist at all
	allow  []string // --allowed-tools: which of them may actually run
	stdin  string   // the bulk of the context
	prompt string   // the short instruction, positional
}

// neverAllowed is refused in every phase. A review reads a repository and a
// running instance; it does not reach the open network on its own, and it does
// not fan out into subagents jalon cannot see.
var neverAllowed = []string{"WebFetch", "WebSearch", "Task"}

// probeTools maps the allowlist onto the CLI's permission syntax, one rule per
// probe. This is the only place the two vocabularies meet, and Config.Allowed
// applies the same prefix rule, so the gate and the permission cannot drift.
func probeTools(probes []string) []string {
	out := make([]string, 0, len(probes))
	for _, p := range probes {
		out = append(out, "Bash("+p+":*)")
	}
	return out
}

func runPhase(ctx context.Context, cfg *Config, dir string, p phase) (string, error) {
	system, err := skill(p.skill)
	if err != nil {
		return "", err
	}
	args := []string{
		"--print", p.prompt,
		"--model", cfg.Agent.ModelReview,
		"--output-format", "text",
		// dontAsk, never bypassPermissions: in --print mode a permission prompt
		// cannot be answered, so a tool call outside the policy is denied and
		// the model carries on without it. Denial is the mechanism.
		"--permission-mode", "dontAsk",
		"--max-budget-usd", fmt.Sprint(cfg.Agent.MaxBudgetUSD),
		"--append-system-prompt", system,
		"--disallowed-tools", strings.Join(neverAllowed, ","),
	}
	if len(p.tools) > 0 {
		args = append(args, "--tools", strings.Join(p.tools, ","))
	}
	if len(p.allow) > 0 {
		args = append(args, "--allowed-tools", strings.Join(p.allow, ","))
	}

	res, err := run(ctx, runOpts{
		dir:     dir, // the worktree is the model's whole world
		name:    "claude",
		args:    args,
		stdin:   p.stdin,
		timeout: cfg.Agent.Timeout,
		maxOut:  1 << 20,
	})
	if err != nil {
		return res.stdout, fmt.Errorf("the %s phase failed: %w", p.name, err)
	}
	return res.stdout, nil
}
