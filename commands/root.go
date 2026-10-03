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
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	verboseGlobalFlag      bool
	extraVerboseGlobalFlag bool
	silentGlobalFlag       bool
	localGlobalFlag        bool
	noSyncGlobalFlag       bool
	forceGlobalFlag        bool
	versionGlobalFlag      bool
	awsRegionGlobalFlag    string
	awsProfileGlobalFlag   string
	awsColorGlobalFlag     string
	networkMonitorFlag     bool

	renderGreenFn    = color.New(color.FgGreen).SprintFunc()
	renderRedFn      = color.New(color.FgRed).SprintFunc()
	renderYellowFn   = color.New(color.FgYellow).SprintFunc()
	renderBlueFn     = color.New(color.FgBlue).SprintFunc()
	renderCyanBoldFn = color.New(color.FgCyan, color.Bold).SprintFunc()
)

func init() {
	RootCmd.PersistentFlags().BoolVarP(&verboseGlobalFlag, "verbose", "v", false, "Turn on verbose mode for all commands")
	RootCmd.PersistentFlags().BoolVarP(&extraVerboseGlobalFlag, "extra-verbose", "e", false, "Turn on extra verbose mode (including regular verbose) for all commands")
	RootCmd.PersistentFlags().BoolVar(&silentGlobalFlag, "silent", false, "Turn on silent mode for all commands: disable logging, etc...")
	RootCmd.PersistentFlags().BoolVarP(&localGlobalFlag, "local", "l", false, "Work offline only using locally synced resources")
	RootCmd.PersistentFlags().BoolVarP(&forceGlobalFlag, "force", "f", false, "Force the command and bypass confirmation prompts")
	RootCmd.PersistentFlags().BoolVar(&noSyncGlobalFlag, "no-sync", false, "Do not run any sync on command")
	RootCmd.PersistentFlags().StringVarP(&awsRegionGlobalFlag, "aws-region", "r", "", "Override AWS region temporarily for the current command")
	RootCmd.PersistentFlags().StringVarP(&awsProfileGlobalFlag, "aws-profile", "p", "", "Override AWS profile temporarily for the current command")
	RootCmd.PersistentFlags().StringVar(&awsColorGlobalFlag, "color", "auto", "Force enabling/disabling colors in display (auto, never, always)")
	RootCmd.PersistentFlags().BoolVar(&networkMonitorFlag, "network-monitor", false, "Debug requests with network monitor")
	RootCmd.PersistentFlags().MarkHidden("network-monitor")

	RootCmd.Flags().BoolVar(&versionGlobalFlag, "version", false, "Print awless-ro version")

	RootCmd.RegisterFlagCompletionFunc("aws-region", completeRegions)
	RootCmd.RegisterFlagCompletionFunc("aws-profile", completeProfiles)
	RootCmd.RegisterFlagCompletionFunc("color", fixedCompletion("auto", "never", "always"))

	RootCmd.SetUsageTemplate(customRootUsage)

	cobra.OnInitialize(func() {
		switch awsColorGlobalFlag {
		case "never":
			color.NoColor = true
		case "always":
			color.NoColor = false
		}
	})
}

var RootCmd = &cobra.Command{
	Use:   "awless-ro COMMAND",
	Short: "Explore your cloud, read-only",
	Long:  "awless-ro is a read-only CLI to explore and sync your cloud infrastructure",
	RunE: func(c *cobra.Command, args []string) error {
		if versionGlobalFlag {
			printVersion(c, args)
			return nil
		}
		return c.Usage()
	},
}

const customRootUsage = `USAGE:{{if .Runnable}}
  {{if .HasAvailableFlags}}{{appendIfNotPresent .UseLine "[flags]"}}{{else}}{{.UseLine}}{{end}}{{end}}{{if gt .Aliases 0}}

ALIASES:
  {{.NameAndAliases}}
{{end}}{{if .HasExample}}

EXAMPLES:
{{ .Example }}{{end}}{{ if .HasAvailableSubCommands}}

COMMANDS:{{range .Commands}}{{if .IsAvailableCommand }}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{ if .HasAvailableLocalFlags}}

FLAGS:
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{ if .HasAvailableInheritedFlags}}

GLOBAL FLAGS:
{{.InheritedFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsHelpCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{ if .HasAvailableSubCommands }}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
