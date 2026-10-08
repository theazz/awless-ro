package match

import (
	"testing"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

func TestParseFilter(t *testing.T) {
	tcases := []struct {
		in     string
		expect Filter
		ok     bool
	}{
		{in: "state==Active", expect: Filter{Key: "state", Value: "Active", Exact: true}, ok: true},
		{in: "state=active", expect: Filter{Key: "state", Value: "active"}, ok: true},
		// A third `=` belongs to the value: the operator is read once, and
		// nothing is re-split afterwards.
		{in: "key===value", expect: Filter{Key: "key", Value: "=value", Exact: true}, ok: true},
		{in: "key=", expect: Filter{Key: "key", Value: ""}, ok: true},
		{in: "key==", expect: Filter{Key: "key", Value: "", Exact: true}, ok: true},
		{in: "tag=a=b", expect: Filter{Key: "tag", Value: "a=b"}, ok: true},
		{in: "tag==a=b", expect: Filter{Key: "tag", Value: "a=b", Exact: true}, ok: true},
		{in: "tag==a==b", expect: Filter{Key: "tag", Value: "a==b", Exact: true}, ok: true},
		{in: "=value", expect: Filter{Key: "", Value: "value"}, ok: true},
		{in: "nokey", ok: false},
		{in: "", ok: false},
	}
	for _, tcase := range tcases {
		got, ok := ParseFilter(tcase.in)
		if ok != tcase.ok {
			t.Fatalf("%q: ok %t, want %t", tcase.in, ok, tcase.ok)
		}
		if ok && got != tcase.expect {
			t.Fatalf("%q: got %+v, want %+v", tcase.in, got, tcase.expect)
		}
	}
}

func TestPropertyFilter(t *testing.T) {
	activeKey := resourcetest.AccessKey("AKIAACTIVEKEY000001").Prop("State", "Active").Prop("Username", "alice").Build()
	inactiveKey := resourcetest.AccessKey("AKIAINACTIVEKEY00002").Prop("State", "Inactive").Prop("Username", "bob").Build()
	emptyState := resourcetest.AccessKey("AKIAEMPTYSTATE00003").Prop("State", "").Build()

	record := func(typ string) cloud.Resource {
		return resourcetest.Record("rec_"+typ).Prop("Type", typ).Prop("Name", "mysite.com").Build()
	}

	tcases := []struct {
		name     string
		match    cloud.Matcher
		resource cloud.Resource
		expect   bool
	}{
		// wallix/awless#252: the exact form excludes the longer value.
		{name: "state==Active on Active", match: PropertyFilter("State", "Active", true), resource: activeKey, expect: true},
		{name: "state==Active on Inactive", match: PropertyFilter("State", "Active", true), resource: inactiveKey, expect: false},
		{name: "exact ignores case", match: PropertyFilter("State", "active", true), resource: activeKey, expect: true},

		// The regression guard: `=` stays a substring match, so it still
		// returns BOTH keys. This assertion fails if `=` is ever made exact,
		// which would change what every documented --filter example returns.
		{name: "state=active on Active", match: PropertyFilter("State", "active", false), resource: activeKey, expect: true},
		{name: "state=active on Inactive", match: PropertyFilter("State", "active", false), resource: inactiveKey, expect: true},

		// wallix/awless#296: "cname", "soa" and "aaaa" all contain "a".
		{name: "type==A on A", match: PropertyFilter("Type", "A", true), resource: record("A"), expect: true},
		{name: "type==A on CNAME", match: PropertyFilter("Type", "A", true), resource: record("CNAME"), expect: false},
		{name: "type==A on SOA", match: PropertyFilter("Type", "A", true), resource: record("SOA"), expect: false},
		{name: "type==A on AAAA", match: PropertyFilter("Type", "A", true), resource: record("AAAA"), expect: false},
		{name: "type=A on CNAME", match: PropertyFilter("Type", "A", false), resource: record("CNAME"), expect: true},

		// A non-string column is stringified on both sides, so `==` works on it.
		{name: "public==true on true", match: PropertyFilter("Public", "true", true), resource: resourcetest.Subnet("sub_1").Prop("Public", true).Build(), expect: true},
		{name: "public==true on false", match: PropertyFilter("Public", "true", true), resource: resourcetest.Subnet("sub_2").Prop("Public", false).Build(), expect: false},
		{name: "port==80 on 8080", match: PropertyFilter("Port", "80", true), resource: resourcetest.Listener("lst_1").Prop("Port", 8080).Build(), expect: false},
		{name: "port==80 on 80", match: PropertyFilter("Port", "80", true), resource: resourcetest.Listener("lst_2").Prop("Port", 80).Build(), expect: true},

		// An empty exact value matches only a property that renders as "";
		// an empty substring value matches any resource carrying the property.
		{name: "state== on empty", match: PropertyFilter("State", "", true), resource: emptyState, expect: true},
		{name: "state== on Active", match: PropertyFilter("State", "", true), resource: activeKey, expect: false},
		{name: "state= on Active", match: PropertyFilter("State", "", false), resource: activeKey, expect: true},

		// A property the resource does not carry never matches, either way.
		{name: "exact on missing property", match: PropertyFilter("Inexisting", "x", true), resource: activeKey, expect: false},
		{name: "substring on missing property", match: PropertyFilter("Inexisting", "x", false), resource: activeKey, expect: false},
	}
	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			if got, want := tcase.match.Match(tcase.resource), tcase.expect; got != want {
				t.Fatalf("got %t, want %t", got, want)
			}
		})
	}
}
