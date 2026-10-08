package graph

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/triplestore"
)

// A subject carrying two rdf:type triples used to be fatal: resolveResourceType
// returned "cannot resolve unique type for resource", which ByProperty handed back
// for the whole query, so `show prod` died while `list keypairs` kept working
// (ByType knows the type up front and never asks). It is not a corrupt graph —
// keypair, classic ELB, database, bucket and s3object all use a bare AWS name as
// their id, and a name is only unique per type, so a keypair and a classic ELB both
// called 'prod' land in one graph once LoadLocalGraphs merges the service files.
//
// resourcetest is unavailable here: it imports graph, so a `package graph` test
// file that used it would be an import cycle. Fixtures are built the way
// graph/graph_test.go does, with InitResource + AddResource.

var (
	keypairCreated = time.Date(2024, 3, 2, 10, 0, 0, 0, time.UTC)
	elbCreated     = time.Date(2023, 6, 1, 10, 0, 0, 0, time.UTC)
)

// multitypedFixture puts a keypair and a classic load balancer on the same subject,
// each setting Name, Created and State to a different value, which is what makes the
// property union observable.
func multitypedFixture(t *testing.T) *Graph {
	t.Helper()

	g := NewGraph()
	keypair := InitResource("keypair", "prod")
	keypair.SetProperty(properties.Name, "zzz")
	keypair.SetProperty(properties.Created, keypairCreated)
	keypair.SetProperty(properties.State, "available")

	elb := InitResource("classicloadbalancer", "prod")
	elb.SetProperty(properties.Name, "aaa")
	elb.SetProperty(properties.Created, elbCreated)
	elb.SetProperty(properties.State, "active")

	if err := g.AddResource(keypair, elb); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestMultitypedSubjectResolvesEveryType(t *testing.T) {
	snap := multitypedFixture(t).store.Snapshot()

	types, err := resolveResourceTypes(snap, "prod")
	if err != nil {
		t.Fatalf("resolveResourceTypes: %s", err)
	}
	want := []string{"classicloadbalancer", "keypair"}
	if !reflect.DeepEqual(types, want) {
		t.Errorf("got %v, want %v", types, want)
	}

	// The singular resolver keeps its signature — graph/visit.go passes it as a
	// value — and answers with the first of them.
	typ, err := resolveResourceType(snap, "prod")
	if err != nil {
		t.Fatalf("resolveResourceType: %s", err)
	}
	if typ != "classicloadbalancer" {
		t.Errorf("got %q, want %q", typ, "classicloadbalancer")
	}
}

func TestMultitypedSubjectKeepsTheUnchangedContracts(t *testing.T) {
	g := NewGraph()
	instance := InitResource("instance", "i-1")
	instance.SetProperty(properties.Name, "web-01")
	if err := g.AddResource(instance); err != nil {
		t.Fatal(err)
	}
	// A node named only by a relation has no type triple at all.
	g.AddAppliesOnRelation(instance, InitResource("volume", "vol-unsynced"))
	snap := g.store.Snapshot()

	types, err := resolveResourceTypes(snap, "i-1")
	if err != nil {
		t.Fatalf("single-typed subject: %s", err)
	}
	if !reflect.DeepEqual(types, []string{"instance"}) {
		t.Errorf("single-typed subject gave %v, want [instance]", types)
	}

	if _, err := resolveResourceTypes(snap, "vol-unsynced"); err != errTypeNotFound {
		t.Errorf("typeless subject gave %v, want errTypeNotFound", err)
	}
	if _, err := resolveResourceTypes(snap, "nothing-at-all"); err != errTypeNotFound {
		t.Errorf("absent subject gave %v, want errTypeNotFound", err)
	}
}

// reorderedGraph serves one subject's triples in a caller-chosen order and delegates
// everything else to a real snapshot. Snapshot() short-circuits on an unchanged
// Source, so re-snapshotting one fixture reads one frozen index however many times
// it is called; feeding the orders in is the only way to exercise them
// deterministically.
//
// The real RDFGraph is embedded rather than faked: unmarshalFullRdf also calls
// Contains, and hands the graph to getPropertyValue, which needs the real index for
// bnode-backed values. Only WithSubject may be overridden.
type reorderedGraph struct {
	triplestore.RDFGraph
	subject string
	triples []triplestore.Triple
}

func (g reorderedGraph) WithSubject(s string) []triplestore.Triple {
	if s == g.subject {
		return g.triples
	}
	return g.RDFGraph.WithSubject(s)
}

// reorderedSubjPredGraph is the same idea for the type lookup. A Go type cannot
// override a promoted method conditionally, hence two wrappers rather than one.
type reorderedSubjPredGraph struct {
	triplestore.RDFGraph
	subject, predicate string
	triples            []triplestore.Triple
}

func (g reorderedSubjPredGraph) WithSubjPred(s, p string) []triplestore.Triple {
	if s == g.subject && p == g.predicate {
		return g.triples
	}
	return g.RDFGraph.WithSubjPred(s, p)
}

// permute calls fn once per permutation of ts, reusing the same backing array. The
// standard library has no permutation helper.
func permute(ts []triplestore.Triple, fn func([]triplestore.Triple)) {
	var rec func(k int)
	rec = func(k int) {
		if k == len(ts) {
			fn(ts)
			return
		}
		for i := k; i < len(ts); i++ {
			ts[k], ts[i] = ts[i], ts[k]
			rec(k + 1)
			ts[k], ts[i] = ts[i], ts[k]
		}
	}
	rec(0)
}

// propertyTriplesOf returns the subject's triples minus rdf:type. The type triples
// stay out of the permuted slice: the union loop skips non-property predicates
// anyway, and the one unmarshalFullRdf checks goes through Contains on the embedded
// real snapshot, so including them would only multiply the orders.
func propertyTriplesOf(snap triplestore.RDFGraph, subject string) []triplestore.Triple {
	var out []triplestore.Triple
	for _, t := range snap.WithSubject(subject) {
		if t.Predicate() == rdf.RdfType {
			continue
		}
		out = append(out, t)
	}
	return out
}

// TestMultitypedSubjectPropertyUnionIsDeterministic is the regression guard for the
// sorted copy in unmarshalFullRdf, and it is a permutation sweep precisely so it can
// fail inside a single run: the index order varies per populated source, not per
// snapshot, so a loop that re-snapshots one fixture observes nothing.
//
// 1 cloud:id + two each of cloud:name, cloud:created and cloud:state is 7 triples,
// so the sweep is exhaustive at 5040 orders and still runs in a fraction of a second
// under -race.
func TestMultitypedSubjectPropertyUnionIsDeterministic(t *testing.T) {
	snap := multitypedFixture(t).store.Snapshot()
	triples := propertyTriplesOf(snap, "prod")
	if len(triples) != 7 {
		t.Fatalf("fixture has %d property triples, want 7 — the sweep size is deliberate", len(triples))
	}

	var first map[string]interface{}
	var orders int
	permute(triples, func(order []triplestore.Triple) {
		res := InitResource("keypair", "prod")
		if err := res.unmarshalFullRdf(reorderedGraph{RDFGraph: snap, subject: "prod", triples: order}); err != nil {
			t.Fatalf("unmarshalFullRdf: %s", err)
		}
		orders++
		if first == nil {
			first = res.Properties()
			return
		}
		if !reflect.DeepEqual(res.Properties(), first) {
			t.Fatalf("order %d produced %v, the first order produced %v", orders, res.Properties(), first)
		}
	})
	if orders != 5040 {
		t.Fatalf("swept %d orders, want 5040", orders)
	}

	// Not just stable but stable at the documented value: Triples.Sort orders by
	// the canonical object key `"value"^^<datatype>` and the union assigns
	// last-wins, so the greatest key is retained. Asserting the value as well as
	// the stability is what makes a sort in the opposite direction fail. The
	// zzz/aaa pair is chosen so that neither value is a prefix of the other, where
	// canonical-key order and plain lexicographic order agree.
	if got := first[properties.Name]; got != "zzz" {
		t.Errorf("retained Name is %v, want zzz (the greatest canonical object key)", got)
	}
	if got := first[properties.State]; got != "available" {
		t.Errorf("retained State is %v, want available", got)
	}
	if got, ok := first[properties.Created].(time.Time); !ok || !got.Equal(keypairCreated) {
		t.Errorf("retained Created is %v, want %v", first[properties.Created], keypairCreated)
	}
}

// A list property is appended to in the same loop, so its element order drifted the
// same way. Its own sweep, because adding three more triples to the fixture above
// would take the exhaustive sweep from 5040 orders to 3.6 million.
func TestMultitypedSubjectListPropertyOrderIsDeterministic(t *testing.T) {
	g := NewGraph()
	keypair := InitResource("keypair", "prod")
	keypair.SetProperty(properties.Tags, []string{"env=prod", "team=platform", "cost-centre=42"})
	if err := g.AddResource(keypair); err != nil {
		t.Fatal(err)
	}
	snap := g.store.Snapshot()

	triples := propertyTriplesOf(snap, "prod")
	if len(triples) != 4 {
		t.Fatalf("fixture has %d property triples, want 4", len(triples))
	}

	var first []string
	permute(triples, func(order []triplestore.Triple) {
		res := InitResource("keypair", "prod")
		if err := res.unmarshalFullRdf(reorderedGraph{RDFGraph: snap, subject: "prod", triples: order}); err != nil {
			t.Fatalf("unmarshalFullRdf: %s", err)
		}
		tags, ok := res.Properties()[properties.Tags].([]string)
		if !ok {
			t.Fatalf("Tags came back as %T", res.Properties()[properties.Tags])
		}
		if first == nil {
			first = tags
			return
		}
		if !reflect.DeepEqual(tags, first) {
			t.Fatalf("got %v, first order gave %v", tags, first)
		}
	})

	// Sorted by canonical object key, which for these string literals is the value.
	want := []string{"cost-centre=42", "env=prod", "team=platform"}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("got %v, want %v", first, want)
	}
}

