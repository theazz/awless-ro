package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// The encrypted-key path used to be hand-rolled on x509.DecryptPEMBlock plus
// x509.ParsePKCS1PrivateKey, which understood exactly one thing: a legacy PEM block
// holding an RSA key. An ed25519 key in the OpenSSH format — what ssh-keygen has
// produced by default for years — could not be loaded at all.
func TestEncryptedOpenSSHKeyIsAccepted(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	passphrase := []byte("correct horse battery staple")

	block, err := gossh.MarshalPrivateKeyWithPassphrase(priv, "test", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	body := pem.EncodeToMemory(block)

	// Without the passphrase the library must tell us so by type, not by message:
	// that is what privateKeyAuth branches on.
	_, err = gossh.ParsePrivateKey(body)
	var missing *gossh.PassphraseMissingError
	if !errors.As(err, &missing) {
		t.Fatalf("expected *ssh.PassphraseMissingError, got %T: %v", err, err)
	}

	signer, err := gossh.ParsePrivateKeyWithPassphrase(body, passphrase)
	if err != nil {
		t.Fatalf("encrypted ed25519 OpenSSH key rejected: %v", err)
	}
	if got := signer.PublicKey().Type(); got != gossh.KeyAlgoED25519 {
		t.Fatalf("got key type %s, want %s", got, gossh.KeyAlgoED25519)
	}
}

// An unencrypted key must not take the passphrase branch, which would block on a
// terminal read in a test.
func TestUnencryptedKeyNeedsNoPassphrase(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := privateKeyAuth(privateKey{path: "mem", body: pem.EncodeToMemory(block)}); err != nil {
		t.Fatalf("unencrypted key rejected: %v", err)
	}
}
