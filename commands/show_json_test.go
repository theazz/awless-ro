package commands

import (
	"bytes"
	"testing"
	"time"

	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
	"github.com/theazz/awless-ro/internal/golden"
)

// showGraph is a region holding a VPC with two subnets, three instances in one of
// them, and a security group applying on one instance.
func showGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g := graph.NewGraph()
	if err := g.AddResource(
		resourcetest.Region("eu-west-1").Build(),
		resourcetest.VPC("vpc-1").Prop(p.Name, "main").Build(),
		resourcetest.Subnet("subnet-2").Prop(p.Vpc, "vpc-1").Build(),
		resourcetest.Subnet("subnet-1").Prop(p.Name, "private").Prop(p.Vpc, "vpc-1").Build(),
		resourcetest.Instance("i-1").Prop(p.Name, "web").Prop(p.State, "running").
			Prop(p.Launched, time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)).Build(),
		resourcetest.Instance("i-3").Prop(p.Name, "worker").Build(),
		resourcetest.Instance("i-2").Prop(p.Name, "db").Build(),
		resourcetest.SecurityGroup("sg-1").Prop(p.Name, "web").Build(),
	); err != nil {
		t.Fatal(err)
	}
	resourcetest.AddParents(g,
		"eu-west-1 -> vpc-1",
		"vpc-1 -> subnet-2", "vpc-1 -> subnet-1",
		"subnet-1 -> i-3", "subnet-1 -> i-1", "subnet-1 -> i-2",
		"vpc-1 -> sg-1",
	)
	sg, _ := g.GetResource("securitygroup", "sg-1")
	inst, _ := g.GetResource("instance", "i-1")
	if err := g.AddAppliesOnRelation(sg, inst); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestShowJSONGolden(t *testing.T) {
	cases := []struct {
		name, typ, id string
		siblings      bool
	}{
		{"show-instance", "instance", "i-1", false},
		{"show-instance-siblings", "instance", "i-1", true},
		{"show-vpc", "vpc", "vpc-1", false},
		{"show-securitygroup", "securitygroup", "sg-1", false},
		{"show-region", "region", "eu-west-1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var first []byte
			for i := range 5 {
				g := showGraph(t)
				res, err := g.GetResource(c.typ, c.id)
				if err != nil {
					t.Fatal(err)
				}
				out, err := buildShowOutput(res, g, c.siblings)
				if err != nil {
					t.Fatal(err)
				}
				var w bytes.Buffer
				if err := writeJSON(&w, out); err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = w.Bytes()
				} else if !bytes.Equal(w.Bytes(), first) {
					t.Fatalf("two runs differ:\n%s\n%s", first, w.Bytes())
				}
			}
			golden.Assert(t, c.name, first)
		})
	}
}

// A resource reads the same from show as from list: show's "properties" is exactly
// what list --format json prints for it.
func TestShowPropertiesAreTheListObject(t *testing.T) {
	g := showGraph(t)
	res, _ := g.GetResource("instance", "i-1")
	out, err := buildShowOutput(res, g, false)
	if err != nil {
		t.Fatal(err)
	}
	var fromShow, fromList bytes.Buffer
	if err := writeJSON(&fromShow, out.Properties); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(&fromList, res.Properties()); err != nil {
		t.Fatal(err)
	}
	if fromShow.String() != fromList.String() {
		t.Fatalf("show:\n%s\nlist:\n%s", fromShow.String(), fromList.String())
	}
}
