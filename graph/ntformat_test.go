package graph

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	p "github.com/theazz/awless-ro/cloud/properties"
)

// The on-disk N-Triples format is the one thing in this package that outlives the
// process: `sync` writes it, every later `--local` command reads it back. These
// tests pin it, so that changing the triple store underneath cannot silently change
// or corrupt what is already on disk.
//
// There are two committed files, and they are not the same format:
//
//   - testdata/legacy.nt was written by github.com/wallix/triplestore, before that
//     dependency was replaced. It is frozen. Nothing regenerates it, because its
//     whole job is to stand in for the graphs sitting in ~/.awless-ro on machines
//     that have not re-synced, and prove they still load.
//   - testdata/graph.nt is what the current writer produces. Regenerate it with
//     -update only when a format change is the point, and say so in the commit.
//
// Both have to yield the same resources.

var updateGolden = flag.Bool("update", false, "rewrite testdata/graph.nt from the current implementation")

const (
	goldenPath = "testdata/graph.nt"
	legacyPath = "testdata/legacy.nt"
)

// ntCorpus covers every shape the serialiser has to deal with: the four literal
// types our vocabulary declares, both kinds of list, a nested struct that becomes
// a blank node, and the string values that escaping has to survive.
//
// Every generated node must be distinguishable by its contents, because
// canonicalNT relies on that to give the random identifiers stable names.
func ntCorpus() []*Resource {
	_, cidr24, _ := net.ParseCIDR("10.0.0.0/24")
	_, cidr16, _ := net.ParseCIDR("172.16.0.0/16")

	inst := InitResource("instance", "inst-one")
	inst.Properties()[p.Name] = "web-server"
	inst.Properties()[p.Created] = time.Unix(1700000000, 0).UTC()
	inst.Properties()[p.Cooldown] = 300
	inst.Properties()[p.Attached] = true
	inst.Properties()[p.Actions] = []string{"start", "stop"}
	inst.Properties()[p.AvailabilityZones] = []string{"eu-west-1a", "eu-west-1b"}

	sgroup := InitResource("securitygroup", "sg-three")
	sgroup.Properties()[p.InboundRules] = []*FirewallRule{
		{PortRange: PortRange{FromPort: 80, ToPort: 80}, Protocol: "tcp", IPRanges: []*net.IPNet{cidr24}},
		{PortRange: PortRange{FromPort: 443, ToPort: 443}, Protocol: "udp", Sources: []string{"sg-other"}},
	}

	bucket := InitResource("bucket", "bucket-two")
	bucket.Properties()[p.Grants] = []*Grant{
		{Permission: "READ", Grantee: Grantee{GranteeID: "usr-a", GranteeDisplayName: "Alice", GranteeType: "CanonicalUser"}},
		{Permission: "WRITE", Grantee: Grantee{GranteeID: "usr-b", GranteeDisplayName: "Bob", GranteeType: "Group"}},
	}

	rtable := InitResource("routetable", "rt-five")
	rtable.Properties()[p.Routes] = []*Route{
		{Destination: cidr16, DestinationPrefixListId: "pl-1"},
	}

	// The string values worth worrying about. A policy document is JSON, so it is
	// full of double quotes; UserData and condition values can carry a literal
	// backslash-n that is not a newline.
	policy := InitResource("policy", "pol-four")
	policy.Properties()[p.Document] = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	// Deliberately only values that survive today. The ones that do not are in
	// TestNTRoundTripsAwkwardLiterals, where they are flagged; putting them here
	// would pin corruption into the golden file.
	notes := InitResource("policy", "pol-six")
	notes.Properties()[p.Document] = "first\nsecond\ttabbed and \"quotes\" plus юникод 世界"

	return []*Resource{inst, sgroup, bucket, rtable, policy, notes}
}

func corpusGraph(t *testing.T) *Graph {
	t.Helper()
	g := NewGraph()
	if err := g.AddResource(ntCorpus()...); err != nil {
		t.Fatal(err)
	}
	return g
}

