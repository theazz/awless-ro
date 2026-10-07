package graph

import (
	"fmt"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/cloud/rdf"
	"github.com/theazz/awless-ro/triplestore"
)

type Resolver interface {
	Resolve(snap triplestore.RDFGraph) ([]*Resource, error)
}

type ById struct {
	Id string
}

func (r *ById) Resolve(snap triplestore.RDFGraph) ([]*Resource, error) {
	resolver := &ByProperty{Key: properties.ID, Value: r.Id}
	return resolver.Resolve(snap)
}

type ByTypeAndProperty struct {
	Type  string
	Key   string
	Value interface{}
}

func (r *ByTypeAndProperty) Resolve(snap triplestore.RDFGraph) ([]*Resource, error) {
	var resources []*Resource

	if r.Value == nil {
		return resources, nil
	}
	rdfpropLabel, ok := rdf.Labels[r.Key]
	if !ok {
		return resources, fmt.Errorf("resolve by property: undefined property label '%s'", r.Key)
	}
	rdfProp, err := rdf.Properties.Get(rdfpropLabel)
	if err != nil {
		return resources, fmt.Errorf("resolve by property: %s", err)
	}
	obj, err := marshalToRdfObject(r.Value, rdfProp.RdfsDefinedBy, rdfProp.RdfsDataType)
	if err != nil {
		return resources, fmt.Errorf("resolve by property: unmarshaling property '%s': %s", r.Key, err)
	}
	for _, t := range snap.WithPredObj(rdfpropLabel, obj) {
		// A subject may carry several types — see resolveResourceTypes — so keep
		// it when the asked-for type is among them rather than comparing against
		// a single resolved type.
		types, err := resolveResourceTypes(snap, t.Subject())
		if err != nil {
			return resources, err
		}
		for _, rt := range types {
			if rt != r.Type {
				continue
			}
			res := InitResource(rt, t.Subject())

			if err := res.unmarshalFullRdf(snap); err != nil {
				return resources, err
			}
			resources = append(resources, res)
		}
	}
	return resources, nil
}

type ByProperty struct {
	Key   string
	Value interface{}
}

func (r *ByProperty) Resolve(snap triplestore.RDFGraph) ([]*Resource, error) {
	var resources []*Resource
	if r.Value == nil {
		return resources, nil
	}
	rdfpropLabel, ok := rdf.Labels[r.Key]
	if !ok {
		return resources, fmt.Errorf("resolve by property: undefined property label '%s'", r.Key)
	}
	rdfProp, err := rdf.Properties.Get(rdfpropLabel)
	if err != nil {
		return resources, fmt.Errorf("resolve by property: %s", err)
	}
	obj, err := marshalToRdfObject(r.Value, rdfProp.RdfsDefinedBy, rdfProp.RdfsDataType)
	if err != nil {
		return resources, fmt.Errorf("resolve by property: unmarshaling property '%s': %s", r.Key, err)
	}
	for _, t := range snap.WithPredObj(rdfpropLabel, obj) {
		// One resource per type the subject carries — see resolveResourceTypes.
		// Each carries the union of the subject's properties, which the per-type
		// column definitions then project back down.
		types, err := resolveResourceTypes(snap, t.Subject())
		if err != nil {
			return resources, err
		}
		for _, rt := range types {
			res := InitResource(rt, t.Subject())

			if err := res.unmarshalFullRdf(snap); err != nil {
				return resources, err
			}
			resources = append(resources, res)
		}
	}
	return resources, nil
}

type And struct {
	Resolvers []Resolver
}

func (r *And) Resolve(snap triplestore.RDFGraph) (result []*Resource, err error) {
	if len(r.Resolvers) == 0 {
		return
	}
	result, err = r.Resolvers[0].Resolve(snap)
	if err != nil {
		return
	}
	gg := NewGraph()
	if err = gg.AddResource(result...); err != nil {
		return
	}
	for _, resolv := range r.Resolvers {
		result, err = resolv.Resolve(gg.store.Snapshot())
		if err != nil {
			return
		}
		gg = NewGraph()
		if err = gg.AddResource(result...); err != nil {
			return
		}
	}
	return
}

type Or struct {
	Resolvers []Resolver
}

func (r *Or) Resolve(snap triplestore.RDFGraph) (result []*Resource, err error) {
	for _, resolv := range r.Resolvers {
		result, err = resolv.Resolve(snap)
		if err != nil {
			return
		}
		if len(result) > 0 {
			return
		}
	}
	return
}

type ByType struct {
	Typ string
}

func (r *ByType) Resolve(snap triplestore.RDFGraph) ([]*Resource, error) {
	var resources []*Resource
	typ := namespacedResourceType(r.Typ)
	for _, t := range snap.WithPredObj(rdf.RdfType, triplestore.Resource(typ)) {
		r := InitResource(r.Typ, t.Subject())
		err := r.unmarshalFullRdf(snap)
		if err != nil {
			return resources, err
		}
		resources = append(resources, r)
	}
	return resources, nil
}

type ByTypes struct {
	Typs []string
}

func (r *ByTypes) Resolve(snap triplestore.RDFGraph) ([]*Resource, error) {
	var res []*Resource
	for _, t := range r.Typs {
		bt := &ByType{t}
		all, err := bt.Resolve(snap)
		if err != nil {
			return res, err
		}
		res = append(res, all...)
	}

	return res, nil
}
