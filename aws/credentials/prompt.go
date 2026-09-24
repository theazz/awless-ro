/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package awscredentials

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"

	awsconfig "github.com/theazz/awless-ro/aws/config"
)

// ErrNoTerminal says credentials are missing and there is no one to ask.
var ErrNoTerminal = errors.New("no AWS credentials found, and stdin is not a terminal so they cannot be asked for")

// ErrProfileExists says the profile already has a section in the shared credentials
// file.
var ErrProfileExists = errors.New("the profile already exists in the AWS credentials file")

// SharedCredentialsPath is where a new profile is written: the same file the AWS CLI
// and every other SDK read.
func SharedCredentialsPath() string {
	return filepath.Join(awsconfig.AWSHomeDir(), "credentials")
}

// Access keys are opaque, and AWS has changed their shape before, so these patterns
// only catch what is obviously not a key: the wrong alphabet, or a length nowhere
// near right. A paste that picked up surrounding whitespace is trimmed rather than
// rejected, because that is the common accident and not an error on the user's part.
var (
	accessKeyIDPattern = regexp.MustCompile(`^[A-Z0-9]{16,128}$`)
	secretKeyPattern   = regexp.MustCompile(`^[A-Za-z0-9/+=]{16,128}$`)
	profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9._@:/+-]{1,64}$`)
)

// terminal is the interaction this package needs, gathered behind an interface so
// the tests can drive it without a tty.
type terminal interface {
	// isInteractive reports whether there is a person to ask.
	isInteractive() bool
	// readLine reads a visible answer.
	readLine(prompt string) (string, error)
	// readSecret reads an answer without echoing it.
	readSecret(prompt string) (string, error)
	// message writes to the user, never to stdout: stdout carries command output
	// that people pipe.
	message(format string, a ...interface{})
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

func (t *stdTerminal) readSecret(prompt string) (string, error) {
	fmt.Fprint(t.out, prompt)
	// ReadPassword turns off echo for the duration, which is the point: upstream
	// read the secret with fmt.Scanln, so it appeared on screen and stayed in the
	// scrollback.
	raw, err := term.ReadPassword(int(t.in.Fd()))
	fmt.Fprintln(t.out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func (t *stdTerminal) message(format string, a ...interface{}) {
	fmt.Fprintf(t.out, format, a...)
}

// newProfile asks for access keys and writes them to the shared credentials file,
// returning the profile they were stored under.
//
// The number of attempts is bounded. Upstream looped until it got a non-empty
// answer, which on a closed stdin never ends: it spun printing an error per
// iteration. Here a read failure is fatal on the spot.
func newProfile(t terminal, suggested string) (profile string, err error) {
	if !t.isInteractive() {
		return "", ErrNoTerminal
	}

	path := SharedCredentialsPath()

	profile, err = askProfileName(t, suggested)
	if err != nil {
		return "", err
	}

	// Checked before asking for the keys, so that a user who cannot be helped is
	// not made to type a secret first.
	exists, err := profileSectionExists(path, profile)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("%w: %q is already defined in %s. Use `aws configure --profile %s` to change it",
			ErrProfileExists, profile, path, profile)
	}

	t.message("\nEnter the access keys for profile %q. They will be stored in %s\n", profile, path)

	keyID, err := askUntilValid(t, "AWS Access Key ID: ", false, func(s string) error {
		if !accessKeyIDPattern.MatchString(s) {
			return errors.New("an access key id is 16 or more upper-case letters and digits")
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	secret, err := askUntilValid(t, "AWS Secret Access Key: ", true, func(s string) error {
		if !secretKeyPattern.MatchString(s) {
			return errors.New("a secret access key is 16 or more letters, digits and /+=")
		}
		if s == keyID {
			return errors.New("that is the access key id again, not the secret")
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if err := appendProfile(path, profile, keyID, secret); err != nil {
		return "", err
	}

	t.message("\nStored credentials for profile %q in %s\n", profile, path)
	return profile, nil
}

func askProfileName(t terminal, suggested string) (string, error) {
	if suggested == "" {
		suggested = "default"
	}
	answer, err := t.readLine(fmt.Sprintf("Profile name [%s]: ", suggested))
	if err != nil {
		return "", fmt.Errorf("reading the profile name: %w", err)
	}
	if answer == "" {
		answer = suggested
	}
	if !profileNamePattern.MatchString(answer) {
		return "", fmt.Errorf("%q is not a usable profile name", answer)
	}
	return answer, nil
}

// askUntilValid gives the user a few tries, because mistyping a forty-character
// secret is normal, then gives up rather than looping.
func askUntilValid(t terminal, prompt string, secret bool, validate func(string) error) (string, error) {
	const attempts = 3

	read := t.readLine
	if secret {
		read = t.readSecret
	}

	for i := range attempts {
		answer, err := read(prompt)
		if err != nil {
			return "", fmt.Errorf("reading input: %w", err)
		}
		if err := validate(answer); err != nil {
			t.message("  %s\n", err)
			continue
		}
		_ = i
		return answer, nil
	}
	return "", fmt.Errorf("no valid value after %d attempts", attempts)
}

var profileSectionPattern = regexp.MustCompile(`(?m)^\s*\[\s*(?:profile\s+)?([^\]]+?)\s*\]`)

// profileSectionExists reports whether the file already defines the profile.
//
// Upstream appended a section without looking, so a profile defined twice was left
// for the ini parser to disambiguate, and the user's real keys could end up shadowed
// by ours.
func profileSectionExists(path, profile string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	for _, match := range profileSectionPattern.FindAllSubmatch(content, -1) {
		if string(match[1]) == profile {
			return true, nil
		}
	}
	return false, nil
}

func appendProfile(path, profile, keyID, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	// A leading newline, in case the file does not end with one; an extra blank
	// line is harmless to every ini parser, a missing one is not.
	section := fmt.Sprintf("\n[%s]\naws_access_key_id = %s\naws_secret_access_key = %s\n", profile, keyID, secret)
	if _, err := f.WriteString(section); err != nil {
		return fmt.Errorf("writing to %s: %w", path, err)
	}

	return f.Close()
}
