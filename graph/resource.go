/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package graph

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/triplestore"
)

type Resource struct {
	kind, id string

	properties map[string]interface{}
	relations  map[string][]*Resource
	meta       map[string]interface{}
}

const notFoundResourceType = "notfound"

func NotFoundResource(id string) *Resource {
	return &Resource{
		id:         id,
		kind:       notFoundResourceType,
		properties: make(map[string]interface{}),
		meta:       make(map[string]interface{}),
		relations:  make(map[string][]*Resource),
	}
}

func InitResource(kind, id string) *Resource {
	return &Resource{
		id:         id,
		kind:       kind,
		properties: map[string]interface{}{properties.ID: id},
		meta:       make(map[string]interface{}),
		relations:  make(map[string][]*Resource),
	}
}

func (res *Resource) String() string {
	if res == nil {
		res = &Resource{}
	}
	return res.Format("%n[%t]")
}

var (
	layoutRegex = regexp.MustCompile("%(\\[(\\w+)\\])?(\\w)")
)

func (res *Resource) Format(layout string) (out string) {
	out = layout
	if matches := layoutRegex.FindAllStringSubmatch(layout, -1); matches != nil {
		for _, match := range matches {
			var val string
			verb := match[len(match)-1]
			switch verb {
			case "i":
				val = "<none>"
				if id := res.Id(); id != "" {
					val = id
				}
			case "t":
				switch {
				case res.Type() == notFoundResourceType:
					val = "<not-found>"
				case res.Type() != "":
					val = res.Type()
				default:
					val = "<none>"
				}
			case "n":
				val = res.Id()
				if name, ok := res.properties[properties.Name].(string); ok && name != "" {
					val = "@" + name
				}
			case "p":
				if v, ok := res.properties[match[2]]; ok {
					val = fmt.Sprint(v)
				}
			default:
				return fmt.Sprintf("invalid verb '%s' in layout '%s'", verb, layout)

			}
			out = strings.Replace(out, match[0], val, 1)
		}
	}
	return
}

func (res *Resource) Type() string {
	return res.kind
}

func (res *Resource) Id() string {
	return res.id
}

func (res *Resource) Properties() map[string]interface{} {
	return res.properties
}

func (res *Resource) Property(k string) (interface{}, bool) {
	v, ok := res.properties[k]
	return v, ok
}

func (res *Resource) Meta(k string) (interface{}, bool) {
	v, ok := res.meta[k]
	return v, ok
}

func (res *Resource) SetProperty(k string, v interface{}) {
	res.properties[k] = v
}

func (res *Resource) AddRelation(typ string, rel *Resource) {
	res.relations[typ] = append(res.relations[typ], rel)
}

// Compare only the id and type of the resources (no properties nor meta)
func (res *Resource) Same(other cloud.Resource) bool {
	if res == nil && other == nil {
		return true
	}
	if res == nil || other == nil {
		return false
	}
	return res.Id() == other.Id() && res.Type() == other.Type()
}

