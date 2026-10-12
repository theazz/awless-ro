package ssh

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// The keys under testdata/fingerprint are throwaway, generated once with the local
// tools purely to pin the fingerprint arithmetic; they protect nothing. The
// expected values below were produced with the AWS-documented commands, quoted next
// to each constant, so the Go computation is checked against an independent tool
// rather than against itself.
const (
	// openssl pkcs8 -in rsa_pkcs1.pem -inform PEM -outform DER -topk8 -nocrypt | openssl sha1
	wantCreatedRSA = "16:9f:93:d6:e0:f0:72:76:12:2b:cd:33:89:c9:e2:65:e6:2e:67:ba"
	// openssl rsa -in rsa_pkcs1.pem -pubout -outform DER | openssl md5
	wantImportedRSAPKIX = "12:d0:3f:e9:bc:d2:52:78:a2:be:52:ae:51:d6:8b:44"
	// ssh-keygen -l -E md5 -f rsa.pub
	wantImportedRSAWire = "ae:c1:1b:d7:de:8e:2f:77:9a:c5:1d:c3:eb:04:f2:d1"
	// ssh-keygen -l -f ed25519.pub (ssh-keygen prints it as "SHA256:<value>")
	wantED25519 = "OyaPfrCREtHv766wp4FzycaWca5Cna42ZmPBLh795wE"
)

func fingerprintFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "fingerprint", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// byForm indexes a candidate list by its form, so a test can assert on the value a
// given shape produced.
func byForm(fps []fingerprint) map[string]string {
	m := make(map[string]string, len(fps))
	for _, fp := range fps {
		m[fp.form] = fp.value
	}
	return m
}

