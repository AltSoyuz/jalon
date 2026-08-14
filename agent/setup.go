package agent

import (
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/template"
)

// jalon emits the setup, it never performs it.
//
// The alternative was a `jalon setup` that creates the system user and writes
// into /etc/systemd/system. That needs root, in the one binary whose entire
// blast radius argument is that the agent user has no sudo, and it would put
// Debian specifics into a tool that has no operating system specific line and
// cross compiles to nine platforms. It also runs fewer than ten times over the
// life of the tool, which is the wrong side of the ratio for a provisioning
// subsystem.
//
// So the output is text. It is inspectable before it runs, diff-able against
// what is deployed, and composes with sh, sudo and tee. The privileged part is
// a clearly marked block that a person runs.
//
// The templates are embedded rather than shipped as files in the repository for
// one reason: the deployment unit is the scp'd binary, not the repository. A
// unit file the operator has to clone jalon to find is a unit file they do not
// have. The skills are embedded for the same reason.
//
//go:embed setup/*.tmpl
var setupFS embed.FS

// InitOptions is what a machine cannot know about itself.
type InitOptions struct {
	Repo   string // absolute path of the target repository
	User   string // the dedicated system user the job runs as
	Port   int    // the local port of the app to probe, 0 for none
	Branch string
	// EnvFile is an optional machine secrets file the unit reads, for the case
	// where the model credentials are a token rather than a stored login.
	// jalon references it and never writes it: the secret stays yours.
	EnvFile string
}

func (o *InitOptions) validate() error {
	switch {
	case o.Repo == "":
		return fmt.Errorf("-repo is required: the absolute path of the repository the agent works in")
	case !filepath.IsAbs(o.Repo):
		return fmt.Errorf("-repo %q must be absolute: a systemd unit has no working directory to be relative to", o.Repo)
	case o.User == "":
		return fmt.Errorf("-user is required: the dedicated system user, which must not be your own and must have no sudo")
	case o.Port < 0 || o.Port > 65535:
		return fmt.Errorf("-port %d is not a port", o.Port)
	}
	if o.EnvFile != "" {
		if !filepath.IsAbs(o.EnvFile) {
			return fmt.Errorf("-env-file %q must be absolute: systemd resolves it from no working directory", o.EnvFile)
		}
		// The file holds a token. Inside the repository it is one `git add -A`
		// away from being published, and jalon never writes it either way.
		if strings.HasPrefix(filepath.Clean(o.EnvFile)+string(filepath.Separator), filepath.Clean(o.Repo)+string(filepath.Separator)) {
			return fmt.Errorf("-env-file %q is inside the repository: a secret there is one commit away from being published, put it under /etc", o.EnvFile)
		}
	}
	if o.Branch == "" {
		o.Branch = "main"
	}
	return nil
}

// Init writes the configuration, the systemd unit and timer, and the commands
// that install them, to w. It touches nothing.
func Init(w io.Writer, version string, opt InitOptions) error {
	if err := opt.validate(); err != nil {
		return err
	}
	tmpl, err := template.New("setup").Funcs(template.FuncMap{"quote": quote}).ParseFS(setupFS, "setup/*.tmpl")
	if err != nil {
		return err
	}
	data := struct {
		InitOptions
		Version   string
		Name      string
		Unit      string
		ConfigDir string
		Config    string
		Probe     string
	}{
		InitOptions: opt,
		Version:     version,
		Name:        filepath.Base(opt.Repo),
		// One unit per target. The name used to be fixed, so setting up a
		// second repository silently replaced the first one's unit.
		Unit:      "jalon-agent-" + unitSafe(filepath.Base(opt.Repo)),
		ConfigDir: configDir,
		Config:    filepath.Join(configDir, configName),
	}
	if opt.Port > 0 {
		data.Probe = fmt.Sprintf("curl -s http://localhost:%d/healthz", opt.Port)
	}
	return tmpl.ExecuteTemplate(w, "setup.sh.tmpl", data)
}

// unitSafe keeps only what systemd accepts in a unit name, so a repository
// directory cannot produce a unit that will not load.
func unitSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// quote is available to the templates so a path with a space cannot split a
// generated command in two.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
