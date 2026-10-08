package awsconv

import (
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/theazz/awless-ro/cloud/properties"
)

// Regression tests for wallix/awless#262: a Route 53 record is identified by
// zone + name + type + set identifier, AWS's own key for a record set.

func geoRecord(zoneID, name, setID, continent string) RecordSetInZone {
	return RecordSetInZone{
		ResourceRecordSet: route53types.ResourceRecordSet{
			Name:            awssdk.String(name),
			Type:            route53types.RRTypeA,
			TTL:             awssdk.Int64(60),
			SetIdentifier:   awssdk.String(setID),
			GeoLocation:     &route53types.GeoLocation{ContinentCode: awssdk.String(continent)},
			ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String("192.0.2.1")}},
		},
		ZoneId:   zoneID,
		ZoneName: "mysite.com.",
	}
}

func recordIDOf(t *testing.T, r RecordSetInZone) string {
	t.Helper()
	res, err := NewResource(r)
	if err != nil {
		t.Fatalf("NewResource: %s", err)
	}
	return res.Id()
}

func TestRecordIdentitySeparatesSetIdentifierSiblings(t *testing.T) {
	eu := recordIDOf(t, geoRecord("/hostedzone/Z1", "geo.mysite.com.", "europe", "EU"))
	na := recordIDOf(t, geoRecord("/hostedzone/Z1", "geo.mysite.com.", "north-america", "NA"))
	if eu == na {
		t.Fatalf("two geolocation siblings got the same id %q", eu)
	}
}

func TestRecordIdentitySeparatesHostedZones(t *testing.T) {
	// Split-horizon DNS: one name, one type, one (empty) set identifier, in a
	// private and a public hosted zone. Two zones, two records.
	public := recordIDOf(t, RecordSetInZone{
		ResourceRecordSet: route53types.ResourceRecordSet{Name: awssdk.String("www.mysite.com."), Type: route53types.RRTypeA},
		ZoneId:            "/hostedzone/Z1", ZoneName: "mysite.com.",
	})
	private := recordIDOf(t, RecordSetInZone{
		ResourceRecordSet: route53types.ResourceRecordSet{Name: awssdk.String("www.mysite.com."), Type: route53types.RRTypeA},
		ZoneId:            "/hostedzone/Z2", ZoneName: "mysite.com.",
	})
	if public == private {
		t.Fatalf("the same record in two hosted zones got the same id %q", public)
	}
}

func TestRecordIdentityIsStableAndIgnoresAttributes(t *testing.T) {
	base := geoRecord("/hostedzone/Z1", "geo.mysite.com.", "europe", "EU")
	want := recordIDOf(t, base)

	if got := recordIDOf(t, geoRecord("/hostedzone/Z1", "geo.mysite.com.", "europe", "EU")); got != want {
		t.Errorf("same four key fields, different ids: %q and %q", got, want)
	}

	// Attributes, not identity: editing any of these in AWS must not mint a new
	// node id and leave the old one behind as a phantom after the next sync.
	mutations := map[string]func(r *RecordSetInZone){
		"Weight": func(r *RecordSetInZone) { r.Weight = awssdk.Int64(20) },
		"Region": func(r *RecordSetInZone) { r.Region = route53types.ResourceRecordSetRegionEuWest1 },
		"TTL":    func(r *RecordSetInZone) { r.TTL = awssdk.Int64(300) },
		"ResourceRecords": func(r *RecordSetInZone) {
			r.ResourceRecords = []route53types.ResourceRecord{{Value: awssdk.String("192.0.2.99")}}
		},
	}
	for field, mutate := range mutations {
		r := geoRecord("/hostedzone/Z1", "geo.mysite.com.", "europe", "EU")
		mutate(&r)
		if got := recordIDOf(t, r); got != want {
			t.Errorf("changing %s changed the id: %q, want %q", field, got, want)
		}
	}
}

func TestRecordIdentityHashesTheRawEscapedName(t *testing.T) {
	r := RecordSetInZone{
		ResourceRecordSet: route53types.ResourceRecordSet{Name: awssdk.String(`\052.example.com.`), Type: route53types.RRTypeA},
		ZoneId:            "/hostedzone/Z1",
		ZoneName:          "example.com.",
	}
	res, err := NewResource(r)
	if err != nil {
		t.Fatal(err)
	}
	// The id is over the name exactly as Route 53 returned it, escape included,
	// so that a change to the decoder can never shift record ids.
	if got, want := res.Id(), HashFields("/hostedzone/Z1", `\052.example.com.`, "A", ""); got != want {
		t.Errorf("id = %q, want the hash of the raw escaped name %q", got, want)
	}
	if got, want := res.Properties()[properties.Name], "*.example.com."; got != want {
		t.Errorf("Name = %#v, want the decoded %q", got, want)
	}
}

func TestRecordPropertiesFromTheZoneWrapper(t *testing.T) {
	r := geoRecord("/hostedzone/Z1", "geo.mysite.com.", "europe", "EU")
	r.ZoneName = `\137internal.mysite.com.`
	res, err := NewResource(r)
	if err != nil {
		t.Fatal(err)
	}
	props := res.Properties()

	// Zone displays the zone NAME (decoded), not the /hostedzone/ Id the hash uses.
	if got, want := props[properties.Zone], "_internal.mysite.com."; got != want {
		t.Errorf("Zone = %#v, want %q", got, want)
	}
	if got, want := props[properties.Set], "europe"; got != want {
		t.Errorf("Set = %#v, want %q", got, want)
	}
	if got, want := props[properties.Continent], "EU"; got != want {
		t.Errorf("Continent = %#v, want %q", got, want)
	}

	// SDK v2 enums are named string types; the triple store matches literals by
	// exact type, so an un-normalised RRType would silently fail to match.
	switch v := props[properties.Type].(type) {
	case string:
		if v != "A" {
			t.Errorf("Type = %q, want %q", v, "A")
		}
	case route53types.RRType:
		t.Errorf("Type is an un-normalised route53types.RRType %q, want a plain string", v)
	default:
		t.Errorf("Type is %T, want a plain string", v)
	}
}

func TestRecordWithoutItsZoneIsRejected(t *testing.T) {
	// One id scheme only. Asserted as err != nil, not on the message: %T renders
	// the package's own name, not this file's import alias.
	if _, err := InitResource(route53types.ResourceRecordSet{Name: awssdk.String("www.mysite.com."), Type: route53types.RRTypeA}); err == nil {
		t.Fatal("InitResource accepted a bare ResourceRecordSet; a record needs its hosted zone to be identified")
	}
}

func TestSimpleRoutingRecordIDLiteral(t *testing.T) {
	// The migration contract: record ids changed in this release, and this is
	// what a simple-routing record's id is now. Spelled out so that any later
	// change to it is a deliberate, visible edit.
	got := recordIDOf(t, RecordSetInZone{
		ResourceRecordSet: route53types.ResourceRecordSet{Name: awssdk.String("www.mysite.com."), Type: route53types.RRTypeA},
		ZoneId:            "/hostedzone/Z1",
		ZoneName:          "mysite.com.",
	})
	if want := "awls-9c4f6cad07d1"; got != want {
		t.Errorf("simple-routing record id = %q, want %q", got, want)
	}
}
