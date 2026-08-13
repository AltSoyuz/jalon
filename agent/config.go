package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configDir  = ".jalon"
	configName = "agent.toml"
)

// Config is the whole configuration surface of the agent layer. It is versioned
// in each target repository, because a value that decides what a model may run
// belongs where a reviewer can see it, not in an environment variable on one
// server.
type Config struct {
	Agent     AgentConfig
	Review    ReviewConfig
	Criterion string   // the command that proves the tree is healthy
	Probes    []string // the only commands a phase may run
	Notify    string   // optional: receives the notification on stdin
	Path      string   // the file this came from
}

type AgentConfig struct {
	ModelTriage  string
	ModelReview  string
	ModelWork    string
	MaxBudgetUSD float64
	DailyJobCap  int
	CounterFile  string
	Timeout      time.Duration
}

type ReviewConfig struct {
	DefaultBranch string
	Worktrees     string // directory, relative to the repository root
}

// configKey is one line of the file. The table below is walked in both
// directions: a key in the file that is not in the table is a refusal naming its
// line, and a key in the table that is not in the file is a refusal naming the
// line to write. There is no default anywhere, on purpose.
type configKey struct {
	name     string
	kind     tomlKind
	fix      string // the exact line to write when it is missing
	optional bool
	assign   func(*Config, tomlValue)
}

var configKeys = []configKey{
	{name: "agent.model_triage", kind: tomlString, fix: `model_triage = "haiku"`,
		assign: func(c *Config, v tomlValue) { c.Agent.ModelTriage = v.str }},
	{name: "agent.model_review", kind: tomlString, fix: `model_review = "opus"`,
		assign: func(c *Config, v tomlValue) { c.Agent.ModelReview = v.str }},
	{name: "agent.model_work", kind: tomlString, fix: `model_work = "sonnet"`,
		assign: func(c *Config, v tomlValue) { c.Agent.ModelWork = v.str }},
	{name: "agent.max_budget_usd_per_job", kind: tomlFloat, fix: `max_budget_usd_per_job = 3.0`,
		assign: func(c *Config, v tomlValue) { c.Agent.MaxBudgetUSD = v.f }},
	{name: "agent.daily_job_cap", kind: tomlInt, fix: `daily_job_cap = 10`,
		assign: func(c *Config, v tomlValue) { c.Agent.DailyJobCap = int(v.i) }},
	{name: "agent.counter_file", kind: tomlString, fix: `counter_file = ".jalon/agent-jobs-today"`,
		assign: func(c *Config, v tomlValue) { c.Agent.CounterFile = v.str }},
	{name: "agent.timeout_seconds", kind: tomlInt, fix: `timeout_seconds = 900`,
		assign: func(c *Config, v tomlValue) { c.Agent.Timeout = time.Duration(v.i) * time.Second }},

	{name: "review.default_branch", kind: tomlString, fix: `default_branch = "main"`,
		assign: func(c *Config, v tomlValue) { c.Review.DefaultBranch = v.str }},
	{name: "review.worktrees", kind: tomlString, fix: `worktrees = ".jalon/worktrees"`,
		assign: func(c *Config, v tomlValue) { c.Review.Worktrees = v.str }},

	{name: "criterion.command", kind: tomlString, fix: `command = "make check"`,
		assign: func(c *Config, v tomlValue) { c.Criterion = v.str }},

	{name: "probes.allowed", kind: tomlArray, fix: `allowed = [` + "\n  \"jalon digest\",\n]",
		assign: func(c *Config, v tomlValue) { c.Probes = v.arr }},

	// The only optional key. jalon knows one command, not one chat service: an
	// absent [notify] prints to stdout, which is what a journal already records.
	{name: "notify.command", kind: tomlString, fix: `command = "..."`, optional: true,
		assign: func(c *Config, v tomlValue) { c.Notify = v.str }},
}

// Load reads and validates <root>/.jalon/agent.toml.
func Load(root string) (*Config, error) {
	display := filepath.Join(configDir, configName)
	src, err := os.ReadFile(filepath.Join(root, configDir, configName))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%s does not exist; create it, the template is in docs/agent.md", display)
	case err != nil:
		return nil, err
	}
	c, err := loadConfig(display, src)
	if err != nil {
		return nil, err
	}
	c.Path = filepath.Join(root, configDir, configName)
	return c, nil
}

