package awscredentials

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsconfig "github.com/theazz/awless-ro/aws/config"
)

// fakeTerminal answers from a script, so the prompting logic can be tested without a
// tty. It records what was read as a secret, which is how the test checks the secret
// was not echoed.
type fakeTerminal struct {
	interactive bool
	answers     []string
	next        int

	readAsSecret  []string
	readAsVisible []string
	messages      strings.Builder
	readErr       error
}

func (f *fakeTerminal) isInteractive() bool { return f.interactive }

func (f *fakeTerminal) take() (string, error) {
	if f.readErr != nil {
		return "", f.readErr
	}
	if f.next >= len(f.answers) {
		return "", io.EOF
	}
	answer := f.answers[f.next]
	f.next++
	return answer, nil
}

func (f *fakeTerminal) readLine(prompt string) (string, error) {
	answer, err := f.take()
	if err == nil {
		f.readAsVisible = append(f.readAsVisible, prompt)
	}
	return answer, err
}

func (f *fakeTerminal) readSecret(prompt string) (string, error) {
	answer, err := f.take()
	if err == nil {
		f.readAsSecret = append(f.readAsSecret, prompt)
	}
	return answer, err
}

func (f *fakeTerminal) message(format string, a ...interface{}) {
	fmt.Fprintf(&f.messages, format, a...)
}

// withAWSHome points the package at a throwaway ~/.aws for the duration of a test.
func withAWSHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	previous := awsconfig.AWSHomeDir
	awsconfig.AWSHomeDir = func() string { return filepath.Join(home, ".aws") }
	t.Cleanup(func() { awsconfig.AWSHomeDir = previous })
	return home
}

const (
	validKeyID  = "AKIAIOSFODNN7EXAMPLE"
	validSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

func TestNewProfileWritesTheSharedCredentialsFile(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{
		interactive: true,
		answers:     []string{"staging", validKeyID, validSecret},
	}

	profile, err := newProfile(term, "")
	if err != nil {
		t.Fatal(err)
	}
	if profile != "staging" {
		t.Errorf("profile = %q, want %q", profile, "staging")
	}

	content, err := os.ReadFile(SharedCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[staging]", "aws_access_key_id = " + validKeyID, "aws_secret_access_key = " + validSecret} {
		if !strings.Contains(string(content), want) {
			t.Errorf("credentials file is missing %q; got:\n%s", want, content)
		}
	}
}

// The secret must be read without echo. Upstream used fmt.Scanln, so it appeared on
// screen and stayed in the scrollback.
func TestSecretIsReadWithoutEcho(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{
		interactive: true,
		answers:     []string{"staging", validKeyID, validSecret},
	}
	if _, err := newProfile(term, ""); err != nil {
		t.Fatal(err)
	}

	if len(term.readAsSecret) != 1 {
		t.Fatalf("expected exactly one value read without echo, got %d: %v", len(term.readAsSecret), term.readAsSecret)
	}
	if !strings.Contains(term.readAsSecret[0], "Secret") {
		t.Errorf("the value read without echo was %q, expected the secret access key", term.readAsSecret[0])
	}
	for _, prompt := range term.readAsVisible {
		if strings.Contains(prompt, "Secret") {
			t.Errorf("the secret access key was read with echo on: %q", prompt)
		}
	}
	if strings.Contains(term.messages.String(), validSecret) {
		t.Error("the secret was written back to the terminal")
	}
}

func TestSuggestedProfileIsUsedWhenTheAnswerIsEmpty(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{
		interactive: true,
		answers:     []string{"", validKeyID, validSecret},
	}

	profile, err := newProfile(term, "work")
	if err != nil {
		t.Fatal(err)
	}
	if profile != "work" {
		t.Errorf("profile = %q, want %q", profile, "work")
	}
}

func TestProfileDefaultsToDefault(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{
		interactive: true,
		answers:     []string{"", validKeyID, validSecret},
	}

	profile, err := newProfile(term, "")
	if err != nil {
		t.Fatal(err)
	}
	if profile != "default" {
		t.Errorf("profile = %q, want %q", profile, "default")
	}
}

// Without a terminal there is no one to ask, and the answer has to be an error rather
// than a prompt into the void. Upstream looped until it got a non-empty line, which on
// a closed stdin never ended.
func TestNoTerminalMeansNoPrompt(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{interactive: false}

	_, err := newProfile(term, "prod")
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("got %v, want %v", err, ErrNoTerminal)
	}
	if term.next != 0 {
		t.Error("something was read from a non-interactive terminal")
	}
	if _, statErr := os.Stat(SharedCredentialsPath()); statErr == nil {
		t.Error("the credentials file was created without any input")
	}
}

// An existing section is left alone. Upstream appended regardless, so a profile ended
// up defined twice and which one won was up to the ini parser — the user's real keys
// could be shadowed by ours.
func TestExistingProfileIsNotTouched(t *testing.T) {
	withAWSHome(t)

	path := SharedCredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := "[prod]\naws_access_key_id = AKIAORIGINAL00000000\naws_secret_access_key = originalsecretoriginalsecret000000000000\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	term := &fakeTerminal{
		interactive: true,
		answers:     []string{"prod", validKeyID, validSecret},
	}

	_, err := newProfile(term, "prod")
	if !errors.Is(err, ErrProfileExists) {
		t.Fatalf("got %v, want %v", err, ErrProfileExists)
	}
	if !strings.Contains(err.Error(), "aws configure") {
		t.Errorf("the error should say what to do instead, got %q", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Errorf("the credentials file was modified:\n%s", content)
	}

	// The keys must not have been asked for at all: there was nothing that could be
	// done with them.
	for _, prompt := range append(term.readAsVisible, term.readAsSecret...) {
		if strings.Contains(prompt, "Key") {
			t.Errorf("access keys were asked for before the profile was found to exist: %q", prompt)
		}
	}
}

func TestProfileSectionExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	content := "" +
		"[default]\n" +
		"aws_access_key_id = x\n" +
		"\n" +
		"  [ spaced ]\n" +
		"\n" +
		"[profile with-prefix]\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		profile string
		want    bool
	}{
		{"default", true},
		{"spaced", true},
		{"with-prefix", true},
		{"missing", false},
		{"defaul", false},
		{"defaultx", false},
	}
	for _, tc := range cases {
		got, err := profileSectionExists(path, tc.profile)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("profileSectionExists(%q) = %v, want %v", tc.profile, got, tc.want)
		}
	}

	// A file that is not there yet defines nothing, which is the first-run case and
	// not an error.
	got, err := profileSectionExists(filepath.Join(dir, "absent"), "default")
	if err != nil {
		t.Fatalf("a missing file should not be an error: %s", err)
	}
	if got {
		t.Error("a missing file reported an existing profile")
	}
}

func TestKeyValidation(t *testing.T) {
	cases := []struct {
		name          string
		keyID, secret string
		wantErr       bool
	}{
		{name: "valid", keyID: validKeyID, secret: validSecret},
		{name: "lower case key id", keyID: strings.ToLower(validKeyID), secret: validSecret, wantErr: true},
		{name: "short key id", keyID: "AKIA", secret: validSecret, wantErr: true},
		{name: "key id with space", keyID: "AKIA IOSFODNN7EXAMPLE", secret: validSecret, wantErr: true},
		{name: "short secret", keyID: validKeyID, secret: "tooshort", wantErr: true},
		{name: "secret with quotes", keyID: validKeyID, secret: `"` + validSecret + `"`, wantErr: true},
		{name: "secret equal to key id", keyID: validKeyID, secret: validKeyID, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withAWSHome(t)

			// Three attempts each, so an invalid value exhausts them and fails
			// rather than blocking.
			answers := []string{"prof"}
			for range 3 {
				answers = append(answers, tc.keyID, tc.secret)
			}
			term := &fakeTerminal{interactive: true, answers: answers}

			_, err := newProfile(term, "")
			if tc.wantErr && err == nil {
				t.Errorf("expected an error for key %q secret %q", tc.keyID, tc.secret)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}

// Keys are usually pasted, and a paste often brings whitespace or a newline with it.
// That is a slip, not a wrong key, so the reader trims rather than the validator
// rejecting.
func TestReadLineTrimsSurroundingWhitespace(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		fmt.Fprintf(w, "  %s  \n", validKeyID)
		fmt.Fprintf(w, "\t%s\r\n", "second")
		w.Close()
	}()

	term := &stdTerminal{in: r, out: io.Discard, r: bufio.NewReader(r)}

	got, err := term.readLine("")
	if err != nil {
		t.Fatal(err)
	}
	if got != validKeyID {
		t.Errorf("got %q, want %q", got, validKeyID)
	}

	got, err = term.readLine("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// The prompt goes to stderr, never to stdout: stdout carries command output that
// people pipe into other things.
func TestPromptsGoToStderr(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		fmt.Fprintln(w, "answer")
		w.Close()
	}()

	var out strings.Builder
	term := &stdTerminal{in: r, out: &out, r: bufio.NewReader(r)}
	if _, err := term.readLine("Profile name: "); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Profile name: ") {
		t.Errorf("the prompt did not reach the configured writer, got %q", out.String())
	}
}

// A read that fails stops the prompt instead of being retried forever.
func TestReadFailureIsFatal(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{interactive: true, readErr: errors.New("stdin closed")}

	if _, err := newProfile(term, "prod"); err == nil {
		t.Fatal("expected an error when input cannot be read")
	}
}

func TestInvalidProfileNameIsRejected(t *testing.T) {
	withAWSHome(t)

	for _, name := range []string{"has space", "[brackets]", strings.Repeat("x", 65), "tab\there"} {
		term := &fakeTerminal{interactive: true, answers: []string{name, validKeyID, validSecret}}
		if _, err := newProfile(term, ""); err == nil {
			t.Errorf("profile name %q was accepted", name)
		}
	}
}

func TestAppendKeepsExistingContent(t *testing.T) {
	withAWSHome(t)

	path := SharedCredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Deliberately without a trailing newline, which is the case that would
	// otherwise glue our section onto the last line.
	original := "[default]\naws_access_key_id = AKIAORIGINAL00000000"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	term := &fakeTerminal{interactive: true, answers: []string{"extra", validKeyID, validSecret}}
	if _, err := newProfile(term, ""); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), original) {
		t.Errorf("existing content was changed:\n%s", content)
	}
	if !strings.Contains(string(content), "\n[extra]\n") {
		t.Errorf("the new section did not start on its own line:\n%q", content)
	}
}

func TestCreatedCredentialsFilePermissions(t *testing.T) {
	withAWSHome(t)

	term := &fakeTerminal{interactive: true, answers: []string{"prof", validKeyID, validSecret}}
	if _, err := newProfile(term, ""); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(SharedCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("credentials file mode %#o, want 0600", got)
	}

	dirInfo, err := os.Stat(filepath.Dir(SharedCredentialsPath()))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf(".aws directory mode %#o, want 0700", got)
	}
}
