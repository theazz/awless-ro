package ssh

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/logger"

	gossh "golang.org/x/crypto/ssh"
)

// noPrompt replaces the passphrase reader for the whole test: the fingerprint scan
// must never reach a prompt, so any call fails the test.
func noPrompt(t *testing.T) {
	t.Helper()
	previous := readPassphrase
	readPassphrase = func(path string) ([]byte, error) {
		t.Errorf("asked for the passphrase of %q during the scan", path)
		return nil, errors.New("must not prompt")
	}
	t.Cleanup(func() { readPassphrase = previous })
}

// genKey returns a fresh ed25519 key and the fingerprint EC2 would record for it:
// the base64 SHA-256, as the API returns it, without the "SHA256:" ssh-keygen adds.
func genKey(t *testing.T) (crypto.Signer, string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := gossh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatal(err)
	}
	return priv, strings.TrimPrefix(gossh.FingerprintSHA256(pub), "SHA256:")
}

// writeKeyFile writes priv as an unencrypted OpenSSH PEM at path with mode.
func writeKeyFile(t *testing.T, path string, priv crypto.Signer, mode os.FileMode) {
	t.Helper()
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// writeEncryptedKeyFile writes priv encrypted with passphrase, and optionally its
// .pub, at path.
func writeEncryptedKeyFile(t *testing.T, path string, priv crypto.Signer, passphrase string, withPub bool) {
	t.Helper()
	block, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if withPub {
		writePubFile(t, path+".pub", priv.Public())
	}
}

func writePubFile(t *testing.T, path string, pub crypto.PublicKey) {
	t.Helper()
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, gossh.MarshalAuthorizedKey(sshPub), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tempDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A key whose file name is nothing like the key pair is still found, in either
// folder, by matching the fingerprint EC2 recorded.
func TestScanFindsDifferentlyNamedKey(t *testing.T) {
	noPrompt(t)
	for _, where := range []string{"first", "second"} {
		f1, f2 := tempDir(t, "f1-"+where), tempDir(t, "f2-"+where)
		priv, recorded := genKey(t)
		folder := f1
		if where == "second" {
			folder = f2
		}
		path := filepath.Join(folder, "laptop-key")
		writeKeyFile(t, path, priv, 0o600)

		got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, f1, f2)
		if !ok || got.path != path {
			t.Errorf("%s folder: got (%q, %t), want %q", where, got.path, ok, path)
		}
	}
}

// With a matching key in both folders, the first folder wins, as the name lookup
// orders them.
func TestScanFolderOrderWins(t *testing.T) {
	noPrompt(t)
	f1, f2 := tempDir(t, "f1"), tempDir(t, "f2")
	priv, recorded := genKey(t)
	writeKeyFile(t, filepath.Join(f1, "a-key"), priv, 0o600)
	writeKeyFile(t, filepath.Join(f2, "a-key"), priv, 0o600)

	got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, f1, f2)
	if !ok || got.path != filepath.Join(f1, "a-key") {
		t.Errorf("got (%q, %t), want the key in the first folder", got.path, ok)
	}
}

// The AWS-created-RSA form (SHA-1 of the private key) is matched from a real RSA
// key, under any file name.
func TestScanFindsCreatedRSAForm(t *testing.T) {
	noPrompt(t)
	folder := tempDir(t, "keys")
	body := fingerprintFixture(t, "rsa_pkcs1.pem")
	path := filepath.Join(folder, "laptop.pem")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: wantCreatedRSA}, folder)
	if !ok || got.path != path {
		t.Errorf("got (%q, %t), want %q", got.path, ok, path)
	}
}