// loadConfig is Load without the filesystem, so that the template printed in
// docs/agent.md can be checked by a test without writing anything.
func loadConfig(display string, src []byte) (*Config, error) {
	doc, err := parseTOML(display, src)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(configKeys))
	names := make([]string, len(configKeys))
	for i, k := range configKeys {
		known[k.name] = true
		names[i] = k.name
	}
	// Unknown keys are reported first: a typo has to be read as a typo, not as
	// the correctly spelled key being absent.
	for _, name := range doc.keys {
		if !known[name] {
			return nil, fmt.Errorf("%s:%d: %s is not a key jalon knows; fix the spelling or delete the line (known keys: %s)",
				display, doc.vals[name].line, name, strings.Join(names, ", "))
		}
	}
	c := &Config{}
	for _, k := range configKeys {
		v, ok := doc.vals[k.name]
		if !ok {
			if k.optional {
				continue
			}
			section, key, _ := strings.Cut(k.name, ".")
			// The fix goes in unquoted so that it can be pasted as written.
			return nil, fmt.Errorf("%s: %s is missing; add this under [%s]: %s", display, key, section, k.fix)
		}
		if v.kind != k.kind {
			// An integer where a float is expected is a typo worth accepting:
			// 3 and 3.0 are the same budget, and refusing it would be pedantry.
			if k.kind == tomlFloat && v.kind == tomlInt {
				v.kind, v.f = tomlFloat, float64(v.i)
			} else {
				return nil, fmt.Errorf("%s:%d: %s must be %s; write %s", display, v.line, k.name, k.kind, k.fix)
			}
		}
		k.assign(c, v)
	}
	return c, c.validate(display, doc)
}

// shellMeta is refused in a probe because the allowlist is a prefix match: one
// entry holding a metacharacter would authorize a second, unlisted command.
const shellMeta = "|&;<>()$`\\\"'\n\t"

func (c *Config) validate(display string, doc *tomlDoc) error {
	at := func(key string) int { return doc.vals[key].line }

	if s := c.Agent.Timeout.Seconds(); s < 1 || s > 3600 {
		return fmt.Errorf("%s:%d: timeout_seconds must be between 1 and 3600; write timeout_seconds = 900",
			display, at("agent.timeout_seconds"))
	}
	if c.Agent.MaxBudgetUSD <= 0 {
		return fmt.Errorf("%s:%d: max_budget_usd_per_job must be greater than 0; write max_budget_usd_per_job = 3.0",
			display, at("agent.max_budget_usd_per_job"))
	}
	if c.Agent.DailyJobCap < 1 {
		return fmt.Errorf("%s:%d: daily_job_cap must be at least 1; write daily_job_cap = 10",
			display, at("agent.daily_job_cap"))
	}
	if err := insideRepo(c.Agent.CounterFile); err != nil {
		return fmt.Errorf("%s:%d: counter_file %v; write counter_file = \".jalon/agent-jobs-today\"",
			display, at("agent.counter_file"), err)
	}
	if err := insideRepo(c.Review.Worktrees); err != nil {
		return fmt.Errorf("%s:%d: worktrees %v; write worktrees = \".jalon/worktrees\"",
			display, at("review.worktrees"), err)
	}
	if b := c.Review.DefaultBranch; b == "" || strings.ContainsAny(b, " \t~^:?*[\\") {
		return fmt.Errorf("%s:%d: default_branch %q is not a branch name; write default_branch = \"main\"",
			display, at("review.default_branch"), b)
	}
	if strings.TrimSpace(c.Criterion) == "" {
		return fmt.Errorf("%s:%d: the criterion is empty; write the command that proves the tree is healthy, like command = \"make check\"",
			display, at("criterion.command"))
	}
	// A criterion that runs jalon doctor recurses: doctor runs the criterion.
	if strings.Contains(c.Criterion, "jalon doctor") {
		return fmt.Errorf("%s:%d: the criterion runs \"jalon doctor\", which runs the criterion; point it at the repository's own check instead, like command = \"make check\"",
			display, at("criterion.command"))
	}

	v := doc.vals["probes.allowed"]
	if len(c.Probes) == 0 {
		return fmt.Errorf("%s:%d: probes.allowed is empty, so a review could read nothing; list at least \"jalon digest\"",
			display, v.line)
	}
	seen := make(map[string]bool, len(c.Probes))
	for i, p := range c.Probes {
		line := v.arrLines[i]
		switch {
		case p == "" || strings.TrimSpace(p) != p:
			return fmt.Errorf("%s:%d: the probe %q has stray spaces; write it as the command you would type, like \"jalon digest\"", display, line, p)
		case strings.ContainsAny(p, shellMeta):
			return fmt.Errorf("%s:%d: the probe %q holds a shell character; a probe is one command, not a shell line, so list each command on its own line", display, line, p)
		case seen[p]:
			return fmt.Errorf("%s:%d: the probe %q is listed twice; delete this line", display, line, p)
		}
		seen[p] = true
	}
	return nil
}

// insideRepo refuses a path that would write outside the repository. jalon
// promises never to do that, and a configuration file is not a licence to.
func insideRepo(p string) error {
	switch {
	case p == "":
		return errors.New("is empty")
	case filepath.IsAbs(p):
		return errors.New("must be relative to the repository root, not absolute")
	case strings.HasPrefix(filepath.ToSlash(filepath.Clean(p)), "../"):
		return errors.New("must stay inside the repository")
	}
	return nil
}

// Allowed reports whether cmd is covered by the probe allowlist. A probe covers
// itself and anything that extends it with an argument, which is the same prefix
// rule the tool policy is built from: the two must never drift apart.
func (c *Config) Allowed(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, p := range c.Probes {
		if cmd == p || strings.HasPrefix(cmd, p+" ") {
			return true
		}
	}
	return false
}