// The type lookup has the same problem one level up: WithSubjPred reads the same
// map-order index, so without sort.Strings the "first type" is arbitrary.
func TestMultitypedSubjectTypeOrderIsDeterministic(t *testing.T) {
	snap := multitypedFixture(t).store.Snapshot()
	typeTriples := append([]triplestore.Triple(nil), snap.WithSubjPred("prod", rdf.RdfType)...)
	if len(typeTriples) != 2 {
		t.Fatalf("fixture has %d rdf:type triples, want 2", len(typeTriples))
	}

	want := []string{"classicloadbalancer", "keypair"}
	permute(typeTriples, func(order []triplestore.Triple) {
		gph := reorderedSubjPredGraph{RDFGraph: snap, subject: "prod", predicate: rdf.RdfType, triples: order}
		types, err := resolveResourceTypes(gph, "prod")
		if err != nil {
			t.Fatalf("resolveResourceTypes: %s", err)
		}
		if !reflect.DeepEqual(types, want) {
			t.Fatalf("got %v, want %v", types, want)
		}
		if typ, err := resolveResourceType(gph, "prod"); err != nil || typ != want[0] {
			t.Fatalf("resolveResourceType gave %q / %v, want %q", typ, err, want[0])
		}
	})
}

// The end-to-end version, through a real Source and its real map-order index build.
// PROBABILISTIC and NOT the regression guard — the sweep above is. It is kept
// because it exercises the parts the sweep stubs out, and it is labelled so nobody
// later mistakes it for the thing that fails when the sort goes away.
func TestMultitypedSubjectIsStableAcrossFreshFixtures(t *testing.T) {
	for i := 0; i < 50; i++ {
		snap := multitypedFixture(t).store.Snapshot()
		res := InitResource("keypair", "prod")
		if err := res.unmarshalFullRdf(snap); err != nil {
			t.Fatalf("iteration %d: %s", i, err)
		}
		if got := res.Properties()[properties.Name]; got != "zzz" {
			t.Fatalf("iteration %d: Name is %v, want zzz", i, got)
		}
	}
}

// unmarshalFullRdf sorts a COPY of the subject's triples. WithSubject returns the
// snapshot's own index slice, and the generated services unmarshal the same subject
// from several goroutines against one shared snapshot while building parent
// relations, so sorting in place is two concurrent writers to one array. Dropping
// the copy trips the race detector; this test also fails without -race, because the
// order assertion catches the mutation on its own.
func TestMultitypedSubjectDoesNotMutateTheSharedIndex(t *testing.T) {
	snap := multitypedFixture(t).store.Snapshot()

	before := append([]triplestore.Triple(nil), snap.WithSubject("prod")...)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := InitResource("keypair", "prod")
			if err := res.unmarshalFullRdf(snap); err != nil {
				t.Errorf("unmarshalFullRdf: %s", err)
			}
		}()
	}
	wg.Wait()

	after := snap.WithSubject("prod")
	if len(after) != len(before) {
		t.Fatalf("the shared index changed length: %d, was %d", len(after), len(before))
	}
	for i := range before {
		if before[i].Equal(after[i]) {
			continue
		}
		t.Fatalf("the shared index was reordered at %d: %s, was %s", i, after[i], before[i])
	}
}
