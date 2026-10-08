package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// What `--filter` means is part of the CLI's public surface, and the two
// upstream reports this answers (wallix/awless#252 and #296) were both written
// as a command and its output. The unit tests pin the predicate; these runs pin
// the whole path from the flag to the printed rows.
//
// Everything comes from a synthesized local graph under a throwaway HOME, so
// there are no credentials, no AWS call and no network. The values are invented.
const (
	activeKeyID   = "AKIAACTIVEKEY000001"
	inactiveKeyID = "AKIAINACTIVEKEY00002"
)

func TestFilterExactMatchFromTheCommandLine(t *testing.T) {
	bin := build(t)
	home := fixtureHome(t)

	cases := []struct {
		name    string
		args    []string
		present []string
		absent  []string
	}{
		{
			// #252: the exact form is the one that can exclude "Inactive".
			name:    "accesskeys state==Active",
			args:    []string{"list", "accesskeys", "--local", "--filter", "state==Active"},
			present: []string{activeKeyID, "Active"},
			absent:  []string{inactiveKeyID},
		},
		{
			// The regression guard: `=` is still a substring match, so this
			// still returns both keys. If it ever returns one, every
			// documented --filter example has changed meaning.
			name:    "accesskeys state=active stays a substring",
			args:    []string{"list", "accesskeys", "--local", "--filter", "state=active"},
			present: []string{activeKeyID, inactiveKeyID},
		},
		{
			name:    "accesskeys state==Active is case insensitive",
			args:    []string{"list", "accesskeys", "--local", "--filter", "state==active"},
			present: []string{activeKeyID},
			absent:  []string{inactiveKeyID},
		},
		{
			// #296: "cname", "soa" and "aaaa" all contain "a".
			name:    "records type==A",
			args:    []string{"list", "records", "--local", "--filter", "type==A", "--columns", "type,name"},
			present: []string{"mysite.com"},
			absent:  []string{"CNAME", "SOA", "AAAA", "anothersite.mysite.com"},
		},
		{
			name:    "records type=A stays a substring",
			args:    []string{"list", "records", "--local", "--filter", "type=A", "--columns", "type,name"},
			present: []string{"CNAME", "SOA", "AAAA", "anothersite.mysite.com"},
		},
		{
			// An exact value that no row carries excludes everything, rather
			// than falling back to the substring behaviour.
			name:   "records type==CNAM matches nothing",
			args:   []string{"list", "records", "--local", "--filter", "type==CNAM", "--columns", "type,name"},
			absent: []string{"CNAME", "SOA", "AAAA", "A,mysite.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// csv, so no column-width logic can elide or wrap a value.
			out := runOffline(t, bin, home, append(tc.args, "--format", "csv")...)
			for _, want := range tc.present {
				if !strings.Contains(out, want) {
					t.Errorf("%q is missing from the output:\n%s", want, out)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(out, unwanted) {
					t.Errorf("%q should have been filtered out:\n%s", unwanted, out)
				}
			}
		})
	}
}

// The operator is only usable if --help names it.
func TestListHelpNamesBothFilterForms(t *testing.T) {
	out := runOffline(t, build(t), t.TempDir(), "list", "--help")
	for _, want := range []string{"key=value", "key==value", "substring", "whole value"} {
		if !strings.Contains(out, want) {
			t.Errorf("list --help should mention %q, got:\n%s", want, out)
		}
	}
}

// runOffline runs the binary with nothing inherited from the environment, so an
// AWS variable or profile on the developer's machine cannot change the profile
// directory, reach the metadata service or pick up credentials.
func runOffline(t *testing.T, bin, home string, args ...string) string {
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
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("could not run the binary: %s", err)
		}
		t.Fatalf("%v exited %d:\n%s", args, exit.ExitCode(), out)
	}
	return string(out)
}

// fixtureHome lays out a local graph the way a sync would: access and dns are
// global rather than per-region (sync.LoadLocalGraphForService).
func fixtureHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	dir := filepath.Join(home, ".awless-ro", "aws", "rdf", "default", "global")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	access := graph.NewGraph()
	if err := access.AddResource(
		resourcetest.AccessKey(activeKeyID).Prop(properties.State, "Active").Prop(properties.Username, "alice").Build(),
		resourcetest.AccessKey(inactiveKeyID).Prop(properties.State, "Inactive").Prop(properties.Username, "bob").Build(),
	); err != nil {
		t.Fatal(err)
	}

	dns := graph.NewGraph()
	if err := dns.AddResource(
		resourcetest.Record("rec_a").Prop(properties.Type, "A").Prop(properties.Name, "mysite.com").Build(),
		resourcetest.Record("rec_cname").Prop(properties.Type, "CNAME").Prop(properties.Name, "anothersite.mysite.com").Build(),
		resourcetest.Record("rec_soa").Prop(properties.Type, "SOA").Prop(properties.Name, "mysite.com").Build(),
		resourcetest.Record("rec_aaaa").Prop(properties.Type, "AAAA").Prop(properties.Name, "mysite.com").Build(),
	); err != nil {
		t.Fatal(err)
	}

	for name, g := range map[string]*graph.Graph{"access": access, "dns": dns} {
		f, err := os.Create(filepath.Join(dir, name+".nt"))
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
	}
	return home
}
