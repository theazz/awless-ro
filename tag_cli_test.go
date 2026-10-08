package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// #28: a comma inside a tag value. pflag splits `--tag 'Environment=Not, tagged'`
// into "Environment=Not" and " tagged", the second half used to be dropped, and
// the command answered "No results found." with exit status 0. The flag, its
// output and the exit status are the public surface, so these run the binary.
//
// No credentials, no network beyond loopback: the graph is synthesized under a
// throwaway HOME, and the one test that needs an AWS endpoint gets a local fake.
// Every value is invented.

const tagQuotingHint = `--tag '"Environment=Not, tagged"'`

// The check has to come before anything talks to AWS: the hooks it precedes
// resolve credentials, which can mean assuming a role or prompting for an MFA
// code, and may sync the whole account. The fake endpoint counts what reaches it;
// the control run shows that it is really where requests go, otherwise a count of
// zero would prove nothing.
func TestTagFlagIsCheckedBeforeAnyAWSCall(t *testing.T) {
	bin := build(t)

	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "fake AWS endpoint for tests", http.StatusInternalServerError)
	}))
	defer srv.Close()
	env := []string{
		"AWS_ENDPOINT_URL=" + srv.URL,
		"AWS_MAX_ATTEMPTS=1",
	}
	// A profile with invented static keys, in a home of its own: the credential
	// resolver loads the named profile from the shared files, so environment
	// keys alone are not enough to get as far as an API call.
	homeWithFakeKeys := func(t *testing.T) string {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o700); err != nil {
			t.Fatal(err)
		}
		creds := "[default]\naws_access_key_id = AKIDFAKEFAKEFAKEFAKE\naws_secret_access_key = fake\n"
		if err := os.WriteFile(filepath.Join(home, ".aws", "credentials"), []byte(creds), 0o600); err != nil {
			t.Fatal(err)
		}
		return home
	}

	t.Run("control: a valid tag reaches the endpoint", func(t *testing.T) {
		before := requests.Load()
		_, stderr, _ := runTagCLI(t, bin, homeWithFakeKeys(t), env, "list", "instances", "--no-sync", "--tag", "Env=Prod")
		if requests.Load() == before {
			t.Fatalf("the fake endpoint saw no request, so it cannot show that a rejected command makes none. stderr:\n%s", stderr)
		}
	})

	for _, tc := range []struct {
		name, tag string
	}{
		{"comma value", "Environment=Not, tagged"},
		{"bare quote", `Environment="Not, tagged"`},
	} {
		t.Run("rejected: "+tc.name, func(t *testing.T) {
			before := requests.Load()
			stdout, stderr, exit := runTagCLI(t, bin, homeWithFakeKeys(t), env, "list", "instances", "--no-sync", "--tag", tc.tag)
			if exit != 1 {
				t.Errorf("exit status %d, want 1", exit)
			}
			if !strings.Contains(stdout+stderr, tagQuotingHint) {
				t.Errorf("the error should show %s, got:\nstdout:\n%s\nstderr:\n%s", tagQuotingHint, stdout, stderr)
			}
			if strings.Contains(stdout, "No results found.") {
				t.Errorf("still answers as if the query had run:\n%s", stdout)
			}
			if n := requests.Load() - before; n != 0 {
				t.Errorf("%d request(s) reached AWS before the tag was refused", n)
			}
		})
	}
}

