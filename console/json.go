package console

import (
	"math"
	"sort"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
)

// JSONProperties is the object a resource is in JSON output. list and show both go
// through it, so that a resource has the same shape whichever command printed it.
//
// The value types are settled here, once, rather than in each displayer: integers
// stay numbers, dates are time.Time and encode as RFC 3339, and the types with a
// netmask encode it as CIDR text (graph/type_json.go). The one value encoding/json
// cannot write at all is a float that is not a number; it becomes null rather than
// failing the whole output.
func JSONProperties(res cloud.Resource) map[string]interface{} {
	props := res.Properties()
	out := make(map[string]interface{}, len(props))
	for k, v := range props {
		out[k] = jsonValue(v)
	}
	return out
}

func jsonValue(v interface{}) interface{} {
	if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return nil
	}
	return v
}

// ResourceRef is how JSON output names a related resource without embedding it. The
// type is part of the reference because an id alone is not unique across types. A
// resource that is referenced but was never synced has the type "notfound".
type ResourceRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

func RefTo(r cloud.Resource) ResourceRef {
	ref := ResourceRef{ID: r.Id(), Type: r.Type()}
	if name, ok := r.Properties()[properties.Name].(string); ok {
		ref.Name = name
	}
	return ref
}

// RefsTo returns the references in (type, id) order: the graph returns relations in
// map order, and the output must not change from one run to the next.
func RefsTo(resources []cloud.Resource) []ResourceRef {
	refs := make([]ResourceRef, 0, len(resources))
	for _, r := range resources {
		refs = append(refs, RefTo(r))
	}
	SortRefs(refs, func(i int) ResourceRef { return refs[i] })
	return refs
}

// SortRefs orders s, whatever holds the references, by (type, id).
func SortRefs[T any](s []T, ref func(int) ResourceRef) {
	sort.SliceStable(s, func(i, j int) bool {
		a, b := ref(i), ref(j)
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.ID < b.ID
	})
}
