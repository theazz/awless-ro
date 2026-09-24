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

package triplestore

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"reflect"
)

const (
	predicateTag = "predicate"
	bnodeTag     = "bnode"
)

// TriplesFromStruct turns a struct into triples, one per tagged field.
//
//	Field string `predicate:"cloud:name"`
//
// A slice field yields one triple per element. A nested struct tagged bnode becomes
// a blank node of its own, linked from the parent by the field's predicate; that is
// how a Grant refers to its Grantee. Fields without a predicate tag, and values with
// no literal representation, are skipped.
//
// The subject is a blank node when isBnode is set, which is how nesting recurses.
func TriplesFromStruct(subject string, i interface{}, isBnode ...bool) []Triple {
	var asBnode bool
	if len(isBnode) > 0 {
		asBnode = isBnode[0]
	}

	val, ok := structValue(reflect.ValueOf(i))
	if !ok {
		return nil
	}

	var out []Triple
	typ := val.Type()

	for idx := 0; idx < typ.NumField(); idx++ {
		field, fieldVal := typ.Field(idx), val.Field(idx)
		if !fieldVal.CanInterface() {
			continue
		}
		if fieldVal.Kind() == reflect.Ptr && fieldVal.IsNil() {
			continue
		}

		predicate := field.Tag.Get(predicateTag)

		if t, ok := tripleFromValue(subject, predicate, fieldVal, asBnode); ok {
			out = append(out, t)
		}

		if bnodeID, nested := field.Tag.Lookup(bnodeTag); nested {
			if nestedVal, isStruct := structValue(fieldVal); isStruct {
				if bnodeID == "" {
					bnodeID = newBnodeID()
				}
				out = append(out, TriplesFromStruct(bnodeID, nestedVal.Interface(), true)...)
				if predicate != "" {
					out = append(out, SubjPred(subject, predicate).Bnode(bnodeID))
				}
				continue
			}
		}

		if fieldVal.Kind() == reflect.Slice {
			for e := 0; e < fieldVal.Len(); e++ {
				if t, ok := tripleFromValue(subject, predicate, fieldVal.Index(e), asBnode); ok {
					out = append(out, t)
				}
			}
		}
	}

	return out
}

func tripleFromValue(subject, predicate string, v reflect.Value, asBnode bool) (Triple, bool) {
	if predicate == "" || !v.CanInterface() {
		return nil, false
	}
	obj, err := ObjectLiteral(v.Interface())
	if err != nil {
		return nil, false
	}
	if asBnode {
		return BnodePred(subject, predicate).Object(obj), true
	}
	return SubjPred(subject, predicate).Object(obj), true
}

func structValue(v reflect.Value) (reflect.Value, bool) {
	switch v.Kind() {
	case reflect.Struct:
		return v, true
	case reflect.Ptr:
		if !v.IsNil() && v.Elem().Kind() == reflect.Struct {
			return v.Elem(), true
		}
	}
	return v, false
}

// newBnodeID names a nested node. The identifier only has to be unique within the
// graph being built, so eight hex digits is enough; it comes from crypto/rand
// because math/rand's global source is shared with whatever else the process is
// doing, and a collision here silently merges two nodes.
func newBnodeID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("triples: cannot generate a blank node identifier: %w", err))
	}
	return fmt.Sprintf("%x", binary.BigEndian.Uint32(b[:]))
}
