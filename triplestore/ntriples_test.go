package triplestore

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func encodeOne(t *testing.T, tri Triple) string {
	t.Helper()
	var buf bytes.Buffer
	if err := NewEncoder(&buf).Encode(tri); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func decodeOne(t *testing.T, nt string) Triple {
	t.Helper()
	got, err := NewDecoder(strings.NewReader(nt)).Decode()
	if err != nil {
		t.Fatalf("decode %q: %s", nt, err)
	}
	if len(got) != 1 {
		t.Fatalf("decode %q: got %d triples, want 1", nt, len(got))
	}
	return got[0]
}

// The string shapes that escaping has to survive. This is the test that matters
// most: every one of these can appear in an IAM policy document or in UserData, and
// a value that does not come back exactly is silent data loss.
func TestStringLiteralsRoundTrip(t *testing.T) {
	values := []string{
		"",
		"plain",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*"}]}`,
		"line1\nline2",
		"a\rb",
		"a\tb",
		`C:\path\to`,
		`a \n b`,
		`ends with backslash \`,
		`"leading and trailing quotes"`,
		`value"`,
		`value" .`,
		`value"@en`,
		`value"^^<xsd:string>`,
		"trailing dot .",
		"_:looks-like-a-bnode",
		"<looks-like-an-iri>",
		"привет 世界 🙂",
		strings.Repeat("long ", 20000),
	}

	for _, value := range values {
		name := value
		if len(name) > 40 {
			name = name[:40] + "..."
		}
		t.Run(name, func(t *testing.T) {
			nt := encodeOne(t, SubjPred("s", "p").StringLiteral(value))
			if strings.Count(nt, "\n") != 1 {
				t.Fatalf("a literal must not break the line: %q", nt)
			}

			got := decodeOne(t, nt)
			lit, ok := got.Object().Literal()
			if !ok {
				t.Fatalf("object is not a literal: %q", nt)
			}
			if lit.Value() != value {
				t.Errorf("value changed\n  in:  %q\n  out: %q\n  nt:  %q", value, lit.Value(), nt)
			}
		})
	}
}

func TestTypedLiteralsRoundTrip(t *testing.T) {
	stamp := time.Date(2023, 11, 14, 22, 13, 20, 123456789, time.UTC)

	cases := []struct {
		name string
		obj  Object
		want interface{}
	}{
		{"true", BooleanLiteral(true), true},
		{"false", BooleanLiteral(false), false},
		// ParseLiteral gives back an int whatever width went in; see its comment.
		{"zero", IntegerLiteral(0), 0},
		{"negative", IntegerLiteral(-42), -42},
		{"large", IntegerLiteral(1 << 62), 1 << 62},
		{"double", DoubleLiteral(1.5), 1.5},
		{"timestamp with nanoseconds", DateTimeLiteral(stamp), stamp},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nt := encodeOne(t, SubjPred("s", "p").Object(tc.obj))
			got := decodeOne(t, nt)

			value, err := ParseLiteral(got.Object())
			if err != nil {
				t.Fatalf("%q: %s", nt, err)
			}
			if stampWant, ok := tc.want.(time.Time); ok {
				stampGot, ok := value.(time.Time)
				if !ok {
					t.Fatalf("got %T, want time.Time", value)
				}
				if !stampGot.Equal(stampWant) {
					t.Errorf("timestamp changed\n  in:  %s\n  out: %s\n  nt:  %q", stampWant, stampGot, nt)
				}
				return
			}
			if value != tc.want {
				t.Errorf("value changed\n  in:  %#v\n  out: %#v\n  nt:  %q", tc.want, value, nt)
			}
		})
	}
}

