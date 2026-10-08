package commands

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theazz/awless-ro/cloud"
	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/logger"
)

// captureLog sends the default logger to a buffer for the duration of a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := logger.DefaultLogger
	logger.DefaultLogger = logger.New("", 0, &buf)
	t.Cleanup(func() { logger.DefaultLogger = previous })
	return &buf
}

// cannedInfra stands in for the AWS infra service. With forbid set, any call fails
// the test: that is how --local proves it never reaches AWS.
type cannedInfra struct {
	t      *testing.T
	forbid bool
	graphs map[string]cloud.GraphAPI
	errs   map[string]error
	calls  atomic.Int32
}

func (c *cannedInfra) FetchByType(_ context.Context, typ string) (cloud.GraphAPI, error) {
	c.calls.Add(1)
	if c.forbid {
		c.t.Errorf("AWS was asked for %s", typ)
	}
	if err := c.errs[typ]; err != nil {
		return graph.NewGraph(), err
	}
	if g, ok := c.graphs[typ]; ok {
		return g, nil
	}
	return graph.NewGraph(), nil
}

func sshGraph(t *testing.T, resources ...*graph.Resource) cloud.GraphAPI {
	t.Helper()
	g := graph.NewGraph()
	if err := g.AddResource(resources...); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestSSHCommandIsEnabled(t *testing.T) {
	if strings.Contains(sshCmd.Short, "not in this release") {
		t.Errorf("Short still says the command is disabled: %q", sshCmd.Short)
	}
	if sshCmd.RunE == nil {
		t.Error("ssh has no RunE")
	}
}

func TestParseUserHost(t *testing.T) {
	cases := []struct{ in, user, host string }{
		{"ec2-user@web", "ec2-user", "web"},
		{"web", "", "web"},
		{"root@192.0.2.1", "root", "192.0.2.1"},
	}
	for _, tc := range cases {
		if u, h := parseUserHost(tc.in); u != tc.user || h != tc.host {
			t.Errorf("parseUserHost(%q) = (%q, %q), want (%q, %q)", tc.in, u, h, tc.user, tc.host)
		}
	}
}

func TestResolveInstance(t *testing.T) {
	captureLog(t)
	g := sshGraph(t,
		resource(cloud.Instance, "i-web", p.Name, "web", p.PublicIP, "198.51.100.10", p.PrivateIP, "10.0.0.10", p.State, "running"),
		resource(cloud.Instance, "i-dup1", p.Name, "dup", p.State, "running"),
		resource(cloud.Instance, "i-dup2", p.Name, "dup", p.State, "stopped"),
		resource(cloud.Instance, "i-twin1", p.Name, "twin", p.State, "running"),
		resource(cloud.Instance, "i-twin2", p.Name, "twin", p.State, "running"),
		resource(cloud.Instance, "i-ghost1", p.Name, "ghost", p.State, "stopped"),
		resource(cloud.Instance, "i-ghost2", p.Name, "ghost", p.State, "terminated"),
	)

	for _, ref := range []string{"web", "198.51.100.10", "10.0.0.10", "i-web"} {
		inst, err := resolveInstance(g, ref)
		if err != nil {
			t.Errorf("%s: %v", ref, err)
			continue
		}
		if inst.Id() != "i-web" {
			t.Errorf("%s resolved to %s, want i-web", ref, inst.Id())
		}
	}

	inst, err := resolveInstance(g, "dup")
	if err != nil || inst.Id() != "i-dup1" {
		t.Errorf("shared name with one running: got (%v, %v), want i-dup1", inst, err)
	}
	if _, err := resolveInstance(g, "twin"); err == nil {
		t.Error("shared name with two running resolved to one of them")
	}
	if _, err := resolveInstance(g, "ghost"); err == nil {
		t.Error("shared name with none running resolved to one of them")
	}

	_, err = resolveInstance(g, "nosuch")
	var nf instanceNotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("unknown instance: got %v, want instanceNotFoundError", err)
	}
	if err.Error() != "instance 'nosuch' not found" {
		t.Errorf("message changed: %q", err)
	}
}

func TestDecorateSSHNotFoundUnderLocal(t *testing.T) {
	err := instanceNotFoundError{"web"}
	if got := decorateSSHNotFound(sshOptions{local: true}, err); !strings.Contains(got.Error(), "awless-ro sync") || !errors.As(got, new(instanceNotFoundError)) {
		t.Errorf("--local: got %q, want a hint to sync", got)
	}
	if got := decorateSSHNotFound(sshOptions{}, err); got != error(err) {
		t.Errorf("without --local the error is changed: %q", got)
	}
	other := errors.New("boom")
	if got := decorateSSHNotFound(sshOptions{local: true}, other); got != other {
		t.Errorf("an unrelated error is changed: %q", got)
	}
}

