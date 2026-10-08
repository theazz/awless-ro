package awsservices

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/match"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/fetch"
)

// Regression tests for wallix/awless#262 (record identity) and #208 (octal
// escapes), driven through the real DNS fetch path against a canned Route 53.

// route53Fake serves hosted zones and their record sets, page by page. It is
// local to this file on purpose: TestBuildDnsRdfGraph depends on the shared
// mockRoute53, and pagination is not expressible there. Both operations are
// implemented because the record fetcher lists zones before it lists records,
// and the embedded nil interface would panic on a missing one.
type route53Fake struct {
	awsfetch.Route53API
	zones []route53types.HostedZone
	// pages[zoneId] is the record sets of a zone, one slice per page.
	pages map[string][][]route53types.ResourceRecordSet

	mu    sync.Mutex
	calls []route53.ListResourceRecordSetsInput
}

func (f *route53Fake) ListHostedZones(_ context.Context, _ *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	return &route53.ListHostedZonesOutput{HostedZones: f.zones}, nil
}

// The page token is carried in StartRecordName as "page-N"; the type and
// identifier are echoed back so the test can check the loop forwards all three.
func (f *route53Fake) ListResourceRecordSets(_ context.Context, in *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	f.mu.Lock()
	f.calls = append(f.calls, *in)
	f.mu.Unlock()

	pages := f.pages[awssdk.ToString(in.HostedZoneId)]
	page := 0
	if in.StartRecordName != nil {
		if _, err := fmt.Sscanf(awssdk.ToString(in.StartRecordName), "page-%d", &page); err != nil {
			return nil, err
		}
	}
	if page >= len(pages) {
		return &route53.ListResourceRecordSetsOutput{}, nil
	}
	out := &route53.ListResourceRecordSetsOutput{ResourceRecordSets: pages[page]}
	if page+1 < len(pages) {
		out.IsTruncated = true
		out.NextRecordName = awssdk.String(fmt.Sprintf("page-%d", page+1))
		out.NextRecordType = route53types.RRTypeA
		out.NextRecordIdentifier = awssdk.String(fmt.Sprintf("id-%d", page+1))
	}
	return out, nil
}

func fetchRecords(t *testing.T, fake *route53Fake) []cloud.Resource {
	t.Helper()
	srv := &Dns{
		Route53API: fake,
		region:     "global",
		fetcher: fetch.NewFetcher(awsfetch.BuildDnsFetchFuncs(
			awsfetch.NewConfig(&awsfetch.AWSAPI{Route53: fake}))),
	}
	g, err := srv.FetchByType(context.Background(), cloud.Record)
	if err != nil {
		t.Fatalf("FetchByType(record): %v", err)
	}
	records, err := g.Find(cloud.NewQuery(cloud.Record))
	if err != nil {
		t.Fatalf("Find(record): %v", err)
	}
	return records
}

func zone(id, name string) route53types.HostedZone {
	return route53types.HostedZone{Id: awssdk.String(id), Name: awssdk.String(name)}
}

func aRecord(name, value string) route53types.ResourceRecordSet {
	return route53types.ResourceRecordSet{
		Name:            awssdk.String(name),
		Type:            route53types.RRTypeA,
		TTL:             awssdk.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String(value)}},
	}
}

func byProp(records []cloud.Resource, prop string) map[string]cloud.Resource {
	out := make(map[string]cloud.Resource)
	for _, r := range records {
		out[fmt.Sprint(r.Properties()[prop])] = r
	}
	return out
}

func TestDnsGeolocationSiblingsAreSeparateRecords(t *testing.T) {
	eu := aRecord("geo.mysite.com.", "192.0.2.1")
	eu.SetIdentifier = awssdk.String("europe")
	eu.GeoLocation = &route53types.GeoLocation{ContinentCode: awssdk.String("EU")}
	na := aRecord("geo.mysite.com.", "192.0.2.2")
	na.SetIdentifier = awssdk.String("north-america")
	na.GeoLocation = &route53types.GeoLocation{ContinentCode: awssdk.String("NA")}

	records := fetchRecords(t, &route53Fake{
		zones: []route53types.HostedZone{zone("/hostedzone/Z1", "mysite.com.")},
		pages: map[string][][]route53types.ResourceRecordSet{"/hostedzone/Z1": {{eu, na}}},
	})

	// Upstream collapsed these into one node carrying both sets and both
	// continents, a pairing that matched no real record.
	if len(records) != 2 {
		t.Fatalf("got %d records, want the 2 geolocation siblings", len(records))
	}
	if records[0].Id() == records[1].Id() {
		t.Fatalf("both siblings share the id %q", records[0].Id())
	}
	bySet := byProp(records, properties.Set)
	for set, wantContinent := range map[string]string{"europe": "EU", "north-america": "NA"} {
		r, ok := bySet[set]
		if !ok {
			t.Fatalf("no record with Set %q; got %v", set, bySet)
		}
		if got := r.Properties()[properties.Continent]; got != wantContinent {
			t.Errorf("record %q: Continent = %#v, want %q (cross-contaminated sibling)", set, got, wantContinent)
		}
	}
}

