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
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	awsconfig "github.com/theazz/awless-ro/aws/config"
	awsservices "github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/sync"
)

// Shell completion is cobra's own: `awless-ro completion bash|zsh|fish|powershell`
// prints a script that calls back into the binary through the hidden __complete
// command, which runs the ValidArgsFunction or flag completion function of the
// command being completed. Everything that knows what to offer is therefore Go, and
// the same for every shell.
//
// Every function here answers from local state only: the synced graph, the config
// definitions, the files under ~/.aws. Completion runs on each Tab press, so it must
// never call AWS, never start a sync and never run first-time setup. That holds by
// construction rather than by care: __complete is a child of RootCmd, so the hooks
// of the command being completed — environment init, credentials, autosync — do not
// run, and RootCmd itself has none.

// completeWith turns values into completions for toComplete, keeping file names out
// of the suggestions. Each value may carry a description after a tab.
func completeWith(values []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var out []cobra.Completion
	for _, v := range values {
		if strings.HasPrefix(v, toComplete) {
			out = append(out, v)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// fixedCompletion completes the first argument, or a flag value, from a fixed list.
func fixedCompletion(values ...string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return completeWith(values, toComplete)
	}
}

// completionProfiles lists the profiles defined in ~/.aws/{config,credentials},
// sorted and once each. Section names containing a space are not profiles —
// "sso-session corp" and "services local" are other kinds of section — and could not
// be completed as a single word anyway.
func completionProfiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range awsconfig.AllProfiles() {
		if strings.ContainsAny(p, " \t") || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func completeRegions(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return completeWith(awsconfig.SuggestedRegions, toComplete)
}

func completeProfiles(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return completeWith(completionProfiles(), toComplete)
}

// completeSwitch offers regions and profiles, in either order, at most one of each.
func completeSwitch(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var haveRegion, haveProfile bool
	for _, a := range args {
		if awsconfig.IsValidRegion(a) {
			haveRegion = true
		} else {
			haveProfile = true
		}
	}
	var values []string
	if !haveRegion {
		values = append(values, awsconfig.SuggestedRegions...)
	}
	if !haveProfile {
		values = append(values, completionProfiles()...)
	}
	return completeWith(values, toComplete)
}

func completeConfigKey(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var values []string
	for _, k := range config.Keys() {
		values = append(values, cobra.CompletionWithDesc(k.Key, k.Help))
	}
	return completeWith(values, toComplete)
}

// completeConfigSet completes the key, then a value for it where the key has a known
// set of values: a region, a profile, true or false.
func completeConfigSet(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeConfigKey(cmd, args, toComplete)
	case 1:
		if args[0] == config.ProfileConfigKey {
			return completeWith(completionProfiles(), toComplete)
		}
		return completeWith(config.ValuesFor(args[0]), toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// localCompletionGraph loads the synced graphs of the given services for the current
// profile and region. It reports false when there is nothing to offer, which includes
// a machine where awless-ro has never run: config.InitAwlessEnv is deliberately not
// called, because on a first run it starts interactive setup.
func localCompletionGraph(services ...string) (cloud.GraphAPI, bool) {
	if _, err := os.Stat(config.DBPath); err != nil {
		return nil, false
	}
	if err := config.LoadConfig(); err != nil {
		return nil, false
	}
	if err := applyRegionAndProfilePrecedence(); err != nil {
		return nil, false
	}
	profile, region := config.GetAWSProfile(), config.GetAWSRegion()

	g := graph.NewGraph()
	for _, s := range services {
		if err := g.Merge(sync.LoadLocalGraphForService(s, profile, region)); err != nil {
			return nil, false
		}
	}
	return g, true
}

// resourceRefs lists what a REFERENCE argument accepts for resources of the given
// types — ids, and names where a resource has one — each described by its type and
// the other half of the pair. A name shared by several resources is offered once.
// Names containing whitespace are left out: a shell cannot complete them as one word.
func resourceRefs(g cloud.GraphAPI, types []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value, desc string) {
		if value == "" || seen[value] || strings.ContainsAny(value, " \t\n") {
			return
		}
		seen[value] = true
		out = append(out, cobra.CompletionWithDesc(value, desc))
	}

	for _, typ := range types {
		resources, err := g.Find(cloud.NewQuery(typ))
		if err != nil {
			continue
		}
		for _, r := range resources {
			name, _ := r.Property(properties.Name)
			nameStr, _ := name.(string)
			if nameStr != "" {
				add(r.Id(), typ+" "+nameStr)
				add(nameStr, typ+" "+r.Id())
			} else {
				add(r.Id(), typ)
			}
		}
	}
	sort.Strings(out)
	return out
}

// resourceRefCompletion completes a single REFERENCE argument from the synced graph.
func resourceRefCompletion(services []string, types []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		g, ok := localCompletionGraph(services...)
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeWith(resourceRefs(g, types), toComplete)
	}
}

// completeShowRef offers the infra and access resources, as the bash completion this
// replaces did: they are what people show, and loading every service on each Tab
// would make completion slow in a large account for little gain.
func completeShowRef(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	perService := awsservices.ResourceTypesPerServiceName()
	types := append(append([]string{}, perService["infra"]...), perService["access"]...)
	sort.Strings(types)
	return resourceRefCompletion([]string{"infra", "access"}, types)(cmd, args, toComplete)
}

// completeSSHTarget offers instances, keeping a USER@ prefix the user has typed.
func completeSSHTarget(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	user, rest, hasUser := strings.Cut(toComplete, "@")
	if !hasUser {
		rest = toComplete
	}
	completions, directive := resourceRefCompletion([]string{"infra"}, []string{cloud.Instance})(cmd, args, rest)
	if !hasUser {
		return completions, directive
	}
	for i, c := range completions {
		completions[i] = user + "@" + c
	}
	return completions, directive
}