// A timestamp is written in UTC whatever zone it arrives in, because the graph has
// no way to record the zone and two spellings of one instant would be two triples.
func TestDateTimeIsNormalisedToUTC(t *testing.T) {
	zone := time.FixedZone("UTC+3", 3*60*60)
	local := time.Date(2023, 11, 14, 22, 13, 20, 0, zone)

	lit, _ := DateTimeLiteral(local).Literal()
	if got, want := lit.Value(), "2023-11-14T19:13:20Z"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBlankNodesRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		tri  Triple
	}{
		{"bnode object", SubjPred("s", "p").Bnode("b1")},
		{"bnode subject", BnodePred("b1", "p").StringLiteral("v")},
		{"bnode both", BnodePred("b1", "p").Bnode("b2")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nt := encodeOne(t, tc.tri)
			got := decodeOne(t, nt)
			if !got.Equal(tc.tri) {
				t.Errorf("triple changed\n  in:  %s\n  out: %s\n  nt:  %q", tc.tri, got, nt)
			}
		})
	}
}

// A blank node is not a resource. Upstream answered true here with an empty
// identifier, so a caller that asked for a resource and got one used "" as a node
// name.
func TestBlankNodeIsNotAResource(t *testing.T) {
	obj := Bnode("b1")

	if _, ok := obj.Resource(); ok {
		t.Error("Resource() claimed success for a blank node")
	}
	if bnode, ok := obj.Bnode(); !ok || bnode != "b1" {
		t.Errorf("Bnode() = %q, %v; want \"b1\", true", bnode, ok)
	}
}

// Two blank nodes are different nodes. Upstream compared them equal, so distinct
// nested structs looked interchangeable.
func TestDistinctBlankNodesAreNotEqual(t *testing.T) {
	if Bnode("b1").Equal(Bnode("b2")) {
		t.Error("two different blank nodes compared equal")
	}
	if !Bnode("b1").Equal(Bnode("b1")) {
		t.Error("a blank node did not compare equal to itself")
	}
}

// Files written before literals were escaped properly must keep loading: raw double
// quotes inside a literal, a raw tab, and a backslash that begins no escape.
func TestReadsTheOlderUnescapedFormat(t *testing.T) {
	const old = `<pol> <rdf:type> <cloud-owl:Policy> .
<pol> <cloud:document> "{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}" .
<pol> <cloud:note> "tabbed	here and "quoted" too" .
<pol> <cloud:path> "C:\path\to" .
<pol> <cloud:count> "3"^^<xsd:integer> .
<pol> <cloud:when> "2023-11-14T22:13:20Z"^^<xsd:dateTime> .
<pol> <cloud:flag> "true"^^<xsd:boolean> .
# a comment
  <pol> <cloud:indented> "leading whitespace is allowed" .
`

	got, err := NewDecoder(strings.NewReader(old)).Decode()
	if err != nil {
		t.Fatal(err)
	}

	byPredicate := make(map[string]Triple, len(got))
	for _, tri := range got {
		byPredicate[tri.Predicate()] = tri
	}

	literal := func(pred string) string {
		t.Helper()
		tri, ok := byPredicate[pred]
		if !ok {
			t.Fatalf("predicate %q missing", pred)
		}
		lit, ok := tri.Object().Literal()
		if !ok {
			t.Fatalf("predicate %q is not a literal", pred)
		}
		return lit.Value()
	}

	if got, want := literal("cloud:document"), `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`; got != want {
		t.Errorf("policy document\n  got  %q\n  want %q", got, want)
	}
	if got, want := literal("cloud:note"), "tabbed\there and \"quoted\" too"; got != want {
		t.Errorf("note\n  got  %q\n  want %q", got, want)
	}
	// \p begins no escape in either generation of the format, so it survives
	// either way. \t is the ambiguous case, covered by the test below.
	if got, want := literal("cloud:path"), `C:\path`+"\to"; got != want {
		t.Errorf("a backslash beginning no escape must survive\n  got  %q\n  want %q", got, want)
	}
	if got, want := literal("cloud:count"), "3"; got != want {
		t.Errorf("integer\n  got  %q\n  want %q", got, want)
	}
	if got, want := literal("cloud:indented"), "leading whitespace is allowed"; got != want {
		t.Errorf("indented line\n  got  %q\n  want %q", got, want)
	}

	if _, ok := byPredicate["rdf:type"].Object().Resource(); !ok {
		t.Error("rdf:type object should be a resource")
	}
}

