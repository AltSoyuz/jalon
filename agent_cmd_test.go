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
	for _, want := range []string{"doctor", "review <issue>", "docs/agent.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the usage does not mention %q:\n%s", want, stdout)
		}
	}
}

func TestReviewValidatesItsArgument(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"review"}, "usage: jalon review <issue number>"},
		{[]string{"review", "abc"}, "is not an issue number"},
		{[]string{"review", "0"}, "is not an issue number"},
		{[]string{"review", "12x"}, "is not an issue number"},
		{[]string{"review", "#12"}, "is not an issue number"},
		{[]string{"review", "1", "2"}, "usage: jalon review <issue number>"},
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
