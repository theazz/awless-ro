package ssh

import (
	"bytes"
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// These tests run the client against a real SSH server, in process, on 127.0.0.1.
// No network beyond the loopback and no system ssh are involved.

func newSigner(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// writeClientKey writes a fresh private key to dir/name and returns its signer.
func writeClientKey(t *testing.T, dir, name string) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

type testServer struct {
	addr  string
	host  string
	port  int
	conns atomic.Int32
}

func newECDSASigner(t *testing.T) gossh.Signer {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// startServer serves SSH on 127.0.0.1 with hostKey (and moreHostKeys, as a real
// sshd offers one key per type), letting in only allowUser authenticating with
// clientKey. It forwards direct-tcpip channels, which is what a jump host does,
// and counts every TCP connection it accepts.
func startServer(t *testing.T, hostKey gossh.Signer, allowUser string, clientKey gossh.PublicKey, moreHostKeys ...gossh.Signer) *testServer {
	t.Helper()

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(meta gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if meta.User() == allowUser && bytes.Equal(key.Marshal(), clientKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("user %s not allowed", meta.User())
		},
	}
	cfg.AddHostKey(hostKey)
	for _, k := range moreHostKeys {
		cfg.AddHostKey(k)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &testServer{addr: ln.Addr().String()}
	host, port, _ := net.SplitHostPort(srv.addr)
	srv.host = host
	srv.port, _ = strconv.Atoi(port)

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
			go serveConn(conn, cfg)
		}
	}()
	return srv
}

func serveConn(conn net.Conn, cfg *gossh.ServerConfig) {
	defer conn.Close()
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "direct-tcpip" {
			nc.Reject(gossh.UnknownChannelType, "only direct-tcpip")
			continue
		}
		// RFC 4254 7.2.
		var target struct {
			Host     string
			Port     uint32
			OrigHost string
			OrigPort uint32
		}
		if err := gossh.Unmarshal(nc.ExtraData(), &target); err != nil {
			nc.Reject(gossh.ConnectionFailed, err.Error())
			continue
		}
		upstream, err := net.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port))))
		if err != nil {
			nc.Reject(gossh.ConnectionFailed, err.Error())
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			upstream.Close()
			continue
		}
		go gossh.DiscardRequests(chReqs)
		go func() {
			defer ch.Close()
			defer upstream.Close()
			done := make(chan struct{}, 2)
			go func() { io.Copy(upstream, ch); done <- struct{}{} }()
			go func() { io.Copy(ch, upstream); done <- struct{}{} }()
			<-done
		}()
	}
}

// useTerminal makes fake the terminal the host-key question goes to.
func useTerminal(t *testing.T, fake terminal) {
	t.Helper()
	previous := hostKeyTerminal
	hostKeyTerminal = fake
	t.Cleanup(func() { hostKeyTerminal = previous })
}

// within fails the test instead of hanging the suite when fn does not return: a
// regression back into the prompt loop would otherwise never finish.
func within(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("did not return within %s", d)
	}
}

type sshEnv struct {
	home, awlessHome, keyDir string
	clientKey                gossh.Signer
}

func (e sshEnv) opensshKnownHosts() string { return filepath.Join(e.home, ".ssh", "known_hosts") }
func (e sshEnv) awlessKnownHosts() string  { return filepath.Join(e.awlessHome, "known_hosts") }

