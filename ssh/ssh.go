package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/theazz/awless-ro/logger"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Client struct {
	*gossh.Client
	Config                  *gossh.ClientConfig
	IP, User, Keypath       string
	Port                    int
	Proxy                   *Client
	HostKeyCallback         gossh.HostKeyCallback
	StrictHostKeyChecking   bool
	InteractiveTerminalFunc func(*gossh.Client) error
	logger                  *logger.Logger
}

func InitClient(keyname string, keyFolders ...string) (*Client, error) {
	var auths []gossh.AuthMethod

	privkey, ok := findPrivateKeyFromName(keyname, keyFolders...)
	if ok {
		if a, err := privateKeyAuth(privkey); err == nil {
			auths = append(auths, a)
		}
	}

	if a, err := agentAuth(); err == nil {
		auths = append(auths, a)
	}

	if len(auths) == 0 {
		return nil, fmt.Errorf("No key provided and no SSH_AUTH_SOCK env variable set, unable to resolve auth")
	}

	return &Client{
		Config: &gossh.ClientConfig{
			Auth:            auths,
			Timeout:         2 * time.Second,
			HostKeyCallback: checkHostKey,
		},
		Keypath:                 privkey.path,
		logger:                  logger.DiscardLogger,
		InteractiveTerminalFunc: func(*gossh.Client) error { return nil },
		StrictHostKeyChecking:   true,
	}, nil
}

func (c *Client) SetLogger(l *logger.Logger) {
	c.logger = l
}

func (c *Client) SetStrictHostKeyChecking(hostKeyChecking bool) {
	c.StrictHostKeyChecking = hostKeyChecking
}

func (c *Client) DialWithUsers(usernames ...string) error {
	var err error
	var client *gossh.Client

	hostport := fmt.Sprintf("%s:%d", c.IP, c.Port)

	for _, user := range usernames {
		newConfig := *c.Config
		newConfig.User = user
		if !c.StrictHostKeyChecking {
			newConfig.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}
		client, err = gossh.Dial("tcp", hostport, &newConfig)
		if err != nil {
			continue
		} else {
			c.logger.ExtraVerbosef("dialed %s successfully with user %s", hostport, user)
			c.User = user
			c.Client = client
			return nil
		}
	}

	return fmt.Errorf("unable to authenticate to %s for users %q. Last error: %s", hostport, usernames, err)
}

func (c *Client) NewClientWithProxy(destinationHost string, destinationPort int, usernames ...string) (*Client, error) {
	hostport := fmt.Sprintf("%s:%d", destinationHost, destinationPort)
	for _, user := range usernames {
		netConn, err := c.Dial("tcp", hostport)
		if err != nil {
			return nil, fmt.Errorf("cannot dial from %s:%d to %s:%d - %s", c.IP, c.Port, destinationHost, destinationPort, err)
		}
		c.logger.ExtraVerbosef("successful tcp connection from %s:%d to %s:%d", c.IP, c.Port, destinationHost, destinationPort)
		newConfig := *c.Config
		newConfig.User = user
		if !c.StrictHostKeyChecking {
			newConfig.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}
		conn, chans, reqs, err := gossh.NewClientConn(netConn, hostport, &newConfig)
		if err != nil {
			netConn.Close()
			c.logger.ExtraVerbosef("cannot proxy with user %s (err: %s)", user, err)
			continue
		}
		c.logger.ExtraVerbosef("proxied successfully with user %s", user)

		return &Client{
			Client:                  gossh.NewClient(conn, chans, reqs),
			Proxy:                   c,
			IP:                      destinationHost,
			User:                    user,
			Keypath:                 c.Keypath,
			Port:                    destinationPort,
			InteractiveTerminalFunc: func(*gossh.Client) error { return nil },
			StrictHostKeyChecking:   c.StrictHostKeyChecking,
			logger:                  logger.DiscardLogger,
		}, nil
	}

	return nil, fmt.Errorf("cannot proxy from %s:%d to %s:%d with users %q", c.IP, c.Port, destinationHost, destinationPort, usernames)
}

func (c *Client) CloseAll() error {
	if c != nil {
		if c.Client != nil {
			return c.Client.Close()
		}
		if c.Proxy != nil {
			return c.Proxy.Close()
		}
	}
	return nil
}

func (c *Client) Connect() (err error) {
	args, installed := c.localExec()
	if installed {
		c.logger.Infof("Login as '%s' on '%s'; client '%s'", c.User, c.IP, args[0])
		c.logger.ExtraVerbosef("running locally %s", args)
		if err := c.CloseAll(); err != nil {
			c.logger.Warning("could not close properly SSH awless-ro client before delegating")
		}
		return syscall.Exec(args[0], args, os.Environ())
	}

	c.logger.Infof("No SSH. Fallback on builtin client. Login as '%s' on '%s'", c.User, c.IP)
	return c.InteractiveTerminalFunc(c.Client)
}

