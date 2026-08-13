package agent

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The method ships inside the binary, so one scp carries the tool and the way it
// works together and they cannot drift. The skills version is the binary
// version.
//
// They are markdown on disk rather than Go string constants because a prompt is
// a thing people edit, diff and argue about in a pull request, and because
// materializing them is how the model actually reads them.
//
// What is NOT here: repository knowledge. That stays in each target repository
// (CLAUDE.md, docs/), and the agent finds it where it works. Duplicating it into
// jalon would be a second copy to keep true.
//
//go:embed skills/*.md
var skillsFS embed.FS

// materializeSkills writes each embedded skill into
// dst/.claude/skills/<name>/SKILL.md, which is where Claude Code looks for
// them. The names carry a jalon- prefix so a repository with its own skills
// cannot collide with these.
//
// The destination is always an ephemeral worktree: the skills die with it.
// Installing them under the agent user's home would be out-of-git state that
// drifts between servers.
func materializeSkills(dst string) ([]string, error) {
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return nil, err
	}
	var written []string
	for _, e := range entries {
		// path, not filepath: an embed.FS is always slash separated.
		b, err := skillsFS.ReadFile(path.Join("skills", e.Name()))
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(dst, ".claude", "skills", strings.TrimSuffix(e.Name(), ".md"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("cannot create %s: %w; check the permissions of %s", dir, err, dst)
		}
		p := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return nil, fmt.Errorf("cannot write %s: %w", p, err)
		}
		written = append(written, p)
	}
	return written, nil
}

// skill returns the body of one embedded skill, with its front matter stripped.
// It is what gets passed to --append-system-prompt, which is guaranteed to be in
// context, unlike a materialized skill the model may or may not choose to load.
func skill(name string) (string, error) {
	b, err := skillsFS.ReadFile(path.Join("skills", name+".md"))
	if err != nil {
		return "", fmt.Errorf("no embedded skill named %q: %w", name, err)
	}
	s := string(b)
	if rest, ok := strings.CutPrefix(s, "---\n"); ok {
		if _, body, found := strings.Cut(rest, "\n---\n"); found {
			s = body
		}
	}
	return strings.TrimSpace(s), nil
}
