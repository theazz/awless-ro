// Package golden compares command output with files kept under testdata/.
//
// The comparison is byte for byte on purpose: the JSON output is read by programs, so
// key order, indentation and number formatting are part of what is pinned, not only
// the values. Regenerate the files after an intended change by running the tests of
// the packages that use them with -update (only those packages define the flag):
//
//	go test ./commands/ ./console/ -update
//
// and read the diff before committing it.
package golden

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/ with the current output")

// Assert fails t unless got is the content of testdata/<name>.golden.
func Assert(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s (run `go test -update` to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}
