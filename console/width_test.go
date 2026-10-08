package console

import (
	"bytes"
	"strings"
	"testing"

	"github.com/olekukonko/tablewriter"
	p "github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/graph/resourcetest"
)

// Synthetic stacks (account 123456789012, invented names) with the long ARNs that
// made tables unreadable: wrapped at a constant 35 columns on any terminal, and
// wrapped hardest when the output went to a pipe.
const (
	computeARN  = longARN
	computeName = "my-production-application-compute-stack-eu-west-1-green"
	networkARN  = "arn:aws:cloudformation:eu-west-1:123456789012:stack/my-production-application-network-stack-eu-west-1-blue/0f1e2d3c-4b5a-6978-8765-4321fedcba09"
	networkName = "my-production-application-network-stack-eu-west-1-blue"
)

// stackColumns are spelled out rather than taken from DefaultsColumnDefinitions,
// which another test in the package replaces.
var stackColumns = []ColumnDefinition{
	StringColumnDefinition{Prop: p.ID},
	StringColumnDefinition{Prop: p.Name},
	StringColumnDefinition{Prop: p.State},
	TimeColumnDefinition{StringColumnDefinition: StringColumnDefinition{Prop: p.Created}},
	TimeColumnDefinition{StringColumnDefinition: StringColumnDefinition{Prop: p.Modified}},
}

// fitStackGraph holds one stack whose table is about 194 columns wide: it fits a
// 218-column terminal.
func fitStackGraph() *graph.Graph {
	g := graph.NewGraph()
	g.AddResource(resourcetest.Stack(computeARN).Prop(p.Name, "web").Prop(p.State, "CREATE_COMPLETE").Build())
	return g
}

// wideStackGraph holds two stacks whose table is about 255 columns wide: it does not
// fit a 218-column terminal.
func wideStackGraph() *graph.Graph {
	g := graph.NewGraph()
	g.AddResource(
		resourcetest.Stack(networkARN).Prop(p.Name, networkName).Prop(p.State, "UPDATE_ROLLBACK_COMPLETE").Build(),
		resourcetest.Stack(computeARN).Prop(p.Name, computeName).Prop(p.State, "CREATE_COMPLETE").Build(),
	)
	return g
}

