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
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/logger"
)

var onlyMyIPFlag, onlyMyNameFlag, onlyMyTypeFlag, onlyMyIDFlag, onlyMyAccountFlag, onlyMyResourcePathFlag bool

func init() {
	RootCmd.AddCommand(whoamiCmd)

	whoamiCmd.Flags().BoolVar(&onlyMyIPFlag, "ip-only", false, "Only returns your IP address as seen by AWS")
	whoamiCmd.Flags().BoolVar(&onlyMyNameFlag, "name-only", false, "Only returns the name of the resource (ex: username for a user)")
	whoamiCmd.Flags().BoolVar(&onlyMyTypeFlag, "type-only", false, "Only returns the type of the resource (ex: user for a user)")
	whoamiCmd.Flags().BoolVar(&onlyMyIDFlag, "id-only", false, "Only returns the ID of the resource")
	whoamiCmd.Flags().BoolVar(&onlyMyAccountFlag, "account-only", false, "Only returns the AWS account number")
	whoamiCmd.Flags().BoolVar(&onlyMyResourcePathFlag, "resource-only", false, "Only returns the AWS ARN resource path suffix (ex: user/jsmith)")
}

var whoamiCmd = &cobra.Command{
	Use:               "whoami",
	ValidArgsFunction: cobra.NoFileCompletions,
	Aliases:           []string{"who"},
	PersistentPreRun:  applyHooks(initAwlessEnvHook, initLoggerHook, initCloudServicesHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(onVersionUpgrade, networkMonitorHook),
	Short:             "Show your account, attached (i.e. managed) and inlined policies",

	Run: func(cmd *cobra.Command, args []string) {
		if onlyMyIPFlag {
			ip := getMyIP()
			if jsonOutput() {
				// Text prints <nil> when the address could not be read; a JSON
				// reader gets a failure instead of a value to misread.
				if ip == nil {
					exitOn(errors.New("cannot read your public IP address from https://checkip.amazonaws.com"))
				}
				printJSON(map[string]string{"ip": ip.String()})
				return
			}
			fmt.Println(ip)
			return
		}

		if localGlobalFlag {
			// Nothing on stdout and exit 0 would hand a JSON reader an empty document.
			if jsonOutput() {
				exitOn(errors.New("whoami asks AWS, which --local rules out"))
			}
			logger.Warning("`--local` flag prevent the command from fetching remote information")
			return
		}

		me, err := awsservices.AccessService.(*awsservices.Access).GetIdentity(context.Background())
		exitOn(err)

		if me.IsRoot() {
			logger.Warning("You are currently root")
			logger.Warning("Best practices suggest to create a new user and affecting it roles of access")
		}

		if key, value, ok := whoamiOnly(me); ok {
			if jsonOutput() {
				printJSON(map[string]string{key: value})
			} else {
				fmt.Println(value)
			}
			return
		}

		if jsonOutput() {
			var policies *awsservices.UserPolicies
			if me.IsUserType() {
				policies = userPolicies(me.Resource)
			}
			printJSON(newWhoamiOutput(me, policies))
			return
		}

		if !me.IsUserType() {
			fmt.Printf("ResourceType: %s, Resource: %s, Id: %s, Account: %s\n", me.ResourceType, me.Resource, me.UserId, me.Account)
			return
		}

		fmt.Printf("Username: %s, Id: %s, Account: %s\n", me.Resource, me.UserId, me.Account)

		policies := userPolicies(me.Resource)
		if policies == nil {
			return
		}

		if attached := policies.Attached; len(attached) > 0 {
			fmt.Println("\nAttached policies (i.e. managed):")
			for _, name := range attached {
				fmt.Printf("\t- %s\n", name)
			}
		} else {
			fmt.Println("\nAttached policies (i.e. managed): none")
		}
		if inlined := policies.Inlined; len(inlined) > 0 {
			fmt.Println("\nInlined policies:")
			for _, name := range inlined {
				fmt.Printf("\t- %s\n", name)
			}
		} else {
			fmt.Println("\nInlined policies: none")
		}
		if byGroup := policies.ByGroup; len(byGroup) > 0 {
			for g, pol := range byGroup {
				fmt.Printf("\nPolicies from group '%s': %s\n", g, strings.Join(pol, ", "))
			}
		}
	},
}

// whoamiOnly answers the --*-only flags: the one value asked for, and the key it has
// in the JSON output.
func whoamiOnly(me *awsservices.Identity) (key, value string, ok bool) {
	switch {
	case onlyMyAccountFlag:
		return "account", me.Account, true
	case onlyMyIDFlag:
		return "userId", me.UserId, true
	case onlyMyNameFlag:
		return "resource", me.Resource, true
	case onlyMyTypeFlag:
		return "resourceType", me.ResourceType, true
	case onlyMyResourcePathFlag:
		return "resourcePath", me.ResourcePath, true
	}
	return "", "", false
}

// userPolicies reads the policies of a user, or says on stderr why it could not and
// returns nil.
func userPolicies(username string) *awsservices.UserPolicies {
	policies, err := awsservices.AccessService.(*awsservices.Access).GetUserPolicies(context.Background(), username)
	if err != nil {
		if awsfetch.IsAccessDenied(err) {
			logger.Warningf("user '%s' is not authorized to list its policies", username)
		} else {
			logger.Error(err)
		}
		return nil
	}
	return policies
}

// whoamiOutput is `whoami --format json`.
type whoamiOutput struct {
	Account      string `json:"account"`
	Arn          string `json:"arn"`
	UserID       string `json:"userId"`
	Resource     string `json:"resource"`
	ResourceType string `json:"resourceType"`
	ResourcePath string `json:"resourcePath"`
	Root         bool   `json:"root"`
	// Only for a user whose policies could be read: absent says "not known", which
	// an empty list would misstate as "none".
	Policies *whoamiPolicies `json:"policies,omitempty"`
}

type whoamiPolicies struct {
	Attached []string `json:"attached"`
	Inlined  []string `json:"inlined"`
	// Policy names by the group they come from.
	ByGroup map[string][]string `json:"byGroup"`
}

func newWhoamiOutput(me *awsservices.Identity, policies *awsservices.UserPolicies) whoamiOutput {
	out := whoamiOutput{
		Account:      me.Account,
		Arn:          me.Arn,
		UserID:       me.UserId,
		Resource:     me.Resource,
		ResourceType: me.ResourceType,
		ResourcePath: me.ResourcePath,
		Root:         me.IsRoot(),
	}
	if policies != nil {
		out.Policies = &whoamiPolicies{
			Attached: sortedOrEmpty(policies.Attached),
			Inlined:  sortedOrEmpty(policies.Inlined),
			ByGroup:  make(map[string][]string, len(policies.ByGroup)),
		}
		for group, names := range policies.ByGroup {
			out.Policies.ByGroup[group] = sortedOrEmpty(names)
		}
	}
	return out
}

// sortedOrEmpty is a sorted copy of s, and [] rather than null when s is empty. The
// policies are collected concurrently, so their order is the order the calls returned.
func sortedOrEmpty(s []string) []string {
	out := append([]string{}, s...)
	sort.Strings(out)
	return out
}

// getMyIP asks AWS for the public address this machine appears to come from. It
// is used by `whoami --ip-only` and by `ssh`, which suggests a security group
// rule when a connection is refused.
//
// Over HTTPS, deliberately: upstream used plaintext HTTP, which both announced
// use of the tool to anyone on the path and let them dictate the answer — and the
// answer ends up in a security group suggestion. A wrong address there is advice
// to open a port to someone else. The body is length-limited because nothing
// forces a server to keep its reply small.
func getMyIP() net.IP {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://checkip.amazonaws.com/")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return nil
	}
	return net.ParseIP(strings.TrimSpace(string(b)))
}