// A folder holding only a non-matching key resolves to nothing, and InitClient says
// so, naming the key pair, the folders and the fingerprint reason.
func TestScanNoMatchReportsMissingKey(t *testing.T) {
	noPrompt(t)
	t.Setenv("SSH_AUTH_SOCK", "")
	folder := tempDir(t, "keys")
	other, _ := genKey(t)
	writeKeyFile(t, filepath.Join(folder, "other"), other, 0o600)
	_, recorded := genKey(t)

	_, err := InitClient(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("got %v, want ErrKeyNotFound", err)
	}
	for _, want := range []string{"aws-pair", folder, "fingerprint AWS recorded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// When the fingerprint is unknown (nothing synced), the error points at `sync`.
func TestMissingKeyWithoutFingerprintPointsAtSync(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	folder := tempDir(t, "keys")
	_, err := InitClient(KeyRef{Name: "aws-pair"}, folder)
	if !errors.Is(err, ErrKeyNotFound) || !strings.Contains(err.Error(), "sync") {
		t.Fatalf("got %v, want ErrKeyNotFound mentioning sync", err)
	}
}

// A matching key whose permissions are too open is refused, not used, with a
// warning that names it. openssh refuses such a key; a read-only tool must not
// quietly use it.
func TestScanRefusesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are meaningless on Windows")
	}
	noPrompt(t)
	var logged bytes.Buffer
	previous := logger.DefaultLogger
	logger.DefaultLogger = logger.New("", 0, &logged)
	t.Cleanup(func() { logger.DefaultLogger = previous })

	folder := tempDir(t, "keys")
	priv, recorded := genKey(t)
	path := filepath.Join(folder, "loose")
	writeKeyFile(t, path, priv, 0o644)

	if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
		t.Error("a world-readable key was used")
	}
	if !strings.Contains(logged.String(), path) || !strings.Contains(logged.String(), "too open") {
		t.Errorf("no warning naming the loose key: %q", logged.String())
	}
}

// An encrypted key is matched through its sibling .pub, with no passphrase prompt;
// without a .pub it is skipped, still without prompting.
func TestScanEncryptedKeyMatchedViaPub(t *testing.T) {
	noPrompt(t)
	t.Run("with .pub", func(t *testing.T) {
		folder := tempDir(t, "keys")
		priv, recorded := genKey(t)
		path := filepath.Join(folder, "enc")
		writeEncryptedKeyFile(t, path, priv, "secret", true)

		got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder)
		if !ok || got.path != path {
			t.Errorf("got (%q, %t), want %q", got.path, ok, path)
		}
	})

	t.Run("without .pub", func(t *testing.T) {
		folder := tempDir(t, "keys")
		priv, recorded := genKey(t)
		writeEncryptedKeyFile(t, filepath.Join(folder, "enc"), priv, "secret", false)

		if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
			t.Error("an encrypted key with no .pub was used")
		}
	})

	t.Run(".pub of another key", func(t *testing.T) {
		folder := tempDir(t, "keys")
		priv, recorded := genKey(t)
		path := filepath.Join(folder, "enc")
		writeEncryptedKeyFile(t, path, priv, "secret", false)
		// A .pub beside it that belongs to a different key.
		stranger, _ := genKey(t)
		writePubFile(t, path+".pub", stranger.Public())

		if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
			t.Error("an encrypted key with a mismatched .pub was used")
		}
	})
}