// newSSHEnv isolates HOME, the awless home and the agent, and writes a client key.
func newSSHEnv(t *testing.T) sshEnv {
	t.Helper()
	root := t.TempDir()
	e := sshEnv{
		home:       filepath.Join(root, "home"),
		awlessHome: filepath.Join(root, "awless"),
		keyDir:     filepath.Join(root, "keys"),
	}
	for _, dir := range []string{filepath.Join(e.home, ".ssh"), e.awlessHome, e.keyDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", e.home)
	t.Setenv("__AWLESS_HOME", e.awlessHome)
	t.Setenv("SSH_AUTH_SOCK", "")
	e.clientKey = writeClientKey(t, e.keyDir, "client.pem")
	return e
}

func (e sshEnv) client(t *testing.T, srv *testServer) *Client {
	t.Helper()
	c, err := InitClient("client", e.keyDir)
	if err != nil {
		t.Fatal(err)
	}
	c.IP, c.Port = srv.host, srv.port
	t.Cleanup(func() { c.CloseAll() })
	return c
}

func writeKnownHost(t *testing.T, file, addr string, key gossh.PublicKey) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(knownhosts.Line([]string{addr}, key) + "\n"); err != nil {
		t.Fatal(err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s exists (err %v); nothing should have been persisted", path, err)
	}
}

func TestDialUnknownHostKeyAcceptedAndPersisted(t *testing.T) {
	env := newSSHEnv(t)
	hostKey := newSigner(t)
	srv := startServer(t, hostKey, "ec2-user", env.clientKey.PublicKey())
	fake := &fakeTerminal{interactive: true, answers: []string{"yes"}}
	useTerminal(t, fake)

	c := env.client(t, srv)
	var err error
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user") })
	if err != nil {
		t.Fatal(err)
	}
	if c.User != "ec2-user" {
		t.Errorf("user %q, want ec2-user", c.User)
	}
	if n := fake.readCount(); n != 1 {
		t.Errorf("asked %d times, want 1", n)
	}

	// No ~/.ssh/known_hosts, so the key goes to the awless one.
	content, err := os.ReadFile(env.awlessKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if want := knownhosts.Line([]string{srv.addr}, hostKey.PublicKey()); !strings.Contains(string(content), want) {
		t.Errorf("known_hosts is %q, want it to contain %q", content, want)
	}
	mustNotExist(t, env.opensshKnownHosts())
}

func TestDialKnownHostKey(t *testing.T) {
	env := newSSHEnv(t)
	hostKey := newSigner(t)
	srv := startServer(t, hostKey, "ec2-user", env.clientKey.PublicKey())
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, hostKey.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: true, onRead: func() {
		t.Error("asked about a host key that is already known")
	}})

	c := env.client(t, srv)
	var err error
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user") })
	if err != nil {
		t.Fatal(err)
	}
	if c.User != "ec2-user" {
		t.Errorf("user %q, want ec2-user", c.User)
	}
}

func TestDialChangedHostKeyRefused(t *testing.T) {
	env := newSSHEnv(t)
	srv := startServer(t, newSigner(t), "ec2-user", env.clientKey.PublicKey())
	// What we remember for this address is some other key.
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, newSigner(t).PublicKey())
	before, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeTerminal{interactive: true, answers: []string{"yes"}}
	useTerminal(t, fake)

	c := env.client(t, srv)
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu", "root") })
	if err == nil || !strings.Contains(err.Error(), "HAS CHANGED") {
		t.Fatalf("got %v, want the changed-host-key refusal", err)
	}
	if !IsHostKeyError(err) {
		t.Error("a changed host key is not reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 1 {
		t.Errorf("%d connections, want 1: a changed host key is not a reason to try another user", n)
	}
	if n := fake.readCount(); n != 0 {
		t.Errorf("asked %d times about a changed key, want 0", n)
	}
	after, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
	mustNotExist(t, env.awlessKnownHosts())
}

// The live-check defect: a host recorded with only its Ed25519 key, offering ECDSA
// and Ed25519 like a stock sshd. The client's default order picks ECDSA, which is
// not recorded, and a genuine host was refused as changed. Like OpenSSH, the
// recorded key type must be negotiated.
func TestDialPrefersRecordedHostKeyType(t *testing.T) {
	env := newSSHEnv(t)
	edKey, ecKey := newSigner(t), newECDSASigner(t)
	srv := startServer(t, ecKey, "ec2-user", env.clientKey.PublicKey(), edKey)
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, edKey.PublicKey())
	before, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	useTerminal(t, &fakeTerminal{interactive: true, onRead: func() {
		t.Error("asked about a host whose key is already known")
	}})

	c := env.client(t, srv)
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user") })
	if err != nil {
		t.Fatalf("a host known by its Ed25519 key was refused: %v", err)
	}
	after, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
	mustNotExist(t, env.awlessKnownHosts())
}

