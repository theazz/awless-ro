package ssh

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/logger"
)

// When `ssh` runs without -i, the key name comes from the instance's KeyPair
// attribute, i.e. from the AWS API. A relative name must therefore stay inside the
// configured key folders.
func TestRelativeKeyNameCannotEscapeKeyFolders(t *testing.T) {
	root := t.TempDir()

	keysDir := filepath.Join(root, "keys")
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// A file that exists, but outside the key folders. Nothing should reach it.
	if err := os.WriteFile(filepath.Join(root, "secret"), []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A legitimate key inside, to show the traversal check does not break the
	// ordinary case.
	inside := filepath.Join(keysDir, "good.pem")
	if err := os.WriteFile(inside, []byte("legit"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, ok := findPrivateKeyFromName("../secret", keysDir); ok {
		t.Fatal("a key name containing .. was resolved outside the key folders")
	}

	priv, ok := findPrivateKeyFromName("good", keysDir)
	if !ok {
		t.Fatal("a key inside the key folder was not found")
	}
	if priv.path != inside {
		t.Fatalf("got %q, want %q", priv.path, inside)
	}
}

// An absolute path means the user passed -i explicitly, so it is still honoured.
func TestAbsoluteKeyPathIsHonoured(t *testing.T) {
	root := t.TempDir()
	key := filepath.Join(root, "id_test")
	if err := os.WriteFile(key, []byte("legit"), 0o600); err != nil {
		t.Fatal(err)
	}

	priv, ok := findPrivateKeyFromName(key, filepath.Join(root, "unrelated"))
	if !ok {
		t.Fatal("absolute key path was rejected")
	}
	if priv.path != key {
		t.Fatalf("got %q, want %q", priv.path, key)
	}
}

func TestWarnsOnKeyReadableByOthers(t *testing.T) {
	root := t.TempDir()

	var logged bytes.Buffer
	previous := logger.DefaultLogger
	logger.DefaultLogger = logger.New("", 0, &logged)
	t.Cleanup(func() { logger.DefaultLogger = previous })

	loose := filepath.Join(root, "loose")
	if err := os.WriteFile(loose, []byte("legit"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Set the mode explicitly: WriteFile's permissions go through the umask.
	if err := os.Chmod(loose, 0o644); err != nil {
		t.Fatal(err)
	}
	warnOnLooseKeyPermissions(loose)
	if !strings.Contains(logged.String(), "too open") {
		t.Fatalf("expected a warning for a 0644 key, got %q", logged.String())
	}

	logged.Reset()
	tight := filepath.Join(root, "tight")
	if err := os.WriteFile(tight, []byte("legit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tight, 0o600); err != nil {
		t.Fatal(err)
	}
	warnOnLooseKeyPermissions(tight)
	if logged.Len() != 0 {
		t.Fatalf("unexpected warning for a 0600 key: %q", logged.String())
	}
}