func (res *Resource) marshalFullRDF() ([]triplestore.Triple, error) {
	var triples []triplestore.Triple

	cloudType := namespacedResourceType(res.Type())
	triples = append(triples, triplestore.SubjPred(res.id, rdf.RdfType).Resource(cloudType))

	for key, value := range res.meta {
		if key == "diff" {
			triples = append(triples, triplestore.SubjPred(res.id, MetaPredicate).StringLiteral(fmt.Sprint(value)))
		}
	}

	for key, value := range res.properties {
		if value == nil {
			continue
		}

		propId, err := rdf.Properties.GetRDFId(key)
		if err != nil {
			return triples, fmt.Errorf("resource %s: marshalling property: %s", res, err)
		}

		propType, err := rdf.Properties.GetDefinedBy(propId)
		if err != nil {
			return triples, fmt.Errorf("resource %s: marshalling property: %s", res, err)
		}
		dataType, err := rdf.Properties.GetDataType(propId)
		if err != nil {
			return triples, fmt.Errorf("resource %s: marshalling property: %s", res, err)
		}
		switch propType {
		case rdf.RdfsLiteral, rdf.RdfsClass:
			obj, err := marshalToRdfObject(value, propType, dataType)
			if err != nil {
				return triples, fmt.Errorf("resource %s: marshalling property '%s': %s", res, key, err)
			}
			triples = append(triples, triplestore.SubjPred(res.Id(), propId).Object(obj))
		case rdf.RdfsList:
			switch dataType {
			case rdf.XsdString:
				list, ok := value.([]string)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a string slice, got a %T", res, key, value)
				}
				for _, l := range list {
					triples = append(triples, triplestore.SubjPred(res.id, propId).StringLiteral(l))
				}
			case rdf.RdfsClass:
				list, ok := value.([]string)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a string slice, got a %T", res, key, value)
				}
				for _, l := range list {
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(l))
				}
			case rdf.NetFirewallRule:
				list, ok := value.([]*FirewallRule)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a firewall rule slice, got a %T", res, key, value)
				}
				for _, r := range list {
					ruleId := randomRdfId()
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(ruleId))
					triples = append(triples, r.marshalToTriples(ruleId)...)
				}
			case rdf.NetRoute:
				list, ok := value.([]*Route)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a route slice, got a %T", res, key, value)
				}
				for _, r := range list {
					routeId := randomRdfId()
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(routeId))
					triples = append(triples, r.marshalToTriples(routeId)...)
				}
			case rdf.Grant:
				list, ok := value.([]*Grant)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a grant slice, got a %T", res, key, value)
				}
				for _, g := range list {
					grantId := randomRdfId()
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(grantId))
					triples = append(triples, g.marshalToTriples(grantId)...)
				}
			case rdf.KeyValue:
				list, ok := value.([]*KeyValue)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a keyvalue slice, got a %T", res, key, value)
				}
				for _, kv := range list {
					keyValId := randomRdfId()
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(keyValId))
					triples = append(triples, kv.marshalToTriples(keyValId)...)
				}
			case rdf.DistributionOrigin:
				list, ok := value.([]*DistributionOrigin)
				if !ok {
					return triples, fmt.Errorf("resource %s: marshalling property '%s': expected a distribution origin slice, got a %T", res, key, value)
				}
				for _, o := range list {
					keyValId := randomRdfId()
					triples = append(triples, triplestore.SubjPred(res.id, propId).Resource(keyValId))
					triples = append(triples, o.marshalToTriples(keyValId)...)
				}
			case rdf.Grant:
			default:
				return triples, fmt.Errorf("resource %s: marshalling property '%s': unexpected rdfs:DataType: %s", res, key, dataType)
			}

		default:
			return triples, fmt.Errorf("resource %s: marshalling property '%s': unexpected rdfs:isDefinedBy: %s", res, key, propType)
		}

	}
	return triples, nil
}

func marshalToRdfObject(i interface{}, definedBy, dataType string) (triplestore.Object, error) {
	switch definedBy {
	case rdf.RdfsLiteral:
		return triplestore.ObjectLiteral(i)
	case rdf.RdfsClass:
		return triplestore.Resource(fmt.Sprint(i)), nil
	default:
		return nil, fmt.Errorf("unexpected rdfs:isDefinedBy: %s", definedBy)
	}
}

func (res *Resource) unmarshalFullRdf(gph triplestore.RDFGraph) error {
	cloudType := namespacedResourceType(res.Type())
	if !gph.Contains(triplestore.SubjPred(res.Id(), rdf.RdfType).Resource(cloudType)) {
		return fmt.Errorf("triple <%s><%s><%s> not found in graph", res.Id(), rdf.RdfType, cloudType)
	}
	// The index this comes out of is built by iterating a Go map
	// (triplestore/source.go), so the order varies from one populated source to
	// the next — per sync, per LoadLocalGraphs, per process. For a subject that
	// carries two types — see resolveResourceTypes — the union below would then
	// resolve a scalar property both types declare differently on each run, and
	// `show --values-for` reads these values straight out.
	//
	// Copy first: WithSubject returns the snapshot's own index slice, with no
	// copy, and unmarshalFullRdf runs from several goroutines against one shared
	// snapshot while the generated services build parent relations, so sorting in
	// place would be two concurrent writers to one array.
	//
	// Triples.Sort orders by the canonical triple key, which for a fixed subject
	// is (predicate, "value"^^<datatype>), so a scalar declared twice keeps the
	// greatest canonical object key and a list arrives in that same fixed order.
	// Sorting the copies concurrently is safe only because source.Add populates
	// every triple's cached key under its write lock before any reader sees it.
	triples := triplestore.Triples(append([]triplestore.Triple(nil), gph.WithSubject(res.Id())...))
	triples.Sort()
	for _, t := range triples {
		pred := t.Predicate()
		if !rdf.Properties.IsRDFProperty(pred) || rdf.Properties.IsRDFSubProperty(pred) {
			continue
		}

		propKey, err := rdf.Properties.GetLabel(pred)
		if err != nil {
			return fmt.Errorf("unmarshalling property: label: %s", err)
		}
		propVal, err := getPropertyValue(gph, t.Object(), pred)
		if err != nil {
			return fmt.Errorf("unmarshalling property '%s' of resource '%s': %s", propKey, res.Id(), err)
		}
		if rdf.Properties.IsRDFList(pred) {
			dataType, err := rdf.Properties.GetDataType(pred)
			if err != nil {
				return fmt.Errorf("unmarshalling property: datatype: %s", err)
			}
			switch dataType {
			case rdf.RdfsClass, rdf.XsdString:
				list, ok := res.properties[propKey].([]string)
				if !ok {
					list = []string{}
				}
				list = append(list, propVal.(string))
				res.properties[propKey] = list
			case rdf.NetFirewallRule:
				list, ok := res.properties[propKey].([]*FirewallRule)
				if !ok {
					list = []*FirewallRule{}
				}
				list = append(list, propVal.(*FirewallRule))
				res.properties[propKey] = list
			case rdf.NetRoute:
				list, ok := res.properties[propKey].([]*Route)
				if !ok {
					list = []*Route{}
				}
				list = append(list, propVal.(*Route))
				res.properties[propKey] = list
			case rdf.Grant:
				list, ok := res.properties[propKey].([]*Grant)
				if !ok {
					list = []*Grant{}
				}
				list = append(list, propVal.(*Grant))
				res.properties[propKey] = list
			case rdf.KeyValue:
				list, ok := res.properties[propKey].([]*KeyValue)
				if !ok {
					list = []*KeyValue{}
				}
				list = append(list, propVal.(*KeyValue))
				res.properties[propKey] = list
			case rdf.DistributionOrigin:
				list, ok := res.properties[propKey].([]*DistributionOrigin)
				if !ok {
					list = []*DistributionOrigin{}
				}
				list = append(list, propVal.(*DistributionOrigin))
				res.properties[propKey] = list
			default:
				return fmt.Errorf("unmarshalling property: unexpected datatype %s", dataType)
			}
		} else {
			res.properties[propKey] = propVal
		}
	}
	return nil
}

