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
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/console"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/sync"
)

var (
	listingFormat              string
	listingFiltersFlag         []string
	listingTagFiltersFlag      []string
	listingTagKeyFiltersFlag   []string
	listingTagValueFiltersFlag []string
	listingColumnsFlag         []string
	listOnlyIDs                bool
	noHeadersFlag              bool
	sortBy                     []string
	reverseFlag                bool
)

func init() {
	RootCmd.AddCommand(listCmd)

	cobra.EnableCommandSorting = false

	for _, srvName := range awsservices.ServiceNames {
		listCmd.AddCommand(listAllResourceInServiceCmd(srvName))
	}

	for _, name := range awsservices.ServiceNames {
		var resources []string
		for _, resType := range awsservices.ResourceTypesPerServiceName()[name] {
			resources = append(resources, resType)
		}
		sort.Strings(resources)
		for _, resType := range resources {
			listCmd.AddCommand(listSpecificResourceCmd(resType))
		}
	}

	listCmd.PersistentFlags().StringVar(&listingFormat, "format", "table", "Output format: table, csv, tsv, json, porcelain")
	listCmd.RegisterFlagCompletionFunc("format", fixedCompletion("table", "csv", "tsv", "json", "porcelain"))
	listCmd.PersistentFlags().StringSliceVar(&listingFiltersFlag, "filter", []string{}, "Filter resources by column, case insensitive: key=value matches a substring, key==value matches the whole value. Ex: --filter type=t2.micro --filter state==running")
	listCmd.PersistentFlags().StringSliceVar(&listingTagFiltersFlag, "tag", []string{}, "Filter EC2 resources given tags (case sensitive!). Ex: --tag Env=Production")
	listCmd.PersistentFlags().StringSliceVar(&listingTagKeyFiltersFlag, "tag-key", []string{}, "Filter EC2 resources given a tag key only (case sensitive!). Ex: --tag-key Env")
	listCmd.PersistentFlags().StringSliceVar(&listingTagValueFiltersFlag, "tag-value", []string{}, "Filter EC2 resources given a tag value only (case sensitive!). Ex: --tag-value Staging")
	listCmd.PersistentFlags().StringSliceVar(&listingColumnsFlag, "columns", []string{}, "Select the properties to display in the columns. Ex: --columns id,name,cidr")
	listCmd.PersistentFlags().BoolVar(&listOnlyIDs, "ids", false, "List only ids")
	listCmd.PersistentFlags().BoolVar(&noHeadersFlag, "no-headers", false, "Do not display headers")
	listCmd.PersistentFlags().BoolVar(&reverseFlag, "reverse", false, "Use in conjunction with --sort to reverse sort")
	listCmd.PersistentFlags().StringSliceVar(&sortBy, "sort", []string{"Id"}, "Sort tables by column(s) name(s)")
}

var listCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Example:           "  awless-ro list instances --sort uptime\n  awless-ro list users --format csv\n  awless-ro list volumes --filter state=use --filter type=gp2\n  awless-ro list volumes --tag-value Purchased\n  awless-ro list vpcs --tag-key Dept --tag-key Internal\n  awless-ro list instances --tag Env=Production,Dept=Marketing\n  awless-ro list instances --filter state=running,type=micro\n  awless-ro list s3objects --filter bucket=pdf-bucket \n  awless-ro list accesskeys --filter state==Active",
	PersistentPreRun:  applyHooks(initLoggerHook, initAwlessEnvHook, initCloudServicesHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(onVersionUpgrade, networkMonitorHook),
	Short:             "List resources: sorting, filtering via tag/properties, output formatting, etc...",
}

