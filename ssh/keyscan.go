package ssh

import (
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/theazz/awless-ro/logger"

	gossh "golang.org/x/crypto/ssh"
)

// maxKeyBytes bounds how much of a candidate file the scan reads. Private keys are
// a few kilobytes; this is large enough for a 4096-bit RSA key and small enough
// that scanning a folder of unrelated files is cheap and cannot be made to read a
// huge file into memory.
const maxKeyBytes = 16 << 10

// findPrivateKey resolves a key ref to a private key file, in the order:
//
//	(a)/(b) by name, exactly as `ssh` always has (findPrivateKeyFromName): the -i
//	        value, or a file named like the key pair, in the key folders;
//	(c)     by fingerprint, only when the key pair's fingerprint is known: a scan
//	        of the same folders for any private key whose fingerprint matches the
//	        one EC2 recorded, however the file is named.
//
// With -i the ref carries no fingerprint, so only the name lookup runs.
func findPrivateKey(ref KeyRef, keyFolders ...string) (privateKey, bool) {
	if priv, ok := findPrivateKeyFromName(ref.Name, keyFolders...); ok {
		return priv, true
	}
	if ref.Fingerprint == "" {
		return privateKey{}, false
	}
	return findPrivateKeyByFingerprint(ref.Name, ref.Fingerprint, keyFolders...)
}

// keyNotFoundError says the key pair's key was found neither by name nor, when its
// fingerprint is known, by matching it, naming where it looked and how to override.
func keyNotFoundError(ref KeyRef, folders []string) error {
	reason := "and its fingerprint is not in the synced graph, so it cannot be matched (run `awless-ro sync`)"
	if ref.Fingerprint != "" {
		reason = "and no key there has the fingerprint AWS recorded for it"
	}
	return fmt.Errorf("%w for key pair %q in %s, %s; pass the key with -i /path/to/key", ErrKeyNotFound, ref.Name, strings.Join(folders, ", "), reason)
}

// findPrivateKeyByFingerprint scans the top level of each key folder, in order, for
// a private key whose fingerprint matches the one EC2 recorded for keyPair. The
// first match wins.
//
// The scan is deliberately conservative (see the per-file checks below): it never
// recurses, never follows a symlink out of a folder, never reads a file that is not
// a plausible key, never prompts for a passphrase, and refuses a matching key whose
// permissions are too open — a read-only tool must not quietly use a world-readable
// key. Nothing about a key's bytes or its fingerprint is ever logged.
func findPrivateKeyByFingerprint(keyPair, recorded string, keyFolders ...string) (privateKey, bool) {
	for _, folder := range keyFolders {
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(folder, entry.Name())
			priv, matched, form, ok := matchKeyFile(path, entry, recorded, keyFolders)
			if !ok {
				continue
			}
			if !matched {
				continue
			}
			logger.Verbosef("using %q for key pair %q: its %s fingerprint matches the one AWS recorded", path, keyPair, form)
			return priv, true
		}
	}
	return privateKey{}, false
}

