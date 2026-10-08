package console

import (
	"reflect"
	"testing"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
)

// A record's default columns are declared twice: as property names in
// ColumnsInListing and as column definitions in DefaultsColumnDefinitions.
// The two lists have to agree, or `list records` and `show` drift apart.
// Set is in both because routing-policy siblings of one name+type (geolocation,
// weighted, …) are separate records and would otherwise print as identical rows.
func TestRecordDefaultColumnsAgree(t *testing.T) {
	var fromDefinitions []string
	for _, def := range DefaultsColumnDefinitions[cloud.Record] {
		fromDefinitions = append(fromDefinitions, def.propKey())
	}
	if got, want := fromDefinitions, ColumnsInListing[cloud.Record]; !reflect.DeepEqual(got, want) {
		t.Fatalf("record column definitions %v do not match the listing columns %v", got, want)
	}

	want := []string{properties.ID, properties.Type, properties.Name, properties.Set}
	if got := ColumnsInListing[cloud.Record][:len(want)]; !reflect.DeepEqual(got, want) {
		t.Errorf("record listing columns start with %v, want %v", got, want)
	}
}
