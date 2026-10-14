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

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/config"
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:               "version",
	ValidArgsFunction: cobra.NoFileCompletions,
	Short:             "Show awless-ro version",
	PreRun:            applyHooks(),

	Run: printVersion,
}

func printVersion(*cobra.Command, []string) {
	if jsonOutput() {
		printJSON(newVersionOutput(config.CurrentBuildInfo))
		return
	}
	fmt.Fprint(os.Stderr, config.AWLESS_ASCII_LOGO)
	fmt.Println(config.CurrentBuildInfo)
}

// versionOutput is `version --format json`. The fields the build did not set are
// left out, as the text form leaves them out.
type versionOutput struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	BuildFor  string `json:"buildFor,omitempty"`
}

func newVersionOutput(b config.BuildInfo) versionOutput {
	return versionOutput{Version: b.Version, Commit: b.Sha, BuildDate: b.Date, OS: b.OS, Arch: b.Arch, BuildFor: b.For}
}
