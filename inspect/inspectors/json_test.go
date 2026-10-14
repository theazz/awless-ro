package inspectors

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
	"github.com/theazz/awless-ro/internal/golden"
)

func rowsJSON(t *testing.T, rows interface{}) []byte {
	t.Helper()
	var w bytes.Buffer
	enc := json.NewEncoder(&w)
	enc.SetIndent("", " ")
	if err := enc.Encode(rows); err != nil {
		t.Fatal(err)
	}
	return w.Bytes()
}

func TestBucketSizerRowsGolden(t *testing.T) {
	g := graph.NewGraph()
	g.AddResource(
		object("a", "zebra", 1_500_000_000),
		object("b", "alpha", 2),
		object("c", "alpha", 3),
		resourcetest.S3Object("orphan").Prop(properties.Size, 42).Build(),
	)
	sizer := &BucketSizer{}
	if err := sizer.Inspect(g); err != nil {
		t.Fatal(err)
	}
	var notes bytes.Buffer
	golden.Assert(t, "bucket_sizer", rowsJSON(t, sizer.Rows(&notes)))
	if !strings.Contains(notes.String(), "named no bucket") {
		t.Errorf("the object naming no bucket is not reported on the side:\n%s", notes.String())
	}
}

// Nothing fetched is an empty array, with the reason on the side rather than in the
// data, where it would be read as a bucket.
func TestBucketSizerRowsWhenNothingWasFetched(t *testing.T) {
	sizer := &BucketSizer{}
	if err := sizer.Inspect(graph.NewGraph()); err != nil {
		t.Fatal(err)
	}
	var notes bytes.Buffer
	if got := string(rowsJSON(t, sizer.Rows(&notes))); got != "[]\n" {
		t.Errorf("got %s, want []", got)
	}
	if !strings.Contains(notes.String(), "aws.storage.s3object.sync") {
		t.Errorf("the setting that turns object sync on is not mentioned:\n%s", notes.String())
	}
}

func TestOpenBucketsRowsGolden(t *testing.T) {
	grant := func(uri string) *graph.Grant {
		return &graph.Grant{Permission: "READ", Grantee: graph.Grantee{GranteeID: uri, GranteeType: "Group"}}
	}
	g := graph.NewGraph()
	g.AddResource(
		resourcetest.Bucket("public-b").Prop(properties.Grants, []*graph.Grant{grant("http://acs.amazonaws.com/groups/global/AllUsers")}).Build(),
		resourcetest.Bucket("public-a").Prop(properties.Grants, []*graph.Grant{
			grant("http://acs.amazonaws.com/groups/global/AllUsers"),
			grant("http://acs.amazonaws.com/groups/global/AuthenticatedUsers"),
		}).Build(),
		resourcetest.Bucket("private").Build(),
	)
	open := &OpenBuckets{}
	if err := open.Inspect(g); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, "open_buckets", rowsJSON(t, open.Rows(nil)))

	none := &OpenBuckets{}
	if err := none.Inspect(graph.NewGraph()); err != nil {
		t.Fatal(err)
	}
	if got := string(rowsJSON(t, none.Rows(nil))); got != "[]\n" {
		t.Errorf("got %s, want []", got)
	}
}

func TestPortScannerRowsGolden(t *testing.T) {
	_, all, _ := net.ParseCIDR("0.0.0.0/0")
	g := graph.NewGraph()
	g.AddResource(
		resourcetest.SecurityGroup("sg-2").Prop(properties.InboundRules, []*graph.FirewallRule{
			{PortRange: graph.PortRange{FromPort: 22, ToPort: 22}, Protocol: "tcp", IPRanges: []*net.IPNet{all}},
		}).Build(),
		resourcetest.SecurityGroup("sg-1").Prop(properties.InboundRules, []*graph.FirewallRule{
			{PortRange: graph.PortRange{Any: true}, Protocol: "any", IPRanges: []*net.IPNet{all}},
		}).Build(),
		resourcetest.Instance("i-1").Prop(properties.Name, "web").Build(),
	)
	sg, _ := g.GetResource("securitygroup", "sg-1")
	inst, _ := g.GetResource("instance", "i-1")
	if err := g.AddAppliesOnRelation(sg, inst); err != nil {
		t.Fatal(err)
	}
	scanner := &PortScanner{}
	if err := scanner.Inspect(g); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, "port_scanner", rowsJSON(t, scanner.Rows(nil)))
}
