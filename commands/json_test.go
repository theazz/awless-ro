package commands

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	awsservices "github.com/theazz/awless-ro/aws/services"
	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
	"github.com/theazz/awless-ro/internal/golden"
	"github.com/theazz/awless-ro/sync"
)

func assertJSONGolden(t *testing.T, name string, v interface{}) {
	t.Helper()
	var w bytes.Buffer
	if err := writeJSON(&w, v); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, name, w.Bytes())
}

// withConfig replaces the loaded configuration for the duration of a test.
func withConfig(t *testing.T, conf, defaults map[string]interface{}) {
	t.Helper()
	prevConf, prevDefaults := config.Config, config.Defaults
	t.Cleanup(func() { config.Config, config.Defaults = prevConf, prevDefaults })
	config.Config, config.Defaults = conf, defaults
}

func TestVersionJSONGolden(t *testing.T) {
	assertJSONGolden(t, "version", newVersionOutput(config.BuildInfo{
		Version: "v0.6.0", Sha: "5ff53f6d", Date: "2026-10-14T00:00:00Z", OS: "darwin", Arch: "arm64", For: "brew",
	}))
	// A plain `go build` sets none of the build fields.
	assertJSONGolden(t, "version-dev", newVersionOutput(config.BuildInfo{Version: "v0.6.0"}))
}

func TestWhoamiJSONGolden(t *testing.T) {
	user := &awsservices.Identity{
		Account: "123456789012", Arn: "arn:aws:iam::123456789012:user/jsmith", UserId: "AIDAEXAMPLE",
		ResourceType: "user", ResourcePath: "user/jsmith", Resource: "jsmith",
	}
	// Unsorted, as they arrive from concurrent calls.
	policies := &awsservices.UserPolicies{
		Username: "jsmith",
		Attached: []string{"ReadOnlyAccess", "IAMUserChangePassword"},
		ByGroup:  map[string][]string{"ops": {"b", "a"}},
	}
	assertJSONGolden(t, "whoami-user", newWhoamiOutput(user, policies))
	// Policies that could not be read are absent, not empty.
	assertJSONGolden(t, "whoami-user-unreadable-policies", newWhoamiOutput(user, nil))

	role := &awsservices.Identity{
		Account: "123456789012", Arn: "arn:aws:sts::123456789012:assumed-role/admin/session", UserId: "AROAEXAMPLE:session",
		ResourceType: "assumed-role", ResourcePath: "assumed-role/admin/session", Resource: "admin/session",
	}
	assertJSONGolden(t, "whoami-role", newWhoamiOutput(role, nil))
}

func TestConfigJSONGolden(t *testing.T) {
	withConfig(t,
		map[string]interface{}{"aws.region": "eu-west-1", "aws.profile": "default", "autosync": true, "aws.storage.s3object.sync": false},
		map[string]interface{}{"region": "us-east-1"},
	)
	assertJSONGolden(t, "config", config.Values())

	v, _ := config.Get("aws.region")
	assertJSONGolden(t, "config-get", configValue{Key: "aws.region", Value: v})
	unset, _ := config.Get("not.set")
	assertJSONGolden(t, "config-get-unset", configValue{Key: "not.set", Value: unset})
}

func TestSwitchJSONGolden(t *testing.T) {
	withConfig(t, map[string]interface{}{"aws.region": "eu-west-1", "aws.profile": "staging"}, map[string]interface{}{})
	assertJSONGolden(t, "switch", currentProfileAndRegion())
}

