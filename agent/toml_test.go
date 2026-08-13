package agent

import (
	"regexp"
	"strings"
	"testing"
)

// Every refusal must name the file, the line, and what to write instead. These
// messages are the whole value of owning a parser rather than depending on one:
// a configuration error is read by a person on a server at the moment something
// is broken.
func TestParseTOMLRefusals(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line string // the ":N:" the message must carry
		want string
	}{
		{"boolean", "[a]\nk = true\n", ":2:", "no booleans"},
		{"single line array", "[a]\nk = [\"x\"]\n", ":2:", "[ alone on its line"},
		{"missing trailing comma", "[a]\nk = [\n  \"x\"\n]\n", ":3:", "no trailing comma"},
		{"unterminated string", "[a]\nk = \"x\n", ":2:", "never closed"},
		{"unclosed array", "[a]\nk = [\n  \"x\",\n", ":2:", "never closed"},
		{"dotted key", "[a]\nb.c = 1\n", ":2:", "dotted key"},
		{"array of tables", "[[a]]\nk = 1\n", ":1:", "array of tables"},
		{"key before section", "k = 1\n", ":1:", "before any [section]"},
		{"duplicate key", "[a]\nk = 1\nk = 2\n", ":3:", "already set on line 2"},
		{"bad escape", "[a]\nk = \"x\\qy\"\n", ":2:", "not one of the four escapes"},
		{"quote in literal", "[a]\nk = 'it''s'\n", ":2:", "cannot contain a '"},
		{"underscore number", "[a]\nk = 1_000\n", ":2:", "not a number"},
		{"hex number", "[a]\nk = 0x3e8\n", ":2:", "not a number"},
		{"empty value", "[a]\nk =\n", ":2:", "value is missing"},
		{"not a key value line", "[a]\nnonsense\n", ":2:", "not a \"key = value\" line"},
		{"unclosed section", "[a\nk = 1\n", ":1:", "no closing ]"},
		{"uppercase key", "[a]\nK = 1\n", ":2:", "not allowed"},
	}
	// Errors carry a "file:line: " prefix and never start a sentence after it,
	// like every other error in this repository. A message may open with a
	// quoted key or a backslash, so the rule is the absence of a capital rather
	// than the presence of a letter.
	style := regexp.MustCompile(`^[^:]+:\d+: [^A-Z]`)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseTOML("agent.toml", []byte(c.src))
			if err == nil {
				t.Fatalf("%s was accepted, want a refusal", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message = %q, want it to contain %q", err, c.want)
			}
			if !strings.Contains(err.Error(), c.line) {
				t.Errorf("message = %q, want it to name line %s", err, c.line)
			}
			if !style.MatchString(err.Error()) {
				t.Errorf("message = %q, want the \"agent.toml:N: lowercase\" shape", err)
			}
		})
	}
}

func TestParseTOMLAccepts(t *testing.T) {
	src := `# a full line comment
[agent]
model_review = "claude-opus-5"          # a trailing comment
max_budget_usd_per_job = 3.0
daily_job_cap = 10
negative = -1

[probes]
allowed = [
  # a comment inside the array

  "curl -s 'http://localhost:8080/h?a=1#frag'",
  'literal \n stays backslash n',
  "escaped \" quote\tand tab",
]
`
	doc, err := parseTOML("agent.toml", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.vals["agent.model_review"].str; got != "claude-opus-5" {
		t.Errorf("model_review = %q", got)
	}
	if got := doc.vals["agent.max_budget_usd_per_job"]; got.kind != tomlFloat || got.f != 3.0 {
		t.Errorf("max_budget_usd_per_job = %+v, want the float 3", got)
	}
	if got := doc.vals["agent.daily_job_cap"]; got.kind != tomlInt || got.i != 10 {
		t.Errorf("daily_job_cap = %+v, want the int 10", got)
	}
	if got := doc.vals["agent.negative"].i; got != -1 {
		t.Errorf("negative = %d, want -1", got)
	}

	arr := doc.vals["probes.allowed"]
	want := []string{
		// The # inside the string is data, not a comment.
		"curl -s 'http://localhost:8080/h?a=1#frag'",
		`literal \n stays backslash n`,
		"escaped \" quote\tand tab",
	}
	if len(arr.arr) != len(want) {
		t.Fatalf("allowed = %q, want %d elements", arr.arr, len(want))
	}
	for i := range want {
		if arr.arr[i] != want[i] {
			t.Errorf("allowed[%d] = %q, want %q", i, arr.arr[i], want[i])
		}
	}
	// The line of each element is what lets a validator point at the bad one.
	if arr.arrLines[0] != 12 {
		t.Errorf("allowed[0] is reported on line %d, want 12", arr.arrLines[0])
	}
	// File order is preserved so errors are reported in reading order.
	if doc.keys[0] != "agent.model_review" || doc.keys[len(doc.keys)-1] != "probes.allowed" {
		t.Errorf("keys = %v, want file order", doc.keys)
	}
}

// A # inside a quoted value must survive, and a # after it must not.
func TestParseTOMLCommentsAndStrings(t *testing.T) {
	doc, err := parseTOML("agent.toml", []byte("[a]\nk = \"x #1\" # dropped\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.vals["a.k"].str; got != "x #1" {
		t.Errorf("k = %q, want %q", got, "x #1")
	}
}
