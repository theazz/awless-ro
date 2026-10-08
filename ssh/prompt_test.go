package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// fakeTerminal answers the host-key question from a script, so the prompt can be
// tested without a tty. It counts reads and records every message, which is how the
// tests prove the question is asked at most once. The lock is there because the
// question is asked from the SSH handshake goroutine.
type fakeTerminal struct {
	mu          sync.Mutex
	interactive bool
	answers     []string
	readErr     error
	// onRead, when set, is called on every read: a test that must not be asked
	// anything fails from here.
	onRead func()

	reads    int
	prompts  []string
	messages strings.Builder
}

func (f *fakeTerminal) isInteractive() bool { return f.interactive }

func (f *fakeTerminal) readLine(prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onRead != nil {
		f.onRead()
	}
	f.reads++
	f.prompts = append(f.prompts, prompt)
	if f.readErr != nil {
		return "", f.readErr
	}
	if len(f.answers) == 0 {
		return "", io.EOF
	}
	answer := f.answers[0]
	f.answers = f.answers[1:]
	return answer, nil
}

func (f *fakeTerminal) message(format string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fmt.Fprintf(&f.messages, format, a...)
}

func (f *fakeTerminal) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *fakeTerminal) written() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messages.String()
}

func testPublicKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// Without a terminal there is no one to answer, so the prompt must fail before
// asking, with a message that says what to do, rather than read EOF as "no" and let
// the caller ask again.
func TestConfirmHostKeyWithoutTerminal(t *testing.T) {
	key := testPublicKey(t)
	fake := &fakeTerminal{interactive: false}

	ok, err := confirmHostKey(fake, "192.0.2.10:22", key, "/home/someone/.ssh/known_hosts")
	if ok {
		t.Fatal("an unknown host key was trusted without a terminal")
	}
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("got %v, want ErrNoTerminal", err)
	}
	for _, want := range []string{"192.0.2.10:22", gossh.FingerprintSHA256(key), "/home/someone/.ssh/known_hosts"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if fake.reads != 0 {
		t.Errorf("read %d answers from a non-terminal, want 0", fake.reads)
	}
	if fake.written() != "" {
		t.Errorf("wrote %q to a non-terminal, want nothing", fake.written())
	}
}

func TestConfirmHostKeyAnswers(t *testing.T) {
	key := testPublicKey(t)

	cases := []struct {
		answer string
		want   bool
	}{
		{"yes", true},
		{"YES ", true},
		{"no", false},
		{"", false},
	}
	for _, tc := range cases {
		fake := &fakeTerminal{interactive: true, answers: []string{tc.answer}}
		got, err := confirmHostKey(fake, "192.0.2.10:22", key, "known_hosts")
		if err != nil {
			t.Fatalf("answer %q: %v", tc.answer, err)
		}
		if got != tc.want {
			t.Errorf("answer %q: got %t, want %t", tc.answer, got, tc.want)
		}
		if fake.reads != 1 {
			t.Errorf("answer %q: %d reads, want 1", tc.answer, fake.reads)
		}
		if !strings.Contains(fake.written(), gossh.FingerprintSHA256(key)) {
			t.Errorf("answer %q: the fingerprint was not shown: %q", tc.answer, fake.written())
		}
	}

	// A read error ends it: no retry.
	fake := &fakeTerminal{interactive: true, readErr: io.EOF}
	got, err := confirmHostKey(fake, "192.0.2.10:22", key, "known_hosts")
	if got || !errors.Is(err, io.EOF) {
		t.Fatalf("read error: got (%t, %v), want (false, EOF)", got, err)
	}
	if fake.reads != 1 {
		t.Errorf("read error: %d reads, want exactly 1", fake.reads)
	}
}
