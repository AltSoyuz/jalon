package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMetrics(t *testing.T, path string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not valid json: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestMetricsAreOptIn(t *testing.T) {
	_, tasksDir := newRepo(t)
	path := filepath.Join(t.TempDir(), "metrics.jsonl")
	t.Setenv(metricsEnv, "")

	runOK(t, "new", "-dir", tasksDir, "silence by default")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("nothing must be written when %s is unset", metricsEnv)
	}
}

func TestMetricsOneLinePerInvocation(t *testing.T) {
	root, tasksDir := newRepo(t)
	path := filepath.Join(t.TempDir(), "metrics.jsonl")
	t.Setenv(metricsEnv, path)
	t.Setenv("JALON_SIG", "gt")

	stdout, _ := runOK(t, "new", "-dir", tasksDir, "measure the digest")
	id := strings.TrimSuffix(filepath.Base(strings.TrimSpace(stdout)), ".md")
	gitCommit(t, root, "["+id+"] first pass")
	runOK(t, "append", "-dir", tasksDir, id, "one entry")
	runOK(t, "digest", "-dir", tasksDir, "-offline", id)
	runOK(t, "render", "-dir", tasksDir)

	events := readMetrics(t, path)
	if len(events) != 4 {
		t.Fatalf("got %d lines, want one per invocation:\n%v", len(events), events)
	}

	verbs := make([]string, len(events))
	for i, e := range events {
		verbs[i] = e["verb"].(string)
		if e["time"] == "" || e["version"] == nil {
			t.Errorf("%v is missing time or version", e)
		}
		if e["id"] != nil && e["id"] != id && e["verb"] != "render" {
			t.Errorf("%v carries the wrong id", e)
		}
	}
	if got := strings.Join(verbs, ","); got != "new,append,digest,render" {
		t.Errorf("verbs = %s", got)
	}

	// The digest line is the one that has to carry the cost of orientation.
	d := events[2]
	for _, k := range []string{"bytes", "tokens", "commits", "git"} {
		if d[k] == nil {
			t.Errorf("the digest line is missing %q: %v", k, d)
		}
	}
	if d["bytes"].(float64) <= 0 || d["tokens"].(float64) <= 0 {
		t.Errorf("digest recorded no size: %v", d)
	}
	if d["git"] != "ok" || d["gh"] != "skipped" {
		t.Errorf("digest -offline must record git ok and gh skipped: %v", d)
	}
	if events[3]["tasks"].(float64) != 1 {
		t.Errorf("render must record the task count: %v", events[3])
	}
}

func TestMetricsRecordFailures(t *testing.T) {
	_, tasksDir := newRepo(t)
	path := filepath.Join(t.TempDir(), "metrics.jsonl")
	t.Setenv(metricsEnv, path)

	runErr(t, "digest", "-dir", tasksDir, "999999")

	events := readMetrics(t, path)
	if len(events) != 1 {
		t.Fatalf("a failed run must still be recorded, got %v", events)
	}
	if e, ok := events[0]["err"].(string); !ok || !strings.Contains(e, "no task matching") {
		t.Errorf("the error must be recorded: %v", events[0])
	}
}

func TestMetricsFailureNeverBreaksTheCommand(t *testing.T) {
	// An unwritable metrics path must warn and let the command succeed: the
	// measurement observes the tool, it does not get to break it.
	_, tasksDir := newRepo(t)
	t.Setenv(metricsEnv, filepath.Join(t.TempDir(), "no-such-dir", "metrics.jsonl"))

	var out, errb strings.Builder
	if err := run([]string{"new", "-dir", tasksDir, "still works"}, &out, &errb); err != nil {
		t.Fatalf("the command must succeed: %v", err)
	}
	if !strings.Contains(errb.String(), "cannot write "+metricsEnv) {
		t.Errorf("the metrics failure must be reported on stderr: %q", errb.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("the command must still print its result")
	}
}