func (c *Client) SSHConfigString(hostname string) string {
	var buf bytes.Buffer

	extraOpts := map[string]string{}
	if len(c.Keypath) > 0 {
		extraOpts["IdentityFile"] = c.Keypath
	}
	if !c.StrictHostKeyChecking {
		extraOpts["StrictHostKeychecking"] = "no"
	}
	if c.Port != 22 {
		extraOpts["Port"] = strconv.Itoa(c.Port)
	}
	if c.Proxy != nil {
		extraOpts["ProxyCommand"] = c.Proxy.proxyCommand()
	}

	params := struct {
		IP, User, Name string
		Extra          map[string]string
	}{c.IP, c.User, hostname, extraOpts}

	template.Must(template.New("ssh_config").Parse(`
Host {{ .Name }}
  Hostname {{ .IP }}
  User {{ .User }}
{{- range $key, $value := .Extra }}
  {{ $key }} {{ $value -}}
{{ end -}}
`)).Execute(&buf, params)

	return buf.String()
}

// ConnectString renders the same command as localExec, but as text meant to be
// pasted into a shell. That is why it quotes: localExec produces argv, which goes
// to execve with no shell in between and must therefore carry no shell syntax,
// whereas this output is read by a shell and has to survive it.
//
// Conflating those two was the bug behind the temporary-script hack that used to
// live at the bottom of this file. See localExec.
func (c *Client) ConnectString() string {
	args, _ := c.localExec()
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	return strings.Join(quoted, " ")
}

// localExec builds the argv for handing the session over to the system ssh.
//
// Every element is a bare value: no shell quoting, because Connect passes this
// straight to execve and there is no shell to interpret it. Upstream wrapped the
// ProxyCommand value in literal single quotes here, which ssh then forwarded to
// /bin/sh as a single quoted word — so the shell looked for a program whose whole
// name was "ssh user@host -W %h:%p" and reported "not found". Rather than remove
// the quotes, upstream wrote the joined string into an executable file in the
// shared temp directory and exec'd that instead, which turned a quoting mistake
// into arbitrary file overwrite through a predictable path plus shell injection
// from any metacharacter in the user, host or key path.
func (c *Client) localExec() ([]string, bool) {
	exists := true
	bin, err := exec.LookPath("ssh")
	if err != nil {
		exists = false
		bin = "ssh"
	}
	args := []string{bin}
	if len(c.Keypath) > 0 {
		args = append(args, "-i", c.Keypath)
	}
	if c.Port != 22 {
		args = append(args, "-p", strconv.Itoa(c.Port))
	}
	if !c.StrictHostKeyChecking {
		args = append(args, "-o", "StrictHostKeychecking=no")
	}

	args = append(args, fmt.Sprintf("%s@%s", c.User, c.IP))

	if c.Proxy != nil {
		args = append(args, "-o", "ProxyCommand="+c.Proxy.proxyCommand())
	}

	return args, exists
}

// proxyCommand renders the jump-host hop for ssh's ProxyCommand option, from the
// perspective of the jump host itself (c is the proxy).
//
// ssh hands this value to /bin/sh, so every interpolated value is shell-quoted
// here. They are not trustworthy: the address comes from the synced graph, that is
// to say from the AWS API, and the key path can come from an instance's KeyPair
// tag. %h and %p are left bare on purpose — ssh substitutes them before the shell
// ever sees the string.
func (c *Client) proxyCommand() string {
	var b strings.Builder
	b.WriteString("ssh ")
	if len(c.Keypath) > 0 {
		b.WriteString("-i ")
		b.WriteString(shellQuote(c.Keypath))
		b.WriteString(" ")
	}
	b.WriteString(shellQuote(fmt.Sprintf("%s@%s", c.User, c.IP)))
	fmt.Fprintf(&b, " -p %d -W %%h:%%p", c.Port)
	return b.String()
}

// shellSafe matches the characters a POSIX shell leaves alone. Anything else means
// the value has to be quoted.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote makes s survive a single pass through a POSIX shell as one word.
// Single quotes protect everything except a single quote itself, which is spliced
// in by closing the quoted run, emitting an escaped quote and reopening.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type privateKey struct {
	path string
	body []byte
}

