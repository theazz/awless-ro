package triplestore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const parentOf = "cloud:parentOf"

// region
//
//	+- vpc
//	     +- subnet-a
//	     |    +- inst-1  (instance)
//	     |    +- inst-2  (instance)
//	     |    +- vol-1   (volume)
//	     +- subnet-b
//	          +- inst-3  (instance)
func treeFixture() RDFGraph {
	src := NewSource()
	src.Add(
		SubjPred("region", parentOf).Resource("vpc"),
		SubjPred("vpc", parentOf).Resource("subnet-a"),
		SubjPred("vpc", parentOf).Resource("subnet-b"),
		SubjPred("subnet-a", parentOf).Resource("inst-1"),
		SubjPred("subnet-a", parentOf).Resource("inst-2"),
		SubjPred("subnet-a", parentOf).Resource("vol-1"),
		SubjPred("subnet-b", parentOf).Resource("inst-3"),

		SubjPred("inst-1", "rdf:type").Resource("Instance"),
		SubjPred("inst-2", "rdf:type").Resource("Instance"),
		SubjPred("inst-3", "rdf:type").Resource("Instance"),
		SubjPred("vol-1", "rdf:type").Resource("Volume"),
	)
	return src.Snapshot()
}

func collect(visited *[]string) VisitFunc {
	return func(_ RDFGraph, node string, depth int) error {
		*visited = append(*visited, fmt.Sprintf("%d:%s", depth, node))
		return nil
	}
}

// Children are visited in sorted order, so that `show` prints the same thing twice
// in a row. Triples themselves have no order.
func TestTraverseDFS(t *testing.T) {
	var visited []string
	if err := NewTree(treeFixture(), parentOf).TraverseDFS("vpc", collect(&visited)); err != nil {
		t.Fatal(err)
	}

	want := "0:vpc 1:subnet-a 2:inst-1 2:inst-2 2:vol-1 1:subnet-b 2:inst-3"
	if got := strings.Join(visited, " "); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestTraverseDFSFromALeaf(t *testing.T) {
	var visited []string
	if err := NewTree(treeFixture(), parentOf).TraverseDFS("inst-1", collect(&visited)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(visited, " "), "0:inst-1"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTraverseAncestors(t *testing.T) {
	var visited []string
	if err := NewTree(treeFixture(), parentOf).TraverseAncestors("inst-1", collect(&visited)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(visited, " "), "0:inst-1 1:subnet-a 2:vpc 3:region"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Siblings are the nodes under the same parent that agree on sameness. awless-ro
// passes the resource type, so an instance's siblings are instances and not the
// subnet's volume.
func TestTraverseSiblings(t *testing.T) {
	sameType := func(g RDFGraph, node string) (string, error) {
		triples := g.WithSubjPred(node, "rdf:type")
		if len(triples) != 1 {
			return "", fmt.Errorf("node %q has %d types", node, len(triples))
		}
		typ, _ := triples[0].Object().Resource()
		return typ, nil
	}

	var visited []string
	if err := NewTree(treeFixture(), parentOf).TraverseSiblings("inst-1", sameType, collect(&visited)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(visited, " "), "0:inst-1 0:inst-2"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// A node with no parent has no siblings but is still visited, or `show --siblings`
// on a top-level resource would print nothing at all.
func TestTraverseSiblingsOfARootNode(t *testing.T) {
	sameType := func(RDFGraph, string) (string, error) { return "", nil }

	var visited []string
	if err := NewTree(treeFixture(), parentOf).TraverseSiblings("region", sameType, collect(&visited)); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(visited, " "), "0:region"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// An error from the visitor stops the traversal and reaches the caller. Upstream
// discarded the error from the recursive call in TraverseDFS, so a failure below the
// first level was reported as success.
func TestTraversalReportsVisitorErrors(t *testing.T) {
	boom := errors.New("boom")

	failOn := func(target string) VisitFunc {
		return func(_ RDFGraph, node string, _ int) error {
			if node == target {
				return boom
			}
			return nil
		}
	}

	tree := NewTree(treeFixture(), parentOf)

	t.Run("deep in DFS", func(t *testing.T) {
		if err := tree.TraverseDFS("region", failOn("inst-2")); !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
	t.Run("deep in ancestors", func(t *testing.T) {
		if err := tree.TraverseAncestors("inst-1", failOn("region")); !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
	t.Run("at the root", func(t *testing.T) {
		if err := tree.TraverseDFS("region", failOn("region")); !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
}

// The edge predicate must point at resources. A literal there means the graph is
// malformed, and saying so beats traversing into an empty node name.
func TestTraverseRejectsNonResourceChildren(t *testing.T) {
	src := NewSource()
	src.Add(SubjPred("parent", parentOf).StringLiteral("not-a-resource"))

	err := NewTree(src.Snapshot(), parentOf).TraverseDFS("parent", func(RDFGraph, string, int) error { return nil })
	if err == nil {
		t.Fatal("expected an error for a literal child")
	}
	if !strings.Contains(err.Error(), "not a resource") {
		t.Errorf("unhelpful error: %q", err)
	}
}

// A node with two parents is not a tree. Silently picking one would make `show`
// depend on map order.
func TestSiblingsRejectMultipleParents(t *testing.T) {
	src := NewSource()
	src.Add(
		SubjPred("p1", parentOf).Resource("child"),
		SubjPred("p2", parentOf).Resource("child"),
	)
	sameType := func(RDFGraph, string) (string, error) { return "", nil }

	err := NewTree(src.Snapshot(), parentOf).TraverseSiblings("child", sameType, func(RDFGraph, string, int) error { return nil })
	if err == nil {
		t.Fatal("expected an error for a node with two parents")
	}
	if !strings.Contains(err.Error(), "parents") {
		t.Errorf("unhelpful error: %q", err)
	}
}

func TestNewTreeRejectsANilGraph(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewTree(nil) should panic rather than fail later")
		}
	}()
	NewTree(nil, parentOf)
}
