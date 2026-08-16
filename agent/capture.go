package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Capture is the phone's way in: one ntfy topic, one line per idea, read at
// every tick and turned into a task stub in the repository the line names.
// The same topic carries jalon's own messages back (the acknowledgement, the
// pull requests, the recap), so the phone reads one thread: what a person
// writes has no title, what jalon writes always has one, and capture skips
// every titled message. One topic, one thread, no second place to look.
// No model chooses the repository: the first word does, because a word the
// owner already types costs less than a wrong guess found a day later. A
// line without a known prefix is sent back as a notification and nothing is
// created, so nothing is silently lost and nothing is silently invented.
type CaptureOptions struct {
	Inbox  string   // the topic URL, e.g. https://ntfy.example/inbox
	Token  string   // bearer token that may read the topic
	Cursor string   // file holding the id of the last message handled
	Notify string   // command receiving what could not be routed, on stdin
	Repos  []string // repository roots; the base name is the prefix a line uses
	Client *http.Client
}

type CaptureResult struct {
	Captured []Captured
	Unrouted []string
}

type Captured struct {
	Repo, ID, Status, PR string
}

// captureLine is "repo: idea" or "repo!: directive": the bang skips the
// review, for the directives the owner is sure about, as docs/agent.md says
// a directive does not need one.
var captureLine = regexp.MustCompile(`^\s*([A-Za-z0-9._-]+)(!?)\s*:\s*(.+?)\s*$`)

// maxPerTick bounds one run. The cursor moves as messages are handled, so the
// rest is picked up next tick; a burst cannot turn one tick into a hundred
// pushes.
const maxPerTick = 20

func Capture(ctx context.Context, env Env, opt CaptureOptions) (CaptureResult, error) {
	var res CaptureResult
	if opt.Inbox == "" {
		return res, errors.New("capture: an inbox topic URL is required")
	}
	if len(opt.Repos) == 0 {
		return res, errors.New("capture: at least one repository root is required")
	}
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: 60 * time.Second}
	}
	repos := make(map[string]string) // prefix -> root
	var names []string
	for _, r := range opt.Repos {
		name := filepath.Base(r)
		names = append(names, name)
		repos[strings.ToLower(name)] = r
		// altsoyuz.com answers to "altsoyuz" too: nobody types a TLD on a phone.
		if i := strings.IndexByte(name, '.'); i > 0 {
			repos[strings.ToLower(name[:i])] = r
		}
	}

	msgs, err := pollInbox(ctx, opt)
	if err != nil {
		return res, fmt.Errorf("capture: %w", err)
	}
	if len(msgs) > maxPerTick {
		fmt.Fprintf(env.Stderr, "jalon: %d messages in the inbox, handling %d this tick and the rest next\n", len(msgs), maxPerTick)
		msgs = msgs[:maxPerTick]
	}
	for _, m := range msgs {
		text := strings.TrimSpace(m.Message)
		if text == "" || m.Title != "" {
			if err := writeCursor(opt.Cursor, m.ID); err != nil {
				return res, fmt.Errorf("capture: %w", err)
			}
			continue
		}
		mm := captureLine.FindStringSubmatch(text)
		root := ""
		if mm != nil {
			root = repos[strings.ToLower(mm[1])]
		}
		if root == "" {
			res.Unrouted = append(res.Unrouted, text)
			back := fmt.Sprintf("no repository in: %q\nsend it again as \"<repo>: ...\" (or \"<repo>!: ...\" to skip the review); known: %s", text, strings.Join(names, ", "))
			fmt.Fprintf(env.Stderr, "jalon: %s\n", strings.ReplaceAll(back, "\n", " "))
			reply(ctx, env, opt, back)
			if err := writeCursor(opt.Cursor, m.ID); err != nil {
				return res, fmt.Errorf("capture: %w", err)
			}
			continue
		}
		status := StatusMeasure
		if mm[2] == "!" {
			status = StatusImplement
		}
		c, err := captureOne(ctx, env, root, mm[3], status, m)
		if err != nil {
			// The cursor stays: a push that failed is retried next tick, and
			// the error says which line and which repository.
			return res, fmt.Errorf("capture: %q into %s: %w", text, filepath.Base(root), err)
		}
		res.Captured = append(res.Captured, c)
		fmt.Fprintf(env.Stdout, "%s %s %s %s\n", c.Repo, c.ID, c.Status, c.PR)
		ack := fmt.Sprintf("captured: %s %s (%s)", c.Repo, c.ID, c.Status)
		if status == StatusImplement {
			ack += ", no review, built at the next tick"
		} else {
			ack += ", measured at the next tick"
		}
		if c.PR != "" {
			ack += "\nthe default branch is protected, merge the stub first: " + c.PR
		}
		reply(ctx, env, opt, ack)
		if err := writeCursor(opt.Cursor, m.ID); err != nil {
			return res, fmt.Errorf("capture: %w", err)
		}
	}
	return res, nil
}

