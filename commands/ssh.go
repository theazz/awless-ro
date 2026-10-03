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
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// sshDisabledReason is what `awless-ro ssh` says instead of running.
//
// The command is kept, rather than removed, so that trying it explains itself instead
// of answering "unknown command" — a fork that quietly dropped a headline feature
// would be worse than one that says it is not ready.
//
// What was found against a live account, and what has to be fixed before this comes
// back on:
//
//   - `ssh <instance> --local` panics. initCloudServicesHook returns early for
//     --local, so awsservices.InfraService is nil, and fetchConnectionInfo fetches
//     from AWS regardless and dereferences it. The command should read the local
//     graph when told to work offline, which is the whole point of the flag.
//   - `--print-cli` and `--print-config` connect to the instance. They exist to print
//     a command line and a config stanza, so they must resolve and print without
//     opening a connection.
//   - The host key prompt loops when there is no terminal, re-asking for every
//     candidate login user and never reading an answer. Same defect that was fixed in
//     credentials: a prompt has to check for a TTY and fail with a message when there
//     is none.
//
// Encouragingly, the parts underneath work: the name resolved to the instance's
// private IP through the local graph, and the connection reached it and returned a
// real host key.
const sshDisabledReason = `ssh is not available in this release.

The connection logic works, but three things have to be fixed first: --local panics,
--print-cli connects instead of printing, and the host key prompt loops when there is
no terminal. Shipping it would crash on the first offline use.

Meanwhile, awless-ro will tell you what to connect to:

    awless-ro show %s --local

and ssh does the rest:

    ssh -i ~/.ssh/<key>.pem <user>@<ip>

Progress: https://github.com/theazz/awless-ro/issues/1`

