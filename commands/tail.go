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
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/aws/tailers"
	"github.com/theazz/awless-ro/cloud"
)

var tailFollowFrequencyFlag time.Duration
var tailEnableFollowFlag bool
var tailNumberEventsFlag int
var stackEventsFilters []string
var stackEventsTailTimeout time.Duration

func init() {
	RootCmd.AddCommand(tailCmd)

	tailCmd.PersistentFlags().DurationVar(&tailFollowFrequencyFlag, "frequency", 10*time.Second, "Fetch refresh frequency")
	tailCmd.PersistentFlags().BoolVar(&tailEnableFollowFlag, "follow", false, "Periodically refresh and append new data to output")
	tailCmd.PersistentFlags().IntVarP(&tailNumberEventsFlag, "number", "n", 10, "Number of events to display")

	tailCmd.AddCommand(scalingActivitiesCmd)

	stackEventsCmd.PersistentFlags().StringArrayVar(&stackEventsFilters, "filters",
		[]string{awstailers.StackEventTimestamp, awstailers.StackEventLogicalID, awstailers.StackEventType, awstailers.StackEventStatus},
		fmt.Sprintf("Filter the output columns. Valid filters: %s, %s, %s, %s, %s",
			awstailers.StackEventLogicalID,
			awstailers.StackEventStatus,
			awstailers.StackEventStatusReason,
			awstailers.StackEventTimestamp,
			awstailers.StackEventType))

	stackEventsCmd.PersistentFlags().DurationVar(&stackEventsTailTimeout, "timeout", time.Duration(1*time.Hour), "Time to wait for stack update to complete, use with 'follow' flag")

	tailCmd.AddCommand(stackEventsCmd)
}

// tailCmd was hidden from help and completion since it first appeared upstream, as
// an experiment. It works, the README documents it, and a command people cannot find
// might as well not exist.
var tailCmd = &cobra.Command{
	Use:               "tail",
	PersistentPreRun:  applyHooks(initLoggerHook, initAwlessEnvHook, initCloudServicesHook, firstInstallDoneHook),
	PersistentPostRun: applyHooks(networkMonitorHook),
	Short:             "Show recent CloudFormation stack events or autoscaling activities, or follow them",
	Example: `  awless-ro tail stack-events my-stack                 # the last 10 events of a stack
  awless-ro tail stack-events my-stack --follow        # follow a deployment until it completes
  awless-ro tail scaling-activities -n 20              # the last 20 autoscaling activities
  awless-ro tail scaling-activities --follow           # wait for new ones`,
}

var scalingActivitiesCmd = &cobra.Command{
	Use:               "scaling-activities",
	ValidArgsFunction: cobra.NoFileCompletions,
	Short:             "Autoscaling activities across all groups, newest last",

	Run: func(cmd *cobra.Command, args []string) {
		exitOn(awstailers.NewScalingActivitiesTailer(tailNumberEventsFlag, tailEnableFollowFlag, tailFollowFrequencyFlag).Tail(os.Stdout))
	},
}

var stackEventsCmd = &cobra.Command{
	Use:               "stack-events STACK",
	ValidArgsFunction: resourceRefCompletion([]string{"cloudformation"}, []string{cloud.Stack}),
	Short:             "Events of a CloudFormation stack; --follow tracks a deployment in progress",

	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			exitOn(fmt.Errorf("expecting stack-name string"))
		}

		exitOn(awstailers.NewCloudformationEventsTailer(args[0], tailNumberEventsFlag, tailEnableFollowFlag, tailFollowFrequencyFlag, stackEventsFilters, stackEventsTailTimeout).Tail(os.Stdout))
	},
}