func TestTargetAddress(t *testing.T) {
	target := func(pub, priv string) *sshTarget {
		return &sshTarget{
			instance:  resource(cloud.Instance, "i-1"),
			publicIP:  pub,
			privateIP: priv,
			state:     "running",
		}
	}
	cases := []struct {
		name      string
		t         *sshTarget
		private   bool
		want      string
		wantError bool
	}{
		{"public wins", target("198.51.100.1", "10.0.0.1"), false, "198.51.100.1", false},
		{"private when no public", target("", "10.0.0.1"), false, "10.0.0.1", false},
		{"--private picks private", target("198.51.100.1", "10.0.0.1"), true, "10.0.0.1", false},
		{"--private without private", target("198.51.100.1", ""), true, "", true},
		{"neither", target("", ""), false, "", true},
	}
	for _, tc := range cases {
		got, err := tc.t.address(tc.private)
		if (err != nil) != tc.wantError || got != tc.want {
			t.Errorf("%s: got (%q, %v), want %q (error %t)", tc.name, got, err, tc.want, tc.wantError)
		}
		if err != nil && (!strings.Contains(err.Error(), "i-1") || !strings.Contains(err.Error(), "running")) {
			t.Errorf("%s: error %q does not name the instance and its state", tc.name, err)
		}
	}
}

func TestUserForImage(t *testing.T) {
	cases := []struct{ name, description, want string }{
		{"ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20240101", "", "ubuntu"},
		{"debian-13-amd64-20250101-1234", "", "admin"},
		{"al2023-ami-2023.6.20250101.0-kernel-6.1-x86_64", "", "ec2-user"},
		{"amzn2-ami-kernel-5.10-hvm-2.0.20250101.0-x86_64-gp2", "", "ec2-user"},
		{"RHEL-9.4.0_HVM-20240101-x86_64-0-Hourly2-GP3", "", "ec2-user"},
		{"suse-sles-15-sp6-v20250101-hvm-ssd-x86_64", "", "ec2-user"},
		{"AlmaLinux OS 9.4.20240101 x86_64", "", "ec2-user"},
		{"bitnami-wordpress-6.5.0-0-r01-linux-debian-12-x86_64-hvm-ebs", "", "bitnami"},
		{"flatcar-stable-3815.2.0-hvm", "", "core"},
		{"fedora-coreos-40.20240101.3.0-x86_64", "", "core"},
		{"Fedora-Cloud-Base-40-1.14.x86_64-hvm", "", "fedora"},
		{"CentOS-7-2111-20220825_1.x86_64", "", "centos"},
		{"Rocky-9-EC2-Base-9.4-20240101.0.x86_64", "", "rocky"},
		{"my-golden-image", "", ""},
		{"", "Ubuntu 24.04", "ubuntu"},
		{"my-golden-image", "Amazon Linux 2023 base", "ec2-user"},
	}
	for _, tc := range cases {
		img := resource(cloud.Image, "ami-1", p.Name, tc.name, p.Description, tc.description)
		if got := userForImage(img); got != tc.want {
			t.Errorf("userForImage(%q, %q) = %q, want %q", tc.name, tc.description, got, tc.want)
		}
	}
}

func TestLoginUsers(t *testing.T) {
	g := sshGraph(t,
		resource(cloud.Image, "ami-ubuntu", p.Name, "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20240101"),
		resource(cloud.Image, "ami-custom", p.Name, "my-golden-image"),
	)

	users, known := loginUsers(g, resource(cloud.Instance, "i-1", p.Image, "ami-ubuntu"), "admin")
	if !reflect.DeepEqual(users, []string{"admin"}) || !known {
		t.Errorf("explicit user: got (%v, %t)", users, known)
	}

	users, known = loginUsers(g, resource(cloud.Instance, "i-1", p.Image, "ami-ubuntu"), "")
	want := []string{"ubuntu", "ec2-user", "centos", "core", "bitnami", "admin", "root"}
	if !reflect.DeepEqual(users, want) || !known {
		t.Errorf("image in graph: got (%v, %t), want (%v, true)", users, known, want)
	}

	for _, image := range []string{"ami-absent", "ami-custom", ""} {
		users, known = loginUsers(g, resource(cloud.Instance, "i-1", p.Image, image), "")
		if !reflect.DeepEqual(users, defaultAMIUsers) || known {
			t.Errorf("image %q: got (%v, %t), want the defaults, not known", image, users, known)
		}
	}
	// The defaults are copied, not shared: a caller may not reorder the package's.
	users[0] = "changed"
	if defaultAMIUsers[0] != "ec2-user" {
		t.Fatal("loginUsers returned the defaultAMIUsers slice itself")
	}
}

