package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Self observability for a process that lives a few milliseconds: one JSON line
// per invocation, appended to a file. A server would expose /metrics; a binary
// this short lived would spend more time serving the endpoint than working.
//
// The raw line is deliberately the only thing recorded. Which averages matter is
// not known yet, and a summary written today would freeze that guess into code.
// docs/measuring.md holds the one liners that read this file; a `stats` verb
// earns itself the day the same one liner gets typed for the third time.
//
// Opt in through $JALON_METRICS. Unset, nothing is written: a tool whose pitch
// is that no data leaves the repository does not get to write telemetry nobody
// asked for.
const metricsEnv = "JALON_METRICS"

type metric struct {
	Time    string `json:"time"`
	Version string `json:"version"`
	Verb    string `json:"verb"`
	ID      string `json:"id,omitempty"`
	MS      int64  `json:"ms"`
	Bytes   int    `json:"bytes,omitempty"`
	Tokens  int    `json:"tokens,omitempty"`
	Files   int    `json:"files,omitempty"`
	Commits int    `json:"commits,omitempty"`
	Tasks   int    `json:"tasks,omitempty"`
	Checks  int    `json:"checks,omitempty"`
	Git     string `json:"git,omitempty"`
	GH      string `json:"gh,omitempty"`
	Err     string `json:"err,omitempty"`

	start time.Time
}

func newMetric(verb string) *metric {
	return &metric{Verb: verb, Version: version, start: time.Now()}
}

// flush appends the line. A metrics failure warns and never fails the command:
// the measurement is there to observe the tool, not to become a way for it to
// break.
func (m *metric) flush(stderr io.Writer, err error) {
	path := os.Getenv(metricsEnv)
	if path == "" || m == nil {
		return
	}
	m.Time = m.start.UTC().Format(time.RFC3339)
	m.MS = time.Since(m.start).Milliseconds()
	if err != nil {
		m.Err = err.Error()
	}
	line, jerr := json.Marshal(m)
	if jerr != nil {
		fmt.Fprintf(stderr, "jalon: cannot encode the metrics line: %v\n", jerr)
		return
	}
	f, ferr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if ferr != nil {
		fmt.Fprintf(stderr, "jalon: cannot write %s=%s: %v\n", metricsEnv, path, ferr)
		return
	}
	// One write of one short line under O_APPEND is atomic on POSIX, so two
	// loops running in parallel cannot interleave their lines.
	if _, werr := f.Write(append(line, '\n')); werr != nil {
		fmt.Fprintf(stderr, "jalon: cannot append to %s: %v\n", path, werr)
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "jalon: cannot close %s: %v\n", path, cerr)
	}
}
