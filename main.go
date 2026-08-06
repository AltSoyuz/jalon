// Command jalon is a file first task manager for humans and agents.
//
// The files under .tasks/ are the truth, git is the sync and the history, this
// binary is the only tool, and the HTML view is a full regeneration from those
// files. No server, no daemon, no external dependency.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// version is stamped at build time with -ldflags.
var version = "dev"

const tasksDirName = ".tasks"

const usage = `jalon is a file first task manager for humans and agents.

usage: jalon <command> [flags] [args]

commands:
  new <title>          create a task file, id YYMMDD-slug (-issue N seeds it
                       from a GitHub issue)
  list                 one line per task: id, status, title
  append <id> <text>   append one entry to the Log, or to Decisions
  digest <id>          print the whole task context as one block
  compact <id>         truncate the Log, report the token budget
  render               regenerate the static HTML view
  close <id>           set the status to done
  version              print the version

Commits are linked to a task by carrying its id in the message:

  git commit -m "[260806] fix the refresh token path"

Run "jalon <command> -h" for the flags of a command.
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(os.Stderr, "jalon: %v\n", err)
		}
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("missing command")
	}
	cmd, rest := args[0], args[1:]
	m := newMetric(cmd)
	var err error
	defer func() { m.flush(stderr, err) }()

	switch cmd {
	case "new":
		err = cmdNew(rest, stdout, stderr, m)
	case "append":
		err = cmdAppend(rest, stdout, stderr, m)
	case "digest":
		err = cmdDigest(rest, stdout, stderr, m)
	case "compact":
		err = cmdCompact(rest, stdout, stderr, m)
	case "list":
		err = cmdList(rest, stdout, stderr, m)
	case "render":
		err = cmdRender(rest, stdout, stderr, m)
	case "close":
		err = cmdClose(rest, stdout, stderr, m)
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		err = fmt.Errorf("unknown command %q (try: jalon help)", cmd)
	}
	return err
}

func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("jalon "+name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

// dirFlag is shared by every command: an explicit path wins, otherwise the
// nearest .tasks directory walking up from the current one, like git does.
func dirFlag(fs *flag.FlagSet) *string {
	return fs.String("dir", "", "tasks directory (default: nearest "+tasksDirName+" walking up)")
}

// checkFlagsAfterArgs refuses a known flag placed after a positional argument.
// The flag package stops parsing there and would silently append "-decision"
// to the text of an entry.
func checkFlagsAfterArgs(fs *flag.FlagSet) error {
	for _, a := range fs.Args() {
		trimmed := strings.TrimLeft(a, "-")
		if trimmed == a {
			continue
		}
		name, _, _ := strings.Cut(trimmed, "=")
		if fs.Lookup(name) != nil {
			return fmt.Errorf("flag -%s must come before the arguments (%s -%s <args>)", name, fs.Name(), name)
		}
	}
	return nil
}

func resolveDir(explicit string) (string, error) {
	if explicit != "" {
		if fi, err := os.Stat(explicit); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("%s is not a directory", explicit)
		}
		return explicit, nil
	}
	return findTasksDir(".")
}

func findTasksDir(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, tasksDirName)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s directory found from %s upwards (create one with: jalon new <title>)", tasksDirName, start)
		}
		dir = parent
	}
}

// repoRoot is the parent of the tasks directory: link paths and git commands
// are relative to it.
func repoRoot(tasksDir string) string {
	abs, err := filepath.Abs(tasksDir)
	if err != nil {
		return filepath.Dir(tasksDir)
	}
	return filepath.Dir(abs)
}

// resolveTask accepts a full id or its date prefix, and refuses to guess when
// the prefix matches several tasks.
func resolveTask(dir, ref string) (*Task, error) {
	if ref == "" {
		return nil, errors.New("a task id is required")
	}
	// The lookup is a glob, so a metacharacter in the reference would silently
	// match something else. Ids never contain one.
	if i := strings.IndexAny(ref, `*?[]\`); i >= 0 {
		return nil, fmt.Errorf("invalid task id %q: %q is not allowed in an id", ref, ref[i])
	}
	matches, err := filepath.Glob(filepath.Join(dir, ref+"*.md"))
	if err != nil {
		return nil, err
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no task matching %q in %s", ref, dir)
	case 1:
		return ParseTask(matches[0])
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = strings.TrimSuffix(filepath.Base(m), ".md")
		}
		return nil, fmt.Errorf("%q matches %d tasks: %s", ref, len(matches), strings.Join(names, ", "))
	}
}

// signature resolves who signs an entry, in an explicit order documented in
// the README: -sig, then $JALON_SIG, then git user.name, then $USER.
func signature(explicit, root string) string {
	if explicit != "" {
		return explicit
	}
	if s := os.Getenv("JALON_SIG"); s != "" {
		return s
	}
	if out, err := gitOutput(root, "config", "user.name"); err == nil {
		if fields := strings.Fields(out); len(fields) > 0 {
			return strings.ToLower(fields[0])
		}
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

func cmdNew(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("new", stderr)
	dir := dirFlag(fs)
	status := fs.String("status", statusTodo, "initial status")
	issueNum := fs.Int("issue", 0, "seed the task from a GitHub issue (one gh call, snapshot, no sync)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	d := *dir
	if d == "" {
		found, err := findTasksDir(".")
		if err != nil {
			d = tasksDirName
		} else {
			d = found
		}
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}

	var issue *ghIssue
	if *issueNum > 0 {
		var err error
		if issue, err = fetchIssue(repoRoot(d), *issueNum); err != nil {
			return fmt.Errorf("new: %w", err)
		}
		if title == "" {
			title = issue.Title
		}
	}
	if title == "" {
		return errors.New("new: a title is required")
	}

	now := time.Now()
	id, err := NewID(now, title)
	if err != nil {
		return fmt.Errorf("new: %w", err)
	}
	t, err := CreateTask(d, id, title, *status, now)
	if err != nil {
		return fmt.Errorf("new: %w", err)
	}
	if issue != nil {
		t.FM.Set(fmKeyIssue, fmt.Sprint(issue.Number))
		body := strings.TrimSpace(issue.Body)
		if body == "" {
			body = "(the issue has no body)"
		}
		if err := t.SetSection(sectionContext, fmt.Sprintf("From %s\n\n%s", issue.URL, body)); err != nil {
			return fmt.Errorf("new: %w", err)
		}
		if err := t.Save(); err != nil {
			return err
		}
	}
	m.ID = t.ID
	fmt.Fprintln(stdout, t.Path)
	return nil
}

func cmdAppend(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("append", stderr)
	dir := dirFlag(fs)
	sig := fs.String("sig", "", "signature (default: $JALON_SIG, then git user.name, then $USER)")
	decision := fs.Bool("decision", false, "append to Decisions instead of Log")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("append: usage: jalon append [-decision] <id> <text>")
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	t, err := resolveTask(d, fs.Arg(0))
	if err != nil {
		return err
	}
	m.ID = t.ID
	section := sectionLog
	if *decision {
		section = sectionDecisions
	}
	entry := FormatEntry(time.Now(), signature(*sig, repoRoot(d)), strings.Join(fs.Args()[1:], " "))
	if err := t.AppendEntry(section, entry); err != nil {
		return err
	}
	if err := t.Save(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s %s: %s\n", t.ID, strings.ToLower(section), entry)
	return nil
}

func cmdClose(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("close", stderr)
	dir := dirFlag(fs)
	sig := fs.String("sig", "", "signature")
	fromMerge := fs.Bool("from-merge", false, "read a commit message on stdin and close every \"closes <id>\" found")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	who := signature(*sig, repoRoot(d))

	refs := fs.Args()
	if *fromMerge {
		msg, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		refs = closesRefs(string(msg))
		if len(refs) == 0 {
			return nil
		}
	}
	if len(refs) == 0 {
		return errors.New("close: usage: jalon close <id>")
	}
	for _, ref := range refs {
		t, err := resolveTask(d, ref)
		if err != nil {
			// Reading a merge message is not the same situation as typing an
			// id: the merge already happened, and one unusable reference must
			// not discard the good ones that follow it.
			if *fromMerge {
				fmt.Fprintf(stderr, "jalon: %v; write a longer id in the merge message\n", err)
				continue
			}
			return err
		}
		if t.Status() == statusDone {
			fmt.Fprintf(stdout, "%s already %s\n", t.ID, statusDone)
			continue
		}
		t.FM.Set("status", statusDone)
		if err := t.AppendEntry(sectionLog, FormatEntry(time.Now(), who, "closed.")); err != nil {
			return err
		}
		if err := t.Save(); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s %s\n", t.ID, statusDone)
	}
	return nil
}

var closesRe = regexp.MustCompile(`(?i)\bcloses\s+\[?(\d{6}[a-z0-9-]*)\]?`)

func closesRefs(msg string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, m := range closesRe.FindAllStringSubmatch(msg, -1) {
		ref := strings.TrimRight(m[1], "-")
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}

func cmdCompact(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("compact", stderr)
	dir := dirFlag(fs)
	keep := fs.Int("keep", 10, "log entries to keep")
	budget := fs.Int("max-tokens", 5000, "token budget for the whole file (estimated as bytes/4)")
	check := fs.Bool("check", false, "report only, exit 1 when over budget, change nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	t, err := resolveTask(d, fs.Arg(0))
	if err != nil {
		return err
	}
	m.ID = t.ID
	before := EstimateTokens(len(t.Bytes()))
	if *check {
		fmt.Fprintf(stdout, "%s: ~%d tokens, budget %d\n", t.ID, before, *budget)
		if before > *budget {
			return fmt.Errorf("%s is over the token budget (~%d > %d); run jalon compact %s", t.ID, before, *budget, t.ID)
		}
		return nil
	}
	removed, err := t.TruncateSection(sectionLog, *keep)
	if err != nil {
		return err
	}
	if removed > 0 {
		if err := t.Save(); err != nil {
			return err
		}
	}
	after := EstimateTokens(len(t.Bytes()))
	fmt.Fprintf(stdout, "%s: %d log entries truncated, ~%d tokens (was ~%d)\n", t.ID, removed, after, before)
	if after > *budget {
		// The Context rewrite is the one thing this tool cannot do for you:
		// it holds no model and will never call one.
		fmt.Fprintf(stderr, "jalon: %s is still over the budget (~%d > %d): rewrite the Context section, it is the only section meant to be rewritten\n", t.ID, after, *budget)
	}
	return nil
}

func cmdDigest(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("digest", stderr)
	dir := dirFlag(fs)
	logKeep := fs.Int("log", 10, "log entries to include, newest last")
	maxFileBytes := fs.Int("max-file-bytes", 8192, "per linked file cap; truncation is always visible in the output")
	offline := fs.Bool("offline", false, "skip every gh call: no pull requests, no issue thread")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	t, err := resolveTask(d, fs.Arg(0))
	if err != nil {
		return err
	}
	m.ID = t.ID
	return digest(stdout, stderr, repoRoot(d), t, m, digestOpts{
		logKeep:      *logKeep,
		maxFileBytes: *maxFileBytes,
		withPRs:      !*offline,
		withIssue:    !*offline,
	})
}

// cmdList is the cheap half of orientation: about fifteen tokens per task to
// see the menu, against two thousand to digest one. It is what a session start
// hook runs, so its stdout is one line per task and nothing else.
func cmdList(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("list", stderr)
	dir := dirFlag(fs)
	status := fs.String("status", "", "keep only this status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkFlagsAfterArgs(fs); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	tasks, err := LoadTasks(d)
	if err != nil {
		return err
	}

	kept := tasks[:0]
	width := 0
	for _, t := range tasks {
		if *status != "" && t.Status() != *status {
			continue
		}
		kept = append(kept, t)
		if len(t.ID) > width {
			width = len(t.ID)
		}
	}
	m.Tasks = len(kept)
	if len(kept) == 0 {
		// An empty list is an answer, not a failure: stdout stays empty for the
		// caller, the explanation goes to stderr for the human.
		if *status != "" {
			fmt.Fprintf(stderr, "jalon: no task with status %q in %s\n", *status, d)
		} else {
			fmt.Fprintf(stderr, "jalon: no task in %s\n", d)
		}
		return nil
	}

	// Newest first, like the rendered index: ids sort chronologically.
	for i := len(kept) - 1; i >= 0; i-- {
		t := kept[i]
		fmt.Fprintf(stdout, "%-*s  %-6s  %s\n", width, t.ID, t.Status(), t.Title())
	}
	return nil
}

func cmdRender(args []string, stdout, stderr io.Writer, m *metric) error {
	fs := newFlagSet("render", stderr)
	dir := dirFlag(fs)
	out := fs.String("o", "", "output directory (default: <tasks dir>/site)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := resolveDir(*dir)
	if err != nil {
		return err
	}
	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(d, "site")
	}
	start := time.Now()
	n, err := renderSite(stderr, d, outDir)
	if err != nil {
		return err
	}
	m.Tasks = n
	fmt.Fprintf(stdout, "%d tasks rendered to %s in %s\n", n, outDir, time.Since(start).Round(time.Millisecond))
	return nil
}

// gitOutput runs git in dir and returns its stdout. Every caller treats an
// error as "no git here", warns, and keeps going: git is a source of context,
// never a requirement.
func gitOutput(dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", err
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func isGitRepo(dir string) bool {
	_, err := gitOutput(dir, "rev-parse", "--git-dir")
	return err == nil
}
