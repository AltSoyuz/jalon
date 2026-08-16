package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Recap is the Sunday evening read: for each target on a machine, what waits
// on a person, and the two numbers of the work kill criterion. No model. It
// gathers what is already measured and only needed reading; what to do next
// comes out of what moved and what waits, read by a person. A model that read
// digests and wrote tasks would be the backlog nobody understands, and that
// stays refused.
type RecapOptions struct {
	Days    int      // the window
	Metrics string   // the JALON_METRICS file; "" means none
	Notify  string   // a command receiving the recap on stdin; "" means stdout only
	Repos   []string // repository roots
	Now     time.Time
}

// staleAfter is when a doing task stops being work in progress and starts
// being drift. A fortnight, because a task is one coherent deliverable and
// two weeks of doing is either two deliverables or none.
const staleAfter = 14 * 24 * time.Hour

func Recap(ctx context.Context, env Env, opt RecapOptions) error {
	if opt.Days <= 0 {
		opt.Days = 7
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	for _, tool := range []string{"jalon", "gh", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("recap: %s is not on PATH", tool)
		}
	}
	var b strings.Builder
	host, _ := os.Hostname()
	fmt.Fprintf(&b, "# jalon recap, %s, last %d days, %s\n\n", opt.Now.UTC().Format("2006-01-02"), opt.Days, host)
	for _, repo := range opt.Repos {
		recapRepo(ctx, &b, repo, opt)
	}
	recapMachine(&b, opt)

	out := b.String()
	fmt.Fprint(env.Stdout, out)
	if opt.Notify != "" {
		if _, err := run(ctx, runOpts{name: "sh", args: []string{"-c", opt.Notify}, stdin: out, timeout: 60 * time.Second}); err != nil {
			return fmt.Errorf("recap: the notification command failed: %w", err)
		}
	}
	return nil
}

func recapRepo(ctx context.Context, b *strings.Builder, repo string, opt RecapOptions) {
	name := filepath.Base(repo)
	fmt.Fprintf(b, "## %s\n\n", name)
	tasks := filepath.Join(repo, ".tasks")
	if _, err := os.Stat(tasks); err != nil {
		fmt.Fprintf(b, "- no .tasks directory\n\n")
		return
	}
	slug := repoSlug(ctx, repo)

	// Open tasks, and the ones doing for longer than a fortnight: that is the
	// signal of drift, not the count.
	doing := listTasks(ctx, tasks, "doing")
	todo := listTasks(ctx, tasks, "todo")
	fmt.Fprintf(b, "- open: %d doing, %d todo\n", len(doing), len(todo))
	for _, t := range doing {
		if age := opt.Now.Sub(idDate(t.id)); age > staleAfter {
			fmt.Fprintf(b, "  - doing for %d days: %s\n", int(age.Hours()/24), t.id)
		}
	}
	// Agreed and never queued: a merged proposal whose status nobody moved to
	// implement is a decision waiting, and it waits silently.
	for _, t := range listTasks(ctx, tasks, "proposed") {
		fmt.Fprintf(b, "- proposed, not queued: %s (%s)\n", t.id, t.title)
	}
	for _, st := range []string{StatusMeasure, StatusImplement} {
		for _, t := range listTasks(ctx, tasks, st) {
			fmt.Fprintf(b, "- queued %s: %s\n", st, t.id)
		}
	}
	// Pull requests the agent opened and nobody merged or closed.
	if slug != "" {
		for _, pr := range openAgentPRs(ctx, repo, slug) {
			fmt.Fprintf(b, "- pull request waiting since %s: %s\n", pr.CreatedAt[:min(10, len(pr.CreatedAt))], pr.URL)
		}
	}
	// Wrecks: each one is a failed job whose task stays out of the queue until
	// a person reads it and removes it.
	if entries, err := os.ReadDir(filepath.Join(repo, filepath.FromSlash(failedDir))); err == nil {
		for _, e := range entries {
			fmt.Fprintf(b, "- wreck to read: %s/%s\n", failedDir, e.Name())
		}
	}
	// Decisions whose ground moved: a done task with a linked file changed
	// since the task closed. Candidates for a night skeptic; today, for a look.
	for _, t := range listTasks(ctx, tasks, "done") {
		closed, links := closedAndLinks(filepath.Join(tasks, t.id+".md"))
		if closed == "" || len(links) == 0 {
			continue
		}
		out, err := git(ctx, repo, append([]string{"log", "--since=" + closed, "--format=%h", "--"}, links...)...)
		if err != nil {
			continue
		}
		if n := len(strings.Fields(out)); n > 0 {
			fmt.Fprintf(b, "- decision ground moved: %s closed %s, %d commit(s) since on %s\n", t.id, closed, n, strings.Join(links, ","))
		}
	}
	// The first number of the work kill criterion, per repository: a work
	// branch carries exactly one commit, so a second one is a correction.
	if slug != "" {
		merged, untouched := mergedWorkBranches(ctx, repo, slug)
		if merged == 0 {
			b.WriteString("- work branches merged: none yet\n")
		} else {
			fmt.Fprintf(b, "- work branches merged (last %d): %d untouched by a person\n", merged, untouched)
		}
	}
	b.WriteString("\n")
}

