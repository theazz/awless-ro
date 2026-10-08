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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// N-Triples, one triple per line:
//
//	<subject> <predicate> <object> .
//
// with the subject or object optionally a blank node written _:name, and the object
// optionally a literal written "text", "text"^^<xsd:type> or "text"@lang.
//
// This is the format of every file under ~/.awless-ro/aws/rdf, so the reader has to
// accept what earlier versions wrote, which was not quite this. See unescapeLiteral
// and parseObject.

// Encoder writes triples as N-Triples.
type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

func (e *Encoder) Encode(triples ...Triple) error {
	var buf bytes.Buffer
	for _, t := range triples {
		encodeTriple(t, &buf)
	}
	_, err := e.w.Write(buf.Bytes())
	return err
}

func encodeTriple(t Triple, buf *bytes.Buffer) {
	if tt, ok := t.(*triple); ok && tt.isSubBnode {
		buf.WriteString("_:" + tt.sub)
	} else {
		buf.WriteString("<" + t.Subject() + ">")
	}
	buf.WriteString(" <" + t.Predicate() + "> ")

	obj := t.Object()
	switch {
	case isBnodeObject(obj):
		bnode, _ := obj.Bnode()
		buf.WriteString("_:" + bnode)
	case isLiteralObject(obj):
		lit, _ := obj.Literal()
		switch {
		case lit.Lang() != "":
			buf.WriteString(`"` + escapeLiteral(lit.Value()) + `"@` + lit.Lang())
		case lit.Type() == XsdString:
			// A plain string carries no datatype, as the specification says.
			buf.WriteString(`"` + escapeLiteral(lit.Value()) + `"`)
		default:
			// Numbers and timestamps have nothing in them that needs escaping.
			buf.WriteString(`"` + lit.Value() + `"^^<` + string(lit.Type()) + ">")
		}
	default:
		res, _ := obj.Resource()
		buf.WriteString("<" + res + ">")
	}

	buf.WriteString(" .\n")
}

func isBnodeObject(o Object) bool {
	_, ok := o.Bnode()
	return ok
}

func isLiteralObject(o Object) bool {
	_, ok := o.Literal()
	return ok
}

// escapeLiteral escapes what N-Triples requires inside a quoted literal.
//
// Earlier versions escaped only the newline and the carriage return, leaving the
// backslash and the double quote raw. That lost information: a real newline became
// the two characters \n while an existing backslash-n stayed the two characters \n,
// so on the way back the reader could not tell them apart and rewrote one into the
// other. Policy documents are JSON, and JSON writes an embedded newline as
// backslash-n, so a document carrying one came back invalid. Escaping the backslash
// is what makes the two distinguishable.
var literalEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
)

func escapeLiteral(s string) string { return literalEscaper.Replace(s) }

// unescapeLiteral reverses escapeLiteral, and also reads what earlier versions
// wrote.
//
// A backslash that does not begin a recognised escape is kept as a backslash rather
// than dropped, which is what lets a path like C:\path out of an older file come
// back unchanged. The one case that cannot be recovered is a backslash-n written by
// an older version: it was already ambiguous on disk.
func unescapeLiteral(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// Decoder reads N-Triples.
type Decoder struct {
	r io.Reader
}

func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: r} }

// maxLineBytes caps a single N-Triples line. The format imposes no limit on a line,
// and a literal can hold a whole IAM policy document, so the real bound is available
// memory rather than any fixed ceiling (this is what lifts the old 8 MiB cap). The
// value is only a corruption guard: a single line past 256 MiB is almost certainly a
// file that is not N-Triples at all — one giant "line" — and a diagnosable error
// beats being killed by the OOM killer. It is a var, not a const, so a test can lower
// it instead of allocating 256 MiB.
var maxLineBytes = 256 << 20

