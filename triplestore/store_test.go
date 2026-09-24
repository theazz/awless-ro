package triplestore

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestAddIsIdempotent(t *testing.T) {
	src := NewSource()
	src.Add(SubjPred("s", "p").StringLiteral("v"))
	src.Add(SubjPred("s", "p").StringLiteral("v"))

	if got := src.Snapshot().Count(); got != 1 {
		t.Fatalf("adding the same triple twice gave %d triples, want 1", got)
	}
}

// Triple identity is subject, predicate and object together: changing any one of
// them is a different triple.
func TestTripleIdentity(t *testing.T) {
	base := SubjPred("s", "p").StringLiteral("v")

	differ := []Triple{
		SubjPred("other", "p").StringLiteral("v"),
		SubjPred("s", "other").StringLiteral("v"),
		SubjPred("s", "p").StringLiteral("other"),
		SubjPred("s", "p").Resource("v"),
		SubjPred("s", "p").Bnode("v"),
		SubjPred("s", "p").IntegerLiteral(1),
		BnodePred("s", "p").StringLiteral("v"),
	}

	src := NewSource()
	src.Add(base)
	for _, tri := range differ {
		src.Add(tri)
		if base.Equal(tri) {
			t.Errorf("%s compared equal to %s", tri, base)
		}
	}
	if got, want := src.Snapshot().Count(), len(differ)+1; got != want {
		t.Fatalf("got %d distinct triples, want %d", got, want)
	}
}

// A string literal and an integer literal that print the same are still different
// objects, or a property would change type silently on reload.
func TestLiteralTypeIsPartOfIdentity(t *testing.T) {
	asText := SubjPred("s", "p").StringLiteral("1")
	asNumber := SubjPred("s", "p").IntegerLiteral(1)

	if asText.Equal(asNumber) {
		t.Fatal(`"1" and 1 compared equal`)
	}
}

func TestSnapshotIndexes(t *testing.T) {
	src := NewSource()
	src.Add(
		SubjPred("a", "type").Resource("Instance"),
		SubjPred("a", "name").StringLiteral("first"),
		SubjPred("a", "tag").StringLiteral("x"),
		SubjPred("a", "tag").StringLiteral("y"),
		SubjPred("b", "type").Resource("Instance"),
		SubjPred("c", "type").Resource("Subnet"),
	)
	snap := src.Snapshot()

	if got, want := len(snap.WithSubject("a")), 4; got != want {
		t.Errorf("WithSubject(a) = %d triples, want %d", got, want)
	}
	if got, want := len(snap.WithSubjPred("a", "tag")), 2; got != want {
		t.Errorf("WithSubjPred(a, tag) = %d triples, want %d", got, want)
	}
	if got, want := len(snap.WithSubjPred("a", "name")), 1; got != want {
		t.Errorf("WithSubjPred(a, name) = %d triples, want %d", got, want)
	}

	var instances []string
	for _, tri := range snap.WithPredObj("type", Resource("Instance")) {
		instances = append(instances, tri.Subject())
	}
	sort.Strings(instances)
	if got, want := fmt.Sprint(instances), "[a b]"; got != want {
		t.Errorf("WithPredObj(type, Instance) subjects = %s, want %s", got, want)
	}

	if len(snap.WithSubject("missing")) != 0 {
		t.Error("WithSubject on an unknown subject should be empty")
	}
	if len(snap.WithPredObj("type", Resource("Nothing"))) != 0 {
		t.Error("WithPredObj on an unknown object should be empty")
	}
	if snap.WithPredObj("type", nil) != nil {
		t.Error("WithPredObj with a nil object should be empty, not panic")
	}
}

func TestContains(t *testing.T) {
	present := SubjPred("s", "p").StringLiteral("v")

	src := NewSource()
	src.Add(present)
	snap := src.Snapshot()

	// Contains works on value, not on pointer identity: callers build a fresh
	// triple to ask the question.
	if !snap.Contains(SubjPred("s", "p").StringLiteral("v")) {
		t.Error("an equal triple built separately was not found")
	}
	if snap.Contains(SubjPred("s", "p").StringLiteral("other")) {
		t.Error("a triple that was never added was found")
	}
	if snap.Contains(nil) {
		t.Error("Contains(nil) should be false, not panic")
	}
}

// A snapshot is immutable: taking one and then writing to the source must not change
// what the snapshot answers. Fetch relies on this, holding a snapshot while other
// goroutines still add to the graph.
func TestSnapshotIsIsolatedFromLaterWrites(t *testing.T) {
	src := NewSource()
	src.Add(SubjPred("a", "p").StringLiteral("1"))

	snap := src.Snapshot()
	if got, want := snap.Count(), 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}

	src.Add(SubjPred("b", "p").StringLiteral("2"))

	if got, want := snap.Count(), 1; got != want {
		t.Errorf("the snapshot changed after a write: got %d, want %d", got, want)
	}
	if got, want := src.Snapshot().Count(), 2; got != want {
		t.Errorf("a fresh snapshot did not see the write: got %d, want %d", got, want)
	}
}

