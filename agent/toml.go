// Package agent is jalon's orchestration layer: it is the only part of the tool
// that calls a model, and it is a separate package so that the boundary is
// enforced by the compiler rather than by discipline.
//
// Nothing in the core (main.go, task.go, digest.go, render.go, github.go,
// metrics.go) may import this package. `rm -rf agent agent_cmd.go`, drop the
// switch cases, and the task manager is intact. TestCoreDoesNotImportAgent in
// the root package is what keeps that true.
package agent

import (
	"fmt"
	"strconv"
	"strings"
)

// A deliberately small TOML subset: enough for .jalon/agent.toml and nothing
// more. Every construct outside the grammar below is an error naming the line
// and the fix, because a configuration file that silently means something other
// than what it looks like is worse than one that refuses to load.
//
//	# comment                 on its own line, or after a value
//	[section]                 bare keys only: no dotted keys, no [[table]]
//	key = "basic string"      \" \\ \n \t are the only escapes
//	key = 'literal string'    no escapes at all, which is what shell probes want
//	key = 42                  decimal integer, optional leading -
//	key = 1.5                 decimal float
//	key = [                   array of strings: [ alone on its line, one
//	  "item",                 "item", per line with a trailing comma on every
//	]                         element including the last, then ] alone
//
// Writing a full TOML parser would be several hundred lines to own for a file
// that holds nine keys. The subset is the measured need.

type tomlKind string

const (
	tomlString tomlKind = "a quoted string"
	tomlInt    tomlKind = "a whole number"
	tomlFloat  tomlKind = "a number"
	tomlArray  tomlKind = "an array of quoted strings"
)

type tomlValue struct {
	line     int
	kind     tomlKind
	str      string
	i        int64
	f        float64
	arr      []string
	arrLines []int // parallel to arr, so a validator can name the bad element
}

// tomlDoc is a flat map of "section.key" to value, plus the file order so that
// errors can be reported in the order a reader scans the file.
type tomlDoc struct {
	keys []string
	vals map[string]tomlValue
}

type tomlParser struct {
	path  string
	lines []string
	i     int // index of the next line to read
}

func parseTOML(display string, src []byte) (*tomlDoc, error) {
	p := &tomlParser{
		path:  display,
		lines: strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n"),
	}
	doc := &tomlDoc{vals: make(map[string]tomlValue)}
	section := ""
	for p.i < len(p.lines) {
		n := p.i + 1
		text, err := p.strip(p.lines[p.i], n)
		if err != nil {
			return nil, err
		}
		p.i++
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") {
			name, err := p.table(text, n)
			if err != nil {
				return nil, err
			}
			section = name
			continue
		}
		key, rest, ok := strings.Cut(text, "=")
		if !ok {
			return nil, p.errf(n, "%q is not a \"key = value\" line; comment it out with # or delete it", text)
		}
		key = strings.TrimSpace(key)
		if err := p.bareKey(key, n); err != nil {
			return nil, err
		}
		if section == "" {
			return nil, p.errf(n, "key %q sits before any [section]; every key in this file belongs to one, so put a [section] header above it", key)
		}
		v, err := p.value(strings.TrimSpace(rest), n)
		if err != nil {
			return nil, err
		}
		full := section + "." + key
		if prev, dup := doc.vals[full]; dup {
			return nil, p.errf(n, "%s is already set on line %d; delete one of the two, jalon never guesses which one you meant", full, prev.line)
		}
		doc.keys = append(doc.keys, full)
		doc.vals[full] = v
	}
	return doc, nil
}

// strip removes a trailing # comment and the surrounding blanks. A # inside a
// string is data, so the scan tracks which quote it is inside: that is the whole
// reason this is not strings.Cut(line, "#").
func (p *tomlParser) strip(line string, n int) (string, error) {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == '"' && c == '\\':
			i++ // the escaped byte is never a delimiter
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return strings.TrimSpace(line[:i]), nil
		}
	}
	if quote != 0 {
		return "", p.errf(n, "the %c quoted string is never closed; close it on the same line, this file has no multi line strings", quote)
	}
	return strings.TrimSpace(line), nil
}

func (p *tomlParser) table(text string, n int) (string, error) {
	if strings.HasPrefix(text, "[[") {
		return "", p.errf(n, "[[%s]] is an array of tables, which this file does not support; use one [section] and an array of strings", strings.Trim(text, "[]"))
	}
	if !strings.HasSuffix(text, "]") {
		return "", p.errf(n, "the [section] header %q has no closing ]", text)
	}
	name := strings.TrimSpace(text[1 : len(text)-1])
	if err := p.bareKey(name, n); err != nil {
		return "", err
	}
	return name, nil
}

