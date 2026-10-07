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
	"math"
	"reflect"
	"testing"
	"time"

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

// sortLess is the "less" defaultSorter.sort hands to sort.Slice for a single
// column: DeepEqual pairs are skipped, every other pair goes to
// valueLowerOrEqual.
func sortLess(a, b interface{}) bool {
	return !reflect.DeepEqual(a, b) && valueLowerOrEqual(a, b)
}

// Distinct types that share a type key: local types of the same package and
// name. The int and string ones print the same value ("1") but differ in
// Go-syntax form; the second int one is identical to the first on every key.
func sameKeyValues() (localInt, localString, localIntTwin interface{}) {
	{
		type collide int
		localInt = collide(1)
	}
	{
		type collide string
		localString = collide("1")
	}
	{
		type collide int
		localIntTwin = collide(1)
	}
	return
}

// Values that print identically but are not equal. A fallback ordering on the
// printed form alone with '<=' reported "lower or equal" in both directions for
// each pair, which left ascending and --reverse order undefined. Each pair must
// now have one and only one order, the same whichever way round the rows arrive.
func TestSortFallbackOnPrintedKeyCollision(t *testing.T) {
	localInt, localString, _ := sameKeyValues()
	if typeOrderKey(localInt) != typeOrderKey(localString) {
		t.Fatalf("fixture no longer exercises a type key tie: %q vs %q", typeOrderKey(localInt), typeOrderKey(localString))
	}

	cases := []struct {
		name string
		a, b interface{}
	}{
		{"int and string printing as 1", 1, "1"},
		{"distinct types sharing a type key", localInt, localString},
		{"nil and empty []string", []string(nil), []string{}},
		{"[]string{\"a b\"} and []string{\"a\", \"b\"}", []string{"a b"}, []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testCollidingPairSorts(t, c.a, c.b)
		})
	}
}

func testCollidingPairSorts(t *testing.T, a, b interface{}) {
	t.Helper()
	rowA := []interface{}{"row-a", a}
	rowB := []interface{}{"row-b", b}

	// At comparator level: exactly one direction may be lower.
	lower, higher := sortLess(a, b), sortLess(b, a)
	if lower && higher {
		t.Fatalf("comparator is not antisymmetric: %#v and %#v are both lower than each other", a, b)
	}
	if !lower && !higher {
		t.Fatalf("comparator is not total: %#v and %#v are neither lower than the other", a, b)
	}

	sortIDs := func(descending bool, rows ...[]interface{}) []string {
		lines := make(table, 0, len(rows))
		for _, r := range rows {
			lines = append(lines, append([]interface{}{}, r...))
		}
		(&defaultSorter{sortBy: []int{1}, descending: descending}).sort(lines)
		ids := make([]string, 0, len(lines))
		for _, l := range lines {
			ids = append(ids, l[0].(string))
		}
		return ids
	}

	ascending := sortIDs(false, rowA, rowB)
	reversed := sortIDs(true, rowA, rowB)

	if reflect.DeepEqual(ascending, reversed) {
		t.Fatalf("--reverse did not flip the colliding pair: ascending %v, reverse %v", ascending, reversed)
	}
	if len(ascending) != 2 || len(reversed) != 2 ||
		ascending[0] != reversed[1] || ascending[1] != reversed[0] {
		t.Fatalf("ascending and reverse are not mirror images: ascending %v, reverse %v", ascending, reversed)
	}

	// Neither direction may depend on the order the rows arrive in, nor on the
	// run: repeat both with the input swapped.
	for i := 0; i < 20; i++ {
		if got := sortIDs(false, rowA, rowB); !reflect.DeepEqual(got, ascending) {
			t.Fatalf("ascending order is not stable: got %v, want %v", got, ascending)
		}
		if got := sortIDs(false, rowB, rowA); !reflect.DeepEqual(got, ascending) {
			t.Fatalf("ascending order depends on input order: got %v, want %v", got, ascending)
		}
		if got := sortIDs(true, rowA, rowB); !reflect.DeepEqual(got, reversed) {
			t.Fatalf("reverse order is not stable: got %v, want %v", got, reversed)
		}
		if got := sortIDs(true, rowB, rowA); !reflect.DeepEqual(got, reversed) {
			t.Fatalf("reverse order depends on input order: got %v, want %v", got, reversed)
		}
	}
}

// The sort "less" must be a strict weak ordering over every kind of value a
// column can hold, mixed together, or sort.Slice's result is undefined:
// irreflexive, asymmetric, transitive, and with "neither lower" transitive too.
// Checked for both directions, since --reverse swaps the operands.
func TestSortLessIsStrictWeakOrdering(t *testing.T) {
	type unsortable struct{ n int }
	localInt, localString, localIntTwin := sameKeyValues()
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	values := []interface{}{
		nil,
		-3, 1, 2, 10,
		1.5, 2.0, math.NaN(), math.NaN(), math.Inf(-1),
		"", "1", "10", "2", "a",
		false, true,
		t1, t2,
		[]string(nil), []string{}, []string{"a b"}, []string{"a", "b"},
		[]int(nil), []int{1}, []int{1, 2}, []int{12},
		unsortable{1}, unsortable{2},
		localInt, localString, localIntTwin,
	}

	directions := map[string]func(a, b interface{}) bool{
		"ascending":  sortLess,
		"descending": func(a, b interface{}) bool { return sortLess(b, a) },
	}
	for name, less := range directions {
		t.Run(name, func(t *testing.T) {
			equiv := func(a, b interface{}) bool { return !less(a, b) && !less(b, a) }
			for _, a := range values {
				if less(a, a) {
					t.Errorf("not irreflexive: %#v < itself", a)
				}
				for _, b := range values {
					if less(a, b) && less(b, a) {
						t.Errorf("not asymmetric: %#v and %#v are both lower than each other", a, b)
					}
					for _, c := range values {
						if less(a, b) && less(b, c) && !less(a, c) {
							t.Errorf("not transitive: %#v < %#v < %#v but not %#v < %#v", a, b, c, a, c)
						}
						if equiv(a, b) && equiv(b, c) && !equiv(a, c) {
							t.Errorf("incomparability not transitive: %#v ~ %#v ~ %#v but not %#v ~ %#v", a, b, c, a, c)
						}
					}
				}
			}
		})
	}

	// Values identical on type key, printed form and Go-syntax form are
	// equivalent: neither may be lower.
	if sortLess(localInt, localIntTwin) || sortLess(localIntTwin, localInt) {
		t.Fatalf("indistinguishable values %#v and %#v must compare as equivalent", localInt, localIntTwin)
	}
}
