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
	sshCmd.Flags().StringVarP(&keyPathFlag, "identity", "i", "", "Set path or name toward the identity (key file) to use to connect through SSH")
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
  awless-ro ssh redis-prod                       # using name only (other infos are derived)
  awless-ro ssh ec2-user@redis-prod              # forcing the user
  awless-ro ssh 34.215.29.221                    # using the IP
  awless-ro ssh root@34.215.29.221 --port 23     # specifying a port
  awless-ro ssh redis-prod --local               # resolve from the synced graph, no AWS call

  awless-ro ssh redis-prod -i keyname            # using AWS keyname (look into ~/.ssh/keyname.pem & ~/.awless-ro/keys/keyname.pem)
  awless-ro ssh redis-prod -i ~/path/toward/key  # specifying a full key path

  awless-ro ssh db-private --through my-bastion  # connect to a private inst through a public one
  awless-ro ssh db-private --private             # connect using the private IP (when you have a VPN, tunnel, etc ...)

  awless-ro ssh redis-prod --print-cli           # print out the full terminal command to connect to instance
  awless-ro ssh redis-prod --print-config        # print out the full SSH config (i.e: ~/.ssh/config) to connect to instance
  
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
	}
	t.users, t.userKnown = loginUsers(g, inst, user)
	return t, nil
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
	path, ok := ssh.ResolveKeyPath(t.keyName, sshKeyFolders()...)
	if !ok {
		logger.Warningf("cannot find SSH key '%s' for %s in %s; printing without -i", t.keyName, t.instance.Id(), strings.Join(sshKeyFolders(), ", "))
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
// the session over. myIP is only asked after a failed dial, to explain it.
func connectSSH(opts sshOptions, g cloud.GraphAPI, arg string, myIP func() net.IP) error {
	first, dest, err := resolveTargets(opts, g, arg)
	if err != nil {
		return err
	}

	firstHop, err := ssh.InitClient(first.keyName, sshKeyFolders()...)
	if err != nil {
		if errors.Is(err, ssh.ErrKeyNotFound) && opts.identity == "" {
			logger.Info("you may want to specify a key filepath with `-i /path/to/key.pem`")
		}
		return err
	}
	firstHop.SetLogger(logger.DefaultLogger)
	firstHop.SetStrictHostKeyChecking(!opts.noStrictHostKey)
	firstHop.InteractiveTerminalFunc = console.InteractiveTerminal
	firstHop.Port = opts.port
	if dest != nil {
		firstHop.Port = opts.throughPort
	}
	if firstHop.IP, err = first.address(opts.private); err != nil {
		return err
	}

	err = firstHop.DialWithUsers(first.users...)
	if isConnectionRefusedErr(err) {
		logger.Warning("cannot connect to this instance, maybe the system is still booting?")
		return err
	}
	if err != nil {
		// A refused host key means the host was reached: nothing to diagnose.
		if !ssh.IsHostKeyError(err) {
			if e := checkInstanceAccessible(first, g, myIP()); e != nil {
				logger.Error(e.Error())
			}
		}
		return err
	}

	targetClient := firstHop
	if dest != nil {
		if dest.privateIP == "" {
			return fmt.Errorf("no private IP resolved for instance %s (state '%s')", dest.instance.Id(), dest.state)
		}
		// The destination's own key pair first, unless -i chose the key for both.
		var destKeypath string
		if opts.identity == "" {
			destKeypath, _ = ssh.ResolveKeyPath(dest.keyName, sshKeyFolders()...)
		}
		targetClient, err = firstHop.NewClientWithProxy(dest.privateIP, opts.port, destKeypath, dest.users...)
		if err != nil {
			return err
		}
	}

	return targetClient.Connect()
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