// bareKey refuses anything that is not [a-z0-9_]+. Dotted keys are refused here
// too: allowing a.b = 1 would make a key's section depend on where you look.
func (p *tomlParser) bareKey(key string, n int) error {
	if key == "" {
		return p.errf(n, "the name is empty; write a name made of lowercase letters, digits and _")
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' {
			continue
		}
		if c == '.' {
			return p.errf(n, "%q is a dotted key, which this file does not support; put the key under a [%s] header instead", key, strings.SplitN(key, ".", 2)[0])
		}
		return p.errf(n, "%q is not a usable name: %q is not allowed; use lowercase letters, digits and _", key, string(c))
	}
	return nil
}

func (p *tomlParser) value(text string, n int) (tomlValue, error) {
	switch {
	case text == "":
		return tomlValue{}, p.errf(n, "the value is missing; write a value after the = or delete the line")
	case text == "[":
		return p.array(n)
	case text[0] == '[':
		return tomlValue{}, p.errf(n, "an array must open with [ alone on its line, then one \"item\", per line, then ] alone; rewrite %s that way", text)
	case text[0] == '"' || text[0] == '\'':
		s, err := p.str(text, n)
		if err != nil {
			return tomlValue{}, err
		}
		return tomlValue{line: n, kind: tomlString, str: s}, nil
	case text == "true" || text == "false":
		return tomlValue{}, p.errf(n, "this file has no booleans; use a number or a quoted string, whichever the key documents")
	default:
		return p.number(text, n)
	}
}

// array reads a multi line array of strings. A trailing comma is required on
// every element, including the last: it costs one byte, it removes all lookahead
// from this function, and it makes adding a probe a one line diff.
func (p *tomlParser) array(open int) (tomlValue, error) {
	v := tomlValue{line: open, kind: tomlArray}
	for p.i < len(p.lines) {
		n := p.i + 1
		text, err := p.strip(p.lines[p.i], n)
		if err != nil {
			return v, err
		}
		p.i++
		switch {
		case text == "":
			continue
		case text == "]":
			return v, nil
		case !strings.HasSuffix(text, ","):
			return v, p.errf(n, "the array element %s has no trailing comma; every element in this file ends with one, including the last", text)
		}
		s, err := p.str(strings.TrimSpace(strings.TrimSuffix(text, ",")), n)
		if err != nil {
			return v, err
		}
		v.arr = append(v.arr, s)
		v.arrLines = append(v.arrLines, n)
	}
	return v, p.errf(open, "this array is never closed; add a line holding only ]")
}

func (p *tomlParser) str(text string, n int) (string, error) {
	if len(text) < 2 || text[0] != '"' && text[0] != '\'' || text[len(text)-1] != text[0] {
		return "", p.errf(n, "%s is not a quoted string; write it as %q", text, strings.Trim(text, `"'`))
	}
	body := text[1 : len(text)-1]
	if text[0] == '\'' {
		// A literal string has no escapes, which is the entire point of it.
		if strings.Contains(body, "'") {
			return "", p.errf(n, "a 'literal string' cannot contain a '; use a \"basic string\" and write \\\" for a quote")
		}
		return body, nil
	}
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			b.WriteByte(body[i])
			continue
		}
		i++
		if i == len(body) {
			return "", p.errf(n, "the string ends on a backslash; write \\\\ for a literal backslash, or use a 'literal string'")
		}
		switch body[i] {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			return "", p.errf(n, `\%s is not one of the four escapes this file supports (\" \\ \n \t); use a 'literal string' to keep the backslash verbatim`, string(body[i]))
		}
	}
	return b.String(), nil
}

func (p *tomlParser) number(text string, n int) (tomlValue, error) {
	// strconv accepts Go literal syntax, so it would read 1_000, 0x3e8 and Inf
	// as numbers. This file takes plain decimal only: a value that means one
	// thing here and another in TOML proper is exactly the trap this parser
	// exists to avoid.
	if strings.ContainsAny(text, "_xXoObBpP") || !strings.ContainsAny(text, "0123456789") {
		return tomlValue{}, p.errf(n, "%s is not a number this file accepts; write plain decimal like 1000 or 1.5, or quote it as %q if it is meant to be text", text, text)
	}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return tomlValue{line: n, kind: tomlInt, i: i}, nil
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return tomlValue{line: n, kind: tomlFloat, f: f}, nil
	}
	return tomlValue{}, p.errf(n, "%s is not a number; quote it as %q if it is meant to be text", text, text)
}

func (p *tomlParser) errf(line int, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", p.path, line, fmt.Sprintf(format, args...))
}