// The identifiers the marshaller invents for nested structs: eight hex digits at
// most, from a random source. Corpus resource identifiers all contain a dash, so
// they can never be mistaken for one of these.
var generatedID = regexp.MustCompile(`<[0-9a-f]{1,8}>|_:[0-9a-f]{1,8}`)

func rawID(match string) string {
	if s, ok := strings.CutPrefix(match, "_:"); ok {
		return s
	}
	return strings.TrimSuffix(strings.TrimPrefix(match, "<"), ">")
}

// canonicalNT puts serialised output into a stable form. Two things vary between
// runs and neither is meaningful: triple order comes out of a map, and the
// identifiers invented for nested structs come from a random source.
//
// Names cannot be handed out in order of appearance, because appearance order is
// exactly what is unstable. Instead each generated identifier gets a signature
// built from every line it appears in, with other generated identifiers replaced by
// their own signatures — iterated until it stops changing, since a signature can
// reference a nested node. Identifiers are then sorted by signature and named
// b0, b1, ... Structurally identical nodes end up with equal signatures, and
// swapping their names leaves the sorted output unchanged, so ties are harmless.
func canonicalNT(t *testing.T, nt string) string {
	t.Helper()

	var lines []string
	for _, l := range strings.Split(nt, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}

	ids := make(map[string]bool)
	for _, l := range lines {
		for _, m := range generatedID.FindAllString(l, -1) {
			ids[rawID(m)] = true
		}
	}

	sig := make(map[string]string, len(ids))
	for id := range ids {
		sig[id] = "?"
	}

	render := func(line, self string) string {
		return generatedID.ReplaceAllStringFunc(line, func(m string) string {
			id := rawID(m)
			if id == self {
				return "SELF"
			}
			return "(" + sig[id] + ")"
		})
	}

	for range len(ids) + 1 {
		next := make(map[string]string, len(ids))
		for id := range ids {
			var mentions []string
			for _, l := range lines {
				for _, m := range generatedID.FindAllString(l, -1) {
					if rawID(m) == id {
						mentions = append(mentions, render(l, id))
						break
					}
				}
			}
			sort.Strings(mentions)
			next[id] = strings.Join(mentions, "|")
		}
		if fmt.Sprint(next) == fmt.Sprint(sig) {
			break
		}
		sig = next
	}

	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return sig[ordered[i]] < sig[ordered[j]] })

	names := make(map[string]string, len(ordered))
	for i, id := range ordered {
		names[id] = fmt.Sprintf("b%d", i)
	}

	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, generatedID.ReplaceAllStringFunc(l, func(m string) string {
			id := rawID(m)
			if strings.HasPrefix(m, "_:") {
				return "_:" + names[id]
			}
			return "<" + names[id] + ">"
		}))
	}
	sort.Strings(out)

	return strings.Join(out, "\n") + "\n"
}

func TestNTSerialisationMatchesGolden(t *testing.T) {
	got := canonicalNT(t, corpusGraph(t).MustMarshal())

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%s missing; regenerate with: go test ./graph/ -run TestNTSerialisationMatchesGolden -update", err)
	}
	if got != string(want) {
		t.Errorf("serialised form changed.\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// Reading is pinned separately from writing, and against both formats: a graph
// written by the abandoned dependency has to yield exactly the resources a graph
// written today does. This is what makes replacing the triple store safe for anyone
// who has already synced.
func TestNTGoldensParseIntoTheSameResources(t *testing.T) {
	for _, path := range []string{legacyPath, goldenPath} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			loaded := NewGraph()
			if err := loaded.UnmarshalFromReaders(strings.NewReader(string(raw))); err != nil {
				t.Fatalf("cannot parse %s: %s", path, err)
			}

			for _, want := range ntCorpus() {
				got, err := loaded.GetResource(want.Type(), want.Id())
				if err != nil {
					t.Errorf("%s %s: %s", want.Type(), want.Id(), err)
					continue
				}
				compareProperties(t, want, got)
			}
		})
	}
}

