package main

import (
	"strings"
	"testing"
)

// The wrappers hold no logic, so what is worth testing here is that the verbs
// are reachable through run(), that their arguments are validated before
// anything is spent, and that the usage text mentions them.

func TestAgentVerbsAreInTheUsage(t *testing.T) {
	stdout, _ := runOK(t, "help")
	for _, want := range []string{"doctor", "review <id>", "work <id>", "docs/agent.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the usage does not mention %q:\n%s", want, stdout)
		}
	}
}

func TestReviewValidatesItsArgument(t *testing.T) {
	_, tasksDir := newRepo(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"review", "-dir", tasksDir}, "usage: jalon review <task id>"},
		{[]string{"review", "-dir", tasksDir, "no-such-task"}, "no task matching"},
		{[]string{"review", "-dir", tasksDir, "1", "2"}, "usage: jalon review <task id>"},
		{[]string{"review", "-dir", tasksDir, "-next", "x"}, "takes no argument"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			err := runErr(t, c.args...)
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// An unusable configuration is reported by doctor rather than by a stack trace,
// and the verb exits non zero so a systemd unit sees the failure.
func TestDoctorFailsWithoutConfig(t *testing.T) {
	noGH(t)
	_, tasksDir := newRepo(t)

	err := runErr(t, "doctor", "-dir", tasksDir)
	if err == nil || !strings.Contains(err.Error(), "checks failed") {
		t.Fatalf("err = %v, want a doctor failure", err)
	}
}
