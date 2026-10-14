package console

import (
	"bytes"
	"encoding/json"
	"math"
	"net"
	"testing"
	"time"

	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
	"github.com/theazz/awless-ro/internal/golden"
)

// jsonGraph holds the value types a listing carries: strings, integers, booleans,
// dates, string lists, and firewall rules whose order the graph does not keep.
func jsonGraph(t *testing.T) *graph.Graph {
	t.Helper()
	cidr := func(s string) *net.IPNet {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	launched := time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)
	g := graph.NewGraph()
	err := g.AddResource(
		resourcetest.Instance("i-2").Prop(p.Name, "web").Prop(p.State, "running").Prop(p.Launched, launched).
			Prop(p.SecurityGroups, []string{"sg-2", "sg-1"}).Build(),
		resourcetest.Instance("i-1").Prop(p.Name, "db").Prop(p.State, "stopped").Build(),
		resourcetest.Subnet("subnet-1").Prop(p.Public, true).Prop(p.Vpc, "vpc-1").Build(),
		resourcetest.SecurityGroup("sg-1").Prop(p.Name, "web").
			Prop(p.InboundRules, []*graph.FirewallRule{
				{PortRange: graph.PortRange{FromPort: 443, ToPort: 443}, Protocol: "tcp", IPRanges: []*net.IPNet{cidr("10.0.0.0/8"), cidr("0.0.0.0/0")}},
				{PortRange: graph.PortRange{FromPort: 22, ToPort: 22}, Protocol: "tcp", Sources: []string{"sg-2"}},
			}).Build(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func printJSON(t *testing.T, g *graph.Graph, opts ...optsFn) []byte {
	t.Helper()
	displayer, err := BuildOptions(append([]optsFn{WithFormat("json")}, opts...)...).SetSource(g).Build()
	if err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	if err := displayer.Print(&w); err != nil {
		t.Fatal(err)
	}
	return w.Bytes()
}

func TestListJSONGolden(t *testing.T) {
	cases := []struct {
		name string
		g    func(*testing.T) *graph.Graph
		opts []optsFn
	}{
		{"list-instances", jsonGraph, []optsFn{WithRdfType("instance")}},
		{"list-securitygroups", jsonGraph, []optsFn{WithRdfType("securitygroup")}},
		{"list-instances-ids", jsonGraph, []optsFn{WithRdfType("instance"), WithIDsOnly(true)}},
		{"list-instances-empty", func(*testing.T) *graph.Graph { return graph.NewGraph() }, []optsFn{WithRdfType("instance")}},
		{"list-service", jsonGraph, nil},
		{"list-service-empty", func(*testing.T) *graph.Graph { return graph.NewGraph() }, nil},
		{"list-service-ids", jsonGraph, []optsFn{WithIDsOnly(true)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Several times over freshly built graphs: what the graph does not keep in
			// order must still come out the same every time.
			first := printJSON(t, c.g(t), c.opts...)
			for range 5 {
				if again := printJSON(t, c.g(t), c.opts...); !bytes.Equal(again, first) {
					t.Fatalf("two runs differ:\n%s\n%s", first, again)
				}
			}
			golden.Assert(t, c.name, first)
		})
	}
}

// --ids keeps its porcelain lines in every format but JSON.
func TestIDsOnlyStaysPorcelainOutsideJSON(t *testing.T) {
	displayer, err := BuildOptions(WithRdfType("instance"), WithFormat("csv"), WithIDsOnly(true)).SetSource(jsonGraph(t)).Build()
	if err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	if err := displayer.Print(&w); err != nil {
		t.Fatal(err)
	}
	if got, want := w.String(), "i-1\ni-2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// encoding/json refuses NaN and the infinities, and one such value used to fail the
// whole listing.
func TestJSONPropertiesWritesNonNumbersAsNull(t *testing.T) {
	res := resourcetest.Instance("i-1").Prop(p.Name, "x").Build()
	res.Properties()["Ratio"] = math.NaN()
	res.Properties()["Ceiling"] = math.Inf(1)
	res.Properties()["Load"] = 0.5

	out, err := json.Marshal(JSONProperties(res))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), `{"Ceiling":null,"ID":"i-1","Load":0.5,"Name":"x","Ratio":null}`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
