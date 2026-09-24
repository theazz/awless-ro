package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/graph"
)

// The synced graph describes the account: instance addresses, security group rules,
// IAM policy documents, bucket names. It is not a secret in the sense a key is, but it
// is reconnaissance handed over for free, so nothing under the storage directory may
// be readable by other users on the machine.
//
// These modes were right already; the test exists so that they cannot drift without
// someone noticing.
func TestSyncedFilesAreNotReadableByOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)

	srv := &mockService{g: graph.NewGraph(), name: "infra", region: "eu-west-1", profile: "admin"}

	if _, err := NewSyncer().Sync(srv); err != nil {
		t.Fatal(err)
	}

	wantDir := os.FileMode(0o700)
	wantFile := os.FileMode(0o600)

	cases := []struct {
		path string
		want os.FileMode
	}{
		{filepath.Join(home, "aws", "rdf"), wantDir},
		{filepath.Join(home, "aws", "rdf", "admin"), wantDir},
		{filepath.Join(home, "aws", "rdf", "admin", "eu-west-1"), wantDir},
		{filepath.Join(home, "aws", "rdf", "admin", "eu-west-1", "infra"+fileExt), wantFile},
	}

	for _, tc := range cases {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Errorf("%s: %s", tc.path, err)
			continue
		}
		if got := info.Mode().Perm(); got != tc.want {
			t.Errorf("%s has mode %#o, want %#o", tc.path, got, tc.want)
		}
	}
}

// A storage directory that cannot be created is reported, rather than surfacing later
// as a file that will not open in a directory that was never there.
func TestSyncReportsAnUncreatableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can write into a read-only directory")
	}

	home := t.TempDir()
	t.Setenv("__AWLESS_HOME", home)

	// The syncer creates the base directory when it is built, so make that one
	// read-only afterwards and let the per-service directory fail.
	syncer := NewSyncer()
	base := filepath.Join(home, "aws", "rdf")
	if err := os.Chmod(base, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(base, 0o700) })

	srv := &mockService{g: graph.NewGraph(), name: "infra", region: "eu-west-1", profile: "admin"}

	_, err := syncer.Sync(srv)
	if err == nil {
		t.Fatal("expected an error when the storage directory cannot be created")
	}
	if got := err.Error(); !strings.Contains(got, "creating") {
		t.Errorf("the error should say what it failed to create, got: %s", got)
	}
}