// reply hands one message to the notify command, which posts it back into
// the thread with a title. A failed reply is reported and never fatal: the
// stub is already pushed, and losing an acknowledgement is not losing work.
func reply(ctx context.Context, env Env, opt CaptureOptions, msg string) {
	if opt.Notify == "" {
		return
	}
	if _, err := run(ctx, runOpts{name: "sh", args: []string{"-c", opt.Notify}, stdin: msg + "\n", timeout: 60 * time.Second}); err != nil {
		fmt.Fprintf(env.Stderr, "jalon: the notification command failed: %v\n", err)
	}
}

type inboxMessage struct {
	ID      string `json:"id"`
	Time    int64  `json:"time"`
	Event   string `json:"event"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// pollInbox reads what arrived since the cursor, oldest first. ntfy's
// since=<id> returns everything after that message; with no cursor, since=all
// returns the whole cache, which is what a first run wants: every idea sent
// before the machine existed is still an idea.
func pollInbox(ctx context.Context, opt CaptureOptions) ([]inboxMessage, error) {
	since := "all"
	if b, err := os.ReadFile(opt.Cursor); err == nil && strings.TrimSpace(string(b)) != "" {
		since = strings.TrimSpace(string(b))
	}
	url := strings.TrimRight(opt.Inbox, "/") + "/json?poll=1&since=" + since
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if opt.Token != "" {
		req.Header.Set("Authorization", "Bearer "+opt.Token)
	}
	resp, err := opt.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot read the inbox: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("the inbox answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var msgs []inboxMessage
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var m inboxMessage
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return nil, fmt.Errorf("the inbox returned unreadable json: %w", err)
		}
		if m.Event == "message" {
			msgs = append(msgs, m)
		}
	}
	return msgs, sc.Err()
}

func writeCursor(path, id string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(id+"\n"), 0o644)
}

// captureOne writes the stub in a fresh worktree of the target, cut from
// origin like every job, and pushes it to the default branch. A branch that
// cannot be pushed to (protected main) gets a task/<id> branch and a pull
// request instead, so the stub is never lost and the owner sees why it took
// one more click.
func captureOne(ctx context.Context, env Env, root, text, status string, m inboxMessage) (Captured, error) {
	c := Captured{Repo: filepath.Base(root), Status: status}
	cfg, err := Load(root)
	if err != nil {
		return c, err
	}
	wt, err := newWorktree(ctx, root, cfg, "capture-"+m.ID)
	if err != nil {
		return c, err
	}
	defer func() {
		if rerr := wt.remove(ctx); rerr != nil {
			fmt.Fprintf(env.Stderr, "jalon: %v\n", rerr)
		}
	}()
	before, err := taskNames(wt.path)
	if err != nil {
		return c, err
	}
	if _, err := run(ctx, runOpts{dir: wt.path, name: "jalon", timeout: 30 * time.Second,
		args: []string{"new", "-dir", filepath.Join(wt.path, ".tasks"), "-status", status, text}}); err != nil {
		return c, err
	}
	after, err := taskNames(wt.path)
	if err != nil {
		return c, err
	}
	for name := range after {
		if !before[name] {
			c.ID = strings.TrimSuffix(name, ".md")
		}
	}
	if c.ID == "" {
		return c, errors.New("jalon new created no task file")
	}
	sent := time.Unix(m.Time, 0).UTC().Format("2006-01-02 15:04")
	if _, err := run(ctx, runOpts{dir: wt.path, name: "jalon", timeout: 30 * time.Second,
		args: []string{"append", "-dir", filepath.Join(wt.path, ".tasks"), c.ID, "captured from the inbox, sent " + sent + " UTC"}}); err != nil {
		return c, err
	}
	msg := fmt.Sprintf("[%s] capture: %s\n\nQueued as %s from the inbox.\n", c.ID, text, status)
	if _, err := git(ctx, wt.path, "add", "--", ".tasks"); err != nil {
		return c, err
	}
	if _, err := git(ctx, wt.path, "commit", "-q", "-m", msg); err != nil {
		return c, err
	}
	if _, err := git(ctx, wt.path, "push", "-q", "origin", "HEAD:"+cfg.Review.DefaultBranch); err != nil {
		// Protected: a branch and a pull request, and the message says so.
		// capture/ and not task/, which review uses for the same id later;
		// forced, because a retry of the same line lands on the same branch.
		branch := "capture/" + c.ID
		if _, serr := git(ctx, wt.path, "switch", "-q", "-C", branch); serr != nil {
			return c, serr
		}
		if _, berr := git(ctx, wt.path, "push", "-q", "--force", "-u", "origin", branch); berr != nil {
			return c, fmt.Errorf("cannot push the stub to %s (%v) nor to %s: %w", cfg.Review.DefaultBranch, err, branch, berr)
		}
		pr, perr := createPR(ctx, wt.path, "Capture: "+c.ID, fmt.Sprintf("Task stub captured from the inbox, queued as `%s`. %s is protected, so this needs a merge; the stub is the whole change.\n", status, cfg.Review.DefaultBranch))
		if perr != nil {
			return c, fmt.Errorf("pushed %s but could not open its pull request: %w", branch, perr)
		}
		c.PR = pr
	}
	return c, nil
}

func taskNames(root string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".tasks", "*.md"))
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[filepath.Base(p)] = true
	}
	return set, nil
}