func physicalLines(out string) []string {
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

func maxLineWidth(lines []string) int {
	var widest int
	for _, l := range lines {
		widest = max(widest, tablewriter.DisplayWidth(l))
	}
	return widest
}

// lineWith returns the index of the line containing s, or -1.
func lineWith(lines []string, s string) int {
	for i, l := range lines {
		if strings.Contains(l, s) {
			return i
		}
	}
	return -1
}

// firstCells concatenates the first cell of every body line (after the header
// separator), i.e. what the lines of a wrapped first column read as when joined.
func firstCells(lines []string) string {
	var b strings.Builder
	for _, l := range lines[2:] {
		if !strings.HasPrefix(l, "|") {
			continue
		}
		b.WriteString(strings.TrimSpace(strings.Split(l, "|")[1]))
	}
	return b.String()
}

func renderStacks(t *testing.T, g *graph.Graph, maxwidth int) string {
	t.Helper()
	displayer, err := BuildOptions(
		WithRdfType("stack"),
		WithColumnDefinitions(stackColumns),
		WithMaxWidth(maxwidth),
	).SetSource(g).Build()
	if err != nil {
		t.Fatal(err)
	}
	var w bytes.Buffer
	if err := displayer.Print(&w); err != nil {
		t.Fatal(err)
	}
	return w.String()
}

func TestTableLayoutFollowsMaxWidth(t *testing.T) {
	t.Run("instances: fits, unchanged", func(t *testing.T) {
		displayer, _ := BuildOptions(
			WithRdfType("instance"),
			WithColumns([]string{"ID", "Name", "State", "Type", "PublicIP"}),
			WithSortBy("state", "name"),
			WithMaxWidth(55),
		).SetSource(createInfraGraph()).Build()
		expected := `|   ID   |  NAME  | STATE ▲ |   TYPE    | PUBLIC IP |
|--------|--------|---------|-----------|-----------|
| inst_3 | apache | running | t2.xlarge |           |
| inst_1 | redis  | running | t2.micro  | 1.2.3.4   |
| inst_2 | django | stopped | t2.medium |           |
`
		var w bytes.Buffer
		if err := displayer.Print(&w); err != nil {
			t.Fatal(err)
		}
		if got := w.String(); got != expected {
			t.Fatalf("got\n%s\nwant\n%s", got, expected)
		}
	})

	t.Run("instances: one column too many is dropped, the rest not wrapped", func(t *testing.T) {
		displayer, _ := BuildOptions(
			WithRdfType("instance"),
			WithColumns([]string{"ID", "Name", "State", "Type", "PublicIP"}),
			WithSortBy("state", "name"),
			WithMaxWidth(45),
		).SetSource(createInfraGraph()).Build()
		expected := `|   ID   |  NAME  | STATE ▲ |   TYPE    |
|--------|--------|---------|-----------|
| inst_3 | apache | running | t2.xlarge |
| inst_1 | redis  | running | t2.micro  |
| inst_2 | django | stopped | t2.medium |
Column truncated to fit terminal: 'Public IP'
`
		var w bytes.Buffer
		if err := displayer.Print(&w); err != nil {
			t.Fatal(err)
		}
		if got := w.String(); got != expected {
			t.Fatalf("got\n%s\nwant\n%s", got, expected)
		}
	})

	unlimitedFit := renderStacks(t, fitStackGraph(), 0)
	naturalFit := maxLineWidth(physicalLines(unlimitedFit))

	for _, tc := range []struct {
		name     string
		maxwidth int
	}{
		{"fit: no limit", 0},
		{"fit: 218-column terminal", 218},
		{"fit: 400 columns", 400},
		{"fit: exactly the natural width", naturalFit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := renderStacks(t, fitStackGraph(), tc.maxwidth)
			if out != unlimitedFit {
				t.Fatalf("differs from the unlimited layout:\n%s\nwant\n%s", out, unlimitedFit)
			}
			lines := physicalLines(out)
			if len(lines) != 3 {
				t.Errorf("%d lines, want header, separator and one row:\n%s", len(lines), out)
			}
			if lineWith(lines, computeARN) < 0 {
				t.Errorf("the ARN is not on one line:\n%s", out)
			}
			if strings.Contains(out, "truncated") {
				t.Errorf("columns dropped:\n%s", out)
			}
		})
	}

	t.Run("fit: one column short of the natural width", func(t *testing.T) {
		out := renderStacks(t, fitStackGraph(), naturalFit-1)
		lines := physicalLines(out)
		if w := maxLineWidth(lines); w > naturalFit-1 {
			t.Errorf("a line is %d wide, limit %d:\n%s", w, naturalFit-1, out)
		}
		if len(lines) != 4 {
			t.Errorf("%d lines, want the row on two:\n%s", len(lines), out)
		}
		if got := firstCells(lines); got != computeARN {
			t.Errorf("the ARN lines join to %q", got)
		}
	})

	t.Run("wide: no limit, one line per row", func(t *testing.T) {
		out := renderStacks(t, wideStackGraph(), 0)
		lines := physicalLines(out)
		if len(lines) != 4 {
			t.Fatalf("%d lines, want 4:\n%s", len(lines), out)
		}
		for _, row := range [][2]string{{computeARN, computeName}, {networkARN, networkName}} {
			if i := lineWith(lines, row[0]); i < 0 || !strings.Contains(lines[i], row[1]) {
				t.Errorf("%s and its name are not on one line:\n%s", row[1], out)
			}
		}
		if w := maxLineWidth(lines); w < 250 {
			t.Errorf("widest line %d, want the natural width (over 250)", w)
		}
	})

	t.Run("wide: 218-column terminal uses the width", func(t *testing.T) {
		out := renderStacks(t, wideStackGraph(), 218)
		lines := physicalLines(out)
		if w := maxLineWidth(lines); w > 218 || w <= 200 {
			t.Errorf("widest line %d, want over 200 and at most 218:\n%s", w, out)
		}
		if len(lines) > 2+2*2 {
			t.Errorf("%d lines, want each row on at most two:\n%s", len(lines), out)
		}
		if strings.Contains(out, "truncated") {
			t.Errorf("columns dropped:\n%s", out)
		}
		for _, name := range []string{computeName, networkName} {
			if lineWith(lines, name) < 0 {
				t.Errorf("name %s is wrapped although it fits:\n%s", name, out)
			}
		}
		if got := firstCells(lines); got != computeARN+networkARN {
			t.Errorf("the ARN lines join to %q", got)
		}
	})

	t.Run("wide: 60 columns drops what cannot fit", func(t *testing.T) {
		out := renderStacks(t, wideStackGraph(), 60)
		lines := physicalLines(out)
		notice := "Columns truncated to fit terminal: 'Name', 'State', 'Created', 'Modified'"
		if last := lines[len(lines)-1]; last != notice {
			t.Errorf("last line %q, want %q", last, notice)
		}
		if w := maxLineWidth(lines[:len(lines)-1]); w > 60 {
			t.Errorf("a table line is %d wide, limit 60:\n%s", w, out)
		}
		if got := firstCells(lines[:len(lines)-1]); got != computeARN+networkARN {
			t.Errorf("the ARN lines join to %q", got)
		}
	})
}

