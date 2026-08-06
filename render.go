package main

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The markdown subset rendered here is a contract, documented in the README:
// ATX headings 1 to 3, fenced code blocks, single level bullet lists,
// paragraphs, inline code and links. Anything else is emitted as escaped
// literal text. The failure mode is "it looks like raw markdown", never broken
// HTML and never an injection.
func mdToHTML(src string) string {
	var b strings.Builder
	var para []string
	inList, inPara, inCode := false, false, false

	flushPara := func() {
		if inPara {
			b.WriteString("<p>" + strings.Join(para, " ") + "</p>\n")
			para = para[:0]
			inPara = false
		}
	}
	closeList := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
	}

	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if inCode {
			if strings.HasPrefix(trimmed, "```") {
				b.WriteString("</code></pre>\n")
				inCode = false
				continue
			}
			b.WriteString(html.EscapeString(line))
			b.WriteString("\n")
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "```"):
			flushPara()
			closeList()
			b.WriteString("<pre><code>")
			inCode = true
		case trimmed == "":
			flushPara()
			closeList()
		case strings.HasPrefix(line, "### "):
			flushPara()
			closeList()
			b.WriteString("<h3>" + inlineHTML(line[4:]) + "</h3>\n")
		case strings.HasPrefix(line, "## "):
			flushPara()
			closeList()
			b.WriteString("<h2>" + inlineHTML(line[3:]) + "</h2>\n")
		case strings.HasPrefix(line, "# "):
			flushPara()
			closeList()
			b.WriteString("<h1>" + inlineHTML(line[2:]) + "</h1>\n")
		case isEntry(line):
			flushPara()
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			b.WriteString("<li>" + inlineHTML(line[2:]) + "</li>\n")
		default:
			closeList()
			inPara = true
			para = append(para, inlineHTML(line))
		}
	}
	if inCode {
		b.WriteString("</code></pre>\n")
	}
	flushPara()
	closeList()
	return b.String()
}

var linkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^()\s]+)\)`)

// inlineHTML escapes first, then reads the escaped text: an href can no longer
// carry a quote, so an attribute cannot be broken out of.
func inlineHTML(s string) string {
	s = html.EscapeString(s)
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '`')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '`')
		if j < 0 {
			break
		}
		b.WriteString(inlineLinks(s[:i]))
		b.WriteString("<code>")
		b.WriteString(s[i+1 : i+1+j])
		b.WriteString("</code>")
		s = s[i+j+2:]
	}
	b.WriteString(inlineLinks(s))
	return b.String()
}

func inlineLinks(s string) string {
	return linkRe.ReplaceAllStringFunc(s, func(m string) string {
		p := linkRe.FindStringSubmatch(m)
		if !safeURL(p[2]) {
			return m
		}
		return `<a href="` + p[2] + `">` + p[1] + `</a>`
	})
}

// safeURL allows relative targets and the three schemes a task page can need.
// Everything else is rendered as literal text rather than linked.
func safeURL(u string) bool {
	i := strings.IndexByte(u, ':')
	if i < 0 {
		return true
	}
	if j := strings.IndexAny(u, "/?#"); j >= 0 && j < i {
		return true
	}
	switch strings.ToLower(u[:i]) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

type commit struct {
	Hash    string
	Date    string
	Subject string
}

type pageData struct {
	ID      string
	Title   string
	Status  string
	Created string
	Links   []string
	Body    template.HTML
	Commits []commit
	CSS     template.CSS
}

type indexGroup struct {
	Status string
	Rows   []indexRow
}

type indexRow struct {
	ID      string
	Title   string
	Created string
	Commits int
}

type indexData struct {
	Groups []indexGroup
	Total  int
	CSS    template.CSS
}

// statusOrder is the conventional order; any other status is appended,
// sorted, because the format is not policed.
var statusOrder = []string{statusTodo, "doing", statusDone}

// renderSite regenerates the whole view. A full rewrite every time is the
// point: at a few hundred files of a few KB it costs milliseconds, and an
// incremental build would be a cache to invalidate for no measured gain.
func renderSite(warn io.Writer, tasksDir, outDir string) (int, error) {
	tasks, err := LoadTasks(tasksDir)
	if err != nil {
		return 0, err
	}
	root := repoRoot(tasksDir)
	byID := commitsByTask(warn, root, tasks)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}

	groups := make(map[string][]indexRow)
	for _, t := range tasks {
		commits := byID[t.ID]
		page := pageData{
			ID:      t.ID,
			Title:   t.Title(),
			Status:  t.Status(),
			Created: t.FM.Get("created"),
			Links:   t.FM.Links(),
			Body:    template.HTML(mdToHTML(t.Body)),
			Commits: commits,
			CSS:     template.CSS(siteCSS),
		}
		f, err := os.Create(filepath.Join(outDir, t.ID+".html"))
		if err != nil {
			return 0, err
		}
		err = pageTmpl.Execute(f, page)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return 0, err
		}
		groups[t.Status()] = append(groups[t.Status()], indexRow{
			ID:      t.ID,
			Title:   page.Title,
			Created: page.Created,
			Commits: len(commits),
		})
	}

	data := indexData{Total: len(tasks), CSS: template.CSS(siteCSS)}
	for _, s := range orderedStatuses(groups) {
		rows := groups[s]
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
		data.Groups = append(data.Groups, indexGroup{Status: s, Rows: rows})
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return 0, err
	}
	err = indexTmpl.Execute(f, data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return len(tasks), err
}

func orderedStatuses(groups map[string][]indexRow) []string {
	var out []string
	seen := make(map[string]bool)
	for _, s := range statusOrder {
		if len(groups[s]) > 0 {
			out = append(out, s)
			seen[s] = true
		}
	}
	var rest []string
	for s := range groups {
		if !seen[s] {
			rest = append(rest, s)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

var commitTokenRe = regexp.MustCompile(`\[(\d{6}[a-z0-9-]*)\]`)

// commitsByTask reads the whole history once and indexes it by id token: one
// git call for the entire site, not one per task.
func commitsByTask(warn io.Writer, root string, tasks []*Task) map[string][]commit {
	byID := make(map[string][]commit, len(tasks))
	if !isGitRepo(root) {
		fmt.Fprintf(warn, "jalon: %s is not a git repository: pages will have no commits\n", root)
		return byID
	}
	out, err := gitOutput(root, "log", "--format=%h%x1f%ad%x1f%s", "--date=short")
	if err != nil {
		fmt.Fprintf(warn, "jalon: git log failed: %v\n", err)
		return byID
	}

	byToken := make(map[string][]commit)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\x1f")
		if len(parts) != 3 {
			continue
		}
		c := commit{Hash: parts[0], Date: parts[1], Subject: parts[2]}
		for _, m := range commitTokenRe.FindAllStringSubmatch(c.Subject, -1) {
			byToken[m[1]] = append(byToken[m[1]], c)
		}
	}

	sharedPrefix := make(map[string]int)
	for _, t := range tasks {
		sharedPrefix[idPrefix(t.ID)]++
	}
	warned := make(map[string]bool)
	for _, t := range tasks {
		prefix := idPrefix(t.ID)
		byID[t.ID] = append(byID[t.ID], byToken[t.ID]...)
		if prefix == t.ID {
			continue
		}
		if len(byToken[prefix]) == 0 {
			continue
		}
		if sharedPrefix[prefix] > 1 && !warned[prefix] {
			warned[prefix] = true
			fmt.Fprintf(warn, "jalon: %d tasks share the prefix %s: commits tagged [%s] are listed on all of them, tag them with the full id\n",
				sharedPrefix[prefix], prefix, prefix)
		}
		byID[t.ID] = append(byID[t.ID], byToken[prefix]...)
	}
	return byID
}

const siteCSS = `:root { --bg:#fff; --fg:#1a1a1a; --muted:#666; --line:#e2e2e2; --accent:#0645ad; --code:#f5f5f5; }
@media (prefers-color-scheme: dark) {
  :root { --bg:#161616; --fg:#e6e6e6; --muted:#9a9a9a; --line:#333; --accent:#7aa2f7; --code:#202020; }
}
* { box-sizing: border-box; }
body { background: var(--bg); color: var(--fg); font: 16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
       margin: 0 auto; max-width: 46rem; padding: 2rem 1rem 4rem; }
a { color: var(--accent); }
h1 { font-size: 1.6rem; margin: 0 0 .2rem; }
h2 { font-size: 1.15rem; margin: 2rem 0 .5rem; border-bottom: 1px solid var(--line); padding-bottom: .2rem; }
h3 { font-size: 1rem; margin: 1.4rem 0 .4rem; }
code { background: var(--code); border-radius: 3px; font-size: .875em; padding: .1em .3em; }
pre { background: var(--code); border-radius: 4px; overflow-x: auto; padding: .8rem 1rem; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; }
th, td { border-bottom: 1px solid var(--line); padding: .4rem .5rem; text-align: left; vertical-align: top; }
th { color: var(--muted); font-size: .8rem; font-weight: 600; text-transform: uppercase; }
td.num { text-align: right; white-space: nowrap; }
.meta { color: var(--muted); font-size: .85rem; margin: 0 0 1.5rem; }
.status { border: 1px solid var(--line); border-radius: 3px; font-size: .75rem; padding: .1rem .4rem; text-transform: uppercase; }
.done { opacity: .6; }
nav { font-size: .85rem; margin-bottom: 1.5rem; }
footer { border-top: 1px solid var(--line); color: var(--muted); font-size: .8rem; margin-top: 3rem; padding-top: 1rem; }
`

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>tasks</title><style>{{.CSS}}</style></head><body>
<h1>Tasks</h1>
<p class="meta">{{.Total}} task(s). Generated by jalon; the markdown files are the truth.</p>
{{range .Groups}}<h2>{{.Status}}</h2>
<table><thead><tr><th>id</th><th>title</th><th>created</th><th>commits</th></tr></thead><tbody>
{{range .Rows}}<tr><td><a href="{{.ID}}.html">{{.ID}}</a></td><td>{{.Title}}</td><td>{{.Created}}</td><td class="num">{{.Commits}}</td></tr>
{{end}}</tbody></table>
{{end}}<footer>jalon render</footer>
</body></html>
`))

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.ID}}</title><style>{{.CSS}}</style></head><body>
<nav><a href="index.html">&larr; all tasks</a></nav>
<p class="meta"><span class="status">{{.Status}}</span> {{.ID}}{{if .Created}} &middot; created {{.Created}}{{end}}
{{if .Links}}<br>links: {{range $i, $l := .Links}}{{if $i}}, {{end}}<code>{{$l}}</code>{{end}}{{end}}</p>
{{.Body}}
{{if .Commits}}<h2>Related commits</h2>
<table><tbody>
{{range .Commits}}<tr><td><code>{{.Hash}}</code></td><td>{{.Date}}</td><td>{{.Subject}}</td></tr>
{{end}}</tbody></table>
{{end}}<footer>jalon render</footer>
</body></html>
`))
