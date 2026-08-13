package main

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The agent layer is a separate package so that the boundary is not a habit but
// something a test can fail on. Go already forbids the other direction, since
// package main cannot be imported at all; this enforces the one Go cannot.
//
// What it protects: `rm -rf agent agent_cmd.go`, drop two cases from the switch
// in main.go, and jalon is still a working task manager with no model, no
// network and no key. The day a core verb reads agent configuration, the design
// is broken, and this is where that shows up.
func TestCoreDoesNotImportAgent(t *testing.T) {
	const agentPkg = `"github.com/AltSoyuz/jalon/agent"`

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	bridges := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, imp := range f.Imports {
			if imp.Path.Value != agentPkg {
				continue
			}
			// Only the agent_* wrappers may reach across. The naming rule makes
			// the bridge one grep to audit.
			if strings.HasPrefix(name, "agent_") {
				bridges++
				continue
			}
			t.Errorf("%s imports the agent package; only the agent_*.go wrappers may. Move the call into agent_cmd.go, or rename this file.",
				fset.Position(imp.Pos()))
		}
	}
	// Without this the test passes vacuously the day someone inlines the agent
	// layer into the core and deletes the wrapper.
	if bridges == 0 {
		t.Error("no agent_*.go file imports the agent package: either the bridge was deleted, or the layer was inlined into the core")
	}
}