// recapMachine is the second number, machine wide: the metrics line does not
// say which repository a job ran in, so this is the bill for the machine.
func recapMachine(b *strings.Builder, opt RecapOptions) {
	fmt.Fprintf(b, "## this machine, last %d days\n\n", opt.Days)
	if opt.Metrics == "" {
		b.WriteString("- no metrics file (set JALON_METRICS or pass -metrics)\n")
		return
	}
	f, err := os.Open(opt.Metrics)
	if err != nil {
		fmt.Fprintf(b, "- metrics file unreadable: %v\n", err)
		return
	}
	defer f.Close()
	since := opt.Now.Add(-time.Duration(opt.Days) * 24 * time.Hour).UTC().Format(time.RFC3339)
	var jobs, failed, reviews, works, noCost int
	var cost float64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var m struct {
			Time string   `json:"time"`
			Verb string   `json:"verb"`
			ID   string   `json:"id"`
			Err  string   `json:"err"`
			Cost *float64 `json:"cost_usd"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		// A tick that found nothing queued has no id, and is not a job.
		if m.ID == "" || m.Time < since || (m.Verb != "review" && m.Verb != "work") {
			continue
		}
		jobs++
		if m.Err != "" {
			failed++
		}
		if m.Verb == "review" {
			reviews++
		} else {
			works++
		}
		if m.Cost == nil {
			noCost++
		} else {
			cost += *m.Cost
		}
	}
	fmt.Fprintf(b, "- agent jobs: %d (%d published, %d failed)\n", jobs, jobs-failed, failed)
	fmt.Fprintf(b, "- cost: %.2f USD reported (%d job(s) reported none)\n", cost, noCost)
	fmt.Fprintf(b, "- per verb: review %d, work %d\n", reviews, works)
}

type listedTask struct{ id, status, title string }

// listTasks is the core's verb, run as a process: package agent does not
// import package main, and a second parser of the task list here would be a
// second thing to keep true. jalon is on PATH wherever a job runs, and it is
// what the probes call too.
func listTasks(ctx context.Context, tasks, status string) []listedTask {
	res, err := run(ctx, runOpts{name: "jalon", args: []string{"list", "-dir", tasks, "-status", status}, timeout: 30 * time.Second})
	if err != nil {
		return nil
	}
	var out []listedTask
	for line := range strings.SplitSeq(strings.TrimSpace(res.stdout), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		out = append(out, listedTask{id: f[0], status: f[1], title: strings.Join(f[2:], " ")})
	}
	return out
}

// idDate reads the day carried by a jalon id, YYMMDD-slug.
func idDate(id string) time.Time {
	if len(id) < 6 {
		return time.Time{}
	}
	t, err := time.Parse("060102", id[:6])
	if err != nil {
		return time.Time{}
	}
	return t
}

func repoSlug(ctx context.Context, repo string) string {
	url, err := git(ctx, repo, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")
	for _, p := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		if s, ok := strings.CutPrefix(url, p); ok {
			return s
		}
	}
	return ""
}

type prRef struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
	MergedAt    string `json:"mergedAt"`
}

func openAgentPRs(ctx context.Context, dir, slug string) []prRef {
	res, err := run(ctx, runOpts{dir: dir, name: "gh", timeout: 60 * time.Second,
		args: []string{"pr", "list", "-R", slug, "--state", "open", "--limit", "50", "--json", "headRefName,url,createdAt"}})
	if err != nil {
		return nil
	}
	var all []prRef
	if json.Unmarshal([]byte(res.stdout), &all) != nil {
		return nil
	}
	var out []prRef
	for _, pr := range all {
		if strings.HasPrefix(pr.HeadRefName, "task/") || strings.HasPrefix(pr.HeadRefName, "work/") {
			out = append(out, pr)
		}
	}
	return out
}

// mergedWorkBranches counts, over the last twenty merged work branches, the
// ones a person never touched. Two steps, because asking the forge for the
// commits of 200 pull requests in one query exceeds its node limit: list
// cheaply, then read the commits of the twenty that matter.
func mergedWorkBranches(ctx context.Context, dir, slug string) (merged, untouched int) {
	res, err := run(ctx, runOpts{dir: dir, name: "gh", timeout: 60 * time.Second,
		args: []string{"pr", "list", "-R", slug, "--state", "merged", "--limit", "200", "--json", "number,headRefName,mergedAt"}})
	if err != nil {
		return 0, 0
	}
	var all []prRef
	if json.Unmarshal([]byte(res.stdout), &all) != nil {
		return 0, 0
	}
	var work []prRef
	for _, pr := range all {
		if strings.HasPrefix(pr.HeadRefName, "work/") {
			work = append(work, pr)
		}
	}
	sort.Slice(work, func(i, j int) bool { return work[i].MergedAt < work[j].MergedAt })
	if len(work) > 20 {
		work = work[len(work)-20:]
	}
	for _, pr := range work {
		res, err := run(ctx, runOpts{dir: dir, name: "gh", timeout: 60 * time.Second,
			args: []string{"pr", "view", fmt.Sprint(pr.Number), "-R", slug, "--json", "commits"}})
		if err != nil {
			continue
		}
		var v struct {
			Commits []json.RawMessage `json:"commits"`
		}
		if json.Unmarshal([]byte(res.stdout), &v) != nil {
			continue
		}
		merged++
		if len(v.Commits) == 1 {
			untouched++
		}
	}
	return merged, untouched
}

// closedAndLinks reads the two things the ground-moved check needs from a
// done task: the date of its last "closed" log entry and its links. A line
// scan, not a parser, the same trade issueOf makes.
func closedAndLinks(path string) (closed string, links []string) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "links: ["); ok {
			v = strings.TrimSuffix(v, "]")
			for _, l := range strings.Split(v, ",") {
				if l = strings.TrimSpace(l); l != "" {
					links = append(links, l)
				}
			}
		}
		if strings.HasPrefix(line, "- ") && len(line) >= 12 && strings.Contains(line, "closed") {
			if _, err := time.Parse("2006-01-02", line[2:12]); err == nil {
				closed = line[2:12]
			}
		}
	}
	return closed, links
}