// \t, \" and \\ are read the way this package writes them, and that reading is not
// the one an older file meant.
//
// The two generations of the format cannot both be honoured. Earlier versions
// escaped only the newline and the carriage return, so in a file they wrote, \t
// could only ever have been a literal backslash followed by a t. Here a literal
// backslash is always doubled, so \t is a tab and a literal backslash-t is written
// \\t. There is no marker in the file to tell the two apart.
//
// Resolved in favour of the current format, on two grounds: it is the one the
// specification describes, and a graph is rewritten completely by the next `sync`,
// which is the command users run most. What is lost is a literal backslash followed
// by t, " or \ inside a string in a graph that has not been re-synced yet.
func TestAmbiguousEscapesFavourTheCurrentFormat(t *testing.T) {
	cases := []struct {
		name, line, want string
	}{
		{"backslash t is a tab", `<s> <p> "a\tb" .`, "a\tb"},
		{"backslash quote is a quote", `<s> <p> "a\"b" .`, `a"b`},
		{"doubled backslash is one backslash", `<s> <p> "a\\b" .`, `a\b`},
		{"doubled then n is a literal backslash n", `<s> <p> "a\\nb" .`, `a\nb`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lit, ok := decodeOne(t, tc.line+"\n").Object().Literal()
			if !ok {
				t.Fatalf("not a literal: %q", tc.line)
			}
			if lit.Value() != tc.want {
				t.Errorf("got %q, want %q", lit.Value(), tc.want)
			}
		})
	}
}

func TestLanguageTaggedLiteral(t *testing.T) {
	tri := SubjPred("s", "p").Lang("en").StringLiteral(`say "hello"`)
	nt := encodeOne(t, tri)

	lit, ok := decodeOne(t, nt).Object().Literal()
	if !ok {
		t.Fatalf("not a literal: %q", nt)
	}
	if got, want := lit.Lang(), "en"; got != want {
		t.Errorf("lang = %q, want %q (nt %q)", got, want, nt)
	}
	if got, want := lit.Value(), `say "hello"`; got != want {
		t.Errorf("value = %q, want %q (nt %q)", got, want, nt)
	}
}

func TestMalformedLinesAreReported(t *testing.T) {
	cases := []struct{ name, line string }{
		{"no terminator", `<s> <p> "v"`},
		{"unterminated subject", `<s <p> "v" .`},
		{"unterminated predicate", `<s> <p "v" .`},
		{"missing object", `<s> <p> .`},
		{"garbage subject", `s <p> "v" .`},
		{"unquoted object", `<s> <p> bare .`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDecoder(strings.NewReader(tc.line + "\n")).Decode(); err == nil {
				t.Errorf("expected an error for %q", tc.line)
			}
		})
	}
}

// The error has to say which line, because a graph file holds thousands of them.
func TestDecodeErrorNamesTheLine(t *testing.T) {
	const in = "<s> <p> \"ok\" .\n<s> <p> \"ok\" .\nbroken\n"
	_, err := NewDecoder(strings.NewReader(in)).Decode()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error should name line 3, got %q", err)
	}
}

func TestMultiDecoderConcatenates(t *testing.T) {
	a := strings.NewReader("<a> <p> \"1\" .\n")
	b := strings.NewReader("<b> <p> \"2\" .\n")
	c := strings.NewReader("")

	got, err := NewMultiDecoder(a, b, c).Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d triples, want 2", len(got))
	}
}

func TestMultiDecoderReportsTheFailingSource(t *testing.T) {
	good := strings.NewReader("<a> <p> \"1\" .\n")
	bad := strings.NewReader("broken\n")

	if _, err := NewMultiDecoder(good, bad).Decode(); err == nil {
		t.Fatal("expected an error")
	}
}
