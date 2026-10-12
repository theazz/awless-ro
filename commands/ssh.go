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

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/match"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/console"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/ssh"
	"github.com/theazz/awless-ro/sync"
)

var keyPathFlag, proxyInstanceThroughFlag string
var sshPortFlag, sshTroughPortFlag int
var printSSHConfigFlag bool
var printSSHCLIFlag bool
var privateIPFlag bool
var disableStrictHostKeyCheckingFlag bool

func init() {
	RootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringVarP(&keyPathFlag, "identity", "i", "", "Path or name of the identity (key file). Without it, the key is found by name and then by matching the key pair's fingerprint in ~/.ssh and ~/.awless-ro/keys")
	sshCmd.Flags().IntVar(&sshPortFlag, "port", 22, "Set SSH target port")
	sshCmd.Flags().IntVar(&sshTroughPortFlag, "through-port", 22, "Set SSH proxy port")
	sshCmd.Flags().StringVar(&proxyInstanceThroughFlag, "through", "", "Name of instance to proxy through to connect to a destination host")
	sshCmd.Flags().BoolVar(&printSSHConfigFlag, "print-config", false, "Print SSH configuration for ~/.ssh/config file.")
	sshCmd.Flags().BoolVar(&printSSHCLIFlag, "print-cli", false, "Print the CLI one-liner to connect with SSH. (/usr/bin/ssh user@ip -i ...)")
	sshCmd.Flags().BoolVar(&privateIPFlag, "private", false, "Use private ip to connect to host")
	sshCmd.Flags().BoolVar(&disableStrictHostKeyCheckingFlag, "disable-strict-host-keychecking", false, "Disable the remote host key check from ~/.ssh/known_hosts or ~/.awless/known_hosts file")
}

var defaultAMIUsers = []string{"ec2-user", "ubuntu", "centos", "core", "bitnami", "admin", "root"}

var sshCmd = &cobra.Command{
	Use:               "ssh [USER@]INSTANCE",
	ValidArgsFunction: completeSSHTarget,
	Short:             "Launch a SSH session to an instance given an id or alias",
	Long:              "Launch a SSH session to an instance given an id or alias. All connection details are derived from a given instance name/id.",
	Example: `  awless-ro ssh i-8d43b21b                       # using the instance id
  awless-ro ssh web-1                            # using name only (other infos are derived)
  awless-ro ssh ec2-user@web-1                   # forcing the user
  awless-ro ssh 203.0.113.10                     # using the IP
  awless-ro ssh root@203.0.113.10 --port 23      # specifying a port
  awless-ro ssh web-1 --local                    # resolve from the synced graph, no AWS call

  awless-ro ssh web-1                            # key found by name, or by matching the key pair fingerprint in ~/.ssh & ~/.awless-ro/keys
  awless-ro ssh web-1 -i keyname                 # using AWS keyname (look into ~/.ssh/keyname.pem & ~/.awless-ro/keys/keyname.pem)
  awless-ro ssh web-1 -i ~/path/toward/key       # specifying a full key path

  awless-ro ssh db-private --through my-bastion  # connect to a private inst through a public one
  awless-ro ssh db-private --private             # connect using the private IP (when you have a VPN, tunnel, etc ...)

  awless-ro ssh web-1 --print-cli                # print out the full terminal command to connect to instance
  awless-ro ssh web-1 --print-config             # print out the full SSH config (i.e: ~/.ssh/config) to connect to instance
  
  awless-ro ssh private-redis --through my-proxy                                # connect to private through proxy instance
  awless-ro ssh private-redis --through my-proxy --through-port 23              # specifying proxy port
  awless-ro ssh 172.31.77.151 --port 2222 --through my-proxy --through-port 23  # specifying target & proxy port`,

	PersistentPreRun:  applyHooks(initLoggerHook, initAwlessEnvHook, initCloudServicesHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(onVersionUpgrade, networkMonitorHook),

	RunE: runSSH,
}

// sshOptions is the command line of `ssh`, gathered so the pieces below can be
// tested without the package-level flag variables.
type sshOptions struct {
	identity, through                                      string
	port, throughPort                                      int
	private, noStrictHostKey, printCLI, printConfig, local bool
}

func sshOptionsFromFlags() sshOptions {
	return sshOptions{
		identity:        keyPathFlag,
		through:         proxyInstanceThroughFlag,
		port:            sshPortFlag,
		throughPort:     sshTroughPortFlag,
		private:         privateIPFlag,
		noStrictHostKey: disableStrictHostKeyCheckingFlag,
		printCLI:        printSSHCLIFlag,
		printConfig:     printSSHConfigFlag,
		local:           localGlobalFlag,
	}
}

func runSSH(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("instance required")
	}
	opts := sshOptionsFromFlags()

	g, err := connectionGraph(opts.local, awsservices.InfraService, localInfraGraph)
	exitOn(err)

	// Printing resolves from the graph and returns before anything dials: no
	// connection is opened and no host key is checked, so it works when the host
	// is unreachable.
	if opts.printCLI || opts.printConfig {
		exitOn(decorateSSHNotFound(opts, printSSH(cmd.OutOrStdout(), opts, g, args[0])))
		return nil
	}

	myIP := getMyIP
	if opts.local {
		// --local means no network call to anything but the instance itself.
		myIP = func() net.IP { return nil }
	}
	exitOn(decorateSSHNotFound(opts, connectSSH(opts, g, args[0], myIP)))
	return nil
}

func isConnectionRefusedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "connection refused")
}

// infraFetcher is the part of the infra service ssh needs.
type infraFetcher interface {
	FetchByType(context.Context, string) (cloud.GraphAPI, error)
}

// connectionGraph is what ssh resolves its target against: the synced graph with
// --local, a fresh fetch otherwise.
//
// With --local, initCloudServicesHook never builds the AWS services, so infra is nil
// and must not be touched; reading the graph is the whole point of the flag.
// Upstream fetched from AWS regardless, and panicked.
func connectionGraph(local bool, infra infraFetcher, loadLocal func() (cloud.GraphAPI, error)) (cloud.GraphAPI, error) {
	if local {
		return loadLocal()
	}
	if infra == nil {
		return nil, errors.New("the AWS infra service is not initialised, cannot fetch instances (use --local to read the synced graph)")
	}

	type result struct {
		typ string
		g   cloud.GraphAPI
		err error
	}
	types := []string{cloud.Instance, cloud.SecurityGroup, cloud.Image}
	results := make(chan result, len(types))
	ctx := context.WithValue(context.Background(), "force", true)
	for _, typ := range types {
		go func() {
			g, err := infra.FetchByType(ctx, typ)
			results <- result{typ, g, err}
		}()
	}

	merged := graph.NewGraph()
	var firstErr error
	for range types {
		r := <-results
		if r.err != nil {
			// Images only tell which login user to try first; without them every
			// default user is tried, which is how upstream always worked.
			if r.typ == cloud.Image {
				logger.Verbosef("cannot fetch images, login users will be guessed: %s", r.err)
				continue
			}
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		if r.g != nil {
			if err := merged.Merge(r.g); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return merged, nil
}

// localInfraGraph reads the synced infra file, which holds the instances, their
// security groups and the account's images.
func localInfraGraph() (cloud.GraphAPI, error) {
	warnIfNothingSynced()
	return sync.LoadLocalGraphForService("infra", config.GetAWSProfile(), config.GetAWSRegion())
}

// instanceNotFoundError keeps findResource's wording, but can be recognised, so
// that --local can say where it looked.
type instanceNotFoundError struct{ name string }

func (e instanceNotFoundError) Error() string {
	return fmt.Sprintf("%s '%s' not found", cloud.Instance, e.name)
}

func decorateSSHNotFound(opts sshOptions, err error) error {
	var nf instanceNotFoundError
	if opts.local && errors.As(err, &nf) {
		return fmt.Errorf("%w in the synced graph: run `awless-ro sync` or drop --local", err)
	}
	return err
}

func parseUserHost(s string) (user, host string) {
	if u, h, ok := strings.Cut(s, "@"); ok {
		return u, h
	}
	return "", s
}

// resolveInstance finds the instance a name, public IP, private IP or id refers to.
// Several matches resolve to the single running one, if there is exactly one.
func resolveInstance(g cloud.GraphAPI, name string) (cloud.Resource, error) {
	instanceMatchers := match.Or(match.Property(properties.Name, name), match.Property(properties.PublicIP, name), match.Property(properties.PrivateIP, name))
	resources, err := g.Find(cloud.NewQuery(cloud.Instance).Match(instanceMatchers))
	if err != nil {
		return nil, err
	}
	switch len(resources) {
	case 0:
		// No instance with that name, use the id
		inst, err := findResource(g, name, cloud.Instance)
		if err != nil {
			return nil, instanceNotFoundError{name}
		}
		return inst, nil
	case 1:
		return resources[0], nil
	}

	idStatus := cloud.Resources(resources).Map(func(r cloud.Resource) string {
		return fmt.Sprintf("%s (%s)", r.Id(), r.Properties()[properties.State])
	})
	logger.Infof("Found %d resources with name '%s': %s", len(resources), name, strings.Join(idStatus, ", "))

	running, err := g.Find(cloud.NewQuery(cloud.Instance).Match(match.And(instanceMatchers, match.Property(properties.State, "running"))))
	if err != nil {
		return nil, err
	}

	switch len(running) {
	case 0:
		logger.Warning("None of them is running, cannot connect through SSH")
		return nil, errors.New("non running instances")
	case 1:
		logger.Infof("Found only one instance running: %s. Will connect to this instance.", running[0].Id())
		return running[0], nil
	default:
		logger.Warning("Connect through the running ones using their id:")
		for _, res := range running {
			var up string
			if uptime, ok := res.Properties()[properties.Launched].(time.Time); ok {
				up = fmt.Sprintf("\t\t(uptime: %s)", console.HumanizeTime(uptime))
			}
			logger.Warningf("\t`awless-ro ssh %s`%s", res.Id(), up)
		}
		return nil, errors.New("use instances ids")
	}
}

// sshTarget is one hop, resolved from the graph: which instance, its addresses, the
// users to log in as and the key to use.
type sshTarget struct {
	name, user string
	// users are the login users to try, in order; never empty.
	users []string
	// userKnown says users[0] is not a guess: given explicitly, or derived from
	// the instance's image.
	userKnown                           bool
	instance                            cloud.Resource
	publicIP, privateIP, state, keyName string
	// keyFingerprint is the fingerprint AWS recorded for keyName, used to find a
	// differently-named local key. Empty with -i or when nothing has been synced.
	keyFingerprint string
}

func resolveTarget(g cloud.GraphAPI, userhost, identity string) (*sshTarget, error) {
	user, name := parseUserHost(userhost)
	inst, err := resolveInstance(g, name)
	if err != nil {
		return nil, err
	}

	t := &sshTarget{name: name, user: user, instance: inst}
	t.privateIP, _ = inst.Properties()[properties.PrivateIP].(string)
	t.publicIP, _ = inst.Properties()[properties.PublicIP].(string)
	t.state, _ = inst.Properties()[properties.State].(string)
	if identity != "" {
		t.keyName = identity
	} else {
		t.keyName, _ = inst.Properties()[properties.KeyPair].(string)
		if t.keyName != "" {
			t.keyFingerprint = keyPairFingerprint(g, t.keyName)
		}
	}
	t.users, t.userKnown = loginUsers(g, inst, user)
	return t, nil
}

// keyRef is what the ssh package needs to find the key: the name, and the
// fingerprint AWS recorded for it when it is known.
func (t *sshTarget) keyRef() ssh.KeyRef {
	return ssh.KeyRef{Name: t.keyName, Fingerprint: t.keyFingerprint}
}

// syncedInfraGraph loads the infra graph written by the last sync. A live run does
// not fetch key pairs (that would be a new AWS request this read-only tool must not
// add), so the fingerprint comes from this file. A variable so tests can replace it.
var syncedInfraGraph = func() (cloud.GraphAPI, error) {
	return sync.LoadLocalGraphForService("infra", config.GetAWSProfile(), config.GetAWSRegion())
}

// keyPairFingerprint is the fingerprint AWS recorded for a key pair: from the graph
// ssh already resolved against, falling back to the synced infra graph when the
// live graph has no key pair (it never fetches them). "" when neither has it; a read
// error is only worth a verbose line, the search just falls back to the name.
func keyPairFingerprint(g cloud.GraphAPI, name string) string {
	if res, err := findResource(g, name, cloud.Keypair); err == nil {
		if fp, _ := res.Properties()[properties.Fingerprint].(string); fp != "" {
			return fp
		}
	}
	synced, err := syncedInfraGraph()
	if err != nil {
		logger.Verbosef("cannot read the synced graph for key pair fingerprints: %s", err)
		return ""
	}
	if res, err := findResource(synced, name, cloud.Keypair); err == nil {
		if fp, _ := res.Properties()[properties.Fingerprint].(string); fp != "" {
			return fp
		}
	}
	return ""
}

// address is the IP to connect to: the private one with --private, otherwise the
// public one and, failing that, the private one.
func (t *sshTarget) address(private bool) (string, error) {
	if private {
		if t.privateIP != "" {
			return t.privateIP, nil
		}
		return "", fmt.Errorf("no private IP resolved for instance %s (state '%s')", t.instance.Id(), t.state)
	}
	if t.publicIP != "" {
		return t.publicIP, nil
	}
	if t.privateIP != "" {
		return t.privateIP, nil
	}
	return "", fmt.Errorf("no public/private IP resolved for instance %s (state '%s')", t.instance.Id(), t.state)
}

// loginUsers lists the users to try, most likely first. An explicit user is the
// only one tried. Otherwise the user that goes with the instance's image comes
// first, when the image is in the graph and recognised, followed by the defaults.
// known reports that the first user is not a guess.
func loginUsers(g cloud.GraphAPI, inst cloud.Resource, explicit string) (users []string, known bool) {
	if explicit != "" {
		return []string{explicit}, true
	}
	if imageID, _ := inst.Properties()[properties.Image].(string); imageID != "" {
		if img, err := findResource(g, imageID, cloud.Image); err == nil {
			if u := userForImage(img); u != "" {
				users = []string{u}
				for _, d := range defaultAMIUsers {
					if d != u {
						users = append(users, d)
					}
				}
				return users, true
			}
		}
	}
	return append([]string(nil), defaultAMIUsers...), false
}

// imageUsers maps a fragment of an image name or description to its login user.
// Order matters: Bitnami names contain their base distro, and Fedora CoreOS names
// contain "fedora".
var imageUsers = []struct {
	fragments []string
	user      string
}{
	{[]string{"bitnami"}, "bitnami"},
	{[]string{"ubuntu"}, "ubuntu"},
	{[]string{"debian"}, "admin"},
	{[]string{"centos"}, "centos"},
	{[]string{"flatcar", "coreos"}, "core"},
	{[]string{"fedora"}, "fedora"},
	{[]string{"rocky"}, "rocky"},
	{[]string{"amzn", "al2023", "amazon linux", "rhel", "red hat", "suse", "sles", "almalinux"}, "ec2-user"},
}

// userForImage is the login user an image's name, then its description, points to;
// "" when neither is recognised.
func userForImage(img cloud.Resource) string {
	for _, prop := range []string{properties.Name, properties.Description} {
		text, _ := img.Properties()[prop].(string)
		text = strings.ToLower(text)
		if text == "" {
			continue
		}
		for _, entry := range imageUsers {
			for _, fragment := range entry.fragments {
				if strings.Contains(text, fragment) {
					return entry.user
				}
			}
		}
	}
	return ""
}

// sshKeyFolders is where a key name is looked up, in order: the awless-ro keys
// directory, then ~/.ssh (#153).
func sshKeyFolders() []string {
	return []string{config.KeysDir, filepath.Join(os.Getenv("HOME"), ".ssh")}
}

// resolveTargets resolves the hop to dial first and, with --through, the
// destination behind it.
func resolveTargets(opts sshOptions, g cloud.GraphAPI, arg string) (first, dest *sshTarget, err error) {
	if opts.through == "" {
		first, err = resolveTarget(g, arg, opts.identity)
		return first, nil, err
	}
	if first, err = resolveTarget(g, opts.through, opts.identity); err != nil {
		return nil, nil, err
	}
	if dest, err = resolveTarget(g, arg, opts.identity); err != nil {
		return nil, nil, err
	}
	return first, dest, nil
}

// plannedKeyPath finds the target's key file without reading it, warning when it is
// missing: ssh may still authenticate with the agent or ~/.ssh/config.
func plannedKeyPath(t *sshTarget) string {
	if t.keyName == "" {
		return ""
	}
	path, ok := ssh.ResolveKeyPath(t.keyRef(), sshKeyFolders()...)
	if !ok {
		logger.Warningf("cannot find SSH key '%s' for %s in %s, by name or by fingerprint; printing without -i", t.keyName, t.instance.Id(), strings.Join(sshKeyFolders(), ", "))
	}
	return path
}

func warnAboutPlannedTarget(t *sshTarget) {
	if !t.userKnown {
		logger.Warningf("cannot tell the login user of %s from its image, guessing '%s'. Force it with USER@%s", t.name, t.users[0], t.name)
	}
	if t.state != "running" {
		logger.Warningf("instance %s is '%s', not running", t.instance.Id(), t.state)
	}
}

// plannedClient describes the connection --print-cli and --print-config print, from
// the graph and the flags alone. It does no network I/O: the user cannot be learnt by
// connecting, so it comes from loginUsers, and the key is found but never parsed.
func plannedClient(opts sshOptions, first, dest *sshTarget) (*ssh.Client, error) {
	port := opts.port
	if dest != nil {
		port = opts.throughPort
	}
	ip, err := first.address(opts.private)
	if err != nil {
		return nil, err
	}
	warnAboutPlannedTarget(first)
	hop := &ssh.Client{
		IP:                    ip,
		Port:                  port,
		User:                  first.users[0],
		Keypath:               plannedKeyPath(first),
		StrictHostKeyChecking: !opts.noStrictHostKey,
	}
	if dest == nil {
		return hop, nil
	}

	if dest.privateIP == "" {
		return nil, fmt.Errorf("no private IP resolved for instance %s (state '%s')", dest.instance.Id(), dest.state)
	}
	warnAboutPlannedTarget(dest)
	keypath := hop.Keypath
	if dest.keyName != first.keyName {
		if p := plannedKeyPath(dest); p != "" {
			keypath = p
		}
	}
	return &ssh.Client{
		IP:                    dest.privateIP,
		Port:                  opts.port,
		User:                  dest.users[0],
		Keypath:               keypath,
		Proxy:                 hop,
		StrictHostKeyChecking: !opts.noStrictHostKey,
	}, nil
}

// printSSH writes the ssh command line, or the ~/.ssh/config stanza, for arg.
func printSSH(w io.Writer, opts sshOptions, g cloud.GraphAPI, arg string) error {
	first, dest, err := resolveTargets(opts, g, arg)
	if err != nil {
		return err
	}
	client, err := plannedClient(opts, first, dest)
	if err != nil {
		return err
	}
	if opts.printConfig {
		// The Host line is the bare instance name, without any USER@: ssh matches
		// Host patterns against the host name alone.
		host := first.name
		if dest != nil {
			host = dest.name
		}
		_, err = fmt.Fprintln(w, client.SSHConfigString(host))
		return err
	}
	_, err = fmt.Fprintln(w, client.ConnectString())
	return err
}

// connectSSH dials the instance, through the jump host with --through, and hands
// the session over.
func connectSSH(opts sshOptions, g cloud.GraphAPI, arg string, myIP func() net.IP) error {
	client, err := dialSSH(opts, g, arg, myIP)
	if err != nil {
		return err
	}
	return client.Connect()
}

// dialSSH resolves the target(s) and opens the connection, through the jump host
// with --through, without handing the session over: that split lets a test dial a
// loopback server and inspect the result. myIP is only asked after a failed dial,
// to explain it.
func dialSSH(opts sshOptions, g cloud.GraphAPI, arg string, myIP func() net.IP) (*ssh.Client, error) {
	first, dest, err := resolveTargets(opts, g, arg)
	if err != nil {
		return nil, err
	}

	firstHop, err := ssh.InitClient(first.keyRef(), sshKeyFolders()...)
	if err != nil {
		return nil, err
	}
	firstHop.SetLogger(logger.DefaultLogger)
	firstHop.SetStrictHostKeyChecking(!opts.noStrictHostKey)
	firstHop.InteractiveTerminalFunc = console.InteractiveTerminal
	firstHop.Port = opts.port
	if dest != nil {
		firstHop.Port = opts.throughPort
	}
	if firstHop.IP, err = first.address(opts.private); err != nil {
		return nil, err
	}

	err = firstHop.DialWithUsers(ssh.LoginUsers{Names: first.users, Known: first.userKnown})
	if err != nil {
		return nil, diagnoseDialError(err, g, opts, first, dest, true, myIP)
	}

	if dest == nil {
		return firstHop, nil
	}
	if dest.privateIP == "" {
		return nil, fmt.Errorf("no private IP resolved for instance %s (state '%s')", dest.instance.Id(), dest.state)
	}
	// The destination's own key pair first, unless -i chose the key for both.
	var destKeypath string
	if opts.identity == "" {
		destKeypath, _ = ssh.ResolveKeyPath(dest.keyRef(), sshKeyFolders()...)
	}
	targetClient, err := firstHop.NewClientWithProxy(dest.privateIP, opts.port, destKeypath, ssh.LoginUsers{Names: dest.users, Known: dest.userKnown})
	if err != nil {
		return nil, diagnoseDialError(err, g, opts, first, dest, false, myIP)
	}
	return targetClient, nil
}

// diagnoseDialError turns a failed dial into the error the user sees. When every
// candidate user was refused it suggests giving one explicitly; a host that was
// reached (an authentication or host-key failure) is not diagnosed for
// reachability, which would needlessly ask the network for this machine's IP.
func diagnoseDialError(err error, g cloud.GraphAPI, opts sshOptions, first, dest *sshTarget, firstHop bool, myIP func() net.IP) error {
	if firstHop && isConnectionRefusedErr(err) {
		logger.Warning("cannot connect to this instance, maybe the system is still booting?")
		return err
	}
	var authErr *ssh.AuthError
	if errors.As(err, &authErr) {
		if len(authErr.Users) > 1 {
			return &sshAuthHintError{hint: authHint(first, dest, firstHop, authErr.Users), cause: authErr}
		}
		return err
	}
	// A refused host key means the host was reached: nothing to diagnose. Only the
	// first hop can be checked for reachability; the destination sits behind it.
	if firstHop && !ssh.IsHostKeyError(err) {
		if e := checkInstanceAccessible(first, g, myIP()); e != nil {
			logger.Error(e.Error())
		}
	}
	return err
}

// sshAuthHintError reads as the hint but still unwraps to the ssh.AuthError, so a
// caller can both show the suggestion and recognise the cause.
type sshAuthHintError struct {
	hint  string
	cause *ssh.AuthError
}

func (e *sshAuthHintError) Error() string { return e.hint }
func (e *sshAuthHintError) Unwrap() error { return e.cause }

// authHint tells the user to name the login user, with the command that does it for
// the hop that refused every user.
func authHint(first, dest *sshTarget, firstHop bool, users []string) string {
	var cmd string
	switch {
	case dest == nil:
		cmd = "awless-ro ssh USER@" + first.name
	case firstHop:
		cmd = fmt.Sprintf("awless-ro ssh %s --through USER@%s", dest.name, first.name)
	default:
		cmd = fmt.Sprintf("awless-ro ssh USER@%s --through %s", dest.name, first.name)
	}
	return fmt.Sprintf("none of %s accepted the key; give the user explicitly: %s", strings.Join(users, ", "), cmd)
}

func checkInstanceAccessible(t *sshTarget, g cloud.GraphAPI, myip net.IP) (err error) {
	if st := t.state; st != "running" {
		logger.Warningf("this instance is '%s' (cannot ssh to a non running state)", st)
		if st == "stopped" {
			logger.Warningf("awless-ro is read-only; you can start it with `aws ec2 start-instances --instance-ids %s`", t.instance.Id())
		}
		return errors.New("instance not accessible")
	}

	sgroups, ok := t.instance.Properties()[properties.SecurityGroups].([]string)
	if ok {
		var sshPortOpen, myIPAllowed bool
		for _, id := range sgroups {
			var sgroup cloud.Resource
			sgroup, err = findResource(g, id, cloud.SecurityGroup)
			if err != nil {
				logger.Errorf("cannot get securitygroup '%s' for instance '%s': %s", id, t.instance.Id(), err)
				break
			}

			rules, ok := sgroup.Properties()[properties.InboundRules].([]*graph.FirewallRule)
			if ok {
				for _, r := range rules {
					if r.PortRange.Contains(22) {
						sshPortOpen = true
					}
					if myip != nil && r.Contains(myip.String()) {
						myIPAllowed = true
					}
				}
			}
		}

		if !sshPortOpen {
			logger.Warning("port 22 is not open on this instance")
			return errors.New("instance not accessible")
		}

		if !myIPAllowed && myip != nil {
			logger.Warningf("your ip %s is not authorized for this instance. You might want to update the securitygroup with:", myip)
			var group = "mygroup"
			if len(sgroups) == 1 {
				group = sgroups[0]
			}
			logger.Warningf("`aws ec2 authorize-security-group-ingress --group-id %s --protocol tcp --port 22 --cidr %s/32`", group, myip)
			return errors.New("instance not accessible")
		}
	}

	return nil
}

func findResource(g cloud.GraphAPI, id, typ string) (cloud.Resource, error) {
	found, err := g.FindOne(cloud.NewQuery(typ).Match(match.Property(properties.ID, id)))
	if found == nil || err != nil {
		return nil, fmt.Errorf("%s '%s' not found", typ, id)
	}

	return found, nil
}
