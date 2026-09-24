package graph

import (
	"testing"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/rdf"
)

// A node named by a relation but never fetched as a resource of its own has no type
// triple. That is ordinary rather than exceptional: relations point across services,
// and a service may be switched off, refused for want of permissions, or simply not
// synced yet.
//
// The traversals used to stop on it, so a security group applying on something outside
// the local graph was enough to end `inspect -i port_scanner` with the internal
// message "resource type not found" — and would have ended `show` on that resource
// too. ListResourcesAppliedOn already answered this with NotFoundResource, so the same
// graph was fine through one door and fatal through another.
func TestTraversalDeliversUntypedNodesInsteadOfFailing(t *testing.T) {
	g := NewGraph()

	group := InitResource("securitygroup", "sg-1")
	eni := InitResource("networkinterface", "eni-1")
	g.AddResource(group, eni)

	// The relation is recorded without the target ever being added as a resource,
	// which is exactly what a cross-service reference looks like locally.
	g.AddAppliesOnRelation(group, eni)
	g.AddAppliesOnRelation(eni, InitResource("instance", "i-unsynced"))

	relations, err := g.ResourceRelations(group, rdf.ApplyOn, true)
	if err != nil {
		t.Fatalf("traversal failed on an untyped node: %s", err)
	}

	byID := make(map[string]string)
	for _, r := range relations {
		byID[r.Id()] = r.Type()
	}

	if got := byID["eni-1"]; got != "networkinterface" {
		t.Errorf("eni-1 came back as %q, want networkinterface", got)
	}
	if got, ok := byID["i-unsynced"]; !ok {
		t.Error("the untyped node was dropped from the traversal entirely")
	} else if got != notFoundResourceType {
		t.Errorf("i-unsynced came back as %q, want %q", got, notFoundResourceType)
	}
}

// The two ways of asking the same question have to agree, or which one a command
// happens to use decides whether it works.
func TestTraversalAndTheDirectListingAgreeOnUntypedNodes(t *testing.T) {
	g := NewGraph()

	group := InitResource("securitygroup", "sg-1")
	g.AddResource(group)
	g.AddAppliesOnRelation(group, InitResource("instance", "i-unsynced"))

	direct, err := g.ListResourcesAppliedOn(group)
	if err != nil {
		t.Fatalf("ListResourcesAppliedOn: %s", err)
	}
	traversed, err := g.ResourceRelations(group, rdf.ApplyOn, false)
	if err != nil {
		t.Fatalf("ResourceRelations: %s", err)
	}

	if len(direct) != 1 || len(traversed) != 1 {
		t.Fatalf("direct returned %d, traversal returned %d, want 1 each", len(direct), len(traversed))
	}
	if direct[0].Id() != traversed[0].Id() || direct[0].Type() != traversed[0].Type() {
		t.Errorf("direct gave %s/%s, traversal gave %s/%s",
			direct[0].Type(), direct[0].Id(), traversed[0].Type(), traversed[0].Id())
	}
}

// Walking children, which is what `show` prints as the lineage, must survive the same
// thing.
func TestVisitingChildrenSurvivesAnUntypedNode(t *testing.T) {
	g := NewGraph()

	vpc := InitResource("vpc", "vpc-1")
	g.AddResource(vpc)
	g.AddParentRelation(vpc, InitResource("subnet", "subnet-unsynced"))

	var seen []string
	err := g.VisitRelations(vpc, rdf.ChildrenOfRel, false, func(r cloud.Resource, _ int) error {
		seen = append(seen, r.Id())
		return nil
	})
	if err != nil {
		t.Fatalf("visiting children failed on an untyped node: %s", err)
	}
	if len(seen) != 1 || seen[0] != "subnet-unsynced" {
		t.Errorf("saw %v, want [subnet-unsynced]", seen)
	}
}
