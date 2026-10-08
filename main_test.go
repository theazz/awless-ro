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

// The exit code of a `--local` command is part of the CLI contract: a script reading
// `list ... --format json` only sees the exit code of the last element of its
// pipeline. A graph file that cannot be read used to be swallowed into "No results
// found." with exit 0 — the same answer as a genuinely empty account. These run the
// built binary against synthesized .nt fixtures (no credentials, no AWS call, no
// network) and assert on the exit code the way only a process-level test can.

const (
	exitGoodInfraNT = "<i-1> <cloud:id> \"i-1\" .\n" +
		"<i-1> <cloud:name> \"web\" .\n" +
		"<i-1> <rdf:type> <cloud-owl:Instance> .\n"

	exitBrokenAccessNT = "<pol-1> <cloud:id> \"pol-1\" .\n" +
		"THIS IS NOT N-TRIPLES\n" +
		"<pol-1> <rdf:type> <cloud-owl:Policy> .\n"
)

// graphHome creates an awless home under a fresh temp dir and writes the given
// fixture files. files maps "<regionDir>/<service>" to the file content, e.g.
// "eu-west-1/infra" or "global/access".
func graphHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	for key, content := range files {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			t.Fatalf("bad fixture key %q, want <regionDir>/<service>", key)
		}
		dir := filepath.Join(home, ".awless-ro", "aws", "rdf", "default", parts[0])
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, parts[1]+".nt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// runLocal runs the binary with an environment that reaches no AWS: --local makes no
// call, and the explicitly emptied credentials plus the disabled metadata endpoint
// mean the first-install path runs non-interactively without touching the network.
func runLocal(t *testing.T, bin, home string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"AWS_REGION=eu-west-1",
		"AWS_DEFAULT_REGION=eu-west-1",
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_ACCESS_KEY_ID=",
		"AWS_SECRET_ACCESS_KEY=",
		"AWS_PROFILE=",
	}
	cmd.Stdin = nil
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()

	exit = 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("could not run the binary: %s", err)
	}
	return outBuf.String(), errBuf.String(), exit
}

// The bug: a command whose own service file is unreadable claimed the account held
// nothing, with the exit code of success.
func TestLocalListUnreadableServiceFails(t *testing.T) {
	bin := build(t)
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
		"global/access":   exitBrokenAccessNT,
	})

	stdout, stderr, exit := runLocal(t, bin, home, "list", "policies", "--local")
	if exit != 1 {
		t.Errorf("exit = %d, want 1 for an unreadable service file", exit)
	}
	if !strings.Contains(stderr, "access.nt") {
		t.Errorf("stderr should name the unreadable file, got:\n%s", stderr)
	}
	if strings.Contains(stdout, "No results found.") {
		t.Errorf("a failed load must not be reported as an empty result; stdout:\n%s", stdout)
	}
}

// The multi-service rule: a broken access.nt must not void a good infra.nt.
func TestLocalListGoodServiceUnaffectedBySiblingBreakage(t *testing.T) {
	bin := build(t)
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
		"global/access":   exitBrokenAccessNT,
	})

	stdout, _, exit := runLocal(t, bin, home, "list", "instances", "--local")
	if exit != 0 {
		t.Errorf("exit = %d, want 0: a broken sibling file must not fail this listing", exit)
	}
	if !strings.Contains(stdout, "i-1") {
		t.Errorf("the instance from a good infra.nt should still list, got:\n%s", stdout)
	}
}

// show reads every file for the profile, so a broken one fails it — this already
// worked and must not regress the other way.
func TestLocalShowOnBrokenFileStillFails(t *testing.T) {
	bin := build(t)
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
		"global/access":   exitBrokenAccessNT,
	})

	_, _, exit := runLocal(t, bin, home, "show", "pol-1", "--local")
	if exit != 1 {
		t.Errorf("exit = %d, want 1 for show over a broken file", exit)
	}
}

// A real empty answer stays a real empty answer: a good infra.nt holds no vpcs.
func TestLocalListGenuinelyEmptyReportsEmpty(t *testing.T) {
	bin := build(t)
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
	})

	stdout, _, exit := runLocal(t, bin, home, "list", "vpcs", "--local")
	if exit != 0 {
		t.Errorf("exit = %d, want 0 for a genuinely empty result", exit)
	}
	if !strings.Contains(stdout, "No results found.") {
		t.Errorf("a genuinely empty result should say so, got:\n%s", stdout)
	}
}

// Nothing synced at all: the "nothing has been synced" line, exit 0. Unchanged.
func TestLocalListNothingSynced(t *testing.T) {
	bin := build(t)
	home := t.TempDir()

	stdout, stderr, exit := runLocal(t, bin, home, "list", "instances", "--local")
	if exit != 0 {
		t.Errorf("exit = %d, want 0 when nothing has been synced", exit)
	}
	if !strings.Contains(stdout+stderr, "sync") {
		t.Errorf("expected a hint that nothing has been synced, got stdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

// Part 2 end to end: a 9 MiB document literal — far past the old scanner ceiling — loads
// and lists. `list` does not render the document value itself, so this is the end-to-end
// proof that the decoder now reads an arbitrarily large single line.
func TestLocalHugeLiteralLoads(t *testing.T) {
	bin := build(t)
	doc := strings.Repeat("x", 9<<20)
	hugeAccessNT := "<pol-huge> <cloud:id> \"pol-huge\" .\n" +
		"<pol-huge> <cloud:name> \"huge-policy\" .\n" +
		"<pol-huge> <cloud:document> \"" + doc + "\" .\n" +
		"<pol-huge> <rdf:type> <cloud-owl:Policy> .\n"
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
		"global/access":   hugeAccessNT,
	})

	stdout, _, exit := runLocal(t, bin, home, "list", "policies", "--local")
	if exit != 0 {
		t.Errorf("list policies exit = %d, want 0 for a huge literal", exit)
	}
	if !strings.Contains(stdout, "pol-huge") {
		t.Errorf("the huge policy should list, got:\n%s", stdout)
	}
}

// show renders every property, so it exercises the full decode-and-display round trip for
// a large single value — the already-fixed half of upstream awless #300.
//
// The literal is kept at 128 KiB on purpose: twice the old 64 KiB bufio.Scanner default,
// so it is large enough to prove the round trip, yet it renders in about a second. Do NOT
// grow it. `show` rendering of a very large single value is quadratic in its size (a
// 1 MiB value already hangs), tracked separately as theazz/awless-ro#34; a multi-MB
// literal here would make `make test` hang rather than fail.
func TestLocalShowRendersLargeLiteral(t *testing.T) {
	bin := build(t)
	doc := strings.Repeat("x", 128<<10)
	bigAccessNT := "<pol-big> <cloud:id> \"pol-big\" .\n" +
		"<pol-big> <cloud:name> \"big-policy\" .\n" +
		"<pol-big> <cloud:document> \"" + doc + "\" .\n" +
		"<pol-big> <rdf:type> <cloud-owl:Policy> .\n"
	home := graphHome(t, map[string]string{
		"eu-west-1/infra": exitGoodInfraNT,
		"global/access":   bigAccessNT,
	})

	stdout, _, exit := runLocal(t, bin, home, "show", "pol-big", "--local")
	if exit != 0 {
		t.Errorf("show pol-big exit = %d, want 0 for a 128 KiB literal", exit)
	}
	if !strings.Contains(stdout, "pol-big") {
		t.Errorf("show should render the policy with a large value, got %.200q", stdout)
	}
}
