package commands

import (
	"encoding/csv"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/theazz/awless-ro/cloud/match"
	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// parseTagFlags parses args the way `list` registers its tag flags. The args are
// what the shell hands over, after it has removed its own quoting. A fresh command
// per call, because a StringSlice appends on every Set after the first and would
// carry one row's values into the next.
func parseTagFlags(t *testing.T, args ...string) (tags, keys, values []string, err error) {
	t.Helper()
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().StringSliceVar(&tags, "tag", []string{}, "")
	cmd.Flags().StringSliceVar(&keys, "tag-key", []string{}, "")
	cmd.Flags().StringSliceVar(&values, "tag-value", []string{}, "")
	err = cmd.ParseFlags(args)
	return
}

// The owner chose to keep the flags as StringSlices, so the documented shorthand
// `--tag Env=Production,Dept=Marketing` keeps working (#28, option A). Switching to
// StringArray would fix the comma by breaking that, and this is where it would show.
// It also pins that parseTagFlags mirrors the real registration.
func TestListTagFlagsStayStringSlices(t *testing.T) {
	for _, name := range []string{"tag", "tag-key", "tag-value"} {
		f := listCmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Fatalf("list has no --%s flag", name)
		}
		if got := f.Value.Type(); got != "stringSlice" {
			t.Errorf("--%s is a %s, want stringSlice", name, got)
		}
	}
}

// Every row of the table measured for #28, with the outcome it must have now. The
// bug was the second row: the fragment " tagged" was dropped, and the command said
// "No results found." with exit status 0.
func TestTagFlagMeasuredBehaviour(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		parsed  []string
		wantErr []string // substrings of the validation error; nil means none expected
		absent  []string // substrings the error must not contain
	}{
		{
			name:   "shorthand gives two tags",
			args:   []string{"--tag", "Env=Prod,Dept=Mkt"},
			parsed: []string{"Env=Prod", "Dept=Mkt"},
		},
		{
			// Also what `--tag "Environment=Not, tagged"` delivers: the shell eats
			// the double quotes.
			name:    "unquoted comma value is refused",
			args:    []string{"--tag", "Environment=Not, tagged"},
			parsed:  []string{"Environment=Not", " tagged"},
			wantErr: []string{`" tagged"`, "key=value", `"Environment=Not", " tagged"`, `--tag '"Environment=Not, tagged"'`},
		},
		{
			name:   "quoted comma value is one tag",
			args:   []string{"--tag", `"Environment=Not, tagged"`},
			parsed: []string{"Environment=Not, tagged"},
		},
		{
			// A backslash is not an escape in CSV, so this splits like the bare
			// form, and the suggestion drops the backslash.
			name:    "backslash does not escape the comma",
			args:    []string{"--tag", `Environment=Not\, tagged`},
			parsed:  []string{`Environment=Not\`, " tagged"},
			wantErr: []string{`" tagged"`, `--tag '"Environment=Not, tagged"'`},
			absent:  []string{`'"Environment=Not\`},
		},
		{
			name:   "repeated flag gives two tags",
			args:   []string{"--tag", "Env=Prod", "--tag", "Dept=Mkt"},
			parsed: []string{"Env=Prod", "Dept=Mkt"},
		},
		{
			name:    "a key alone is refused",
			args:    []string{"--tag", "Env"},
			parsed:  []string{"Env"},
			wantErr: []string{`"Env"`, "e.g. --tag Env=Production", `--tag '"Env=Not, tagged"'`},
		},
		{
			name:    "a trailing comma is refused",
			args:    []string{"--tag", "Env=Prod,"},
			parsed:  []string{"Env=Prod", ""},
			wantErr: []string{`invalid --tag ""`},
		},
		{
			// The suggestion rebuilds the whole run of fragments, not just the first.
			name:    "a value split twice",
			args:    []string{"--tag", "a=1, b, c"},
			parsed:  []string{"a=1", " b", " c"},
			wantErr: []string{`invalid --tag " b"`, `--tag '"a=1, b, c"'`},
		},
		{
			name:   "wildcards are unaffected",
			args:   []string{"--tag", "Env=Prod*"},
			parsed: []string{"Env=Prod*"},
		},
		{
			// Every element has an "=", and the query builder trims the space.
			name:   "a space after the comma between two tags",
			args:   []string{"--tag", "Env=Prod, Dept=Mkt"},
			parsed: []string{"Env=Prod", " Dept=Mkt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags, keys, values, err := parseTagFlags(t, tc.args...)
			if err != nil {
				t.Fatalf("parse: %s", err)
			}
			if !reflect.DeepEqual(tags, tc.parsed) {
				t.Fatalf("parsed %q, want %q", tags, tc.parsed)
			}
			warnings, err := validateTagFilters(tags, keys, values)
			if len(warnings) != 0 {
				t.Errorf("unexpected warnings: %q", warnings)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %s", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted, want an error")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should contain %s, got:\n%s", want, err)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("error should not contain %s, got:\n%s", unwanted, err)
				}
			}
		})
	}
}

