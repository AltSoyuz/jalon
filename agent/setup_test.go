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
	if !strings.Contains(script, "systemctl disable --now jalon-agent-app.timer") {
		t.Errorf("the script does not say how to undo itself:\n%s", script)
	}
}

// The unit is the part most likely to be wrong on a real machine, and PATH is
// the most common reason a job that works by hand fails under systemd.
func TestInitUnitCarriesPathAndUser(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "agentuser"})
	unit := heredoc(t, script, ".service <<'UNIT'", "UNIT")

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

// A service does not inherit a shell's PATH, and the emitted one has to cover
// the binaries the job actually runs: the model CLI in a user directory, and
// the toolchain the criterion needs. Getting this wrong is the most common way
// a job that works by hand fails under systemd.
func TestInitUnitPathCoversTheToolchain(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "agentuser"})
	unit := heredoc(t, script, ".service <<'UNIT'", "UNIT")

	var path string
	for _, line := range strings.Split(unit, "\n") {
		if rest, ok := strings.CutPrefix(line, "Environment=PATH="); ok {
			path = rest
		}
	}
	if path == "" {
		t.Fatalf("the unit sets no PATH:\n%s", unit)
	}
	for _, want := range []string{
		"/home/agentuser/.local/bin", // where claude installs itself
		"/usr/local/go/bin",          // where the official Go tarball lands
		"/usr/bin",
	} {
		if !strings.Contains(path, want) {
			t.Errorf("PATH is missing %s: %s", want, path)
		}
	}
}

// The secrets file is referenced, never written: jalon must not put a token
// into a generated script that people paste around.
func TestInitEnvFileIsReferencedNotWritten(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "u", EnvFile: "/etc/jalon-agent.env"})

	unit := heredoc(t, script, ".service <<'UNIT'", "UNIT")
	if !strings.Contains(unit, "EnvironmentFile=/etc/jalon-agent.env") {
		t.Errorf("the unit does not read the secrets file:\n%s", unit)
	}
	// Nothing in the script may write it, and the root block must refuse when
	// it is absent or world readable rather than let the timer fail later.
	for _, forbidden := range []string{"CLAUDE_CODE_OAUTH_TOKEN=sk", "> /etc/jalon-agent.env"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("the script writes the secrets file (%q):\n%s", forbidden, script)
		}
	}
	for _, want := range []string{"missing /etc/jalon-agent.env", "is not 0600"} {
		if !strings.Contains(script, want) {
			t.Errorf("the root block does not refuse on %q:\n%s", want, script)
		}
	}
}

// Absent the flag, nothing about a secrets file appears at all.
func TestInitWithoutEnvFile(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "u"})
	if strings.Contains(script, "EnvironmentFile") {
		t.Errorf("no -env-file was given but the unit reads one:\n%s", script)
	}
}

// A token inside the repository is one `git add -A` from being published.
func TestInitRefusesASecretInsideTheRepo(t *testing.T) {
	var b strings.Builder
	err := Init(&b, "test", InitOptions{Repo: "/srv/app", User: "u", EnvFile: "/srv/app/.env"})
	if err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Fatalf("err = %v, want a refusal naming the repository", err)
	}
}

// A fresh system user has no git identity, and a review only discovers it at
// the commit, after paying for three model calls. The setup gives it one.
func TestInitSetsAGitIdentity(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "jalon-agent"})

	if !strings.Contains(script, "git config --global user.name") {
		t.Fatalf("the setup never sets a git identity:\n%s", script)
	}
	// It belongs to the unprivileged half: it is the agent user's own config,
	// written into its own home, and needs no privilege at all.
	_, unprivileged, ok := strings.Cut(script, "# ---------- UNPRIVILEGED ----------")
	if !ok {
		t.Fatal("the script has no unprivileged section")
	}
	if !strings.Contains(unprivileged, "git config --global user.name") {
		t.Error("the identity is set in the root block, but it is the agent user's own config")
	}
	// Only when unset, so re-running never overwrites a chosen identity.
	if !strings.Contains(script, `if [ -z "$(git config user.name || true)" ]; then`) {
		t.Errorf("the identity is set unconditionally, so a re-run would clobber it:\n%s", script)
	}
}

