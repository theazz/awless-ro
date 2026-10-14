package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// stdout is where command output goes. A variable so that tests can read what a
// command printed; everything said about the output rather than the output itself
// goes to stderr, through the logger.
var stdout io.Writer = os.Stdout

// jsonOutput reports whether the command was asked for JSON.
func jsonOutput() bool {
	return formatGlobalFlag == "json"
}

// writeJSON encodes v as one JSON document, indented like `list --format json`.
func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(v)
}

// printJSON writes v to stdout as JSON and exits on failure, the way the text
// output of the commands does.
func printJSON(v interface{}) {
	exitOn(writeJSON(stdout, v))
}

// formatsAnnotation lists, in a command's Annotations, the --format values it can
// print, comma separated. A command without one inherits its parent's, and the
// default is text or JSON. "any" leaves the format to the command: list takes all
// five and has its own long-standing answer to an unknown one.
const formatsAnnotation = "formats"

// checkFormat refuses a --format the command cannot print. applyHooks and chainHooks
// call it before any hook, so every command goes through it, and a format that cannot
// be honoured is refused before anything touches AWS rather than silently answered in
// another: a program asking for JSON and receiving a table fails somewhere far less
// obvious.
func checkFormat(cmd *cobra.Command) error {
	allowed := formatsOf(cmd)
	for _, f := range allowed {
		if f == "any" || f == formatGlobalFlag {
			return nil
		}
	}
	quoted := make([]string, len(allowed))
	for i, f := range allowed {
		quoted[i] = "--format " + f
	}
	return fmt.Errorf("%s cannot print --format %s: use %s", cmd.CommandPath(), formatGlobalFlag, strings.Join(quoted, " or "))
}

// formatsOf is the --format values cmd takes, from its annotation or its parent's.
func formatsOf(cmd *cobra.Command) []string {
	for c := cmd; c != nil; c = c.Parent() {
		if formats, ok := c.Annotations[formatsAnnotation]; ok {
			return strings.Split(formats, ",")
		}
	}
	return []string{"table", "json"}
}

// completeFormat offers the formats the command being completed takes, so that
// completion does not suggest one checkFormat would then refuse.
func completeFormat(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	formats := formatsOf(cmd)
	if len(formats) == 1 && formats[0] == "any" {
		formats = []string{"table", "csv", "tsv", "json", "porcelain"}
	}
	return completeWith(formats, toComplete)
}
