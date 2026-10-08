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

package console

import (
	"fmt"
	"io"
	"os"
	"os/signal"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func GetTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// StdoutTerminalWidth reports the width of the terminal stdout is, and false when
// stdout is not a terminal (a pipe, a file) or its size cannot be read.
func StdoutTerminalWidth() (int, bool) {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return 0, false
	}
	w, _, err := term.GetSize(fd)
	if err != nil || w <= 0 {
		return 0, false
	}
	return w, true
}

// TableWidth is the width tables are laid out in, 0 meaning no limit.
//
// An explicit width (--max-width) wins, 0 included, terminal or not. Otherwise a
// terminal's own width is used, and output that is not going to a terminal has no
// limit at all: one row per line however long, as ps, docker ps and kubectl get do,
// so a pipe or a file gets whole values rather than ones broken for a screen nobody
// is looking at. COLUMNS is deliberately not read: on a terminal the ioctl is
// authoritative, and in a pipe it is usually an unexported shell variable, so
// honouring it would make the output depend on the shell.
//
// terminalWidth is StdoutTerminalWidth outside tests.
func TableWidth(explicit int, explicitSet bool, terminalWidth func() (int, bool)) int {
	if explicitSet {
		return explicit
	}
	if w, ok := terminalWidth(); ok && w > 0 {
		return w
	}
	return 0
}

func GetTerminalHeight() int {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return h
}

func InteractiveTerminal(client *ssh.Client) error {
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go io.Copy(stdin, os.Stdin)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdout)

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, stderr)

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	// Request pseudo terminal
	width := GetTerminalWidth()
	if width == 0 {
		width = 100
	}
	height := GetTerminalHeight()
	if height == 0 {
		height = 100
	}
	if err := session.RequestPty("xterm", height, width, modes); err != nil {
		return err
	}

	// Start remote shell
	if err := session.Shell(); err != nil {
		return err
	}

	// Buffered: signal.Notify never blocks, so an unbuffered channel silently
	// drops signals that arrive while propagateSignals is busy.
	signalc := make(chan os.Signal, 1)
	defer func() {
		signal.Reset()
		close(signalc)
	}()
	go propagateSignals(signalc, session, stdin)
	signal.Notify(signalc, os.Interrupt, os.Kill)
	return session.Wait()
}

func propagateSignals(signalc chan os.Signal, session *ssh.Session, stdin io.WriteCloser) {
	for s := range signalc {
		switch s {
		case os.Interrupt:
			fmt.Fprint(stdin, "\x03")
		}
	}
}