// Decode reads every triple from the underlying reader.
//
// A literal can be arbitrarily large — a whole IAM policy document, say — so lines
// are read with a growing buffer rather than the fixed-size bufio.Scanner that
// capped a line at 8 MiB. parseLine still needs the whole line in memory (parseObject
// scans backwards, see its comment), so the accumulated buffer is memory the parser
// needs anyway.
func (d *Decoder) Decode() ([]Triple, error) {
	var out []Triple

	r := bufio.NewReader(d.r)
	var buf []byte
	for line := 1; ; line++ {
		buf = buf[:0]
		eof := false
		for {
			frag, err := r.ReadSlice('\n')
			if len(buf)+len(frag) > maxLineBytes {
				return out, fmt.Errorf("line %d: single line exceeds %d bytes; is this an N-Triples file?", line, maxLineBytes)
			}
			buf = append(buf, frag...)
			if err == nil {
				break // ReadSlice stopped at the delimiter.
			}
			if errors.Is(err, bufio.ErrBufferFull) {
				continue // Line longer than the reader's buffer; keep accumulating.
			}
			if errors.Is(err, io.EOF) {
				eof = true
				break
			}
			return out, fmt.Errorf("line %d: %s", line, err)
		}

		if len(buf) == 0 && eof {
			break // Trailing newline: nothing left to read.
		}

		// Drop the line terminator ReadSlice left on, tolerating CRLF the way
		// bufio.ScanLines did (parseObject requires the line to end with '.', so a
		// stray '\r' would break every CRLF file).
		text := buf
		if n := len(text); n > 0 && text[n-1] == '\n' {
			text = text[:n-1]
		}
		if n := len(text); n > 0 && text[n-1] == '\r' {
			text = text[:n-1]
		}
		trimmed := strings.TrimLeft(string(text), " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if eof {
				break
			}
			continue
		}
		t, err := parseLine(trimmed)
		if err != nil {
			return out, fmt.Errorf("line %d: %s", line, err)
		}
		out = append(out, t)
		if eof {
			break
		}
	}

	return out, nil
}

func parseLine(line string) (Triple, error) {
	subject, isBnode, rest, err := parseNode(line)
	if err != nil {
		return nil, fmt.Errorf("subject: %s", err)
	}

	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "<") {
		return nil, fmt.Errorf("predicate: expected <iri>, got %q", truncate(rest))
	}
	end := strings.IndexByte(rest, '>')
	if end < 0 {
		return nil, fmt.Errorf("predicate: unterminated <iri> in %q", truncate(rest))
	}
	predicate := rest[1:end]
	rest = strings.TrimLeft(rest[end+1:], " \t")

	obj, err := parseObject(rest)
	if err != nil {
		return nil, err
	}

	b := SubjPred(subject, predicate)
	b.isSubBnode = isBnode
	return b.Object(obj), nil
}

// parseNode reads a subject, which is either <iri> or _:name.
func parseNode(s string) (node string, isBnode bool, rest string, err error) {
	switch {
	case strings.HasPrefix(s, "_:"):
		s = s[2:]
		end := strings.IndexAny(s, " \t")
		if end < 0 {
			return "", false, "", fmt.Errorf("unterminated blank node in %q", truncate(s))
		}
		return s[:end], true, s[end:], nil
	case strings.HasPrefix(s, "<"):
		end := strings.IndexByte(s, '>')
		if end < 0 {
			return "", false, "", fmt.Errorf("unterminated <iri> in %q", truncate(s))
		}
		return s[1:end], false, s[end+1:], nil
	default:
		return "", false, "", fmt.Errorf("expected <iri> or _:name, got %q", truncate(s))
	}
}

