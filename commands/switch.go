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
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/config"
	"github.com/theazz/awless-ro/config"
)

func init() {
	RootCmd.AddCommand(switchCmd)
}

var switchRequestedProfile, switchRequestedRegion string

// switchFlagsError refuses -p/--aws-profile and -r/--aws-region on a switch that saves
// something. Those flags last for one command, while switch saves for every later one,
// so combining them only raised the question of which one wins. switch takes both the
// profile and the region as arguments instead.
func switchFlagsError(profileFlag, regionFlag string) error {
	if profileFlag == "" && regionFlag == "" {
		return nil
	}
	return errors.New("switch takes the profile and region as arguments, not -p/--aws-profile or -r/--aws-region: " +
		"awless-ro switch PROFILE REGION (e.g. awless-ro switch staging eu-west-1)")
}

var switchCmd = &cobra.Command{
	Use:               "switch [REGION] [PROFILE]",
	ValidArgsFunction: completeSwitch,
	Aliases:           []string{"sw"},
	Short:             "Quick way to switch awless-ro config to given profile and/or region",
	Example: `  awless-ro switch eu-west-2           # now using region eu-west-2'
  awless-ro switch mfa                 # now using profile mfa (with mfa a valid profile in ~/.aws/{config,credentials})
  awless-ro switch default us-west-1   # now using region us-west-1 and the default profile
  awless-ro sw eu-west-3 admin         # now using profile admin in region eu-west-3`,
	PersistentPreRun: applyHooks(initAwlessEnvHook, initLoggerHook),
	PersistentPostRun: applyHooks(
		includeHookIf(&config.TriggerSyncOnConfigUpdate, initCloudServicesHook),
		notifyOnRegionOrProfilePrecedenceHook,
	),

	Run: func(cmd *cobra.Command, args []string) {
		switchRequestedProfile, switchRequestedRegion = "", ""
		if len(args) < 1 || len(args) > 2 {
			if jsonOutput() {
				printJSON(currentProfileAndRegion())
				return
			}
			fmt.Fprintf(os.Stdout, "currently in region '%s' with profile '%s' (switch -h for help and examples)\n", config.GetAWSRegion(), config.GetAWSProfile())
			return
		}
		exitOn(switchFlagsError(awsProfileGlobalFlag, awsRegionGlobalFlag))
		for _, arg := range args {
			if awsconfig.IsValidRegion(arg) {
				exitOn(config.Set(config.RegionConfigKey, arg))
				switchRequestedRegion = arg
				continue
			}
			if awsconfig.IsValidProfile(arg) {
				exitOn(config.Set(config.ProfileConfigKey, arg))
				switchRequestedProfile = arg
				continue
			}
			exitOn(fmt.Errorf("could not find profile: '%s' in $HOME/.aws/{credentials,config}", arg))
		}
		// What was saved. An environment variable or the profile's own region may
		// still override it for the commands that follow; the warning about that goes
		// to stderr, from notifyOnRegionOrProfilePrecedenceHook.
		if jsonOutput() {
			printJSON(currentProfileAndRegion())
		}
	},
}

// profileAndRegion is the JSON form of switch.
type profileAndRegion struct {
	Profile string `json:"profile"`
	Region  string `json:"region"`
}

func currentProfileAndRegion() profileAndRegion {
	return profileAndRegion{Profile: config.GetAWSProfile(), Region: config.GetAWSRegion()}
}
