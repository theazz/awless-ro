package ssh

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestInitClient(t *testing.T) {
	//Create env
	f, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(f)
	t.Setenv("HOME", f)
	// No agent: every case below has to be answered by the key lookup alone.
	t.Setenv("SSH_AUTH_SOCK", "")
	sshPath := filepath.Join(f, ".ssh")
	err = os.MkdirAll(sshPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	keypath1 := filepath.Join(sshPath, "mykey.pem")
	rawkey := `-----BEGIN RSA PRIVATE KEY-----
MIICWwIBAAKBgQDRuxRbc9wQE2Q2grxE7McYoMMIVZRuTYMkqYKYjjhiPo9enlLT
+2QtTma7Yz8WPGSr384F13FYPrz/oYf8P0ZAyKH6WRqVZwIVY5gFLbOKR1tvnPm6
KtrDXBwvXB8g8QWKIF4Ck8yemuvIe9z/SHiE2aaADJms4Of7Bd3B1XNjQQIDAQAB
AoGAXL90szS7XsiUip6qD3j+Wt/NIARojZbtperoe/p46MltsZQmYORNWtPPDpNH
NNgkVPW2MFMkJrgn8IxIjL6WnAYFrz9shpvxSY+ihiICAlXXxuuZ+bEsaBsOmcuM
L0kOoDs8iL7FauSZ2L8M+Vg/Q6A2DvV53+Qm+8lnmeIwkdkCQQD69j6qoFR67896
+EDk6n++IJyrQJyAYyTbbLAx+8b7WXDXYCn9NUprcc69eQ9cidwyiUj4+gXdRPsl
N/ngTjOPAkEA1fDwFlI0ROFFh5DJs//2T0QlHWvVMHJFzkTbPTxA2FjlxZoM2c8x
MEzcrMwg6qwup7MsMswJCxoNWnYEpINULwJASlmZx0MoxCM3/N5/m1I99j4DLFlA
BGlbCgbxTF2jXePpomVDC1k2aw6UiV3MR0YwjmhNzjWEd0Fwhl5HEUUZ0QJAVraQ
aUuqWdzAtMDPsEBn0hr5vCIPx9IZTxCDmB9K3SWzA9N7r/CVrFELBJK8KMHfKyOp
H3GpnLFThj3dhdyhCwJAcYdeLp39POwN0d4Dwf7Bu0sMRZIZrQpSbtO7ypOBwi3j
hTSx5geAH2W73IyiTK8zIdgPMJPh69//5OhFzhQ8Ug==
-----END RSA PRIVATE KEY-----`

	os.WriteFile(keypath1, []byte(rawkey), 0644)

	awlessKeysPath := filepath.Join(f, ".awless-keys")
	err = os.MkdirAll(awlessKeysPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	keypath2 := filepath.Join(awlessKeysPath, "mysecondkey.pem")
	os.WriteFile(keypath2, []byte(rawkey), 0644)
	// A key saved without an extension, as `ssh-keygen` names them.
	plainkey := filepath.Join(awlessKeysPath, "plainkey")
	os.WriteFile(plainkey, []byte(rawkey), 0600)
	missingFolder := filepath.Join(f, "does-not-exist")

	tcases := []struct {
		keyname    string
		keyfolders []string
		expkeypath string
	}{
		{"plainkey", []string{sshPath, awlessKeysPath}, plainkey},
		// A first folder that does not exist does not stop the lookup (#153: the
		// awless keys dir is often absent, ~/.ssh is where the key really is).
		{"mysecondkey", []string{missingFolder, awlessKeysPath}, keypath2},
		{"mykey.pem", []string{missingFolder, sshPath}, keypath1},
		{"mykey", []string{sshPath, awlessKeysPath}, keypath1},
		{"mykey.pem", []string{sshPath, awlessKeysPath}, keypath1},
		{filepath.Join(sshPath, "mykey.pem"), []string{sshPath, awlessKeysPath}, keypath1},
		{filepath.Join(sshPath, "mykey"), []string{sshPath, awlessKeysPath}, keypath1},
		{"mysecondkey", []string{sshPath, awlessKeysPath}, keypath2},
		{"mysecondkey.pem", []string{sshPath, awlessKeysPath}, keypath2},
		{keypath2, []string{sshPath, awlessKeysPath}, keypath2},
	}

	for _, tcase := range tcases {
		client, err := InitClient(tcase.keyname, tcase.keyfolders...)
		if err != nil {
			t.Fatal(err)
		}

		if got, want := client.Keypath, tcase.expkeypath; got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
		if got, want := client.StrictHostKeyChecking, true; got != want {
			t.Fatalf("got %t, want %t", got, want)
		}
	}

}

func TestCheckHostKey(t *testing.T) {
	//Create env
	f, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(f)
	t.Setenv("HOME", f)
	t.Setenv("__AWLESS_HOME", filepath.Join(f, ".awless-ro"))
	err = os.MkdirAll(filepath.Join(f, ".ssh"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	knownHostsFileContent := `1.2.3.4 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBFJz/HFJUq6SaXD5FdLe6ddIpmNPFim7E3NkNCSNurDun/h3BOIzNGfuseyMn32n24oQayhjkX8eGqevJIA38E=
3.4.5.6 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBKl6fXNb/yA0w7brzqNuOCwLJ/aPEMerl7/lsF0Y/1oafD2bxzj+QsEZo4XK/kvwCjqQArFO5nET+Tz015C6Kk=
4.5.6.7 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBKl6fXNb/yA0w7brzqNuOCwLJ/aPEMerl7/lsF0Y/1oafD2bxzj+QsEZo4XK/kvwCjqQArFO5nET+Tz015C6Kk=
`
	knowHostsFile := filepath.Join(f, ".ssh", "known_hosts")
	err = os.WriteFile(knowHostsFile, []byte(knownHostsFileContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	ecdsa1, _, _, _, err := gossh.ParseAuthorizedKey([]byte("1.2.3.4 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBFJz/HFJUq6SaXD5FdLe6ddIpmNPFim7E3NkNCSNurDun/h3BOIzNGfuseyMn32n24oQayhjkX8eGqevJIA38E="))
	if err != nil {
		t.Fatalf("error parsing ecdsa1: %s", err)
	}
	ecdsa2, _, _, _, err := gossh.ParseAuthorizedKey([]byte("3.4.5.6 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBKl6fXNb/yA0w7brzqNuOCwLJ/aPEMerl7/lsF0Y/1oafD2bxzj+QsEZo4XK/kvwCjqQArFO5nET+Tz015C6Kk="))
	if err != nil {
		t.Fatalf("error parsing ecdsa1: %s", err)
	}
	knownKeys := make(map[string]gossh.PublicKey)
	numberKeysAdded := 0
	previousTrust := trustKeyFunc
	t.Cleanup(func() { trustKeyFunc = previousTrust })
	trustKeyFunc = func(hostname string, remote net.Addr, key gossh.PublicKey, _ string) (bool, error) {
		knownKeys[hostname] = key
		numberKeysAdded++
		return true, nil
	}
	tcases := []struct {
		ip     string
		key    gossh.PublicKey
		expErr string
	}{
		{"1.2.3.4", ecdsa1, ""},
		{"2.3.4.5", ecdsa1, ""},
		{"2.3.4.5", ecdsa1, ""},
		{"3.4.5.6", ecdsa1, fmt.Sprintf(`
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
AWLESS DETECTED THAT THE REMOTE HOST PUBLIC KEY HAS CHANGED
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@

Host key for '3.4.5.6:22' has changed and you did not disable strict host key checking.
Someone may be trying to intercept your connection (man-in-the-middle attack). Otherwise, the host key may have been changed.

The fingerprint for the ecdsa-sha2-nistp256 key sent by the remote host is SHA256:t88p7xhU/1D3USAczv1d88hTZJbOeWN/ktcNmeWh6qI.
You persisted:
-> SHA256:rEoUFSq4TQYk6NO00isOyR7s/4keg0NePYE39XIf5vY (ecdsa-sha2-nistp256 key in %s:2)

To get rid of this message, update '%[1]s:2'`, knowHostsFile)},
		{"3.4.5.6", ecdsa2, ""},
	}

	for i, tcase := range tcases {
		addr, er := net.ResolveTCPAddr("", tcase.ip+":22")
		if er != nil {
			t.Fatal(er)
		}
		got := checkHostKey(tcase.ip+":22", addr, tcase.key)
		var gotStr string
		if got != nil {
			gotStr = got.Error()
		}
		if got, want := gotStr, tcase.expErr; got != want {
			t.Fatalf("case %d: got '%s', want '%s'", i+1, got, want)
		}
	}
	expectedKnownKeys := map[string]gossh.PublicKey{
		"2.3.4.5:22": ecdsa1,
	}
	if got, want := numberKeysAdded, 1; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
	if got, want := knownKeys, expectedKnownKeys; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	knownHostContent, err := os.ReadFile(knowHostsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(knownHostContent), knownHostsFileContent+"2.3.4.5 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBBFJz/HFJUq6SaXD5FdLe6ddIpmNPFim7E3NkNCSNurDun/h3BOIzNGfuseyMn32n24oQayhjkX8eGqevJIA38E=\n"; got != want {
		t.Fatalf("got \n%s\nwant\n%s\n", got, want)
	}
}

func TestCLIAndConfig(t *testing.T) {
	tcases := []struct {
		client      *Client
		cli, config string
	}{
		{
			&Client{Port: 22, IP: "1.2.3.4", User: "ec2-user", StrictHostKeyChecking: true},
			"/usr/bin/ssh ec2-user@1.2.3.4",
			"\nHost TestHost\n  Hostname 1.2.3.4\n  User ec2-user",
		},
		{
			&Client{Port: 8022, IP: "1.2.3.4", User: "ec2-user", StrictHostKeyChecking: true},
			"/usr/bin/ssh -p 8022 ec2-user@1.2.3.4",
			"\nHost TestHost\n  Hostname 1.2.3.4\n  User ec2-user\n  Port 8022",
		},
		{
			&Client{Port: 22, IP: "1.2.3.4", User: "ec2-user", StrictHostKeyChecking: true, Keypath: "/path/to/key"},
			"/usr/bin/ssh -i /path/to/key ec2-user@1.2.3.4",
			"\nHost TestHost\n  Hostname 1.2.3.4\n  User ec2-user\n  IdentityFile /path/to/key",
		},
		{
			&Client{Port: 22, IP: "1.2.3.4", User: "ec2-user", StrictHostKeyChecking: false},
			"/usr/bin/ssh -o StrictHostKeychecking=no ec2-user@1.2.3.4",
			"\nHost TestHost\n  Hostname 1.2.3.4\n  User ec2-user\n  StrictHostKeychecking no",
		},
		// The jump-host cases. There were none before, which is why the
		// ProxyCommand value carried literal single quotes for years: nothing
		// looked at it. ConnectString is shell text and quotes the option, argv
		// does not — see TestLocalExecArgvCarriesNoShellSyntax.
		{
			&Client{
				Port: 22, IP: "10.0.0.1", User: "ec2-user", StrictHostKeyChecking: true,
				Proxy: &Client{Port: 22, IP: "52.0.0.1", User: "admin"},
			},
			"/usr/bin/ssh ec2-user@10.0.0.1 -o 'ProxyCommand=ssh admin@52.0.0.1 -p 22 -W %h:%p'",
			"\nHost TestHost\n  Hostname 10.0.0.1\n  User ec2-user\n  ProxyCommand ssh admin@52.0.0.1 -p 22 -W %h:%p",
		},
		{
			&Client{
				Port: 22, IP: "10.0.0.1", User: "ec2-user", StrictHostKeyChecking: true, Keypath: "/path/to/key",
				Proxy: &Client{Port: 2222, IP: "52.0.0.1", User: "admin", Keypath: "/path/to/jumpkey"},
			},
			"/usr/bin/ssh -i /path/to/key ec2-user@10.0.0.1 -o 'ProxyCommand=ssh -i /path/to/jumpkey admin@52.0.0.1 -p 2222 -W %h:%p'",
			"\nHost TestHost\n  Hostname 10.0.0.1\n  User ec2-user\n  IdentityFile /path/to/key\n  ProxyCommand ssh -i /path/to/jumpkey admin@52.0.0.1 -p 2222 -W %h:%p",
		},
	}

	var got string
	for i, tcase := range tcases {

		got = tcase.client.ConnectString()
		if got != tcase.cli {
			t.Fatalf("case %d: got '%s', want '%s'", i+1, got, tcase.cli)
		}

		got = tcase.client.SSHConfigString("TestHost")
		if got != tcase.config {
			t.Fatalf("case %d: got '%s', want '%s'", i+1, got, tcase.config)
		}
	}
}

// localExec feeds execve directly, with no shell in between, so not one element
// may contain shell syntax. Upstream wrapped the ProxyCommand value in literal
// single quotes, ssh forwarded them to /bin/sh, and the shell then looked for a
// command whose entire name was "ssh user@host -W %h:%p".
func TestLocalExecArgvCarriesNoShellSyntax(t *testing.T) {
	c := &Client{
		Port: 22, IP: "10.0.0.1", User: "ec2-user", StrictHostKeyChecking: true,
		Proxy: &Client{Port: 22, IP: "52.0.0.1", User: "admin", Keypath: "/path/to/jumpkey"},
	}

	args, _ := c.localExec()

	var proxyArg string
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			proxyArg = args[i+1]
		}
	}
	if want := "ProxyCommand=ssh -i /path/to/jumpkey admin@52.0.0.1 -p 22 -W %h:%p"; proxyArg != want {
		t.Fatalf("got %q, want %q", proxyArg, want)
	}
	for _, a := range args {
		if strings.ContainsAny(a, `'"`) {
			t.Fatalf("argv element %q contains shell quoting; execve passes it through literally", a)
		}
	}
}

// The ProxyCommand value is executed by ssh through /bin/sh, and the values spliced
// into it are not trustworthy: the address comes from the synced graph, the key path
// can come from an instance's KeyPair tag.
func TestProxyCommandQuotesUntrustedValues(t *testing.T) {
	proxy := &Client{
		Port: 22,
		IP:   "52.0.0.1",
		User: "admin; touch /tmp/pwned",
		// A path a shell would otherwise split, plus an embedded single quote.
		Keypath: "/path/with space/it's",
	}

	got := (&Client{Port: 22, IP: "10.0.0.1", User: "ec2-user", Proxy: proxy}).Proxy.proxyCommand()
	want := `ssh -i '/path/with space/it'\''s' 'admin; touch /tmp/pwned@52.0.0.1' -p 22 -W %h:%p`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	tcases := []struct{ in, want string }{
		{"", "''"},
		{"plain", "plain"},
		{"/path/to/key", "/path/to/key"},
		{"ec2-user@1.2.3.4", "ec2-user@1.2.3.4"},
		{"ProxyCommand=ssh -W %h:%p", "'ProxyCommand=ssh -W %h:%p'"},
		{"a b", "'a b'"},
		{"; rm -rf /", "'; rm -rf /'"},
		{"$(whoami)", "'$(whoami)'"},
		{"back`tick`", "'back`tick`'"},
		{"it's", `'it'\''s'`},
	}
	for _, tc := range tcases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
