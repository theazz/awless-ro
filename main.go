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

package main

import (
	"os"

	"github.com/theazz/awless-ro/commands"
)

func main() {
	// The exit status was discarded here, so every failure reported itself on stderr
	// and then exited 0. That is wrong for any command, and actively misleading for
	// the ones built to be read by a script: `id=$(awless-ro search images canonical
	// --latest-id)` looked like it had succeeded with an empty id when the lookup had
	// failed. cobra has already printed the error, so there is nothing to add.
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
