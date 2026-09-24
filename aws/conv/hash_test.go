package awsconv

import (
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// A DNS record and a CloudWatch metric have no id of their own in AWS, so one is
// derived from the fields that identify them. That derived value is the node's identity
// in the graph, which means two different resources hashing alike do not look similar,
// they merge into one.

// Fields are separated, so a boundary between them cannot be moved without changing the
// result. Before, they were concatenated: HashFields("ab", "c") equalled
// HashFields("a", "bc").
//
// For metrics this was not a contrivance, because both fields are free-form text. The
// first case here is the one that made this worth fixing: the standard AWS/EC2
// namespace with CPUUtilization collided with a custom AWS/EC2C namespace publishing a
// metric called PUUtilization.
func TestHashFieldsDistinguishesFieldBoundaries(t *testing.T) {
	collisions := [][2][2]string{
		{{"AWS/EC2", "CPUUtilization"}, {"AWS/EC2C", "PUUtilization"}},
		{{"ab", "c"}, {"a", "bc"}},
		{{"my/ns", "Latency"}, {"my/nsL", "atency"}},
		{{"", "ab"}, {"a", "b"}},
		{{"a.b", "c"}, {"a", "b.c"}},
	}

	for _, pair := range collisions {
		first := HashFields(pair[0][0], pair[0][1])
		second := HashFields(pair[1][0], pair[1][1])
		if first == second {
			t.Errorf("%v and %v hash alike (%s); the two resources would merge into one graph node", pair[0], pair[1], first)
		}
	}
}

// The same input always gives the same identifier, in this process and the next: a
// record's node has to keep its identity across syncs, or every sync would produce a
// fresh set of nodes.
func TestHashFieldsIsStable(t *testing.T) {
	first := HashFields("my.zone.", "CNAME")
	for range 10 {
		if got := HashFields("my.zone.", "CNAME"); got != first {
			t.Fatalf("got %s then %s for the same input", first, got)
		}
	}

	// The value itself is pinned, so that a change to the hashing shows up here as a
	// deliberate decision rather than as a pile of unrelated fixture diffs. Changing
	// it means existing local graphs carry the old identifiers until the next sync.
	if want := "awls-0cc32d0b9c65"; first != want {
		t.Errorf("identifier for the same input changed: got %s, want %s.\n"+
			"If this was intended, note in the commit that local graphs keep the old ids until re-synced.", first, want)
	}
}

func TestHashFieldsShape(t *testing.T) {
	id := HashFields("a", "b")

	if !strings.HasPrefix(id, "awls-") {
		t.Errorf("identifier %q should be recognisable as ours by its prefix", id)
	}
	// Long enough that accidental collisions are unlikely at per-account resource
	// counts, short enough to sit in a table of output.
	if got, want := len(id), len("awls-")+12; got != want {
		t.Errorf("identifier length %d, want %d: %q", got, want, id)
	}
	if strings.ContainsAny(id, "\x00 \t\n") {
		t.Errorf("identifier %q contains whitespace or a NUL and would not survive being written to the graph", id)
	}
}

// The identifier a resource gets when it goes through NewResource has to be the one the
// helpers derive, or a relation built from the fields would point at a node that does
// not exist. aws/services/relations.go recomputes a metric's id exactly this way to
// link an alarm to it.
func TestCompositeIDsMatchWhatNewResourceProduces(t *testing.T) {
	record := route53types.ResourceRecordSet{
		Name: awssdk.String("sub.example.com."),
		Type: route53types.RRTypeCname,
	}
	res, err := NewResource(record)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Id(), HashFields("sub.example.com.", "CNAME"); got != want {
		t.Errorf("record id = %q, want %q", got, want)
	}

	metric := cloudwatchtypes.Metric{
		Namespace:  awssdk.String("AWS/EC2"),
		MetricName: awssdk.String("CPUUtilization"),
	}
	res, err = NewResource(metric)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Id(), HashFields("AWS/EC2", "CPUUtilization"); got != want {
		t.Errorf("metric id = %q, want %q", got, want)
	}
}

// Two records differing only in type are different resources, which is the reason the
// type is part of the identifier at all: a name may carry an A record and a TXT record
// at once.
func TestRecordsWithTheSameNameButDifferentTypesAreDistinct(t *testing.T) {
	name := "example.com."

	a, err := NewResource(route53types.ResourceRecordSet{Name: awssdk.String(name), Type: route53types.RRTypeA})
	if err != nil {
		t.Fatal(err)
	}
	txt, err := NewResource(route53types.ResourceRecordSet{Name: awssdk.String(name), Type: route53types.RRTypeTxt})
	if err != nil {
		t.Fatal(err)
	}

	if a.Id() == txt.Id() {
		t.Errorf("an A record and a TXT record on %q got the same id %q", name, a.Id())
	}
}