// The same on the second hop, where the jump host dials the destination.
func TestProxyHopPrefersRecordedHostKeyType(t *testing.T) {
	env := newSSHEnv(t)
	jumpEd, jumpEC := newSigner(t), newECDSASigner(t)
	destEd, destEC := newSigner(t), newECDSASigner(t)
	jump := startServer(t, jumpEC, "ec2-user", env.clientKey.PublicKey(), jumpEd)
	dest := startServer(t, destEC, "ec2-user", env.clientKey.PublicKey(), destEd)
	writeKnownHost(t, env.awlessKnownHosts(), jump.addr, jumpEd.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), dest.addr, destEd.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: false})

	c := env.client(t, jump)
	if err := c.DialWithUsers("ec2-user"); err != nil {
		t.Fatal(err)
	}
	var proxied *Client
	var err error
	within(t, 10*time.Second, func() {
		proxied, err = c.NewClientWithProxy(dest.host, dest.port, "", "ec2-user")
	})
	if err != nil {
		t.Fatalf("a destination known by its Ed25519 key was refused: %v", err)
	}
	t.Cleanup(func() { proxied.Client.Close() })
}

// A host that offers only a key type nobody recorded for it has not changed its
// key: it is still refused, but not with the man-in-the-middle alarm that tells the
// user to edit a correct known_hosts line.
func TestDialUnrecordedHostKeyTypeRefusedNotChanged(t *testing.T) {
	env := newSSHEnv(t)
	srv := startServer(t, newECDSASigner(t), "ec2-user", env.clientKey.PublicKey())
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, newSigner(t).PublicKey())
	before, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeTerminal{interactive: true, answers: []string{"yes"}}
	useTerminal(t, fake)

	c := env.client(t, srv)
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu") })
	if err == nil {
		t.Fatal("a host key of an unrecorded type was accepted")
	}
	if strings.Contains(err.Error(), "HAS CHANGED") {
		t.Errorf("reported as a changed key: %v", err)
	}
	for _, want := range []string{"ecdsa-sha2-nistp256", "ssh-ed25519", env.opensshKnownHosts()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if !IsHostKeyError(err) {
		t.Error("not reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 1 {
		t.Errorf("%d connections, want 1", n)
	}
	if n := fake.readCount(); n != 0 {
		t.Errorf("asked %d times, want 0", n)
	}
	after, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
	mustNotExist(t, env.awlessKnownHosts())
}

func TestKnownHostKeyAlgorithms(t *testing.T) {
	env := newSSHEnv(t)
	const known, rsaHost, unknown, dsaHost = "198.51.100.1:22", "198.51.100.2:22", "198.51.100.3:22", "198.51.100.4:22"
	writeKnownHost(t, env.awlessKnownHosts(), known, newSigner(t).PublicKey())
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPub, err := gossh.NewPublicKey(&rsaKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	writeKnownHost(t, env.awlessKnownHosts(), rsaHost, rsaPub)

	writeKnownHost(t, env.awlessKnownHosts(), dsaHost, dsaSigner(t).PublicKey())

	secure := gossh.SupportedAlgorithms().HostKeys
	noInsecure := func(name string, got []string) {
		t.Helper()
		for _, algo := range gossh.InsecureAlgorithms().HostKeys {
			if slices.Contains(got, algo) {
				t.Errorf("%s: %q offers the insecure %s", name, got, algo)
			}
		}
	}

	// Never empty: the library default would bring ssh-rsa and DSA back.
	got := knownHostKeyAlgorithms(unknown)
	if !slices.Equal(got, secure) {
		t.Errorf("unknown host: %q, want the secure algorithms %q", got, secure)
	}
	noInsecure("unknown host", got)

	got = knownHostKeyAlgorithms(known)
	if len(got) != len(secure) || got[0] != gossh.KeyAlgoED25519 {
		t.Errorf("Ed25519 host: %q, want ssh-ed25519 first, then the other %d", got, len(secure)-1)
	}
	noInsecure("Ed25519 host", got)

	// A recorded RSA key goes through RSA-SHA2 only.
	got = knownHostKeyAlgorithms(rsaHost)
	if want := []string{gossh.KeyAlgoRSASHA256, gossh.KeyAlgoRSASHA512}; len(got) != len(secure) || !slices.Equal(got[:2], want) {
		t.Errorf("RSA host: %q, want %q first, then the other %d", got, want, len(secure)-2)
	}
	noInsecure("RSA host", got)

	// A recorded DSA key does not make DSA acceptable again.
	got = knownHostKeyAlgorithms(dsaHost)
	if !slices.Equal(got, secure) {
		t.Errorf("DSA host: %q, want the secure algorithms %q", got, secure)
	}
	noInsecure("DSA host", got)
}

// rsaSigner returns a fresh RSA host key. Without restriction a server offers it as
// rsa-sha2-256, rsa-sha2-512 and ssh-rsa.
func rsaSigner(t *testing.T) gossh.AlgorithmSigner {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer.(gossh.AlgorithmSigner)
}

// sha1RSASigner is an RSA host key that the server offers only as ssh-rsa, like a
// host older than OpenSSH 7.2.
func sha1RSASigner(t *testing.T, key gossh.AlgorithmSigner) gossh.Signer {
	t.Helper()
	signer, err := gossh.NewSignerWithAlgorithms(key, []string{gossh.KeyAlgoRSA})
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

var dsaOnce struct {
	sync.Once
	key *dsa.PrivateKey
	err error
}

// dsaSigner returns a DSA host key. Generating the parameters is slow, so the key
// is shared by the tests.
func dsaSigner(t *testing.T) gossh.Signer {
	t.Helper()
	dsaOnce.Do(func() {
		key := new(dsa.PrivateKey)
		if dsaOnce.err = dsa.GenerateParameters(&key.Parameters, rand.Reader, dsa.L1024N160); dsaOnce.err != nil {
			return
		}
		if dsaOnce.err = dsa.GenerateKey(key, rand.Reader); dsaOnce.err == nil {
			dsaOnce.key = key
		}
	})
	if dsaOnce.err != nil {
		t.Fatal(dsaOnce.err)
	}
	signer, err := gossh.NewSignerFromKey(dsaOnce.key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// algorithmRecorder is a host key that remembers the algorithm it signed with,
// which is the host-key algorithm the handshake settled on.
type algorithmRecorder struct {
	gossh.AlgorithmSigner
	mu   sync.Mutex
	used []string
}

func (r *algorithmRecorder) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*gossh.Signature, error) {
	r.mu.Lock()
	r.used = append(r.used, algorithm)
	r.mu.Unlock()
	return r.AlgorithmSigner.SignWithAlgorithm(rand, data, algorithm)
}

func (r *algorithmRecorder) algorithms() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.used)
}

// A host offering only ssh-rsa (RSA with SHA-1) or only DSA is refused, whether its
// key is recorded or not and whether host keys are checked or not: the client never
// offers those algorithms. The refusal is about the host, so no other user is tried,
// nothing is asked and nothing is persisted.
func TestDialRefusesInsecureHostKeyAlgorithms(t *testing.T) {
	cases := []struct {
		name, offered string
		hostKey       func(t *testing.T) (offered gossh.Signer, recorded gossh.PublicKey)
	}{
		{"ssh-rsa", gossh.KeyAlgoRSA, func(t *testing.T) (gossh.Signer, gossh.PublicKey) {
			key := rsaSigner(t)
			return sha1RSASigner(t, key), key.PublicKey()
		}},
		{"dsa", gossh.InsecureKeyAlgoDSA, func(t *testing.T) (gossh.Signer, gossh.PublicKey) {
			key := dsaSigner(t)
			return key, key.PublicKey()
		}},
	}
	for _, tc := range cases {
		for _, mode := range []string{"unknown", "recorded", "not strict"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				env := newSSHEnv(t)
				offered, recorded := tc.hostKey(t)
				srv := startServer(t, offered, "ec2-user", env.clientKey.PublicKey())
				if mode == "recorded" {
					writeKnownHost(t, env.awlessKnownHosts(), srv.addr, recorded)
				}
				before, _ := os.ReadFile(env.awlessKnownHosts())
				fake := &fakeTerminal{interactive: true, answers: []string{"yes"}}
				useTerminal(t, fake)

				c := env.client(t, srv)
				c.SetStrictHostKeyChecking(mode != "not strict")
				var err error
				within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu") })
				if err == nil {
					t.Fatalf("a host offering only %s was accepted", tc.offered)
				}
				var negotiation *gossh.AlgorithmNegotiationError
				if !errors.As(err, &negotiation) || negotiation.What != "host key" {
					t.Errorf("got %v, want a host-key algorithm negotiation failure", err)
				}
				for _, want := range []string{"insecure", tc.offered} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not mention %q", err, want)
					}
				}
				if strings.Contains(err.Error(), "HAS CHANGED") || strings.Contains(err.Error(), "authenticate") {
					t.Errorf("misreported: %v", err)
				}
				if !IsHostKeyError(err) {
					t.Error("not reported as a host-key error")
				}
				if n := srv.conns.Load(); n != 1 {
					t.Errorf("%d connections, want 1: another user cannot fix the host's algorithms", n)
				}
				if n := fake.readCount(); n != 0 {
					t.Errorf("asked %d times, want 0", n)
				}
				after, _ := os.ReadFile(env.awlessKnownHosts())
				if !bytes.Equal(before, after) {
					t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
				}
				mustNotExist(t, env.opensshKnownHosts())
			})
		}
	}
}

