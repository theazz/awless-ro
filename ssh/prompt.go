package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// ErrNoTerminal says a host key needs confirming and there is no one to ask.
var ErrNoTerminal = errors.New("the host key is not known and stdin is not a terminal, so it cannot be confirmed")

// terminal is the interaction the host-key prompt needs, gathered behind an
// interface so the tests can drive it without a tty. Same shape as the credentials
// prompt in aws/credentials.
type terminal interface {
	// isInteractive reports whether there is a person to ask.
	isInteractive() bool
	// readLine reads a visible answer.
	readLine(prompt string) (string, error)
	// message writes to the user, never to stdout: stdout carries command output
	// that people pipe.
	message(format string, a ...any)
}

type stdTerminal struct {
	in  *os.File
	out io.Writer
	r   *bufio.Reader
}

func newStdTerminal() *stdTerminal {
	return &stdTerminal{in: os.Stdin, out: os.Stderr, r: bufio.NewReader(os.Stdin)}
}

func (t *stdTerminal) isInteractive() bool { return term.IsTerminal(int(t.in.Fd())) }

func (t *stdTerminal) readLine(prompt string) (string, error) {
	fmt.Fprint(t.out, prompt)
	line, err := t.r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (t *stdTerminal) message(format string, a ...any) {
	fmt.Fprintf(t.out, format, a...)
}

// hostKeyTerminal is where an unknown host key is asked about. A variable so the
// tests can substitute a scripted terminal.
var hostKeyTerminal terminal = newStdTerminal()

// confirmHostKey asks once whether to trust and persist an unknown host key.
//
// Upstream read the answer with fmt.Scanln and no terminal check, so a closed stdin
// read as "no", the caller moved on to the next candidate user, and the question was
// printed again for every one of them. Without a terminal this fails before asking,
// and a read error is returned rather than retried.
func confirmHostKey(t terminal, hostname string, key gossh.PublicKey, file string) (bool, error) {
	if !t.isInteractive() {
		return false, fmt.Errorf("%w: %s presented a %s key with fingerprint %s. Connect once from a terminal to review and accept it, or add it to %s yourself",
			ErrNoTerminal, hostname, key.Type(), gossh.FingerprintSHA256(key), file)
	}

	t.message("awless-ro could not validate the authenticity of '%s' (unknown host)\n", hostname)
	t.message("%s public key fingerprint is %s.\n", key.Type(), gossh.FingerprintSHA256(key))
	answer, err := t.readLine(fmt.Sprintf("Do you want to continue connecting and persist this key to '%s' (yes/no)? ", file))
	if err != nil {
		return false, fmt.Errorf("reading the answer about the host key of %s: %w", hostname, err)
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}