func TestSyncJSONGolden(t *testing.T) {
	withConfig(t, map[string]interface{}{"aws.storage.s3object.sync": false}, map[string]interface{}{})

	g := graph.NewGraph()
	if err := g.AddResource(
		resourcetest.Bucket("logs").Prop(p.Name, "logs").Build(),
		resourcetest.Bucket("backups").Build(),
	); err != nil {
		t.Fatal(err)
	}
	stats := syncStats("storage", g)
	assertJSONGolden(t, "sync", syncOutput{Profile: "default", Region: "eu-west-1", Services: []syncedService{stats}, Errors: []string{}})

	// The stderr line names what is off rather than counting it as zero.
	if got, want := stats.String(), "storage: 2 buckets, s3objects: off (aws.storage.s3object.sync)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckFormat(t *testing.T) {
	prev := formatGlobalFlag
	t.Cleanup(func() { formatGlobalFlag = prev })
	listInstances, _, err := RootCmd.Find([]string{"list", "instances"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		cmd     *cobra.Command
		format  string
		refused bool
	}{
		{whoamiCmd, "table", false},
		{whoamiCmd, "json", false},
		{whoamiCmd, "csv", true},
		{configGetCmd, "json", false},
		{configGetCmd, "porcelain", true},
		{stackEventsCmd, "json", false},
		{sshCmd, "table", false},
		{sshCmd, "json", true},
		// list keeps its own answer to every format, the unknown ones included.
		{listCmd, "csv", false},
		{listCmd, "yaml", false},
		{listInstances, "tsv", false},
	}
	for _, c := range cases {
		formatGlobalFlag = c.format
		err := checkFormat(c.cmd)
		if refused := err != nil; refused != c.refused {
			t.Errorf("%s --format %s: refused=%t, want %t (%v)", c.cmd.CommandPath(), c.format, refused, c.refused, err)
		}
	}

	formatGlobalFlag = "csv"
	if err := checkFormat(whoamiCmd); err == nil || !strings.Contains(err.Error(), "--format table or --format json") {
		t.Errorf("the error does not say what to use instead: %v", err)
	}
}

// checkFormat runs from applyHooks and chainHooks, so a command is covered as long as
// cobra runs one of them for it. Every runnable command has to have a pre-run hook of
// its own or inherit a persistent one.
func TestEveryCommandRunsAHookThatChecksTheFormat(t *testing.T) {
	var check func(c *cobra.Command)
	check = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			check(sub)
		}
		if !c.Runnable() || c == RootCmd || c.Name() == "help" || c.Name() == "completion" || strings.HasPrefix(c.Name(), "__") {
			return
		}
		if c.PreRun != nil || c.PreRunE != nil {
			return
		}
		for p := c; p != nil; p = p.Parent() {
			if p.PersistentPreRun != nil || p.PersistentPreRunE != nil {
				return
			}
		}
		t.Errorf("%s runs no hook, so its --format is never checked", c.CommandPath())
	}
	check(RootCmd)
}

// A service that could not be read is named, with the reason, rather than counted as
// an account holding nothing.
func TestSyncJSONWithAFailedService(t *testing.T) {
	withConfig(t, map[string]interface{}{"aws.storage.s3object.sync": false}, map[string]interface{}{})

	failed, other := splitSyncErrors(sync.Errors{
		&sync.ServiceError{Service: "access", Err: errors.New("AccessDenied: not authorized to perform iam:ListUsers")},
		errors.New("creating /nope: permission denied"),
	})
	if want := map[string]string{"access": "AccessDenied: not authorized to perform iam:ListUsers"}; len(failed) != 1 || failed["access"] != want["access"] {
		t.Fatalf("failed services: got %v, want %v", failed, want)
	}
	if len(other) != 1 || !strings.Contains(other[0], "/nope") {
		t.Fatalf("other errors: got %v", other)
	}

	access := syncStats("access", graph.NewGraph())
	access.Error = failed["access"]
	assertJSONGolden(t, "sync-failed-service", syncOutput{Profile: "default", Region: "eu-west-1", Services: []syncedService{access}, Errors: other})

	if f, o := splitSyncErrors(nil); len(f) != 0 || o == nil || len(o) != 0 {
		t.Errorf("no error: got %v and %#v, want nothing and an empty list", f, o)
	}
}

func TestFormatCompletionOffersWhatTheCommandTakes(t *testing.T) {
	listInstances, _, err := RootCmd.Find([]string{"list", "instances"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cmd  *cobra.Command
		want string
	}{
		{whoamiCmd, "table json"},
		{sshCmd, "table"},
		{listInstances, "table csv tsv json porcelain"},
	}
	for _, c := range cases {
		got, _ := completeFormat(c.cmd, nil, "")
		var names []string
		for _, g := range got {
			names = append(names, strings.SplitN(string(g), "\t", 2)[0])
		}
		if strings.Join(names, " ") != c.want {
			t.Errorf("%s: got %v, want %s", c.cmd.CommandPath(), names, c.want)
		}
	}
}