// The same on the second hop.
func TestProxyHopRefusesSHA1RSAHostKey(t *testing.T) {
	env := newSSHEnv(t)
	jumpKey, destKey := newSigner(t), rsaSigner(t)
	jump := startServer(t, jumpKey, "ec2-user", env.clientKey.PublicKey())
	dest := startServer(t, sha1RSASigner(t, destKey), "ec2-user", env.clientKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), jump.addr, jumpKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), dest.addr, destKey.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: false})

	c := env.client(t, jump)
	if err := c.DialWithUsers("ec2-user"); err != nil {
		t.Fatal(err)
	}
	var err error
	within(t, 10*time.Second, func() {
		_, err = c.NewClientWithProxy(dest.host, dest.port, "", "ec2-user", "ubuntu")
	})
	var negotiation *gossh.AlgorithmNegotiationError
	if !errors.As(err, &negotiation) || !IsHostKeyError(err) {
		t.Fatalf("got %v, want a host-key algorithm negotiation failure", err)
	}
	if n := dest.conns.Load(); n != 1 {
		t.Errorf("destination saw %d connections, want 1", n)
	}
}

// A recorded RSA key is still usable: a host offering it with SHA-2 and SHA-1
// signatures, like a stock sshd, is reached through RSA-SHA2.
func TestDialRecordedRSAKeyUsesSHA2(t *testing.T) {
	env := newSSHEnv(t)
	hostKey := &algorithmRecorder{AlgorithmSigner: rsaSigner(t)}
	srv := startServer(t, hostKey, "ec2-user", env.clientKey.PublicKey(), newECDSASigner(t), newSigner(t))
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, hostKey.PublicKey())
	before, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	useTerminal(t, &fakeTerminal{interactive: true, onRead: func() {
		t.Error("asked about a host whose key is already known")
	}})

	c := env.client(t, srv)
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user") })
	if err != nil {
		t.Fatalf("a host known by its RSA key was refused: %v", err)
	}
	used := hostKey.algorithms()
	if len(used) == 0 {
		t.Fatal("the RSA host key was not used")
	}
	for _, algo := range used {
		if algo != gossh.KeyAlgoRSASHA256 && algo != gossh.KeyAlgoRSASHA512 {
			t.Errorf("negotiated %s, want rsa-sha2-256 or rsa-sha2-512", algo)
		}
	}
	after, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
	mustNotExist(t, env.awlessKnownHosts())
}