// The legacy encrypted PEM fixture, matched through its .pub by the imported-RSA MD5
// form, with no prompt.
func TestScanEncryptedLegacyRSAViaPub(t *testing.T) {
	noPrompt(t)
	folder := tempDir(t, "keys")
	if err := os.WriteFile(filepath.Join(folder, "legacy"), fingerprintFixture(t, "rsa_enc_legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	pubBody := fingerprintFixture(t, "rsa_enc_legacy.pub")
	if err := os.WriteFile(filepath.Join(folder, "legacy.pub"), pubBody, 0o644); err != nil {
		t.Fatal(err)
	}
	// The fingerprint EC2 would record for it: the RFC 4716 MD5 of its public key.
	pub, _, _, _, err := gossh.ParseAuthorizedKey(pubBody)
	if err != nil {
		t.Fatal(err)
	}
	recorded := byForm(publicKeyFingerprints(pub))[formImportedRSAWire]

	got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder)
	if !ok || got.path != filepath.Join(folder, "legacy") {
		t.Errorf("got (%q, %t), want the legacy key", got.path, ok)
	}
}

// Nothing outside the folders is ever read: a symlink to a matching key, a matching
// key in a subdirectory, an over-size file, and a plain text file are all ignored.
func TestScanStaysInsideFoldersAndSkipsNonKeys(t *testing.T) {
	noPrompt(t)

	t.Run("symlink out of the folder", func(t *testing.T) {
		folder := tempDir(t, "keys")
		outside := tempDir(t, "outside")
		priv, recorded := genKey(t)
		target := filepath.Join(outside, "real")
		writeKeyFile(t, target, priv, 0o600)
		if err := os.Symlink(target, filepath.Join(folder, "link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
			t.Error("a symlink pointing outside the folder was followed")
		}
	})

	t.Run("subdirectory", func(t *testing.T) {
		folder := tempDir(t, "keys")
		sub := filepath.Join(folder, "sub")
		if err := os.MkdirAll(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		priv, recorded := genKey(t)
		writeKeyFile(t, filepath.Join(sub, "key"), priv, 0o600)
		if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
			t.Error("a key in a subdirectory was found; the scan must not recurse")
		}
	})

	t.Run("over-size file", func(t *testing.T) {
		folder := tempDir(t, "keys")
		priv, recorded := genKey(t)
		block, err := gossh.MarshalPrivateKey(priv, "")
		if err != nil {
			t.Fatal(err)
		}
		body := append(pem.EncodeToMemory(block), bytes.Repeat([]byte("#padding\n"), 4096)...)
		if err := os.WriteFile(filepath.Join(folder, "huge"), body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); ok {
			t.Error("an over-size file was read")
		}
	})

	t.Run("plain text file is ignored, real key still found", func(t *testing.T) {
		folder := tempDir(t, "keys")
		if err := os.WriteFile(filepath.Join(folder, "config"), []byte("Host *\n  User me\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		priv, recorded := genKey(t)
		path := filepath.Join(folder, "real-key")
		writeKeyFile(t, path, priv, 0o600)
		got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder)
		if !ok || got.path != path {
			t.Errorf("got (%q, %t), want the real key; the text file must be ignored without error", got.path, ok)
		}
	})
}

// Step (b) beats step (c): a file named like the key pair is used even when it does
// not match the fingerprint, and a differently-named matching key is left alone.
func TestNameLookupBeatsFingerprintScan(t *testing.T) {
	noPrompt(t)
	folder := tempDir(t, "keys")
	named, _ := genKey(t)
	writeKeyFile(t, filepath.Join(folder, "aws-pair.pem"), named, 0o600)
	matching, recorded := genKey(t)
	writeKeyFile(t, filepath.Join(folder, "laptop-key"), matching, 0o600)

	got, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder)
	if !ok || got.path != filepath.Join(folder, "aws-pair.pem") {
		t.Errorf("got (%q, %t), want the key named like the key pair", got.path, ok)
	}
}

// With no fingerprint (the -i case) there is no scan, even when a matching key is
// present under another name.
func TestNoFingerprintNoScan(t *testing.T) {
	noPrompt(t)
	folder := tempDir(t, "keys")
	priv, _ := genKey(t)
	writeKeyFile(t, filepath.Join(folder, "laptop-key"), priv, 0o600)

	if _, ok := findPrivateKey(KeyRef{Name: "other-name"}, folder); ok {
		t.Error("a scan ran although no fingerprint was given")
	}
}

// The verbose log names the matched file and the matching form, and leaks neither
// the key material nor the fingerprint value.
func TestScanVerboseLogDoesNotLeak(t *testing.T) {
	noPrompt(t)
	var logged bytes.Buffer
	previous := logger.DefaultLogger
	logger.DefaultLogger = logger.New("", 0, &logged)
	logger.DefaultLogger.SetVerbose(logger.VerboseF)
	t.Cleanup(func() { logger.DefaultLogger = previous })

	folder := tempDir(t, "keys")
	priv, recorded := genKey(t)
	path := filepath.Join(folder, "laptop-key")
	writeKeyFile(t, path, priv, 0o600)

	if _, ok := findPrivateKey(KeyRef{Name: "aws-pair", Fingerprint: recorded}, folder); !ok {
		t.Fatal("the matching key was not found")
	}
	out := logged.String()
	if !strings.Contains(out, path) || !strings.Contains(out, formED25519) {
		t.Errorf("log does not name the file and the form: %q", out)
	}
	if strings.Contains(out, "PRIVATE KEY") || strings.Contains(out, recorded) {
		t.Errorf("log leaks key material or the fingerprint value: %q", out)
	}
}

// End to end: InitClient with a key found only by fingerprint authenticates against
// a real loopback server.
func TestInitClientWithScanMatchedKeyAuthenticates(t *testing.T) {
	noPrompt(t)
	env := newSSHEnv(t)
	signer := writeClientKey(t, env.keyDir, "laptop-key")
	recorded := strings.TrimPrefix(gossh.FingerprintSHA256(signer.PublicKey()), "SHA256:")
	hostKey := newSigner(t)
	srv := startServer(t, hostKey, "ec2-user", signer.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), srv.addr, hostKey.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: false})

	c, err := InitClient(KeyRef{Name: "aws-pair", Fingerprint: recorded}, env.keyDir)
	if err != nil {
		t.Fatal(err)
	}
	c.IP, c.Port = srv.host, srv.port
	t.Cleanup(func() { c.CloseAll() })
	if err := c.DialWithUsers(guessed("ec2-user")); err != nil {
		t.Fatalf("a key found by fingerprint did not authenticate: %v", err)
	}
}