// findPrivateKeyFromName locates a private key by name, trying the name as given
// and then inside each of keyFolders, with and without a .pem suffix.
//
// keyname is not necessarily something the user typed: when `ssh` is called without
// -i it comes from the instance's KeyPair attribute, that is to say from the AWS
// API. So a relative name is confined to keyFolders — otherwise a key named
// "../../../etc/shadow" would send us reading outside them. An absolute path is
// still honoured, because that is the user passing -i explicitly.
func findPrivateKeyFromName(keyname string, keyFolders ...string) (privateKey, bool) {
	var priv privateKey

	if len(keyname) == 0 {
		return priv, false
	}

	keyPaths := []string{
		keyname,
	}
	if !strings.HasPrefix(keyname, ".pem") {
		keyPaths = append(keyPaths, fmt.Sprintf("%s.pem", keyname))
	}
	for _, folder := range keyFolders {
		if filepath.IsAbs(keyname) {
			break
		}
		if _, err := os.Stat(folder); err != nil {
			continue
		}
		keyPaths = append(keyPaths, filepath.Join(folder, keyname))
		if !strings.HasPrefix(keyname, ".pem") {
			keyPaths = append(keyPaths, filepath.Join(folder, fmt.Sprintf("%s.pem", keyname)))
		}
	}

	for _, path := range keyPaths {
		if !filepath.IsAbs(keyname) && escapesFolders(path, keyFolders) {
			logger.Verbosef("ignoring key path %q: it resolves outside %v", path, keyFolders)
			continue
		}
		b, err := os.ReadFile(path)
		if err == nil {
			warnOnLooseKeyPermissions(path)
			priv.path = path
			priv.body = b
			return priv, true
		}
		if !os.IsNotExist(err) {
			return priv, false
		}
	}

	return priv, false
}

// escapesFolders reports whether path, once ".." is resolved, still sits inside one
// of folders. A bare name with no separators is also accepted: that is the common
// case of looking in the current directory.
func escapesFolders(path string, folders []string) bool {
	if filepath.Clean(path) == filepath.Base(path) {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	for _, folder := range folders {
		absFolder, err := filepath.Abs(folder)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absFolder, abs)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

// warnOnLooseKeyPermissions mirrors what openssh does when a private key is
// readable by anyone besides its owner, except that it warns instead of refusing:
// the key may still be usable, and a read-only tool failing to connect is worse
// than a noisy one. Directory and symlink modes are not inspected — os.Stat follows
// the link, which is the file that actually gets read.
func warnOnLooseKeyPermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		logger.Warningf("permissions %#o on private key %q are too open; it is readable by others. openssh would refuse this key", mode, path)
	}
}

func checkHostKey(hostname string, remote net.Addr, key gossh.PublicKey) error {
	var knownHostsFiles []string
	var fileToAddKnownKey string

	opensshFile := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	if _, err := os.Stat(opensshFile); err == nil {
		knownHostsFiles = append(knownHostsFiles, opensshFile)
		fileToAddKnownKey = opensshFile
	}

	awlessFile := filepath.Join(os.Getenv("__AWLESS_HOME"), "known_hosts")
	if _, err := os.Stat(awlessFile); err == nil {
		knownHostsFiles = append(knownHostsFiles, awlessFile)
	}
	if fileToAddKnownKey == "" {
		fileToAddKnownKey = awlessFile
	}

	checkKnownHostFunc, err := knownhosts.New(knownHostsFiles...)
	if err != nil {
		return err
	}
	knownhostsErr := checkKnownHostFunc(hostname, remote, key)
	keyError, ok := knownhostsErr.(*knownhosts.KeyError)
	if !ok {
		return knownhostsErr
	}
	if len(keyError.Want) == 0 {
		if trustKeyFunc(hostname, remote, key, fileToAddKnownKey) {
			f, err := os.OpenFile(fileToAddKnownKey, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString(knownhosts.Line([]string{hostname}, key) + "\n")
			return err
		} else {
			return errors.New("Host public key verification failed.")
		}
	}

	var knownKeyInfos string
	var knownKeyFiles []string
	for _, knownKey := range keyError.Want {
		knownKeyInfos += fmt.Sprintf("\n-> %s (%s key in %s:%d)", gossh.FingerprintSHA256(knownKey.Key), knownKey.Key.Type(), knownKey.Filename, knownKey.Line)
		knownKeyFiles = append(knownKeyFiles, fmt.Sprintf("'%s:%d'", knownKey.Filename, knownKey.Line))
	}

	return fmt.Errorf(`
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
AWLESS DETECTED THAT THE REMOTE HOST PUBLIC KEY HAS CHANGED
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@

Host key for '%s' has changed and you did not disable strict host key checking.
Someone may be trying to intercept your connection (man-in-the-middle attack). Otherwise, the host key may have been changed.

The fingerprint for the %s key sent by the remote host is %s.
You persisted:%s

To get rid of this message, update %s`, hostname, key.Type(), gossh.FingerprintSHA256(key), knownKeyInfos, strings.Join(knownKeyFiles, ","))
}

var trustKeyFunc func(hostname string, remote net.Addr, key gossh.PublicKey, keyFileName string) bool = func(hostname string, remote net.Addr, key gossh.PublicKey, keyFileName string) bool {
	fmt.Printf("awless-ro could not validate the authenticity of '%s' (unknown host)\n", hostname)
	fmt.Printf("%s public key fingerprint is %s.\n", key.Type(), gossh.FingerprintSHA256(key))
	fmt.Printf("Do you want to continue connecting and persist this key to '%s' (yes/no)? ", keyFileName)
	var yesorno string
	_, err := fmt.Scanln(&yesorno)
	if err != nil {
		return false
	}
	return strings.ToLower(yesorno) == "yes"
}
