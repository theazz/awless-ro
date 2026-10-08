package awsconv

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

func TestUnescapeDNSName(t *testing.T) {
	tcases := []struct {
		in   string
		want string
	}{
		// \052 is OCTAL. 0o52 == 42 == '*'. If this row starts expecting
		// "4mysite.com." someone changed the base to decimal to match RFC 1035
		// §5.1 — Route 53 does not use the decimal form, and that change
		// silently corrupts every escaped name.
		{in: `\052mysite.com.`, want: "*mysite.com."},

		// Same guard from the other side: two octal triples that are a UTF-8
		// sequence. 0o303,0o251 == 195,169 == "é". Decimal 303 does not fit in
		// a byte at all.
		{in: `\303\251t\303\251.example.com.`, want: "été.example.com."},

		// Rule 1, the other reported form: 0o137 == 95 == '_'.
		{in: `\137dmarc.example.com.`, want: "_dmarc.example.com."},

		// Rule 3: a backslash followed by a non-octal character yields that
		// character verbatim, which is what makes \\ -> \ work.
		{in: `a\\b.example.com.`, want: `a\b.example.com.`},

		// Rule 5: no backslash, returned unchanged.
		{in: "www.example.com.", want: "www.example.com."},

		// Rule 1 then rule 4: a trailing lone backslash is kept verbatim.
		{in: `\052\`, want: `*\`},

		// Rule 2, value < 0x20: a control octet stays escaped, because the
		// escape IS its readable form.
		{in: `\001.example.com.`, want: `\001.example.com.`},

		// Rule 2, value == 0x7F: DEL is a control octet too.
		{in: `\177.example.com.`, want: `\177.example.com.`},

		// Rule 1 decodes 0o377 == 0xff, which is not valid UTF-8 on its own, so
		// the utf8.ValidString fail-safe returns the ORIGINAL string.
		{in: `\377.example.com.`, want: `\377.example.com.`},

		// Rule 2, value > 0xFF: the whole \400..\777 range has no octet AWS
		// could mean, so it is left alone.
		{in: `\400.example.com.`, want: `\400.example.com.`},
		{in: `\777.example.com.`, want: `\777.example.com.`},

		// Rule 3: not octal digits, so only the '\' is dropped. Harmless, and
		// never a form AWS emits.
		{in: `\999.example.com.`, want: "999.example.com."},

		// Rule 3, documented as mildly lossy: an escaped dot becomes an
		// ordinary dot. Route 53 escapes octets as octal triples, so it does not
		// emit this form.
		{in: `a\.b.example.com.`, want: "a.b.example.com."},
	}

	for i, tc := range tcases {
		if got := unescapeDNSName(tc.in); got != tc.want {
			t.Errorf("%d: unescapeDNSName(%q): got %q, want %q", i, tc.in, got, tc.want)
		}
	}
}

// TestUnescapeDNSNameOctalBaseStructural is the STRUCTURAL base guard, so the
// guard survives a rewrite of the table above. A decoder reading the triple as
// decimal fails at the very first value: \040 is 0o40 == 32 == ' ' in octal, but
// 40 == '(' in decimal.
//
// The range is 0x20..0x7E (printable ASCII) and nothing wider, deliberately: a
// single decoded octet in 0x80..0xFF is not valid UTF-8 on its own, so rule 1
// decodes it and the utf8.ValidString fail-safe then returns the original
// escaped string — the round-trip would fail at v = 0x80 BY DESIGN, not by bug.
// 0x7F is left escaped by rule 2. The 0x80..0xFF range is covered instead by the
// \377 and \303\251 rows of the table, which assert the fail-safe and the
// multi-byte reassembly respectively.
func TestUnescapeDNSNameOctalBaseStructural(t *testing.T) {
	for v := 0x20; v <= 0x7E; v++ {
		if got := unescapeDNSName(fmt.Sprintf(`\%03o`, v)); got != string(rune(v)) {
			t.Errorf("octal base broken at %#x: got %q, want %q", v, got, string(rune(v)))
		}
	}
}

// unescapeDNSName is total: it never panics, and for valid UTF-8 in it returns
// valid UTF-8 out.
func TestUnescapeDNSNameIsTotal(t *testing.T) {
	inputs := []string{
		"",
		`\`,
		`\\`,
		`\\\`,
		`\0`,
		`\01`,
		`\012`,
		`\0123`,
		`\377\377`,
		`\303`,
		`\303\251`,
		`\\052`,
		`\052\137\303\251`,
		"été.example.com.",
		`\é`,
		`a\`,
		`.\.\.`,
	}

	for _, in := range inputs {
		if !utf8.ValidString(in) {
			t.Fatalf("test input %q is not valid UTF-8", in)
		}
		got := unescapeDNSName(in)
		if !utf8.ValidString(got) {
			t.Errorf("unescapeDNSName(%q) returned invalid UTF-8: %q", in, got)
		}
	}
}

func TestDecodeDNSNameFnLeavesNonStringResultsAlone(t *testing.T) {
	boom := fmt.Errorf("inner failed")

	tcases := []struct {
		name    string
		inner   transformFn
		want    interface{}
		wantErr error
	}{
		{
			name:  "a string result is decoded",
			inner: func(i interface{}) (interface{}, error) { return `\052mysite.com.`, nil },
			want:  "*mysite.com.",
		},
		{
			name:  "a nil result propagates as nil, nil",
			inner: func(i interface{}) (interface{}, error) { return nil, nil },
			want:  nil,
		},
		{
			name:    "an error propagates unchanged",
			inner:   func(i interface{}) (interface{}, error) { return nil, boom },
			want:    nil,
			wantErr: boom,
		},
		{
			name:  "a non-string result is untouched",
			inner: func(i interface{}) (interface{}, error) { return int64(42), nil },
			want:  int64(42),
		},
	}

	for _, tc := range tcases {
		got, err := decodeDNSNameFn(tc.inner)(nil)
		if err != tc.wantErr {
			t.Fatalf("%s: got error %v, want %v", tc.name, err, tc.wantErr)
		}
		if got != tc.want {
			t.Errorf("%s: got %v (%T), want %v (%T)", tc.name, got, got, tc.want, tc.want)
		}
	}
}
