package console

import (
	"strings"
	"testing"

	"github.com/olekukonko/tablewriter"
)

// longARN is a synthetic 144-character stack ARN, the kind of value that made tables
// unreadable when wrapped at a constant width.
const longARN = "arn:aws:cloudformation:eu-west-1:123456789012:stack/my-production-application-compute-stack-eu-west-1-green/1a2b3c4d-5e6f-7890-abcd-ef0123456789"

func TestWrapCell(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		want  []string // expected lines; nil = only check the invariants
		// joined: the lines concatenate back to the input (a token broken
		// without adding or removing anything).
		joined bool
	}{
		{name: "fits", in: "short value", width: 20, want: []string{"short value"}},
		{name: "exactly fits", in: "0123456789", width: 10, want: []string{"0123456789"}},
		{name: "no limit", in: longARN, width: 0, want: []string{longARN}},
		{name: "negative is no limit", in: longARN, width: -5, want: []string{longARN}},
		{name: "spaces filled greedily", in: "my very long line with spaces", width: 8,
			want: []string{"my very", "long", "line", "with", "spaces"}},
		{name: "arn breaks after separators", in: longARN, width: 40, joined: true,
			want: []string{
				"arn:aws:cloudformation:eu-west-1:",
				"123456789012:stack/my-production-",
				"application-compute-stack-eu-west-1-",
				"green/1a2b3c4d-5e6f-7890-abcd-",
				"ef0123456789",
			}},
		{name: "arn wide budget", in: longARN, width: 120, joined: true,
			want: []string{
				"arn:aws:cloudformation:eu-west-1:123456789012:stack/my-production-application-compute-stack-eu-west-1-green/1a2b3c4d-",
				"5e6f-7890-abcd-ef0123456789",
			}},
		{name: "no break characters: hard cut", in: "abcdefghijklmnopqrstuvwxyz", width: 10, joined: true,
			want: []string{"abcdefghij", "klmnopqrst", "uvwxyz"}},
		{name: "existing newlines kept", in: "first line\nsecond", width: 6,
			want: []string{"first", "line", "second"}},
		{name: "ansi escapes are zero-width and never split",
			in: "\x1b[36mKey\x1b[0m:value-that-is-long", width: 10, joined: true,
			want: []string{"\x1b[36mKey\x1b[0m:value-", "that-is-", "long"}},
		{name: "wide runes count two", in: "日本語のテキスト", width: 6, joined: true,
			want: []string{"日本語", "のテキ", "スト"}},
		{name: "accented text", in: "élévation façade", width: 9,
			want: []string{"élévation", "façade"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapCell(tc.in, tc.width)
			lines := strings.Split(got, "\n")
			if tc.want != nil && strings.Join(lines, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("width %d:\n got %q\nwant %q", tc.width, lines, tc.want)
			}
			if tc.joined && strings.Join(lines, "") != tc.in {
				t.Errorf("lines do not concatenate back to the input: %q", lines)
			}
			if tc.width > 0 {
				for _, l := range lines {
					if w := tablewriter.DisplayWidth(l); w > tc.width {
						t.Errorf("line %q is %d wide, limit %d", l, w, tc.width)
					}
				}
			}
		})
	}
}

// A rune wider than the column cannot be made to fit; it gets a line of its own
// instead of looping or being dropped.
func TestWrapCellRuneWiderThanColumn(t *testing.T) {
	got := wrapCell("ab日cd", 1)
	if want := "a\nb\n日\nc\nd"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
