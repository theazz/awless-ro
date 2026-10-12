package ssh

import (
	"crypto/ed25519"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"regexp"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// EC2 records a key pair's fingerprint in one of a few shapes, depending on how the
// key pair was made, and the SDK and the User Guide do not even agree on all of
// them. awless-ro never knows which shape a given key pair uses, so it computes
// every shape a local key could produce and matches any against the one AWS
// recorded. The forms are the ones documented in the EC2 User Guide, "Verify the
// fingerprint of your key pair"
// (https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/verify-keys.html), and in
// the SDK v2 doc comment on ec2 types KeyPairInfo.KeyFingerprint.
const (
	// formCreatedRSA is what EC2 records for an RSA key pair it generated itself:
	// the SHA-1 digest of the PKCS#8 DER encoding of the *private* key
	// (openssl pkcs8 -topk8 -nocrypt -outform DER | openssl sha1). Only a private
	// key can produce it, so an encrypted key never matches this form.
	formCreatedRSA = "AWS-created RSA (SHA-1 of the PKCS#8 private key)"
	// formImportedRSAPKIX is one of the two shapes an imported RSA key pair gets:
	// the MD5 digest of the PKIX (SubjectPublicKeyInfo) DER encoding of the public
	// key, as the User Guide's `openssl rsa -pubout -outform DER | openssl md5`.
	formImportedRSAPKIX = "imported RSA (MD5 of the public key)"
	// formImportedRSAWire is the other imported-RSA shape: the MD5 of the SSH
	// wire-format public key (RFC 4716), as `ssh-keygen -l -E md5`. The User Guide
	// and the SDK doc disagree on which MD5 EC2 shows for an imported RSA key, so
	// both are computed and either matches.
	formImportedRSAWire = "imported RSA (MD5 of the SSH public key, RFC 4716)"
	// formED25519 is what EC2 records for an ED25519 key pair, created or imported:
	// the base64 SHA-256 of the SSH wire-format public key, as `ssh-keygen -l`.
	formED25519 = "ED25519 (SHA-256 of the public key)"
)

// fingerprint is one shape a key can present, tagged with the form that produced
// it so a match can be logged without ever printing the value itself.
type fingerprint struct {
	form  string
	value string
}

// privateKeyFingerprints is every fingerprint shape the private key in body can
// produce.
//
// An encrypted key returns the *ssh.PassphraseMissingError from the parser
// unchanged and no fingerprints: the scan must never prompt for a passphrase, so
// such a key is matched through its sibling .pub (public-key forms) instead.
// A key type that is not an EC2 key pair type (ECDSA, DSA) yields no candidate.
func privateKeyFingerprints(body []byte) ([]fingerprint, error) {
	key, err := gossh.ParseRawPrivateKey(body)
	if err != nil {
		return nil, err
	}

	switch k := key.(type) {
	case *rsa.PrivateKey:
		var fps []fingerprint
		if der, err := x509.MarshalPKCS8PrivateKey(k); err == nil {
			sum := sha1.Sum(der)
			fps = append(fps, fingerprint{formCreatedRSA, colonHex(sum[:])})
		}
		// When the PEM already holds a PKCS#8 block ("PRIVATE KEY"), its bytes are
		// the exact DER EC2 hashed; hash them too, in case re-marshalling ever
		// normalises the encoding differently.
		if block, _ := pem.Decode(body); block != nil && block.Type == "PRIVATE KEY" {
			sum := sha1.Sum(block.Bytes)
			fps = append(fps, fingerprint{formCreatedRSA, colonHex(sum[:])})
		}
		if pub, err := gossh.NewPublicKey(&k.PublicKey); err == nil {
			fps = append(fps, publicKeyFingerprints(pub)...)
		}
		return fps, nil
	case ed25519.PrivateKey:
		pub, err := gossh.NewPublicKey(k.Public())
		if err != nil {
			return nil, err
		}
		return publicKeyFingerprints(pub), nil
	case *ed25519.PrivateKey:
		pub, err := gossh.NewPublicKey(k.Public())
		if err != nil {
			return nil, err
		}
		return publicKeyFingerprints(pub), nil
	default:
		return nil, nil
	}
}

// publicKeyFingerprints is every fingerprint shape a public key can produce: the
// two imported-RSA MD5 forms for an RSA key, the SHA-256 form for an ED25519 key.
// The created-RSA form needs the private key and so is not among them.
func publicKeyFingerprints(pub gossh.PublicKey) []fingerprint {
	ck, ok := pub.(gossh.CryptoPublicKey)
	if !ok {
		return nil
	}
	var fps []fingerprint
	switch cpub := ck.CryptoPublicKey().(type) {
	case *rsa.PublicKey:
		if der, err := x509.MarshalPKIXPublicKey(cpub); err == nil {
			sum := md5.Sum(der)
			fps = append(fps, fingerprint{formImportedRSAPKIX, colonHex(sum[:])})
		}
		fps = append(fps, fingerprint{formImportedRSAWire, gossh.FingerprintLegacyMD5(pub)})
	case ed25519.PublicKey:
		fps = append(fps, fingerprint{formED25519, gossh.FingerprintSHA256(pub)})
	}
	return fps
}

// colonHex renders bytes as the lower-case colon-separated hex EC2 shows for its
// MD5 and SHA-1 fingerprints, e.g. "16:9f:93:...".
func colonHex(b []byte) string {
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = hex.EncodeToString([]byte{c})
	}
	return strings.Join(parts, ":")
}

// colonHexFingerprint matches the colon-hex shape MD5 and SHA-1 fingerprints take.
var colonHexFingerprint = regexp.MustCompile(`^([0-9A-Fa-f]{2}:)+[0-9A-Fa-f]{2}$`)

// normaliseFingerprint puts a fingerprint into a shape two sources can be compared
// in: colon hex is lower-cased (hex is case-insensitive), everything else has a
// leading "SHA256:" and trailing base64 "=" padding stripped (ssh-keygen omits the
// padding and prefixes the digest, EC2 does neither) and is compared as given.
func normaliseFingerprint(s string) string {
	s = strings.TrimSpace(s)
	if colonHexFingerprint.MatchString(s) {
		return strings.ToLower(s)
	}
	s = strings.TrimPrefix(s, "SHA256:")
	return strings.TrimRight(s, "=")
}

// matchFingerprint reports the first candidate shape equal, once normalised, to the
// fingerprint EC2 recorded. An empty recorded fingerprint matches nothing.
func matchFingerprint(recorded string, candidates []fingerprint) (fingerprint, bool) {
	want := normaliseFingerprint(recorded)
	if want == "" {
		return fingerprint{}, false
	}
	for _, c := range candidates {
		if normaliseFingerprint(c.value) == want {
			return c, true
		}
	}
	return fingerprint{}, false
}
