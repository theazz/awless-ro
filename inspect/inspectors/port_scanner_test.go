package inspectors

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

func mustRule(t *testing.T, protocol string, from, to int64, cidrs []string, sources ...string) *graph.FirewallRule {
	t.Helper()
	rule := &graph.FirewallRule{
		PortRange: graph.PortRange{FromPort: from, ToPort: to},
		Protocol:  protocol,
		Sources:   sources,
	}
	for _, c := range cidrs {
		_, cidr, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatal(err)
		}
		rule.IPRanges = append(rule.IPRanges, cidr)
	}
	return rule
}

func allPortsRule(t *testing.T, cidr string) *graph.FirewallRule {
	t.Helper()
	rule := mustRule(t, "any", 0, 0, []string{cidr})
	rule.PortRange.Any = true
	return rule
}

func scanGraph(t *testing.T) *graph.Graph {
	t.Helper()

	bastionInst := resourcetest.Instance("i-bastion").Prop(properties.Name, "bastion-1").Build()
	webInst := resourcetest.Instance("i-web").Prop(properties.Name, "web-1").Build()
	dbInst := resourcetest.Instance("i-db").Prop(properties.Name, "db-1").Build()

	bastion := resourcetest.SecurityGroup("sg-open").
		Prop(properties.Name, "bastion").
		Prop(properties.InboundRules, []*graph.FirewallRule{
			mustRule(t, "tcp", 22, 22, []string{"0.0.0.0/0"}),
		}).Build()
	web := resourcetest.SecurityGroup("sg-v6").
		Prop(properties.Name, "web").
		Prop(properties.InboundRules, []*graph.FirewallRule{
			mustRule(t, "tcp", 443, 443, []string{"::/0"}),
			mustRule(t, "tcp", 80, 80, []string{"0.0.0.0/0"}, "pl-123"),
		}).Build()
	db := resourcetest.SecurityGroup("sg-db").
		Prop(properties.Name, "postgres").
		Prop(properties.InboundRules, []*graph.FirewallRule{
			mustRule(t, "tcp", 5432, 5432, []string{"10.0.0.0/16"}, "sg-open"),
		}).Build()
	anon := resourcetest.SecurityGroup("sg-anon").
		Prop(properties.InboundRules, []*graph.FirewallRule{
			allPortsRule(t, "10.0.0.0/8"),
		}).Build()

	g := graph.NewGraph()
	if err := g.AddResource(bastion, web, db, anon, bastionInst, webInst, dbInst); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []struct{ sg, inst *graph.Resource }{
		{bastion, bastionInst},
		{web, webInst},
		{db, dbInst},
	} {
		if err := g.AddAppliesOnRelation(rel.sg, rel.inst); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

// The inspector used to print the port and nothing about who could reach it, so
// an open port and a port open to 0.0.0.0/0 read the same.
func TestPortScannerShowsRuleSources(t *testing.T) {
	var out bytes.Buffer
	scanner := &PortScanner{}
	if err := scanner.Inspect(scanGraph(t)); err != nil {
		t.Fatal(err)
	}
	scanner.Print(&out)
	got := out.String()

	for _, want := range []string{
		"Securitygroup bastion (sg-open), applying on @bastion-1[instance]:",
		"port 22/tcp from 0.0.0.0/0  (open to the internet)",
		"Securitygroup web (sg-v6), applying on @web-1[instance]:",
		"port 443/tcp from ::/0  (open to the internet)",
		"port 80/tcp from 0.0.0.0/0, prefix list pl-123  (open to the internet)",
		"Securitygroup postgres (sg-db), applying on @db-1[instance]:",
		"port 5432/tcp from 10.0.0.0/16, sg bastion (sg-open)",
		"Securitygroup sg-anon applying on nothing:",
		"all ports/any protocol from 10.0.0.0/8",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}
}

// Groups open to the internet come first, then by name: the inspector printed
// them in map order, so the same account gave a different report each run.
func TestPortScannerOutputIsOrderedAndStable(t *testing.T) {
	first := ""
	for range 5 {
		var out bytes.Buffer
		scanner := &PortScanner{}
		if err := scanner.Inspect(scanGraph(t)); err != nil {
			t.Fatal(err)
		}
		scanner.Print(&out)

		if first == "" {
			first = out.String()
		} else if out.String() != first {
			t.Fatalf("two runs over the same graph printed different output:\n%s\nand\n%s", first, out.String())
		}
	}

	order := []string{
		"Securitygroup bastion (sg-open)",
		"Securitygroup web (sg-v6)",
		"Securitygroup postgres (sg-db)",
		"Securitygroup sg-anon",
	}
	last := -1
	for _, want := range order {
		i := strings.Index(first, want)
		if i < 0 {
			t.Fatalf("output is missing %q:\n%s", want, first)
		}
		if i < last {
			t.Fatalf("%q is out of order:\n%s", want, first)
		}
		last = i
	}
}

// A rule whose sources are empty still has to say so rather than printing a
// dangling "from".
func TestPortScannerRuleWithoutSources(t *testing.T) {
	g := graph.NewGraph()
	sg := resourcetest.SecurityGroup("sg-empty").
		Prop(properties.InboundRules, []*graph.FirewallRule{mustRule(t, "tcp", 22, 22, nil)}).Build()
	if err := g.AddResource(sg); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	scanner := &PortScanner{}
	if err := scanner.Inspect(g); err != nil {
		t.Fatal(err)
	}
	scanner.Print(&out)

	if !strings.Contains(out.String(), "port 22/tcp from no source") {
		t.Errorf("a rule without sources is not reported:\n%s", out.String())
	}
}