var sshCmd = &cobra.Command{
	Use:   "ssh [USER@]INSTANCE",
	Short: "[not in this release] Launch a SSH session to an instance given an id or alias",
	Long:  "Launch a SSH session to an instance given an id or alias. All connection details are derived from a given instance name/id.",
	Example: `  awless-ro ssh i-8d43b21b                       # using the instance id
  awless-ro ssh redis-prod                       # using name only (other infos are derived)
  awless-ro ssh ec2-user@redis-prod              # forcing the user
  awless-ro ssh 34.215.29.221                    # using the IP
  awless-ro ssh root@34.215.29.221 --port 23     # specifying a port

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

	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("instance required")
		}

		// Refused before anything else, and before any AWS call. See
		// sshDisabledReason for what is wrong and what has to be fixed; runSSH below
		// is the command as it stands, kept whole so that turning it back on is
		// deleting this line.
		return fmt.Errorf(sshDisabledReason, args[0])
	},
}

// runSSH is the ssh command, not currently reachable. See sshDisabledReason.
func runSSH(cmd *cobra.Command, args []string) error {
	{
		var err error
		var connectionCtx *instanceConnectionContext

		if proxyInstanceThroughFlag != "" {
			connectionCtx, err = initInstanceConnectionContext(proxyInstanceThroughFlag, keyPathFlag)
		} else {
			connectionCtx, err = initInstanceConnectionContext(args[0], keyPathFlag)
		}
		exitOn(err)

		firsHopClient, err := ssh.InitClient(connectionCtx.keypath, config.KeysDir, filepath.Join(os.Getenv("HOME"), ".ssh"))
		exitOn(err)

		if err != nil && strings.Contains(err.Error(), "cannot find SSH key") && keyPathFlag == "" {
			logger.Info("you may want to specify a key filepath with `-i /path/to/key.pem`")
		}
		exitOn(err)

		firsHopClient.SetLogger(logger.DefaultLogger)
		firsHopClient.SetStrictHostKeyChecking(!disableStrictHostKeyCheckingFlag)
		firsHopClient.InteractiveTerminalFunc = console.InteractiveTerminal
		if proxyInstanceThroughFlag != "" {
			firsHopClient.Port = sshTroughPortFlag
		} else {
			firsHopClient.Port = sshPortFlag
		}

		if privateIPFlag {
			if priv := connectionCtx.privip; priv != "" {
				firsHopClient.IP = connectionCtx.privip
			} else {
				exitOn(fmt.Errorf(
					"no private IP resolved for instance %s (state '%s')",
					connectionCtx.instance.Id(), connectionCtx.state,
				))
			}
		} else {
			if pub := connectionCtx.ip; pub != "" {
				firsHopClient.IP = connectionCtx.ip
			} else if priv := connectionCtx.privip; priv != "" {
				firsHopClient.IP = connectionCtx.privip
			} else {
				exitOn(fmt.Errorf("no public/private IP resolved for instance %s (state '%s')", connectionCtx.instance.Id(), connectionCtx.state))
			}
		}

		if connectionCtx.user != "" {
			err = firsHopClient.DialWithUsers(connectionCtx.user)
		} else {
			err = firsHopClient.DialWithUsers(defaultAMIUsers...)
		}

		if isConnectionRefusedErr(err) {
			logger.Warning("cannot connect to this instance, maybe the system is still booting?")
			exitOn(err)
			return nil
		}

		if err != nil {
			if e := connectionCtx.checkInstanceAccessible(); e != nil {
				logger.Error(e.Error())
			}
			exitOn(err)
		}

		targetClient := firsHopClient

		if proxyInstanceThroughFlag != "" {
			destInstanceCtx, err := initInstanceConnectionContext(args[0], keyPathFlag)
			exitOn(err)
			if destInstanceCtx.user != "" {
				targetClient, err = firsHopClient.NewClientWithProxy(destInstanceCtx.privip, sshPortFlag, destInstanceCtx.user)
			} else {
				targetClient, err = firsHopClient.NewClientWithProxy(destInstanceCtx.privip, sshPortFlag, defaultAMIUsers...)
			}
			exitOn(err)
		}

		if printSSHConfigFlag {
			host := connectionCtx.instanceName
			if proxyInstanceThroughFlag != "" {
				host = args[0]
			}
			fmt.Println(targetClient.SSHConfigString(host))
			return nil
		}

		if printSSHCLIFlag {
			fmt.Println(targetClient.ConnectString())
			return nil
		}

		exitOn(targetClient.Connect())
		return nil
	}
}

func isConnectionRefusedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "connection refused")
}

type instanceConnectionContext struct {
	ip, privip          string
	myip                net.IP
	user, keypath       string
	state, instanceName string
	instance            cloud.Resource
	resourcesGraph      cloud.GraphAPI
}

func initInstanceConnectionContext(userhost, keypath string) (*instanceConnectionContext, error) {
	ctx := &instanceConnectionContext{}

	if strings.Contains(userhost, "@") {
		ctx.user = strings.Split(userhost, "@")[0]
		ctx.instanceName = strings.Split(userhost, "@")[1]
	} else {
		ctx.instanceName = userhost
	}

	ctx.fetchConnectionInfo()

	instanceMatchers := match.Or(match.Property(properties.Name, ctx.instanceName), match.Property(properties.PublicIP, ctx.instanceName), match.Property(properties.PrivateIP, ctx.instanceName))
	resources, err := ctx.resourcesGraph.Find(cloud.NewQuery(cloud.Instance).Match(instanceMatchers))
	exitOn(err)
	switch len(resources) {
	case 0:
		// No instance with that name, use the id
		ctx.instance, err = findResource(ctx.resourcesGraph, ctx.instanceName, cloud.Instance)
		exitOn(err)
	case 1:
		ctx.instance = resources[0]
	default:
		idStatus := cloud.Resources(resources).Map(func(r cloud.Resource) string {
			return fmt.Sprintf("%s (%s)", r.Id(), r.Properties()[properties.State])
		})
		logger.Infof("Found %d resources with name '%s': %s", len(resources), ctx.instanceName, strings.Join(idStatus, ", "))

		var running []cloud.Resource
		running, err = ctx.resourcesGraph.Find(cloud.NewQuery(cloud.Instance).Match(match.And(instanceMatchers, match.Property(properties.State, "running"))))
		exitOn(err)

		switch len(running) {
		case 0:
			logger.Warning("None of them is running, cannot connect through SSH")
			return ctx, errors.New("non running instances")
		case 1:
			logger.Infof("Found only one instance running: %s. Will connect to this instance.", running[0].Id())
			ctx.instance = running[0]
		default:
			logger.Warning("Connect through the running ones using their id:")
			for _, res := range running {
				var up string
				if uptime, ok := res.Properties()[properties.Launched].(time.Time); ok {
					up = fmt.Sprintf("\t\t(uptime: %s)", console.HumanizeTime(uptime))
				}
				logger.Warningf("\t`awless-ro ssh %s`%s", res.Id(), up)
			}
			return ctx, errors.New("use instances ids")
		}
	}

	ctx.privip, _ = ctx.instance.Properties()[properties.PrivateIP].(string)
	ctx.ip, _ = ctx.instance.Properties()[properties.PublicIP].(string)
	ctx.state, _ = ctx.instance.Properties()[properties.State].(string)

	if keypath != "" {
		ctx.keypath = keypath
	} else {
		keypair, ok := ctx.instance.Properties()[properties.KeyPair].(string)
		if ok {
			ctx.keypath = fmt.Sprint(keypair)
		}
	}

	return ctx, nil
}

func (ctx *instanceConnectionContext) fetchConnectionInfo() {
	var resourcesGraph, sgroupsGraph cloud.GraphAPI
	var myip net.IP
	var wg sync.WaitGroup
	var errc = make(chan error)

	wg.Add(1)
	go func() {
		var err error
		defer wg.Done()
		resourcesGraph, err = awsservices.InfraService.FetchByType(context.WithValue(context.Background(), "force", true), cloud.Instance)
		if err != nil {
			errc <- err
		}
	}()

	wg.Add(1)
	go func() {
		var err error
		defer wg.Done()
		sgroupsGraph, err = awsservices.InfraService.FetchByType(context.WithValue(context.Background(), "force", true), cloud.SecurityGroup)
		if err != nil {
			errc <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		myip = getMyIP()
	}()
	go func() {
		wg.Wait()
		close(errc)
	}()
	for err := range errc {
		if err != nil {
			exitOn(err)
		}
	}
	resourcesGraph.Merge(sgroupsGraph)

	ctx.resourcesGraph = resourcesGraph
	ctx.myip = myip
	return
}

func (ctx *instanceConnectionContext) checkInstanceAccessible() (err error) {
	if st := ctx.state; st != "running" {
		logger.Warningf("this instance is '%s' (cannot ssh to a non running state)", st)
		if st == "stopped" {
			logger.Warningf("awless-ro is read-only; you can start it with `aws ec2 start-instances --instance-ids %s`", ctx.instance.Id())
		}
		return errors.New("instance not accessible")
	}

	sgroups, ok := ctx.instance.Properties()[properties.SecurityGroups].([]string)
	if ok {
		var sshPortOpen, myIPAllowed bool
		for _, id := range sgroups {
			var sgroup cloud.Resource
			sgroup, err = findResource(ctx.resourcesGraph, id, cloud.SecurityGroup)
			if err != nil {
				logger.Errorf("cannot get securitygroup '%s' for instance '%s': %s", id, ctx.instance.Id(), err)
				break
			}

			rules, ok := sgroup.Properties()[properties.InboundRules].([]*graph.FirewallRule)
			if ok {
				for _, r := range rules {
					if r.PortRange.Contains(22) {
						sshPortOpen = true
					}
					if ctx.myip != nil && r.Contains(ctx.myip.String()) {
						myIPAllowed = true
					}
				}
			}
		}

		if !sshPortOpen {
			logger.Warning("port 22 is not open on this instance")
			return errors.New("instance not accessible")
		}

		if !myIPAllowed && ctx.myip != nil {
			logger.Warningf("your ip %s is not authorized for this instance. You might want to update the securitygroup with:", ctx.myip)
			var group = "mygroup"
			if len(sgroups) == 1 {
				group = sgroups[0]
			}
			logger.Warningf("`aws ec2 authorize-security-group-ingress --group-id %s --protocol tcp --port 22 --cidr %s/32`", group, ctx.myip)
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