// --max-width is about laying out a table; every other format is for programs and
// must come out byte for byte the same whatever the width.
func TestNonTableFormatsIgnoreMaxWidth(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []optsFn
	}{
		{"csv", []optsFn{WithFormat("csv")}},
		{"tsv", []optsFn{WithFormat("tsv")}},
		{"json", []optsFn{WithFormat("json")}},
		{"porcelain", []optsFn{WithFormat("porcelain")}},
		{"ids", []optsFn{WithIDsOnly(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var outputs []string
			for _, maxwidth := range []int{0, 20, 400} {
				opts := append([]optsFn{WithRdfType("stack"), WithColumnDefinitions(stackColumns)}, tc.opts...)
				opts = append(opts, WithMaxWidth(maxwidth))
				displayer, err := BuildOptions(opts...).SetSource(wideStackGraph()).Build()
				if err != nil {
					t.Fatal(err)
				}
				var w bytes.Buffer
				if err := displayer.Print(&w); err != nil {
					t.Fatal(err)
				}
				outputs = append(outputs, w.String())
			}
			if !strings.Contains(outputs[0], computeARN) {
				t.Fatalf("output does not hold the ARN whole:\n%s", outputs[0])
			}
			for i, maxwidth := range []int{20, 400} {
				if outputs[i+1] != outputs[0] {
					t.Errorf("max width %d changed the output:\n%s\nwant\n%s", maxwidth, outputs[i+1], outputs[0])
				}
			}
		})
	}
}

func TestResourceDisplayFollowsMaxWidth(t *testing.T) {
	r, err := fitStackGraph().GetResource("stack", computeARN)
	if err != nil {
		t.Fatal(err)
	}
	render := func(maxwidth int) []string {
		displayer, _ := BuildOptions(
			WithColumnDefinitions(stackColumns),
			WithFormat("table"),
			WithMaxWidth(maxwidth),
		).SetSource(r).Build()
		var w bytes.Buffer
		if err := displayer.Print(&w); err != nil {
			t.Fatal(err)
		}
		return physicalLines(w.String())
	}

	if lines := render(0); lineWith(lines, computeARN) < 0 {
		t.Errorf("no limit: the ARN is not on one line:\n%s", strings.Join(lines, "\n"))
	}

	lines := render(80)
	if w := maxLineWidth(lines); w > 80 {
		t.Errorf("a line is %d wide, limit 80:\n%s", w, strings.Join(lines, "\n"))
	}
	var values strings.Builder
	for _, l := range lines[2:] {
		values.WriteString(strings.TrimSpace(strings.Split(l, "|")[2]))
	}
	if lineWith(lines, computeARN) >= 0 || !strings.Contains(values.String(), computeARN) {
		t.Errorf("at 80 columns the ARN should be wrapped and its lines join back to it:\n%s", strings.Join(lines, "\n"))
	}
}

// The hidden `list <service>` table: shared out the same way, never dropping a column.
func TestMultiResourcesDisplayFollowsMaxWidth(t *testing.T) {
	g := createInfraGraph()
	g.AddResource(resourcetest.Stack(computeARN).Prop(p.Name, "web").Build())
	render := func(maxwidth int) []string {
		displayer, _ := BuildOptions(WithFormat("table"), WithMaxWidth(maxwidth)).SetSource(g).Build()
		var w bytes.Buffer
		if err := displayer.Print(&w); err != nil {
			t.Fatal(err)
		}
		return physicalLines(w.String())
	}

	if lines := render(0); lineWith(lines, computeARN) < 0 {
		t.Errorf("no limit: the ARN is not on one line:\n%s", strings.Join(lines, "\n"))
	}
	lines := render(80)
	if w := maxLineWidth(lines); w > 80 {
		t.Errorf("a line is %d wide, limit 80:\n%s", w, strings.Join(lines, "\n"))
	}
	if lineWith(lines, "VALUE") < 0 || lineWith(lines, "inst_3") < 0 {
		t.Errorf("a column went missing:\n%s", strings.Join(lines, "\n"))
	}
}