// sync fetches every AWS service in parallel into one graph, so concurrent writes
// and snapshots have to be safe. Run with -race for this to mean anything.
func TestConcurrentAddAndSnapshot(t *testing.T) {
	src := NewSource()

	const writers, each = 8, 200
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range each {
				src.Add(SubjPred(fmt.Sprintf("s%d-%d", w, i), "p").StringLiteral("v"))
			}
		}(w)
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				src.Snapshot().Count()
			}
		}()
	}
	wg.Wait()

	if got, want := src.Snapshot().Count(), writers*each; got != want {
		t.Fatalf("got %d triples, want %d", got, want)
	}
}

func TestCopyTriples(t *testing.T) {
	src := NewSource()
	src.Add(
		SubjPred("a", "p").StringLiteral("1"),
		SubjPred("b", "p").StringLiteral("2"),
	)

	copied := Triples(src.CopyTriples())
	if got, want := len(copied), 2; got != want {
		t.Fatalf("got %d triples, want %d", got, want)
	}

	// The copy is independent: adding to it must not reach the source.
	other := NewSource()
	other.Add(copied...)
	other.Add(SubjPred("c", "p").StringLiteral("3"))

	if got, want := src.Snapshot().Count(), 2; got != want {
		t.Errorf("the source changed through its copy: got %d, want %d", got, want)
	}
}

func TestTriplesEqualIgnoresOrder(t *testing.T) {
	a := Triples{
		SubjPred("s", "p").StringLiteral("1"),
		SubjPred("s", "p").StringLiteral("2"),
	}
	b := Triples{
		SubjPred("s", "p").StringLiteral("2"),
		SubjPred("s", "p").StringLiteral("1"),
	}
	c := Triples{
		SubjPred("s", "p").StringLiteral("1"),
		SubjPred("s", "p").StringLiteral("3"),
	}

	if !a.Equal(b) {
		t.Error("the same triples in a different order compared unequal")
	}
	if a.Equal(c) {
		t.Error("different triples compared equal")
	}
	if a.Equal(Triples{a[0]}) {
		t.Error("sets of different sizes compared equal")
	}
}

type probeStruct struct {
	Name     string   `predicate:"cloud:name"`
	Count    int      `predicate:"cloud:count"`
	Enabled  bool     `predicate:"cloud:enabled"`
	Tags     []string `predicate:"cloud:tag"`
	Untagged string
	Nested   probeNested `predicate:"cloud:nested" bnode:"fixed-bnode"`
	Missing  *string     `predicate:"cloud:missing"`
	Stringer *net.IPNet  `predicate:"cloud:cidr"`
}

type probeNested struct {
	Inner string `predicate:"cloud:inner"`
}

func TestTriplesFromStruct(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("10.0.0.0/24")

	got := Triples(TriplesFromStruct("subj", probeStruct{
		Name:     "n",
		Count:    3,
		Enabled:  true,
		Tags:     []string{"a", "b"},
		Untagged: "ignored",
		Nested:   probeNested{Inner: "deep"},
		Stringer: cidr,
	}))

	want := Triples{
		SubjPred("subj", "cloud:name").StringLiteral("n"),
		SubjPred("subj", "cloud:count").IntegerLiteral(3),
		SubjPred("subj", "cloud:enabled").BooleanLiteral(true),
		SubjPred("subj", "cloud:tag").StringLiteral("a"),
		SubjPred("subj", "cloud:tag").StringLiteral("b"),
		// A nested struct becomes a blank node, linked from the parent.
		SubjPred("subj", "cloud:nested").Bnode("fixed-bnode"),
		BnodePred("fixed-bnode", "cloud:inner").StringLiteral("deep"),
		// net.IPNet has no literal type of its own; it is stored as its text form
		// through fmt.Stringer.
		SubjPred("subj", "cloud:cidr").StringLiteral("10.0.0.0/24"),
	}

	if !got.Equal(want) {
		t.Errorf("triples differ\n  got\n%s\n  want\n%s", got, want)
	}
}

// A field with no predicate tag is not stored, and neither is a nil pointer: absent
// has to stay absent rather than become an empty value.
func TestTriplesFromStructSkipsUntaggedAndNil(t *testing.T) {
	for _, tri := range TriplesFromStruct("subj", probeStruct{Untagged: "ignored"}) {
		if tri.Predicate() == "cloud:missing" {
			t.Error("a nil pointer field was stored")
		}
		if lit, ok := tri.Object().Literal(); ok && lit.Value() == "ignored" {
			t.Error("a field without a predicate tag was stored")
		}
	}
}

// Nested nodes get generated names when the bnode tag is empty, and two of them must
// never collide, or two nested structs merge into one.
func TestGeneratedBlankNodeNamesAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for range 2000 {
		id := newBnodeID()
		if seen[id] {
			t.Fatalf("generated blank node name %q twice", id)
		}
		seen[id] = true
	}
}

func TestTriplesFromStructIgnoresNonStructs(t *testing.T) {
	for _, in := range []interface{}{nil, "text", 42, []string{"a"}, (*probeNested)(nil)} {
		if got := TriplesFromStruct("subj", in); got != nil {
			t.Errorf("TriplesFromStruct(%T) = %v, want nil", in, got)
		}
	}
}

func TestObjectLiteralRejectsWhatItCannotStore(t *testing.T) {
	_, err := ObjectLiteral(struct{ A int }{1})
	if err == nil {
		t.Fatal("expected an error for a type with no literal representation")
	}
	if !strings.Contains(err.Error(), "struct") {
		t.Errorf("the error should name the type, got %q", err)
	}
}
