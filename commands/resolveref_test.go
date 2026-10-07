package commands

import (
	"testing"

	"github.com/theazz/awless-ro/cloud"
	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
)

// `awless-ro show <ref>` takes whatever the user has to hand: an id, an ARN, or a
// name. The order it tries them in is the whole feature — referring to resources by
// their human names instead of their ids is the reason this exists — and it was
// untested, because this package had no tests at all.

func refGraph(t *testing.T) cloud.GraphAPI {
	t.Helper()

	g := graph.NewGraph()
	err := g.AddResource(
		resource("instance", "i-0123", p.Name, "web-server", p.Arn, "arn:aws:ec2:eu-west-1:1:instance/i-0123"),
		resource("instance", "i-4567", p.Name, "database"),
		// Deliberately awkward: one resource's id is another's name.
		resource("instance", "ambiguous", p.Name, "something-else"),
		resource("instance", "i-8901", p.Name, "ambiguous"),
		// Two resources sharing a name, which is allowed in AWS.
		resource("volume", "vol-1", p.Name, "shared"),
		resource("volume", "vol-2", p.Name, "shared"),
		// Two resources sharing an *id*: both use a bare AWS name as their id,
		// and a name is only unique per type, so they are one graph subject
		// carrying two rdf:type triples.
		resource("keypair", "prod", p.Name, "prod"),
		resource("classicloadbalancer", "prod", p.Name, "prod"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func resource(kind, id string, keyValues ...string) *graph.Resource {
	res := graph.InitResource(kind, id)
	for i := 0; i+1 < len(keyValues); i += 2 {
		res.Properties()[keyValues[i]] = keyValues[i+1]
	}
	return res
}

func idsOf(resources []cloud.Resource) []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.Id())
	}
	return out
}

func TestResolveResourceFromRef(t *testing.T) {
	g := refGraph(t)

	cases := []struct {
		name     string
		ref      string
		wantIDs  []string
		wantProp string
	}{
		{
			name:     "by id",
			ref:      "i-0123",
			wantIDs:  []string{"i-0123"},
			wantProp: p.ID,
		},
		{
			name:     "by arn",
			ref:      "arn:aws:ec2:eu-west-1:1:instance/i-0123",
			wantIDs:  []string{"i-0123"},
			wantProp: p.Arn,
		},
		{
			name:     "by name when nothing else matches",
			ref:      "database",
			wantIDs:  []string{"i-4567"},
			wantProp: p.Name,
		},
		{
			// An id takes precedence over a name, so a reference that could be
			// either resolves to the resource whose id it is.
			name:     "id wins over name",
			ref:      "ambiguous",
			wantIDs:  []string{"ambiguous"},
			wantProp: p.ID,
		},
		{
			// The @ prefix is how a user says "I mean the name", which is the only
			// way to reach the resource shadowed by the case above.
			name:     "at prefix forces a name lookup",
			ref:      "@ambiguous",
			wantIDs:  []string{"i-8901"},
			wantProp: p.Name,
		},
		{
			name:     "at prefix on an unambiguous name",
			ref:      "@web-server",
			wantIDs:  []string{"i-0123"},
			wantProp: p.Name,
		},
		{
			// Names are not unique in AWS, so a name may resolve to several
			// resources and the caller has to be told about all of them rather than
			// handed an arbitrary one.
			name:     "a shared name returns every match",
			ref:      "@shared",
			wantIDs:  []string{"vol-1", "vol-2"},
			wantProp: p.Name,
		},
		{
			// A shared id is one subject with two types, and it resolves to one
			// resource per type. An id is tried before a name, so a bare 'prod'
			// comes back through p.ID.
			name:     "a shared id returns one resource per type",
			ref:      "prod",
			wantIDs:  []string{"prod", "prod"},
			wantProp: p.ID,
		},
		{
			name:     "nothing matches",
			ref:      "no-such-thing",
			wantIDs:  []string{},
			wantProp: p.Name,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, resources, prop := resolveResourceFromRef(g, tc.ref)

			got := idsOf(resources)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %v, want %v", got, tc.wantIDs)
			}
			// Order comes out of the graph, so compare as a set.
			for _, want := range tc.wantIDs {
				found := false
				for _, id := range got {
					if id == want {
						found = true
					}
				}
				if !found {
					t.Errorf("%s missing from %v", want, got)
				}
			}
			if prop != tc.wantProp {
				t.Errorf("resolved through %q, want %q", prop, tc.wantProp)
			}
		})
	}
}

// An @ prefix must not leak into the value being searched for, or `show @web` would
// look for a resource literally named "@web".
func TestDeprefix(t *testing.T) {
	cases := map[string]string{
		"@web":      "web",
		"web":       "web",
		"@@web":     "@web",
		"":          "",
		"@":         "",
		"user@host": "user@host",
	}
	for in, want := range cases {
		if got := deprefix(in); got != want {
			t.Errorf("deprefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// With the prefix, a name lookup is the only thing tried: it must not quietly fall
// back to matching an id, or @ would stop meaning anything.
func TestAtPrefixDoesNotFallBackToID(t *testing.T) {
	g := refGraph(t)

	_, resources, prop := resolveResourceFromRef(g, "@i-0123")

	if len(resources) != 0 {
		t.Errorf("@i-0123 resolved to %v; with the prefix only names are searched", idsOf(resources))
	}
	if prop != p.Name {
		t.Errorf("resolved through %q, want %q", prop, p.Name)
	}
}