// The quoted form has to do more than pass validation: the one tag it yields must
// match a resource carrying that tag.
func TestQuotedCommaTagMatchesTheResource(t *testing.T) {
	tags, _, _, err := parseTagFlags(t, "--tag", `"Environment=Not, tagged"`)
	if err != nil {
		t.Fatal(err)
	}
	inst := resourcetest.Instance("i-0comma").Prop(properties.Tags, []string{"Environment=Not, tagged"}).Build()
	// Split the way the query builder does: first "=", both sides trimmed.
	kv := strings.SplitN(tags[0], "=", 2)
	if !match.Tag(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])).Match(inst) {
		t.Errorf("--tag %q does not match a resource tagged Environment=Not, tagged", tags[0])
	}
}

// A misplaced quote is a pflag parse error, not a validation one. The hint is added
// under pflag's message, which stays, and the error chain is kept.
func TestBareQuoteGetsTheQuotingHint(t *testing.T) {
	_, _, _, err := parseTagFlags(t, "--tag", `Environment="Not, tagged"`)
	if !errors.Is(err, csv.ErrBareQuote) {
		t.Fatalf("want a bare-quote error from pflag, got %v", err)
	}
	hinted := hintCSVQuoting(nil, err)
	if !errors.Is(hinted, csv.ErrBareQuote) {
		t.Error("the hint lost the original error")
	}
	if !strings.HasPrefix(hinted.Error(), err.Error()) {
		t.Errorf("pflag's message should come first, got:\n%s", hinted)
	}
	if !strings.Contains(hinted.Error(), `--tag '"Environment=Not, tagged"'`) {
		t.Errorf("the hint should show the quoting that works, got:\n%s", hinted)
	}
}

