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
	var repos []repoFacts
	for _, repo := range opt.Repos {
		repos = append(repos, gather(ctx, repo, opt))
	}
	out := render(repos, opt)
	fmt.Fprint(env.Stdout, out)
	if opt.Notify != "" {
		if _, err := run(ctx, runOpts{name: "sh", args: []string{"-c", opt.Notify}, stdin: out, timeout: 60 * time.Second}); err != nil {
			return fmt.Errorf("recap: the notification command failed: %w", err)
		}
	}
	return nil
}

// repoFacts is what one repository has to say, gathered first and rendered
// after, so the text can lead with what waits on a person across every
// repository and skip what has nothing to say.
type repoFacts struct {
	name           string
	noTasks        bool
	doing, todo    []listedTask
	stale          []listedTask // doing for longer than staleAfter
	proposed       []listedTask // agreed, never queued
	measure        []listedTask
	implement      []listedTask
	prs            []prRef
	wrecks         []string
	moved          []movedDecision
	merged, intact int // merged work branches, and those untouched by a person
	hasForge       bool
}

type movedDecision struct {
	task    listedTask
	closed  string
	commits int
	files   []string
}

func gather(ctx context.Context, repo string, opt RecapOptions) repoFacts {
	f := repoFacts{name: filepath.Base(repo)}
	tasks := filepath.Join(repo, ".tasks")
	if _, err := os.Stat(tasks); err != nil {
		f.noTasks = true
		return f
	}
	f.doing = listTasks(ctx, tasks, "doing")
	f.todo = listTasks(ctx, tasks, "todo")
	for _, t := range f.doing {
		if opt.Now.Sub(idDate(t.id)) > staleAfter {
			f.stale = append(f.stale, t)
		}
	}
	f.proposed = listTasks(ctx, tasks, "proposed")
	f.measure = listTasks(ctx, tasks, StatusMeasure)
	f.implement = listTasks(ctx, tasks, StatusImplement)
	if entries, err := os.ReadDir(filepath.Join(repo, filepath.FromSlash(failedDir))); err == nil {
		for _, e := range entries {
			f.wrecks = append(f.wrecks, e.Name())
		}
	}
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
			f.moved = append(f.moved, movedDecision{task: t, closed: closed, commits: n, files: links})
		}
	}
	if slug := repoSlug(ctx, repo); slug != "" {
		f.hasForge = true
		f.prs = openAgentPRs(ctx, repo, slug)
		f.merged, f.intact = mergedWorkBranches(ctx, repo, slug)
	}
	return f
}

// render writes plain text for a phone screen: what waits on a person first,
// across repositories, then what is queued, what is open, what moved, and the
// agent's week. No markdown syntax, since a notification shows it literally;
// titles before ids, since a person recognizes a title; empty sections and
// silent repositories are left out.
func render(repos []repoFacts, opt RecapOptions) string {
	var b strings.Builder
	host, _ := os.Hostname()
	fmt.Fprintf(&b, "jalon, week to %s (%s)\n", opt.Now.UTC().Format("2006-01-02"), host)

	var waiting []string
	for _, r := range repos {
		for _, t := range r.proposed {
			waiting = append(waiting, fmt.Sprintf("%s: agreed, not queued: %s", r.name, label(t)))
		}
		for _, pr := range r.prs {
			waiting = append(waiting, fmt.Sprintf("%s: pull request since %s: %s", r.name, pr.CreatedAt[:min(10, len(pr.CreatedAt))], pr.URL))
		}
		for _, w := range r.wrecks {
			waiting = append(waiting, fmt.Sprintf("%s: failed job to read: %s/%s", r.name, failedDir, w))
		}
		for _, t := range r.stale {
			waiting = append(waiting, fmt.Sprintf("%s: doing for %d days: %s", r.name, int(opt.Now.Sub(idDate(t.id)).Hours()/24), label(t)))
		}
	}
	b.WriteString("\nWAITING ON YOU\n")
	if len(waiting) == 0 {
		b.WriteString("- nothing\n")
	}
	for _, w := range waiting {
		fmt.Fprintf(&b, "- %s\n", w)
	}

	var queued []string
	for _, r := range repos {
		for _, t := range r.measure {
			queued = append(queued, fmt.Sprintf("%s: to measure: %s", r.name, label(t)))
		}
		for _, t := range r.implement {
			queued = append(queued, fmt.Sprintf("%s: to build: %s", r.name, label(t)))
		}
	}
	if len(queued) > 0 {
		b.WriteString("\nQUEUED FOR THE AGENT\n")
		for _, q := range queued {
			fmt.Fprintf(&b, "- %s\n", q)
		}
	}

	b.WriteString("\nOPEN\n")
	any := false
	for _, r := range repos {
		if r.noTasks {
			fmt.Fprintf(&b, "- %s: no .tasks directory\n", r.name)
			any = true
			continue
		}
		if len(r.doing)+len(r.todo) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "- %s: %d doing, %d todo\n", r.name, len(r.doing), len(r.todo))
		for _, t := range r.doing {
			fmt.Fprintf(&b, "    doing: %s\n", label(t))
		}
	}
	if !any {
		b.WriteString("- nothing open anywhere\n")
	}

	moved := false
	for _, r := range repos {
		if len(r.moved) > 0 {
			moved = true
		}
	}
	if moved {
		b.WriteString("\nGROUND MOVED UNDER CLOSED DECISIONS\n")
		b.WriteString("(a linked file changed since the task closed; worth a look, not an alarm)\n")
		for _, r := range repos {
			if len(r.moved) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- %s: %d\n", r.name, len(r.moved))
			for i, m := range r.moved {
				if i == 3 {
					fmt.Fprintf(&b, "    and %d more\n", len(r.moved)-3)
					break
				}
				fmt.Fprintf(&b, "    %s: %d commit(s) since %s on %s\n", label(m.task), m.commits, m.closed, strings.Join(m.files, ", "))
			}
		}
	}

	fmt.Fprintf(&b, "\nAGENT, LAST %d DAYS\n", opt.Days)
	recapMachine(&b, opt)
	var merged []string
	for _, r := range repos {
		if r.hasForge && r.merged > 0 {
			merged = append(merged, fmt.Sprintf("%s %d/%d", r.name, r.intact, r.merged))
		}
	}
	if len(merged) > 0 {
		fmt.Fprintf(&b, "- work branches merged untouched by a person: %s\n", strings.Join(merged, ", "))
	} else {
		b.WriteString("- work branches merged: none yet\n")
	}
	return b.String()
}

// label is how a task is named to a person: its title, then the id in
// parentheses because that is what a status edit or a digest needs.
func label(t listedTask) string {
	if t.title == "" || t.title == t.id {
		return t.id
	}
	return t.title + " (" + t.id + ")"
}

// recapMachine is the second number, machine wide: the metrics line does not
// say which repository a job ran in, so this is the bill for the machine.
func recapMachine(b *strings.Builder, opt RecapOptions) {
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
	fmt.Fprintf(b, "- %d job(s): %d review, %d work; %d published, %d failed\n", jobs, reviews, works, jobs-failed, failed)
	if noCost > 0 {
		fmt.Fprintf(b, "- cost: %.2f USD reported, %d job(s) reported none\n", cost, noCost)
	} else {
		fmt.Fprintf(b, "- cost: %.2f USD\n", cost)
	}
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