var listSpecificResourceCmd = func(resType string) *cobra.Command {
	return &cobra.Command{
		ValidArgsFunction: cobra.NoFileCompletions,
		Use:               cloud.PluralizeResource(resType),
		Short:             fmt.Sprintf("[%s] List %s %s", awsservices.ServicePerResourceType[resType], strings.ToUpper(awsservices.APIPerResourceType[resType]), cloud.PluralizeResource(resType)),

		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				var plural string
				if len(args) > 1 {
					plural = "s"
				}
				logger.Errorf("invalid parameter%s '%s'", plural, strings.Join(args, " "))
				if strings.Contains(args[0], "=") {
					if !promptConfirmDefaultYes("Did you mean `awless-ro list %s --filter %s`? ", cloud.PluralizeResource(resType), strings.Join(args, " ")) {
						os.Exit(1)
					}
					listingFiltersFlag = append(listingFiltersFlag, args...)
				} else {
					os.Exit(1)
				}
			}
			var g cloud.GraphAPI

			if localGlobalFlag {
				warnIfNothingSynced()
				if srvName, ok := awsservices.ServicePerResourceType[resType]; ok {
					var err error
					g, err = sync.LoadLocalGraphForService(srvName, config.GetAWSProfile(), config.GetAWSRegion())
					exitOn(err)
				} else {
					exitOn(fmt.Errorf("cannot find service for resource type %s", resType))
				}
			} else {
				srv, err := cloud.GetServiceForType(resType)
				exitOn(err)
				fetchContext := context.WithValue(context.Background(), "force", true)
				g, err = srv.FetchByType(context.WithValue(fetchContext, "filters", listingFiltersFlag), resType)
				exitOn(err)
			}

			printResources(g, resType)
		},
	}
}

var listAllResourceInServiceCmd = func(srvName string) *cobra.Command {
	return &cobra.Command{
		ValidArgsFunction: cobra.NoFileCompletions,
		Use:               srvName,
		Short:             fmt.Sprintf("List all %s resources", srvName),
		Hidden:            true,

		Run: func(cmd *cobra.Command, args []string) {
			g, err := sync.LoadLocalGraphForService(srvName, config.GetAWSProfile(), config.GetAWSRegion())
			exitOn(err)
			displayer, err := console.BuildOptions(
				console.WithFormat(listingFormat),
				console.WithMaxWidth(console.GetTerminalWidth()),
				// --columns was accepted and ignored here, so asking a
				// service-wide listing for particular columns silently got the
				// default ones. There is no single resource type to look the
				// columns up for, so each name is read as a property, which is
				// what every resource has in common.
				console.WithColumns(listingColumnsFlag),
				console.WithIDsOnly(listOnlyIDs),
			).SetSource(g).Build()
			exitOn(err)
			exitOn(displayer.Print(os.Stdout))
		},
	}
}

// warnIfNothingSynced says so when --local is asked to read a local copy that does
// not exist yet, instead of letting the command answer "No results found." — which
// says something about the account, in a situation where the account was never read.
func warnIfNothingSynced() {
	profile, region := config.GetAWSProfile(), config.GetAWSRegion()
	if !sync.NothingSyncedFor(profile, region) {
		return
	}
	logger.Infof("nothing has been synced for profile '%s' in region '%s' yet, so --local has "+
		"nothing to read. Run `awless-ro sync`, or drop --local to ask AWS directly.", profile, region)
}

func printResources(g cloud.GraphAPI, resType string) {
	displayer, err := console.BuildOptions(
		console.WithRdfType(resType),
		console.WithColumns(listingColumnsFlag),
		console.WithFilters(listingFiltersFlag),
		console.WithTagFilters(listingTagFiltersFlag),
		console.WithTagKeyFilters(listingTagKeyFiltersFlag),
		console.WithTagValueFilters(listingTagValueFiltersFlag),
		console.WithMaxWidth(console.GetTerminalWidth()),
		console.WithFormat(listingFormat),
		console.WithIDsOnly(listOnlyIDs),
		console.WithSortBy(sortBy...),
		console.WithReverseSort(reverseFlag),
		console.WithNoHeaders(noHeadersFlag),
	).SetSource(g).Build()
	exitOn(err)

	exitOn(displayer.Print(os.Stdout))
}
