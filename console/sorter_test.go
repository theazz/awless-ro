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

package console

import (
	"bytes"
	"testing"

	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// Regression tests for upstream wallix/awless#248: `list subnets --sort public`
// panicked with "can not compare values of type bool" because valueLowerOrEqual
// (displayer.go) had no case for bool. See console/upstream_issue_248_test.go on
// branch upstream-bug-validation for the original reproduction.
func TestSortByBooleanColumn(t *testing.T) {
	columns := []ColumnDefinition{
		StringColumnDefinition{Prop: p.ID},
		StringColumnDefinition{Prop: p.Public},
	}

	run := func(t *testing.T, g *graph.Graph, sortBy string, reverse bool) string {
		displayer, err := BuildOptions(
			WithRdfType("subnet"),
			WithColumnDefinitions(columns),
			WithFormat("csv"),
			WithSortBy(sortBy),
			WithReverseSort(reverse),
		).SetSource(g).Build()
		if err != nil {
			t.Fatal(err)
		}
		var w bytes.Buffer
		if err := displayer.Print(&w); err != nil {
			t.Fatal(err)
		}
		return w.String()
	}

	twoDifferentBools := func() *graph.Graph {
		g := graph.NewGraph()
		if err := g.AddResource(
			resourcetest.Subnet("sub-true").Prop(p.Public, true).Build(),
			resourcetest.Subnet("sub-false").Prop(p.Public, false).Build(),
		); err != nil {
			t.Fatal(err)
		}
		return g
	}

	t.Run("two different bools ascending, false before true", func(t *testing.T) {
		got := run(t, twoDifferentBools(), "public", false)
		want := "ID,Public\nsub-false,false\nsub-true,true\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("two different bools, --reverse, true before false", func(t *testing.T) {
		got := run(t, twoDifferentBools(), "public", true)
		want := "ID,Public\nsub-true,true\nsub-false,false\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("upper case sort key PUBLIC does not panic", func(t *testing.T) {
		got := run(t, twoDifferentBools(), "PUBLIC", false)
		want := "ID,Public\nsub-false,false\nsub-true,true\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("one nil one bool does not panic", func(t *testing.T) {
		g := graph.NewGraph()
		if err := g.AddResource(
			resourcetest.Subnet("sub-true").Prop(p.Public, true).Build(),
			resourcetest.Subnet("sub-nil").Build(),
		); err != nil {
			t.Fatal(err)
		}
		// Only assert it does not panic; nil-vs-value ordering is not the
		// contract under test here.
		run(t, g, "public", false)
	})

	t.Run("both nil does not panic", func(t *testing.T) {
		g := graph.NewGraph()
		if err := g.AddResource(
			resourcetest.Subnet("sub-1").Build(),
			resourcetest.Subnet("sub-2").Build(),
		); err != nil {
			t.Fatal(err)
		}
		run(t, g, "public", false)
	})

	t.Run("equal bools does not panic", func(t *testing.T) {
		g := graph.NewGraph()
		if err := g.AddResource(
			resourcetest.Subnet("sub-1").Prop(p.Public, true).Build(),
			resourcetest.Subnet("sub-2").Prop(p.Public, true).Build(),
		); err != nil {
			t.Fatal(err)
		}
		run(t, g, "public", false)
	})
}

// The same boolean column exists, under the same xsd:boolean datatype, on several
// other resource types: Subnet/Image/Database/LaunchConfiguration Public, Vpc
// Default, Volume Encrypted. Each must sort without panicking too.
func TestSortByBooleanColumnOtherResourceTypes(t *testing.T) {
	cases := []struct {
		rdfType string
		prop    string
		build   func(id string, v bool) *graph.Resource
	}{
		{"subnet", p.Public, func(id string, v bool) *graph.Resource { return resourcetest.Subnet(id).Prop(p.Public, v).Build() }},
		{"image", p.Public, func(id string, v bool) *graph.Resource { return resourcetest.Image(id).Prop(p.Public, v).Build() }},
		{"database", p.Public, func(id string, v bool) *graph.Resource { return resourcetest.Database(id).Prop(p.Public, v).Build() }},
		{"launchconfiguration", p.Public, func(id string, v bool) *graph.Resource {
			return resourcetest.LaunchConfig(id).Prop(p.Public, v).Build()
		}},
		{"vpc", p.Default, func(id string, v bool) *graph.Resource { return resourcetest.VPC(id).Prop(p.Default, v).Build() }},
		{"volume", p.Encrypted, func(id string, v bool) *graph.Resource { return resourcetest.Volume(id).Prop(p.Encrypted, v).Build() }},
	}

	for _, c := range cases {
		t.Run(c.rdfType, func(t *testing.T) {
			g := graph.NewGraph()
			if err := g.AddResource(c.build("r-true", true), c.build("r-false", false)); err != nil {
				t.Fatal(err)
			}
			displayer, err := BuildOptions(
				WithRdfType(c.rdfType),
				WithColumnDefinitions([]ColumnDefinition{
					StringColumnDefinition{Prop: p.ID},
					StringColumnDefinition{Prop: c.prop},
				}),
				WithFormat("csv"),
				WithSortBy(c.prop),
			).SetSource(g).Build()
			if err != nil {
				t.Fatal(err)
			}
			var w bytes.Buffer
			if err := displayer.Print(&w); err != nil {
				t.Fatalf("Print panicked or errored for %s.%s: %v", c.rdfType, c.prop, err)
			}
		})
	}
}

// An unknown/unsortable value type must not panic either: it falls back to a
// stable, string-based order instead of crashing the process.
func TestValueLowerOrEqualUnsortableTypeDoesNotPanic(t *testing.T) {
	type unsortable struct{ n int }

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("valueLowerOrEqual panicked on an unsortable type: %v", r)
		}
	}()

	a := unsortable{n: 1}
	b := unsortable{n: 2}

	// Only the fallback must apply: no particular ordering is promised, just
	// a deterministic one and no panic.
	first := valueLowerOrEqual(a, b)
	second := valueLowerOrEqual(a, b)
	if first != second {
		t.Fatalf("fallback ordering is not stable: got %v then %v", first, second)
	}

	// Mismatched types on the same column must not panic either.
	valueLowerOrEqual(a, "a string")
	valueLowerOrEqual(1, "a string")
}
