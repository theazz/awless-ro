package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

func agentAuth() (ssh.AuthMethod, error) {
	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeysCallback(agent.NewClient(sock).Signers), nil
}

func privateKeyAuth(priv privateKey) (ssh.AuthMethod, error) {
	signer, err := ssh.ParsePrivateKey(priv.body)
	if err != nil {
		// x/crypto reports a missing passphrase with a dedicated type. Upstream
		// matched on the text of the message instead, which silently stops working
		// the day that wording changes — and it already had: the string it looked
		// for belongs to an older release.
		var needsPassphrase *ssh.PassphraseMissingError
		if errors.As(err, &needsPassphrase) {
			return encryptedPrivKeyAuth(priv)
		}
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

// encryptedPrivKeyAuth prompts for the passphrase and decrypts the key.
//
// Decryption is left to ssh.ParsePrivateKeyWithPassphrase. Upstream rolled its own
// on x509.DecryptPEMBlock, which the standard library documents as insecure — the
// encryption it implements is unauthenticated, so the ciphertext can be tampered
// with undetected — and which only ever understood PKCS#1 RSA. That excluded the
// modern OpenSSH format and every ed25519 key, so those simply failed to load.
func encryptedPrivKeyAuth(priv privateKey) (ssh.AuthMethod, error) {
	fmt.Fprintf(os.Stderr, "This SSH key is encrypted. Please enter passphrase for key '%s':", priv.path)
	passphrase, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr)

	signer, err := ssh.ParsePrivateKeyWithPassphrase(priv.body, passphrase)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}
