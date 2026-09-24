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

// Package triples stores RDF triples and reads and writes them as N-Triples.
//
// It replaces github.com/wallix/triplestore, which was abandoned in 2018 along with
// the rest of upstream awless. Only what awless-ro actually uses is here: the binary
// codec, the dot-graph writer, the streaming codecs and ten of the fourteen literal
// types were carried by that module and used by nobody. What remains is roughly a
// quarter of the original.
//
// The wire format is unchanged, so graphs already synced to disk keep loading. Two
// things were fixed rather than reproduced, both described where they occur:
// string literals are now escaped per the specification, and a blank node no longer
// pretends to be a resource.
package triplestore

import (
	"fmt"
	"sort"
	"strings"
)

// XsdType is the datatype of a literal, written after ^^ in N-Triples.
//
// The spellings are the ones already on disk, and some are not what the name
// suggests: a Go int is written as xsd:integer even though the vocabulary in
// cloud/rdf calls the same property xsd:int. They are what they are because files
// in ~/.awless-ro contain them.
type XsdType string

const (
	XsdString   = XsdType("xsd:string")
	XsdBoolean  = XsdType("xsd:boolean")
	XsdDateTime = XsdType("xsd:dateTime")
	XsdInteger  = XsdType("xsd:integer")
	XsdDouble   = XsdType("xsd:double")

	// Nothing in awless-ro writes these any more, but graphs synced by upstream
	// may contain them, so the parser still understands them.
	XsdFloat         = XsdType("xsd:float")
	XsdByte          = XsdType("xsd:byte")
	XsdShort         = XsdType("xsd:short")
	XsdUinteger      = XsdType("xsd:unsignedInt")
	XsdUnsignedByte  = XsdType("xsd:unsignedByte")
	XsdUnsignedShort = XsdType("xsd:unsignedShort")
)

// Triple is a subject, a predicate and an object.
type Triple interface {
	Subject() string
	Predicate() string
	Object() Object
	Equal(Triple) bool

	// key identifies the triple. It is unexported so that this package stays the
	// only implementation, which is what lets the indexes and the deduplicating
	// map rely on it.
	key() string
}

// Object is the third part of a triple: a resource identifier, a literal or a blank
// node.
type Object interface {
	// Literal reports the literal value, if the object is one.
	Literal() (Literal, bool)
	// Resource reports the resource identifier, if the object is one. A blank node
	// is not a resource: upstream answered true here with an empty identifier,
	// which handed callers a blank string they then used as a node name.
	Resource() (string, bool)
	// Bnode reports the blank node identifier, if the object is one.
	Bnode() (string, bool)
	Equal(Object) bool

	key() string
}

// Literal is a value with a datatype, and optionally a language tag.
type Literal interface {
	Type() XsdType
	Value() string
	Lang() string
}

type triple struct {
	sub, pred  string
	isSubBnode bool
	obj        object
	cachedKey  string
}

func (t *triple) Subject() string   { return t.sub }
func (t *triple) Predicate() string { return t.pred }
func (t *triple) Object() Object    { return t.obj }

func (t *triple) key() string {
	if t.cachedKey == "" {
		var sub string
		if t.isSubBnode {
			sub = "_:" + t.sub
		} else {
			sub = "<" + t.sub + ">"
		}
		t.cachedKey = sub + "<" + t.pred + ">" + t.obj.key()
	}
	return t.cachedKey
}

func (t *triple) Equal(other Triple) bool {
	if t == nil || other == nil {
		return t == nil && other == nil
	}
	return t.key() == other.key()
}

func (t *triple) clone() *triple {
	return &triple{sub: t.sub, pred: t.pred, isSubBnode: t.isSubBnode, obj: t.obj, cachedKey: t.cachedKey}
}

func (t *triple) String() string {
	return t.key()
}

type object struct {
	isLit, isBnode  bool
	resource, bnode string
	lit             literal
}

func (o object) Literal() (Literal, bool) { return o.lit, o.isLit }
func (o object) Bnode() (string, bool)    { return o.bnode, o.isBnode }

func (o object) Resource() (string, bool) { return o.resource, !o.isLit && !o.isBnode }

func (o object) key() string {
	switch {
	case o.isLit:
		if o.lit.langtag != "" {
			return `"` + o.lit.val + `"@` + o.lit.langtag
		}
		return `"` + o.lit.val + `"^^<` + string(o.lit.typ) + ">"
	case o.isBnode:
		return "_:" + o.bnode
	default:
		return "<" + o.resource + ">"
	}
}

// Equal compares objects by identity. Upstream returned true for any two blank
// nodes, so distinct nested structs looked interchangeable.
func (o object) Equal(other Object) bool {
	if other == nil {
		return false
	}
	return o.key() == other.key()
}

type literal struct {
	typ          XsdType
	val, langtag string
}

func (l literal) Type() XsdType { return l.typ }
func (l literal) Value() string { return l.val }
func (l literal) Lang() string  { return l.langtag }

// Triples is a set of triples with order-insensitive comparison, which is what
// tests of graph contents need.
type Triples []Triple

func (ts Triples) Equal(others Triples) bool {
	if len(ts) != len(others) {
		return false
	}
	mine := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		mine[t.key()] = struct{}{}
	}
	for _, t := range others {
		if _, ok := mine[t.key()]; !ok {
			return false
		}
	}
	return true
}

func (ts Triples) Sort() {
	sort.Slice(ts, func(i, j int) bool { return ts[i].key() < ts[j].key() })
}

func (ts Triples) Map(fn func(Triple) string) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, fn(t))
	}
	return out
}

func (ts Triples) String() string {
	return "[" + strings.Join(ts.Map(func(t Triple) string { return fmt.Sprint(t) }), "\n") + "]"
}
