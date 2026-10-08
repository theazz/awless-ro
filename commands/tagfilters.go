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
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/logger"
)

// The tag flags are StringSlices, so pflag splits every value at its commas, CSV
// style. That is what makes the documented shorthand `--tag
// Env=Production,Dept=Marketing` work, and it stays. The cost is that a tag value
// containing a comma is cut in two before awless-ro sees it: `--tag
// 'Environment=Not, tagged'` arrives as "Environment=Not" and " tagged". The second
// half has no "=", and the query builder used to drop it without a word, so the
// command answered "No results found." and exited 0. Everything below turns that
// into a refusal that names the fragment and shows the quoting that works.

// csvQuotingHint is printed under pflag's own message when a slice flag fails to
// parse as CSV, typically `--tag 'Environment="Not, tagged"'`, where the quote
// does not open the field.
const csvQuotingHint = `List flags read their value as comma-separated, CSV-style: a value that contains a comma or a double quote is quoted as a whole, with double quotes inside single quotes for the shell, e.g. --tag '"Environment=Not, tagged"'. A literal " inside it is written "".`

// validateTagFilters checks the tag flags as pflag left them, before anything
// talks to AWS.
//
// A --tag element without "=" can never be a tag filter, so it is an error. For
// --tag-key and --tag-value there is no such test: any string is a valid key or
// value. An element that starts with whitespace is the tell-tale of a split at ", ",
// but a leading space is also legal in an AWS tag, and once split, a correctly
// quoted '" x"' looks exactly the same. Refusing it would leave such a value
// unreachable, so it only earns a warning.
func validateTagFilters(tags, keys, values []string) (warnings []string, err error) {
	for i, t := range tags {
		if strings.Contains(t, "=") {
			continue
		}
		if i == 0 {
			return nil, fmt.Errorf("invalid --tag %q: expected key=value, e.g. --tag Env=Production. To match a tag whose value contains a comma, quote it: --tag '\"Env=Not, tagged\"'", t)
		}
		// tags[i-1] has an "=" because i is the first element without one.
		run := tags[i-1 : endOfRun(tags, i, func(s string) bool { return !strings.Contains(s, "=") })]
		return nil, fmt.Errorf("invalid --tag %q: expected key=value. Commas separate tags, so this was read as %s. To match a tag whose value contains a comma, quote it: --tag %s",
			t, quoteAll(run), suggestQuoted(run, "Env=Not, tagged"))
	}

	startsWithSpace := func(s string) bool { return s != strings.TrimLeftFunc(s, unicode.IsSpace) }
	for _, f := range []struct {
		flag, what, example string
		elems               []string
	}{
		{"--tag-key", "key", "Cost, center", keys},
		{"--tag-value", "value", "Not, tagged", values},
	} {
		for i := 0; i < len(f.elems); i++ {
			if !startsWithSpace(f.elems[i]) {
				continue
			}
			end := endOfRun(f.elems, i, startsWithSpace)
			if i == 0 {
				warnings = append(warnings, fmt.Sprintf("%s %q starts with a space. To match a %s that contains a comma, quote it: %s %s",
					f.flag, f.elems[i], f.what, f.flag, suggestQuoted(nil, f.example)))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s %q starts with a space, which usually means a comma split it from %q. To match a %s that contains a comma, quote it: %s %s",
					f.flag, f.elems[i], f.elems[i-1], f.what, f.flag, suggestQuoted(f.elems[i-1:end], f.example)))
			}
			// One warning per split value, not one per fragment.
			i = end - 1
		}
	}
	return warnings, nil
}

// endOfRun returns the index just past the run of elements from start on that
// satisfy in.
func endOfRun(elems []string, start int, in func(string) bool) int {
	end := start
	for end < len(elems) && in(elems[end]) {
		end++
	}
	return end
}

func quoteAll(elems []string) string {
	quoted := make([]string, len(elems))
	for i, e := range elems {
		quoted[i] = fmt.Sprintf("%q", e)
	}
	return strings.Join(quoted, ", ")
}

// suggestQuoted rebuilds what the user most likely typed and shows it in the
// quoting that survives both the shell and pflag. It is a hint and nothing more:
// the fragments are never re-joined for matching. A trailing backslash on a
// fragment is dropped, since it was an attempt to escape the comma, which CSV
// does not support. When the rebuilt value holds a quote of either kind, the
// single-around-double form would need escaping the user is unlikely to copy
// correctly, so the fixed example is shown instead.
func suggestQuoted(run []string, example string) string {
	parts := make([]string, len(run))
	for i, p := range run {
		parts[i] = strings.TrimSuffix(p, `\`)
	}
	value := strings.Join(parts, ",")
	if value == "" || strings.ContainsAny(value, `'"`) {
		value = example
	}
	return `'"` + value + `"'`
}

// checkTagFiltersHook is the hook form of validateTagFilters, so it composes with
// applyHooks: a returned error ends the run on stderr with exit status 1.
func checkTagFiltersHook(*cobra.Command, []string) error {
	warnings, err := validateTagFilters(listingTagFiltersFlag, listingTagKeyFiltersFlag, listingTagValueFiltersFlag)
	for _, w := range warnings {
		logger.Warning(w)
	}
	return err
}

// hintCSVQuoting adds csvQuotingHint to a CSV quoting error and leaves every other
// flag error alone. pflag keeps the csv error in its chain, so errors.Is finds it
// without comparing messages or importing pflag.
func hintCSVQuoting(_ *cobra.Command, err error) error {
	if errors.Is(err, csv.ErrBareQuote) || errors.Is(err, csv.ErrQuote) {
		return fmt.Errorf("%w\n%s", err, csvQuotingHint)
	}
	return err
}

// guardTagFilterFlags documents and checks the tag flags of cmd, which must
// already have registered them as persistent flags.
func guardTagFilterFlags(cmd *cobra.Command) {
	// Appended rather than written into the registration, so the original
	// sentence and example stay exactly as they were.
	for name, example := range map[string]string{
		"tag":       `--tag '"Env=Not, tagged"'`,
		"tag-key":   `--tag-key '"Cost, center"'`,
		"tag-value": `--tag-value '"Not, tagged"'`,
	} {
		f := cmd.PersistentFlags().Lookup(name)
		f.Usage += ". Commas separate several; quote one that contains a comma: " + example
	}

	// Subcommands inherit the flag error func, so `list instances` gets it too.
	cmd.SetFlagErrorFunc(hintCSVQuoting)

	// The check has to run before the existing hooks, not in Run: those resolve
	// credentials, which may assume a role or prompt for an MFA code, and may sync
	// the whole account. A command line that cannot be answered should not get
	// that far. initLoggerHook comes first so that --silent also silences the
	// warnings; running it again in next is harmless.
	check := applyHooks(initLoggerHook, checkTagFiltersHook)
	next := cmd.PersistentPreRun
	cmd.PersistentPreRun = func(c *cobra.Command, args []string) {
		check(c, args)
		next(c, args)
	}
}