// matchKeyFile decides whether one directory entry is a usable private key file
// matching recorded. ok is false when the entry is not even a candidate (not a
// regular file, wrong size, not a PEM private key, unreadable); matched reports a
// fingerprint match, and form names the shape that matched.
func matchKeyFile(path string, entry os.DirEntry, recorded string, keyFolders []string) (priv privateKey, matched bool, form string, ok bool) {
	// A symlink, directory, socket or device is never followed or read: that is
	// how the scan stays inside the key folders.
	if !entry.Type().IsRegular() {
		logger.ExtraVerbosef("skipping %q: not a regular file", path)
		return priv, false, "", false
	}
	info, err := entry.Info()
	if err != nil {
		return priv, false, "", false
	}
	// entry.Type() can be unknown on some filesystems; the Lstat-based mode is the
	// authoritative check that this is a regular file and not a symlink.
	if !info.Mode().IsRegular() {
		logger.ExtraVerbosef("skipping %q: not a regular file", path)
		return priv, false, "", false
	}
	if info.Size() == 0 || info.Size() > maxKeyBytes {
		logger.ExtraVerbosef("skipping %q: size %d is not that of a private key", path, info.Size())
		return priv, false, "", false
	}
	if escapesFolders(path, keyFolders) {
		logger.ExtraVerbosef("skipping %q: it resolves outside the key folders", path)
		return priv, false, "", false
	}

	body, err := readKeyFile(path, info)
	if err != nil {
		logger.ExtraVerbosef("skipping %q: %s", path, err)
		return priv, false, "", false
	}
	block, _ := pem.Decode(body)
	if block == nil || !strings.HasSuffix(block.Type, "PRIVATE KEY") {
		logger.ExtraVerbosef("skipping %q: not a PEM private key", path)
		return priv, false, "", false
	}

	candidates, err := privateKeyFingerprints(body)
	var missing *gossh.PassphraseMissingError
	if errors.As(err, &missing) {
		// An encrypted key: never prompt. Match it only through its sibling .pub,
		// using the key's own embedded public key (OpenSSH format) only to check
		// the .pub really belongs to it.
		candidates = encryptedKeyCandidates(path, keyFolders, missing.PublicKey)
		if len(candidates) == 0 {
			logger.ExtraVerbosef("skipping %q: encrypted and no usable .pub beside it", path)
			return priv, false, "", false
		}
	} else if err != nil {
		logger.ExtraVerbosef("skipping %q: %s", path, err)
		return priv, false, "", false
	}

	fp, matched := matchFingerprint(recorded, candidates)
	if !matched {
		return priv, false, "", true
	}
	if loosePermissions(info) {
		logger.Warningf("private key %q matches key pair but its permissions are too open; openssh would refuse it, so it is not used (chmod 600 it)", path)
		return priv, false, "", true
	}
	return privateKey{path: path, body: body}, true, fp.form, true
}

// readKeyFile reads up to maxKeyBytes of path, guarding against the file being
// swapped between the directory listing and the open: it reopens and checks the
// handle is the same file the listing described.
func readKeyFile(path string, listed os.FileInfo) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(listed, opened) {
		return nil, errors.New("file changed under the scan")
	}
	// One byte past the limit so an over-size file is noticed rather than silently
	// truncated into something that might parse.
	body, err := io.ReadAll(io.LimitReader(f, maxKeyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxKeyBytes {
		return nil, errors.New("larger than a private key")
	}
	return body, nil
}

// encryptedKeyCandidates is the public-key fingerprint shapes to match an encrypted
// private key by, read from its sibling <path>.pub. embeddedPub, when the OpenSSH
// format gave one without the passphrase, is used only to confirm the .pub belongs
// to this key; a mismatch skips the file. The passphrase is never asked for.
func encryptedKeyCandidates(path string, keyFolders []string, embeddedPub gossh.PublicKey) []fingerprint {
	pubPath := path + ".pub"
	info, err := os.Lstat(pubPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxKeyBytes {
		return nil
	}
	if escapesFolders(pubPath, keyFolders) {
		return nil
	}
	body, err := readKeyFile(pubPath, info)
	if err != nil {
		return nil
	}
	pub, _, _, _, err := gossh.ParseAuthorizedKey(body)
	if err != nil {
		return nil
	}
	if embeddedPub != nil && !equalPublicKeys(embeddedPub, pub) {
		logger.ExtraVerbosef("skipping %q: its .pub does not match the key", path)
		return nil
	}
	return publicKeyFingerprints(pub)
}

func equalPublicKeys(a, b gossh.PublicKey) bool {
	return a.Type() == b.Type() && string(a.Marshal()) == string(b.Marshal())
}

// loosePermissions reports whether a private key's mode is readable by anyone other
// than its owner. On Windows the Unix permission bits are meaningless (Go reports
// 0666 for every file), so the check does not apply there.
func loosePermissions(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return info.Mode().Perm()&0o077 != 0
}
