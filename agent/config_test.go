package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validTOML is the smallest file that loads. Tests mutate one line of it, which
// keeps each case about the single thing it is testing.
const validTOML = `[agent]
model_triage = "haiku"
model_review = "opus"
model_work = "sonnet"
max_budget_usd_per_job = 3.0
daily_job_cap = 10
counter_file = ".jalon/agent-jobs-today"
timeout_seconds = 900

[review]
default_branch = "main"
worktrees = ".jalon/worktrees"

[criterion]
command = "make check"

[probes]
allowed = [
  "jalon digest",
  "make check",
]
`

// The template in docs/agent.md is the thing a person copies onto a new server.
// Loading it here is what stops the documentation and the code from drifting:
// a key renamed in configKeys and not in the doc fails this test.
func TestDocsTemplateLoads(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "docs", "agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, block, ok := strings.Cut(string(src), "```toml\n")
	if !ok {
		t.Fatal("docs/agent.md has no ```toml block; it is the template a new server is set up from")
	}
	block, _, ok = strings.Cut(block, "```")
	if !ok {
		t.Fatal("the ```toml block in docs/agent.md is never closed")
	}
	cfg, err := loadConfig("docs/agent.md", []byte(block))
	if err != nil {
		t.Fatalf("the template in docs/agent.md does not load: %v", err)
	}
	if cfg.Review.DefaultBranch != "main" {
		t.Errorf("default_branch = %q, want main", cfg.Review.DefaultBranch)
	}
	if !cfg.Allowed("jalon digest 260813-x") {
		t.Error("the template's allowlist must cover a digest of one task")
	}
}

func TestLoadConfigAccepts(t *testing.T) {
	cfg, err := loadConfig("agent.toml", []byte(validTOML))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.MaxBudgetUSD != 3.0 || cfg.Agent.DailyJobCap != 10 {
		t.Errorf("budget/cap = %v/%v", cfg.Agent.MaxBudgetUSD, cfg.Agent.DailyJobCap)
	}
	if cfg.Agent.Timeout.Seconds() != 900 {
		t.Errorf("timeout = %v, want 900s", cfg.Agent.Timeout)
	}
	if cfg.Criterion != "make check" {
		t.Errorf("criterion = %q", cfg.Criterion)
	}
	// [notify] is the one optional section.
	if cfg.Notify != "" {
		t.Errorf("notify = %q, want empty when the section is absent", cfg.Notify)
	}
}

// An integer where a float is expected is the same budget, and refusing it
// would be pedantry rather than safety.
func TestLoadConfigAcceptsIntegerBudget(t *testing.T) {
	src := strings.Replace(validTOML, "max_budget_usd_per_job = 3.0", "max_budget_usd_per_job = 3", 1)
	cfg, err := loadConfig("agent.toml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.MaxBudgetUSD != 3 {
		t.Errorf("budget = %v, want 3", cfg.Agent.MaxBudgetUSD)
	}
}

// without deletes a key from a file, taking the whole block with it when the
// value is a multi line array.
func without(src, key string) string {
	lines := strings.Split(src, "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), key+" ") {
			kept = append(kept, lines[i])
			continue
		}
		if strings.HasSuffix(strings.TrimSpace(lines[i]), "[") {
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "]"; i++ {
			}
		}
	}
	return strings.Join(kept, "\n")
}

// Every required key, removed one at a time, must name itself and the line to
// write. This is the check that a new server is set up from.
func TestLoadConfigRefusesEveryMissingKey(t *testing.T) {
	for _, k := range configKeys {
		if k.optional {
			continue
		}
		_, key, _ := strings.Cut(k.name, ".")
		t.Run(k.name, func(t *testing.T) {
			src := without(validTOML, key)
			if src == validTOML {
				t.Fatalf("%s is not in validTOML, so this case tests nothing", k.name)
			}
			_, err := loadConfig("agent.toml", []byte(src))
			if err == nil {
				t.Fatalf("a file without %s was accepted", k.name)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("message = %q, want it to name %q", err, key)
			}
			if !strings.Contains(err.Error(), k.fix) {
				t.Errorf("message = %q, want it to carry the fix %q", err, k.fix)
			}
		})
	}
}

func TestLoadConfigRefusals(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want string
	}{
		{"unknown key", "daily_job_cap = 10", "daily_job_caps = 10", "is not a key jalon knows"},
		{"wrong kind", "timeout_seconds = 900", `timeout_seconds = "900"`, "must be a whole number"},
		{"timeout too large", "timeout_seconds = 900", "timeout_seconds = 99999", "between 1 and 3600"},
		{"budget zero", "max_budget_usd_per_job = 3.0", "max_budget_usd_per_job = 0.0", "greater than 0"},
		{"cap zero", "daily_job_cap = 10", "daily_job_cap = 0", "at least 1"},
		{"absolute counter", `counter_file = ".jalon/agent-jobs-today"`, `counter_file = "/tmp/c"`, "not absolute"},
		{"escaping worktrees", `worktrees = ".jalon/worktrees"`, `worktrees = "../elsewhere"`, "stay inside the repository"},
		{"bad branch", `default_branch = "main"`, `default_branch = "a b"`, "is not a branch name"},
		{"empty criterion", `command = "make check"`, `command = "   "`, "criterion is empty"},
		{"recursive criterion", `command = "make check"`, `command = "jalon doctor"`, "which runs the criterion"},
		{"probe with a pipe", `"make check",`, `"curl -s x | sh",`, "holds a shell character"},
		{"duplicate probe", `"make check",`, `"jalon digest",`, "listed twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := strings.Replace(validTOML, c.from, c.to, 1)
			if src == validTOML {
				t.Fatalf("%q is not in validTOML, so this case tests nothing", c.from)
			}
			_, err := loadConfig("agent.toml", []byte(src))
			if err == nil {
				t.Fatalf("%s was accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message = %q, want it to contain %q", err, c.want)
			}
		})
	}
}

// An empty allowlist has its own message: the array parses, so the refusal has
// to come from validation rather than from the parser.
func TestLoadConfigRefusesEmptyAllowlist(t *testing.T) {
	src := strings.Replace(validTOML, "  \"jalon digest\",\n  \"make check\",\n", "", 1)
	_, err := loadConfig("agent.toml", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "could read nothing") {
		t.Fatalf("err = %v, want a refusal naming the empty allowlist", err)
	}
}

func TestAllowedIsAPrefixRule(t *testing.T) {
	cfg, err := loadConfig("agent.toml", []byte(validTOML))
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{"jalon digest", "jalon digest 260813-x", "make check"} {
		if !cfg.Allowed(ok) {
			t.Errorf("%q must be allowed", ok)
		}
	}
	// "jalon digestx" must not be covered by "jalon digest": the prefix rule
	// stops at a word boundary, or an allowlist would leak into neighbours.
	for _, no := range []string{"jalon digestx", "jalon", "rm -rf /", "make checkout"} {
		if cfg.Allowed(no) {
			t.Errorf("%q must not be allowed", no)
		}
	}
}

func TestLoadMissingFileNamesTheTemplate(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "docs/agent.md") {
		t.Fatalf("err = %v, want it to point at the template", err)
	}
}