// The job updates itself, so a merge reaches the server without a second copy
// of the binary to keep in step. The alternative was a jalon update verb, which
// would have duplicated gh and git in Go for one usage.
func TestInitUnitUpdatesItself(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "u"})
	unit := heredoc(t, script, ".service <<'UNIT'", "UNIT")

	// A network blip must not fail the tick hourly, so the pull carries the
	// systemd "ignore failure" prefix; the build must not, because code that
	// does not compile must not run.
	if !strings.Contains(unit, "ExecStartPre=-/usr/bin/git -C /srv/app pull --ff-only") {
		t.Errorf("the pull is missing, or is fatal, or invents merges:\n%s", unit)
	}
	if !strings.Contains(unit, "ExecStartPre=/usr/bin/make -C /srv/app build") {
		t.Errorf("the build is missing or non fatal:\n%s", unit)
	}
	// The binary is the one just built, in the checkout the agent owns: no
	// second copy anywhere, and no privileged install step.
	if !strings.Contains(unit, "ExecStart=/srv/app/bin/jalon review -next") {
		t.Errorf("the job does not run the binary it just built:\n%s", unit)
	}
	if strings.Contains(script, "/usr/local/bin/jalon") {
		t.Errorf("the setup still refers to a shared copy of the binary:\n%s", script)
	}
	// The model runs `jalon new` inside the worktree, so that same binary has
	// to be on the phase's PATH.
	if !strings.Contains(unit, "Environment=PATH=/srv/app/bin:") {
		t.Errorf("the built binary is not first on PATH:\n%s", unit)
	}
	// With the version moving on its own, the only way to say afterwards which
	// build produced which task is the metrics line.
	if !strings.Contains(unit, "Environment=JALON_METRICS=") {
		t.Errorf("nothing records which version ran:\n%s", unit)
	}
}

// The script tells the reader to edit the probes, so a regeneration that
// overwrote the file would destroy exactly the work it asked for.
func TestInitNeverOverwritesAnEditedConfig(t *testing.T) {
	script := emit(t, InitOptions{Repo: "/srv/app", User: "u"})

	// Written beside, then moved only when nothing is there.
	if !strings.Contains(script, "cat > '.jalon/agent.toml'.new <<'TOML'") {
		t.Errorf("the config is written directly over the existing one:\n%s", script)
	}
	for _, want := range []string{
		"if [ ! -f '.jalon/agent.toml' ]; then",
		"cmp -s '.jalon/agent.toml' '.jalon/agent.toml'.new",
		"left .jalon/agent.toml.new beside it",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the script is missing %q:\n%s", want, script)
		}
	}
}

// One unit per target. The name was fixed, so setting up a second repository
// silently replaced the first one's unit: on a machine already running a timer,
// the root block would have swapped the target out from under it.
func TestInitNamesTheUnitAfterTheTarget(t *testing.T) {
	a := emit(t, InitOptions{Repo: "/home/agent/altsoyuz.com", User: "u"})
	b := emit(t, InitOptions{Repo: "/home/agent/jalon", User: "u"})

	for _, want := range []string{
		"/etc/systemd/system/jalon-agent-altsoyuz.com.service",
		"/etc/systemd/system/jalon-agent-altsoyuz.com.timer",
		"systemctl enable --now jalon-agent-altsoyuz.com.timer",
	} {
		if !strings.Contains(a, want) {
			t.Errorf("the setup does not name the unit after the target (%q):\n%s", want, a)
		}
	}
	// The two must not collide anywhere, including in the revert instruction.
	if strings.Contains(a, "jalon-agent-jalon") || strings.Contains(b, "altsoyuz") {
		t.Error("the two targets share a unit name")
	}
	if !strings.Contains(b, "systemctl disable --now jalon-agent-jalon.timer") {
		t.Errorf("the revert instruction does not name this target's unit:\n%s", b)
	}
}

// A directory name is not a unit name: anything systemd will not load has to be
// replaced rather than emitted and discovered at install time.
func TestInitSanitisesTheUnitName(t *testing.T) {
	s := emit(t, InitOptions{Repo: "/srv/my app (old)", User: "u"})
	if !strings.Contains(s, "jalon-agent-my-app--old-.service") {
		t.Errorf("the unit name was not sanitised:\n%s", s)
	}
}
