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
	"fmt"
	"strconv"
	"time"
)

// Builder collects a subject and a predicate; one of its methods then supplies the
// object and returns the finished triple.
type Builder struct {
	sub, pred  string
	isSubBnode bool
	langtag    string
}

// SubjPred starts a triple.
func SubjPred(s, p string) *Builder {
	return &Builder{sub: s, pred: p}
}

// BnodePred starts a triple whose subject is a blank node.
func BnodePred(s, p string) *Builder {
	return &Builder{sub: s, pred: p, isSubBnode: true}
}

// Lang sets the language tag for a following string literal.
func (b *Builder) Lang(l string) *Builder {
	b.langtag = l
	return b
}

func (b *Builder) build(o object) Triple {
	return &triple{sub: b.sub, pred: b.pred, isSubBnode: b.isSubBnode, obj: o}
}

// Resource finishes the triple with a resource identifier as the object.
func (b *Builder) Resource(s string) Triple {
	return b.build(object{resource: s})
}

// Bnode finishes the triple with a blank node as the object.
func (b *Builder) Bnode(s string) Triple {
	return b.build(object{isBnode: true, bnode: s})
}

// Object finishes the triple with an already-built object.
func (b *Builder) Object(o Object) Triple {
	obj, ok := o.(object)
	if !ok {
		// Object carries an unexported method, so nothing outside this package can
		// supply another implementation; reaching here means a nil interface.
		return b.build(object{})
	}
	return b.build(obj)
}

// StringLiteral finishes the triple with a plain string literal.
func (b *Builder) StringLiteral(s string) Triple {
	return b.build(object{isLit: true, lit: literal{typ: XsdString, val: s, langtag: b.langtag}})
}

// Resource builds a resource identifier object, for use with WithPredObj and
// Builder.Object.
func Resource(s string) Object {
	return object{resource: s}
}

// Bnode builds a blank node object.
func Bnode(s string) Object {
	return object{isBnode: true, bnode: s}
}

// StringLiteral builds a plain string literal object.
func StringLiteral(s string) Object {
	return object{isLit: true, lit: literal{typ: XsdString, val: s}}
}

// UnsupportedLiteralTypeError says a Go value has no literal representation here.
// It is an error rather than a silent skip: a property that cannot be stored must
// not look like a property that was absent.
type UnsupportedLiteralTypeError struct {
	Value interface{}
}

func (e UnsupportedLiteralTypeError) Error() string {
	return fmt.Sprintf("triples: no literal representation for %T", e.Value)
}

// ObjectLiteral turns a Go value into a literal object.
//
// The accepted set is deliberately narrow, matching what the AWS conversion layer
// produces: it normalises SDK values down to string, bool, the integer kinds,
// float64 and time.Time before they get here. fmt.Stringer is accepted because
// several stored types, such as net.IPNet and PortRange, are represented by their
// text form.
func ObjectLiteral(i interface{}) (Object, error) {
	switch v := i.(type) {
	case string:
		return StringLiteral(v), nil
	case bool:
		return BooleanLiteral(v), nil
	case int:
		return IntegerLiteral(int64(v)), nil
	case int8:
		return IntegerLiteral(int64(v)), nil
	case int16:
		return IntegerLiteral(int64(v)), nil
	case int32:
		return IntegerLiteral(int64(v)), nil
	case int64:
		return IntegerLiteral(v), nil
	case uint:
		return IntegerLiteral(int64(v)), nil
	case uint8:
		return IntegerLiteral(int64(v)), nil
	case uint16:
		return IntegerLiteral(int64(v)), nil
	case uint32:
		return IntegerLiteral(int64(v)), nil
	case uint64:
		return IntegerLiteral(int64(v)), nil
	case float32:
		return DoubleLiteral(float64(v)), nil
	case float64:
		return DoubleLiteral(v), nil
	case time.Time:
		return DateTimeLiteral(v), nil
	case *time.Time:
		if v == nil {
			return nil, UnsupportedLiteralTypeError{i}
		}
		return DateTimeLiteral(*v), nil
	case fmt.Stringer:
		return StringLiteral(v.String()), nil
	default:
		return nil, UnsupportedLiteralTypeError{i}
	}
}

// ParseLiteral turns a literal object back into a Go value.
//
// Integers come back as int and floats as float64, whatever width was written. The
// width was never meaningful: these are counts, ports and timeouts, and the type
// the caller sees has to stay int because that is what the resource comparisons and
// the display layer were built against. On the platforms this ships for, int is 64
// bits, so nothing is narrowed.
func ParseLiteral(obj Object) (interface{}, error) {
	lit, ok := obj.Literal()
	if !ok {
		return nil, fmt.Errorf("triples: object is not a literal")
	}
	switch lit.Type() {
	case XsdString:
		return lit.Value(), nil
	case XsdBoolean:
		return strconv.ParseBool(lit.Value())
	case XsdDateTime:
		var t time.Time
		err := t.UnmarshalText([]byte(lit.Value()))
		return t, err
	case XsdInteger, XsdByte, XsdShort, XsdUinteger, XsdUnsignedByte, XsdUnsignedShort:
		return strconv.Atoi(lit.Value())
	case XsdDouble, XsdFloat:
		return strconv.ParseFloat(lit.Value(), 64)
	default:
		return nil, fmt.Errorf("triples: unknown literal type %q", lit.Type())
	}
}

// ParseString reports the text of a string literal.
func ParseString(obj Object) (string, error) {
	lit, ok := obj.Literal()
	if !ok {
		return "", fmt.Errorf("triples: object is not a literal")
	}
	if lit.Type() != XsdString {
		return "", fmt.Errorf("triples: literal is a %s, not a string", lit.Type())
	}
	return lit.Value(), nil
}

func BooleanLiteral(b bool) Object {
	return object{isLit: true, lit: literal{typ: XsdBoolean, val: strconv.FormatBool(b)}}
}

func IntegerLiteral(i int64) Object {
	return object{isLit: true, lit: literal{typ: XsdInteger, val: strconv.FormatInt(i, 10)}}
}

func DoubleLiteral(f float64) Object {
	return object{isLit: true, lit: literal{typ: XsdDouble, val: strconv.FormatFloat(f, 'g', -1, 64)}}
}

// DateTimeLiteral stores a timestamp in UTC.
//
// The layout is RFC3339Nano, which is what time.Time.MarshalText produces and
// therefore what is already on disk. Plain RFC3339 would be wrong: it truncates the
// fractional second, and AWS timestamps carry one.
func DateTimeLiteral(t time.Time) Object {
	return object{isLit: true, lit: literal{typ: XsdDateTime, val: t.UTC().Format(time.RFC3339Nano)}}
}

func (b *Builder) BooleanLiteral(v bool) Triple  { return b.Object(BooleanLiteral(v)) }
func (b *Builder) IntegerLiteral(v int64) Triple { return b.Object(IntegerLiteral(v)) }
func (b *Builder) DoubleLiteral(v float64) Triple {
	return b.Object(DoubleLiteral(v))
}
func (b *Builder) DateTimeLiteral(v time.Time) Triple { return b.Object(DateTimeLiteral(v)) }