// Every RSA encoding — PKCS#1, PKCS#8, OpenSSH — describes the same key, so all
// three must yield the same created-RSA, imported-PKIX and RFC 4716 values, equal
// to the constants computed by openssl and ssh-keygen.
func TestRSAPrivateKeyFingerprintsAcrossEncodings(t *testing.T) {
	for _, name := range []string{"rsa_pkcs1.pem", "rsa_pkcs8.pem", "rsa_openssh"} {
		fps, err := privateKeyFingerprints(fingerprintFixture(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := byForm(fps)
		if got[formCreatedRSA] != wantCreatedRSA {
			t.Errorf("%s: created-RSA %q, want %q", name, got[formCreatedRSA], wantCreatedRSA)
		}
		if got[formImportedRSAPKIX] != wantImportedRSAPKIX {
			t.Errorf("%s: imported-PKIX %q, want %q", name, got[formImportedRSAPKIX], wantImportedRSAPKIX)
		}
		if got[formImportedRSAWire] != wantImportedRSAWire {
			t.Errorf("%s: RFC 4716 %q, want %q", name, got[formImportedRSAWire], wantImportedRSAWire)
		}
		if _, ok := got[formED25519]; ok {
			t.Errorf("%s: an RSA key produced an ED25519 fingerprint", name)
		}
		// The two imported MD5 forms are over different inputs and must differ.
		if got[formImportedRSAPKIX] == got[formImportedRSAWire] {
			t.Errorf("%s: the PKIX and wire MD5 forms are equal, one is wrong", name)
		}
	}
}

// The public key alone yields the two imported-RSA forms, never the created one,
// which needs the private key.
func TestRSAPublicKeyFingerprints(t *testing.T) {
	pub, _, _, _, err := gossh.ParseAuthorizedKey(fingerprintFixture(t, "rsa.pub"))
	if err != nil {
		t.Fatal(err)
	}
	got := byForm(publicKeyFingerprints(pub))
	if got[formImportedRSAPKIX] != wantImportedRSAPKIX {
		t.Errorf("imported-PKIX %q, want %q", got[formImportedRSAPKIX], wantImportedRSAPKIX)
	}
	if got[formImportedRSAWire] != wantImportedRSAWire {
		t.Errorf("RFC 4716 %q, want %q", got[formImportedRSAWire], wantImportedRSAWire)
	}
	if _, ok := got[formCreatedRSA]; ok {
		t.Error("a public key produced the created-RSA fingerprint, which needs the private key")
	}
}

// The ED25519 private key and its .pub give the same base64 SHA-256, equal to the
// value ssh-keygen prints.
func TestED25519Fingerprints(t *testing.T) {
	fps, err := privateKeyFingerprints(fingerprintFixture(t, "ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	got := byForm(fps)
	if normaliseFingerprint(got[formED25519]) != wantED25519 {
		t.Errorf("private key ED25519 %q, want %q", got[formED25519], wantED25519)
	}
	if len(fps) != 1 {
		t.Errorf("an ED25519 key produced %d forms, want 1", len(fps))
	}

	pub, _, _, _, err := gossh.ParseAuthorizedKey(fingerprintFixture(t, "ed25519.pub"))
	if err != nil {
		t.Fatal(err)
	}
	pubGot := byForm(publicKeyFingerprints(pub))
	if normaliseFingerprint(pubGot[formED25519]) != wantED25519 {
		t.Errorf(".pub ED25519 %q, want %q", pubGot[formED25519], wantED25519)
	}
}

// matchFingerprint has to see through the shapes EC2 and ssh-keygen differ in: the
// base64 digest with and without "SHA256:" and "=" padding, and colon hex in either
// case. A one-character change must not match.
func TestMatchFingerprintNormalisation(t *testing.T) {
	fps, err := privateKeyFingerprints(fingerprintFixture(t, "ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	for _, recorded := range []string{wantED25519, wantED25519 + "=", "SHA256:" + wantED25519} {
		if _, ok := matchFingerprint(recorded, fps); !ok {
			t.Errorf("ED25519 fingerprint %q did not match", recorded)
		}
	}

	rsaFps, err := privateKeyFingerprints(fingerprintFixture(t, "rsa_pkcs1.pem"))
	if err != nil {
		t.Fatal(err)
	}
	for _, recorded := range []string{wantCreatedRSA, strings.ToUpper(wantCreatedRSA), wantImportedRSAPKIX, wantImportedRSAWire} {
		if _, ok := matchFingerprint(recorded, rsaFps); !ok {
			t.Errorf("RSA fingerprint %q did not match", recorded)
		}
	}

	// One hex digit off is a different key.
	bad := "17:9f:93:d6:e0:f0:72:76:12:2b:cd:33:89:c9:e2:65:e6:2e:67:ba"
	if _, ok := matchFingerprint(bad, rsaFps); ok {
		t.Error("a fingerprint off by one character matched")
	}
	if _, ok := matchFingerprint("", rsaFps); ok {
		t.Error("an empty recorded fingerprint matched")
	}
}

// Both encrypted fixtures must report a missing passphrase and offer no
// fingerprints: the scan relies on that to skip them without ever prompting.
func TestEncryptedPrivateKeysYieldNoFingerprints(t *testing.T) {
	for _, name := range []string{"ed25519_enc", "rsa_enc_legacy"} {
		fps, err := privateKeyFingerprints(fingerprintFixture(t, name))
		var missing *gossh.PassphraseMissingError
		if !errors.As(err, &missing) {
			t.Errorf("%s: got err %v, want *ssh.PassphraseMissingError", name, err)
		}
		if len(fps) != 0 {
			t.Errorf("%s: an encrypted key produced %d fingerprints, want 0", name, len(fps))
		}
	}
}

// An ECDSA key is not an EC2 key pair type: it yields no candidate at all.
func TestECDSAKeyHasNoFingerprints(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatal(err)
	}
	fps, err := privateKeyFingerprints(pem.EncodeToMemory(block))
	if err != nil {
		t.Fatal(err)
	}
	if len(fps) != 0 {
		t.Errorf("an ECDSA key produced %d fingerprints, want 0", len(fps))
	}
}