// RSA-SHA2 still checks the key: a different RSA key for the address is the
// changed-key alarm, with nothing asked and nothing persisted.
func TestDialChangedRSAHostKeyRefused(t *testing.T) {
	env := newSSHEnv(t)
	srv := startServer(t, rsaSigner(t), "ec2-user", env.clientKey.PublicKey())
	writeKnownHost(t, env.opensshKnownHosts(), srv.addr, rsaSigner(t).PublicKey())
	before, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeTerminal{interactive: true, answers: []string{"yes"}}
	useTerminal(t, fake)

	c := env.client(t, srv)
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu") })
	if err == nil || !strings.Contains(err.Error(), "HAS CHANGED") {
		t.Fatalf("got %v, want the changed-host-key refusal", err)
	}
	if !IsHostKeyError(err) {
		t.Error("not reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 1 {
		t.Errorf("%d connections, want 1", n)
	}
	if n := fake.readCount(); n != 0 {
		t.Errorf("asked %d times, want 0", n)
	}
	after, err := os.ReadFile(env.opensshKnownHosts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
	mustNotExist(t, env.awlessKnownHosts())
}

// The reported defect: with stdin not a terminal, the question was re-asked for
// every candidate user and never answered.
func TestDialWithoutTerminalFailsFast(t *testing.T) {
	env := newSSHEnv(t)
	srv := startServer(t, newSigner(t), "ec2-user", env.clientKey.PublicKey())
	fake := &fakeTerminal{interactive: false}
	useTerminal(t, fake)

	c := env.client(t, srv)
	var err error
	within(t, 10*time.Second, func() {
		err = c.DialWithUsers("ec2-user", "ubuntu", "centos", "core", "bitnami", "admin", "root")
	})
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("got %v, want ErrNoTerminal", err)
	}
	if !IsHostKeyError(err) {
		t.Error("an unconfirmed host key is not reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 1 {
		t.Errorf("%d connections, want 1", n)
	}
	if n := fake.readCount(); n != 0 {
		t.Errorf("%d reads from a non-terminal, want 0", n)
	}
	if n := strings.Count(fake.written(), "(yes/no)"); n > 1 {
		t.Errorf("question written %d times, want at most once", n)
	}
	mustNotExist(t, env.opensshKnownHosts())
	mustNotExist(t, env.awlessKnownHosts())
}

func TestDialDeclinedHostKeyStops(t *testing.T) {
	env := newSSHEnv(t)
	srv := startServer(t, newSigner(t), "ec2-user", env.clientKey.PublicKey())
	fake := &fakeTerminal{interactive: true, answers: []string{"no", "no", "no"}}
	useTerminal(t, fake)

	c := env.client(t, srv)
	var err error
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu", "root") })
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("got %v, want the host key refusal", err)
	}
	if !IsHostKeyError(err) {
		t.Error("a declined host key is not reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 1 {
		t.Errorf("%d connections, want 1", n)
	}
	if n := fake.readCount(); n != 1 {
		t.Errorf("asked %d times, want 1", n)
	}
	mustNotExist(t, env.opensshKnownHosts())
	mustNotExist(t, env.awlessKnownHosts())
}

// Stopping on a host-key verdict must not stop the search for the right user.
func TestDialTriesNextUserOnAuthFailure(t *testing.T) {
	env := newSSHEnv(t)
	hostKey := newSigner(t)
	srv := startServer(t, hostKey, "ubuntu", env.clientKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), srv.addr, hostKey.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: false})

	c := env.client(t, srv)
	var err error
	within(t, 10*time.Second, func() { err = c.DialWithUsers("ec2-user", "ubuntu") })
	if err != nil {
		t.Fatal(err)
	}
	if c.User != "ubuntu" {
		t.Errorf("user %q, want ubuntu", c.User)
	}
	if IsHostKeyError(errors.New("ssh: unable to authenticate")) {
		t.Error("an authentication failure is reported as a host-key error")
	}
	if n := srv.conns.Load(); n != 2 {
		t.Errorf("%d connections, want 2 (one per user tried)", n)
	}
}

