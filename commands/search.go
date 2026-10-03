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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	awsimage "github.com/theazz/awless-ro/aws/image"
	"github.com/theazz/awless-ro/aws/services"
	"github.com/theazz/awless-ro/config"
	"github.com/theazz/awless-ro/logger"
)

var (
	latestImageIDOnly bool
	imageOwnerFlag    string
	imageNameFlag     string
)

func init() {
	RootCmd.AddCommand(searchCmd)

	awsImagesCmd.Flags().BoolVar(&latestImageIDOnly, "latest-id", false, "Print only the id of the newest matching image")
	awsImagesCmd.Flags().StringVar(&imageOwnerFlag, "owner", "", "Search a specific AWS account id instead of a known vendor")
	awsImagesCmd.Flags().StringVar(&imageNameFlag, "name", "", "Image name pattern, * allowed. Requires --owner")

	searchCmd.AddCommand(awsImagesCmd)
}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search AWS catalogues that are not part of your account",
}

var awsImagesCmd = &cobra.Command{
	Use:               "images",
	ValidArgsFunction: completeImageQuery,
	// The query is checked before anything touches AWS, so that a mistyped owner or
	// a --name without --owner is answered immediately instead of after a credential
	// prompt that was never going to help.
	PersistentPreRun:  applyHooks(validateImageSearchHook, initAwlessEnvHook, initLoggerHook, initCloudServicesHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(networkMonitorHook),
	Short:             "Resolve official AMIs in the current region, newest first",
	Long: fmt.Sprintf(`Resolve official AMIs in the current region, newest first.

A query is a colon-separated string, everything optional but the owner:

		%s

Known owners: %s

Every search is pinned to the account that publishes the images. Anyone may publish a
public AMI under any name, so a search by name alone would return whatever a stranger
has called "ubuntu-noble-latest"; that is the whoAMI name confusion attack. For the
same reason --name requires --owner, and the account id you give is used as-is.

CoreOS and CentOS were dropped: CoreOS Container Linux ended in 2020, CentOS Linux 7
in June 2024, and the account publishing CentOS Stream images could not be verified.
Use --owner with --name for those, or for any vendor not listed.`,
		awsimage.QuerySpec, strings.Join(awsimage.SupportedOwners(), ", ")),
	Example: `  awless-ro search images canonical
  awless-ro search images canonical --latest-id
  awless-ro search images canonical:ubuntu:jammy
  awless-ro search images debian:::arm64
  awless-ro search images redhat::9
  awless-ro search images amazonlinux:amzn2
  awless-ro search images windows::2025
  awless-ro search images --owner 123456789012 --name 'my-base-image-*'`,

	RunE: func(cmd *cobra.Command, args []string) error {
		// Already known to be valid: validateImageSearchHook parsed it before AWS was
		// touched. Parsing is cheap and has no side effects, so doing it twice beats
		// smuggling the result through a package variable.
		search, err := buildImageSearch(args)
		if err != nil {
			return err
		}

		api, err := awsservices.EC2API()
		if err != nil {
			return err
		}

		logger.Verbosef("searching images in region '%s'", config.GetAWSRegion())

		images, err := awsimage.NewResolver(api).Resolve(context.Background(), search)
		if err != nil {
			return err
		}

		if latestImageIDOnly {
			fmt.Println(images[0].ID)
			return nil
		}

		encoded, err := json.MarshalIndent(images, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(encoded))
		return nil
	},
}

func validateImageSearchHook(cmd *cobra.Command, args []string) error {
	_, err := buildImageSearch(args)
	return err
}

func buildImageSearch(args []string) (awsimage.Search, error) {
	var search awsimage.Search

	// --name without --owner is refused here as well as in the resolver. Rejecting it
	// at the flag level says what to do about it; the check further down is what
	// guarantees no request can be built without an account.
	if imageNameFlag != "" && imageOwnerFlag == "" {
		return search, fmt.Errorf("--name requires --owner: searching by name alone returns images from any account, including ones published to be mistaken for the real thing")
	}

	if imageOwnerFlag != "" {
		if len(args) > 0 {
			return search, fmt.Errorf("give either a query or --owner, not both")
		}
		search.AccountID = imageOwnerFlag
		search.NamePattern = imageNameFlag
		return search, nil
	}

	if len(args) < 1 {
		return search, fmt.Errorf("expecting an image query (%s, everything optional but the owner) or --owner with --name.\nKnown owners: %s",
			awsimage.QuerySpec, strings.Join(awsimage.SupportedOwners(), ", "))
	}

	query, err := awsimage.ParseQuery(args[0])
	if err != nil {
		return search, err
	}
	search.Query = &query
	return search, nil
}

// completeImageQuery completes the owner, the first segment of a query. The segments
// after it are free-form per vendor, so nothing is guessed for them.
func completeImageQuery(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 || strings.Contains(toComplete, ":") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completeWith(awsimage.SupportedOwners(), toComplete)
}
