package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	awsconfig "github.com/theazz/awless-ro/aws/config"
	"github.com/theazz/awless-ro/graph"
	rt "github.com/theazz/awless-ro/graph/resourcetest"
)

// values strips the descriptions cobra carries after a tab, so tests compare what the
// shell would insert.
func values(completions []cobra.Completion) []string {
	var out []string
	for _, c := range completions {
		v, _, _ := strings.Cut(c, "\t")
		out = append(out, v)
	}
	return out
}

func TestResourceRefsOfferIdsAndNames(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		rt.Instance("i-1").Prop("ID", "i-1").Prop("Name", "web").Build(),
		rt.Instance("i-2").Prop("ID", "i-2").Prop("Name", "web").Build(),
		rt.Instance("i-3").Prop("ID", "i-3").Build(),
		rt.Instance("i-4").Prop("ID", "i-4").Prop("Name", "has space").Build(),
		rt.VPC("vpc-1").Prop("ID", "vpc-1").Prop("Name", "main").Build(),
	)

	got := resourceRefs(g, []string{"instance"})

	// Names are not unique in AWS; "web" is offered once and show lists the
	// candidates. A name with a space cannot be completed as one shell word.
	want := []string{"i-1", "i-2", "i-3", "i-4", "web"}
	if !reflect.DeepEqual(values(got), want) {
		t.Errorf("got %v, want %v", values(got), want)
	}

	for _, c := range got {
		if strings.HasPrefix(c, "i-1\t") && c != "i-1\tinstance web" {
			t.Errorf("an id should be described by its type and name, got %q", c)
		}
	}
}

func TestResourceRefsOnlyOfferTheRequestedTypes(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		rt.Instance("i-1").Prop("ID", "i-1").Build(),
		rt.VPC("vpc-1").Prop("ID", "vpc-1").Build(),
	)

	if got := values(resourceRefs(g, []string{"vpc"})); !reflect.DeepEqual(got, []string{"vpc-1"}) {
		t.Errorf("got %v, want only the vpc", got)
	}
}

func withAWSHome(t *testing.T, config, credentials string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0600)
	os.WriteFile(filepath.Join(dir, "credentials"), []byte(credentials), 0600)

	previous := awsconfig.AWSHomeDir
	awsconfig.AWSHomeDir = func() string { return dir }
	t.Cleanup(func() { awsconfig.AWSHomeDir = previous })
}

func TestCompletionProfilesAreProfilesOnly(t *testing.T) {
	withAWSHome(t,
		"[default]\n[profile staging]\n[sso-session corp]\n[services local]\n[profile prod]\n",
		"[default]\n[prod]\n")

	want := []string{"default", "prod", "staging"}
	if got := completionProfiles(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v: sorted, once each, and no sso-session or services sections", got, want)
	}
}

func TestSwitchOffersWhatIsStillMissing(t *testing.T) {
	withAWSHome(t, "[profile staging]\n", "")

	has := func(cs []cobra.Completion, v string) bool {
		for _, c := range values(cs) {
			if c == v {
				return true
			}
		}
		return false
	}

	both, _ := completeSwitch(switchCmd, nil, "")
	if !has(both, "eu-west-1") || !has(both, "staging") {
		t.Errorf("with nothing given, both regions and profiles should be offered, got %v", values(both))
	}

	afterRegion, _ := completeSwitch(switchCmd, []string{"eu-west-1"}, "")
	if has(afterRegion, "eu-west-2") || !has(afterRegion, "staging") {
		t.Errorf("after a region only profiles should be offered, got %v", values(afterRegion))
	}

	afterProfile, _ := completeSwitch(switchCmd, []string{"staging"}, "")
	if !has(afterProfile, "eu-west-2") || has(afterProfile, "staging") {
		t.Errorf("after a profile only regions should be offered, got %v", values(afterProfile))
	}

	if full, _ := completeSwitch(switchCmd, []string{"staging", "eu-west-1"}, ""); len(full) != 0 {
		t.Errorf("switch takes two arguments at most, got %v", values(full))
	}
}

func TestConfigSetCompletesKeysThenValues(t *testing.T) {
	withAWSHome(t, "[profile staging]\n[sso-session corp]\n", "")

	keys, _ := completeConfigSet(configSetCmd, nil, "aws.region")
	if got := values(keys); !reflect.DeepEqual(got, []string{"aws.region"}) {
		t.Errorf("key completion: got %v", got)
	}

	cases := []struct {
		key  string
		want []string
	}{
		{"autosync", []string{"true", "false"}},
		{"aws.profile", []string{"staging"}},
	}
	for _, tc := range cases {
		got, _ := completeConfigSet(configSetCmd, []string{tc.key}, "")
		if !reflect.DeepEqual(values(got), tc.want) {
			t.Errorf("values for %s: got %v, want %v", tc.key, values(got), tc.want)
		}
	}

	regions, _ := completeConfigSet(configSetCmd, []string{"aws.region"}, "eu-west-")
	if len(regions) == 0 {
		t.Error("aws.region should complete to regions")
	}

	if got, _ := completeConfigSet(configSetCmd, []string{"autosync", "true"}, ""); len(got) != 0 {
		t.Errorf("nothing follows the value, got %v", values(got))
	}
}

func TestSSHTargetKeepsTheUser(t *testing.T) {
	// No synced data in the test environment, so nothing is offered; what matters
	// here is that a USER@ prefix does not turn into a lookup for a resource named
	// "ec2-user@...".
	got, directive := completeSSHTarget(sshCmd, nil, "ec2-user@")
	for _, c := range values(got) {
		if !strings.HasPrefix(c, "ec2-user@") {
			t.Errorf("completion %q lost the user", c)
		}
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("an instance reference is never a file, got directive %d", directive)
	}
}
