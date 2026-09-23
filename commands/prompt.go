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
	"strings"
)

// promptConfirmDefaultYes asks the user a yes/no question on stderr, treating
// an empty answer as yes. It previously lived in the removed run.go; it is kept
// because read-only commands use it to offer a correction when arguments look
// like a mistyped filter.
func promptConfirmDefaultYes(msg string, a ...interface{}) bool {
	var yesorno string
	fmt.Fprintf(os.Stderr, "%s [Y/n] ", fmt.Sprintf(msg, a...))
	fmt.Scanln(&yesorno)
	switch strings.TrimSpace(strings.ToLower(yesorno)) {
	case "y", "yes", "":
		return true
	}
	return false
}
