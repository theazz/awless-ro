package graph_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// The exported half of the fix for a subject carrying two rdf:type triples
// (wallix/awless#224). Reaching it through FindWithProperties is what `show` does:
// resolveResourceFromRef asks for the id, then the ARN, then the name.

func typesOf(t *testing.T, resources []cloud.Resource) []string {
	t.Helper()
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.Type())
	}
	sort.Strings(out)
	return out
}

// The collision a real sync produces: both ids are a bare AWS name, and a name is
// only unique per type.
func TestFindWithPropertiesOnACollidingName(t *testing.T) {
	g := graph.NewGraph()
	err := g.AddResource(
		resourcetest.KeyPair("prod").Prop(properties.Name, "prod").Build(),
		resourcetest.ClassicLoadBalancer("prod").Prop(properties.Name, "prod").Build(),
		resourcetest.Instance("i-1").Build(),
	)
	if err != nil {
		t.Fatal(err)
	}

	resources, err := g.FindWithProperties(map[string]interface{}{properties.ID: "prod"})
	if err != nil {
		t.Fatalf("FindWithProperties returned an error: %s", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if got, want := typesOf(t, resources), []string{"classicloadbalancer", "keypair"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// FindWithProperties runs its resolvers through And, which re-adds the
	// intermediate result to a fresh graph and resolves again. The merged subject
	// has to survive that round-trip, or the second pass would see the same
	// ambiguity and answer differently.
	gg := graph.NewGraph()
	for _, r := range resources {
		res, ok := r.(*graph.Resource)
		if !ok {
			t.Fatalf("expected a *graph.Resource, got %T", r)
		}
		if err := gg.AddResource(res); err != nil {
			t.Fatal(err)
		}
	}
	again, err := gg.FindWithProperties(map[string]interface{}{properties.ID: "prod"})
	if err != nil {
		t.Fatalf("second pass returned an error: %s", err)
	}
	if len(again) != 2 {
		t.Fatalf("second pass gave %d resources, want 2", len(again))
	}
	for i := range again {
		if !reflect.DeepEqual(again[i].Properties(), resources[i].Properties()) {
			t.Errorf("round-trip changed %s: %v, was %v", again[i].Id(), again[i].Properties(), resources[i].Properties())
		}
	}
}

// The shape the upstream report was filed against: one Lambda ARN typed both as a
// function and as a policy, arriving through the file merge rather than from one
// AddResource call — the route LoadLocalGraphs takes.
func TestFindWithPropertiesOnAMergedArnFromFiles(t *testing.T) {
	const arn = "arn:aws:lambda:eu-west-1:123456789012:function:deploy-notifier"

	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	lambda := write("lambda.nt",
		"<"+arn+"> <cloud:id> \""+arn+"\" .\n"+
			"<"+arn+"> <cloud:name> \"deploy-notifier\" .\n"+
			"<"+arn+"> <rdf:type> <cloud-owl:Function> .\n")
	access := write("access.nt",
		"<"+arn+"> <cloud:id> \""+arn+"\" .\n"+
			"<"+arn+"> <rdf:type> <cloud-owl:Policy> .\n")

	g, err := graph.NewGraphFromFiles(lambda, access)
	if err != nil {
		t.Fatalf("graph load: %s", err)
	}

	resources, err := g.FindWithProperties(map[string]interface{}{properties.ID: arn})
	if err != nil {
		t.Fatalf("FindWithProperties returned an error: %s", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	if got, want := typesOf(t, resources), []string{"function", "policy"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Both carry the union of the subject's properties; the per-type column
	// definitions project that back down for display.
	for _, r := range resources {
		if got := r.Properties()[properties.Name]; got != "deploy-notifier" {
			t.Errorf("%s has Name %v, want deploy-notifier", r.Type(), got)
		}
	}
}

// FindResource is deliberately left as an error for an ambiguous id: it returns a
// single resource and has no honest answer, and returning the first by sort would
// hide the ambiguity from a caller that asked for exactly one thing. It has no
// production caller — `show` goes through FindWithProperties — so this pins the
// decision rather than a behaviour anyone depends on. The fixture is built with
// NewGraph + AddResource because NewGraphFromFiles returns a cloud.GraphAPI while
// FindResource is a *Graph method.
func TestFindResourceStillRefusesAnAmbiguousID(t *testing.T) {
	g := graph.NewGraph()
	err := g.AddResource(
		resourcetest.KeyPair("prod").Build(),
		resourcetest.ClassicLoadBalancer("prod").Build(),
	)
	if err != nil {
		t.Fatal(err)
	}

	res, err := g.FindResource("prod")
	if err == nil {
		t.Fatalf("expected an error, got %v", res)
	}
	if got, want := err.Error(), "multiple resources with id 'prod' found"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSingleTypedSubjectIsUnaffected(t *testing.T) {
	g := graph.NewGraph()
	err := g.AddResource(
		resourcetest.Instance("i-1").Prop(properties.Name, "web-01").Prop(properties.State, "running").Build(),
	)
	if err != nil {
		t.Fatal(err)
	}

	resources, err := g.FindWithProperties(map[string]interface{}{properties.ID: "i-1"})
	if err != nil {
		t.Fatalf("FindWithProperties: %s", err)
	}
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	want := map[string]interface{}{
		properties.ID:    "i-1",
		properties.Name:  "web-01",
		properties.State: "running",
	}
	if got := resources[0].Properties(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A node named only by a relation has no type triple, and the traversals already
// answered that with NotFoundResource. Making type resolution total must not have
// turned that into something else.
func TestTypelessSubjectStillComesBackNotFound(t *testing.T) {
	g := graph.NewGraph()
	group := resourcetest.SecurityGroup("sg-1").Build()
	if err := g.AddResource(group); err != nil {
		t.Fatal(err)
	}
	g.AddAppliesOnRelation(group, graph.InitResource("instance", "i-unsynced"))

	applied, err := g.ListResourcesAppliedOn(group)
	if err != nil {
		t.Fatalf("ListResourcesAppliedOn: %s", err)
	}
	if len(applied) != 1 {
		t.Fatalf("got %d resources, want 1", len(applied))
	}
	if got := applied[0].Type(); got != "notfound" {
		t.Errorf("got type %q, want notfound", got)
	}
}
