package agent

import (
	"context"
	"encoding/json"
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

// phaseResult is what a phase printed and what it cost. The cost comes from
// the CLI's JSON envelope; the kill criterion in docs/agent.md is stated in
// dollars per merged branch, and until this was read no file in this package
// held a dollar amount.
type phaseResult struct {
	out  string  // the model's final text, what the next phase and the files get
	cost float64 // USD, from total_cost_usd; -1 when the envelope did not carry it
}

// envelope is the part of `claude --output-format json` this package reads.
type envelope struct {
	Result string   `json:"result"`
	Cost   *float64 `json:"total_cost_usd"`
}

// unwrap reads the JSON envelope the CLI prints. Anything that is not one is
// returned as the text it is: a run that died before printing its envelope
// still leaves what it managed on stdout, and that is worth saving as it is
// rather than replaced by a parse error. The cost is then unknown, and every
// place that prints a cost says so rather than printing zero.
func unwrap(stdout string) phaseResult {
	var e envelope
	if err := json.Unmarshal([]byte(stdout), &e); err != nil || e.Result == "" && e.Cost == nil {
		return phaseResult{out: stdout, cost: -1}
	}
	r := phaseResult{out: e.Result, cost: -1}
	if e.Cost != nil {
		r.cost = *e.Cost
	}
	return r
}

// formatCost is the one way a cost is written, in the PR body and in the
// notification, so an unknown one reads the same everywhere.
func formatCost(usd float64) string {
	if usd < 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.2f USD", usd)
}

// sumCost adds phase costs; one unknown makes the sum unknown, because a
// partial total printed as a total is a lie about the cheaper half.
func sumCost(costs ...float64) float64 {
	total := 0.0
	for _, c := range costs {
		if c < 0 {
			return -1
		}
		total += c
	}
	return total
}

func runPhase(ctx context.Context, cfg *Config, dir string, p phase) (phaseResult, error) {
	system, err := skill(p.skill)
	if err != nil {
		return phaseResult{cost: -1}, err
	}
	args := []string{
		"--print", p.prompt,
		"--model", cfg.Agent.ModelReview,
		// json rather than text for one field: total_cost_usd. The text is
		// taken out of the envelope by unwrap.
		"--output-format", "json",
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
	r := unwrap(res.stdout)
	if err != nil {
		return r, fmt.Errorf("the %s phase failed: %w", p.name, err)
	}
	return r, nil
}