func TestNTRoundTripsCorpus(t *testing.T) {
	serialised := corpusGraph(t).MustMarshal()

	reloaded := NewGraph()
	if err := reloaded.UnmarshalFromReaders(strings.NewReader(serialised)); err != nil {
		t.Fatalf("cannot parse our own output: %s", err)
	}

	for _, want := range ntCorpus() {
		got, err := reloaded.GetResource(want.Type(), want.Id())
		if err != nil {
			t.Errorf("%s %s: %s", want.Type(), want.Id(), err)
			continue
		}
		compareProperties(t, want, got)
	}
}

// TestNTRoundTripsAwkwardLiterals is the value-level version of the round trip: a
// property goes in, the same property has to come back out.
//
// The last three cases were corrupted by the serialiser this replaced, and were
// listed here as known-broken before the replacement so that fixing them would show
// up as a test failure telling us to drop the flag. That is what happened. The one
// that mattered in practice is the literal backslash-n: the old writer turned a real
// newline into the two characters \n but left an existing backslash alone, so the
// reader could not tell the two apart and rewrote one into the other. Policy
// documents are JSON, and JSON spells an embedded newline exactly that way.
func TestNTRoundTripsAwkwardLiterals(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "plain", value: "hello world"},
		{name: "json policy", value: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`},
		{name: "newline", value: "line1\nline2"},
		{name: "carriage return", value: "a\rb"},
		{name: "tab", value: "a\tb"},
		{name: "windows path", value: `C:\path\to`},
		{name: "trailing quote", value: `value"`},
		{name: "quote space dot", value: `value" .`},
		{name: "unicode", value: "привет 世界 🙂"},
		{name: "closing brace", value: "}"},
		{name: "ends with dot", value: "ends with ."},
		{name: "dot inside", value: "has . inside"},

		{name: "literal backslash n", value: `a \n b`},
		{name: "quote then langtag", value: `value"@en`},
		{name: "quote then datatype", value: `value"^^x`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGraph()
			res := InitResource("policy", "pol-one")
			res.Properties()[p.Document] = tc.value
			if err := g.AddResource(res); err != nil {
				t.Fatal(err)
			}
			serialised := g.MustMarshal()

			reloaded := NewGraph()
			err := reloaded.UnmarshalFromReaders(strings.NewReader(serialised))
			var back string
			if err == nil {
				var got *Resource
				got, err = reloaded.GetResource("policy", "pol-one")
				if err == nil {
					back, _ = got.Properties()[p.Document].(string)
				}
			}

			if err != nil || back != tc.value {
				t.Errorf("value did not survive the round trip.\n  in:   %q\n  out:  %q\n  err:  %v\n  .nt:  %q", tc.value, back, err, serialised)
			}
		})
	}
}

// compareProperties ignores the order of list-valued properties. A list is stored
// as one triple per element, and triples carry no order, so the order a list comes
// back in is not part of the format and must not be asserted.
func compareProperties(t *testing.T, want, got *Resource) {
	t.Helper()
	for key, wantVal := range want.Properties() {
		gotVal, ok := got.Properties()[key]
		if !ok {
			t.Errorf("%s %s: property %s missing after reload", want.Type(), want.Id(), key)
			continue
		}
		if valueKey(wantVal) != valueKey(gotVal) {
			t.Errorf("%s %s: property %s\n  want %s\n  got  %s", want.Type(), want.Id(), key, valueKey(wantVal), valueKey(gotVal))
		}
	}
}

func valueKey(v interface{}) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return fmt.Sprint(v)
	}
	parts := make([]string, rv.Len())
	for i := range parts {
		parts[i] = fmt.Sprint(rv.Index(i).Interface())
	}
	sort.Strings(parts)
	return "[" + strings.Join(parts, " ") + "]"
}