// parseObject reads the object together with the trailing " ." terminator.
//
// It works backwards from the end of the line. Scanning forwards would mean finding
// where a quoted literal ends, and files written by earlier versions contain raw,
// unescaped double quotes inside literals — every JSON policy document does — so
// there is no reliable closing quote to find from the left. The end of the line is
// unambiguous: the terminating dot, before it the object, and a typed literal's
// datatype is the last <...> on the line.
func parseObject(s string) (Object, error) {
	s = strings.TrimRight(s, " \t")
	if !strings.HasSuffix(s, ".") {
		return nil, fmt.Errorf("object: line does not end with '.': %q", truncate(s))
	}
	s = strings.TrimRight(s[:len(s)-1], " \t")
	if s == "" {
		return nil, fmt.Errorf("object: missing")
	}

	switch {
	case strings.HasPrefix(s, "_:"):
		return Bnode(s[2:]), nil

	case strings.HasSuffix(s, ">"):
		open := strings.LastIndexByte(s, '<')
		if open < 0 {
			return nil, fmt.Errorf("object: unterminated <iri> in %q", truncate(s))
		}
		iri := s[open+1 : len(s)-1]
		before := s[:open]

		if !strings.HasSuffix(before, "^^") {
			if before != "" {
				return nil, fmt.Errorf("object: unexpected %q before <iri>", truncate(before))
			}
			return Resource(iri), nil
		}

		value, err := quotedValue(strings.TrimSuffix(before, "^^"))
		if err != nil {
			return nil, err
		}
		// The value of a typed literal is never escaped on the way out, but
		// unescaping it is harmless and keeps older files readable.
		return object{isLit: true, lit: literal{typ: XsdType(iri), val: unescapeLiteral(value)}}, nil

	case strings.HasSuffix(s, `"`):
		value, err := quotedValue(s)
		if err != nil {
			return nil, err
		}
		return StringLiteral(unescapeLiteral(value)), nil

	default:
		// "text"@lang — the language tag has no quotes, so the literal is
		// everything up to the last quote.
		at := strings.LastIndexByte(s, '@')
		if at < 0 {
			return nil, fmt.Errorf("object: expected <iri>, _:name or a quoted literal, got %q", truncate(s))
		}
		value, err := quotedValue(strings.TrimRight(s[:at], " \t"))
		if err != nil {
			return nil, err
		}
		return object{isLit: true, lit: literal{typ: XsdString, val: unescapeLiteral(value), langtag: s[at+1:]}}, nil
	}
}

// quotedValue strips the surrounding quotes from a literal. The closing quote is the
// last character and the opening quote the first, precisely so that anything in
// between, escaped or not, is left alone.
func quotedValue(s string) (string, error) {
	if len(s) < 2 || !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return "", fmt.Errorf("object: expected a quoted literal, got %q", truncate(s))
	}
	return s[1 : len(s)-1], nil
}

func truncate(s string) string {
	const max = 60
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// NewMultiDecoder reads several sources concurrently and concatenates the result.
// The graph for a profile is split over one file per service, and they are
// independent.
func NewMultiDecoder(readers ...io.Reader) *MultiDecoder {
	return &MultiDecoder{readers: readers}
}

type MultiDecoder struct {
	readers []io.Reader
}

func (d *MultiDecoder) Decode() ([]Triple, error) {
	type result struct {
		triples []Triple
		err     error
		reader  io.Reader
	}

	results := make([]result, len(d.readers))
	var wg sync.WaitGroup
	for i, r := range d.readers {
		wg.Add(1)
		go func(i int, r io.Reader) {
			defer wg.Done()
			triples, err := NewDecoder(r).Decode()
			results[i] = result{triples: triples, err: err, reader: r}
		}(i, r)
	}
	wg.Wait()

	// Results are collected in the order given rather than as they arrive, so that
	// the same set of files always produces the same error and the same triples.
	// Every failing reader is reported, not just the first: one unreadable service
	// file must not hide another, and the caller decides what a partial read means.
	var all []Triple
	var errs []error
	for _, res := range results {
		if res.err != nil {
			if f, ok := res.reader.(*os.File); ok {
				errs = append(errs, fmt.Errorf("file %q: %w", f.Name(), res.err))
			} else {
				errs = append(errs, res.err)
			}
			continue
		}
		all = append(all, res.triples...)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return all, nil
}