// Upstream wallix/awless#269: a value with a space and no comma. It must keep
// working on all three flags, with no warning.
func TestTagValueWithASpaceStaysAccepted(t *testing.T) {
	for _, args := range [][]string{
		{"--tag", "Environment=Not tagged"},
		{"--tag-value", "Not tagged"},
		{"--tag-key", "Cost center"},
	} {
		tags, keys, values, err := parseTagFlags(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(tags) + len(keys) + len(values); got != 1 {
			t.Errorf("%q parsed into %d elements, want 1", args, got)
		}
		warnings, err := validateTagFilters(tags, keys, values)
		if err != nil || len(warnings) != 0 {
			t.Errorf("%q: error %v, warnings %q", args, err, warnings)
		}
	}
	inst := resourcetest.Instance("i-0space").Prop(properties.Tags, []string{"Environment=Not tagged"}).Build()
	if !match.Tag("Environment", "Not tagged").Match(inst) {
		t.Error("Tag(Environment, Not tagged) no longer matches")
	}
}

// --tag-key and --tag-value only warn. Any string is a legal key or value, a leading
// space included, and after the split a correctly quoted '" x"' looks the same as
// the second half of 'Not, x', so an error would make such a value unreachable.
// The warning goes to stderr; the exit status and the output do not change.
func TestTagKeyAndValueWarnOnASplitAtCommaSpace(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		parsed   []string
		warnings [][]string // one entry per expected warning, with its substrings
	}{
		{
			name:     "value split at comma-space",
			args:     []string{"--tag-value", "Not, tagged"},
			parsed:   []string{"Not", " tagged"},
			warnings: [][]string{{`--tag-value " tagged"`, `"Not"`, `--tag-value '"Not, tagged"'`}},
		},
		{
			name:     "key split at comma-space",
			args:     []string{"--tag-key", "Cost, center"},
			parsed:   []string{"Cost", " center"},
			warnings: [][]string{{`--tag-key " center"`, `"Cost"`, `--tag-key '"Cost, center"'`}},
		},
		{
			name:   "quoted value",
			args:   []string{"--tag-value", `"Not, tagged"`},
			parsed: []string{"Not, tagged"},
		},
		{
			// Undetectable: both halves are plausible values. Documented, and
			// asserted so that changing it is a decision.
			name:   "split without a space goes unnoticed",
			args:   []string{"--tag-value", "Not,tagged"},
			parsed: []string{"Not", "tagged"},
		},
		{
			name:     "one warning per split value",
			args:     []string{"--tag-value", "a, b, c"},
			parsed:   []string{"a", " b", " c"},
			warnings: [][]string{{`--tag-value " b"`, `'"a, b, c"'`}},
		},
		{
			name:     "a leading space with nothing before it",
			args:     []string{"--tag-value", `" x"`},
			parsed:   []string{" x"},
			warnings: [][]string{{`--tag-value " x" starts with a space`, `--tag-value '"Not, tagged"'`}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags, keys, values, err := parseTagFlags(t, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got := append(keys, values...); !reflect.DeepEqual(got, tc.parsed) {
				t.Fatalf("parsed %q, want %q", got, tc.parsed)
			}
			warnings, err := validateTagFilters(tags, keys, values)
			if err != nil {
				t.Fatalf("--tag-key/--tag-value must not fail, got: %s", err)
			}
			if len(warnings) != len(tc.warnings) {
				t.Fatalf("got %d warnings, want %d: %q", len(warnings), len(tc.warnings), warnings)
			}
			for i, wants := range tc.warnings {
				for _, want := range wants {
					if !strings.Contains(warnings[i], want) {
						t.Errorf("warning should contain %s, got:\n%s", want, warnings[i])
					}
				}
			}
		})
	}
}

// The guard only helps if it is attached to the real command: the help text, and a
// flag error func that `list instances` and the other subcommands inherit.
func TestGuardIsWiredIntoList(t *testing.T) {
	for _, name := range []string{"tag", "tag-key", "tag-value"} {
		if usage := listCmd.PersistentFlags().Lookup(name).Usage; !strings.Contains(usage, `'"`) {
			t.Errorf("--%s help does not show the quoting: %s", name, usage)
		}
	}
	if len(listCmd.Commands()) == 0 {
		t.Fatal("list has no subcommands")
	}
	sub := listCmd.Commands()[0]
	_, _, _, err := parseTagFlags(t, "--tag", `Environment="Not, tagged"`)
	if got := sub.FlagErrorFunc()(sub, err); !strings.Contains(got.Error(), csvQuotingHint) {
		t.Errorf("list %s does not add the quoting hint, got:\n%s", sub.Name(), got)
	}
	other := errors.New("x")
	if got := sub.FlagErrorFunc()(sub, other); got != other {
		t.Errorf("an unrelated flag error was changed into %v", got)
	}
	if listCmd.PersistentPreRun == nil {
		t.Error("list lost its PersistentPreRun")
	}
}
