/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package triplestore

import (
	"fmt"
	"sort"
)

// Tree reads a graph as a tree, taking one predicate as the parent-to-child edge.
// awless-ro uses it for the resource hierarchy: region contains vpc contains subnet
// contains instance.
//
// Children are visited in sorted order so that `show` prints the same thing twice in
// a row; triples themselves have no order.
type Tree struct {
	g         RDFGraph
	predicate string
}

// VisitFunc is called once per node, with the graph, the node and its depth relative
// to where the traversal started. Returning an error stops the traversal.
type VisitFunc func(RDFGraph, string, int) error

func NewTree(g RDFGraph, predicate string) *Tree {
	if g == nil {
		panic("triples: NewTree called with a nil graph")
	}
	return &Tree{g: g, predicate: predicate}
}

// TraverseDFS walks the node and its descendants, parents before children.
func (t *Tree) TraverseDFS(node string, visit VisitFunc, depths ...int) error {
	depth := firstOrZero(depths)

	if err := visit(t.g, node, depth); err != nil {
		return err
	}

	children, err := t.childrenOf(node)
	if err != nil {
		return err
	}
	for _, child := range children {
		// Upstream discarded this error, so a failure deep in the tree was
		// reported as success.
		if err := t.TraverseDFS(child, visit, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// TraverseAncestors walks the node and then upwards to the root.
func (t *Tree) TraverseAncestors(node string, visit VisitFunc, depths ...int) error {
	depth := firstOrZero(depths)

	if err := visit(t.g, node, depth); err != nil {
		return err
	}

	var parents []string
	for _, tri := range t.g.WithPredObj(t.predicate, Resource(node)) {
		parents = append(parents, tri.Subject())
	}
	sort.Strings(parents)

	for _, parent := range parents {
		if err := t.TraverseAncestors(parent, visit, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// TraverseSiblings visits the nodes sharing a parent with the given node and
// agreeing with it on sameness, as decided by sameAs. awless-ro uses the resource
// type, so that `show --siblings` on an instance lists instances and not the subnet's
// other children of every kind.
//
// A node with no parent is its own only sibling.
func (t *Tree) TraverseSiblings(node string, sameAs func(RDFGraph, string) (string, error), visit VisitFunc) error {
	parentTriples := t.g.WithPredObj(t.predicate, Resource(node))

	if len(parentTriples) == 0 {
		return visit(t.g, node, 0)
	}
	if len(parentTriples) > 1 {
		return fmt.Errorf("triples: tree on %q: node %q has %d parents, expected one", t.predicate, node, len(parentTriples))
	}

	siblings, err := t.childrenOf(parentTriples[0].Subject())
	if err != nil {
		return err
	}

	want, err := sameAs(t.g, node)
	if err != nil {
		return err
	}

	for _, sibling := range siblings {
		got, err := sameAs(t.g, sibling)
		if err != nil {
			return err
		}
		if got != want {
			continue
		}
		if err := visit(t.g, sibling, 0); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tree) childrenOf(node string) ([]string, error) {
	triples := t.g.WithSubjPred(node, t.predicate)
	children := make([]string, 0, len(triples))
	for _, tri := range triples {
		child, ok := tri.Object().Resource()
		if !ok {
			return nil, fmt.Errorf("triples: tree on %q: child of %q is not a resource identifier", t.predicate, node)
		}
		children = append(children, child)
	}
	sort.Strings(children)
	return children, nil
}

func firstOrZero(vals []int) int {
	if len(vals) > 0 {
		return vals[0]
	}
	return 0
}
