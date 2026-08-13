package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func emit(t *testing.T, opt InitOptions) string {
	t.Helper()
	var b strings.Builder
	if err := Init(&b, "test", opt); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// heredoc returns the body a heredoc writes, which is how the generated script
// carries the config and the units. It matches on the opener rather than on the
// filename: the paths are also mentioned in comments above them.
func heredoc(t *testing.T, script, opener, terminator string) string {
	t.Helper()
	_, rest, ok := strings.Cut(script, opener)
	if !ok {
		t.Fatalf("the script has no %q heredoc:\n%s", opener, script)
	}
	body, _, ok := strings.Cut(rest, "\n"+terminator+"\n")
	if !ok {
		t.Fatalf("the %q heredoc is never closed", opener)
	}
	return strings.TrimPrefix(body, "\n")
}

func emittedConfig(t *testing.T, script string) string {
	t.Helper()
	return heredoc(t, script, "<<'TOML'", "TOML")
}

// The whole point of emitting rather than installing is that the output is
// text a person reads. It still has to be correct text: the configuration it
// writes must load through the same parser every other config goes through.
func TestInitEmitsALoadableConfig(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "jalon-agent", Port: 8080})

	cfg, err := loadConfig("generated", []byte(emittedConfig(t, script)))
	if err != nil {
		t.Fatalf("the emitted configuration does not load: %v", err)
	}
	if !cfg.Allowed("curl -s http://localhost:8080/healthz") {
		t.Errorf("the -port probe is missing from the allowlist: %v", cfg.Probes)
	}
	if cfg.Agent.ModelReview == "sonnet" || cfg.Agent.ModelReview == "opus" {
		t.Errorf("model_review = %q, want a pinned id: an alias retargets silently", cfg.Agent.ModelReview)
	}
}

// Without -port there is no app to probe, and the allowlist must not carry a
// curl aimed at nothing.
func TestInitWithoutPort(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "jalon-agent"})
	cfg, err := loadConfig("generated", []byte(emittedConfig(t, script)))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cfg.Probes {
		if strings.HasPrefix(p, "curl") {
			t.Errorf("no port was given but the allowlist has %q", p)
		}
	}
}

// The generated script is run by a person, so a syntax error in it is a defect
// jalon shipped, not one they made.
func TestInitEmitsValidShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for this test")
	}
	path := filepath.Join(t.TempDir(), "setup.sh")
	script := emit(t, InitOptions{Repo: "/srv/an app", User: "jalon-agent", Port: 8080})
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-n", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the generated script is not valid shell: %v\n%s", err, out)
	}
	// A path with a space must not split the command in two.
	if !strings.Contains(script, `REPO='/srv/an app'`) {
		t.Errorf("the repository path is not quoted:\n%s", script)
	}
}

// The privileged half is gated behind an explicit flag: running the script the
// obvious way must not create a user or write into /etc.
func TestInitKeepsRootWorkBehindAFlag(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "jalon-agent"})

	head, tail, ok := strings.Cut(script, `if [ "${1-}" = "--root" ]; then`)
	if !ok {
		t.Fatal("the root section is not gated behind --root")
	}
	for _, privileged := range []string{"useradd", "/etc/systemd/system", "systemctl"} {
		if strings.Contains(head, privileged) {
			t.Errorf("%q appears before the --root gate", privileged)
		}
		if !strings.Contains(tail, privileged) {
			t.Errorf("%q is missing from the root section", privileged)
		}
	}
	// And the way back out is stated, because every change must be reversible.
	if !strings.Contains(script, "systemctl disable --now jalon-agent.timer") {
		t.Errorf("the script does not say how to undo itself:\n%s", script)
	}
}

// The unit is the part most likely to be wrong on a real machine, and PATH is
// the most common reason a job that works by hand fails under systemd.
func TestInitUnitCarriesPathAndUser(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "agentuser"})
	unit := heredoc(t, script, "jalon-agent.service <<'UNIT'", "UNIT")

	for _, want := range []string{
		"User=agentuser",
		"WorkingDirectory=/srv/app",
		"Environment=PATH=",
		"Type=oneshot",
		"NoNewPrivileges=yes",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("the unit is missing %q:\n%s", want, unit)
		}
	}
	// The unit must only invoke a verb that exists.
	if !strings.Contains(unit, "jalon review -next") {
		t.Errorf("the unit does not run a real command:\n%s", unit)
	}
}

func TestInitRefusals(t *testing.T) {
	cases := []struct {
		name string
		opt  InitOptions
		want string
	}{
		{"no repo", InitOptions{User: "u"}, "-repo is required"},
		{"relative repo", InitOptions{Repo: "srv/app", User: "u"}, "must be absolute"},
		{"no user", InitOptions{Repo: "/srv/app"}, "-user is required"},
		{"bad port", InitOptions{Repo: "/srv/app", User: "u", Port: 70000}, "is not a port"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			err := Init(&b, "test", c.opt)
			if err == nil {
				t.Fatalf("%s was accepted", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}
