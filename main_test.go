package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

// A bad --max-width is a mistake on the command line, so it is refused while the
// command line is parsed: before any hook, hence before any setup under the home
// directory, any AWS session or any AWS call.
func TestInvalidMaxWidthIsRefusedBeforeAnythingRuns(t *testing.T) {
	bin := build(t)

	for _, args := range [][]string{
		{"list", "stacks", "--local", "--max-width", "-1"},
		{"list", "stacks", "--local", "--max-width=-1"},
		{"list", "stacks", "--local", "--max-width", "abc"},
		{"list", "stacks", "--local", "--max-width", "1.5"},
		{"show", "x", "--local", "--max-width", "-5"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := t.TempDir()
			cmd := exec.Command(bin, args...)
			cmd.Env = append(os.Environ(), "HOME="+home,
				"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_PROFILE=")
			out, err := cmd.CombinedOutput()

			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("want exit status 1, got %v. Output:\n%s", err, out)
			}
			if !strings.Contains(string(out), "max-width") {
				t.Errorf("the error does not name --max-width:\n%s", out)
			}
			entries, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Errorf("something ran: %d entries in the home directory, starting with %s", len(entries), entries[0].Name())
			}
		})
	}
}

// Output going to a pipe or a file has no width to fit, so the table is not wrapped at
// all: each record on one line, whole, however long. The stacks are synthetic.
func TestTableOutputToAPipeIsNotWrapped(t *testing.T) {
	bin := build(t)
	home := t.TempDir()

	const (
		arn1  = "arn:aws:cloudformation:eu-west-1:123456789012:stack/my-production-application-network-stack-eu-west-1-blue/0f1e2d3c-4b5a-6978-8765-4321fedcba09"
		name1 = "my-production-application-network-stack-eu-west-1-blue"
		arn2  = "arn:aws:cloudformation:eu-west-1:123456789012:stack/my-production-application-compute-stack-eu-west-1-green/1a2b3c4d-5e6f-7890-abcd-ef0123456789"
		name2 = "my-production-application-compute-stack-eu-west-1-green"
	)
	dir := filepath.Join(home, ".awless-ro", "aws", "rdf", "default", "eu-west-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	triples := `<s1> <cloud:id> "` + arn1 + `" .
<s1> <cloud:name> "` + name1 + `" .
<s1> <cloud:state> "UPDATE_ROLLBACK_COMPLETE" .
<s1> <rdf:type> <cloud-owl:Stack> .
<s2> <cloud:id> "` + arn2 + `" .
<s2> <cloud:name> "` + name2 + `" .
<s2> <cloud:state> "CREATE_COMPLETE" .
<s2> <rdf:type> <cloud-owl:Stack> .
`
	if err := os.WriteFile(filepath.Join(dir, "cloudformation.nt"), []byte(triples), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(extra ...string) string {
		t.Helper()
		cmd := exec.Command(bin, append([]string{"list", "stacks", "--local"}, extra...)...)
		cmd.Env = append(os.Environ(), "HOME="+home,
			"AWS_REGION=eu-west-1", "AWS_DEFAULT_REGION=eu-west-1", "AWS_EC2_METADATA_DISABLED=true",
			"AWS_ACCESS_KEY_ID=", "AWS_SECRET_ACCESS_KEY=", "AWS_PROFILE=")
		out, err := cmd.Output() // stdout is a pipe
		if err != nil {
			t.Fatalf("%v: %v\n%s", extra, err, out)
		}
		return string(out)
	}
	run() // the first run sets the home up and may greet; not what is under test

	plain := run()
	lines := strings.Split(plain, "\n")
	for _, row := range [][2]string{{arn1, name1}, {arn2, name2}} {
		found := false
		for _, l := range lines {
			if strings.Contains(l, row[0]) && strings.Contains(l, row[1]) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s and its ARN are not whole on one line:\n%s", row[1], plain)
		}
	}
	if strings.Contains(plain, "truncated") {
		t.Errorf("columns dropped in a pipe:\n%s", plain)
	}

	if got := run("--max-width", "0"); got != plain {
		t.Errorf("--max-width 0 differs from the default in a pipe:\n%s\nwant\n%s", got, plain)
	}

	narrow := run("--max-width", "80")
	if !strings.Contains(narrow, "truncated to fit terminal") {
		t.Errorf("--max-width 80 dropped nothing:\n%s", narrow)
	}
	for _, l := range strings.Split(narrow, "\n") {
		if strings.Contains(l, "truncated") {
			break
		}
		if n := utf8.RuneCountInString(l); n > 80 {
			t.Errorf("--max-width 80 printed a line of %d: %q", n, l)
		}
	}

	if csv, csv80 := run("--format", "csv"), run("--format", "csv", "--max-width", "80"); csv != csv80 || !strings.Contains(csv, arn1) {
		t.Errorf("--max-width changed csv:\n%s\nwant\n%s", csv80, csv)
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