func TestDnsSameNameInTwoZonesAreSeparateRecords(t *testing.T) {
	records := fetchRecords(t, &route53Fake{
		zones: []route53types.HostedZone{
			zone("/hostedzone/Z1", "mysite.com."),
			zone("/hostedzone/Z2", "mysite.com."),
		},
		pages: map[string][][]route53types.ResourceRecordSet{
			"/hostedzone/Z1": {{aRecord("www.mysite.com.", "192.0.2.1")}},
			"/hostedzone/Z2": {{aRecord("www.mysite.com.", "10.0.0.1")}},
		},
	})
	if len(records) != 2 {
		t.Fatalf("got %d records, want one per hosted zone", len(records))
	}
	if records[0].Id() == records[1].Id() {
		t.Fatalf("split-horizon records share the id %q", records[0].Id())
	}
	for _, r := range records {
		if got := r.Properties()[properties.Zone]; got != "mysite.com." {
			t.Errorf("record %s: Zone = %#v, want the zone name %q", r.Id(), got, "mysite.com.")
		}
	}
	byValue := map[string]string{}
	for _, r := range records {
		byValue[fmt.Sprint(r.Properties()[properties.Records])] = r.Id()
	}
	if len(byValue) != 2 {
		t.Errorf("the two records carry the same payload, want one each: %v", byValue)
	}
}

func TestDnsWildcardNameIsDecodedAndFilterable(t *testing.T) {
	records := fetchRecords(t, &route53Fake{
		zones: []route53types.HostedZone{zone("/hostedzone/Z1", "example.com.")},
		pages: map[string][][]route53types.ResourceRecordSet{
			"/hostedzone/Z1": {{aRecord(`\052.example.com.`, "192.0.2.1")}},
		},
	})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if got, want := records[0].Properties()[properties.Name], "*.example.com."; got != want {
		t.Fatalf("Name = %#v, want %q", got, want)
	}
	// The predicate `--filter name=*.example.com` builds (console/displayer.go).
	if !match.PropertyFilter(properties.Name, "*.example.com", false).Match(records[0]) {
		t.Error("--filter name=*.example.com does not match the decoded wildcard record")
	}
}

func TestDnsAliasIsDecodedButTxtPayloadIsRaw(t *testing.T) {
	alias := route53types.ResourceRecordSet{
		Name:        awssdk.String("cdn.example.com."),
		Type:        route53types.RRTypeA,
		AliasTarget: &route53types.AliasTarget{DNSName: awssdk.String(`\052.edge.example.net.`), HostedZoneId: awssdk.String("Z2")},
	}
	// A TXT payload is opaque data: its backslashes must survive untouched.
	const payload = `"v=spf1 \052 literal\\backslash"`
	txt := route53types.ResourceRecordSet{
		Name:            awssdk.String("example.com."),
		Type:            route53types.RRTypeTxt,
		TTL:             awssdk.Int64(300),
		ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String(payload)}},
	}
	records := fetchRecords(t, &route53Fake{
		zones: []route53types.HostedZone{zone("/hostedzone/Z1", "example.com.")},
		pages: map[string][][]route53types.ResourceRecordSet{"/hostedzone/Z1": {{alias, txt}}},
	})
	byType := byProp(records, properties.Type)
	if got, want := byType["A"].Properties()[properties.Alias], "*.edge.example.net."; got != want {
		t.Errorf("Alias = %#v, want the decoded %q", got, want)
	}
	if got, want := byType["TXT"].Properties()[properties.Records], []string{payload}; !reflect.DeepEqual(got, want) {
		t.Errorf("TXT Records = %#v, want byte for byte %#v", got, want)
	}
}

func TestDnsRecordPaginationWrapsEveryPageOnce(t *testing.T) {
	fake := &route53Fake{
		zones: []route53types.HostedZone{zone("/hostedzone/Z1", "mysite.com.")},
		pages: map[string][][]route53types.ResourceRecordSet{"/hostedzone/Z1": {
			{aRecord("a.mysite.com.", "192.0.2.1"), aRecord("b.mysite.com.", "192.0.2.2")},
			{aRecord("c.mysite.com.", "192.0.2.3")},
			{aRecord("d.mysite.com.", "192.0.2.4")},
		}},
	}
	records := fetchRecords(t, fake)

	var names []string
	ids := map[string]bool{}
	for _, r := range records {
		names = append(names, fmt.Sprint(r.Properties()[properties.Name]))
		ids[r.Id()] = true
		if got := r.Properties()[properties.Zone]; got != "mysite.com." {
			t.Errorf("record %v: Zone = %#v, want %q on every page", r.Properties()[properties.Name], got, "mysite.com.")
		}
	}
	sort.Strings(names)
	if want := []string{"a.mysite.com.", "b.mysite.com.", "c.mysite.com.", "d.mysite.com."}; !reflect.DeepEqual(names, want) {
		t.Errorf("records = %v, want every page exactly once: %v", names, want)
	}
	if len(ids) != len(records) {
		t.Errorf("%d records but %d distinct ids", len(records), len(ids))
	}

	if len(fake.calls) != 3 {
		t.Fatalf("ListResourceRecordSets called %d times, want 3 (one per page)", len(fake.calls))
	}
	for i, call := range fake.calls[1:] {
		page := i + 1
		if got, want := awssdk.ToString(call.StartRecordName), fmt.Sprintf("page-%d", page); got != want {
			t.Errorf("call %d: StartRecordName = %q, want %q", page, got, want)
		}
		if call.StartRecordType != route53types.RRTypeA {
			t.Errorf("call %d: StartRecordType = %q, want the NextRecordType of the previous page", page, call.StartRecordType)
		}
		if got, want := awssdk.ToString(call.StartRecordIdentifier), fmt.Sprintf("id-%d", page); got != want {
			t.Errorf("call %d: StartRecordIdentifier = %q, want %q", page, got, want)
		}
	}
}