// The whole path from the flag to the printed rows, on a local graph with one
// instance per case: a space in the value (#269), a comma in the value (#28), and
// two plain tags (the shorthand).
func TestTagFlagAgainstALocalGraph(t *testing.T) {
	bin := build(t)
	home := tagGraphHome(t)
	// The first run in a new home prints a welcome on stderr; get it out of the
	// way so the cases below see only what they cause.
	if _, stderr, exit := runTagCLI(t, bin, home, nil, "list", "instances", "--local"); exit != 0 {
		t.Fatalf("first run exited %d:\n%s", exit, stderr)
	}

	cases := []struct {
		name   string
		args   []string
		exit   int
		ids    []string // exactly these instances are listed; nil means none checked
		stderr []string
		quiet  bool // nothing on stderr
	}{
		{
			// The bug. Before: exit 0 and "No results found." (an empty csv here).
			name:   "unquoted comma value is refused",
			args:   []string{"--tag", "Environment=Not, tagged"},
			exit:   1,
			stderr: []string{`" tagged"`, tagQuotingHint},
		},
		{
			name: "quoted comma value finds the instance",
			args: []string{"--tag", `"Environment=Not, tagged"`},
			ids:  []string{"i-0comma"},
		},
		{
			name: "a space and no comma still works (#269)",
			args: []string{"--tag", "Environment=Not tagged"},
			ids:  []string{"i-0space"},
		},
		{
			name: "shorthand",
			args: []string{"--tag", "Environment=Production,Dept=Marketing"},
			ids:  []string{"i-0plain"},
		},
		{
			name: "repeated flag",
			args: []string{"--tag", "Environment=Production", "--tag", "Dept=Marketing"},
			ids:  []string{"i-0plain"},
		},
		{
			name:   "a backslash does not escape the comma",
			args:   []string{"--tag", `Environment=Not\, tagged`},
			exit:   1,
			stderr: []string{`" tagged"`, tagQuotingHint},
		},
		{
			name:   "bare quote gets the quoting hint",
			args:   []string{"--tag", `Environment="Not, tagged"`},
			exit:   1,
			stderr: []string{"bare", tagQuotingHint},
		},
		{
			// Only a warning: a leading space is legal in a tag value.
			name:   "tag-value split at comma-space warns",
			args:   []string{"--tag-value", "Not, tagged"},
			ids:    []string{},
			stderr: []string{`--tag-value " tagged"`, `--tag-value '"Not, tagged"'`},
		},
		{
			name:  "quoted tag-value finds the instance",
			args:  []string{"--tag-value", `"Not, tagged"`},
			ids:   []string{"i-0comma"},
			quiet: true,
		},
		{
			name:  "tag-value with a space still works (#269)",
			args:  []string{"--tag-value", "Not tagged"},
			ids:   []string{"i-0space"},
			quiet: true,
		},
		{
			name:  "--silent silences the warning",
			args:  []string{"--silent", "--tag-value", "Not, tagged"},
			ids:   []string{},
			quiet: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// csv, so no column-width logic can elide or wrap a value.
			args := append([]string{"list", "instances", "--local", "--format", "csv", "--columns", "id,name"}, tc.args...)
			stdout, stderr, exit := runTagCLI(t, bin, home, nil, args...)
			if exit != tc.exit {
				t.Errorf("exit status %d, want %d.\nstdout:\n%s\nstderr:\n%s", exit, tc.exit, stdout, stderr)
			}
			if tc.ids != nil {
				for _, id := range []string{"i-0space", "i-0comma", "i-0plain"} {
					want := false
					for _, w := range tc.ids {
						want = want || w == id
					}
					if got := strings.Contains(stdout, id); got != want {
						t.Errorf("%s listed: %t, want %t. stdout:\n%s", id, got, want, stdout)
					}
				}
			}
			// A refused command prints nothing on stdout, not even a header, so a
			// script cannot mistake it for an empty answer.
			if tc.exit != 0 && stdout != "" {
				t.Errorf("stdout should be empty, got:\n%s", stdout)
			}
			for _, want := range tc.stderr {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr should contain %s, got:\n%s", want, stderr)
				}
			}
			if tc.quiet && stderr != "" {
				t.Errorf("stderr should be empty, got:\n%s", stderr)
			}
		})
	}
}

// The quoting is only usable if --help shows it.
func TestTagFlagHelpShowsTheQuoting(t *testing.T) {
	stdout, stderr, exit := runTagCLI(t, build(t), t.TempDir(), nil, "list", "instances", "--help")
	if exit != 0 {
		t.Fatalf("exit status %d:\n%s", exit, stderr)
	}
	for _, want := range []string{`--tag '"Env=Not, tagged"'`, `--tag-value '"Not, tagged"'`, `--tag-key '"Cost, center"'`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list instances --help should show %s, got:\n%s", want, stdout)
		}
	}
}

// runTagCLI runs the binary with nothing inherited from the environment but
// extraEnv, so an AWS variable or profile on the developer's machine cannot pick
// the profile, reach the metadata service or supply credentials. stdout and
// stderr are kept apart because where a message lands is part of the behaviour.
func runTagCLI(t *testing.T, bin, home string, extraEnv []string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"AWS_REGION=eu-west-1",
		"AWS_DEFAULT_REGION=eu-west-1",
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_PROFILE=",
	}, extraEnv...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("could not run the binary: %s", err)
		}
		exit = exitErr.ExitCode()
	}
	return out.String(), errOut.String(), exit
}

// tagGraphHome lays out a local infra graph where a sync would put it
// (sync.LoadLocalGraphForService: infra is regional).
func tagGraphHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	dir := filepath.Join(home, ".awless-ro", "aws", "rdf", "default", "eu-west-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	g := graph.NewGraph()
	if err := g.AddResource(
		resourcetest.Instance("i-0space").Prop(properties.Name, "space").Prop(properties.Tags, []string{"Environment=Not tagged"}).Build(),
		resourcetest.Instance("i-0comma").Prop(properties.Name, "comma").Prop(properties.Tags, []string{"Environment=Not, tagged"}).Build(),
		resourcetest.Instance("i-0plain").Prop(properties.Name, "plain").Prop(properties.Tags, []string{"Environment=Production", "Dept=Marketing"}).Build(),
	); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "infra.nt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.MarshalTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return home
}
