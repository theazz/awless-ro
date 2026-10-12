// Package sshtest is an in-process SSH server for tests that need to dial one, on
// 127.0.0.1, without any network beyond the loopback and without the system ssh.
// It is the same idea as graph/resourcetest: a non-test file so tests in other
// packages can import it. It authenticates a single user with a single key and
// counts connections; it serves no channels, which is all a dial-and-authenticate
// test needs.
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Server is a running loopback SSH server. Stop it is not needed: it is torn down
// with the test that started it.
type Server struct {
	Host    string
	Port    int
	Addr    string
	hostKey gossh.PublicKey
	conns   atomic.Int32
}

// Conns is how many TCP connections the server has accepted.
func (s *Server) Conns() int { return int(s.conns.Load()) }

// KnownHostsLine is the server's address and host key as a known_hosts line, to
// seed a client's known_hosts so the dial does not prompt.
func (s *Server) KnownHostsLine() string {
	return knownhosts.Line([]string{s.Addr}, s.hostKey)
}

// Start brings up a server that lets in allowUser authenticating with clientKey.
func Start(t testing.TB, allowUser string, clientKey gossh.PublicKey) *Server {
	t.Helper()

	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := gossh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(meta gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if meta.User() == allowUser && string(key.Marshal()) == string(clientKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("user %s not allowed", meta.User())
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	srv := &Server{Host: host, Addr: ln.Addr().String(), hostKey: hostSigner.PublicKey()}
	srv.Port, _ = strconv.Atoi(port)

	var mu sync.Mutex
	var open []net.Conn
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range open {
			c.Close()
		}
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			srv.conns.Add(1)
			mu.Lock()
			open = append(open, conn)
			mu.Unlock()
			go func(c net.Conn) {
				sshConn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				go gossh.DiscardRequests(reqs)
				for nc := range chans {
					nc.Reject(gossh.UnknownChannelType, "no channels")
				}
				sshConn.Close()
			}(conn)
		}
	}()
	return srv
}
