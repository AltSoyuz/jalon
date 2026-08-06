package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Section names, in the order an agent reads them.
const (
	sectionContext   = "Context"
	sectionDecisions = "Decisions"
	sectionLog       = "Log"
)

const (
	statusTodo = "todo"
	statusDone = "done"
)

const dateLayout = "2006-01-02"

// idLayout is YYMMDD: MMDD alone repeats every year and would make
// `git log --grep '[0806]'` return several years of unrelated commits.
const idLayout = "060102"

// Task is one markdown file: an ordered front matter block plus the raw body.
// The body is never reformatted, so hand edits survive a rewrite.
type Task struct {
	ID   string
	Path string
	FM   FrontMatter
	Body string
}

// FrontMatter keeps insertion order so that unknown keys survive a rewrite.
type FrontMatter struct {
	keys []string
	vals map[string]string
}

func (f *FrontMatter) Get(key string) string { return f.vals[key] }

func (f *FrontMatter) Set(key, value string) {
	if f.vals == nil {
		f.vals = make(map[string]string)
	}
	if _, ok := f.vals[key]; !ok {
		f.keys = append(f.keys, key)
	}
	f.vals[key] = value
}

func (f *FrontMatter) Keys() []string { return f.keys }

// Links returns the `links` value, which is a YAML flow sequence so that a
// forge still renders the front matter block as a table.
func (f *FrontMatter) Links() []string {
	v := strings.TrimSpace(f.Get("links"))
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseTask reads a task file. The id is the file name without its extension.
func ParseTask(path string) (*Task, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t := &Task{
		ID:   strings.TrimSuffix(filepath.Base(path), ".md"),
		Path: path,
	}
	if t.FM, t.Body, err = parseFrontMatter(string(b)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

func parseFrontMatter(src string) (FrontMatter, string, error) {
	var fm FrontMatter
	src = strings.ReplaceAll(src, "\r\n", "\n")
	if !strings.HasPrefix(src, "---\n") {
		return fm, "", errors.New("missing front matter (the file must start with ---)")
	}
	rest := src[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
		} else {
			return fm, "", errors.New("unterminated front matter (no closing ---)")
		}
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fm, "", fmt.Errorf("front matter line %q is not key: value", line)
		}
		fm.Set(strings.TrimSpace(key), strings.TrimSpace(value))
	}
	body := ""
	if tail := rest[end:]; len(tail) > len("\n---\n") {
		body = tail[len("\n---\n"):]
	}
	return fm, body, nil
}

// Bytes renders the task back to its file form. Front matter values are
// normalized to "key: value"; everything else is byte for byte the body.
func (t *Task) Bytes() []byte {
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range t.FM.Keys() {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(t.FM.Get(k))
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	b.WriteString(t.Body)
	return []byte(b.String())
}

func (t *Task) Save() error {
	return os.WriteFile(t.Path, t.Bytes(), 0o644)
}

func (t *Task) Status() string {
	if s := strings.TrimSpace(t.FM.Get("status")); s != "" {
		return s
	}
	return statusTodo
}

// Title is the first level one heading of the body, or the id.
func (t *Task) Title() string {
	for _, line := range strings.Split(t.Body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return t.ID
}

// Section returns the content of a "## name" section, heading excluded.
func (t *Task) Section(name string) string {
	lines := strings.Split(t.Body, "\n")
	head, end, ok := sectionLines(lines, name)
	if !ok {
		return ""
	}
	return strings.Trim(strings.Join(lines[head+1:end], "\n"), "\n")
}

// SetSection replaces the content of a "## name" section, heading kept. It is
// the counterpart of Section, and the only supported way to rewrite Context.
func (t *Task) SetSection(name, content string) error {
	lines := strings.Split(t.Body, "\n")
	head, end, ok := sectionLines(lines, name)
	if !ok {
		return fmt.Errorf("%s: section %q not found", t.ID, name)
	}
	out := append([]string{}, lines[:head+1]...)
	out = append(out, "")
	out = append(out, strings.Split(strings.Trim(content, "\n"), "\n")...)
	out = append(out, "")
	t.Body = strings.Join(append(out, lines[end:]...), "\n")
	return nil
}

// Entries returns the list item lines of a section, one per entry.
func (t *Task) Entries(name string) []string {
	var out []string
	for _, line := range strings.Split(t.Section(name), "\n") {
		if isEntry(line) {
			out = append(out, line)
		}
	}
	return out
}

func isEntry(line string) bool {
	return strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")
}

// sectionLines locates a "## name" section and returns the heading index and
// the index just past its last line. Headings inside a code fence are ignored,
// so a task holding a markdown snippet does not confuse the scan.
func sectionLines(lines []string, name string) (head, end int, ok bool) {
	fence := false
	head = -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if head < 0 {
			if strings.HasPrefix(line, "## ") && strings.TrimSpace(line[3:]) == name {
				head = i
			}
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			return head, i, true
		}
	}
	if head < 0 {
		return 0, 0, false
	}
	return head, len(lines), true
}

// AppendEntry adds one line at the end of a section. Newlines in text are
// collapsed: one entry is always one line, which is what makes truncation and
// `grep` predictable.
func (t *Task) AppendEntry(section, entry string) error {
	lines := strings.Split(t.Body, "\n")
	head, end, ok := sectionLines(lines, section)
	if !ok {
		return fmt.Errorf("%s: section %q not found", t.ID, section)
	}
	last := head
	for i := head + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	out := append([]string{}, lines[:last+1]...)
	if last == head {
		out = append(out, "")
	}
	out = append(out, entry)
	out = append(out, lines[last+1:]...)
	t.Body = strings.Join(out, "\n")
	return nil
}

// FormatEntry builds "- 2026-08-06 gt: text".
func FormatEntry(now time.Time, sig, text string) string {
	text = strings.Join(strings.Fields(text), " ")
	return fmt.Sprintf("- %s %s: %s", now.Format(dateLayout), sig, text)
}

// TruncateSection keeps the last n entries of a section and replaces the older
// ones with a single line pointing at git, which holds the full history.
func (t *Task) TruncateSection(section string, keep int) (removed int, err error) {
	lines := strings.Split(t.Body, "\n")
	head, end, ok := sectionLines(lines, section)
	if !ok {
		return 0, fmt.Errorf("%s: section %q not found", t.ID, section)
	}
	var idx []int
	for i := head + 1; i < end; i++ {
		if isEntry(lines[i]) {
			idx = append(idx, i)
		}
	}
	if len(idx) <= keep {
		return 0, nil
	}
	// An entry hand wrapped over several lines must go with its own
	// continuation lines, or truncation would leave orphan prose behind.
	drop := make(map[int]bool, len(idx)-keep)
	for _, i := range idx[:len(idx)-keep] {
		drop[i] = true
		for j := i + 1; j < end && !isEntry(lines[j]) && strings.TrimSpace(lines[j]) != "" && !strings.HasPrefix(lines[j], "#"); j++ {
			drop[j] = true
		}
	}
	marker := fmt.Sprintf("- (%d earlier entries truncated; see: git log -p --follow %s)", len(drop), t.Path)
	out := make([]string, 0, len(lines))
	inserted := false
	for i, line := range lines {
		if drop[i] {
			if !inserted {
				out = append(out, marker)
				inserted = true
			}
			continue
		}
		out = append(out, line)
	}
	t.Body = strings.Join(out, "\n")
	return len(drop), nil
}

// EstimateTokens is bytes/4. It is an estimate, not a tokenizer: the point is
// to have a budget signal without depending on one.
func EstimateTokens(n int) int { return n / 4 }

var (
	nonSlug  = regexp.MustCompile(`[^a-z0-9]+`)
	deaccent = strings.NewReplacer(
		"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"î", "i", "ï", "i", "í", "i",
		"ô", "o", "ö", "o", "ó", "o", "õ", "o",
		"ù", "u", "û", "u", "ü", "u", "ú", "u",
		"ç", "c", "ñ", "n", "ÿ", "y", "œ", "oe", "æ", "ae",
	)
)

const slugMaxLen = 40

// Slug builds the readable half of an id.
func Slug(title string) string {
	s := deaccent.Replace(strings.ToLower(strings.TrimSpace(title)))
	s = strings.Trim(nonSlug.ReplaceAllString(s, "-"), "-")
	if len(s) > slugMaxLen {
		s = strings.Trim(s[:slugMaxLen], "-")
	}
	return s
}

// NewID builds YYMMDD-slug.
func NewID(now time.Time, title string) (string, error) {
	slug := Slug(title)
	if slug == "" {
		return "", fmt.Errorf("title %q has no character usable in an id", title)
	}
	return now.Format(idLayout) + "-" + slug, nil
}

const taskTemplate = `---
status: %s
created: %s
links: []
---

# %s

## Context

## Decisions

## Log
`

// CreateTask writes a new task file. It never overwrites an existing one.
func CreateTask(dir, id, title, status string, now time.Time) (*Task, error) {
	path := filepath.Join(dir, id+".md")
	body := fmt.Sprintf(taskTemplate, status, now.Format(dateLayout), title)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%s already exists; pick a different title", path)
		}
		return nil, err
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return ParseTask(path)
}

// LoadTasks reads every task file of a directory, sorted by id.
func LoadTasks(dir string) ([]*Task, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	tasks := make([]*Task, 0, len(paths))
	for _, p := range paths {
		t, err := ParseTask(p)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}
