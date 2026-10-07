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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A literal can hold a whole IAM policy document, and nothing in the N-Triples format
// caps its size. The old bufio.Scanner capped a line at 8 MiB, so a large document
// turned into a read error. These guard the growing-buffer reader that replaced it.

// 500 KB is the already-fixed half of upstream #300 (its failing file was 454 KB).
// It must keep round-tripping byte-for-byte.
func TestLongLineHalfMegabyteRoundTrips(t *testing.T) {
	value := strings.Repeat("abcd", 500<<10/4) // exactly 500 KiB
	nt := encodeOne(t, SubjPred("s", "p").StringLiteral(value))

	got := decodeOne(t, nt)
	lit, ok := got.Object().Literal()
	if !ok {
		t.Fatal("object is not a literal")
	}
	if lit.Value() != value {
		t.Errorf("500 KB literal changed length: in %d bytes, out %d bytes", len(value), len(lit.Value()))
	}
}

// 12 MiB is past the old 8 MiB bound; before the fix this was a hard read error.
func TestLongLinePastOldCeilingRoundTrips(t *testing.T) {
	value := strings.Repeat("x", 12<<20)
	nt := encodeOne(t, SubjPred("s", "p").StringLiteral(value))

	got := decodeOne(t, nt)
	lit, ok := got.Object().Literal()
	if !ok {
		t.Fatal("object is not a literal")
	}
	if lit.Value() != value {
		t.Errorf("12 MiB literal changed length: in %d bytes, out %d bytes", len(value), len(lit.Value()))
	}
}

// The reader must not become quadratic: decoding one big line should allocate on the
// order of the line's own size, not a multiple of it. The bound is a ballpark guard
// (expected ~3×), not a budget — raise the multiplier if it proves flaky, do not
// delete it, it is the only check that the unbounded reader stayed linear.
func TestLongLineDoesNotBalloonMemory(t *testing.T) {
	value := strings.Repeat("x", 12<<20)
	// Build the fixture string before measuring so only the decode is counted.
	nt := encodeOne(t, SubjPred("s", "p").StringLiteral(value))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	got, err := NewDecoder(strings.NewReader(nt)).Decode()
	if err != nil {
		t.Fatal(err)
	}

	runtime.ReadMemStats(&after)
	if len(got) != 1 {
		t.Fatalf("got %d triples, want 1", len(got))
	}

	allocated := after.TotalAlloc - before.TotalAlloc
	// 8× rather than the ~3× typically expected: the growing append buffer can carry
	// transient slack, and this is a quadratic-blowup guard, not a tight budget.
	if limit := uint64(8 * len(nt)); allocated > limit {
		t.Errorf("decoding a %d-byte line allocated %d bytes, more than %d; the reader may be quadratic", len(nt), allocated, limit)
	}
}

// The safety valve: a single line past maxLineBytes is a diagnosable error naming the
// line and the limit, rather than an OOM kill. Lower the cap so the test need not
// allocate 256 MiB.
func TestSingleLineOverCapIsAnError(t *testing.T) {
	saved := maxLineBytes
	maxLineBytes = 4 << 10
	t.Cleanup(func() { maxLineBytes = saved })

	line := "<s> <p> \"" + strings.Repeat("x", 5<<10) + "\" .\n"
	_, err := NewDecoder(strings.NewReader(line)).Decode()
	if err == nil {
		t.Fatal("expected an error for a line past the cap")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error should name the line, got %q", err)
	}
	if !strings.Contains(err.Error(), "4096") {
		t.Errorf("error should name the limit, got %q", err)
	}
}

// bufio.ScanLines dropped a trailing \r; the growing-buffer reader has to do the same
// or every CRLF file breaks, because parseObject requires the line to end with '.'.
func TestCRLFLinesDecode(t *testing.T) {
	in := "<s> <p> \"ok\" .\r\n<s2> <p> \"ok\" .\r\n"
	got, err := NewDecoder(strings.NewReader(in)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d triples, want 2", len(got))
	}
}

// A final line with no trailing newline still parses.
func TestFinalLineWithoutNewlineDecodes(t *testing.T) {
	in := "<s> <p> \"ok\" ." // no \n
	got, err := NewDecoder(strings.NewReader(in)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d triples, want 1", len(got))
	}
}

// Two broken sources must both be named, so one unreadable service file cannot hide
// another. The readers are *os.File so their names appear in the error.
func TestMultiDecoderNamesEveryFailingFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) *os.File {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}

	a := write("access.nt", "<pol-1> <cloud:id> \"pol-1\" .\nTHIS IS NOT N-TRIPLES\n")
	b := write("dns.nt", "also broken\n")

	_, err := NewMultiDecoder(a, b).Decode()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "access.nt") {
		t.Errorf("error should name access.nt, got %q", err)
	}
	if !strings.Contains(err.Error(), "dns.nt") {
		t.Errorf("error should name dns.nt, got %q", err)
	}
}