func (r *Resource) unmarshalMeta(gph triplestore.RDFGraph) error {
	for _, t := range gph.WithSubjPred(r.Id(), MetaPredicate) {
		text, err := triplestore.ParseString(t.Object())
		if err != nil {
			return err
		}
		r.meta["diff"] = text
	}
	return nil
}

func namespacedResourceType(typ string) string {
	return fmt.Sprintf("%s:%s", rdf.CloudOwlNS, strings.Title(typ))
}

type Resources []*Resource

func (res Resources) Map(f func(*Resource) string) (out []string) {
	for _, r := range res {
		out = append(out, f(r))
	}
	return
}

func Subtract(one, other map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for propK, propV := range one {
		var found bool
		if otherV, ok := other[propK]; ok {
			if reflect.DeepEqual(propV, otherV) {
				found = true
			}
		}
		if !found {
			result[propK] = propV
		}
	}

	return result
}

var errTypeNotFound = errors.New("resource type not found")

// resolveResourceTypes returns every cloud type a subject carries, in
// lexicographic order.
//
// A subject carrying several types is an ordinary consequence of how ids are
// built, not a corrupt graph: the resources AWS names rather than numbers use the
// bare name as their id, and a name is only unique per type — a keypair and a
// classic load balancer can both be called 'prod' — while LoadLocalGraphs merges
// one file per service into a single graph, so ids that only ever collided across
// services collide after the merge.
//
// The order is sorted because the index this reads comes out of a Go map
// (triplestore/source.go), so WithSubjPred returns the type triples in an order
// that varies from one populated source to the next.
func resolveResourceTypes(g triplestore.RDFGraph, id string) ([]string, error) {
	typeTs := g.WithSubjPred(id, rdf.RdfType)
	if len(typeTs) == 0 {
		return nil, errTypeNotFound
	}
	// No dedup: the triple store is a set keyed on the whole triple, so the same
	// rdf:type cannot be here twice.
	types := make([]string, 0, len(typeTs))
	for _, t := range typeTs {
		typ, err := unmarshalResourceType(t.Object())
		if err != nil {
			return nil, err
		}
		types = append(types, typ)
	}
	sort.Strings(types)
	return types, nil
}

// resolveResourceType returns the first of the subject's types. Its signature is
// fixed: graph/visit.go passes it as a value to triplestore.Tree.TraverseSiblings.
func resolveResourceType(g triplestore.RDFGraph, id string) (string, error) {
	types, err := resolveResourceTypes(g, id)
	if err != nil {
		return "", err
	}
	return types[0], nil
}

func lowerFirstLetter(s string) string {
	a := []rune(s)
	a[0] = unicode.ToLower(a[0])
	return string(a)
}

func unmarshalResourceType(obj triplestore.Object) (string, error) {
	node, ok := obj.Resource()
	if !ok {
		return "", fmt.Errorf("object is not a resource identifier, %v", obj)
	}
	return lowerFirstLetter(trimNS(node)), nil
}

func trimNS(s string) string {
	spl := strings.Split(s, ":")
	if len(spl) == 0 {
		return s
	}
	return spl[len(spl)-1]
}
