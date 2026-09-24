package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The exit status is the one thing about this binary that only a test running it can
// check, and it was wrong: Execute's error was discarded, so every failure printed
// itself and exited 0. A script reading `awless-ro search images canonical
// --latest-id` could not tell an id from a failed lookup.
//
// Both cases below fail before any hook runs, so this needs no credentials, no AWS
// call and no awless home directory.
func TestExitStatusReportsFailure(t *testing.T) {
	bin := build(t)

	cases := []struct {
		name string
		args []string
		want int
	}{
		{"unknown command", []string{"definitely-not-a-command"}, 1},
		{"unknown flag", []string{"--definitely-not-a-flag"}, 1},
		{"help", []string{"--help"}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			// A home of its own, so a stray first-install cannot touch the
			// developer's ~/.awless-ro, and no AWS variables inherited.
			cmd.Env = append(os.Environ(),
				"HOME="+t.TempDir(),
				"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_PROFILE=",
			)
			out, err := cmd.CombinedOutput()

			got := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				got = exit.ExitCode()
			} else if err != nil {
				t.Fatalf("could not run the binary: %s", err)
			}

			if got != tc.want {
				t.Errorf("exit status %d, want %d. Output:\n%s", got, tc.want, out)
			}
		})
	}
}

func build(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "awless-ro")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s\n%s", err, out)
	}
	return bin
}

// An unknown command has to say so rather than exit quietly, because the usual cause
// is a typo close to a real command.
func TestUnknownCommandSaysWhatIsWrong(t *testing.T) {
	cmd := exec.Command(build(t), "lst")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, _ := cmd.CombinedOutput()

	if !strings.Contains(string(out), "lst") {
		t.Errorf("the output should name what was not recognised, got:\n%s", out)
	}
}