// #153: the key is looked up in the awless-ro keys directory and then in ~/.ssh.
func TestSSHKeyFolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := []string{config.KeysDir, filepath.Join(home, ".ssh")}
	if got := sshKeyFolders(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The reported panic: --local leaves the AWS services nil, and ssh fetched from AWS
// regardless.
func TestConnectionGraphLocalReadsOnlyTheGraph(t *testing.T) {
	local := sshGraph(t, resource(cloud.Instance, "i-web", p.Name, "web"))
	loader := func() (cloud.GraphAPI, error) { return local, nil }

	infra := &cannedInfra{t: t, forbid: true}
	g, err := connectionGraph(true, infra, loader)
	if err != nil {
		t.Fatal(err)
	}
	if g != local {
		t.Error("--local did not return the local graph")
	}
	if n := infra.calls.Load(); n != 0 {
		t.Errorf("%d AWS calls under --local, want 0", n)
	}

	// The exact state of the reported crash: no infra service at all.
	g, err = connectionGraph(true, nil, loader)
	if err != nil || g != local {
		t.Errorf("--local without an infra service: got (%v, %v)", g, err)
	}
}

func TestLocalInfraGraphReadsTheSyncedFile(t *testing.T) {
	captureLog(t)
	awlessHome := t.TempDir()
	t.Setenv("__AWLESS_HOME", awlessHome)
	previous := config.Config
	config.Config = map[string]interface{}{
		config.ProfileConfigKey: "testprofile",
		config.RegionConfigKey:  "eu-west-3",
	}
	t.Cleanup(func() { config.Config = previous })

	dir := filepath.Join(awlessHome, "aws", "rdf", "testprofile", "eu-west-3")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	synced := sshGraph(t, resource(cloud.Instance, "i-web", p.Name, "web", p.PrivateIP, "10.0.0.10"))
	f, err := os.Create(filepath.Join(dir, "infra.nt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := synced.MarshalTo(f); err != nil {
		t.Fatal(err)
	}
	f.Close()

	g, err := connectionGraph(true, nil, localInfraGraph)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := resolveInstance(g, "web")
	if err != nil {
		t.Fatal(err)
	}
	if inst.Id() != "i-web" {
		t.Errorf("got %s, want i-web", inst.Id())
	}
}

func TestConnectionGraphLive(t *testing.T) {
	captureLog(t)
	infra := &cannedInfra{t: t, graphs: map[string]cloud.GraphAPI{
		cloud.Instance:      sshGraph(t, resource(cloud.Instance, "i-web", p.Name, "web")),
		cloud.SecurityGroup: sshGraph(t, resource(cloud.SecurityGroup, "sg-1")),
		cloud.Image:         sshGraph(t, resource(cloud.Image, "ami-1")),
	}}
	g, err := connectionGraph(false, infra, func() (cloud.GraphAPI, error) {
		t.Error("the local graph was read without --local")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for typ, id := range map[string]string{cloud.Instance: "i-web", cloud.SecurityGroup: "sg-1", cloud.Image: "ami-1"} {
		if _, err := findResource(g, id, typ); err != nil {
			t.Errorf("merged graph: %v", err)
		}
	}
	if n := infra.calls.Load(); n != 3 {
		t.Errorf("%d fetches, want 3", n)
	}

	boom := errors.New("boom")
	if _, err := connectionGraph(false, &cannedInfra{t: t, errs: map[string]error{cloud.Instance: boom}}, nil); !errors.Is(err, boom) {
		t.Errorf("instance fetch error: got %v, want it returned", err)
	}
	// Images only order the login users: failing to read them is not fatal.
	if _, err := connectionGraph(false, &cannedInfra{t: t, errs: map[string]error{cloud.Image: boom}}, nil); err != nil {
		t.Errorf("image fetch error: got %v, want nil", err)
	}
	if _, err := connectionGraph(false, nil, nil); err == nil {
		t.Error("no infra service without --local: got nil error")
	}
}

// countingListener accepts and counts every TCP connection made to it.
type countingListener struct {
	ln      net.Listener
	port    int
	accepts atomic.Int32
	wg      sync.WaitGroup
}

func listenCounting(t *testing.T) *countingListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &countingListener{ln: ln, port: ln.Addr().(*net.TCPAddr).Port}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			c.accepts.Add(1)
			conn.Close()
		}
	}()
	t.Cleanup(c.close)
	return c
}

func (c *countingListener) close() {
	c.ln.Close()
	c.wg.Wait()
}

// --print-cli and --print-config describe a connection; they must not make one.
// Upstream dialled to learn the user, so printing needed a reachable host and
// went through the host-key prompt.
func TestPrintNeverDials(t *testing.T) {
	captureLog(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	previousKeysDir := config.KeysDir
	config.KeysDir = filepath.Join(home, ".awless-ro", "keys")
	t.Cleanup(func() { config.KeysDir = previousKeysDir })
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "testkey.pem")
	if err := os.WriteFile(key, []byte("never parsed"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := sshGraph(t,
		resource(cloud.Instance, "i-web", p.Name, "web", p.PublicIP, "127.0.0.1", p.PrivateIP, "10.0.0.10", p.State, "running", p.KeyPair, "testkey", p.Image, "ami-ubuntu"),
		resource(cloud.Image, "ami-ubuntu", p.Name, "ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-20240101"),
		resource(cloud.Instance, "i-bastion", p.Name, "bastion", p.PublicIP, "127.0.0.1", p.State, "running", p.KeyPair, "testkey"),
		resource(cloud.Instance, "i-db", p.Name, "db", p.PrivateIP, "10.0.0.20", p.State, "running", p.KeyPair, "testkey"),
	)

	ln := listenCounting(t)
	port := strconv.Itoa(ln.port)
	proxy := "ssh -i " + key + " ec2-user@127.0.0.1 -p " + port + " -W %h:%p"

	cases := []struct {
		name string
		arg  string
		opts sshOptions
		want []string
	}{
		{"cli", "web", sshOptions{printCLI: true, port: ln.port, throughPort: 22},
			[]string{"-i " + key, "-p " + port, "ubuntu@127.0.0.1"}},
		{"config", "web", sshOptions{printConfig: true, port: ln.port, throughPort: 22},
			[]string{"Host web", "Hostname 127.0.0.1", "User ubuntu", "Port " + port, "IdentityFile " + key}},
		{"cli through", "db", sshOptions{printCLI: true, through: "bastion", port: 22, throughPort: ln.port},
			[]string{"-i " + key, "ec2-user@10.0.0.20", "ProxyCommand=" + proxy}},
		{"config through", "db", sshOptions{printConfig: true, through: "bastion", port: 22, throughPort: ln.port},
			[]string{"Host db", "Hostname 10.0.0.20", "User ec2-user", "IdentityFile " + key, "ProxyCommand " + proxy}},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		if err := printSSH(&out, tc.opts, g, tc.arg); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: output %q does not contain %q", tc.name, out.String(), want)
			}
		}
	}

	// The Host line is the bare instance name, with or without --through and an
	// explicit USER@: ssh matches Host patterns against the host name alone.
	hostLines := []struct {
		arg  string
		opts sshOptions
		want string
	}{
		{"web", sshOptions{printConfig: true, port: ln.port, throughPort: 22}, "Host web"},
		{"admin@web", sshOptions{printConfig: true, port: ln.port, throughPort: 22}, "Host web"},
		{"db", sshOptions{printConfig: true, through: "bastion", port: 22, throughPort: ln.port}, "Host db"},
		{"admin@db", sshOptions{printConfig: true, through: "bastion", port: 22, throughPort: ln.port}, "Host db"},
	}
	for _, tc := range hostLines {
		var out bytes.Buffer
		if err := printSSH(&out, tc.opts, g, tc.arg); err != nil {
			t.Errorf("%s (through %q): %v", tc.arg, tc.opts.through, err)
			continue
		}
		var got []string
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(line, "Host ") {
				got = append(got, line)
			}
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s (through %q): Host lines %q, want [%q]", tc.arg, tc.opts.through, got, tc.want)
		}
		if strings.HasPrefix(tc.arg, "admin@") && !strings.Contains(out.String(), "User admin") {
			t.Errorf("%s (through %q): the explicit user is lost: %q", tc.arg, tc.opts.through, out.String())
		}
	}

	// Anything that dialled would have reached the listener by now.
	time.Sleep(200 * time.Millisecond)
	ln.close()
	if n := ln.accepts.Load(); n != 0 {
		t.Errorf("printing opened %d connections, want 0", n)
	}

	// An unreachable host still prints: there is nothing listening on this port.
	var out bytes.Buffer
	if err := printSSH(&out, sshOptions{printCLI: true, port: ln.port, throughPort: 22}, g, "web"); err != nil {
		t.Fatalf("closed port: %v", err)
	}
	if !strings.Contains(out.String(), "ubuntu@127.0.0.1") {
		t.Errorf("closed port: got %q", out.String())
	}
}

func TestPlannedClientUnknownUserGuessesFirstDefault(t *testing.T) {
	logged := captureLog(t)
	t.Setenv("HOME", t.TempDir())
	g := sshGraph(t, resource(cloud.Instance, "i-web", p.Name, "web", p.PublicIP, "198.51.100.10", p.State, "running", p.Image, "ami-absent"))

	first, err := resolveTarget(g, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := plannedClient(sshOptions{port: 22}, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.User != "ec2-user" {
		t.Errorf("user %q, want ec2-user", client.User)
	}
	if !strings.Contains(logged.String(), "guessing 'ec2-user'") || !strings.Contains(logged.String(), "USER@web") {
		t.Errorf("no guess warning: %q", logged.String())
	}
}