func TestProxyHopHostKeyWithoutTerminal(t *testing.T) {
	env := newSSHEnv(t)
	jumpKey, destKey := newSigner(t), newSigner(t)
	jump := startServer(t, jumpKey, "ec2-user", env.clientKey.PublicKey())
	dest := startServer(t, destKey, "ec2-user", env.clientKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), jump.addr, jumpKey.PublicKey())
	fake := &fakeTerminal{interactive: false}
	useTerminal(t, fake)

	c := env.client(t, jump)
	if err := c.DialWithUsers("ec2-user"); err != nil {
		t.Fatal(err)
	}

	var err error
	within(t, 10*time.Second, func() {
		_, err = c.NewClientWithProxy(dest.host, dest.port, "", "ec2-user", "ubuntu", "root")
	})
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("got %v, want ErrNoTerminal", err)
	}
	if n := dest.conns.Load(); n != 1 {
		t.Errorf("destination saw %d connections, want 1", n)
	}
	if n := fake.readCount(); n != 0 {
		t.Errorf("%d reads from a non-terminal, want 0", n)
	}

	writeKnownHost(t, env.awlessKnownHosts(), dest.addr, destKey.PublicKey())
	var proxied *Client
	within(t, 10*time.Second, func() {
		proxied, err = c.NewClientWithProxy(dest.host, dest.port, "", "ec2-user", "ubuntu", "root")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxied.Client.Close() })
	if proxied.Proxy != c {
		t.Error("the proxied client does not point at its jump host")
	}
	if proxied.User != "ec2-user" {
		t.Errorf("user %q, want ec2-user", proxied.User)
	}
}

