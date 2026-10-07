package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// A first run that names a profile must take the region from that profile instead of
// asking for one.
//
// It used to ask: the region was resolved during environment setup without the chosen
// profile, so a shared config holding only `[profile beta] region = ...` and no
// [default] section looked like nothing at all, and the run dropped into the
// interactive region selector. Whatever was typed there became the stored default
// region, for a profile that already said which region it meant. Worse, with no
// terminal the prompt simply fails, so a first run in a script could not get past it.
//
// Only a test running the binary can show this, because the fault is in the order
// start-up does things. --local keeps it offline: no credentials are resolved and no
// AWS call is made.
func TestFirstRunTakesTheRegionFromTheChosenProfile(t *testing.T) {
	bin := build(t)

	awsConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(awsConfig, []byte("[profile beta]\nregion = us-east-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-p", "beta", "--no-sync", "--local", "list", "instances")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"AWS_CONFIG_FILE="+awsConfig,
		"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(t.TempDir(), "credentials-that-do-not-exist"),
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_REGION=", "AWS_DEFAULT_REGION=", "AWS_PROFILE=", "AWS_DEFAULT_PROFILE=",
		"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=",
	)
	// /dev/null rather than this test's stdin, so a run that does reach the prompt
	// fails instead of blocking the suite.
	cmd.Stdin = nil

	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("the run did not finish, most likely waiting on the region prompt. Output:\n%s", out)
	}
	if err != nil {
		t.Fatalf("exit status: %s. Output:\n%s", err, out)
	}

	if strings.Contains(string(out), "Please enter one region") {
		t.Errorf("a region was asked for although the profile names one. Output:\n%s", out)
	}
	if !strings.Contains(string(out), "region 'us-east-2'") {
		t.Errorf("the region should have come from the profile, got:\n%s", out)
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

// The completion scripts come from cobra, and the zsh one has to be installable: a
// package manager drops it into a site-functions directory, where zsh only picks up a
// file that starts with #compdef. The script this replaced was meant to be sourced and
// was silently ignored when installed that way.
func TestCompletionScriptsForEveryShell(t *testing.T) {
	bin := build(t)

	for shell, first := range map[string]string{
		"bash":       "# bash completion",
		"zsh":        "#compdef awless-ro",
		"fish":       "# fish completion",
		"powershell": "# powershell completion",
	} {
		t.Run(shell, func(t *testing.T) {
			cmd := exec.Command(bin, "completion", shell)
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("completion %s: %s", shell, err)
			}
			if !strings.HasPrefix(string(out), first) {
				t.Errorf("completion %s should start with %q, got %.60q", shell, first, out)
			}
		})
	}
}

// Completion runs on every Tab press, so it must answer from local state and nothing
// else. On a machine where awless-ro has never run that means offering nothing: no
// first-run setup, no credential prompt, no sync, and nothing written to the home
// directory.
func TestCompletionOnAFreshMachineDoesNotSetAnythingUp(t *testing.T) {
	bin := build(t)
	home := t.TempDir()

	for _, args := range [][]string{
		{"__complete", "show", ""},
		{"__complete", "ssh", ""},
		{"__complete", "tail", "stack-events", ""},
		{"__complete", "whoami", ""},
	} {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "HOME="+home,
			"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_PROFILE=")
		cmd.Stdin = nil
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %s", args, err)
		}
		// cobra ends with the directive line; anything before it is a candidate.
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) != 1 || !strings.HasPrefix(lines[0], ":") {
			t.Errorf("%v offered candidates with nothing synced:\n%s", args, out)
		}
	}

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("completion created %d entries in the home directory, starting with %s", len(entries), entries[0].Name())
	}
}

func TestCompletionOfGlobalFlags(t *testing.T) {
	cmd := exec.Command(build(t), "__complete", "list", "instances", "-r", "eu-west-")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "eu-west-1\n") {
		t.Errorf("-r should complete to regions, got:\n%s", out)
	}
}