// The destination's own key goes first on the second hop: the jump host's key is
// not accepted by the destination here, only the destination's is.
func TestProxyHopUsesDestinationKey(t *testing.T) {
	env := newSSHEnv(t)
	jumpKey, destKey := newSigner(t), newSigner(t)
	destClientKey := writeClientKey(t, env.keyDir, "dest.pem")
	jump := startServer(t, jumpKey, "ec2-user", env.clientKey.PublicKey())
	dest := startServer(t, destKey, "ubuntu", destClientKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), jump.addr, jumpKey.PublicKey())
	writeKnownHost(t, env.awlessKnownHosts(), dest.addr, destKey.PublicKey())
	useTerminal(t, &fakeTerminal{interactive: false})

	c := env.client(t, jump)
	if err := c.DialWithUsers("ec2-user"); err != nil {
		t.Fatal(err)
	}
	destKeypath := filepath.Join(env.keyDir, "dest.pem")
	var proxied *Client
	var err error
	within(t, 10*time.Second, func() {
		proxied, err = c.NewClientWithProxy(dest.host, dest.port, destKeypath, "ec2-user", "ubuntu")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxied.Client.Close() })
	if proxied.User != "ubuntu" || proxied.Keypath != destKeypath {
		t.Errorf("got user %q key %q, want ubuntu and %q", proxied.User, proxied.Keypath, destKeypath)
	}
}
